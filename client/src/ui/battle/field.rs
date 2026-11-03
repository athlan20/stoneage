//! STAGE-167：敌左我右的四列舞台。前后排变为左右列，编号和规则保持原样。
use super::{Tier, actor, effect, position};
use crate::app::battle::{
    BattleState,
    slots::{self, Cell},
};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
    widgets::Paragraph,
};
use std::{collections::BTreeMap, time::Instant};

pub struct Look {
    pub now: Instant,
    pub unicode: bool,
    pub focus: Option<Cell>,
    pub reduce_motion: bool,
}

pub(super) fn homes(b: &BattleState, area: Rect, tier: Tier) -> BTreeMap<u8, Rect> {
    let h = if tier == Tier::Full { 5 } else { 2 };
    let cw = (area.width.saturating_sub(8) / 5).clamp(12, 18);
    let top = area.y + area.height.saturating_sub(h * 5) / 2;
    let fractions = [12, 32, 68, 88];
    let mut home = BTreeMap::new();
    for no in 0..20 {
        let cell = slots::cell_of(no);
        let Some(unit) = b.unit_at(cell) else {
            continue;
        };
        let center = area.x + area.width * fractions[slots::screen_column(b.me_side(), no)] / 100;
        let x = center
            .saturating_sub(cw / 2)
            .clamp(area.x, area.right().saturating_sub(cw));
        home.insert(unit.no, Rect::new(x, top + cell.col as u16 * h, cw, h));
    }
    home
}

pub fn render(frame: &mut Frame, b: &BattleState, area: Rect, tier: Tier, look: &Look) {
    let home = homes(b, area, tier);
    let poses = position::poses(b, &home, area, look);
    let mut units: Vec<_> = home.keys().copied().collect();
    units.sort_by_key(|no| poses.get(no) != home.get(no));
    for no in units {
        let (Some(unit), Some(at)) = (b.units.get(&no), poses.get(&no)) else {
            continue;
        };
        actor::draw(frame, b, unit, slots::cell_of(no), *at, area, tier, look);
    }
    if tier == Tier::Full && area.height >= 27 {
        let style = Style::default().fg(Color::DarkGray);
        frame.render_widget(
            Paragraph::new("敌方 · 后列 / 前列").style(style),
            Rect::new(area.x + 1, area.y, 24, 1),
        );
        frame.render_widget(
            Paragraph::new("我方 · 前列 / 后列").style(style),
            Rect::new(area.right().saturating_sub(25), area.y, 24, 1),
        );
    }
    // 仅绘制身体与效果；整句动作文字统一由下方战斗日志显示。
    effect::draw(frame, b, &poses, area, tier, look);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{BattleStart, BattleUnit};

    #[test]
    fn every_unit_is_on_its_army_side_and_pets_are_in_front_for_both_player_sides() {
        for you in [0, 10] {
            let b = BattleState::from_start(BattleStart {
                you,
                units: (0..20)
                    .map(|no| BattleUnit {
                        no,
                        hp_pct: 100,
                        ..Default::default()
                    })
                    .collect(),
                ..Default::default()
            });
            for (tier, width, height) in [(Tier::Full, 118, 25), (Tier::Compact, 78, 10)] {
                let area = Rect::new(1, 4, width, height);
                let at = homes(&b, area, tier);
                assert_eq!(at.len(), 20);
                for no in 0..20 {
                    let r = at[&no];
                    assert!(
                        area.contains((r.x, r.y).into())
                            && r.right() <= area.right()
                            && r.bottom() <= area.bottom()
                    );
                    if no / 10 == b.me_side() {
                        assert!(r.x > area.x + area.width / 2);
                    } else {
                        assert!(r.right() < area.x + area.width / 2);
                    }
                    if no % 10 < 5 {
                        assert_eq!(r.y, at[&(no + 5)].y, "主人和宠物横向同行");
                        if no / 10 == b.me_side() {
                            assert!(r.x > at[&(no + 5)].x);
                        } else {
                            assert!(r.x < at[&(no + 5)].x);
                        }
                    }
                }
            }
        }
    }
}
