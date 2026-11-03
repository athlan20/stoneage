//! 宠物页动作面板与详情抢行（STAGE-249，`docs/ui.md` 宠物页）：面板打开时 80×24 和 120×24 的详情保留表头、
//! 四项属性、两行汇总，宠物表 5 栏不被挤掉；8 项动作靠滚动选中，第 8 项「放生」能看到；面板关闭时详情完整。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, Character, ElementPoints, Location, Pet, PetGrowth, PetQuality, PetRates, PetSkill, Pets, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

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

/// 含某段文字的那一行（没有就返回空串）。
fn line_with<'a>(s: &'a str, pat: &str) -> &'a str {
    s.lines().find(|l| l.contains(pat)).unwrap_or("")
}

/// 乌力：休息中，有成长记录、四项素质，和 `render_pet_detail.rs` 的 `raised()` 同口径。
fn raised() -> Pet {
    Pet {
        slot: 0,
        species_id: 1,
        species: "乌力".into(),
        name: "乌力".into(),
        level: 5,
        transmigration: 0,
        exp: 10,
        next_exp: 67,
        hp: 35,
        max_hp: 35,
        attack: 5,
        defense: 6,
        agility: 7,
        loyalty: 80,
        elements: ElementPoints {
            earth: 80,
            water: 20,
            fire: 0,
            wind: 0,
        },
        state: "rest".into(),
        skills: vec![PetSkill {
            slot: 1,
            id: 1,
            name: "攻击".into(),
            ..Default::default()
        }],
        growth: Some(PetGrowth {
            basis: "raised".into(),
            from_level: Some(1),
            rates: PetRates {
                hp: Some(7.25),
                attack: Some(4.0),
                defense: Some(5.0),
                agility: Some(3.25),
            },
        }),
        quality: vec![
            PetQuality {
                stat: "vital".into(),
                points: 22,
                tier: -1,
            },
            PetQuality {
                stat: "strength".into(),
                points: 30,
                tier: 0,
            },
            PetQuality {
                stat: "toughness".into(),
                points: 50,
                tier: -4,
            },
            PetQuality {
                stat: "dexterity".into(),
                points: 12,
                tier: -2,
            },
        ],
    }
}

/// 进入世界、切到「3 宠物」页，栏位 0 放一只宠物（默认选中它）。
fn pet_page(pet: Pet) -> App {
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
    let mut world = WorldState::new(entry);
    world.nav.push(Layer::Page(0));
    world.pets = Pets {
        max_slots: 5,
        pets: vec![pet],
    };
    a.world = Some(world);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('3'));
    a
}

#[test]
fn panel_open_keeps_attributes_and_summary_at_80x24_and_120x24() {
    for w in [80, 120] {
        let mut a = pet_page(raised());
        key(&mut a, KeyCode::Enter);
        let s = draw(&mut a, w, 24);
        for want in [
            "HP 35/35",
            "攻击 5",
            "防御 6",
            "敏捷 7",
            "体力",
            "总成长",
            "总掉档",
            "基础属性",
            "隐藏素质",
        ] {
            assert!(s.contains(want), "{w}×24 动作面板打开，详情缺 {want}\n{s}");
        }
        assert!(
            line_with(&s, "总成长").contains("总掉档"),
            "{w}×24 两行汇总在同一行\n{s}"
        );
        assert!(line_with(&s, "总成长").contains("−7"), "{w}×24 总掉档 −7\n{s}");
    }
}

#[test]
fn panel_open_keeps_the_pet_table_intact_at_80x24() {
    let mut a = pet_page(raised());
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 80, 24);
    assert!(
        s.contains("宠物栏位") && s.contains("编号") && s.contains("乌力"),
        "宠物表表头和出战行都在\n{s}"
    );
    for slot in ["1", "2", "3", "4"] {
        assert!(s.contains(&format!("{slot}    —")), "宠物表第 {slot} 栏还在\n{s}");
    }
}

#[test]
fn eighth_action_scrolls_into_view_and_runs_at_80x24() {
    let mut a = pet_page(raised());
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("出战") && s.contains("待命"), "80×24 第一屏露前两项\n{s}");
    assert!(!s.contains("放生"), "80×24 第一屏放不下第 8 项\n{s}");

    for _ in 0..7 {
        key(&mut a, KeyCode::Down);
    }
    let s = draw(&mut a, 80, 24);
    assert!(
        s.contains("对话") && s.contains("放生"),
        "滚到第 8 项，第 7、8 项可见\n{s}"
    );

    key(&mut a, KeyCode::Enter);
    assert!(
        a.log.all().iter().any(|m| m.text.contains("放生后不能恢复")),
        "选中的确实是放生，进入确认"
    );
}

#[test]
fn disabled_reason_is_visible_when_its_row_scrolls_into_view_at_80x24() {
    // 休息中的宠物：「休息」是第 3 项，滚到它时灰字说明在同一行可见。
    let mut a = pet_page(raised());
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    let s = draw(&mut a, 80, 24);
    assert!(
        line_with(&s, "已经在休息").contains("休息  （已经在休息）"),
        "80×24 滚到休息，说明在动作行里\n{s}"
    );
}

#[test]
fn panel_closed_keeps_the_full_detail_at_80x24_and_120x24() {
    for w in [80, 120] {
        let mut a = pet_page(raised());
        let s = draw(&mut a, w, 24);
        for want in [
            "昵称",
            "忠诚",
            "等级",
            "地 80",
            "HP 35/35",
            "攻击 5",
            "总成长",
            "总掉档",
            "技能",
        ] {
            assert!(s.contains(want), "{w}×24 面板关闭，详情完整，缺 {want}\n{s}");
        }
    }
}

#[test]
fn taller_window_gives_the_panel_all_eight_rows_and_the_detail_more_rows() {
    // 80×32：中间 20 行，面板 10 行（8 项全露），详情 10 行中去掉两行汇总以外的，昵称、技能栏回来。
    let mut a = pet_page(raised());
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 80, 32);
    assert!(s.contains("放生"), "80×32 八项都在第一屏\n{s}");
    assert!(
        s.contains("忠诚") && s.contains("技能"),
        "80×32 详情比 80×24 多出昵称和技能栏\n{s}"
    );
    assert!(line_with(&s, "总成长").contains("总掉档"), "80×32 两行汇总都在\n{s}");
}

const SKILLS_HIDDEN: &str = "技能栏放不下，关闭动作菜单后查看";

#[test]
fn skills_hidden_hint_shows_when_panel_open_at_80x24_and_120x24() {
    // STAGE-256 D1：面板打开、技能栏放不下，详情下边框写提示，内容行不变。
    for w in [80, 120] {
        let mut a = pet_page(raised());
        key(&mut a, KeyCode::Enter);
        let s = draw(&mut a, w, 24);
        assert!(s.contains(SKILLS_HIDDEN), "{w}×24 面板打开，技能栏放不下，应有提示\n{s}");
        assert!(!s.contains("技能  1"), "{w}×24 技能栏确实省略了\n{s}");
        assert!(
            line_with(&s, "总成长").contains("总掉档"),
            "{w}×24 提示不挤掉两行汇总\n{s}"
        );
        assert!(s.contains("HP 35/35") && s.contains("体力"), "{w}×24 必留行都在\n{s}");
    }
}

#[test]
fn skills_hidden_hint_absent_when_skills_fit_or_panel_closed() {
    // 80×32：技能栏放得下，没有提示。
    let mut a = pet_page(raised());
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 80, 32);
    assert!(s.contains("技能  1"), "80×32 技能栏放得下\n{s}");
    assert!(!s.contains(SKILLS_HIDDEN), "80×32 技能栏放得下，不应有提示\n{s}");

    // 面板关闭、80×24：技能栏也在，没有提示。
    let mut a = pet_page(raised());
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains(SKILLS_HIDDEN), "面板关闭不应有提示\n{s}");

    // 面板关闭、矮终端把技能栏省掉：仍然不写提示（只在动作面板打开时提示）。
    let mut a = pet_page(raised());
    let s = draw(&mut a, 80, 20);
    assert!(!s.contains("技能  1"), "80×20 面板关闭时技能栏省略\n{s}");
    assert!(!s.contains(SKILLS_HIDDEN), "面板关闭不应有提示\n{s}");
}

#[test]
fn hp_row_keeps_the_space_before_the_rate_with_panel_open_at_80x24() {
    // STAGE-256 O3：面板打开时 HP 行和关闭时一样是「HP 32/32 (—)」，括号前有空格。
    let mut p = raised();
    p.hp = 32;
    p.max_hp = 32;
    p.growth = None;
    for open in [false, true] {
        let mut a = pet_page(p.clone());
        if open {
            key(&mut a, KeyCode::Enter);
        }
        let s = draw(&mut a, 80, 24);
        let row = s.lines().find(|l| l.contains("HP 32/32")).unwrap_or("");
        assert!(row.contains("HP 32/32 (—)"), "面板{}时 HP 行缺空格\n{s}", if open { "打开" } else { "关闭" });
    }
}
