use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{Terminal, backend::TestBackend};
use stoneage_client::{
    app::{App, Layer, LoadState, Options, SceneColumn, Screen, WorldState},
    proto::{MapExit, Npc, NpcWindow, TriggerCell, WorldEntry},
};
use unicode_width::UnicodeWidthStr;
fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = 2000;
    w.entry.position.map_name = "玛丽娜丝渔村".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    w.scene.exits = (1..=31)
        .map(|i| MapExit {
            id: format!("door{i}"),
            to_floor: 100,
            to_name: format!("出口{i:02}"),
            trigger_cells: vec![TriggerCell { x: i, y: 33 }],
            time_gate: "all".into(),
            available: true,
            ..Default::default()
        })
        .collect();
    w.npcs = (1..=33)
        .map(|i| Npc {
            id: i,
            name: format!("村民{i:02}"),
            type_: "对话".into(),
            x: 10,
            y: 20,
        })
        .collect();
    a.world = Some(w);
    a
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
#[test]
fn independent_scroll_selection_fixed_entries_and_modal_priority() {
    for unicode in [true, false] {
        for (width, height) in [(80, 24), (120, 40)] {
            let mut a = app();
            a.unicode = unicode;
            let first = draw(&mut a, width, height);
            assert!(first.contains("出口01") && first.contains("村民01"));
            assert!(!first.contains("回城（存档点）"));
            key(&mut a, KeyCode::End);
            let s = draw(&mut a, width, height);
            assert!(s.contains("回城（存档点）"), "{s}");
            let left_top = a.world.as_ref().unwrap().scene_scroll;
            key(&mut a, KeyCode::Right);
            key(&mut a, KeyCode::End);
            let s = draw(&mut a, width, height);
            assert!(s.contains("村民33") && s.contains("回城（存档点）"), "{s}");
            key(&mut a, KeyCode::Left);
            assert_eq!(a.world.as_ref().unwrap().scene_index, 33);
            assert_eq!(a.world.as_ref().unwrap().scene_scroll, left_top);
            key(&mut a, KeyCode::Home);
            draw(&mut a, width, height);
            key(&mut a, KeyCode::PageDown);
            assert_eq!(
                a.world.as_ref().unwrap().scene_index,
                a.world.as_ref().unwrap().scene.rows
            );
            let s = draw(&mut a, width, height);
            assert!(s.contains(&format!(
                "出口{:02}",
                a.world.as_ref().unwrap().scene_index + 1
            )));
            let old = a.world.as_ref().unwrap().scene_index;
            key(&mut a, KeyCode::Esc);
            key(&mut a, KeyCode::Esc);
            assert_eq!(a.world.as_ref().unwrap().scene_index, old);
            key(&mut a, KeyCode::Char('/'));
            key(&mut a, KeyCode::Right);
            assert_eq!(a.world.as_ref().unwrap().scene.column, SceneColumn::Exit);
            key(&mut a, KeyCode::Esc);
            a.world.as_mut().unwrap().npc_window = Some(NpcWindow {
                window_id: "w".into(),
                ..Default::default()
            });
            key(&mut a, KeyCode::Right);
            assert_eq!(a.world.as_ref().unwrap().scene.column, SceneColumn::Exit);
        }
    }
}
#[test]
fn long_names_all_cells_and_closed_reason_readable() {
    for unicode in [true, false] {
        for (width, height) in [(80, 24), (120, 40)] {
            let mut a = app();
            a.unicode = unicode;
            let w = a.world.as_mut().unwrap();
            let e = &mut w.scene.exits[0];
            e.to_name = "非常长的中文目标地图名字用于验证完整信息页".into();
            e.available = false;
            e.time_gate = "morning".into();
            e.unavailable_reason = Some("出口数据冲突，暂不可用".into());
            e.one_way = true;
            e.trigger_cells = (1..=40).map(|x| TriggerCell { x, y: 99 }).collect();
            let s = draw(&mut a, width, height);
            assert!(s.contains("[关]"));
            key(&mut a, KeyCode::Tab);
            let s = draw(&mut a, width, height);
            let compact = s.replace(' ', "").replace('\n', "");
            assert!(
                compact.contains("非常长的中文目标地图名字用于验证完整信息页"),
                "{s}"
            );
            for _ in 0..50 {
                key(&mut a, KeyCode::PageDown);
            }
            let s = draw(&mut a, width, height);
            assert!(s.contains("(40, 99)"), "{s}");
            assert!(s.contains("普通出口表判断"));
            key(&mut a, KeyCode::Esc);
            assert!(!a.world.as_ref().unwrap().scene.details);
            assert!(a.world.as_ref().unwrap().nav.is(&Layer::Scene));
        }
    }
}
#[test]
fn loading_failure_empty_and_offline_keep_navigation_and_fixed_entries() {
    for status in [
        LoadState::Loading,
        LoadState::Failed("断网".into()),
        LoadState::Ready,
    ] {
        let mut a = app();
        let w = a.world.as_mut().unwrap();
        w.scene.exits.clear();
        w.npcs.clear();
        w.scene.exit_load = status.clone();
        w.scene.npc_load = status;
        w.online = false;
        let s = draw(&mut a, 80, 24);
        assert!(s.contains("回城（存档点）") && s.contains("快捷传送"));
        key(&mut a, KeyCode::Right);
        key(&mut a, KeyCode::Enter);
        assert!(!a.session.busy());
        key(&mut a, KeyCode::Left);
        key(&mut a, KeyCode::End);
        assert_eq!(a.world.as_ref().unwrap().scene_index, 2);
        key(&mut a, KeyCode::Home);
        key(&mut a, KeyCode::Enter);
        // STAGE-161：第一个固定入口是「快捷传送」，打开的是菜单第 1 页（不再是场景里的地点列表）。
        assert!(a.world.as_ref().unwrap().nav.is(&Layer::Page(0)));
    }
}
