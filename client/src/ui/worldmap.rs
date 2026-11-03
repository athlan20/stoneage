//! 世界地图（`docs/map.md`）：Canvas + 海岸线点阵，按画布比例留白，y 轴翻转。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::symbols::Marker;
use ratatui::text::{Line, Span};
use ratatui::widgets::canvas::{Canvas, Points};

use super::worldmap_data::{BOUNDS_X, BOUNDS_Y, JALGA, SAINASU};
use crate::proto::HOMETOWNS;

const MARGIN: f64 = 80.0;

/// 画布的世界坐标范围：取两座岛的外接框，再按画布点阵的宽高比扩成同比例，避免岛被拉伸。
fn bounds(area: Rect) -> ([f64; 2], [f64; 2]) {
    let (mut x0, mut x1) = (BOUNDS_X.0 - MARGIN, BOUNDS_X.1 + MARGIN);
    let (mut y0, mut y1) = (BOUNDS_Y.0 - MARGIN, BOUNDS_Y.1 + MARGIN);
    let dots_w = f64::from(area.width.max(1)) * 2.0;
    let dots_h = f64::from(area.height.max(1)) * 4.0;
    let want = dots_w / dots_h;
    let (w, h) = (x1 - x0, y1 - y0);
    if w / h < want {
        let extra = (h * want - w) / 2.0;
        x0 -= extra;
        x1 += extra;
    } else {
        let extra = (w / want - h) / 2.0;
        y0 -= extra;
        y1 += extra;
    }
    // 世界坐标 y 向南，Canvas 的 y 向上，所以取负。
    ([x0, x1], [-y1, -y0])
}

/// 地图上的一个标记：黄色 `X` 加文字（出生村名或地点编号）。
pub struct Mark {
    /// 世界格坐标。
    pub world: (f64, f64),
    pub label: String,
    pub highlight: bool,
}

/// 创建角色选出生地时的四个出生村标记；`selected` 是高亮的序号。
pub fn hometown_marks(selected: Option<usize>) -> Vec<Mark> {
    HOMETOWNS
        .iter()
        .enumerate()
        .map(|(i, t)| Mark { world: t.world, label: format!("X {}", t.name), highlight: selected == Some(i) })
        .collect()
}

/// 画世界地图。`player` 是玩家 `@` 的世界坐标（不在图上就传 `None`）。
/// 标记落在同一个格子时只画第一个（地点按编号传入，村里的标记让过近郊的），
/// 免得近郊点把村口的标记盖掉——地图上的 `X编号` 就是 `warp` 的参数（STAGE-81 去掉了右侧的地点表）。
pub fn draw(frame: &mut Frame, area: Rect, unicode: bool, marks: &[Mark], player: Option<(f64, f64)>) {
    let (xb, yb) = bounds(area);
    let coast: Vec<(f64, f64)> = SAINASU
        .iter()
        .chain(JALGA.iter())
        .map(|&(x, y)| (f64::from(x), -f64::from(y)))
        .collect();
    // 每个字符格对应的世界坐标跨度（x 方向盲文 2 点一格，标签按字符格打印）。
    let per_col = (xb[1] - xb[0]) / f64::from(area.width.max(1));
    let per_row = (yb[1] - yb[0]) / f64::from(area.height.max(1));
    // 世界坐标落在哪个字符格（去重和判断玩家是否站在标记上用同一个算法）。
    let cell = |(x, y): (f64, f64)| (((x - xb[0]) / per_col).round() as i32, ((-y - yb[0]) / per_row).round() as i32);
    let mut seen = std::collections::HashSet::new();
    let marks: Vec<&Mark> = marks.iter().filter(|m| seen.insert(cell(m.world))).collect();
    // 玩家站在选中的地点上：`@` 盖住 X，用反色让选中的地点仍看得出来（STAGE-116）。
    let on_highlight = player.is_some_and(|p| marks.iter().any(|m| m.highlight && cell(m.world) == cell(p)));
    let canvas = Canvas::default()
        .marker(if unicode { Marker::Braille } else { Marker::Dot })
        .x_bounds(xb)
        .y_bounds(yb)
        .paint(|ctx| {
            ctx.draw(&Points { coords: &coast, color: Color::White });
            ctx.layer();
            for m in &marks {
                let style = if m.highlight {
                    Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD | Modifier::REVERSED)
                } else {
                    Style::default().fg(Color::Yellow)
                };
                ctx.print(m.world.0, -m.world.1, Line::from(Span::styled(m.label.clone(), style)));
            }
            if let Some((x, y)) = player {
                ctx.layer();
                let mut style = Style::default().fg(Color::Green).add_modifier(Modifier::BOLD);
                if on_highlight {
                    style = style.add_modifier(Modifier::REVERSED);
                }
                ctx.print(x, -y, Line::from(Span::styled("@", style)));
            }
        });
    frame.render_widget(canvas, area);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bounds_keep_aspect_ratio() {
        for (w, h) in [(100, 30), (60, 40), (200, 20)] {
            let (xb, yb) = bounds(Rect::new(0, 0, w, h));
            let ratio = (xb[1] - xb[0]) / (yb[1] - yb[0]);
            let want = f64::from(w) * 2.0 / (f64::from(h) * 4.0);
            assert!((ratio - want).abs() < 1e-6, "{ratio} vs {want}");
        }
    }

    /// 画一帧，返回画面上 `@` 那一格的样式。
    fn at_style(marks: &[Mark], player: Option<(f64, f64)>) -> Option<Style> {
        let mut term = ratatui::Terminal::new(ratatui::backend::TestBackend::new(60, 20)).unwrap();
        term.draw(|f| draw(f, f.area(), true, marks, player)).unwrap();
        let buf = term.backend().buffer().clone();
        buf.content().iter().find(|c| c.symbol() == "@").map(|c| c.style())
    }

    #[test]
    fn player_on_the_selected_place_keeps_the_highlight_visible() {
        // 玩家站在选中的村子上：@ 盖住 X，要用反色让选中的地点仍看得出来（STAGE-116）。
        let marks = hometown_marks(Some(0));
        let on = at_style(&marks, Some(HOMETOWNS[0].world)).expect("@ 应该画出来");
        assert!(on.add_modifier.contains(Modifier::REVERSED));
        let off = at_style(&marks, Some(HOMETOWNS[1].world)).expect("@ 应该画出来");
        assert!(!off.add_modifier.contains(Modifier::REVERSED));
    }

    #[test]
    fn hometowns_inside_bounds() {
        let (xb, yb) = bounds(Rect::new(0, 0, 100, 30));
        for t in &HOMETOWNS {
            assert!(t.world.0 > xb[0] && t.world.0 < xb[1]);
            assert!(-t.world.1 > yb[0] && -t.world.1 < yb[1]);
        }
    }
}
