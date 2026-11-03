//! STAGE-152 场景页与遇敌列：切列边界、空图提示、禁用原因、动作面板默认项。
//! STAGE-273：场景页改为四列（出口/NPC/附近玩家/遇敌），遇敌列现在要按三次 → 才到达。
//! STAGE-286：能抓 1 级宠物的区域行首加标记，详情列出宠物名（道具挡住时补“持有指定道具后可抓”）。

#[allow(dead_code)]
mod support;
use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{Terminal, backend::TestBackend};
use stoneage_client::{
    app::{App, LoadState, Options, SceneColumn, Screen, WorldState},
    proto::{AreaEnemy, AreaPoint, AreaRect, EncounterArea, WorldEntry},
};
use support::{Harness, opts};
use unicode_width::UnicodeWidthStr;

fn area(number: u32, available: bool) -> EncounterArea {
    EncounterArea {
        id: format!("100-{number}-1-1-9-9"),
        number,
        rect: AreaRect { x1: 1, y1: 1, x2: 9, y2: 9 },
        anchor: AreaPoint { x: 1, y: 1 },
        available,
        unavailable_reason: (!available).then(|| "需要持有指定道具才能遇到这里的敌人".to_string()),
        max_enemies: 2,
        level_min: 1,
        level_max: 3,
        enemies: vec![AreaEnemy {
            id: 119,
            name: "昆伊".into(),
            level_min: 1,
            level_max: 1,
            create_max: 2,
            weight: 50,
            group_id: 88,
        }],
        required_items: if available { vec![] } else { vec![1234] },
        forbidden_items: vec![],
        level1_pets: vec![],
    }
}

fn app(areas: Vec<EncounterArea>) -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    w.scene.area_load = LoadState::Ready;
    // 已经请求过一次：切列不会再触发按需加载（离线下会一直停在“加载中”）。
    w.scene.area_query = 1;
    w.scene.areas = areas;
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

fn column(a: &App) -> SceneColumn {
    a.world.as_ref().unwrap().scene.column
}

/// ←→ 在四列之间移动，到两端停住（STAGE-273：中间多了附近玩家列）。
#[test]
fn four_columns_move_and_stop_at_edges() {
    let mut a = app(vec![area(21, true)]);
    assert_eq!(column(&a), SceneColumn::Exit);
    key(&mut a, KeyCode::Left);
    assert_eq!(column(&a), SceneColumn::Exit, "最左列再往左应停住");
    key(&mut a, KeyCode::Right);
    assert_eq!(column(&a), SceneColumn::Npc);
    key(&mut a, KeyCode::Right);
    assert_eq!(column(&a), SceneColumn::Players);
    key(&mut a, KeyCode::Right);
    assert_eq!(column(&a), SceneColumn::Encounter);
    key(&mut a, KeyCode::Right);
    assert_eq!(column(&a), SceneColumn::Encounter, "最右列再往右应停住");
    key(&mut a, KeyCode::Left);
    assert_eq!(column(&a), SceneColumn::Players);
}

/// 空地图给出“没有可遇敌区域”，而不是加载失败。
#[test]
fn empty_map_says_there_is_no_area() {
    let mut a = app(vec![]);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    assert_eq!(a.world.as_ref().unwrap().scene.areas.len(), 0);
    assert_eq!(column(&a), SceneColumn::Encounter);
    let text = draw(&mut a, 80, 24);
    // STAGE-273：四列后遇敌列在 80 宽下只剩约 18 列，句末“域”会被截掉，这里只断言看得见的前半句。
    assert!(text.contains("本地图没有可遇敌"), "{text}");
    assert!(!text.contains("加载失败"), "{text}");
    // 空列表时 Enter 什么都不发，也不弹面板。
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().scene.prompt.is_none());
}

/// 禁用区域仍然列出，Enter 只说明原因，不打开动作面板。
#[test]
fn unavailable_area_explains_instead_of_starting() {
    let mut a = app(vec![area(21, false)]);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("[禁]"), "{text}");
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().scene.prompt.is_none());
}

/// 可用区域 Enter 打开面板，默认「遇敌 1 次」，三个选项都能看到；Esc 关掉。
#[test]
fn prompt_defaults_to_once_and_esc_cancels() {
    let mut a = app(vec![area(21, true)]);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Enter);
    let prompt = a.world.as_ref().unwrap().scene.prompt.expect("应当打开动作面板");
    assert_eq!(prompt.choice, stoneage_client::app::EncounterChoice::Once);
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("遇敌 1 次"), "{text}");
    assert!(text.contains("无限遇敌"), "{text}");
    assert!(text.contains("取消"), "{text}");
    assert!(text.contains("到此区域后立即遇敌"), "{text}");
    // ↑↓ 在三个选项之间移动，光标不会越界。
    key(&mut a, KeyCode::Down);
    assert_eq!(
        a.world.as_ref().unwrap().scene.prompt.unwrap().choice,
        stoneage_client::app::EncounterChoice::Infinite
    );
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    assert_eq!(
        a.world.as_ref().unwrap().scene.prompt.unwrap().choice,
        stoneage_client::app::EncounterChoice::Cancel
    );
    key(&mut a, KeyCode::Esc);
    assert!(a.world.as_ref().unwrap().scene.prompt.is_none());
    // 面板关掉之后列还在遇敌列，没有被 Esc 带回菜单。
    assert_eq!(column(&a), SceneColumn::Encounter);
}

/// 80×24 下四列都在，且遇敌列能看到区域号与敌人名。
#[test]
fn compact_terminal_still_shows_four_columns() {
    let mut a = app(vec![area(21, true)]);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("出口"), "{text}");
    assert!(text.contains("NPC"), "{text}");
    assert!(text.contains("附近玩家"), "{text}");
    assert!(text.contains("遇敌"), "{text}");
    assert!(text.contains("#21"), "{text}");
    assert!(text.contains("昆伊"), "{text}");
}

/// STAGE-160：进世界后一个键都不按，遇敌列也要自己加载完，不能一直停在“加载中…”上。
#[test]
fn encounter_column_loads_before_the_cursor_reaches_it() {
    let mut h = Harness::new("ui_exits", opts);
    h.text("alice");
    h.key(KeyCode::Tab);
    h.text("secret1");
    h.key(KeyCode::Enter);
    h.wait_screen(Screen::Characters);
    h.key(KeyCode::Enter);
    h.key(KeyCode::Enter);
    h.wait_screen(Screen::World);
    h.wait("遇敌列自动加载完成", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.scene.area_load == LoadState::Ready)
    });
    assert_eq!(column(&h.app), SceneColumn::Exit, "光标没挪过，还停在出口列");
    h.abort();
}

/// STAGE-286：带 1 级宠物名的区域。
fn pet_area(number: u32, available: bool, pets: &[&str]) -> EncounterArea {
    let mut a = area(number, available);
    a.level1_pets = pets.iter().map(|p| p.to_string()).collect();
    a
}

/// 与 `app()` 相同，但可指定 unicode 开关（降级时标记是 `[宠]`）。
fn app_unicode(areas: Vec<EncounterArea>, unicode: bool) -> App {
    let mut a = App::new(Options { unicode, ..Options::default() });
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    w.scene.area_load = LoadState::Ready;
    w.scene.area_query = 1;
    w.scene.areas = areas;
    a.world = Some(w);
    a
}

fn encounter_line<'a>(text: &'a str, number: u32) -> &'a str {
    let tag = format!("#{number} ");
    text.lines().find(|l| l.contains(&tag)).unwrap_or_else(|| panic!("没有 #{number} 这一行：\n{text}"))
}

/// 能抓 1 级宠物的区域行首有标记，没有宠物的行不加；80x24 下敌人名仍然可见。
#[test]
fn level1_pet_area_gets_a_marker_in_row() {
    let mut a = app_unicode(vec![pet_area(21, true, &["凯比特", "昆伊"]), area(22, true)], true);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let text = draw(&mut a, 80, 24);
    assert!(encounter_line(&text, 21).contains("◆"), "{text}");
    assert!(encounter_line(&text, 21).contains("昆伊"), "{text}");
    assert!(!encounter_line(&text, 22).contains("◆"), "{text}");
}

/// --unicode 关闭时标记退回 `[宠]`，不出现 ◆。
#[test]
fn level1_pet_marker_falls_back_to_ascii() {
    let mut a = app_unicode(vec![pet_area(21, true, &["凯比特"])], false);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let text = draw(&mut a, 80, 24);
    assert!(encounter_line(&text, 21).contains("[宠]"), "{text}");
    assert!(!text.contains("◆"), "{text}");
}

/// Tab 详情列出 1 级宠物；道具挡住的区域补“持有指定道具后可抓”，标记照常显示。
#[test]
fn pet_details_list_pets_and_gate_hint() {
    let mut a = app_unicode(vec![pet_area(21, false, &["凯比特", "昆伊"])], true);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    assert!(encounter_line(&draw(&mut a, 80, 24), 21).contains("[禁]"));
    key(&mut a, KeyCode::Tab);
    let text = draw(&mut a, 80, 24);
    assert!(text.contains("1级宠物：凯比特、昆伊（持有指定道具后可抓）"), "{text}");
}

/// 没有 1 级宠物的区域：行上没有标记，详情里也没有宠物一行。
#[test]
fn area_without_pets_has_no_marker_and_no_pet_line() {
    let mut a = app_unicode(vec![area(21, true)], true);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let text = draw(&mut a, 80, 24);
    assert!(!encounter_line(&text, 21).contains("◆"), "{text}");
    key(&mut a, KeyCode::Tab);
    assert!(!draw(&mut a, 80, 24).contains("1级宠物"));
}
