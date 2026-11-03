//! STAGE-295：交易（`docs/protocol.md` 5.72、`docs/trade.md` 第五节）的请求、响应与推送，以及结束时消息栏的文字。
//! 邀请与收到的请求和组队共用 `party_requests` 的队列（`kind` 为 `trade`）；窗口状态在 `trade_state.rs`。

use std::time::Instant;

use serde_json::{json, Value};

use super::party_requests::PendingRequest;
use super::trade_state::TradeWindow;
use super::trade_text::{ended_text, last_error_text, request_result_message};
use super::{App, WorldState};
use crate::proto::{
    PartyRequest, Trade, TradeEnded, TradeEnvelope, TradeRequest, TradeRequestEnvelope, TradeRequestResult,
};

/// 收到的请求在共用队列里的 `kind`（服务端的请求对象没有 kind，客户端自己标记）。
pub const TRADE_KIND: &str = "trade";

/// 交易的请求（`Pending::Trade` 携带）；结果由响应或推送告诉界面。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TradeOp {
    Invite {
        name: String,
    },
    Reply {
        id: String,
        accept: bool,
    },
    Cancel {
        id: String,
    },
    Offer {
        id: String,
        items: Vec<u32>,
        gold: u64,
        pet: Option<u32>,
    },
    Lock {
        id: String,
        rev: u64,
    },
    Unlock {
        id: String,
    },
    Confirm {
        id: String,
        rev: u64,
    },
}

impl TradeOp {
    fn wire(&self) -> (&'static str, Value) {
        match self {
            Self::Invite { name } => ("trade_invite", json!({ "name": name })),
            Self::Reply { id, accept } => ("trade_reply", json!({ "id": id, "accept": accept })),
            Self::Cancel { id } => ("trade_cancel", json!({ "id": id })),
            Self::Offer { id, items, gold, pet } => (
                "trade_offer",
                json!({ "id": id, "items": items, "gold": gold, "pet": pet }),
            ),
            Self::Lock { id, rev } => ("trade_lock", json!({ "id": id, "rev": rev })),
            Self::Unlock { id } => ("trade_unlock", json!({ "id": id })),
            Self::Confirm { id, rev } => ("trade_confirm", json!({ "id": id, "rev": rev })),
        }
    }

    /// 发出时要标记“处理中”的请求编号（答复、撤回）。
    fn request_id(&self) -> Option<&str> {
        match self {
            Self::Reply { id, .. } | Self::Cancel { id } => Some(id),
            _ => None,
        }
    }

    /// 是否作用在交易窗口上（发出时窗口进入处理中）。
    fn on_window(&self) -> bool {
        !matches!(self, Self::Invite { .. } | Self::Reply { .. })
    }
}

/// 收到的交易请求放进共用队列：`trade_from` 记下对方的等级和转生，弹窗用它判断是不是新手号。
pub fn new_trade_request(req: TradeRequest, now: Instant) -> PendingRequest {
    let expires_at = now + std::time::Duration::from_secs(req.expires_in);
    PendingRequest {
        req: PartyRequest {
            id: req.id,
            kind: TRADE_KIND.into(),
            from: req.from,
            to: req.to,
            party: None,
            expires_in: req.expires_in,
        },
        expires_at,
        answering: false,
        trade_from: Some((req.from_level, req.from_transmigration)),
    }
}

fn window_of(w: &WorldState, id: &str) -> bool {
    w.trade.as_ref().is_some_and(|t| t.trade.id == id)
}

impl App {
    /// 交易请求：观察模式下不发，发出时在途的答复和窗口操作进入处理中。返回是否发出。
    pub(super) fn trade_request(&mut self, op: TradeOp) -> bool {
        if self.observer_blocks() {
            return false;
        }
        let (ty, data) = op.wire();
        let request_id = op.request_id().map(str::to_string);
        let on_window = op.on_window();
        let sent = self.send(ty, data, super::pending::Pending::Trade(op));
        if let (true, Some(w)) = (sent, self.world.as_mut()) {
            if let Some(id) = request_id {
                if let Some(p) = w.party_requests.find_mut(&id) {
                    p.answering = true;
                }
            }
            if on_window {
                if let Some(t) = w.trade.as_mut() {
                    t.busy = true;
                }
            }
        }
        sent
    }

    /// 邀请成功：发出的请求进发出列表，等对方答复。
    fn trade_invite_sent(&mut self, name: &str, data: Value) {
        let Ok(env) = serde_json::from_value::<TradeRequestEnvelope>(data) else {
            return;
        };
        if let Some(w) = self.world.as_mut() {
            let mut p = new_trade_request(env.request, Instant::now());
            p.req.to = name.to_string();
            w.party_requests.outgoing.push(p);
        }
        self.log.push(
            super::Kind::Trade,
            format!("已向 {name} 发出交易邀请，等对方答复（30 秒）"),
        );
    }

    /// 响应成功（`trade_*`）。
    pub(super) fn on_trade_ok(&mut self, op: TradeOp, data: Value) {
        match op {
            TradeOp::Invite { name } => self.trade_invite_sent(&name, data),
            TradeOp::Reply { id, accept } => {
                let from = self.world.as_mut().and_then(|w| {
                    let found = w
                        .party_requests
                        .incoming
                        .iter()
                        .find(|p| p.req.id == id)
                        .map(|p| p.req.from.clone());
                    w.party_requests.remove(&id);
                    found
                });
                if accept {
                    if let Ok(env) = serde_json::from_value::<TradeEnvelope>(data) {
                        self.open_trade(env.trade);
                    }
                } else {
                    self.log
                        .party(format!("已拒绝 {} 的交易邀请", from.unwrap_or_default()));
                }
            }
            TradeOp::Cancel { id } => {
                // STAGE-295：取消窗口的文字只由 trade_ended 写一次；这里只说真的撤回了邀请（请求栏里确有这条）。
                // 窗口若还开着（响应先到），这里先关掉，随后的 trade_ended 照样写「你取消了交易」。
                let withdrawn = self.world.as_mut().is_some_and(|w| {
                    if window_of(w, &id) {
                        w.trade = None;
                        return false;
                    }
                    w.party_requests.remove(&id).is_some()
                });
                if withdrawn {
                    self.log.push(super::Kind::Trade, "已撤回交易邀请");
                }
            }
            TradeOp::Confirm { .. } if data.get("done").and_then(Value::as_bool) == Some(true) => {
                // 交换成功：窗口由随后的 trade_ended（completed）关闭，这里只解除处理中。
                self.trade_busy(false);
            }
            TradeOp::Offer { .. } | TradeOp::Lock { .. } | TradeOp::Unlock { .. } | TradeOp::Confirm { .. } => {
                self.trade_busy(false);
                if let Ok(env) = serde_json::from_value::<TradeEnvelope>(data) {
                    self.apply_trade(env.trade);
                }
            }
        }
    }

    /// 响应失败：请求栏的“处理中”解除；交易已不存在时关窗口；错误文字由通用路径写。
    pub(super) fn on_trade_failed(&mut self, op: &TradeOp, code: &str) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        match op {
            TradeOp::Reply { id, .. } | TradeOp::Cancel { id } if code == "REQUEST_NOT_FOUND" => {
                w.party_requests.remove(id);
            }
            TradeOp::Reply { id, .. } => {
                if let Some(p) = w.party_requests.find_mut(id) {
                    p.answering = false;
                }
            }
            _ => {}
        }
        if code == "TRADE_NOT_FOUND" {
            w.trade = None;
        } else {
            self.trade_busy(false);
        }
    }

    fn trade_busy(&mut self, busy: bool) {
        if let Some(t) = self.world.as_mut().and_then(|w| w.trade.as_mut()) {
            t.busy = busy;
        }
    }

    /// 打开交易窗口（接受邀请的响应、或 trade_updated 首次到达）。
    pub(super) fn open_trade(&mut self, trade: Trade) {
        let peer = trade.peer.name.clone();
        if let Some(w) = self.world.as_mut() {
            w.party_requests.remove(&trade.id);
            w.trade = Some(TradeWindow::new(trade));
        }
        self.log.push(super::Kind::Trade, format!("开始和 {peer} 交易"));
    }

    /// 交易对象的更新：窗口已开则替换（对方改报价时写提示），没开则打开。
    pub(super) fn apply_trade(&mut self, trade: Trade) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        if w.trade.is_none() {
            self.open_trade(trade);
            return;
        }
        let peer = trade.peer.name.clone();
        let err = trade.last_error.clone();
        let Some(win) = w.trade.as_mut() else {
            return;
        };
        match win.apply(trade) {
            None => return,
            Some(true) => self.log.system("对方改动了报价，双方锁定已取消"),
            Some(false) => {}
        }
        if let Some(e) = err {
            self.log.error(last_error_text(&e, &peer));
        }
    }

    /// 推送 `trade_request`：收到交易邀请，进共用队列（弹窗由界面决定画不画）。
    pub(super) fn on_trade_request(&mut self, data: Value) {
        let Ok(env) = serde_json::from_value::<TradeRequestEnvelope>(data) else {
            self.log.error("交易请求推送格式无效");
            return;
        };
        let from = env.request.from.clone();
        let Some(w) = self.world.as_mut() else {
            return;
        };
        if w.observing() {
            // 观察方或 agent 操控时不弹窗、不入队，只在消息栏提示（`docs/trade.md` 5.2）。
            self.log
                .push(super::Kind::Trade, format!("{from} 想和你交易（agent 正在操控）"));
            return;
        }
        w.party_requests
            .incoming
            .push(new_trade_request(env.request, Instant::now()));
        self.log
            .push(super::Kind::Trade, format!("{from} 想和你交易（30 秒内答复）"));
    }

    /// 推送 `trade_request_result`：邀请结束，从两栏移除，并在消息栏说一句。
    pub(super) fn on_trade_request_result(&mut self, data: Value) {
        let Ok(r) = serde_json::from_value::<TradeRequestResult>(data) else {
            self.log.error("交易结果推送格式无效");
            return;
        };
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let Some(incoming) = w.party_requests.remove(&r.id) else {
            return;
        };
        if let Some(text) = request_result_message(&r.from, &r.to, &r.status, &r.code, incoming) {
            self.log.push(super::Kind::Trade, text);
        }
    }

    /// 推送 `trade_updated`：交易对象有变化（建立、对方改报价、锁定、确定、回到编辑）。
    pub(super) fn on_trade_updated(&mut self, data: Value) {
        let Ok(env) = serde_json::from_value::<TradeEnvelope>(data) else {
            self.log.error("交易推送格式无效");
            return;
        };
        self.apply_trade(env.trade);
    }

    /// 推送 `trade_ended`：窗口关闭，消息栏写结果。
    pub(super) fn on_trade_ended(&mut self, data: Value) {
        let Ok(e) = serde_json::from_value::<TradeEnded>(data) else {
            self.log.error("交易结束推送格式无效");
            return;
        };
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let me = w.entry.character.name.clone();
        let peer = w.trade.as_ref().map(|t| t.trade.peer.name.clone()).unwrap_or_default();
        w.trade = None;
        let ok = e.status == "completed";
        let (text, red) = ended_text(&e, &me, &peer);
        if red {
            self.log.error(text);
        } else if ok {
            self.log.push(super::Kind::TradeOk, text);
        } else {
            self.log.push(super::Kind::Trade, text);
        }
    }
}
