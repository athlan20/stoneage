//! STAGE-250：战斗顶栏第二行显示骑着的宠物（`docs/battle-ui.md`「总体布局」）。
//! 内容区高度 ≥ 24 行、骑乘时顶栏 2 行（边框加 2 行内容）；否则 1 行，和原来逐字节一致。

use std::time::Instant;

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleStart, BattleTurn, BattleUnit, Character, ElementPoints, Location, Mount, Progress,
    WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn draw(app: &mut App, w: u16, h: u16) -> Vec<String> {
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

/// 骑着贝鲁卡（30/40）的勇者，和一只出战的乌力；`down` 为真时骑宠已经倒下。
fn battle(rider_name: &str, down: bool) -> BattleState {
    let mut hero = unit(0, "player", "勇者小明", 12, 75);
    hero.hp = Some(45);
    hero.max_hp = Some(60);
    hero.mp = Some(100);
    hero.max_mp = Some(100);
    hero.elements = ElementPoints {
        earth: 60,
        water: 40,
        fire: 0,
        wind: 0,
    };
    hero.ride = true;
    hero.mount = Some(Mount {
        name: rider_name.into(),
        hp: Some(if down { 0 } else { 30 }),
        max_hp: Some(40),
        hp_pct: if down { 0 } else { 75 },
    });
    let mut pet = unit(5, "pet", "乌力", 5, 100);
    pet.hp = Some(35);
    pet.max_hp = Some(35);
    let mut boar = unit(15, "monster", "野猪", 3, 100);
    boar.elements = ElementPoints {
        earth: 100,
        water: 0,
        fire: 0,
        wind: 0,
    };
    let start = BattleStart { party: None,
        battle_id: "b_1".into(),
        kind: "wild".into(),
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow {
            escape: true,
            capture: true,
        },
        units: vec![hero, pet, boar],
        npc: None,
        field: None,
    };
    let mut b = BattleState::from_start(start);
    b.on_turn(
        BattleTurn {
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: vec!["player".into()],
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
            field: None,
        },
        Instant::now(),
    );
    b
}

/// 进入世界并挂上战斗（`observer` 为真表示 agent 在操控）。
fn battle_app(battle: BattleState, observer: bool) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    let mut entry = WorldEntry::default();
    entry.character = Character {
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
    };
    if observer {
        entry.control.you = "observer".into();
    }
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

#[test]
fn rider_shows_a_second_top_line_at_tall_sizes() {
    for (w, h) in [(200, 60), (120, 40), (100, 30)] {
        let mut a = battle_app(battle("贝鲁卡", false), false);
        let s = draw(&mut a, w, h);
        assert!(s[1].contains("第 1 回合 · 人物指令"), "{w}×{h} 第一行：{}", s[1]);
        assert!(s[2].contains("骑 贝鲁卡 HP 30/40"), "{w}×{h} 第二行应是骑宠：{}", s[2]);
        assert!(!s[3].contains("骑 "), "{w}×{h} 只有两行顶栏");
        assert!(
            s[3].contains('└') || s[3].contains('├') || s[3].contains('─'),
            "{w}×{h} 顶栏下边框在第 4 行：{}",
            s[3]
        );
    }
}

#[test]
fn compact_80x24_keeps_the_single_line_bar() {
    let mut with = battle_app(battle("贝鲁卡", false), false);
    let s = draw(&mut with, 80, 24);
    assert!(
        !s.iter().any(|l| l.contains("骑 贝鲁卡")),
        "80×24 不显示骑宠行\n{}",
        s.join("\n")
    );
    assert!(s[1].contains("第 1 回合 · 人物指令"), "80×24 第一行是原来的信息条");
    assert!(
        s[2].contains('└') || s[2].contains('─'),
        "80×24 顶栏下边框在第 3 行：{}",
        s[2]
    );
    // 同一场战斗只是没有 mount（骑着但老服务端没给骑宠血量）：整屏一样，骑宠不占行。
    let mut plain = battle_app(battle("贝鲁卡", false), false);
    let p = plain.world.as_mut().unwrap();
    p.battle.as_mut().unwrap().units.get_mut(&0).unwrap().mount = None;
    assert_eq!(s, draw(&mut plain, 80, 24));
}

#[test]
fn fainted_rider_is_red_and_says_damage_falls_on_the_player() {
    let mut a = battle_app(battle("贝鲁卡", true), false);
    let s = draw(&mut a, 120, 40);
    assert!(s[2].contains("骑 贝鲁卡 HP 0/40 已倒下，伤害全由人承担"), "{}", s[2]);
    let buf = {
        let mut term = Terminal::new(TestBackend::new(120, 40)).unwrap();
        term.draw(|f| ui::render(f, &mut a)).unwrap();
        term.backend().buffer().clone()
    };
    let x = s[2].find("已倒下").map(|b| s[2][..b].width() as u16).unwrap();
    assert_eq!(buf[(x, 2)].fg, ratatui::style::Color::Red, "归零提示是红字");
}

#[test]
fn no_rider_means_no_second_line_at_any_size() {
    for (w, h) in [(200, 60), (120, 40), (100, 30), (80, 24)] {
        let mut b = battle("贝鲁卡", false);
        let hero = b.units.get_mut(&0).unwrap();
        hero.ride = false;
        hero.mount = None;
        let mut a = battle_app(b, false);
        let s = draw(&mut a, w, h);
        assert!(!s.iter().any(|l| l.contains("骑 ")), "{w}×{h} 没骑乘不出现骑宠行");
    }
}

#[test]
fn observer_sees_the_rider_line_too() {
    let mut a = battle_app(battle("贝鲁卡", false), true);
    let s = draw(&mut a, 120, 40);
    assert!(s[1].contains("agent 操控中"), "观察模式第一行照旧");
    assert!(s[2].contains("骑 贝鲁卡 HP 30/40"), "观察模式下也显示骑宠：{}", s[2]);
}
