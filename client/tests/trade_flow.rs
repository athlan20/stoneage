//! STAGE-295 交易的端到端（客户端 + 模拟服务端）：`/trade 路人乙` 邀请，对方接受后窗口打开，放 5 号格小斧头，
//! 锁定，对方锁定并确定，本端确定，成交。回放 `docs/protocol/cases/ui_trade_flow.jsonl`（严格模式）。

mod support;

use crossterm::event::KeyCode::{Down, Enter, Tab, Up};
use stoneage_client::app::trade_state::TradeWindow;
use stoneage_client::app::{App, Kind, Screen};
use support::{opts, Harness};

fn win(a: &App) -> Option<&TradeWindow> {
    a.world.as_ref().and_then(|w| w.trade.as_ref())
}

fn enter_world(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
}

#[test]
fn trade_from_command_to_successful_exchange() {
    let mut h = Harness::new("ui_trade_flow", opts);
    enter_world(&mut h);
    h.command("trade 路人乙");
    h.wait("交易窗口打开", 5, |a| win(a).is_some());
    // 默认焦点在「取消交易」，↑ 五次到 0 号报价行，Enter 打开物品选择，再 Enter 放入背包里的小斧头。
    h.keys(&[Up, Up, Up, Up, Up, Enter, Enter]);
    h.wait("小斧头放进我的报价", 5, |a| {
        win(a).is_some_and(|t| !t.trade.me.items.is_empty())
    });
    // 报价行 0 → 锁定：↓ 四次，Enter 锁定。
    h.keys(&[Down, Down, Down, Down, Enter]);
    h.wait("我方已锁定", 5, |a| win(a).is_some_and(|t| t.trade.me.locked));
    h.wait("对方已锁定并确定，焦点回到取消交易", 5, |a| {
        win(a).is_some_and(|t| t.trade.peer.locked && t.trade.peer.confirmed && t.focus_is_cancel())
    });
    // 「确定」出现在「取消交易」上方：↑ 一次，Enter 确定。
    h.keys(&[Up, Enter]);
    h.wait("交易结束，窗口关闭", 5, |a| win(a).is_none());
    assert!(
        h.app
            .log
            .all()
            .iter()
            .any(|m| m.text == "交易 OK！获得：回复药 x5；付出：小斧头 x1"),
        "消息栏缺少成交文字"
    );
    h.finish();
}
