//! 消息页的滚动视图（`docs/ui.md`「消息页」）：以消息序号为锚点翻页，
//! 新消息到来或丢弃最早消息时视图不跳。纯逻辑，不含渲染。

use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

use super::messages::{Message, MessageLog};

/// 定宽类型标签的列宽（`Kind::label` 都是 6 列）。
pub const LABEL_WIDTH: usize = 6;

/// 消息页的滚动状态。`anchor` 是屏幕顶部那条消息的序号；`None` 表示跟随最新。
/// `seen` 是锚定那一刻的最新序号，用来算“有 N 条新消息”（只算锚定之后新到的）。
#[derive(Debug, Default, Clone, Copy)]
pub struct MessageView {
    pub anchor: Option<u64>,
    /// 锚点消息顶部被滚过去的行数（逐行滚动用，STAGE-116）：锚点消息换行后的前几行不显示。跟随最新和翻页时为 0。
    pub skip: usize,
    pub seen: u64,
    /// 上次渲染消息页时的可用行数与列数，翻页按键要用；渲染后更新。
    pub rows: usize,
    pub cols: usize,
}

impl MessageView {
    /// 离开消息页再回来时重新停在最新。
    pub fn reset(&mut self) {
        self.anchor = None;
        self.skip = 0;
    }

    pub fn end(&mut self) {
        self.anchor = None;
        self.skip = 0;
    }

    pub fn home(&mut self, log: &MessageLog) {
        if let Some(m) = log.first() {
            self.pin(m.seq, log);
        }
    }

    /// 锚定到某条消息的顶部；从“跟随最新”转过来时记下当前最新序号作为“已看到”的水位，
    /// 已在旧位置继续翻页时水位不变（那些新消息仍然没看到）。
    fn pin(&mut self, seq: u64, log: &MessageLog) {
        self.pin_at(seq, 0, log);
    }

    /// 锚定到某条消息的第 `skip` 行（逐行滚动用）。
    fn pin_at(&mut self, seq: u64, skip: usize, log: &MessageLog) {
        if self.anchor.is_none() {
            self.seen = log.latest_seq();
        }
        self.anchor = Some(seq);
        self.skip = skip;
    }
}

/// 把正文按显示宽度切成行（每行不超过 `width` 列，贪心，不断开任何字符）。
/// 空文本是一行空行；`width` 为 0 时原样一行，由调用方保证不发生。
pub fn wrap_text(text: &str, width: usize) -> Vec<&str> {
    if width == 0 || text.width() <= width {
        return vec![text];
    }
    let mut out = Vec::new();
    let mut start = 0;
    let mut w = 0;
    for (i, c) in text.char_indices() {
        let cw = c.width().unwrap_or(0);
        if w + cw > width && w > 0 {
            out.push(&text[start..i]);
            start = i;
            w = 0;
        }
        w += cw;
    }
    out.push(&text[start..]);
    out
}

/// 一条消息换行后的行数。
fn lines_of(m: &Message, cols: usize) -> usize {
    wrap_text(&m.text, cols.saturating_sub(LABEL_WIDTH)).len()
}

/// 屏幕顶部的位置：消息序号和它顶部滚过去的行数（逐行滚动，STAGE-116）。
/// 跟随最新时从最后一行往上数正好 `rows` 行，顶部那条可能只露出后几行，不留空行；锚定时是锚点加跳过的行数。
/// 整个日志还放不满一屏时顶部是第一条、跳过 0，剩下的空行由渲染补在上面。空日志返回 `None`。
pub fn top(log: &MessageLog, view: &MessageView) -> Option<(u64, usize)> {
    let first = log.first()?.seq;
    match view.anchor {
        Some(a) => Some((a.max(first), top_skip(log, view))),
        None => {
            let rows = view.rows.max(1);
            let mut filled = 0;
            for m in log.all().iter().rev() {
                filled += lines_of(m, view.cols);
                if filled >= rows {
                    return Some((m.seq, filled - rows));
                }
            }
            Some((first, 0))
        }
    }
}

/// 当前窗口（起止序号，含两端）：从屏幕顶部往下填满一屏。底部露出一部分的消息也算在内（它的其余行在屏幕外，列表截掉）。
pub fn window(log: &MessageLog, view: &MessageView) -> Option<(u64, u64)> {
    let (start, skip) = top(log, view)?;
    let rows = view.rows.max(1);
    let mut used = 0;
    let mut end = start;
    // 顶部消息滚过去的行不占屏幕，只算它剩下的行。
    let mut hidden = skip;
    for m in log.from_seq(start) {
        used += lines_of(m, view.cols).saturating_sub(std::mem::take(&mut hidden));
        end = m.seq;
        if used >= rows {
            break;
        }
    }
    Some((start, end))
}

/// 锚点消息顶部跳过的行数（逐行滚动，STAGE-116）。锚点已被丢弃（夹到现存最早一条）或跟随最新时为 0。
pub fn top_skip(log: &MessageLog, view: &MessageView) -> usize {
    match (view.anchor, log.first()) {
        (Some(a), Some(first)) if a >= first.seq => view.skip,
        _ => 0,
    }
}

/// 跟随最新时的屏幕顶部，翻到底的判断用。
fn follow_top(log: &MessageLog, view: &MessageView) -> Option<(u64, usize)> {
    top(log, &MessageView { anchor: None, ..*view })
}

/// 翻一页：页高减 1 行，保留一行重叠。`down` 为假是 `PgUp`；翻到底转回跟随。
pub fn page(log: &MessageLog, view: &mut MessageView, down: bool) {
    let Some(first) = log.first().map(|m| m.seq) else { return };
    let latest = log.latest_seq();
    let step = view.rows.saturating_sub(1);
    let cols = view.cols;
    if !down {
        let top = match view.anchor {
            Some(a) => a.max(first),
            None => match follow_top(log, view) {
                Some((t, _)) => t,
                None => return,
            },
        };
        let mut budget = step;
        let mut new_top = top;
        for m in log.all().iter().rev().skip_while(|m| m.seq >= top) {
            budget = budget.saturating_sub(lines_of(m, cols));
            new_top = m.seq;
            if budget == 0 {
                break;
            }
        }
        view.pin(new_top, log);
    } else {
        // 向下走 step 行的落点：正好在消息边界时取下一条，落在消息中间时取该条。
        let Some(a) = view.anchor else { return };
        let Some(follow) = follow_top(log, view) else { return };
        let top = a.max(first);
        let mut budget = step;
        let mut new_top = top;
        let mut it = log.from_seq(top).peekable();
        while let Some(m) = it.next() {
            let lines = lines_of(m, cols);
            if budget >= lines {
                budget -= lines;
                if budget == 0 {
                    new_top = it.peek().map_or(latest, |n| n.seq);
                    break;
                }
            } else {
                new_top = m.seq;
                break;
            }
        }
        if (new_top, 0) < follow {
            view.pin(new_top, log);
        } else {
            view.end(); // 翻到底转回跟随
        }
    }
}

/// 逐行滚动（输入行为空时 `↑` `↓`，STAGE-116，`docs/ui.md`「消息页」）：`down` 为假是 `↑`（往更早），为真是 `↓`。
/// 按换行后的行走，锚点消息的每一行都停得住。`↓` 走到跟随最新的位置就转回跟随；跟随最新时 `↓` 不动，`↑` 往上走一行。
pub fn line(log: &MessageLog, view: &mut MessageView, down: bool) {
    let Some(first) = log.first().map(|m| m.seq) else { return };
    let Some(follow) = follow_top(log, view) else { return };
    let cols = view.cols;
    // 屏幕顶部的位置：锚定时是锚点加跳过的行，跟随时是跟随的屏幕顶部。
    let (top, skip) = match view.anchor {
        Some(a) => (a.max(first), top_skip(log, view)),
        None => follow,
    };
    if !down {
        if skip > 0 {
            view.pin_at(top, skip - 1, log);
        } else if let Some(prev) = log.all().iter().rev().find(|m| m.seq < top) {
            // 上一条的最后一行落到屏幕顶部。
            view.pin_at(prev.seq, lines_of(prev, cols) - 1, log);
        }
        return;
    }
    if view.anchor.is_none() {
        return;
    }
    let Some(current) = log.from_seq(top).next() else { return };
    let next = if skip + 1 < lines_of(current, cols) {
        (top, skip + 1)
    } else {
        match log.from_seq(top).nth(1) {
            Some(next) => (next.seq, 0),
            None => return,
        }
    };
    // 滚到跟随最新的屏幕顶部或更靠后，就回到跟随。
    if next >= follow {
        view.end();
    } else {
        view.pin_at(next.0, next.1, log);
    }
}

/// 锚定状态下没看到的新消息条数（自锚定那一刻起）；跟随最新时为 0。
pub fn unseen_count(log: &MessageLog, view: &MessageView) -> u64 {
    match view.anchor {
        Some(_) => log.latest_seq().saturating_sub(view.seen),
        None => 0,
    }
}

#[cfg(test)]
mod tests;
