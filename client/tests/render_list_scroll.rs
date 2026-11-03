//! 列表光标走出可见范围时，列表要跟着滚（STAGE-147）：
//! 场景页的本地图 NPC 列表、地图页右侧的 NPC 表、任务页的任务列表，光标到哪一行都要在框里看得见。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Layer, MAP_MENU, Options, QUEST_MENU, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, Location, Npc, Quest, Quests, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
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

fn key(app: &mut App, code: KeyCode) {
    app.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn entry(x: i32, y: i32) -> WorldEntry {
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
    };
    entry.position.floor = 1000;
    entry.position.map_name = "萨姆吉尔村".into();
    entry.position.x = x;
    entry.position.y = y;
    entry
}

fn npcs(n: usize) -> Vec<Npc> {
    (1..=n)
        .map(|i| Npc { id: i as u32, name: format!("村民{i:02}"), type_: "对话".into(), x: 50, y: 116 })
        .collect()
}

/// 右列仍保留 STAGE-147 的 NPC 光标跟随，固定入口独立在左列。
#[test]
fn scene_list_keeps_the_cursor_row_in_view() {
    let mut a=app();let mut world=WorldState::new(entry(50,116));world.npcs=npcs(30);
    world.scene.npc_load=stoneage_client::app::LoadState::Ready;
    a.world=Some(world);a.screen=Screen::World;
    assert!(draw(&mut a,100,24).contains("村民01"));
    key(&mut a,KeyCode::Right);key(&mut a,KeyCode::End);
    let s=draw(&mut a,100,24);assert!(s.contains("村民30")&&s.contains("回城（存档点）"),"{s}");
    let top=a.world.as_ref().unwrap().scene.npc_top;
    key(&mut a,KeyCode::Up);draw(&mut a,100,24);
    assert_eq!(a.world.as_ref().unwrap().scene.npc_top,top);
}


/// 任务页：30 个任务，光标一路往下到最后一个时要看得见。
#[test]
fn quest_list_keeps_the_cursor_row_in_view() {
    let mut a = app();
    let mut world = WorldState::new(entry(50, 116));
    world.nav.push(Layer::Page(QUEST_MENU));
    world.menu = QUEST_MENU;
    world.quests = Quests {
        quests: (1..=30).map(|n| Quest { no: n, name: format!("任务{n:02}"), state: "done".into() }).collect(),
        changed: vec![],
    };
    a.world = Some(world);
    a.screen = Screen::World;

    let s = draw(&mut a, 100, 24);
    assert!(s.contains("任务01"), "第一页从第一个任务开始\n{s}");
    assert!(!s.contains("任务30"), "最后一个第一页看不到\n{s}");

    for _ in 0..40 {
        key(&mut a, KeyCode::Down);
    }
    let s = draw(&mut a, 100, 24);
    assert!(s.contains("任务30"), "光标到最后一个任务了，列表要滚下来\n{s}");
}
