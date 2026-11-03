//! 消息页滚动视图的单元测试（`msgview.rs`）。

use super::*;
use crate::app::Kind;

/// 每条消息单行（短文本），指定窗口尺寸的视图。
fn view(rows: usize) -> MessageView {
    MessageView { anchor: None, skip: 0, seen: 0, rows, cols: 80 }
}

fn log_of(n: u64) -> MessageLog {
    let mut log = MessageLog::default();
    for i in 1..=n {
        log.system(format!("第{i}条"));
    }
    log
}

#[test]
fn wrap_by_display_width() {
    // 20 个汉字 40 列，每 10 列一行 → 4 行。
    let text = "一二三四五六七八九十壹贰叁肆伍陆柒捌玖";
    let lines = wrap_text(text, 10);
    assert_eq!(lines.len(), 4);
    assert!(lines.iter().all(|l| l.width() <= 10));
    assert_eq!(lines.join(""), text);
}

#[test]
fn wrap_never_splits_a_char() {
    // 宽度 5：汉字 2 列，第三个汉字放不下要整体挪到下一行。
    let lines = wrap_text("一二三", 5);
    assert_eq!(lines, vec!["一二", "三"]);
}

#[test]
fn wrap_empty_and_short() {
    assert_eq!(wrap_text("", 10), vec![""]);
    assert_eq!(wrap_text("短句", 10), vec!["短句"]);
}

#[test]
fn wrap_emoji_counts_double() {
    let lines = wrap_text("😀😀😀", 4);
    assert_eq!(lines, vec!["😀😀", "😀"]);
}

#[test]
fn window_follows_latest() {
    let log = log_of(20);
    assert_eq!(window(&log, &view(5)), Some((16, 20)));
    // 消息不足一屏时从头显示。
    let small = log_of(3);
    assert_eq!(window(&small, &view(5)), Some((1, 3)));
}

#[test]
fn window_accounts_for_wrapped_lines() {
    let mut log = MessageLog::default();
    for _ in 0..3 {
        log.system("一二三四五六七八九十"); // 20 列，cols=16 → 正文 10 列 → 2 行
    }
    // 5 行正好放 2 条半：最上面那条只露出最后一行，屏幕不留空行（STAGE-116）。
    let v = MessageView { rows: 5, cols: 16, ..view(5) };
    assert_eq!(window(&log, &v), Some((1, 3)));
    assert_eq!(top(&log, &v), Some((1, 1)), "顶部消息跳过第一行");
}

#[test]
fn anchored_window_stays_on_new_messages() {
    let mut log = log_of(10);
    let mut v = view(5);
    // 模拟从跟随状态锚定到 3：水位是当时的最新序号 10。
    v.anchor = Some(3);
    v.seen = 10;
    assert_eq!(window(&log, &v), Some((3, 7)));
    log.system("新来的");
    assert_eq!(window(&log, &v), Some((3, 7)), "锚定时新消息不顶走视图");
    assert_eq!(unseen_count(&log, &v), 1, "只算锚定之后新到的");
    v.end();
    assert_eq!(unseen_count(&log, &v), 0, "回到最新后提示清零");
}

#[test]
fn paging_while_anchored_keeps_seen_level() {
    // 锚定后继续翻页（还没回到最新），新消息提示不清零。
    let mut log = log_of(10);
    let mut v = view(5);
    page(&log, &mut v, false); // 锚定到 2，水位 10
    assert_eq!(v.anchor, Some(2));
    assert_eq!(unseen_count(&log, &v), 0);
    log.system("新1");
    log.system("新2");
    page(&log, &mut v, false); // 继续往上翻
    assert_eq!(unseen_count(&log, &v), 2, "继续翻页不重置水位");
}

#[test]
fn anchor_clamps_to_first_after_eviction() {
    let mut log = MessageLog::default();
    for i in 1..=1100 {
        log.system(format!("第{i}条"));
    }
    let mut v = view(5);
    v.anchor = Some(1); // 已被丢弃
    assert_eq!(window(&log, &v), Some((101, 105)), "视图停在现存的最早一条");
    assert_eq!(log.first().unwrap().seq, 101);
}

#[test]
fn page_up_overlaps_one_line() {
    let log = log_of(20);
    let mut v = view(5);
    page(&log, &mut v, false);
    assert_eq!(v.anchor, Some(12), "从 16 往上翻 4 行");
    page(&log, &mut v, false);
    assert_eq!(v.anchor, Some(8));
    // 连翻到顶不越界。
    for _ in 0..10 {
        page(&log, &mut v, false);
    }
    assert_eq!(v.anchor, Some(1));
}

#[test]
fn page_down_returns_to_follow_at_bottom() {
    let log = log_of(20);
    let mut v = view(5);
    v.anchor = Some(2);
    page(&log, &mut v, true);
    assert_eq!(v.anchor, Some(6));
    page(&log, &mut v, true); // 6 → 10
    page(&log, &mut v, true); // 10 → 14
    page(&log, &mut v, true); // 14 → 18
    page(&log, &mut v, true); // 18 会越过跟随起点 16 → 回到跟随
    assert_eq!(v.anchor, None);
    assert_eq!(window(&log, &v), Some((16, 20)));
    // 已在最新时继续翻没有变化。
    page(&log, &mut v, true);
    assert_eq!(v.anchor, None);
}

#[test]
fn home_and_end() {
    let log = log_of(20);
    let mut v = view(5);
    v.home(&log);
    assert_eq!(v.anchor, Some(1));
    assert_eq!(window(&log, &v), Some((1, 5)));
    v.end();
    assert_eq!(v.anchor, None);
    assert_eq!(unseen_count(&log, &v), 0);
}

#[test]
fn empty_log_is_noop() {
    let log = MessageLog::default();
    let mut v = view(5);
    page(&log, &mut v, false);
    page(&log, &mut v, true);
    v.home(&log);
    assert_eq!(v.anchor, None);
    assert_eq!(window(&log, &v), None);
    assert_eq!(unseen_count(&log, &v), 0);
}

#[test]
fn eviction_does_not_move_anchored_view() {
    // 丢弃最早的消息时，锚点还在现存范围内的话视图不动（STAGE-27 验收 5）。
    let mut log = MessageLog::default();
    for i in 1..=1000 {
        log.system(format!("第{i}条"));
    }
    let mut v = view(5);
    v.anchor = Some(995);
    v.seen = 1000; // 锚定时的水位
    assert_eq!(window(&log, &v), Some((995, 999)));
    for i in 1001..=1010 {
        log.system(format!("第{i}条"));
    }
    // 现存 11–1010（丢了最早的 10 条），锚点 995 还在，窗口不动；新消息只进提示。
    assert_eq!(log.first().unwrap().seq, 11);
    assert_eq!(window(&log, &v), Some((995, 999)));
    assert_eq!(unseen_count(&log, &v), 10);
}

#[test]
fn label_is_six_columns_for_all_kinds() {
    for k in [Kind::Command, Kind::System, Kind::Error, Kind::Agent] {
        assert_eq!(k.label().width(), LABEL_WIDTH);
    }
}

/// 第 1 条换成 3 行（正文 10 列，12 个汉字），第 2、3 条各一行。
fn log_with_wrapped() -> MessageLog {
    let mut log = MessageLog::default();
    log.system("一二三四五六七八九十壹贰叁");
    log.system("短");
    log.system("尾");
    log
}

#[test]
fn line_scroll_walks_each_wrapped_line() {
    let log = log_with_wrapped();
    let (long, short) = (1, 2);
    let mut v = MessageView { anchor: None, skip: 0, seen: 0, rows: 2, cols: 16 };
    // 跟随最新时屏幕顶部是“短”，↑ 先落到“长”的最后一行。
    line(&log, &mut v, false);
    assert_eq!((v.anchor, v.skip), (Some(long), 2));
    assert_eq!(window(&log, &v), Some((long, short)), "跳过的两行不占屏幕");
    line(&log, &mut v, false);
    assert_eq!((v.anchor, v.skip), (Some(long), 1));
    line(&log, &mut v, false);
    assert_eq!((v.anchor, v.skip), (Some(long), 0));
    line(&log, &mut v, false);
    assert_eq!((v.anchor, v.skip), (Some(long), 0), "已经是最早的一行，不动");
    // ↓ 逐行走回来，走到跟随的顶部（“短”）就回到跟随。
    line(&log, &mut v, true);
    assert_eq!((v.anchor, v.skip), (Some(long), 1));
    line(&log, &mut v, true);
    assert_eq!((v.anchor, v.skip), (Some(long), 2));
    line(&log, &mut v, true);
    assert_eq!(v.anchor, None, "走到跟随的顶部就回到跟随");
}

#[test]
fn line_down_while_following_does_nothing() {
    let log = log_with_wrapped();
    let mut v = MessageView { anchor: None, skip: 0, seen: 0, rows: 2, cols: 16 };
    line(&log, &mut v, true);
    assert_eq!((v.anchor, v.skip), (None, 0));
}

#[test]
fn skip_is_dropped_when_the_anchor_is_evicted() {
    // 丢掉最早的消息后，锚点落到已经没有的位置：从现存最早一条的第一行开始，跳过的行数不再算。
    let mut log = MessageLog::default();
    for i in 1..=1010 {
        log.system(format!("第{i}条"));
    }
    let v = MessageView { anchor: Some(5), skip: 2, ..view(3) };
    assert_eq!(top_skip(&log, &v), 0);
    assert_eq!(window(&log, &v).map(|w| w.0), Some(11));
}

#[test]
fn anchored_window_shows_the_half_message_at_the_bottom() {
    // 锚定在“短”，底部是两行的长消息只剩一行空间：长消息算进窗口（列表截掉它的第二行），不留空行（STAGE-116）。
    let mut log = MessageLog::default();
    log.system("短");
    log.system("一二三四五六七八九十壹贰叁");
    log.system("尾");
    let v = MessageView { anchor: Some(1), rows: 2, cols: 16, ..view(2) };
    assert_eq!(window(&log, &v), Some((1, 2)));
}
