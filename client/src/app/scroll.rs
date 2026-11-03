//! 滚动：PgUp/PgDn 翻页（`page_bag` 背包页），Home/End 滚到最前或最后（STAGE-103，`docs/interaction.md`「按键表」）。
//! Home/End 只在输入行为空、当前页可以滚动时作用，其余时候是光标的行首、行尾。
//! 任务攻略、百科词条这类详情栏的滚动（STAGE-115）由 `detail_nav` 处理：焦点在详情栏时 `↑↓` 逐行滚动。

use super::select::Nav;
use super::world::{BAG_LAST_SLOT, BAG_MENU, MESSAGES_MENU, QUEST_MENU, WIKI_MENU, WorldState};
use super::App;

/// 背包格的第一格编号（装备格 0–4 之后的背包格从 5 开始）。
const FIRST_BAG_SLOT: usize = 5;

/// 当前页面是不是可滚动的（Home/End 的前提）。
pub fn scrolls(menu: usize) -> bool {
    matches!(menu, MESSAGES_MENU | BAG_MENU | QUEST_MENU | WIKI_MENU)
}

/// 背包页物品表滚到最后一页时顶部的格子编号：最后一页贴底。
fn bag_max_top(rows: usize) -> usize {
    let total = BAG_LAST_SLOT + 1 - FIRST_BAG_SLOT; // 背包格 5–19 共 15 个
    (FIRST_BAG_SLOT + total).saturating_sub(rows).max(FIRST_BAG_SLOT)
}

/// 「背包」页物品表整页滚动：顶部显示的格子编号在 5–19 之间，最后一页贴底。
pub(super) fn page_bag(world: &mut WorldState, down: bool) {
    let rows = world.bag_rows.max(1);
    let max_top = bag_max_top(rows);
    if down {
        world.bag_scroll = (world.bag_scroll + rows).min(max_top);
    } else {
        world.bag_scroll = world.bag_scroll.saturating_sub(rows).max(FIRST_BAG_SLOT).min(max_top);
    }
}

/// 列表跟着选中项滚动的顶部行号（STAGE-147）：先把上次的 `top` 夹到能滚的范围里（最后一页贴底，
/// 不留空白），选中项走出可见范围时把它收进来——往上出了顶就顶在顶部，往下出了底就贴着底。
/// 场景页的 NPC 列表、地图页的 NPC 表、任务页的任务列表都用这一套（`rows` 是可见行数，为 0 时是 0）。
pub fn follow_top(top: usize, selected: usize, rows: usize, total: usize) -> usize {
    if rows == 0 || total == 0 {
        return 0;
    }
    let mut top = top.min(total.saturating_sub(rows));
    if selected < top {
        top = selected;
    } else if selected >= top + rows {
        top = selected + 1 - rows;
    }
    top
}

/// 详情栏的导航键：`↑↓` 逐行，`PgUp` `PgDn` 翻一屏（留一行重叠），`Home` `End` 到最前或最后（渲染时收口）。
/// 返回是否处理；Tab 之类不归详情管，返回 false。
fn step_scroll(scroll: &mut usize, rows: usize, nav: Nav) -> bool {
    let page = rows.saturating_sub(1).max(1);
    *scroll = match nav {
        Nav::Up => scroll.saturating_sub(1),
        Nav::Down => scroll.saturating_add(1),
        Nav::PageUp => scroll.saturating_sub(page),
        Nav::PageDown => scroll.saturating_add(page),
        Nav::Home => 0,
        Nav::End => usize::MAX,
        Nav::Tab | Nav::BackTab => return false,
    };
    true
}

/// 当前页的详情栏（任务攻略、百科词条）接收导航键。没有打开的详情时返回 false。
pub(super) fn detail_nav(world: &mut WorldState, nav: Nav) -> bool {
    let (scroll, rows) = match world.menu {
        QUEST_MENU => match world.quest_guide.as_mut() {
            Some(guide) => (&mut guide.scroll, guide.view_rows),
            None => return false,
        },
        WIKI_MENU => match world.wiki.open.as_mut() {
            Some(open) => (&mut open.scroll, open.view_rows),
            None => return false,
        },
        _ => return false,
    };
    step_scroll(scroll, rows, nav)
}

impl App {
    /// Home（`end` 为假）滚到最前，End 滚到最后。消息页和背包页在这里滚；任务、百科的列表和详情
    /// 由页面选择处理（`select.rs`、`detail_nav`），这里不再管。
    pub(super) fn scroll_edge(&mut self, end: bool) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        match world.menu {
            MESSAGES_MENU if end => world.msg_view.end(),
            MESSAGES_MENU => world.msg_view.home(&self.log),
            BAG_MENU => {
                world.bag_scroll = if end { bag_max_top(world.bag_rows.max(1)) } else { FIRST_BAG_SLOT };
            }
            _ => {}
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{bag_max_top, follow_top};

    #[test]
    fn bag_bottom_keeps_the_last_page_full() {
        // 背包格 5–19 共 15 个，每页 5 行：最后一页顶部是 15（15–19）。
        assert_eq!(bag_max_top(5), 15);
        // 一页能放下全部：顶部就是第一格。
        assert_eq!(bag_max_top(20), 5);
    }

    #[test]
    fn follow_top_only_scrolls_when_the_cursor_leaves_the_window() {
        // 10 行可见，光标在窗口里不动。
        assert_eq!(follow_top(0, 4, 10, 30), 0);
        assert_eq!(follow_top(5, 9, 10, 30), 5);
        // 光标往下出了底：贴着底滚。
        assert_eq!(follow_top(0, 10, 10, 30), 1);
        assert_eq!(follow_top(5, 20, 10, 30), 11);
        // 光标往上出了顶：顶在顶部。
        assert_eq!(follow_top(11, 2, 10, 30), 2);
    }

    #[test]
    fn follow_top_keeps_the_last_page_full() {
        // 最后一页贴底，不留空白；窗口没有可见行时从顶部开始。
        assert_eq!(follow_top(99, 29, 10, 30), 20);
        assert_eq!(follow_top(7, 3, 0, 30), 0);
        assert_eq!(follow_top(3, 0, 10, 0), 0);
    }
}
