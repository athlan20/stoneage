//! STAGE-186：分身面板的本地按键流程和拦截。不连服务端，只走到“发出之前”；真正发出的请求见
//! `scenarios_party_clone.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::battle::{BattleState, Pick};
use stoneage_client::app::{App, Kind, Options, Screen, WorldState};
use stoneage_client::proto::{BattleAllow, BattleStart, BattleTurn, BattleUnit, Progress, WorldEntry};

fn unit(no: u8, kind: &str, name: &str, auto: bool) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level: 3, hp_pct: 100, auto, ..BattleUnit::default() }
}

/// 队长 + 一个分身（1 号）和它的宠物（6 号），敌方两只野猪；`optional` 是服务端列出的可指定分身。
fn clone_app(optional: &[&str], observer: bool) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![
            unit(0, "player", "勇者小明", false),
            unit(1, "player", "勇者小明·分身1", true),
            unit(5, "pet", "乌力", false),
            unit(6, "pet", "乌力", true),
            unit(15, "monster", "野猪", false),
            unit(16, "monster", "野猪", false),
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
            optional: optional.iter().map(|s| s.to_string()).collect(),
            progress: Progress { done: 0, total: 1 },
        },
        std::time::Instant::now(),
    );
    let mut world = WorldState::new(WorldEntry::default());
    world.online = true;
    if observer {
        world.entry.control.you = "observer".into();
    }
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn pick(a: &App) -> Pick {
    a.world.as_ref().unwrap().battle.as_ref().unwrap().pick
}

fn last_error(a: &App) -> String {
    a.log.all().iter().rev().find(|m| m.kind == Kind::Error).map(|m| m.text.clone()).unwrap_or_default()
}

#[test]
fn f_without_clones_explains() {
    let mut a = clone_app(&[], false);
    key(&mut a, KeyCode::Char('f'));
    assert_eq!(pick(&a), Pick::Action);
    assert!(last_error(&a).contains("没有可以指定目标的分身"), "{}", last_error(&a));
}

#[test]
fn panel_navigation_goes_clones_then_aim_then_back() {
    let mut a = clone_app(&["clone:1", "clone_pet:1"], false);
    key(&mut a, KeyCode::Char('f'));
    assert_eq!(pick(&a), Pick::Clones);
    key(&mut a, KeyCode::Down);
    assert_eq!(a.world.as_ref().unwrap().battle.as_ref().unwrap().clone_focus, 1);
    key(&mut a, KeyCode::Enter);
    assert_eq!(pick(&a), Pick::CloneAim { who: 6 }, "高亮的是分身 1 的宠物（6 号）");
    key(&mut a, KeyCode::Esc);
    assert_eq!(pick(&a), Pick::Clones);
    key(&mut a, KeyCode::Esc);
    assert_eq!(pick(&a), Pick::Action);
}

#[test]
fn aim_only_accepts_living_enemies_and_unlisted_clones_are_refused() {
    let mut a = clone_app(&["clone:1"], false);
    key(&mut a, KeyCode::Char('f'));
    key(&mut a, KeyCode::Char('6')); // 5–8 是分身的宠物；分身 1 的宠物（键 5）没列出
    assert_eq!(pick(&a), Pick::Clones);
    key(&mut a, KeyCode::Char('7'));
    assert_eq!(pick(&a), Pick::Clones, "分身 2 的宠物没列出");
    assert!(last_error(&a).contains("不能行动"), "{}", last_error(&a));
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(pick(&a), Pick::CloneAim { who: 1 });
    key(&mut a, KeyCode::Char('9')); // 敌方 9 号没有单位
    assert_eq!(pick(&a), Pick::CloneAim { who: 1 });
    assert!(last_error(&a).contains("没有单位"), "{}", last_error(&a));
}

#[test]
fn observer_cannot_open_the_panel() {
    let mut a = clone_app(&["clone:1"], true);
    key(&mut a, KeyCode::Char('f'));
    assert_eq!(pick(&a), Pick::Action);
    assert!(last_error(&a).contains("agent"), "观察模式下被拦并提示：{}", last_error(&a));
}

#[test]
fn typed_command_rejects_own_side_and_unlisted_clone() {
    let mut a = clone_app(&["clone:1"], false);
    key(&mut a, KeyCode::Char('/'));
    for c in "clone 1 m1".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("只能攻击敌方"), "{}", last_error(&a));
    for c in "clone 2 e1".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("不能指定目标"), "{}", last_error(&a));
}

fn draw(a: &mut App, w: u16, h: u16) -> String {
    use ratatui::Terminal;
    use ratatui::backend::TestBackend;
    use unicode_width::UnicodeWidthStr;
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| stoneage_client::ui::render(f, a)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..h)
        .map(|y| {
            let (mut line, mut x) = (String::new(), 0);
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

/// 80×24 的紧凑档也有入口：指令条写 f 分身，面板列出每行的目标，选完的分身卡片写“自→目标号”。
#[test]
fn compact_screen_keeps_the_entry_and_the_panel() {
    let mut a = clone_app(&["clone:1", "clone_pet:1"], false);
    let s = draw(&mut a, 80, 24);
    println!("{s}");
    assert!(s.contains("f 分身"), "{s}");
    key(&mut a, KeyCode::Char('f'));
    a.world.as_mut().unwrap().battle.as_mut().unwrap().clone_targets.insert(1, 16);
    let s = draw(&mut a, 80, 24);
    println!("{s}");
    assert!(s.contains("分身 ·") && s.contains("→ 敌 2") && s.contains("默认→ 敌 2"), "{s}");
    key(&mut a, KeyCode::Esc);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("自→2"), "{s}");
}

/// STAGE-186 × STAGE-173：托管（自动战斗）开着时，分身面板不受托管锁限制，本机角色的选招键仍被锁。
#[test]
fn clone_panel_works_while_auto_battle_is_on() {
    let mut a = clone_app(&["clone:1", "clone_pet:1"], false);
    a.world.as_mut().unwrap().auto_battle = true;
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(pick(&a), Pick::Action, "托管中数字选招仍被锁住");
    key(&mut a, KeyCode::Char('f'));
    assert_eq!(pick(&a), Pick::Clones, "托管中 f 能打开分身面板");
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert_eq!(pick(&a), Pick::CloneAim { who: 6 });
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Esc);
    assert_eq!(pick(&a), Pick::Action, "Esc 逐层退回，不被托管提示吞掉");
}

#[test]
fn auto_lock_hint_mentions_f_only_when_clones_are_listed() {
    let hint = |optional: &[&str]| {
        let mut a = clone_app(optional, false);
        a.world.as_mut().unwrap().auto_battle = true;
        key(&mut a, KeyCode::Char('1'));
        a.log.all().iter().rev().find(|m| m.kind == Kind::System).map(|m| m.text.clone()).unwrap_or_default()
    };
    assert!(hint(&["clone:1"]).contains("f 指定分身"));
    assert!(!hint(&[]).contains("f 指定分身"));
}
