//! STAGE-242：宠物详情的两行汇总（`docs/ui.md` 宠物页 A）。
//! 「总成长」= 攻击、防御、敏捷三项成长率之和（不含 HP）；「总掉档」= 体力、腕力、耐久力、速度四项 tier 之和（含体力）。
//! 计算和汇总行的画法放在这里，详情的排版在 petdetail.rs。

use ratatui::text::{Line, Span};

use super::petdetail::{STATS, rate_text, tier_span};
use super::petpage::LABEL_STYLE;
use super::widgets::pad;
use crate::proto::{PetGrowth, PetQuality};

/// 总成长：攻击、防御、敏捷三项成长率之和。没有 growth，或任一项算不出（null）时返回 None。
pub fn growth_total(growth: Option<&PetGrowth>) -> Option<f64> {
    let r = &growth?.rates;
    Some(r.attack? + r.defense? + r.agility?)
}

/// 总掉档：四项 tier 之和，只算收到的项（缺的那项不计入，也不写 —）；四项都没有时返回 None。
pub fn tier_total(quality: &[PetQuality]) -> Option<i32> {
    STATS
        .iter()
        .filter_map(|(stat, _)| quality.iter().find(|q| q.stat == *stat))
        .map(|q| q.tier)
        .reduce(|a, b| a + b)
}

/// 左列：` 总成长 19.25`，标签样式与属性行一致，值两位小数，算不出写 —。
pub fn growth_total_cell(growth: Option<&PetGrowth>) -> Line<'static> {
    Line::from(vec![
        Span::styled(" 总成长 ", LABEL_STYLE),
        Span::raw(rate_text(growth_total(growth))),
    ])
}

/// 右列：` 总掉档     −7`，档位与上面的素质行对齐在同一列；0 写满（绿），负数写 −N（黄）；四项都没有写 —。
pub fn tier_total_cell(quality: &[PetQuality], unicode: bool) -> Line<'static> {
    let mut spans = vec![Span::raw(" "), Span::styled(pad("总掉档", 6), LABEL_STYLE)];
    match tier_total(quality) {
        Some(t) => {
            spans.push(Span::raw("     "));
            spans.push(tier_span(t, unicode));
        }
        None => spans.push(Span::styled("     —", LABEL_STYLE)),
    }
    Line::from(spans)
}
