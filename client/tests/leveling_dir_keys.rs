//! STAGE-279 验收（界面与状态）：快捷传送根层的「练级传送」→ 摘要（打开时取一次）→ L2 取第 1 段（按段取）→
//! 第一个练级点只有一个落点，直接进 L4 → 选「只传送不遇敌」，成功回场景。
//! 模拟服务端按用例 `ui_leveling_dir` 回放（摘要与段内容写死在用例里）。
//! 另有状态机的纯测试：段下标、空段、多落点经过 L3、单落点跳过 L3、Esc 逐层退回。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Down, End, Enter, Esc, Tab};
use serde_json::json;
use stoneage_client::app::leveling_dir::{
    LevelingAction, LevelingDir, LevelingLevel, LevelingStatus, segment_index_of, segment_index_of_min,
};
use stoneage_client::app::{App, Layer, MAP_MENU, Screen};
use stoneage_client::proto::{LevelingCatalog, LevelingEntry};
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 场景 → 游戏菜单 → 快捷传送（菜单第 5 项）。
fn open_travel(h: &mut Harness) {
    h.key(Esc);
    h.repeat(Down, 4);
    h.key(Enter);
}

fn leveling(app: &App) -> &LevelingDir {
    &app.world.as_ref().unwrap().travel.leveling
}

fn on_travel_page(app: &App) -> bool {
    app.world.as_ref().is_some_and(|w| w.nav.is(&Layer::Page(MAP_MENU)))
}

fn has_log(app: &App, needle: &str) -> bool {
    app.log.all().iter().any(|m| m.text.contains(needle))
}

#[test]
fn leveling_travels_to_the_single_spot_entry() {
    let mut h = Harness::new("ui_leveling_dir", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.wait("拿到快捷传送目录", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.travel.ready())
    });

    open_travel(&mut h);
    h.wait("打开快捷传送页", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.menu == MAP_MENU && w.nav.is(&Layer::Page(MAP_MENU)))
    });
    // 根层末尾是「练级传送」（紧跟「1级宠物」之后）。
    assert_eq!(
        h.app.world.as_ref().unwrap().travel.view().last().map(|r| r.leveling),
        Some(true),
        "根层末尾是「练级传送」"
    );

    // 光标到末尾 Enter：打开练级传送，摘要这时才去取。
    h.key(End);
    h.key(Enter);
    h.wait("摘要取到", 5, |a| leveling(a).status == LevelingStatus::Ready);
    assert!(leveling(&h.app).is_open());
    assert_eq!(leveling(&h.app).cursor(), 0, "1 级角色光标在 1-10 段");
    assert_eq!(leveling(&h.app).row_count(), 13, "L1 是 13 段");

    // L1 → L2：按段取练级点。
    h.key(Enter);
    h.wait("第 1 段取到", 5, |a| leveling(a).is_segment_loaded(0));
    assert_eq!(leveling(&h.app).row_count(), 2, "1-10 段两个练级点");

    // 第一个练级点只有一个落点：直接进 L4。
    h.key(Enter);
    assert_eq!(
        leveling(&h.app).level(),
        &LevelingLevel::Actions {
            seg: 0,
            entry: 0,
            spot: 0
        }
    );

    // L4 选「只传送不遇敌」。
    h.repeat(Down, 2);
    h.key(Enter);
    h.wait("已传送", 5, |a| has_log(a, "已传送到 萨伊那斯"));
    assert!(!leveling(&h.app).is_open(), "成功后退出练级传送，回场景");
    assert!(!on_travel_page(&h.app), "成功回场景");
    h.finish();
}

#[test]
fn segment_index_follows_level() {
    assert_eq!(segment_index_of(1), 0);
    assert_eq!(segment_index_of(10), 0);
    assert_eq!(segment_index_of(11), 1);
    assert_eq!(segment_index_of(25), 2);
    assert_eq!(segment_index_of(999), 12, "超过 130 级也在最后一段");
    assert_eq!(segment_index_of(0), 0);
    assert_eq!(segment_index_of_min(1), Some(0));
    assert_eq!(segment_index_of_min(121), Some(12));
    assert_eq!(segment_index_of_min(5), None);
    assert_eq!(segment_index_of_min(131), None);
}

/// 13 段摘要：111-130 为空段。
fn catalog() -> LevelingCatalog {
    let counts = [
        (47, 18),
        (63, 39),
        (34, 70),
        (24, 71),
        (30, 73),
        (52, 99),
        (27, 54),
        (8, 11),
        (13, 26),
        (16, 24),
        (9, 10),
        (0, 0),
        (0, 0),
    ];
    let segments: Vec<_> = counts
        .iter()
        .enumerate()
        .map(|(i, (e, m))| json!({"min": i * 10 + 1, "max": i * 10 + 10, "entry_count": e, "map_count": m}))
        .collect();
    serde_json::from_value(json!({"catalog_version": "t", "segments": segments})).unwrap()
}

fn entry(monsters: &[(&str, i64, i64)], spots: &[&str]) -> LevelingEntry {
    let monsters: Vec<_> = monsters
        .iter()
        .map(|(n, lo, hi)| json!({"name": n, "level_min": lo, "level_max": hi}))
        .collect();
    let spots: Vec<_> = spots
        .iter()
        .map(|id| {
            json!({"area_id": id, "number": 1, "floor": 100, "map_name": "萨伊那斯",
                   "anchor": {"x": 10, "y": 10}, "rect": {"x1": 0, "y1": 0, "x2": 20, "y2": 20},
                   "max_enemies": 3, "steps": 26})
        })
        .collect();
    serde_json::from_value(json!({"monsters": monsters, "spots": spots})).unwrap()
}

fn open_dir(level: u32) -> LevelingDir {
    let mut d = LevelingDir::default();
    d.open(level);
    d.install(catalog());
    d
}

#[test]
fn open_lands_on_player_segment_and_install_marks_ready() {
    let d = open_dir(25);
    assert!(d.is_open());
    assert_eq!(d.status, LevelingStatus::Ready);
    assert_eq!(d.cursor(), 2, "25 级在 21-30 段");
    assert_eq!(d.segment_summary(1).map(|s| s.entry_count), Some(63));
}

#[test]
fn empty_segment_has_no_entries_and_enter_does_not_descend() {
    let mut d = open_dir(1);
    for _ in 0..11 {
        d.navigate(stoneage_client::app::TravelNav::Down);
    }
    assert_eq!(d.cursor(), 11, "111-120 段");
    assert_eq!(d.enter(), LevelingAction::Descend);
    assert_eq!(d.level(), &LevelingLevel::Entries { seg: 11 });
    assert_eq!(d.pending_segment(), Some(11), "进段才去取");
    d.install_segment(11, vec![]);
    assert!(d.is_segment_loaded(11));
    assert_eq!(d.row_count(), 0, "空段没有行，画面写“本段没有野外遇敌点”");
    assert_eq!(d.enter(), LevelingAction::None);
    assert_eq!(d.back(), LevelingAction::Descend);
    assert_eq!(d.level(), &LevelingLevel::Segments);
}

#[test]
fn single_spot_entry_skips_spot_layer_and_multi_spot_goes_through_it() {
    let mut d = open_dir(1);
    d.install_segment(
        0,
        vec![
            entry(&[("布比", 2, 3)], &["A"]),
            entry(&[("奇宝", 1, 4)], &["B1", "B2"]),
        ],
    );
    assert_eq!(d.enter(), LevelingAction::Descend);
    // 单落点：Enter 直接进 L4。
    assert_eq!(d.enter(), LevelingAction::Descend);
    assert_eq!(
        d.level(),
        &LevelingLevel::Actions {
            seg: 0,
            entry: 0,
            spot: 0
        }
    );
    d.back();
    // 多落点：先进 L3，选第二个落点再进 L4。
    d.navigate(stoneage_client::app::TravelNav::Down);
    assert_eq!(d.enter(), LevelingAction::Descend);
    assert_eq!(d.level(), &LevelingLevel::Spots { seg: 0, entry: 1 });
    assert_eq!(d.row_count(), 2);
    d.navigate(stoneage_client::app::TravelNav::Down);
    assert_eq!(d.enter(), LevelingAction::Descend);
    assert_eq!(
        d.level(),
        &LevelingLevel::Actions {
            seg: 0,
            entry: 1,
            spot: 1
        }
    );
    // L4 第一项：直接开战，带的是第二个落点。
    match d.enter() {
        LevelingAction::Go { spot, mode } => {
            assert_eq!(mode, "once");
            assert_eq!(spot.area_id, "B2");
        }
        other => panic!("应为开战，实际 {other:?}"),
    }
}

#[test]
fn esc_walks_back_one_level_then_leaves() {
    let mut d = open_dir(1);
    d.install_segment(0, vec![entry(&[("布比", 2, 3)], &["A"])]);
    d.enter();
    d.enter();
    assert_eq!(d.depth(), 3);
    assert_eq!(d.back(), LevelingAction::Descend);
    assert_eq!(d.level(), &LevelingLevel::Entries { seg: 0 });
    assert_eq!(d.back(), LevelingAction::Descend);
    assert_eq!(d.back(), LevelingAction::Leave);
    assert!(!d.is_open());
}
