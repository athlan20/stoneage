//! STAGE-273 附近玩家列：四列切换与各列光标、整表替换与增量推送、加载中/失败/空三种状态、80×24 不越界。
//! 直接构造 App 状态（同 scene_encounter），不走模拟服务端；请求与推送的线上流程由用例回放覆盖。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{Terminal, backend::TestBackend};
use stoneage_client::{
    app::{App, LoadState, Options, SceneColumn, Screen, WorldState},
    proto::{NearbyPlayer, PlayerChange, WorldEntry},
};
use unicode_width::UnicodeWidthStr;

fn player(name: &str, level: u32, in_battle: bool) -> NearbyPlayer {
    NearbyPlayer {
        name: name.into(),
        level,
        in_battle,
        ..Default::default()
    }
}

/// 进世界、场景页、出口/NPC/遇敌都已加载，只有附近玩家列由各测试自己决定状态。
fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    w.scene.area_load = LoadState::Ready;
    w.scene.area_query = 1;
    w.scene.players.load = LoadState::Ready;
    a.world = Some(w);
    a
}

fn world(a: &mut App) -> &mut WorldState {
    a.world.as_mut().unwrap()
}

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

fn draw(a: &mut App, width: u16, height: u16) -> String {
    let mut t = Terminal::new(TestBackend::new(width, height)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, a)).unwrap();
    let b = t.backend().buffer();
    (0..height)
        .map(|y| {
            let mut s = String::new();
            let mut x = 0;
            while x < width {
                let v = b[(x, y)].symbol();
                s.push_str(v);
                x += UnicodeWidthStr::width(v).max(1) as u16;
            }
            s
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn column(a: &App) -> SceneColumn {
    a.world.as_ref().unwrap().scene.column
}

/// 出口 → NPC → 附近玩家：按两次 →。
fn go_to_players(a: &mut App) {
    key(a, KeyCode::Right);
    key(a, KeyCode::Right);
    assert_eq!(column(a), SceneColumn::Players);
}

/// 四列之间切换时，附近玩家列的光标留在原处；回到该列时仍是切走前的那一行。
#[test]
fn players_cursor_is_kept_when_leaving_and_returning() {
    let mut a = app();
    world(&mut a).scene.players.replace(
        vec![
            player("甲", 30, false),
            player("乙", 20, false),
            player("丙", 10, false),
        ],
        false,
    );
    go_to_players(&mut a);
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&mut a).scene.players.index, 1);
    key(&mut a, KeyCode::Right);
    assert_eq!(column(&a), SceneColumn::Encounter);
    key(&mut a, KeyCode::Left);
    assert_eq!(column(&a), SceneColumn::Players);
    assert_eq!(world(&mut a).scene.players.index, 1, "切走再切回，光标不应重置");
    assert_eq!(
        world(&mut a).scene.players.selected().map(|p| p.name.as_str()),
        Some("乙")
    );
}

/// 整表替换按等级从高到低排；增量 enter/update 整条替换，leave 删除，选中行按名字保住。
#[test]
fn incremental_changes_apply_and_keep_selection_by_name() {
    let mut a = app();
    let s = &mut world(&mut a).scene.players;
    s.replace(
        vec![player("甲", 10, false), player("乙", 20, false), player("丙", 5, false)],
        false,
    );
    let names: Vec<_> = s.list.iter().map(|p| p.name.clone()).collect();
    assert_eq!(names, ["乙", "甲", "丙"], "等级高的在前");
    s.index = 1; // 选中「甲」
    s.apply(vec![
        PlayerChange {
            op: "enter".into(),
            player: Some(player("丁", 40, false)),
            name: "丁".into(),
        },
        PlayerChange {
            op: "update".into(),
            player: Some(player("甲", 50, true)),
            name: "甲".into(),
        },
        PlayerChange {
            op: "leave".into(),
            player: None,
            name: "丙".into(),
        },
        PlayerChange {
            op: "unknown".into(),
            player: None,
            name: "乙".into(),
        },
    ]);
    let names: Vec<_> = s.list.iter().map(|p| p.name.clone()).collect();
    assert_eq!(
        names,
        ["甲", "丁", "乙"],
        "update 后等级变了要重新排序，丙离开，未知 op 忽略"
    );
    assert_eq!(
        s.selected().map(|p| p.name.as_str()),
        Some("甲"),
        "选中行按名字保住，不跟着下标跑"
    );
    assert!(s.list[0].in_battle, "update 整条替换，战斗标记跟着变");
}

/// 加载中、失败（带 F5 重试）、本图没有其他玩家，三种状态各自的文字；有玩家时不显示空状态。
#[test]
fn loading_failure_and_empty_each_show_their_own_text() {
    let mut a = app();
    go_to_players(&mut a);

    world(&mut a).scene.players.load = LoadState::Loading;
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("加载中"), "{text}");

    world(&mut a).scene.players.load = LoadState::Failed("超时".into());
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("加载失败"), "{text}");
    assert!(text.contains("F5 重试"), "{text}");

    world(&mut a).scene.players.load = LoadState::Ready;
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("本图没有其他玩家"), "{text}");

    world(&mut a)
        .scene
        .players
        .replace(vec![player("甲乙", 12, false)], false);
    let text = draw(&mut a, 80, 24);
    assert!(!text.contains("本图没有其他玩家"), "{text}");
    assert!(text.contains("甲乙"), "{text}");
    assert!(text.contains("Lv12"), "{text}");
}

/// 80×24 下四列都在，名字很长时截断名字但等级和战斗标记保留，每行显示宽度不超过 80。
#[test]
fn long_names_fit_in_compact_terminal() {
    let mut a = app();
    world(&mut a).scene.players.replace(
        vec![
            player("这是一个非常非常长的玩家名字用来测试截断", 99, true),
            player("乙", 1, false),
        ],
        true,
    );
    go_to_players(&mut a);
    let text = draw(&mut a, 80, 24);
    for name in ["出口", "NPC", "附近玩家", "遇敌"] {
        assert!(text.contains(name), "{text}");
    }
    assert!(text.contains("Lv99"), "{text}");
    // Options::default() 是 ASCII 降级，战斗标记为 `!`（Unicode 下是 ⚔）。
    assert!(text.contains("Lv99 !"), "{text}");
    assert!(text.contains("这是…"), "名字被截断：{text}");
    assert!(text.contains("附近玩家 2+"), "truncated 时标题带 +：{text}");
    for line in text.lines() {
        assert!(UnicodeWidthStr::width(line) <= 80, "行宽超过 80：{line:?}");
    }
}
