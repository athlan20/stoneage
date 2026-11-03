//! STAGE-274：组队请求的状态（收到的、发出的）、30 秒有效期、请求弹窗的按键（`docs/ui.md`「请求弹窗」，
//! `docs/interaction.md`「组队请求弹窗」）。状态只在内存里；到期由 `party_requests_tick` 扫描，过期的移除并写一句消息。

use std::collections::HashSet;
use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::Value;

use super::party_players::PlayersOp;
use super::{App, WorldState};
use crate::proto::{PartyRequest, PartyRequestEnvelope, PartyRequestResult};

/// 一条请求和它的截止时刻。`answering` 表示已经答复、等响应：按钮显示「处理中」，不重复发送。
#[derive(Debug, Clone)]
pub struct PendingRequest {
    pub req: PartyRequest,
    pub expires_at: Instant,
    pub answering: bool,
    /// 交易邀请的对方等级和转生（弹窗判断新手号用）；组队请求为 None。
    pub trade_from: Option<(u32, u32)>,
}

impl PendingRequest {
    /// 剩余秒数，向上取整（还剩 0.3 秒也算 1 秒，到期才是 0）。
    pub fn remaining_secs(&self, now: Instant) -> u64 {
        self.expires_at.saturating_duration_since(now).as_secs_f64().ceil() as u64
    }
}

pub fn new_request(req: PartyRequest, now: Instant) -> PendingRequest {
    let expires_at = now + Duration::from_secs(req.expires_in);
    PendingRequest {
        req,
        expires_at,
        answering: false,
        trade_from: None,
    }
}

/// 弹窗里的焦点，默认「拒绝」（默认取消）。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum PopupFocus {
    #[default]
    Reject,
    Accept,
}

#[derive(Debug, Default)]
pub struct PartyRequests {
    /// 收到的邀请或申请，按到达先后排。
    pub incoming: Vec<PendingRequest>,
    /// 自己发出、等对方答复的请求。
    pub outgoing: Vec<PendingRequest>,
    pub focus: PopupFocus,
    /// 按 Esc「稍后处理」的请求编号：不再自动弹窗，仍留在请求栏。
    pub snoozed: HashSet<String>,
}

impl PartyRequests {
    /// 第一条没有被「稍后处理」的收到的请求（弹窗显示它，一次只弹一条）。
    pub fn popup(&self) -> Option<&PendingRequest> {
        self.incoming.iter().find(|r| !self.snoozed.contains(&r.req.id))
    }

    /// 请求栏里 Enter 重新打开弹窗：解除「稍后」，并提到最前。
    pub fn reopen(&mut self, id: &str) {
        self.snoozed.remove(id);
        self.focus = PopupFocus::Reject;
        if let Some(i) = self.incoming.iter().position(|r| r.req.id == id) {
            let r = self.incoming.remove(i);
            self.incoming.insert(0, r);
        }
    }

    /// 从两栏里移除（答复、撤回、结束）。返回 `Some(true)` 表示是收到的那条，`None` 表示没找到。
    pub fn remove(&mut self, id: &str) -> Option<bool> {
        self.snoozed.remove(id);
        if let Some(i) = self.incoming.iter().position(|r| r.req.id == id) {
            self.incoming.remove(i);
            return Some(true);
        }
        let i = self.outgoing.iter().position(|r| r.req.id == id)?;
        self.outgoing.remove(i);
        Some(false)
    }

    pub fn find_mut(&mut self, id: &str) -> Option<&mut PendingRequest> {
        self.incoming
            .iter_mut()
            .chain(self.outgoing.iter_mut())
            .find(|r| r.req.id == id)
    }
}

/// 弹窗现在该不该显示：战斗中、NPC 对话窗口、确认层、自动战斗设置开着时不弹，只在消息栏提示（`docs/ui.md`）。
pub fn shown_popup(w: &WorldState) -> Option<&PendingRequest> {
    if !w.online || w.battle.is_some() || w.npc_window.is_some() || w.confirm.is_some() || w.auto_edit.is_some() {
        return None;
    }
    w.party_requests.popup()
}

/// 结束事件写进消息栏的话（`incoming` 表示这条是收到的）。
pub fn result_message(r: &PartyRequestResult, incoming: bool) -> Option<String> {
    if r.kind == super::trade::TRADE_KIND {
        return super::trade_text::request_result_message(&r.from, &r.to, &r.status, &r.code, incoming);
    }
    let what = if r.kind == "apply" { "申请" } else { "邀请" };
    let text = match (incoming, r.status.as_str()) {
        (true, "expired") => format!("{}的组队{what}已过期", r.from),
        (true, "cancelled") => format!("{}的组队{what}{}", r.from, cancel_tail(r)),
        (false, "accepted") => format!("{}接受了你的组队{what}", r.to),
        (false, "declined") => format!("{}拒绝了你的组队{what}", r.to),
        (false, "expired") => format!("发给{}的组队{what}已过期", r.to),
        (false, "cancelled") => format!("发给{}的组队{what}{}", r.to, cancel_tail(r)),
        _ => return None,
    };
    Some(text)
}

fn cancel_tail(r: &PartyRequestResult) -> String {
    if r.code.is_empty() {
        "已撤回".into()
    } else {
        let why = super::party_players::party_error_hint(&r.code).unwrap_or("原因未知");
        format!("已作废：{why}")
    }
}

impl App {
    /// 推送 `party_request`：收到邀请或申请，进收到的列表，弹窗由界面按 `shown_popup` 决定画不画。
    pub(super) fn on_party_request(&mut self, data: Value) {
        let Ok(env) = serde_json::from_value::<PartyRequestEnvelope>(data) else {
            self.log.error("组队请求推送格式无效");
            return;
        };
        let text = if env.request.kind == "apply" {
            format!("{} 申请加入你的队伍", env.request.from)
        } else {
            format!("{} 邀请你加入队伍", env.request.from)
        };
        let Some(w) = self.world.as_mut() else {
            return;
        };
        w.party_requests.incoming.push(new_request(env.request, Instant::now()));
        self.log.party(text);
    }

    /// 推送 `party_request_result`：请求结束，从两栏移除，并在消息栏说一句。
    pub(super) fn on_party_request_result(&mut self, data: Value) {
        let Ok(r) = serde_json::from_value::<PartyRequestResult>(data) else {
            self.log.error("组队结果推送格式无效");
            return;
        };
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let Some(incoming) = w.party_requests.remove(&r.id) else {
            return;
        };
        if let Some(text) = result_message(&r, incoming) {
            self.log.party(text);
        }
    }

    /// 每个 tick 调一次：到期的请求移除并写一句消息（过期自动关弹窗，`docs/ui.md`）。
    pub fn party_requests_tick(&mut self, now: Instant) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let mut dead = Vec::new();
        for (list, incoming) in [
            (&mut w.party_requests.incoming, true),
            (&mut w.party_requests.outgoing, false),
        ] {
            let (gone, alive): (Vec<_>, Vec<_>) = std::mem::take(list).into_iter().partition(|p| p.expires_at <= now);
            *list = alive;
            dead.extend(gone.into_iter().map(|p| (p, incoming)));
        }
        let texts: Vec<String> = dead
            .iter()
            .filter_map(|(p, incoming)| {
                let r = PartyRequestResult {
                    id: p.req.id.clone(),
                    kind: p.req.kind.clone(),
                    status: "expired".into(),
                    from: p.req.from.clone(),
                    to: p.req.to.clone(),
                    code: String::new(),
                };
                result_message(&r, *incoming)
            })
            .collect();
        for t in texts {
            self.log.party(t);
        }
    }

    /// 弹窗开着时：←→ 换按钮，Enter 激活（默认「拒绝」），Esc 稍后处理（不发请求）；其余按键照常。返回 true 表示已处理。
    pub(super) fn party_popup_key(&mut self, key: KeyEvent) -> bool {
        let Some(w) = self.world.as_mut() else {
            return false;
        };
        let Some(p) = shown_popup(w) else {
            return false;
        };
        let id = p.req.id.clone();
        let answering = p.answering;
        let is_trade = p.req.kind == super::trade::TRADE_KIND;
        match key.code {
            KeyCode::Left | KeyCode::Right => {
                w.party_requests.focus = match w.party_requests.focus {
                    PopupFocus::Reject => PopupFocus::Accept,
                    PopupFocus::Accept => PopupFocus::Reject,
                };
                true
            }
            KeyCode::Esc => {
                w.party_requests.snoozed.insert(id);
                w.party_requests.focus = PopupFocus::Reject;
                true
            }
            KeyCode::Enter => {
                if answering {
                    return true;
                }
                let accept = w.party_requests.focus == PopupFocus::Accept;
                w.party_requests.focus = PopupFocus::Reject;
                if is_trade {
                    self.trade_request(super::trade::TradeOp::Reply { id, accept });
                } else {
                    self.players_request(PlayersOp::Reply { id, accept });
                }
                true
            }
            _ => false,
        }
    }
}
