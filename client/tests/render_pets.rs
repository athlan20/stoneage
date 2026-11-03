//! 宠物页（STAGE-31）的界面测试：空栏位、详情字段、出战优先、`pet status` 切换、已倒下，`docs/ui.md`「宠物页 A」。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{Appearance, Character, ElementPoints, Location, Pet, PetSkill, Pets, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() })
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

/// 在底部输入行打一行命令并回车。
fn command(app: &mut App, line: &str) {
    if app.world.as_ref().is_some_and(|w|!w.nav.is(&stoneage_client::app::Layer::Command)){app.on_key(crossterm::event::KeyEvent::new(crossterm::event::KeyCode::Char('/'),crossterm::event::KeyModifiers::NONE));}
    for c in line.chars() {
        key(app, KeyCode::Char(c));
    }
    key(app, KeyCode::Enter);
}

/// 乌力：编号 1 的模板，Lv 5，休息（`docs/protocol.md` 5.19 的示例）。
fn wuli(slot: u8, state: &str) -> Pet {
    Pet {
        slot,
        species_id: 1,
        species: "乌力".into(),
        name: "乌力".into(),
        level: 5,
        transmigration: 0,
        exp: 0,
        next_exp: 67,
        hp: 35,
        max_hp: 35,
        attack: 5,
        defense: 6,
        agility: 7,
        loyalty: 8,
        elements: ElementPoints { earth: 80, water: 20, fire: 0, wind: 0 },
        state: state.into(),
        skills: vec![PetSkill { slot: 1, id: 1, name: "攻击".into(), ..Default::default() }],
        growth: None,
        quality: vec![],
    }
}

/// 进入世界并切到「3 宠物」页。
fn pet_page(pets: Pets, unicode: bool) -> App {
    let mut a = app(unicode);
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.pets = pets;
    a.world = Some(world);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('3'));
    a
}

fn with(pets: Vec<Pet>) -> Pets {
    Pets { max_slots: 5, pets }
}

#[test]
fn empty_pets_show_twenty_empty_slots_and_hint() {
    let mut a = pet_page(Pets::default(), true);
    let s = draw(&mut a, 120, 40);
    for want in ["宠物栏位", "编号", "名字", "状态", "宠物", "还没有宠物"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    // STAGE-346：20 个栏位各占一行，名字列写灰色的 —（验收 1）。
    assert_eq!(s.matches('—').count(), 20, "20 个空栏位都要有占位符\n{s}");
}

#[test]
fn pet_detail_shows_every_field() {
    let mut a = pet_page(with(vec![wuli(0, "rest")]), true);
    let s = draw(&mut a, 120, 40);
    for want in ["乌力 · 0 号 · 休息", "种族", "忠诚", "等级", "经验", "0/67", "HP", "35/35", "攻击", "防御", "敏捷", "地", "水", "火", "风", "技能", "1 攻击"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
    // 栏位表里这一行同时有名字、等级、HP 和状态（详情标题行只有名字和状态）。
    let row = s.lines().find(|l| l.contains("乌力") && l.contains("35/35")).expect("栏位表里有乌力");
    assert!(row.contains("5") && row.contains("休息"), "表里有等级和状态\n{row}");
}

/// 80×24 是最小窗口：详情的每一行都要放得下（10 行），技能栏和最后的地水火风不能被挤掉。
#[test]
fn detail_fits_the_smallest_window() {
    let mut a = pet_page(with(vec![wuli(0, "rest")]), true);
    let s = draw(&mut a, 80, 24);
    for want in ["乌力 · 0 号", "种族", "经验", "HP", "攻击", "技能", "1 攻击", "风"] {
        assert!(s.contains(want), "80×24 缺少 {want}\n{s}");
    }
}

/// 出战的宠物默认显示；没有出战的显示第一只。
#[test]
fn battle_pet_is_shown_by_default() {
    let mut a = pet_page(with(vec![wuli(0, "rest"), wuli(2, "battle")]), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("乌力 · 2 号 · 出战"), "默认显示出战的 2 号\n{s}");

    let mut a = pet_page(with(vec![wuli(1, "rest"), wuli(3, "standby")]), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("乌力 · 1 号 · 休息"), "没有出战的显示第一只\n{s}");
}

/// `pet status <编号>` 切换详情，结果保留到下次切换。
#[test]
fn pet_status_switches_the_detail() {
    let mut second = wuli(2, "battle");
    second.name = "小龙".into();
    second.species = "暴龙宝宝".into();
    let mut a = pet_page(with(vec![wuli(0, "rest"), second]), true);
    command(&mut a, "pet status 0");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("乌力 · 0 号 · 休息"), "切到 0 号\n{s}");
    assert!(s.contains("0 号：乌力"), "切换后消息栏写一行\n{s}");

    command(&mut a, "pet status 2");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("小龙 · 2 号 · 出战"), "切到 2 号\n{s}");
}

#[test]
fn pet_status_rejects_bad_or_empty_slots() {
    let mut a = pet_page(with(vec![wuli(0, "rest")]), true);
    command(&mut a, "pet status 3");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("3 号栏位没有宠物"), "空栏位要报错\n{s}");
    command(&mut a, "pet status 20");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("宠物栏位要在 0–19 之间"), "超出范围要报错\n{s}");
    command(&mut a, "pet");
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("用法：pet status"), "缺参数给用法\n{s}");
    assert!(s.contains("乌力 · 0 号 · 休息"), "选中的不变\n{s}");
}

/// HP 为 0 的宠物显示「已倒下」，不管服务端给的是什么状态。
#[test]
fn fainted_pet_is_marked() {
    let mut p = wuli(0, "battle");
    p.hp = 0;
    let mut a = pet_page(with(vec![p]), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("已倒下"), "HP 为 0 显示已倒下\n{s}");
}

#[test]
fn survives_every_size_and_ascii_mode() {
    let mut p = wuli(3, "standby");
    p.skills = vec![
        PetSkill { slot: 1, id: 1, name: "攻击".into(), ..Default::default() },
        PetSkill { slot: 4, id: 3, name: "破除防御".into(), ..Default::default() },
    ];
    for unicode in [true, false] {
        let mut a = pet_page(with(vec![wuli(0, "rest"), p.clone()]), unicode);
        for (w, h) in [(1, 1), (79, 23), (80, 24), (100, 30), (120, 40), (200, 60)] {
            draw(&mut a, w, h);
        }
    }
}

/// 骑着的宠物：状态列和详情标题都显示“骑乘”（STAGE-123，`docs/ui.md` 宠物页 A）。
#[test]
fn riding_pet_shows_ride_status() {
    let mut a = pet_page(with(vec![wuli(1, "ride")]), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("乌力 · 1 号 · 骑乘"), "详情标题显示骑乘\n{s}");
    let row = s.lines().find(|l| l.contains("乌力") && l.contains("35/35")).expect("栏位表里有乌力");
    assert!(row.contains("骑乘"), "栏位表的状态列显示骑乘\n{row}");
}

/// STAGE-196：详情的等级行带“转生”一项；旧服务端不带 transmigration 字段时按 0 显示。
#[test]
fn pet_detail_shows_transmigration_on_level_row() {
    let mut p = wuli(0, "rest");
    p.transmigration = 2;
    let mut a = pet_page(with(vec![p]), true);
    let s = draw(&mut a, 120, 40);
    let row = s.lines().find(|l| l.contains("转生")).unwrap_or("");
    assert!(row.contains("转生  2"), "转生次数写在等级行\n{s}");

    let old: Pet = serde_json::from_str(r#"{"slot":0,"name":"乌力","level":5}"#).unwrap();
    assert_eq!(old.transmigration, 0);
    let mut a = pet_page(with(vec![old]), true);
    let s = draw(&mut a, 120, 40);
    let row = s.lines().find(|l| l.contains("转生")).unwrap_or("");
    assert!(row.contains("转生  0"), "缺省按 0 显示\n{s}");
}

/// STAGE-243：菜单里骑乘项的标签随状态变：没骑着写「骑乘」，骑着这只写「下骑」。
#[test]
fn ride_action_label_follows_the_pet_state() {
    let mut a = pet_page(with(vec![wuli(0, "rest")]), true);
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("骑乘"), "休息的宠物菜单写骑乘\n{s}");
    assert!(!s.contains("下骑"), "没骑着就不写下骑\n{s}");

    let mut a = pet_page(with(vec![wuli(0, "ride")]), true);
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("下骑"), "骑着这只，菜单写下骑\n{s}");
}

/// STAGE-243：8 项动作在 80×24 下放不下，第 8 项「放生」靠滚动选中后能看到（120×24 也一样）。
#[test]
fn eighth_action_scrolls_into_view_at_80x24() {
    for (w, h) in [(80, 24), (120, 24)] {
        let mut a = pet_page(with(vec![wuli(0, "rest")]), true);
        key(&mut a, KeyCode::Enter);
        let s = draw(&mut a, w, h);
        if w == 80 {
            assert!(!s.contains("放生"), "80×24 第一屏放不下第 8 项\n{s}");
        }
        for _ in 0..7 {
            key(&mut a, KeyCode::Down);
        }
        let s = draw(&mut a, w, h);
        assert!(s.contains("放生"), "{w}×{h} 滚动后第 8 项可见\n{s}");
        assert!(s.contains("宠物栏位") && s.contains("乌力"), "宠物表还在\n{s}");
    }
}
