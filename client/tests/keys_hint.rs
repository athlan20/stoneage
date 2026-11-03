//! 输入行提示与观察模式的按键拦截（STAGE-104，`docs/interaction.md`「提示行」「快捷键与命令」）。
//! 不连服务端：提示看输入行那一行渲染出来的文字，拦截看消息栏和本地状态；真正发出去的流程见 `scenarios_*.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{BattleState, Phase, Pick};
use stoneage_client::app::{App, Confirm, Kind, Options, Screen, WorldState};
use stoneage_client::proto::{
    BattleAllow, BattleEnd, BattleRound, BattleStart, BattleTurn, BattleUnit, Inventory, InventorySlot, NpcOption,
    NpcWindow, Pet, Pets, Progress, TargetType, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

const WIDTH: u16 = 120;
const HEIGHT: u16 = 40;

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    })
}

/// 进入世界、没有任何弹窗的应用。
fn world_app() -> App {
    let mut a = app();
    let mut w = WorldState::new(WorldEntry::default());
    w.nav.push(stoneage_client::app::Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

/// agent 正在操控（玩家是观察方）。
fn observed_app() -> App {
    let mut a = world_app();
    a.world.as_mut().unwrap().entry.control.you = "observer".into();
    a
}

fn pet_app() -> App {
    let mut a = world_app();
    a.world.as_mut().unwrap().pets = Pets {
        pets: vec![Pet {
            slot: 0,
            name: "乌力".into(),
            ..Pet::default()
        }],
        ..Pets::default()
    };
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn text(a: &mut App, s: &str) {
    if a.screen==Screen::World && a.world.as_ref().is_some_and(|w|!w.text_entry_active()){key(a,KeyCode::Char('/'));}
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

/// 输入一整行并回车（和手敲命令一样）。
fn line(a: &mut App, s: &str) {
    text(a, s);
    key(a, KeyCode::Enter);
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn last_error(a: &App) -> String {
    a.log
        .all()
        .iter()
        .rev()
        .find(|m| m.kind == Kind::Error)
        .map(|m| m.text.clone())
        .unwrap_or_default()
}

/// 消息栏里回显过的命令（按顺序）。
fn commands(a: &App) -> Vec<String> {
    a.log
        .all()
        .iter()
        .filter(|m| m.kind == Kind::Command)
        .map(|m| m.text.clone())
        .collect()
}

/// 画一帧，返回整屏文字（宽字符占两格，拼接时跳过占位格）。
fn draw(a: &mut App) -> String {
    let mut term = Terminal::new(TestBackend::new(WIDTH, HEIGHT)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..HEIGHT)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < WIDTH {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 输入行（底部消息框的最后一行内容）：提示就画在这一行右边。
fn input_row(a: &mut App) -> String {
    draw(a).lines().rev().nth(1).unwrap_or_default().to_string()
}

fn option(no: u32, label: &str) -> NpcOption {
    NpcOption {
        no,
        label: label.into(),
    }
}

fn window(options: Vec<NpcOption>) -> NpcWindow {
    NpcWindow {
        npc: 225,
        name: "村长家的厨师".into(),
        options,
        ..NpcWindow::default()
    }
}

fn unit(no: u8, kind: &str, name: &str, pct: u32) -> BattleUnit {
    BattleUnit {
        no,
        kind: kind.into(),
        name: name.into(),
        level: 3,
        hp_pct: pct,
        ..BattleUnit::default()
    }
}

/// 一场战斗，轮到我方勇者下指令（`needs` 里有 player）。
fn battle_app(needs: &[&str]) -> App {
    let mut a = world_app();
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow {
            escape: true,
            capture: true,
        },
        units: vec![
            unit(0, "player", "勇者小明", 100),
            unit(5, "pet", "乌力", 100),
            unit(15, "monster", "野猪", 100),
            unit(16, "monster", "野猪", 100),
        ],
        ..BattleStart::default()
    };
    let mut battle = BattleState::from_start(start);
    battle.on_turn(
        BattleTurn {
                field: None,
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: needs.iter().map(|s| s.to_string()).collect(),
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        std::time::Instant::now(),
    );
    a.world.as_mut().unwrap().battle = Some(battle);
    // 背包里有一瓶回复药，道具才能选（同 keys_battle.rs）。
    a.world.as_mut().unwrap().inventory = Inventory {
        gold: 0,
        slots: vec![InventorySlot {
            slot: 5,
            item_id: 1,
            name: "回复药".into(),
            count: 3,
            target_type: TargetType::Ally,
            ..InventorySlot::default()
        }],
    };
    a
}

fn battle(a: &App) -> &BattleState {
    world(a).battle.as_ref().unwrap()
}

// ---- 提示行 ----

#[test]
fn world_hint_lists_clone_creation_and_message_scroll_keys() {
    let mut a = world_app();
    // 快捷传送和社交均有独立入口提示；STAGE-162 已开放分身队。
    assert!(input_row(&mut a).contains("快捷传送") || input_row(&mut a).contains("Enter"), "{}", input_row(&mut a));
    key(&mut a, KeyCode::Char('6'));
    assert!(input_row(&mut a).contains("Enter 组成分身队 · / 命令"), "{}", input_row(&mut a));
    key(&mut a, KeyCode::Char('7'));
    assert!(input_row(&mut a).contains("←→ 栏目 · ↑↓ 滚动 · End 最新"), "消息页的 ↑↓ 逐行滚动（STAGE-116）");
}

#[test]
fn old_tab_and_history_hint_is_gone() {
    let mut a = world_app();
    assert!(!draw(&mut a).contains("Tab 补全"), "空行的提示只写当前层的键");
}

#[test]
fn dialog_hint_lists_option_keys_or_close_key() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![option(1, "买东西"), option(2, "卖东西")]));
    assert!(input_row(&mut a).contains("1-2 选择 · Esc 关闭"));

    world_mut(&mut a).npc_window = Some(window(vec![]));
    assert!(input_row(&mut a).contains("Enter 关闭"));

    world_mut(&mut a).npc_window = Some(NpcWindow {
        accepts_text: true,
        ..window(vec![])
    });
    assert!(input_row(&mut a).contains("reply 文本 · Esc 关闭"));
}

#[test]
fn confirm_hint_wins_over_the_dialog_under_it() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![option(1, "买东西")]));
    world_mut(&mut a).confirm = Some(Confirm::PetDrop {
        slot: 0,
        name: "乌力".into(),
    });
    assert!(input_row(&mut a).contains("y 确认 · 其他取消"));
    // 确认层在弹窗之上：数字是文字，不选选项。
    key(&mut a, KeyCode::Char('1'));
    assert!(world(&a).npc_window.is_some());
    assert_eq!(world(&a).input.text(), "1");
}

#[test]
fn typed_text_shows_the_command_usage_even_with_a_dialog_open() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![option(1, "买东西")]));
    text(&mut a, "talk");
    assert!(
        input_row(&mut a).contains("talk 编号"),
        "有字时显示命令用法，不显示按键"
    );
}

#[test]
fn battle_hints_follow_the_current_pick() {
    let mut a = battle_app(&["player"]);
    let row = input_row(&mut a);
    assert!(
        row.contains("指令") && !row.contains("Esc"),
        "指令条停在第一项，Esc 不起作用，不写 Esc：{row}"
    );

    // 1 是攻击，进入选目标。
    key(&mut a, KeyCode::Char('1'));
    assert!(matches!(battle(&a).pick, Pick::Target { .. }));
    assert!(input_row(&mut a).contains("←↑↓→ 选目标 · Enter 确认 · 数字直选 · Esc 返回"));

    // 道具列表（3 是道具）：Esc 退一步。
    let mut b = battle_app(&["player"]);
    key(&mut b, KeyCode::Char('3'));
    assert_eq!(battle(&b).pick, Pick::Item);
    assert!(input_row(&mut b).contains("数字选编号 · Esc 返回"));
}

#[test]
fn battle_replay_and_result_hints() {
    let mut a = battle_app(&["player"]);
    let round = BattleRound {
        turn: 1,
        ..BattleRound::default()
    };
    battle_mut(&mut a).on_round(round, std::time::Instant::now());
    assert_eq!(battle(&a).phase, Phase::Replay);
    let row = input_row(&mut a);
    assert!(!row.contains("跳过") && !row.contains("倍速"), "回放阶段没有按键提示（速度/跳过在系统页设）：{row}");

    // 结算：结果窗口开着时提示「Enter 继续」，关窗是本地操作，观察模式下也不换成观察模式的提示。
    let mut b = battle_app(&["player"]);
    battle_mut(&mut b).show_result(BattleEnd { duel_point: None,
        result: "win".into(),
        turn: 2,
        ..BattleEnd::default()
    });
    assert_eq!(battle(&b).phase, Phase::Ended);
    assert!(input_row(&mut b).contains("Enter 继续"));
    world_mut(&mut b).entry.control.you = "observer".into();
    assert!(input_row(&mut b).contains("Enter 继续"), "关结果窗口是本地操作，提示照常");
}

// ---- 观察模式的拦截 ----

#[test]
fn observer_digit_on_dialog_is_blocked_and_echoes_the_choice() {
    let mut a = observed_app();
    world_mut(&mut a).npc_window = Some(window(vec![option(1, "买东西"), option(2, "卖东西")]));
    key(&mut a, KeyCode::Char('1'));
    assert!(world(&a).npc_window.is_some(), "被拦下，窗口还开着");
    assert!(last_error(&a).contains("输入 take 收回控制权"), "{}", last_error(&a));
    assert_eq!(commands(&a), vec!["choose 1"], "回显等价的 choose 1");
    assert!(
        input_row(&mut a).contains("agent 操控中 · take 收回"),
        "操作键被拦时提示换成观察模式的提示"
    );
}

#[test]
fn digit_on_dialog_echoes_choose_before_sending() {
    let mut a = world_app();
    world_mut(&mut a).npc_window = Some(window(vec![option(1, "买东西"), option(2, "卖东西")]));
    key(&mut a, KeyCode::Char('2'));
    assert_eq!(commands(&a), vec!["choose 2"]);
}

#[test]
fn y_on_confirm_echoes_the_command_with_yes_instead_of_y() {
    let mut a = pet_app();
    line(&mut a, "pet drop 0");
    line(&mut a, "y");
    assert_eq!(
        commands(&a),
        vec!["pet drop 0", "pet drop 0 yes"],
        "y 回显成带 yes 的原命令"
    );
}

#[test]
fn observer_blocks_y_on_confirm_and_drops_the_question() {
    let mut a = observed_app();
    world_mut(&mut a).confirm = Some(Confirm::PetDrop {
        slot: 0,
        name: "乌力".into(),
    });
    line(&mut a, "y");
    assert!(world(&a).confirm.is_none(), "确认作废");
    assert!(last_error(&a).contains("输入 take 收回控制权"), "{}", last_error(&a));
}

#[test]
fn observer_is_blocked_from_battle_digits_and_enter_without_echo() {
    let mut a = battle_app(&["player"]);
    world_mut(&mut a).entry.control.you = "observer".into();
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(battle(&a).pick, Pick::Action, "数字不选指令");
    assert!(last_error(&a).contains("输入 take 收回控制权"), "{}", last_error(&a));
    key(&mut a, KeyCode::Enter);
    assert_eq!(battle(&a).pick, Pick::Action, "回车也不出招");
    assert!(commands(&a).is_empty(), "战斗中不回显");
    assert!(input_row(&mut a).contains("agent 操控中 · take 收回"));
}

#[test]
fn observer_leaves_battle_arrows_alone() {
    let mut a = battle_app(&["player"]);
    world_mut(&mut a).entry.control.you = "observer".into();
    // 左右方向键只是本地移动（不是操作），不拦。
    key(&mut a, KeyCode::Right);
    assert!(last_error(&a).is_empty(), "方向键不拦：{}", last_error(&a));
}

#[test]
fn observer_typed_battle_command_is_blocked() {
    let mut a = battle_app(&["player"]);
    world_mut(&mut a).entry.control.you = "observer".into();
    line(&mut a, "attack e3");
    assert!(last_error(&a).contains("输入 take 收回控制权"), "{}", last_error(&a));
    assert_eq!(world(&a).input.text(), "", "被拦的命令不留在输入行");
}

fn world_mut(a: &mut App) -> &mut WorldState {
    a.world.as_mut().expect("已进入世界")
}

fn battle_mut(a: &mut App) -> &mut BattleState {
    world_mut(a).battle.as_mut().unwrap()
}
