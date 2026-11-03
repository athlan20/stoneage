//! 页面选择（STAGE-113，`docs/interaction.md`「页面选择」）：没有模态层、输入行为空时，`↑↓` `PgUp` `PgDn` `Home` `End`
//! 移动页面里的选中项，`Tab` 在栏之间切换，`Enter` 执行或填入选中项的命令。
//! 每页在 `page_columns` 里声明自己的栏（宠物、背包在 `pets.rs`、`bag_select.rs`，「任务」「百科」在 `quests.rs`、`wiki.rs`）；
//! 选中项按编号记，列表变化后仍落在同一项上。每栏都有自己的选中项（焦点栏的那一行反色，其余栏只加粗带 `▶`）；
//! `Tab` 走到画得出来的栏，空栏也能停（那里没有选中项）。这里只管状态和规则，按键的接入在 `page_keys.rs`，提示行在 `hint.rs`。

use super::bag_select;
use super::char_select;
use super::cursor;
use super::pets;
use super::quests;
use super::wiki;
use super::world::{BAG_MENU, CHAR_MENU, PETS_MENU, QUEST_MENU, SOCIAL_MENU, WIKI_MENU, WorldState};

/// 栏里的一项。`key` 是稳定的编号（槽位、任务编号、NPC 编号……），列表变化后按它找回选中项。
#[derive(Clone, Debug, PartialEq)]
pub struct Item {
    pub key: i64,
    /// 能不能选：空栏位、空格子为 false，移动时跳过（和战斗列表面板一样）。
    pub usable: bool,
    /// Enter 做什么；没有就是 Enter 不起作用。
    pub enter: Option<OnEnter>,
}

/// Enter 的两种结果。查看类执行（不回显，也不进历史，等价敲了这条查看命令）；
/// 操作类只把命令填进输入行、光标放在末尾，玩家再按一次 Enter 才执行。
#[derive(Clone, Debug, PartialEq)]
pub enum OnEnter {
    Run(String),
    Fill(String),
    /// 打开对象动作菜单（STAGE-132 的背包）。
    Open,
}

/// 一栏。`name` 是 Tab 提示里写的栏名（如“物品”）；栏的顺序就是 Tab 的顺序，第一栏是默认焦点栏。
/// `shown` 为假表示这一栏现在没画出来（如窄屏时被攻略盖住的列表）：不接收 Tab 和方向键，选中位置照样记着。
/// `detail` 为真是详情栏：`↑↓` 等键归页面滚动，不移动选中项（`scroll.rs` 的 `detail_nav`）。
/// `default_key` 是还没有记过选中项时选中的那一项（宠物页是出战的那只），None 时选能选的第一项。
#[derive(Clone, Debug, PartialEq)]
pub struct Column {
    pub name: &'static str,
    pub items: Vec<Item>,
    pub shown: bool,
    pub detail: bool,
    pub default_key: Option<i64>,
}

/// 一栏记住的选中位置：按编号记（`key`），`index` 是上次的行号，编号没了时落到它附近。
#[derive(Clone, Debug, Default)]
pub struct Slot {
    pub key: Option<i64>,
    pub index: usize,
}

/// 一页的选择状态：焦点栏，以及每栏各自的选中项。切走再切回来仍在；离开世界时随 `WorldState` 一起丢弃。
/// `pinned` 表示焦点栏是玩家用 `Tab`、`↑↓` 或命令选出来的（空栏也算）；没有选过时焦点落在第一个有能选项的栏上。
#[derive(Clone, Debug, Default)]
pub struct PageFocus {
    pub column: usize,
    pub slots: Vec<Slot>,
    pub pinned: bool,
}

/// 页面的导航键。`↑↓` 移动一项；`PgUp` `PgDn` 跨一页；`Home` `End` 到第一项、最后一项；`Tab` `Shift+Tab` 换栏。
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Nav {
    Up,
    Down,
    PageUp,
    PageDown,
    Home,
    End,
    Tab,
    BackTab,
}

/// 这一页的栏。「地图」「角色」「宠物」「背包」「任务」「百科」按 `world.menu` 在各自模块里声明；「消息」页没有栏，它的 `↑↓` 逐行滚动（`page_keys.rs`）。
pub fn page_columns(world: &WorldState) -> Vec<Column> {
    match world.menu {
        CHAR_MENU => char_select::char_columns(world),
        PETS_MENU => pets::pet_columns(world),
        BAG_MENU => bag_select::bag_columns(world),
        QUEST_MENU => quests::columns(world),
        SOCIAL_MENU => super::party::party_columns(world),
        WIKI_MENU => wiki::columns(world),
        _ => Vec::new(),
    }
}

/// 能不能接收焦点：画得出来，并且有能选的项。
fn has_usable(column: &Column) -> bool {
    column.shown && column.items.iter().any(|item| item.usable)
}

/// 这一页有没有能选的项。有就由页面接手 `↑↓`；没有（包括没有声明栏）时 `↑↓` 照旧翻历史。
pub fn selectable(columns: &[Column]) -> bool {
    columns.iter().any(has_usable)
}

/// 焦点栏的编号。玩家选过（`pinned`）时就是记下的那一栏，空栏也是（焦点在空栏上时没有选中行）；
/// 没选过时是第一个有能选项的栏，都没有时是第一个画出来的栏。
pub fn focus_column(columns: &[Column], focus: &PageFocus) -> usize {
    let n = columns.len();
    if n == 0 {
        return 0;
    }
    let start = focus.column.min(n - 1);
    if focus.pinned && columns[start].shown {
        return start;
    }
    let order = || (0..n).map(|k| (start + k) % n);
    order().find(|&c| has_usable(&columns[c])).or_else(|| order().find(|&c| columns[c].shown)).unwrap_or(start)
}

/// 焦点栏和它的选中行（栏号、行号）；焦点栏是空栏、没画出来或没有能选的项时是 None。
pub fn locate(columns: &[Column], focus: &PageFocus) -> Option<(usize, usize)> {
    if columns.is_empty() {
        return None;
    }
    let column = focus_column(columns, focus);
    if !columns[column].shown {
        return None;
    }
    Some((column, selected_index(columns, focus, column)?))
}

/// 一栏的选中行（不管它是不是焦点栏）。选中项先按编号找回；编号已经不在列表里（被用掉、被放生）时，
/// 落到原来行号附近的能选的一项。没有能选的项时是 None。
pub fn selected_index(columns: &[Column], focus: &PageFocus, column: usize) -> Option<usize> {
    let items = &columns.get(column)?.items;
    let slot = focus.slots.get(column).cloned().unwrap_or_default();
    let position = |key: i64| items.iter().position(|item| item.key == key);
    let wanted = match slot.key {
        Some(key) => position(key).unwrap_or(slot.index),
        None => columns[column].default_key.and_then(position).unwrap_or(slot.index),
    };
    cursor::nearest(items.len(), |i| items[i].usable, wanted)
}

/// 每栏的选中项编号（第 i 个是第 i 栏）：没画出来或没有能选的项时为 None。界面给每栏的选中行画 `▶`。
pub fn selections(columns: &[Column], focus: &PageFocus) -> Vec<Option<i64>> {
    (0..columns.len())
        .map(|column| {
            if !columns[column].shown {
                return None;
            }
            selected_index(columns, focus, column).map(|index| columns[column].items[index].key)
        })
        .collect()
}

/// 焦点栏的编号和每栏的选中项编号，界面画选中行用（焦点栏反色，其余栏只加粗）。
pub fn marks(columns: &[Column], focus: &PageFocus) -> (usize, Vec<Option<i64>>) {
    (focus_column(columns, focus), selections(columns, focus))
}

/// 渲染用：名字为 `name` 的栏的选中行，以及这一栏是不是焦点栏。这一栏没有能选的项时是 None。
pub fn cursor_of(columns: &[Column], focus: &PageFocus, name: &str) -> Option<(usize, bool)> {
    let column = columns.iter().position(|c| c.name == name)?;
    let index = selected_index(columns, focus, column)?;
    Some((index, focus_column(columns, focus) == column))
}

/// 页面渲染用：当前页名字为 `name` 的栏的选中行和焦点状态（见 `cursor_of`）。
pub fn page_cursor(world: &WorldState, name: &str) -> Option<(usize, bool)> {
    cursor_of(&page_columns(world), &world.select[world.menu], name)
}

/// 焦点在详情栏上：这时 `↑↓` 等键归页面滚动。
pub fn on_detail(columns: &[Column], focus: &PageFocus) -> bool {
    locate(columns, focus).is_some_and(|(c, _)| columns[c].detail)
}

fn named(columns: &[Column], name: &str) -> Option<usize> {
    columns.iter().position(|c| c.name == name)
}

/// 把名字为 `name` 的栏的选中项定到编号 `key` 上。命令“看某一项”之后用（`docs/interaction.md`
/// 「命令和选中项是同一个东西」）。没有这一栏，或这一栏里没有这一项时不动；焦点栏不变。
pub fn point_named(columns: &[Column], focus: &mut PageFocus, name: &str, key: i64) {
    let Some(column) = named(columns, name) else { return };
    let Some(index) = columns[column].items.iter().position(|item| item.key == key) else { return };
    set_slot(columns, focus, column, index);
}

/// 把名字为 `name` 的栏的选中项清掉，回到这一栏的第一项（列表换了内容时用）。焦点栏不变。
pub fn reset_named(columns: &[Column], focus: &mut PageFocus, name: &str) {
    let Some(column) = named(columns, name) else { return };
    if focus.slots.len() < columns.len() {
        focus.slots.resize_with(columns.len(), Slot::default);
    }
    focus.slots[column] = Slot::default();
}

/// 焦点挪到名字为 `name` 的栏。这一栏画得出来、有能选的项才挪，返回是否挪了。
pub fn focus_named(columns: &[Column], focus: &mut PageFocus, name: &str) -> bool {
    let Some(column) = named(columns, name) else { return false };
    if !has_usable(&columns[column]) {
        return false;
    }
    focus.column = column;
    focus.pinned = true;
    true
}

/// 选中项：焦点栏里的那一项。
pub fn selected<'a>(columns: &'a [Column], focus: &PageFocus) -> Option<&'a Item> {
    let (column, index) = locate(columns, focus)?;
    columns[column].items.get(index)
}

/// 把选中项移到编号为 `key` 的能选项上（`pet status N` 这类“看某一项”的命令用，见 `docs/interaction.md`「命令和选中项是同一个东西」）。
/// 没有这一项、或者它不能选时返回 false，选中项不动。
pub fn select_key(columns: &[Column], focus: &mut PageFocus, key: i64) -> bool {
    let found = columns.iter().enumerate().find_map(|(column, col)| {
        col.items.iter().position(|item| item.key == key && item.usable).map(|index| (column, index))
    });
    match found {
        Some((column, index)) => {
            remember(columns, focus, column, index);
            true
        }
        None => false,
    }
}

/// Enter 要做的事：选中项有命令才有。
pub fn enter(columns: &[Column], focus: &PageFocus) -> Option<OnEnter> {
    selected(columns, focus)?.enter.clone()
}

/// 处理一次导航键，返回是否已经处理。这一页没有能选的项（或者没有声明栏）时返回 false，
/// 调用方照常处理这个键（`↑↓` 翻历史）；焦点在空栏上时 `↑↓` 等键算已处理，但没有东西可动。
/// `PgUp` `PgDn` 跨 `page` 项：列表页传它的可见行数，这样一次翻的是一屏（`docs/interaction.md`「按键表」）。
pub fn navigate_page(columns: &[Column], focus: &mut PageFocus, nav: Nav, page: usize) -> bool {
    if matches!(nav, Nav::Tab | Nav::BackTab) {
        return switch_column(columns, focus, nav == Nav::Tab);
    }
    if !selectable(columns) {
        return false;
    }
    let Some((column, index)) = locate(columns, focus) else {
        return true;
    };
    let items = &columns[column].items;
    let usable = |i: usize| items[i].usable;
    let len = items.len();
    let page = page.max(1);
    let to = match nav {
        Nav::Up => cursor::step(len, usable, index, 1, false),
        Nav::Down => cursor::step(len, usable, index, 1, true),
        Nav::PageUp => cursor::step(len, usable, index, page, false),
        Nav::PageDown => cursor::step(len, usable, index, page, true),
        Nav::Home => cursor::nearest(len, usable, 0).unwrap_or(index),
        Nav::End => (0..len).rev().find(|&i| usable(i)).unwrap_or(index),
        Nav::Tab | Nav::BackTab => unreachable!("换栏已在上面处理"),
    };
    remember(columns, focus, column, to);
    true
}

/// 下一个（`forward`）或上一个画得出来的栏，循环；空栏也算（`docs/interaction.md`「页面选择」）。只剩焦点栏自己时是 None。
fn next_column(columns: &[Column], focus: &PageFocus, forward: bool) -> Option<usize> {
    let current = focus_column(columns, focus);
    let n = columns.len();
    (1..n)
        .map(|k| {
            if forward {
                (current + k) % n
            } else {
                (current + n - k) % n
            }
        })
        .find(|&c| columns[c].shown)
}

fn switch_column(columns: &[Column], focus: &mut PageFocus, forward: bool) -> bool {
    let Some(next) = next_column(columns, focus, forward) else {
        return false;
    };
    focus.column = next;
    focus.pinned = true;
    true
}

/// 记下选中的那一项（焦点栏和该栏的编号、行号）。
fn remember(columns: &[Column], focus: &mut PageFocus, column: usize, index: usize) {
    focus.column = column;
    focus.pinned = true;
    set_slot(columns, focus, column, index);
}

fn set_slot(columns: &[Column], focus: &mut PageFocus, column: usize, index: usize) {
    if focus.slots.len() < columns.len() {
        focus.slots.resize_with(columns.len(), Slot::default);
    }
    focus.slots[column] = Slot {
        key: Some(columns[column].items[index].key),
        index,
    };
}

/// 提示行的键（`docs/interaction.md`「提示行」）：有选中项时写 `↑↓ 选择`，选中项有 Enter 时写 `Enter 打开` 或 `Enter 填 命令名`；
/// 还有下一栏时写 `Tab 栏名`。焦点在空栏上时没有选中项，只写 `Tab`。
pub fn hint(columns: &[Column], focus: &PageFocus) -> Vec<String> {
    let mut keys = Vec::new();
    if locate(columns, focus).is_some() {
        keys.push("↑↓ 选择".to_string());
    }
    match enter(columns, focus) {
        Some(OnEnter::Run(_)) => keys.push("Enter 打开".into()),
        Some(OnEnter::Fill(cmd)) => keys.push(format!("Enter 填 {}", cmd.split_whitespace().next().unwrap_or(""))),
        Some(OnEnter::Open) => keys.push("Enter 动作".into()),
        None => {}
    }
    if let Some(next) = next_column(columns, focus, true) {
        keys.push(format!("Tab {}", columns[next].name));
    }
    keys
}

#[cfg(test)]
mod tests;
