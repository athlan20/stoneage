//! STAGE-167：最多六格的细线血条（▁），掉血残影仍使用独立颜色；ASCII 用 _ / = / .。
use ratatui::style::{Color, Style};
use ratatui::text::{Line, Span};

use super::card::hp_color;
use super::fx::Gauge;

/// 血条一行：实心（颜色按血量，倒下是灰的）、残影、空心。
pub fn thin(g: Gauge, width: usize, unicode: bool, down: bool) -> Line<'static> {
    let width = width.min(6);
    let fill = (g.fill.min(100) as usize * width).div_ceil(100);
    let ghost = (g.ghost.min(100) as usize * width).div_ceil(100).max(fill);
    let color = if down {
        Color::DarkGray
    } else {
        hp_color(g.fill)
    };
    Line::from(
        (0..width)
            .map(|i| {
                let fg = if i < fill {
                    color
                } else if i < ghost {
                    Color::LightRed
                } else {
                    Color::DarkGray
                };
                let mark = if unicode {
                    "▁"
                } else if i < fill {
                    "_"
                } else if i < ghost {
                    "="
                } else {
                    "."
                };
                Span::styled(mark, Style::default().fg(fg))
            })
            .collect::<Vec<_>>(),
    )
}
#[cfg(test)]
mod tests {
    use super::*;

    fn text(l: Line<'static>) -> String {
        l.spans.iter().map(|s| s.content.to_string()).collect()
    }

    fn g(fill: u32, ghost: u32) -> Gauge {
        Gauge { fill, ghost }
    }

    #[test]
    fn bars_are_short_and_visually_thin_in_both_character_modes() {
        assert_eq!(text(thin(g(100, 100), 12, true, false)), "▁▁▁▁▁▁");
        assert_eq!(text(thin(g(40, 40), 12, true, false)), "▁▁▁▁▁▁");
        assert_eq!(
            text(thin(g(40, 40), 12, false, false)),
            "___...",
            "ASCII 用下划线和点"
        );
        assert_eq!(text(thin(g(0, 0), 12, true, true)), "▁▁▁▁▁▁");
    }

    #[test]
    fn ghost_sits_between_the_fill_and_the_old_value() {
        assert_eq!(text(thin(g(40, 70), 12, true, false)), "▁▁▁▁▁▁");
        assert_eq!(
            text(thin(g(40, 70), 12, false, false)),
            "___==.",
            "ASCII 残影用 ="
        );
        assert_eq!(
            text(thin(g(40, 40), 12, true, false)),
            "▁▁▁▁▁▁",
            "没有残影时全部实心或空心色"
        );
    }
    #[test]
    fn damage_ghost_keeps_its_color_on_the_thin_track() {
        let line = thin(g(40, 70), 20, true, false);
        assert_eq!(line.spans.len(), 6, "宽屏也不拉长血条");
        assert_eq!(
            line.spans
                .iter()
                .filter(|s| s.style.fg == Some(Color::LightRed))
                .count(),
            2
        );
        assert!(line.spans.iter().all(|s| s.content == "▁"));
    }
}
