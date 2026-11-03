//! 战斗画面（STAGE-37，`docs/battle-ui.md`「总体布局」「三档尺寸」）：中间区域整个换成战场，
//! 上边框右侧是倒计时，最下面一行是指令条；宽度够时右侧是战斗记录和目标信息，结束时叠一个结果窗口。

mod actor;
mod bar;
mod clone_panel;
mod card;
mod effect;
mod field;
mod footer;
mod fx;
mod gauge;
mod paint;
mod panel;
mod position;
mod result;

mod spell_effect;
mod topbar;
mod weapons;

pub use topbar::{line as top_line, mount_line, top_height};

use std::time::Instant;

use ratatui::Frame;
use ratatui::layout::{Alignment, Constraint, Layout, Rect};
use ratatui::style::{Color, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Paragraph};

use crate::app::App;
use crate::app::battle::{BattleState, Phase};
use crate::ui::world::BATTLE_BOTTOM_ROWS;
use field::Look;

/// 五个纵向站位，每个单位五行或两行。
const FULL_FIELD_HEIGHT: u16 = 25;
const MIN_INNER_HEIGHT: u16 = 14;

/// 角色的两档尺寸：由战场可用高度决定；下方日志随宽高调整。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Tier {
    Full,
    Compact,
}

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(b) = app.world.as_ref().and_then(|w| w.battle.as_ref()) else {
        return;
    };
    let now = Instant::now();
    let block = Block::bordered()
        .title(if b.duel { format!(" 决斗 · {} ", b.scene) } else { format!(" 战斗 · {} · 敌左我右 ", b.scene) })
        .title(countdown(b, now, app.unicode).right_aligned());
    let inner = block.inner(area);
    frame.render_widget(block, area);
    if inner.height < MIN_INNER_HEIGHT {
        frame.render_widget(
            Paragraph::new("战场太小，请把窗口拉大").alignment(Alignment::Center),
            inner,
        );
        return;
    }

    let [field_area, footer_area] = Layout::vertical([
        Constraint::Min(0),
        // 日志高度按顶栏以下的高度分档（STAGE-248）：内容区 = 战斗块高度 + 底部的行数。
        Constraint::Length(footer::height(inner.width, area.height + BATTLE_BOTTOM_ROWS)),
    ])
    .areas(inner);
    let tier = if field_area.height >= FULL_FIELD_HEIGHT {
        Tier::Full
    } else {
        Tier::Compact
    };
    let look = Look {
        now,
        unicode: app.unicode,
        reduce_motion: app.reduce_motion,
        focus: match b.pick {
            crate::app::battle::Pick::Target { .. } | crate::app::battle::Pick::CloneAim { .. } => Some(b.cursor),
            _ => None,
        },
    };
    field::render(frame, b, field_area, tier, &look);
    if b.pick.is_list() {
        panel::render_float(frame, app, b, field_area);
    }
    if b.pick == crate::app::battle::Pick::Clones {
        clone_panel::render_float(frame, b, field_area);
    }
    footer::render(frame, app, b, footer_area);
    if let Some(end) = b.result.as_ref() {
        result::render(frame, area, end, b.has_clones(), app.world.as_ref().is_some_and(|w| w.observing()));
    }
}

/// 上边框右侧的倒计时：`倒计时 ━━━━────  18s`，等出招或等别人时显示，结算时不显示。
fn countdown(b: &BattleState, now: Instant, unicode: bool) -> Line<'static> {
    if !matches!(b.phase, Phase::Input(_) | Phase::Waiting) {
        return Line::default();
    }
    let Some(left) = b.remaining_secs(now) else {
        // 真人队同场（STAGE-275）：还没有人交指令，没有截止时间，写“——”。
        if b.party {
            return Line::from(format!(" 倒计时 {} ", crate::app::battle::party::countdown_text(None)));
        }
        return Line::default();
    };
    const WIDTH: usize = 12;
    let window = b.turn_window.as_secs_f64().max(1.0);
    let ratio = (left as f64 / window).clamp(0.0, 1.0);
    let filled = (ratio * WIDTH as f64).round() as usize;
    let (full, empty) = if unicode { ("━", "─") } else { ("=", "-") };
    let color = if ratio > 0.5 {
        Color::Green
    } else if ratio > 0.2 {
        Color::Yellow
    } else {
        Color::Red
    };
    Line::from(vec![
        Span::raw(" 倒计时 "),
        Span::styled(
            format!("{}{}", full.repeat(filled), empty.repeat(WIDTH - filled)),
            Style::default().fg(color),
        ),
        Span::raw(format!(" {left:>2}s ")),
    ])
}
