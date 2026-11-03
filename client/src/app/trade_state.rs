//! STAGE-295：交易窗口的本地状态。保存服务端交易对象的最新副本、焦点、对方改报价的标记、修改报价的子选择；
//! 纯逻辑，不碰网络。报价行的序号固定为：0、1 物品格，2 石币，3 宠物（`docs/trade.md` 5.2）。

use crate::proto::{Trade, TradeSide};

/// 报价行数（两个物品格、石币、宠物）。
pub const OFFER_ROWS: usize = 4;

/// 窗口里的焦点：报价行、底部按钮（锁定或取消锁定、确定、取消交易）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Focus {
    Row(usize),
    Lock,
    Confirm,
    Cancel,
}

/// 修改报价的子选择：某一行的物品、宠物列表，或石币输入。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Picker {
    pub row: usize,
    pub cursor: usize,
    /// 石币行的输入（只在石币行用）。
    pub gold: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TradeWindow {
    /// 服务端最新的交易对象（`me` / `peer` 已按本端视角）。
    pub trade: Trade,
    /// 有请求在途（锁定、确定、改报价等），按钮显示处理中，不重复发送。
    pub busy: bool,
    /// 对方改了报价：状态行提示，直到自己下一次按键清掉。
    pub peer_changed: bool,
    /// 对方报价里变了的行（按 `OFFER_ROWS` 的序号），前面加 `●`。
    pub changed_rows: Vec<usize>,
    pub focus: Focus,
    pub picker: Option<Picker>,
    /// Esc 后的“取消这笔交易？”确认：`cancel_selected` 为真时焦点在「取消交易」，默认「继续交易」。
    pub ask_cancel: bool,
    pub cancel_selected: bool,
    /// 收到这份交易对象的时刻：剩余时间按它和 `idle_expires_in` 算。
    pub seen_at: std::time::Instant,
}

impl TradeWindow {
    pub fn new(trade: Trade) -> Self {
        Self {
            trade,
            busy: false,
            peer_changed: false,
            changed_rows: Vec::new(),
            focus: Focus::Cancel,
            picker: None,
            ask_cancel: false,
            cancel_selected: false,
            seen_at: std::time::Instant::now(),
        }
    }

    /// 收到新的交易对象（推送或响应）。rev 比本地小的忽略，返回 None；否则返回对方报价是否变了（锁定变化不算）。
    /// 对方改报价或任一方的锁定、确定变化时，焦点回到「取消交易」，防止误按确定。
    pub fn apply(&mut self, next: Trade) -> Option<bool> {
        if next.rev < self.trade.rev {
            return None;
        }
        let rows = peer_changed_rows(&self.trade.peer, &next.peer);
        let locks_moved = lock_states(&self.trade) != lock_states(&next);
        let offer_moved = !rows.is_empty();
        if offer_moved {
            self.peer_changed = true;
            self.changed_rows = rows;
        }
        if locks_moved || offer_moved {
            self.focus = Focus::Cancel;
            self.picker = None;
        }
        self.trade = next;
        self.seen_at = std::time::Instant::now();
        Some(offer_moved)
    }

    /// 自己有了下一次按键：清掉对方改报价的提示和标记。
    pub fn acknowledge(&mut self) {
        self.peer_changed = false;
        self.changed_rows.clear();
    }

    /// 报价全空且没人锁定：Esc 直接取消，不问。
    pub fn offer_is_empty(&self) -> bool {
        let me = &self.trade.me;
        me.items.is_empty() && me.gold == 0 && me.pet.is_none()
    }

    /// 距离无操作自动取消还剩几秒（按收到时的快照减去已过去的时间）。
    pub fn idle_left(&self, now: std::time::Instant) -> u64 {
        let spent = now.saturating_duration_since(self.seen_at).as_secs();
        self.trade.idle_expires_in.saturating_sub(spent)
    }

    pub fn focus_is_cancel(&self) -> bool {
        self.focus == Focus::Cancel
    }

    pub fn nobody_locked(&self) -> bool {
        !self.trade.me.locked && !self.trade.peer.locked
    }

    /// 底部按钮和报价行组成的焦点循环（顺序即 ↑↓ 的顺序）。
    pub fn focus_order(&self) -> Vec<Focus> {
        let mut v: Vec<Focus> = (0..OFFER_ROWS).map(Focus::Row).collect();
        v.push(Focus::Lock);
        if self.trade.me.locked && self.trade.peer.locked {
            v.push(Focus::Confirm);
        }
        v.push(Focus::Cancel);
        v
    }

    /// 焦点在循环里走一步（↑ 为 -1，↓ 为 +1），到头停住。
    pub fn move_focus(&mut self, delta: i32) {
        let order = self.focus_order();
        let i = order.iter().position(|f| *f == self.focus).unwrap_or(order.len() - 1) as i32;
        let next = (i + delta).clamp(0, order.len() as i32 - 1) as usize;
        self.focus = order[next];
    }
}

/// 双方的锁定和确定状态，用来判断这次推送有没有动锁。
fn lock_states(t: &Trade) -> (bool, bool, bool, bool) {
    (t.me.locked, t.me.confirmed, t.peer.locked, t.peer.confirmed)
}

/// 比较对方两侧报价，返回变了的行序号。
pub fn peer_changed_rows(old: &TradeSide, new: &TradeSide) -> Vec<usize> {
    let mut rows = Vec::new();
    for i in 0..2usize {
        let a = old.items.get(i).map(|t| (t.item_id, t.count, t.durability));
        let b = new.items.get(i).map(|t| (t.item_id, t.count, t.durability));
        if a != b {
            rows.push(i);
        }
    }
    if old.gold != new.gold {
        rows.push(2);
    }
    let pet_a = old.pet.as_ref().map(|p| (p.name.as_str(), p.level));
    let pet_b = new.pet.as_ref().map(|p| (p.name.as_str(), p.level));
    if pet_a != pet_b {
        rows.push(3);
    }
    rows
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{TradeItem, TradePet};

    fn item(id: u32, count: u32) -> TradeItem {
        TradeItem {
            item_id: id,
            count,
            ..Default::default()
        }
    }

    fn trade(rev: u64) -> Trade {
        Trade {
            id: "t-1".into(),
            rev,
            ..Default::default()
        }
    }

    #[test]
    fn peer_offer_change_marks_rows() {
        let mut w = TradeWindow::new(trade(2));
        let mut next = trade(3);
        next.peer.items = vec![item(7, 5)];
        next.peer.pet = Some(TradePet {
            name: "乌力".into(),
            level: 5,
            ..Default::default()
        });
        assert_eq!(w.apply(next), Some(true));
        assert!(w.peer_changed);
        assert_eq!(w.changed_rows, vec![0, 3]);
        w.acknowledge();
        assert!(!w.peer_changed && w.changed_rows.is_empty());
    }

    #[test]
    fn lock_only_change_is_not_offer_change_but_resets_focus() {
        let mut w = TradeWindow::new(trade(2));
        w.focus = Focus::Lock;
        let mut next = trade(3);
        next.peer.locked = true;
        assert_eq!(w.apply(next), Some(false));
        assert!(!w.peer_changed);
        assert_eq!(w.focus, Focus::Cancel);
    }

    #[test]
    fn stale_rev_is_ignored() {
        let mut w = TradeWindow::new(trade(5));
        assert_eq!(w.apply(trade(4)), None);
        assert_eq!(w.trade.rev, 5);
    }

    #[test]
    fn confirm_appears_only_when_both_locked() {
        let mut w = TradeWindow::new(trade(1));
        assert!(!w.focus_order().contains(&Focus::Confirm));
        w.trade.me.locked = true;
        w.trade.peer.locked = true;
        assert!(w.focus_order().contains(&Focus::Confirm));
    }

    #[test]
    fn focus_moves_and_stops_at_ends() {
        let mut w = TradeWindow::new(trade(1));
        w.focus = Focus::Row(0);
        w.move_focus(-1);
        assert_eq!(w.focus, Focus::Row(0));
        w.move_focus(10);
        assert_eq!(w.focus, Focus::Cancel);
    }
}
