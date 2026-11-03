//! 战斗日志区（STAGE-248，`docs/battle-ui.md`「动作文字与战斗日志」）的界面测试：
//! 日志区是消息日志的最近几条，战斗文字和系统、错误、命令、队伍、耐久按时间混排、带标签；
//! 底部不再单独画消息区；日志高度按终端高度分档；非战斗底部保持 9 行。
//! 战场直接用 `BattleState` 搭出来，不经过网络。

use std::time::Instant;

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{BattleState, Phase};
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleEnd, BattleEvent, BattleRound, BattleStart, BattleTurn, BattleUnit, Character,
    Location, Progress, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    })
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..h)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < w {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn unit(no: u8, kind: &str, name: &str, level: u32, pct: u32) -> BattleUnit {
    BattleUnit {
        no,
        kind: kind.into(),
        name: name.into(),
        level,
        hp_pct: pct,
        ..BattleUnit::default()
    }
}

/// 一场野外战斗（只有开场的记录，没有回合）。
fn field_battle() -> BattleState {
    let mut hero = unit(0, "player", "勇者小明", 12, 75);
    hero.hp = Some(45);
    hero.max_hp = Some(60);
    let boar = unit(15, "monster", "野猪", 3, 100);
    BattleState::from_start(BattleStart { party: None,
        field: None,
        battle_id: "b_1".into(),
        kind: "wild".into(),
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow {
            escape: true,
            capture: true,
        },
        units: vec![hero, boar],
        npc: None,
    })
}

/// 进入世界并挂上战斗；和真实界面一样先把开场记录写进消息日志。
fn battle_app(battle: BattleState) -> App {
    let mut a = app();
    let entry = WorldEntry {
        character: Character {
            slot: 0,
            name: "勇者小明".into(),
            level: 12,
            appearance: Appearance {
                body: 3,
                color: 2,
                eye: 1,
                mouth: 4,
            },
            location: Location {
                floor: 1000,
                map_name: "萨姆吉尔村".into(),
            },
        },
        ..WorldEntry::default()
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a.flush_battle_log();
    a
}

/// 屏幕上某段文字所在的行号（找不到就报错并打印整屏）。
fn row_of(screen: &str, want: &str) -> usize {
    screen
        .lines()
        .position(|l| l.contains(want))
        .unwrap_or_else(|| panic!("缺少“{want}”\n{screen}"))
}

#[test]
fn log_mixes_battle_and_other_messages_in_time_order_with_labels() {
    let mut a = battle_app(field_battle());
    a.log.system("系统甲");
    a.log.error("错误乙");
    a.log.command("命令丙");
    a.log.party("队伍丁");
    a.log.crush("耐久戊", false);
    a.log.crush("耐久己", true);
    a.log.battle("战斗庚");
    // 200×60 的日志区有 10 条，这里的 8 条都看得到。
    let s = draw(&mut a, 200, 60);
    let wants = [
        "战斗  进入战斗：野外",
        "系统  系统甲",
        "错误  错误乙",
        "命令  命令丙",
        "队伍  队伍丁",
        "耐久  耐久戊",
        "耐久  耐久己",
        "战斗  战斗庚",
    ];
    let rows: Vec<usize> = wants.iter().map(|w| row_of(&s, w)).collect();
    assert!(rows.windows(2).all(|p| p[0] < p[1]), "顺序应当按发生的时间\n{s}");
}

/// 每一行的文字（和 `draw` 的读法一致），按行号取。
fn line(screen: &str, y: usize) -> &str {
    screen.lines().nth(y).unwrap_or("")
}

#[test]
fn battle_bottom_has_no_message_area_and_each_text_shows_once() {
    let mut a = battle_app(field_battle());
    a.log.system("系统甲");
    let s = draw(&mut a, 120, 40);
    // 底部 4 行：消息框的标题在第 36 行（40 - 4），输入行在第 38 行，分隔线在第 37 行。
    assert_eq!(row_of(&s, "消息"), 36, "战斗中底部只有 4 行\n{s}");
    assert!(line(&s, 38).contains("> "), "输入行在第 38 行\n{s}");
    assert_eq!(s.matches("系统甲").count(), 1, "同一条文字只出现一次\n{s}");
    assert_eq!(s.matches("进入战斗：野外").count(), 1, "同一条文字只出现一次\n{s}");
}

#[test]
fn observer_menu_over_battle_keeps_the_normal_bottom() {
    let mut a = battle_app(field_battle());
    // 观察模式下菜单盖在战斗上（STAGE-182）：不算战斗在前台，底部照旧 9 行。
    a.world.as_mut().unwrap().battle_veil = true;
    a.log.system("系统甲");
    let s = draw(&mut a, 120, 40);
    assert_eq!(row_of(&s, "消息"), 31, "非战斗底部 9 行，消息框标题在第 31 行\n{s}");
    assert!(row_of(&s, "系统甲") > 31, "消息区照旧显示\n{s}");
}

#[test]
fn normal_bottom_keeps_nine_rows_with_five_message_rows() {
    let mut a = battle_app(field_battle());
    a.world.as_mut().unwrap().battle = None;
    for i in 1..=6 {
        a.log.system(format!("消息甲{i}"));
    }
    let s = draw(&mut a, 120, 40);
    assert_eq!(row_of(&s, "消息"), 31, "非战斗底部 9 行\n{s}");
    // 消息区 5 行：第 32–36 行，最近的 5 条，分隔线在第 37 行，输入行在第 38 行。
    assert_eq!(row_of(&s, "消息甲2"), 32, "{s}");
    assert_eq!(row_of(&s, "消息甲6"), 36, "{s}");
    assert!(!s.contains("消息甲1"), "最早的一条已经滚出去\n{s}");
    assert!(line(&s, 37).contains("────"), "分隔线在第 37 行\n{s}");
    assert!(line(&s, 38).contains("> "), "输入行在第 38 行\n{s}");
}

/// 日志区的边框：标题行到下一个带 `┘` 的行（指令区和日志区底边同一行）。
fn log_block(screen: &str) -> (usize, usize) {
    let top = row_of(screen, "战斗日志");
    let bottom = (top + 1..)
        .find(|&y| line(screen, y).contains('┘'))
        .expect("日志区有下边框");
    (top, bottom)
}

/// 四档尺寸（STAGE-248 表）：日志区总高、可见条数，战场至少 12 行。
#[test]
fn log_height_and_visible_rows_follow_the_four_sizes() {
    for (w, h, total, visible) in [(200, 60, 12, 10), (120, 40, 9, 7), (100, 30, 6, 4)] {
        let mut a = battle_app(field_battle());
        for i in 1..=20 {
            a.log.battle(format!("第{i}条"));
        }
        let s = draw(&mut a, w, h);
        let (top, bottom) = log_block(&s);
        assert_eq!(bottom - top + 1, total, "{w}×{h} 日志区总高\n{s}");
        let shown = (1..=20).filter(|i| s.contains(&format!("第{i}条"))).count();
        assert_eq!(shown, visible, "{w}×{h} 可见条数\n{s}");
        assert!(s.contains("第20条"), "最新的一条在最下面\n{s}");
        // 战斗块（从顶边到底部消息框的上边）减去日志区，剩下的战场至少 12 行。
        let field = row_of(&s, "消息") - row_of(&s, "战斗 · 野外") - usize::from(total);
        assert!(field >= 12, "{w}×{h} 战场只有 {field} 行\n{s}");
    }
}

/// 80×24 是窄屏：3 行指令 + 1 行日志，日志只显示最新的一条。
#[test]
fn narrow_80x24_keeps_three_command_rows_and_one_log_line() {
    let mut a = battle_app(field_battle());
    a.log.battle("第1条");
    a.log.battle("第2条");
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("战斗日志 · 战斗  第2条"), "{s}");
    assert!(!s.contains("第1条"), "窄屏只留最新一条\n{s}");
    assert_eq!(row_of(&s, "消息"), 20, "底部 4 行：指令 3 行 + 日志 1 行之后\n{s}");
}

/// 输入、回放、结算切换时底部高度和站位都不变（STAGE-248 验收 4）。
#[test]
fn phases_do_not_change_the_log_height_or_the_field_position() {
    let t0 = Instant::now();
    let mut input = field_battle();
    input.on_turn(
        BattleTurn {
            field: None,
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: vec!["player".into()],
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        t0,
    );
    assert!(matches!(input.phase, Phase::Input(_)));
    let mut replay = field_battle();
    replay.on_round(
        BattleRound {
            battle_id: "b_1".into(),
            turn: 1,
            events: vec![BattleEvent {
                kind: "attack".into(),
                actor: Some(0),
                ..BattleEvent::default()
            }],
            state: vec![],
        },
        t0,
    );
    assert_eq!(replay.phase, Phase::Replay);
    let mut ended = field_battle();
    ended.show_result(BattleEnd { duel_point: None,
        result: "win".into(),
        ..BattleEnd::default()
    });

    let mut seen = vec![];
    for b in [input, replay, ended] {
        let mut a = battle_app(b);
        let s = draw(&mut a, 120, 40);
        let (top, bottom) = log_block(&s);
        seen.push((row_of(&s, "消息"), bottom - top + 1, row_of(&s, "勇者小明")));
    }
    assert!(
        seen.windows(2).all(|p| p[0] == p[1]),
        "三个阶段的底部和站位应当一样：{seen:?}"
    );
    assert_eq!(seen[0].0, 36, "底部 4 行");
    assert_eq!(seen[0].1, 9, "120×40 的日志区 9 行");
}
