//! 列表面板的界面（STAGE-107，`docs/battle-ui.md`「列表面板」）：80×24 从右侧浮出、120×40 整栏换成列表，
//! 15 个道具逐个都能被高亮看到（PgUp、PgDn 翻页），收回宠物那一行的号是 `-`。

use std::time::Instant;

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    BattleAllow, BattleStart, BattleTurn, BattleUnit, Inventory, InventorySlot, Pet, Progress, TargetType, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

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
                // 汉字占两格，第二格是空串，跳过它，免得文字被拆开。
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 高亮行的号：屏幕上第一个 `▶` 后面的数字（`▶ 5`、`▶12`）；`-` 是收回那一行。
fn marker(screen: &str) -> Option<String> {
    let rest = screen.split('▶').nth(1)?;
    let text: String = rest
        .trim_start()
        .chars()
        .take_while(|c| c.is_ascii_digit() || *c == '-')
        .collect();
    Some(text)
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn item(slot: u8, name: &str, target: TargetType, effect: &str) -> InventorySlot {
    InventorySlot {
        slot,
        item_id: i32::from(slot),
        name: name.into(),
        count: 1,
        effect: effect.into(),
        target_type: target,
        ..InventorySlot::default()
    }
}

fn bag() -> Vec<InventorySlot> {
    (5..=19u8)
        .map(|slot| match slot {
            9 => item(slot, "小斧头", TargetType::None, ""),
            12 => item(slot, "回复药", TargetType::Ally, "体100回复（1体）"),
            _ => item(slot, &format!("杂物{slot}"), TargetType::Ally, ""),
        })
        .collect()
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

/// 进行中的战斗（需要人物出招），背包 5–19 号都有东西，宠物由调用方给。
fn app_with(pets: Vec<Pet>) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow {
            escape: true,
            capture: true,
        },
        units: vec![
            unit(0, "player", "勇者小明", 12, 75),
            unit(5, "pet", "乌力", 5, 100),
            unit(15, "monster", "野猪", 3, 100),
            unit(16, "monster", "野猪", 3, 40),
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
            needs: vec!["player".into(), "pet".into()],
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        Instant::now(),
    );
    let mut world = WorldState::new(WorldEntry::default());
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.inventory = Inventory { gold: 0, slots: bag() };
    world.pets.pets = pets;
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

#[test]
fn every_one_of_the_fifteen_items_can_be_seen_and_picked_at_80x24() {
    let mut a = app_with(vec![]);
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains("道具 ·"), "没有打开列表时不出现");
    key(&mut a, KeyCode::Char('3'));
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("道具 ·"), "列表从右侧浮出\n{s}");
    assert!(
        s.contains("回复药") && s.contains("体100回复"),
        "默认高亮的回复药在第一页\n{s}"
    );
    // 从回复药（12 号）往上走到 5 号，再一行一行往下走：每一页都要有高亮行，走过的号都要高亮过。
    for _ in 0..7 {
        key(&mut a, KeyCode::Up);
    }
    let mut seen: Vec<String> = Vec::new();
    let mut saw_unusable = false;
    for _ in 5..=19u8 {
        let s = draw(&mut a, 80, 24);
        saw_unusable |= s.contains("9 杂物") || s.contains("9 小斧头");
        let mark = marker(&s).expect("每一页都有高亮行");
        if !seen.contains(&mark) {
            seen.push(mark);
        }
        key(&mut a, KeyCode::Down);
    }
    // 小斧头（9 号）战斗中不能用：灰色显示在列表里，但方向键跳过它，所以高亮不会落在它上面。
    let want: Vec<String> = (5..=19u8).filter(|&n| n != 9).map(|n| n.to_string()).collect();
    assert_eq!(seen, want, "能用的 14 个道具都能看到并高亮");
    assert!(saw_unusable, "不能用的 9 号也列在里面（灰色）");
}

#[test]
fn floating_list_preserves_the_battle_log_below_the_field() {
    let mut a = app_with(vec![]);
    let s = draw(&mut a, 180, 40);
    assert!(
        s.contains("战斗日志") && s.contains("敌左我右"),
        "战场在上，日志在下"
    );
    key(&mut a, KeyCode::Char('3'));
    let s = draw(&mut a, 180, 40);
    assert!(s.contains("道具 ·") && s.contains("回复药"), "宽屏时整栏换成列表\n{s}");
    assert!(s.contains("战斗日志"), "选择道具时日志仍保留");
    key(&mut a, KeyCode::Esc);
    let s = draw(&mut a, 180, 40);
    assert!(s.contains("战斗日志") && !s.contains("道具 ·"), "Esc 收起，右侧栏恢复");
}

#[test]
fn pet_list_shows_the_recall_row_with_a_dash() {
    let battle_pet = Pet {
        slot: 0,
        name: "乌力".into(),
        state: "battle".into(),
        hp: 35,
        max_hp: 35,
        level: 5,
        ..Pet::default()
    };
    let standby = Pet {
        slot: 1,
        name: "波利".into(),
        state: "standby".into(),
        hp: 20,
        max_hp: 20,
        level: 4,
        ..Pet::default()
    };
    let mut a = app_with(vec![battle_pet, standby]);
    key(&mut a, KeyCode::Char('7'));
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("换宠 ·"), "换宠列表打开\n{s}");
    assert!(s.contains(" 2 —"), "空栏位也列着，2 号是空的\n{s}");
    assert!(s.contains("Lv4 20/20") && s.contains("波利"), "1 号栏位的波利\n{s}");
    assert_eq!(marker(&s).as_deref(), Some("1"), "默认高亮第一只能上场的宠物");
    // STAGE-346：20 个栏位加收回共 21 行，收回那一行在最后一页，把高亮移过去才看得到。
    a.world.as_mut().unwrap().battle.as_mut().unwrap().list_focus = 20;
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("- 收回出战宠物"), "最后一行是收回，号是 -\n{s}");
}
