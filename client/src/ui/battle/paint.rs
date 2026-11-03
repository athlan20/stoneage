//! STAGE-167：在舞台范围内写字符；偏移和宽字符都按终端显示列裁切。
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use unicode_width::UnicodeWidthChar;

pub(super) fn text(frame: &mut Frame, area: Rect, x: i32, y: i32, text: &str, style: Style) {
    if y < area.y as i32 || y >= area.bottom() as i32 {
        return;
    }
    let mut col = x;
    for ch in text.chars() {
        let w = UnicodeWidthChar::width(ch).unwrap_or(0) as i32;
        if w > 0 && col >= area.x as i32 && col + w <= area.right() as i32 {
            frame
                .buffer_mut()
                .set_string(col as u16, y as u16, ch.to_string(), style);
        }
        col += w;
    }
}

pub(super) fn glyph(frame: &mut Frame, area: Rect, x: i32, y: i32, ch: char, color: Color) {
    text(
        frame,
        area,
        x,
        y,
        &ch.to_string(),
        Style::default().fg(color),
    );
}
