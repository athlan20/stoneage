//! 宠物详情重排（STAGE-217，`docs/ui.md` 宠物页 A）的界面测试：四种成长口径、五档素质、地水火风颜色、
//! ASCII 降级、80×24 与 120×40 的放得下、满级、缺字段的旧宠物。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use ratatui::buffer::Buffer;
use ratatui::style::Color;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, Character, ElementPoints, Location, Pet, PetGrowth, PetQuality, PetRates, PetSkill, Pets, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode,
        ..Options::default()
    })
}

/// 画一帧，返回缓冲区（要看颜色时用）。
fn buffer(app: &mut App, w: u16, h: u16) -> Buffer {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    term.backend().buffer().clone()
}

/// 整屏文字，宽字符只取一次。
fn text(buf: &Buffer) -> String {
    let area = *buf.area();
    (0..area.height)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < area.width {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    text(&buffer(app, w, h))
}

/// 缓冲区里所有等于 `sym` 的单元格的前景色。
fn colors_of(buf: &Buffer, sym: &str) -> Vec<Color> {
    let area = *buf.area();
    let mut out = vec![];
    for y in 0..area.height {
        for x in 0..area.width {
            let cell = &buf[(x, y)];
            if cell.symbol() == sym {
                out.push(cell.fg);
            }
        }
    }
    out
}

fn key(app: &mut App, code: KeyCode) {
    app.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

/// 乌力：编号 0 的 5 级宠物，休息。成长率和素质由各个测试按需要改。
fn wuli(slot: u8) -> Pet {
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
        growth: None,
        quality: vec![],
    }
}

/// 1 级起养、现在 5 级的乌力（`docs/protocol.md` 5.19 的示例）。
fn raised() -> Pet {
    let mut p = wuli(0);
    p.growth = Some(PetGrowth {
        basis: "raised".into(),
        from_level: Some(1),
        rates: PetRates {
            hp: Some(7.25),
            attack: Some(1.1),
            defense: Some(0.85),
            agility: Some(0.92),
        },
    });
    p.quality = vec![
        PetQuality {
            stat: "vital".into(),
            points: 22,
            tier: -1,
        },
        PetQuality {
            stat: "strength".into(),
            points: 18,
            tier: -2,
        },
        PetQuality {
            stat: "toughness".into(),
            points: 20,
            tier: 0,
        },
        PetQuality {
            stat: "dexterity".into(),
            points: 24,
            tier: -4,
        },
    ];
    p
}

/// 进入世界并切到「3 宠物」页。
fn pet_page(pet: Option<Pet>, unicode: bool) -> App {
    let mut a = app(unicode);
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
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.pets = Pets {
        max_slots: 5,
        pets: pet.into_iter().collect(),
    };
    a.world = Some(world);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('3'));
    a
}

/// 详情里的一行（含某段文字）。
fn line_with<'a>(s: &'a str, want: &str) -> &'a str {
    s.lines()
        .find(|l| l.contains(want))
        .unwrap_or_else(|| panic!("没有含 {want} 的行\n{s}"))
}

#[test]
fn raised_pet_shows_rates_and_header() {
    let mut a = pet_page(Some(raised()), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("基础属性（成长/级）"), "1 级起养的表头\n{s}");
    assert!(!s.contains("自 Lv"), "1 级起养不写起始等级\n{s}");
    assert!(line_with(&s, "HP 35/35").contains("(7.25)"), "HP 成长率两位小数\n{s}");
    assert!(line_with(&s, "攻击").contains("(1.10)"), "攻击成长率\n{s}");
    assert!(line_with(&s, "防御").contains("(0.85)"), "防御成长率\n{s}");
    assert!(line_with(&s, "敏捷").contains("(0.92)"), "敏捷成长率\n{s}");
}

#[test]
fn caught_pet_header_names_the_start_level() {
    let mut p = raised();
    p.growth = Some(PetGrowth {
        basis: "caught".into(),
        from_level: Some(5),
        rates: PetRates::default(),
    });
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 120, 40);
    assert!(
        s.contains("基础属性（成长/级，自 Lv5 起）"),
        "捕获后升级的表头写起始等级\n{s}"
    );
}

/// 80 列只有 24 列可写左列表头：去掉“成长/级”，但“自 Lv5 起”保留。
#[test]
fn caught_header_keeps_start_level_at_80_columns() {
    let mut p = raised();
    p.growth = Some(PetGrowth {
        basis: "caught".into(),
        from_level: Some(5),
        rates: PetRates::default(),
    });
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("基础属性（自 Lv5 起）"), "80 列表头\n{s}");
}

/// 刚获得、还没升过级：起始等级就是现在的等级，成长率全是 —。
#[test]
fn freshly_caught_pet_has_no_rates_yet() {
    let mut p = wuli(0);
    p.growth = Some(PetGrowth {
        basis: "caught".into(),
        from_level: Some(5),
        rates: PetRates::default(),
    });
    p.quality = raised().quality;
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("自 Lv5 起"), "表头写起始等级\n{s}");
    for want in ["HP 35/35 (—)", "攻击 5 (—)", "防御 6 (—)", "敏捷 7 (—)"] {
        assert!(s.contains(want), "缺少 {want}\n{s}");
    }
}

/// 历史宠物（unknown）：不倒推，写成无记录，成长率全是 —。
#[test]
fn unknown_pet_says_no_record() {
    let mut p = wuli(0);
    p.growth = Some(PetGrowth {
        basis: "unknown".into(),
        from_level: None,
        rates: PetRates::default(),
    });
    let mut a = pet_page(Some(p), true);
    for (w, h) in [(80, 24), (120, 40)] {
        let s = draw(&mut a, w, h);
        assert!(s.contains("基础属性（成长：无记录）"), "{w}×{h} 缺无记录表头\n{s}");
        assert!(line_with(&s, "HP 35/35").contains("(—)"), "{w}×{h} 成长率是 —\n{s}");
    }
}

/// 旧服务端没有 growth、quality 字段：按无记录和 — 处理，不出错。
#[test]
fn pet_without_growth_or_quality_fields_shows_dashes() {
    let old: Pet = serde_json::from_str(
        r#"{"slot":0,"species":"乌力","name":"乌力","level":5,"hp":35,"max_hp":35,"attack":5,"defense":6,"agility":7}"#,
    )
    .unwrap();
    let mut a = pet_page(Some(old), true);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("基础属性（成长：无记录）"), "\n{s}");
    for name in ["体力", "腕力", "耐久力", "速度"] {
        assert!(line_with(&s, name).contains('—'), "{name} 写 —\n{s}");
    }
}

/// 五档逐一：0 是“满”绿色，−1 到 −4 是黄色的 −N。
#[test]
fn quality_tiers_render_full_green_and_yellow_minus() {
    let mut p = wuli(0);
    p.quality = vec![PetQuality {
        stat: "vital".into(),
        points: 22,
        tier: 0,
    }];
    let mut a = pet_page(Some(p), true);
    let buf = buffer(&mut a, 120, 40);
    assert!(text(&buf).contains("满"), "tier 0 写满");
    // STAGE-242：只有体力一项时，素质行和总掉档（只算收到的项，合计 0）各一个“满”。
    assert_eq!(colors_of(&buf, "满"), vec![Color::Green, Color::Green], "满是绿色");

    for n in 1..=4 {
        let mut p = wuli(0);
        p.quality = vec![PetQuality {
            stat: "dexterity".into(),
            points: 24,
            tier: -n,
        }];
        let mut a = pet_page(Some(p), true);
        let buf = buffer(&mut a, 120, 40);
        let s = text(&buf);
        assert!(
            line_with(&s, "速度").contains(&format!("−{n}")),
            "tier -{n} 写 −{n}\n{s}"
        );
        // STAGE-242：素质行和总掉档各一个 −N，都是黄色。
        assert_eq!(colors_of(&buf, "−"), vec![Color::Yellow, Color::Yellow], "−{n} 是黄色");
    }
}

#[test]
fn elements_have_fixed_colors() {
    let mut a = pet_page(Some(raised()), true);
    let buf = buffer(&mut a, 120, 40);
    assert_eq!(colors_of(&buf, "地"), vec![Color::Yellow]);
    assert_eq!(colors_of(&buf, "水"), vec![Color::Blue]);
    assert_eq!(colors_of(&buf, "火"), vec![Color::Red]);
    assert_eq!(colors_of(&buf, "风"), vec![Color::Green]);
    let s = text(&buf);
    assert!(
        line_with(&s, "地").contains("地 80  水 20  火 0  风 0"),
        "数值是模板值\n{s}"
    );
}

/// `--unicode false`：没有 Braille、粗线和真正的减号，档位用 ASCII 的 -。
#[test]
fn ascii_mode_has_no_braille_thick_line_or_real_minus() {
    let mut a = pet_page(Some(raised()), false);
    for (w, h) in [(80, 24), (120, 40)] {
        let s = draw(&mut a, w, h);
        assert!(
            !s.chars().any(|c| ('\u{2800}'..='\u{28ff}').contains(&c)),
            "{w}×{h} 不能有 Braille\n{s}"
        );
        assert!(!s.contains('━'), "{w}×{h} 不能有粗线\n{s}");
        assert!(!s.contains('−'), "{w}×{h} 不能有真正的减号\n{s}");
        assert!(line_with(&s, "体力").contains("-1"), "{w}×{h} 用 ASCII 减号\n{s}");
    }
}

/// 80×24 和 120×40：四项两列都在，技能行在，每行右边框都还在（没有越界）。
#[test]
fn detail_fits_80x24_and_120x40() {
    for (w, h) in [(80, 24), (120, 40)] {
        let mut a = pet_page(Some(raised()), true);
        let s = draw(&mut a, w, h);
        for (label, value) in [
            ("HP 35/35", "35/35"),
            ("体力", "22"),
            ("腕力", "18"),
            ("耐久力", "20"),
            ("速度", "24"),
        ] {
            let row = line_with(&s, label);
            assert!(row.contains(value), "{w}×{h} {label} 行缺 {value}\n{s}");
        }
        assert!(
            line_with(&s, "HP 35/35").contains("体力"),
            "{w}×{h} 左右两列同一行\n{s}"
        );
        assert!(line_with(&s, "技能").contains("1 攻击"), "{w}×{h} 技能行在\n{s}");
        for want in ["昵称", "种族", "忠诚", "等级", "转生", "经验", "地", "隐藏素质"] {
            assert!(s.contains(want), "{w}×{h} 缺 {want}\n{s}");
        }
        // 详情的每一行都以右边框结束：内容没有顶出去。
        for want in ["HP 35/35", "技能", "地 80"] {
            let row = line_with(&s, want);
            assert!(row.trim_end().ends_with('│'), "{w}×{h} 行越界：{row}");
        }
    }
}

/// 满级（next_exp 为 -1）显示“满级”，不显示经验分数。
#[test]
fn max_level_shows_full_instead_of_exp() {
    let mut p = raised();
    p.next_exp = -1;
    p.exp = 999;
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 120, 40);
    assert!(line_with(&s, "等级").contains("经验  满级"), "满级\n{s}");
    assert!(!s.contains("999/"), "满级不写经验分数\n{s}");
}

/// 昵称和种族过长时用 … 省略，而不是换行或顶出边框。
#[test]
fn long_nickname_is_ellipsized_at_80_columns() {
    let mut p = raised();
    p.name = "一二三四五六七八九十".into();
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains('…'), "长名字用 … 省略\n{s}");
    assert!(line_with(&s, "昵称").trim_end().ends_with('│'), "昵称行不越界\n{s}");
}

/// STAGE-242：总成长 = 攻击 + 防御 + 敏捷（不含 HP），两位小数；总掉档 = 四项 tier 之和（含体力）。
/// raised：1.10 + 0.85 + 0.92 = 2.87（HP 的 7.25 不计入）；tier −1、−2、0、−4 合计 −7。两行同一行。
#[test]
fn summary_rows_total_growth_and_tiers_at_120x40() {
    let mut a = pet_page(Some(raised()), true);
    let s = draw(&mut a, 120, 40);
    let row = line_with(&s, "总成长");
    assert!(row.contains("2.87"), "总成长不含 HP\n{s}");
    assert!(
        row.contains("总掉档") && row.contains("−7"),
        "总掉档与总成长同一行\n{s}"
    );
}

/// STAGE-242：80×24 和 120×24 的详情区都是 10 行（底部消息栏 9 行），两行汇总、四项属性、技能栏都在，每行不越界。
#[test]
fn summary_rows_fit_at_80x24_and_120x24() {
    for w in [80, 120] {
        let mut a = pet_page(Some(raised()), true);
        let s = draw(&mut a, w, 24);
        for want in ["HP 35/35", "体力", "腕力", "耐久力", "速度", "总成长", "总掉档", "技能"] {
            assert!(s.contains(want), "{w}×24 缺 {want}\n{s}");
        }
        for want in ["HP 35/35", "总成长", "技能"] {
            let row = line_with(&s, want);
            assert!(row.trim_end().ends_with('│'), "{w}×24 {want} 行越界：{row}");
        }
        assert!(line_with(&s, "技能").contains("1 攻击"), "{w}×24 技能栏完整\n{s}");
    }
}

/// STAGE-242 起，打开动作面板会挤小详情区；STAGE-249 改为：80×24 下面板只露 2 行，详情保留两行汇总
/// （更多断言见 `render_pet_panel.rs`）。这里 120×40 也要两行汇总都在。
#[test]
fn summary_rows_stay_with_action_panel_open() {
    let mut a = pet_page(Some(raised()), true);
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 80, 24);
    assert!(
        line_with(&s, "总成长").contains("总掉档"),
        "STAGE-249：80×24 面板打开，两行汇总同一行都在\n{s}"
    );

    let mut a = pet_page(Some(raised()), true);
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 120, 40);
    assert!(
        line_with(&s, "总成长").contains("总掉档"),
        "120×40 有动作面板也够高，两行汇总同一行都在\n{s}"
    );
}

/// STAGE-242：没有成长记录、没有素质时两行汇总都写 —，不写 0.00，也不写 满。
#[test]
fn summary_rows_show_dash_without_growth_or_quality() {
    let mut a = pet_page(Some(wuli(0)), true);
    let s = draw(&mut a, 120, 24);
    assert!(line_with(&s, "总成长").contains('—'), "总成长写 —\n{s}");
    assert!(line_with(&s, "总掉档").contains('—'), "总掉档写 —\n{s}");
    assert!(!s.contains("0.00") && !s.contains("满"), "不写 0.00 和满\n{s}");
}

/// STAGE-242：缺的素质不计入总掉档（不写 —）；只缺一项成长率时总成长写 —。
#[test]
fn summary_totals_skip_missing_items() {
    let mut p = raised();
    p.quality.retain(|q| q.stat != "vital");
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 120, 24);
    assert!(line_with(&s, "总掉档").contains("−6"), "缺体力时 −2 −0 −4 = −6\n{s}");

    let mut p = raised();
    if let Some(g) = p.growth.as_mut() {
        g.rates.agility = None;
    }
    let mut a = pet_page(Some(p), true);
    let s = draw(&mut a, 120, 24);
    assert!(line_with(&s, "总成长").contains('—'), "缺一项成长率时总成长写 —\n{s}");
}

/// STAGE-242：四项 tier 都是 0 时总掉档写“满”（绿色）；无 Unicode 时减号用 ASCII。
#[test]
fn summary_tier_total_full_green_and_ascii_minus() {
    let mut p = raised();
    for q in &mut p.quality {
        q.tier = 0;
    }
    let mut a = pet_page(Some(p), true);
    let buf = buffer(&mut a, 120, 24);
    assert!(line_with(&text(&buf), "总掉档").contains("满"), "总掉档写满");
    assert_eq!(colors_of(&buf, "满").len(), 5, "四项加总掉档共 5 个满");
    assert!(colors_of(&buf, "满").iter().all(|c| *c == Color::Green), "满是绿色");

    let mut a = pet_page(Some(raised()), false);
    let s = draw(&mut a, 120, 24);
    assert!(line_with(&s, "总掉档").contains("-7"), "ASCII 模式写 -7\n{s}");
}
