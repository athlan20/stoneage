//! 宠物页右边的详情（STAGE-217，`docs/ui.md` 宠物页 A）。从上到下：昵称·种族·忠诚、等级·转生·经验、地水火风、
//! 两列表头、左列基础属性与成长率、右列隐藏素质档位、两行汇总（STAGE-242，见 petsummary.rs）、技能栏。
//! 80×24 时内容区只有 42 列、10 行（底部消息栏占 9 行），每一行都要放得下：长文字用 `…` 省略，不换行；
//! 左列表头按宽度从长到短选一个写法。STAGE-249：内容区高度不够时按 `parts_for` 取舍行，表头、四项属性、汇总必留，
//! 动作面板打开后只剩这 6 行（昵称、技能栏、等级、地水火风让位）。

use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph};
use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

use super::petpage::{HEADER_STYLE, LABEL_STYLE, status_label};
use super::petsummary::{growth_total_cell, tier_total_cell};
use super::widgets::pad;
use crate::proto::{ElementPoints, Pet, PetGrowth, PetQuality};

/// 左列（基础属性）至少这么宽，好放下“基础属性（成长：无记录）”这样的表头。
const LEFT_MIN: u16 = 26;
/// 右列（隐藏素质）的宽度：` 耐久力 50  −4` 约 14 列。
const RIGHT_W: u16 = 16;
/// 技能栏位数（`docs/protocol.md` 5.19，enemybase.txt 的 E_T_PETSKILL1..7）。
const SKILL_SLOTS: u8 = 7;
/// 素质的四项，顺序与 `docs/protocol.md` 5.19 的 `quality` 一致（服务端固定发这四项）。
pub(super) const STATS: [(&str, &str); 4] = [
    ("vital", "体力"),
    ("strength", "腕力"),
    ("toughness", "耐久力"),
    ("dexterity", "速度"),
];

/// STAGE-249：详情里能随高度取舍的行。表头、四项属性、汇总行是必留的 6 行；多出来的行按 `parts_for` 的顺序加。
const MUST_ROWS: u16 = 6;

/// 详情的一行（从上到下）。
enum Part {
    Nick,
    Level,
    Element,
    Header,
    Attr(usize),
    Summary,
    Skills,
}

/// STAGE-249：按内容区高度决定画哪些行，多出 1 行给昵称，2 行给技能栏，3 行给等级，4 行给地水火风。
/// 动作面板打开后 80×24、120×24 的详情只有 6 行，这时只剩表头、四项属性和汇总（`docs/ui.md` 宠物页）。
fn parts_for(height: u16) -> Vec<Part> {
    let extra = height.saturating_sub(MUST_ROWS);
    let mut out = vec![];
    if extra >= 1 {
        out.push(Part::Nick);
    }
    if extra >= 3 {
        out.push(Part::Level);
    }
    if extra >= 4 {
        out.push(Part::Element);
    }
    out.push(Part::Header);
    out.extend((0..4).map(Part::Attr));
    out.push(Part::Summary);
    if extra >= 2 {
        out.push(Part::Skills);
    }
    out
}

/// STAGE-256：动作面板打开、技能栏放不下时的提示。内容区没有多余的行可用（必留行已经占满），
/// 所以写在详情的下边框上，不挤掉任何一行。
const SKILLS_HIDDEN_HINT: &str = "技能栏放不下，关闭动作菜单后查看";

/// 画详情（含边框）。`area` 是整块区域，边框标题是 `昵称 · 号 · 状态`。`panel_open` 是动作面板是否打开。
pub fn render(frame: &mut Frame, unicode: bool, p: &Pet, area: Rect, panel_open: bool) {
    let (label, _) = status_label(p.status());
    let mut block = Block::bordered().title(format!("{} · {} 号 · {}", p.name, p.slot, label));
    let inner = block.inner(area);
    let parts = parts_for(inner.height);
    if panel_open && !parts.iter().any(|part| matches!(part, Part::Skills)) {
        // 边框两侧各占 1 列，文字前后各留 1 列空格。
        let width = usize::from(area.width).saturating_sub(4);
        if width >= 4 {
            let hint = ellipsize(SKILLS_HIDDEN_HINT, width);
            block = block.title_bottom(Span::styled(format!(" {hint} "), LABEL_STYLE));
        }
    }
    frame.render_widget(block, area);
    let rows = Layout::vertical(vec![Constraint::Length(1); parts.len()]).split(inner);

    // 两列的分界：左列至少 LEFT_MIN，宽终端时各占一半，右列至少 RIGHT_W。
    let left_w = (inner.width / 2).max(LEFT_MIN).min(inner.width.saturating_sub(RIGHT_W));
    let rates = p.growth.as_ref().map(|g| g.rates).unwrap_or_default();
    let left = [
        ability_cell("HP", format!("{}/{}", p.hp, p.max_hp), rate_text(rates.hp)),
        ability_cell("攻击", p.attack.to_string(), rate_text(rates.attack)),
        ability_cell("防御", p.defense.to_string(), rate_text(rates.defense)),
        ability_cell("敏捷", p.agility.to_string(), rate_text(rates.agility)),
    ];
    let right = STATS.map(|(stat, name)| {
        let q = p.quality.iter().find(|q| q.stat == stat);
        quality_cell(name, q, unicode)
    });

    for (part, row) in parts.iter().zip(rows.iter()) {
        match part {
            Part::Nick => frame.render_widget(Paragraph::new(nick_line(p, usize::from(inner.width))), *row),
            Part::Level => frame.render_widget(Paragraph::new(level_line(p)), *row),
            Part::Element => frame.render_widget(Paragraph::new(element_line(&p.elements)), *row),
            Part::Skills => frame.render_widget(Paragraph::new(skills_line(p)), *row),
            Part::Header | Part::Attr(_) | Part::Summary => {
                let [lr, rr] = Layout::horizontal([Constraint::Length(left_w), Constraint::Min(0)]).areas(*row);
                let (l, r) = match part {
                    Part::Header => {
                        let title = growth_header(p.growth.as_ref(), usize::from(left_w).saturating_sub(2));
                        (
                            Line::from(Span::styled(format!(" {title}"), HEADER_STYLE)),
                            Line::from(Span::styled(" 隐藏素质", HEADER_STYLE)),
                        )
                    }
                    Part::Attr(i) => (left[*i].clone(), right[*i].clone()),
                    _ => (
                        growth_total_cell(p.growth.as_ref()),
                        tier_total_cell(&p.quality, unicode),
                    ),
                };
                frame.render_widget(Paragraph::new(l), lr);
                frame.render_widget(Paragraph::new(r), rr);
            }
        }
    }
}

/// 技能栏：1–7 号栏位都写出来，没有技能的写 —（编号和命令参数一致）。
fn skills_line(p: &Pet) -> Line<'static> {
    let mut line = vec![Span::styled(" 技能  ", LABEL_STYLE)];
    for n in 1..=SKILL_SLOTS {
        match p.skills.iter().find(|s| s.slot == n) {
            Some(s) => line.push(Span::raw(format!("{n} {}  ", s.name))),
            None => line.push(Span::styled(format!("{n} —  "), LABEL_STYLE)),
        }
    }
    Line::from(line)
}

/// 一行「名 值」：名字用灰色，值是普通字。
fn field(name: &str, value: impl Into<String>) -> Vec<Span<'static>> {
    vec![Span::styled(format!(" {name}  "), LABEL_STYLE), Span::raw(value.into())]
}

/// 第一行：昵称、种族、忠诚。名字和种族超长时省略，忠诚是数字不省略。
fn nick_line(p: &Pet, width: usize) -> Line<'static> {
    let loyalty = p.loyalty.to_string();
    let fixed = " 昵称  ".width() + " 种族  ".width() + " 忠诚  ".width() + loyalty.width() + "    ".width() * 2;
    let budget = width.saturating_sub(fixed);
    // 种族最多占一半的预算，名字占剩下的；种族不用满时名字可以多占。
    let species_cap = p.species.width().min(budget / 2);
    let name = ellipsize(&p.name, budget.saturating_sub(species_cap));
    let species = ellipsize(&p.species, budget.saturating_sub(name.width()));
    let mut line = field("昵称", name);
    line.push(Span::raw("    "));
    line.extend(field("种族", species));
    line.push(Span::raw("    "));
    line.extend(field("忠诚", loyalty));
    Line::from(line)
}

/// 第二行：等级、转生次数、经验。经验按级计，已是最高级（next_exp 为 -1）时写满级。
fn level_line(p: &Pet) -> Line<'static> {
    let exp = if p.next_exp < 0 {
        "满级".to_string()
    } else {
        format!("{}/{}", p.exp, p.next_exp)
    };
    let mut line = field("等级", p.level.to_string());
    line.push(Span::raw("    "));
    line.extend(field("转生", p.transmigration.to_string()));
    line.push(Span::raw("    "));
    line.extend(field("经验", exp));
    Line::from(line)
}

/// 第三行：地水火风，颜色固定（地黄、水蓝、火红、风绿），数值是模板值。
fn element_line(e: &ElementPoints) -> Line<'static> {
    Line::from(vec![
        Span::raw(" "),
        Span::styled(format!("地 {}", e.earth), Style::default().fg(Color::Yellow)),
        Span::raw("  "),
        Span::styled(format!("水 {}", e.water), Style::default().fg(Color::Blue)),
        Span::raw("  "),
        Span::styled(format!("火 {}", e.fire), Style::default().fg(Color::Red)),
        Span::raw("  "),
        Span::styled(format!("风 {}", e.wind), Style::default().fg(Color::Green)),
    ])
}

/// 左列表头：按成长率的口径说明。宽度不够时依次去掉“成长/级”、去掉“基础属性（…）”的括号内容。
pub fn growth_header(growth: Option<&PetGrowth>, width: usize) -> String {
    let candidates: Vec<String> = match growth {
        Some(g) if g.basis == "caught" => match g.from_level {
            Some(lv) => vec![
                format!("基础属性（成长/级，自 Lv{lv} 起）"),
                format!("基础属性（自 Lv{lv} 起）"),
                "基础属性".into(),
            ],
            None => vec!["基础属性（成长/级）".into(), "基础属性".into()],
        },
        Some(g) if g.basis == "raised" => vec!["基础属性（成长/级）".into(), "基础属性".into()],
        // unknown（历史宠物）、旧服务端没有 growth 字段：不倒推，写无记录。
        _ => vec!["基础属性（成长：无记录）".into(), "基础属性".into()],
    };
    candidates
        .iter()
        .find(|t| t.width() <= width)
        .cloned()
        .unwrap_or_else(|| "基础属性".into())
}

/// 成长率的文字：两位小数，算不出（None）写 —。
pub fn rate_text(rate: Option<f64>) -> String {
    match rate {
        Some(v) => format!("{v:.2}"),
        None => "—".into(),
    }
}

/// 素质档位：0 是满，绿色；负数是距离满值的差，写成 −N（非 unicode 时用 -N），黄色。
pub fn tier_span(tier: i32, unicode: bool) -> Span<'static> {
    if tier >= 0 {
        Span::styled("满", Style::default().fg(Color::Green))
    } else {
        let minus = if unicode { '−' } else { '-' };
        Span::styled(
            format!("{minus}{}", tier.unsigned_abs()),
            Style::default().fg(Color::Yellow),
        )
    }
}

/// 左列一项：` HP 35/35 (7.25)`，成长率写在括号里。
fn ability_cell(name: &str, value: String, rate: String) -> Line<'static> {
    Line::from(vec![
        Span::styled(format!(" {name} "), LABEL_STYLE),
        Span::raw(format!("{value} ({rate})")),
    ])
}

/// 右列一项：` 体力    22  −1`。没有这一项的素质写 —，对齐在档位那一列。
fn quality_cell(name: &str, q: Option<&PetQuality>, unicode: bool) -> Line<'static> {
    let mut spans = vec![Span::raw(" "), Span::styled(pad(name, 6), LABEL_STYLE)];
    match q {
        Some(q) => {
            spans.push(Span::raw(format!("{:>3}  ", q.points)));
            spans.push(tier_span(q.tier, unicode));
        }
        None => spans.push(Span::styled("     —", LABEL_STYLE)),
    }
    Line::from(spans)
}

/// 按显示宽度截断，超出时末尾加 `…`（汉字占 2 列）。
pub fn ellipsize(text: &str, width: usize) -> String {
    if text.width() <= width {
        return text.to_string();
    }
    if width == 0 {
        return String::new();
    }
    let mut out = String::new();
    let mut used = 0;
    for c in text.chars() {
        let w = UnicodeWidthChar::width(c).unwrap_or(0);
        if used + w > width - 1 {
            break;
        }
        used += w;
        out.push(c);
    }
    out.push('…');
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn caught(from: Option<u32>) -> PetGrowth {
        PetGrowth {
            basis: "caught".into(),
            from_level: from,
            ..Default::default()
        }
    }

    #[test]
    fn rate_text_keeps_two_decimals_and_dash_for_none() {
        assert_eq!(rate_text(Some(7.25)), "7.25");
        assert_eq!(rate_text(Some(4.0)), "4.00");
        assert_eq!(rate_text(Some(1.1)), "1.10");
        assert_eq!(rate_text(None), "—");
    }

    #[test]
    fn tier_zero_is_full_green_and_negative_is_yellow_minus() {
        let full = tier_span(0, true);
        assert_eq!(full.content, "满");
        assert_eq!(full.style.fg, Some(Color::Green));
        for t in -4..=-1 {
            let s = tier_span(t, true);
            assert_eq!(s.content, format!("−{}", -t));
            assert_eq!(s.style.fg, Some(Color::Yellow));
        }
        assert_eq!(tier_span(-2, false).content, "-2", "非 unicode 时用 ASCII 减号");
    }

    #[test]
    fn growth_header_follows_basis_and_width() {
        let raised = PetGrowth {
            basis: "raised".into(),
            from_level: Some(1),
            ..Default::default()
        };
        assert_eq!(growth_header(Some(&raised), 40), "基础属性（成长/级）");
        assert_eq!(
            growth_header(Some(&caught(Some(5))), 40),
            "基础属性（成长/级，自 Lv5 起）"
        );
        // 80 列只有 24 列可写：去掉“成长/级”。
        assert_eq!(growth_header(Some(&caught(Some(10))), 24), "基础属性（自 Lv10 起）");
        assert_eq!(growth_header(Some(&caught(Some(10))), 10), "基础属性");
        let unknown = PetGrowth {
            basis: "unknown".into(),
            ..Default::default()
        };
        assert_eq!(growth_header(Some(&unknown), 24), "基础属性（成长：无记录）");
        assert_eq!(growth_header(None, 24), "基础属性（成长：无记录）");
    }

    #[test]
    fn ellipsize_cuts_wide_text_and_adds_mark() {
        assert_eq!(ellipsize("乌力", 4), "乌力");
        assert_eq!(ellipsize("勇者小明一二", 6), "勇者…");
        assert_eq!(ellipsize("abcdef", 3), "ab…");
        assert_eq!(ellipsize("abc", 0), "");
    }
}
