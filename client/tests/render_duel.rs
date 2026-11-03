//! STAGE-307 决斗的界面测试（`docs/ui.md`「决斗」条目、`docs/pk.md` 五）：附近玩家的「决」标记（`--unicode false` 为 D）、
//! 动作面板的「决斗」（灰掉时写原因）、社交页设置栏第四行「接受决斗」、决斗的战场标题与己方（you 为 10–14）、
//! 结果窗口的决斗点数（没有经验和掉落）、角色页的「决斗点数」。战场直接用 `BattleState` 搭，不经过网络。

use crossterm::event::KeyCode::{self, Char, Enter, Tab};
use crossterm::event::{KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{BattleState, slots};
use stoneage_client::app::{App, Options, SOCIAL_MENU, Screen, WorldState, social_settings};
use stoneage_client::proto::{BattleEnd, BattleStart, BattleUnit, DuelPoint, NearbyPlayer, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() });
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    w.entry.character.name = "勇者小明".into();
    w.entry.position.floor = 1000;
    w.entry.position.map_name = "萨姆吉尔村".into();
    w.entry.duel_point = 100;
    a.world = Some(w);
    a
}

/// 整屏的文字（宽字符跳过补位格），每行一段。
fn draw(a: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
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

fn line_with<'a>(screen: &'a str, key: &str) -> &'a str {
    screen.lines().find(|l| l.contains(key)).unwrap_or("")
}

fn player(name: &str, level: u32, duel: bool) -> NearbyPlayer {
    NearbyPlayer { name: name.into(), level, duel, ..NearbyPlayer::default() }
}

fn with_players(a: &mut App) {
    let w = a.world.as_mut().unwrap();
    w.scene.players.replace(vec![player("阿牛", 30, true), player("小红", 12, false)], false);
}

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

#[test]
fn nearby_player_who_accepts_duel_gets_a_kanji_mark() {
    let mut a = app(true);
    with_players(&mut a);
    let s = draw(&mut a, 120, 40);
    assert!(line_with(&s, "阿牛").contains("决"), "接受决斗的人行尾标「决」\n{s}");
    assert!(!line_with(&s, "小红").contains("决"), "没开的人不标\n{s}");
}

#[test]
fn nearby_player_mark_is_d_without_unicode() {
    let mut a = app(false);
    with_players(&mut a);
    let s = draw(&mut a, 120, 40);
    assert!(line_with(&s, "阿牛").contains(" D"), "--unicode false 为 D\n{s}");
    assert!(!line_with(&s, "阿牛").contains('决'), "降级时不用汉字\n{s}");
}

#[test]
fn duel_action_is_grayed_with_reason_for_a_player_who_refuses() {
    let mut a = app(true);
    with_players(&mut a);
    let w = a.world.as_mut().unwrap();
    w.scene.players.index = 1; // 小红：没开接受决斗
    w.scene.players.prompt = Some(3); // 动作面板里的「决斗」
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("决斗 · 对方没有接受决斗"), "灰掉并写原因\n{s}");
}

#[test]
fn duel_action_has_no_reason_for_an_accepting_player() {
    let mut a = app(true);
    with_players(&mut a);
    let w = a.world.as_mut().unwrap();
    w.scene.players.index = 0; // 阿牛：开着
    w.scene.players.prompt = Some(3);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("决斗") && !s.contains("没有接受决斗"), "能打时不写原因\n{s}");
}

#[test]
fn social_settings_has_accept_duel_row_with_explanation() {
    let mut a = app(true);
    key(&mut a, Char('/'));
    for c in "party".chars() {
        key(&mut a, Char(c));
    }
    key(&mut a, Enter);
    assert_eq!(a.world.as_ref().unwrap().menu, SOCIAL_MENU);
    key(&mut a, Tab);
    key(&mut a, Tab);
    assert!(social_settings::on_settings_column(a.world.as_ref().unwrap()));
    let s = draw(&mut a, 120, 40);
    let row = line_with(&s, "接受决斗");
    assert!(row.contains('关'), "默认关\n{s}");
    assert!(s.contains("打完一场或重新登录后自动关闭；开着时同图的人可以直接向你发起决斗"), "第四行的说明\n{s}");
}

fn unit(no: u8, kind: &str, name: &str, level: u32, pct: u32) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level, hp_pct: pct, ..BattleUnit::default() }
}

/// 决斗的开场：应战方（你是 10）对发起方「路人乙」，己方是勇者小明（血量 2%）和它的宠物。
fn duel_start() -> BattleStart {
    BattleStart {
        kind: "duel".into(),
        scene: "萨姆吉尔村".into(),
        you: 10,
        units: vec![
            unit(0, "player", "路人乙", 1, 100),
            unit(5, "pet", "乌力乌力", 1, 100),
            unit(10, "player", "勇者小明", 1, 2),
            unit(15, "pet", "乌力乌力", 1, 100),
        ],
        ..BattleStart::default()
    }
}

fn with_battle(a: &mut App, b: BattleState) {
    a.world.as_mut().unwrap().battle = Some(b);
}

#[test]
fn duel_battle_title_and_own_side_for_the_challenged_player() {
    let mut a = app(true);
    with_battle(&mut a, BattleState::from_start(duel_start()));
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("决斗 · 萨姆吉尔村"), "标题写「决斗 · 地图名」\n{s}");
    assert!(!s.contains("敌左我右"), "决斗的标题不写敌左我右\n{s}");
    // you 为 10 时，己方是 1 号阵营：顶栏写的自己血量（勇者小明 2%）、以及单位 10 归「我」、单位 0 归「敌」。
    assert!(line_with(&s, "HP 2%").contains("勇者小明"), "顶栏是己方（you 为 10）\n{s}");
    assert_eq!(slots::label(1, 10).chars().next(), Some('我'));
    assert_eq!(slots::label(1, 0).chars().next(), Some('敌'));
}

#[test]
fn duel_result_window_shows_points_and_no_rewards() {
    let mut a = app(true);
    let mut b = BattleState::from_start(duel_start());
    b.show_result(BattleEnd {
        result: "lose".into(),
        turn: 1,
        duel_point: Some(DuelPoint { change: -10, total: 90 }),
        ..BattleEnd::default()
    });
    with_battle(&mut a, b);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("决斗失败"), "结果窗口的标题\n{s}");
    assert!(s.contains("决斗点数 -10（现在 90）"), "决斗点数的增减和现值\n{s}");
    for banned in ["经验", "石币", "没有奖励", "掉落", "获得"] {
        assert!(!s.contains(banned), "决斗的结果不写 {banned}\n{s}");
    }
}

#[test]
fn duel_result_window_shows_gain_and_zero_change() {
    let mut a = app(true);
    let mut b = BattleState::from_start(duel_start());
    b.show_result(BattleEnd {
        result: "win".into(),
        turn: 1,
        duel_point: Some(DuelPoint { change: 10, total: 110 }),
        ..BattleEnd::default()
    });
    with_battle(&mut a, b);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("决斗胜利") && s.contains("决斗点数 +10（现在 110）"), "{s}");
}

#[test]
fn character_page_shows_duel_points() {
    let mut a = app(true);
    key(&mut a, Char('/'));
    for c in "page 2".chars() {
        key(&mut a, Char(c));
    }
    key(&mut a, Enter);
    let s = draw(&mut a, 120, 40);
    let row = line_with(&s, "决斗点数");
    assert!(row.contains("100"), "角色页基本信息里的决斗点数\n{s}");
}
