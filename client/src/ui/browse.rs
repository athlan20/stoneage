use crate::app::{App, BrowseStep, Layer};
use ratatui::{
    Frame,
    layout::{Constraint, Layout, Rect},
    style::{Color, Style},
    widgets::{Block, Clear, List, ListState, Paragraph},
};

pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(w) = app.world.as_ref() else {
        return;
    };
    if w.nav.is(&Layer::Detail) || w.nav.is(&Layer::Command) {
        return;
    }
    let Some(step) = &w.browse.step else {
        return;
    };
    let pop = super::widgets::centered(area, area.width.saturating_sub(2).min(90), area.height);
    frame.render_widget(Clear, pop);
    let (title, rows, index, note) = match step {
        BrowseStep::Quest { quest, index } => (
            quest.name.clone(),
            vec![
                "阅读攻略".into(),
                "当前步骤与相关 NPC".into(),
                "搜索 NPC".into(),
            ],
            *index,
            "↑↓ 选择 · Enter 打开 · Esc 返回".to_string(),
        ),
        BrowseStep::Npcs {
            quest,
            items,
            index,
            reason,
            loading,
        } => {
            let mut rows: Vec<String> = items
                .iter()
                .map(|i| {
                    format!(
                        "{} · {} · {}",
                        if i.relation == "current" {
                            "当前候选"
                        } else {
                            "仅供参考"
                        },
                        i.npc.name,
                        i.npc.map_name
                    )
                })
                .collect();
            rows.push("搜索 NPC".into());
            (
                format!("{} · NPC", quest.name),
                rows,
                *index,
                if *loading {
                    "正在查询…".into()
                } else {
                    reason.clone()
                },
            )
        }
        BrowseStep::Search { input } => {
            let block = Block::bordered().title("搜索百科 · Enter 搜索 · Esc 取消");
            let inner = block.inner(pop);
            frame.render_widget(block, pop);
            let (text, cursor) = input.view(usize::from(inner.width.saturating_sub(2)));
            frame.render_widget(Paragraph::new(format!("> {text}")), inner);
            frame.set_cursor_position((inner.x + 2 + cursor, inner.y));
            return;
        }
    };
    let block = Block::bordered().title(title);
    let inner = block.inner(pop);
    frame.render_widget(block, pop);
    let [list, footer] = Layout::vertical([Constraint::Min(1), Constraint::Length(2)]).areas(inner);
    let mut state = ListState::default().with_selected(Some(index));
    frame.render_stateful_widget(
        List::new(rows)
            .highlight_symbol("> ")
            .highlight_style(Style::default().fg(Color::Yellow)),
        list,
        &mut state,
    );
    frame.render_widget(
        Paragraph::new(note).wrap(ratatui::widgets::Wrap { trim: false }),
        footer,
    );
}
