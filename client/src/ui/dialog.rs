//! NPC 对话窗口（STAGE-69，`docs/ui.md`「服务端驱动的界面」）：中间区域上方弹出的
//! 居中带边框窗口（`Clear` 清掉底下的内容），标题是 NPC 名字，正文按行显示，
//! 选项带编号。窗口打开时输入行为空按数字选选项（没有选项就是关掉），`Esc` 关闭。

use ratatui::Frame;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Clear, Paragraph, Wrap};

use super::widgets::centered;
use crate::app::App;

/// 窗口宽度（放不下时贴着区域宽度收窄）。
const WINDOW_WIDTH: u16 = 56;

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let Some(w) = world.npc_window.as_ref() else {
        return;
    };
    // 强者表的排行页（STAGE-308）：表格窗口。服务端推的窗口都带 window_id（按键走菜单式），
    // 所以要排在菜单窗口前面判断，否则表格永远画不出来。
    if let Some(rk) = w.ranking.as_ref() {
        let selected = (!w.window_id.is_empty() && !w.options.is_empty())
            .then(|| world.dialog.index.min(w.options.len() - 1));
        super::dialog_ranking::render(frame, area, w, rk, selected);
        return;
    }
    if !w.window_id.is_empty() {
        super::dialog_menu::render(frame, world, area);
        return;
    }
    // 正文按 \n 分行，每行再按宽度换行（告示牌一类的长文）。
    let body_width = usize::from(WINDOW_WIDTH.saturating_sub(2)).max(10);
    let mut lines: Vec<String> = vec![];
    for raw in w.text.split('\n') {
        let wrapped = crate::app::msgview::wrap_text(raw, body_width);
        if wrapped.is_empty() {
            lines.push(String::new());
        }
        lines.extend(wrapped.iter().map(|s| (*s).to_string()));
    }
    let has_options = !w.options.is_empty();
    let hint = if w.accepts_text {
        "reply 文本 发送 · Esc 关闭"
    } else if has_options {
        "数字选择 · Esc 关闭"
    } else {
        "按数字或 Esc 关闭"
    };
    let mut content: Vec<Line> = vec![Line::raw("")];
    for l in &lines {
        content.push(Line::raw(format!(" {l}")));
    }
    if has_options {
        content.push(Line::raw(""));
        for o in &w.options {
            content.push(Line::from(vec![
                Span::styled(format!(" {}. ", o.no), Style::default().fg(Color::Yellow)),
                Span::raw(o.label.clone()),
            ]));
        }
    }
    content.push(Line::raw(""));
    content.push(Line::styled(
        format!(" {hint}"),
        Style::default().fg(Color::DarkGray),
    ));

    // 高度按内容的行数算（+ 上下边框），窗口放不下时贴着区域高度截断。
    let height = content.len() as u16 + 2;
    let pop = centered(
        area,
        WINDOW_WIDTH.min(area.width.saturating_sub(2)),
        height.min(area.height),
    );
    frame.render_widget(Clear, pop);

    // 标题：名字（有类型时带上），黄字；空名字用类型名。
    let title_text = if w.name.is_empty() {
        w.type_.clone()
    } else {
        w.name.clone()
    };
    let title = if w.type_.is_empty() || w.name == w.type_ {
        title_text
    } else {
        format!("{title_text} · {}", w.type_)
    };
    let block = Block::bordered().title(Span::styled(
        title,
        Style::default()
            .fg(Color::Yellow)
            .add_modifier(Modifier::BOLD),
    ));
    let inner = block.inner(pop);
    frame.render_widget(block, pop);
    frame.render_widget(Paragraph::new(content).wrap(Wrap { trim: false }), inner);
}
