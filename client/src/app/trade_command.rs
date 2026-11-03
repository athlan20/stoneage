//! STAGE-295：`trade` 命令（`docs/trade.md` 5.1）：`trade 名字` 邀请，`trade accept|decline [名字]` 答复，`trade cancel` 撤回或取消。
//! 菜单和命令共用 `trade_request`；交易窗口里的锁定、确定不做命令（只有窗口里的按钮）。

use super::trade::TradeOp;
use super::{App, WorldState};

/// 找待答复的交易邀请：指定名字就找那个人发的，否则取最早收到的一条。
fn incoming_trade(w: &WorldState, name: Option<&str>) -> Option<String> {
    w.party_requests
        .incoming
        .iter()
        .find(|p| p.req.kind == super::trade::TRADE_KIND && name.is_none_or(|n| p.req.from == n))
        .map(|p| p.req.id.clone())
}

impl App {
    pub(super) fn trade_command(&mut self, rest: &str) {
        let args: Vec<&str> = rest.split_whitespace().collect();
        match args.as_slice() {
            [] => self
                .log
                .error("用法：trade 名字 | trade accept|decline [名字] | trade cancel"),
            ["cancel"] => self.trade_cancel_command(),
            ["accept" | "同意", rest @ ..] => self.trade_reply_command(rest.first().copied(), true),
            ["decline" | "拒绝", rest @ ..] => self.trade_reply_command(rest.first().copied(), false),
            [name] => {
                if self.world.as_ref().is_some_and(|w| w.trade.is_some()) {
                    self.log.error("交易中，先完成或取消交易");
                    return;
                }
                self.trade_request(TradeOp::Invite {
                    name: (*name).to_string(),
                });
            }
            _ => self
                .log
                .error("用法：trade 名字 | trade accept|decline [名字] | trade cancel"),
        }
    }

    /// `trade cancel`：交易中取消窗口；只有请求的，撤回最早发出的那条。
    fn trade_cancel_command(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let id = match (
            &w.trade,
            w.party_requests
                .outgoing
                .iter()
                .find(|p| p.req.kind == super::trade::TRADE_KIND),
        ) {
            (Some(t), _) => t.trade.id.clone(),
            (None, Some(p)) => p.req.id.clone(),
            (None, None) => {
                self.log.error("没有进行中的交易或交易邀请");
                return;
            }
        };
        self.trade_request(TradeOp::Cancel { id });
    }

    /// `trade accept|decline [名字]`：答复收到的交易邀请。
    fn trade_reply_command(&mut self, name: Option<&str>, accept: bool) {
        let found = self.world.as_ref().and_then(|w| incoming_trade(w, name));
        match found {
            Some(id) => {
                self.trade_request(TradeOp::Reply { id, accept });
            }
            None => self.log.error("没有待处理的交易邀请"),
        }
    }
}
