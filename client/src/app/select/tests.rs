use super::super::cursor::PAGE_STEP;
use super::*;

/// 测试用的默认步长（`PAGE_STEP`，不是列表页的一屏）。
fn navigate(columns: &[Column], focus: &mut PageFocus, nav: Nav) -> bool {
    navigate_page(columns, focus, nav, PAGE_STEP)
}

fn item(key: i64, usable: bool) -> Item {
    Item {
        key,
        usable,
        enter: None,
    }
}

/// 两栏：物品栏 1、2（空）、3；装备栏 10。
fn columns() -> Vec<Column> {
    vec![
        Column {
            name: "物品",
            items: vec![item(1, true), item(2, false), item(3, true)],
            shown: true,
            detail: false,
            default_key: None,
        },
        Column {
            name: "装备",
            items: vec![item(10, true)],
            shown: true,
            detail: false,
            default_key: None,
        },
    ]
}

fn at(columns: &[Column], focus: &PageFocus) -> (usize, i64) {
    let (c, i) = locate(columns, focus).expect("有能选的项");
    (c, columns[c].items[i].key)
}

#[test]
fn default_is_the_first_usable_row_of_the_first_column() {
    let cols = columns();
    assert_eq!(at(&cols, &PageFocus::default()), (0, 1));
    assert!(selectable(&cols));
    assert!(!selectable(&[]), "没有声明栏的页不接手 ↑↓");
}

#[test]
fn arrows_skip_unusable_rows_and_stop_at_the_ends() {
    let cols = columns();
    let mut f = PageFocus::default();
    assert!(navigate(&cols, &mut f, Nav::Down));
    assert_eq!(at(&cols, &f), (0, 3), "空行跳过");
    navigate(&cols, &mut f, Nav::Down);
    assert_eq!(at(&cols, &f), (0, 3), "到头不循环");
    navigate(&cols, &mut f, Nav::Up);
    assert_eq!(at(&cols, &f), (0, 1));
    navigate(&cols, &mut f, Nav::Up);
    assert_eq!(at(&cols, &f), (0, 1));
}

#[test]
fn home_end_and_paging() {
    let cols = columns();
    let mut f = PageFocus::default();
    navigate(&cols, &mut f, Nav::End);
    assert_eq!(at(&cols, &f), (0, 3));
    navigate(&cols, &mut f, Nav::Home);
    assert_eq!(at(&cols, &f), (0, 1));
    navigate(&cols, &mut f, Nav::PageDown);
    assert_eq!(at(&cols, &f), (0, 3), "不足一页时停在最后一项");
    navigate(&cols, &mut f, Nav::PageUp);
    assert_eq!(at(&cols, &f), (0, 1));
}

#[test]
fn tab_cycles_columns_and_each_column_remembers_its_row() {
    let cols = columns();
    let mut f = PageFocus::default();
    navigate(&cols, &mut f, Nav::Down);
    assert!(navigate(&cols, &mut f, Nav::Tab));
    assert_eq!(at(&cols, &f), (1, 10), "装备栏只有一项");
    assert!(navigate(&cols, &mut f, Nav::Tab));
    assert_eq!(at(&cols, &f), (0, 3), "回到物品栏，还停在上次的那一项");
    assert!(navigate(&cols, &mut f, Nav::BackTab));
    assert_eq!(at(&cols, &f), (1, 10), "Shift+Tab 反向");
}

#[test]
fn a_page_with_one_column_ignores_tab() {
    let cols = vec![columns().remove(0)];
    let mut f = PageFocus::default();
    assert!(!navigate(&cols, &mut f, Nav::Tab));
}

#[test]
fn selection_follows_the_number_when_the_list_changes() {
    let mut cols = columns();
    cols[0].items.push(item(4, true)); // 物品栏：1、2（空）、3、4
    let mut f = PageFocus::default();
    navigate(&cols, &mut f, Nav::Down);
    assert_eq!(at(&cols, &f), (0, 3));
    // 前面的一项用掉了：编号 3 还在，选中项仍是它。
    cols[0].items.remove(0);
    assert_eq!(at(&cols, &f), (0, 3));
    // 编号 3 没了：落到原来行号（2）附近的能选的一项，就是它后面的 4。
    cols[0].items.retain(|it| it.key != 3);
    assert_eq!(at(&cols, &f), (0, 4));
}

#[test]
fn a_page_without_usable_rows_takes_no_navigation() {
    let cols = vec![Column {
        name: "空",
        items: vec![item(1, false)],
        shown: true,
        detail: false,
        default_key: None,
    }];
    let mut f = PageFocus::default();
    assert!(!navigate(&cols, &mut f, Nav::Down), "↑↓ 要交给历史");
    assert!(locate(&cols, &f).is_none());
    assert!(!navigate(&[], &mut f, Nav::Down));
}

#[test]
fn enter_and_hint_follow_the_selected_item() {
    let mut cols = columns();
    cols[0].items[0].enter = Some(OnEnter::Fill("item use 1".into()));
    cols[0].items[2].enter = Some(OnEnter::Run("quest 3".into()));
    let mut f = PageFocus::default();
    assert_eq!(enter(&cols, &f), Some(OnEnter::Fill("item use 1".into())));
    assert_eq!(hint(&cols, &f), ["↑↓ 选择", "Enter 填 item", "Tab 装备"]);
    navigate(&cols, &mut f, Nav::Down);
    assert_eq!(enter(&cols, &f), Some(OnEnter::Run("quest 3".into())));
    assert_eq!(hint(&cols, &f), ["↑↓ 选择", "Enter 打开", "Tab 装备"]);
    navigate(&cols, &mut f, Nav::Tab);
    assert_eq!(enter(&cols, &f), None, "没有命令的项 Enter 不起作用，提示也不写");
    assert_eq!(hint(&cols, &f), ["↑↓ 选择", "Tab 物品"]);
}

#[test]
fn hidden_column_keeps_its_row_but_takes_no_focus() {
    let mut cols = columns();
    let mut f = PageFocus::default();
    navigate(&cols, &mut f, Nav::Down);
    cols[0].shown = false;
    assert_eq!(at(&cols, &f), (1, 10), "没画出来的栏不接收焦点");
    assert!(!navigate(&cols, &mut f, Nav::Tab), "只剩装备栏，Tab 不起作用");
    cols[0].shown = true;
    assert_eq!(at(&cols, &f), (0, 3), "列表画出来后焦点回到它：焦点栏记着，只是暂时落到别的栏");
    assert_eq!(selected_index(&cols, &f, 0), Some(2), "隐藏期间记着的行还在（编号 3 在第 2 行）");
}

#[test]
fn detail_column_is_routed_by_the_page_and_is_skipped_when_closed() {
    let mut cols = columns();
    cols.push(Column {
        name: "详情",
        items: vec![item(0, true)],
        shown: false,
        detail: true,
        default_key: None,
    });
    let mut f = PageFocus::default();
    navigate(&cols, &mut f, Nav::Tab);
    assert_eq!(at(&cols, &f), (1, 10), "关着的详情栏不在 Tab 之中");
    assert!(!on_detail(&cols, &f));
    cols[2].shown = true;
    assert!(navigate(&cols, &mut f, Nav::Tab));
    assert!(on_detail(&cols, &f), "打开后 Tab 能到详情栏");
}

#[test]
fn point_and_focus_by_name() {
    let cols = columns();
    let mut f = PageFocus::default();
    point_named(&cols, &mut f, "物品", 3);
    assert_eq!(at(&cols, &f), (0, 3), "命令看某一项后选中项跟过去");
    assert_eq!(f.column, 0, "定位不改焦点栏");
    point_named(&cols, &mut f, "物品", 2);
    assert_eq!(at(&cols, &f), (0, 3), "空格子不能定位");
    assert!(focus_named(&cols, &mut f, "装备"));
    assert_eq!(at(&cols, &f), (1, 10));
    assert!(!focus_named(&cols, &mut f, "没有这一栏"));
    reset_named(&cols, &mut f, "物品");
    focus_named(&cols, &mut f, "物品");
    assert_eq!(at(&cols, &f), (0, 1), "清掉后回到第一项");
}

#[test]
fn cursor_of_reports_selected_row_and_focus() {
    let cols = columns();
    let mut f = PageFocus::default();
    navigate(&cols, &mut f, Nav::Down);
    assert_eq!(cursor_of(&cols, &f, "物品"), Some((2, true)), "编号 3 在第 2 行");
    assert_eq!(cursor_of(&cols, &f, "装备"), Some((0, false)), "非焦点栏的选中行也要标出来");
    assert_eq!(cursor_of(&cols, &f, "没有"), None);
}

#[test]
fn tab_goes_to_an_empty_drawn_column_and_nothing_moves_there() {
    // 装备栏画着但一件都没有：Tab 也能停上去，那里没有选中项，↑↓ 不翻历史也不动。
    let mut cols = columns();
    cols[1].items[0].usable = false;
    let mut f = PageFocus::default();
    assert!(navigate(&cols, &mut f, Nav::Tab), "画得出来的空栏也能停");
    assert_eq!(focus_column(&cols, &f), 1);
    assert!(locate(&cols, &f).is_none(), "空栏里没有选中项");
    assert!(navigate(&cols, &mut f, Nav::Down), "焦点在空栏时 ↑↓ 不翻历史，也不动");
    assert_eq!(focus_column(&cols, &f), 1);
    assert_eq!(hint(&cols, &f), ["Tab 物品"], "提示行只写能去的地方");
    assert!(navigate(&cols, &mut f, Nav::Tab));
    assert_eq!(at(&cols, &f), (0, 1), "Tab 回来，物品栏还停在原位");
}

#[test]
fn every_drawn_column_keeps_its_own_selected_row() {
    let cols = columns();
    let mut f = PageFocus::default();
    assert_eq!(selections(&cols, &f), vec![Some(1), Some(10)], "不是焦点栏也有选中项");
    navigate(&cols, &mut f, Nav::Down);
    navigate(&cols, &mut f, Nav::Tab);
    assert_eq!(marks(&cols, &f), (1, vec![Some(3), Some(10)]), "焦点在装备栏，物品栏的选中项还在");
    let mut hidden = cols.clone();
    hidden[0].shown = false;
    assert_eq!(selections(&hidden, &f), vec![None, Some(10)], "没画出来的栏没有选中行");
}
