//! 战斗中聊天不被战斗文字冲走：聊天框或聊天命令开着时日志区只显示聊天；
//! 观察模式的简略日志档（`battle-log` 命令）。战场直接用 `BattleState` 搭出来，不经过网络。

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleStart, BattleUnit, Character, Location, WorldEntry,
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

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::proto::ChatChannel;

fn type_line(a: &mut App, s: &str) {
    for c in s.chars() {
        a.on_key(KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE));
    }
}

fn chatty_app() -> App {
    let mut a = battle_app(field_battle());
    a.log.chat(ChatChannel::Map, " 小红: 你好呀");
    for i in 0..12 {
        a.log.battle(format!("勇者 攻击 野猪 -{i}"));
    }
    a
}

#[test]
fn battle_text_pushes_chat_out_of_the_log_until_chat_is_focused() {
    let mut a = chatty_app();
    let s = draw(&mut a, 200, 60);
    assert!(!s.contains("你好呀"), "战斗文字刷屏后聊天看不到\n{s}");
}

#[test]
fn chat_command_in_the_command_bar_shows_only_chat() {
    let mut a = chatty_app();
    a.on_key(KeyEvent::new(KeyCode::Char('/'), KeyModifiers::NONE));
    type_line(&mut a, "say 在吗");
    assert!(a.chat_focus());
    let s = draw(&mut a, 200, 60);
    assert!(s.contains("你好呀"), "聊天命令开着时日志区显示聊天\n{s}");
    assert!(!s.contains("勇者 攻击 野猪"), "战斗文字让位\n{s}");
}

#[test]
fn other_commands_keep_the_battle_log() {
    let mut a = chatty_app();
    a.on_key(KeyEvent::new(KeyCode::Char('/'), KeyModifiers::NONE));
    type_line(&mut a, "status");
    assert!(!a.chat_focus());
}

#[test]
fn narrow_log_line_shows_the_latest_chat_while_chatting() {
    let mut a = chatty_app();
    a.on_key(KeyEvent::new(KeyCode::Char('/'), KeyModifiers::NONE));
    type_line(&mut a, "world hi");
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("聊天 ·") && s.contains("你好呀"), "窄屏聊天时那一行日志给聊天\n{s}");
}

#[test]
fn battle_log_command_sets_and_reports_the_mode() {
    use stoneage_client::app::battle::brief::LogMode;
    let mut a = battle_app(field_battle());
    a.on_key(KeyEvent::new(KeyCode::Char('/'), KeyModifiers::NONE));
    type_line(&mut a, "battle-log brief");
    a.on_key(KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE));
    assert_eq!(a.battle_log, LogMode::Brief);
    assert!(a.log.all().iter().any(|m| m.text.contains("战斗日志：简略")));
}

#[test]
fn observer_defaults_to_brief_and_own_control_to_full() {
    let mut a = battle_app(field_battle());
    a.flush_battle_log();
    assert!(!a.world.as_ref().unwrap().battle.as_ref().unwrap().brief.on, "自己操控默认完整");
    a.world.as_mut().unwrap().entry.control.you = "observer".into();
    a.flush_battle_log();
    assert!(a.world.as_ref().unwrap().battle.as_ref().unwrap().brief.on, "观察模式默认简略");
}
