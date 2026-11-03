//! STAGE-274：真人组队的请求与命令（`docs/protocol.md` 5.67）：邀请、申请、答复、撤回、离队、踢人；
//! 社交页「请求」栏与真人队成员的 Enter 动作；`party` 的真人组队子命令。弹窗与到期在 `party_requests.rs`，预检在 `party_rules.rs`。

use std::time::Instant;

use serde_json::{Value, json};

use super::battle::party::{disband_text, member_leave_text};
use super::party::party_columns;
use super::party_requests::new_request;
use super::party_rules as rules;
use super::select::{self, Column, Item, OnEnter};
use super::session::Pending;
use super::world::SOCIAL_MENU;
use super::{App, Confirm, WorldState};
use crate::proto::{PartyChanged, PartyReply, PartyRequestEnvelope};

/// 社交页「请求」栏（第二栏）。
pub const REQUEST_COLUMN: &str = "请求";

/// 真人组队的请求（`Pending::PartyPlayers` 携带），结果由响应或推送告诉界面。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PlayersOp {
    Invite { name: String },
    Join { name: String },
    Reply { id: String, accept: bool },
    Cancel { id: String },
    Leave,
    Kick { name: String },
    Leader { name: String },
}

/// 真人组队的 14 个错误码（`docs/protocol.md` 5.67）各自的中文原因；其余错误码沿用服务端的文字。
pub fn party_error_hint(code: &str) -> Option<&'static str> {
    Some(match code {
        "PLAYER_NOT_FOUND" => "没有这个在线角色",
        "PLAYER_NOT_NEARBY" => "对方不在同一张地图，走近一点再说",
        "TARGET_IN_BATTLE" => "对方正在战斗中",
        "TARGET_IN_PARTY" => "对方已经有队伍了",
        "TARGET_NOT_LEADER" => "对方不是队长，或还没有队伍；想加入请找队长，想组队请用邀请",
        "TARGET_BUSY" => "对方待处理的组队请求已满（5 条），稍后再试",
        "TARGET_REFUSES" => "对方关闭了组队邀请，不接收别人的邀请",
        "PARTY_FULL" => "队伍已满（5 人）",
        "NOT_PARTY_LEADER" => "只有队长能这样做",
        "NOT_PARTY_MEMBER" => "对方不是你队伍里的队员",
        "PARTY_IN_BATTLE" => "队伍正在战斗中，不能变更队伍",
        "NOT_IN_PARTY" => "你现在没有真人队伍",
        "INVALID_TARGET" => "对象不能是自己（离队请用 party leave）",
        "PARTY_FOLLOWER" => "跟随队长，离队后才能自己行动",
        "REQUEST_NOT_FOUND" => "这条组队请求已经结束",
        "REQUEST_PENDING" => "已经有一条待处理的同类请求，等对方答复",
        _ => return None,
    })
}

/// 请求栏的栏位：收到的在前、发出的在后，编号即在两者拼起来的顺序里的下标。
pub fn request_column(world: &WorldState) -> Column {
    let n = world.party_requests.incoming.len() + world.party_requests.outgoing.len();
    // 没有请求时放一个不可操作的占位项（编号 -1），让栏可以切到，页面画「没有待处理的组队请求」。
    let items = if n == 0 {
        vec![Item {
            key: -1,
            usable: true,
            enter: None,
        }]
    } else {
        (0..n)
            .map(|i| Item {
                key: i as i64,
                usable: true,
                enter: Some(OnEnter::Open),
            })
            .collect()
    };
    Column {
        name: REQUEST_COLUMN,
        items,
        shown: true,
        detail: false,
        default_key: None,
    }
}

/// 社交页现在停在请求栏。
pub fn on_request_column(world: &WorldState) -> bool {
    select::focus_column(&party_columns(world), &world.select[SOCIAL_MENU]) == 1
}

/// 请求栏里选中的行（同 `request_column` 的编号）。
pub fn selected_request(world: &WorldState) -> Option<usize> {
    let cols = party_columns(world);
    select::selections(&cols, &world.select[SOCIAL_MENU])
        .get(1)
        .copied()
        .flatten()
        .and_then(|k| usize::try_from(k).ok())
}

/// 发请求前的本地预检：只看本地已经知道的（附近列表里的对方、自己的队伍）。
fn precheck(w: &WorldState, op: &PlayersOp) -> Option<&'static str> {
    match op {
        PlayersOp::Invite { name } => w
            .scene
            .players
            .list
            .iter()
            .find(|p| &p.name == name)
            .and_then(|t| rules::invite_reason(w, t)),
        PlayersOp::Join { name } => w
            .scene
            .players
            .list
            .iter()
            .find(|p| &p.name == name)
            .and_then(|t| rules::apply_reason(w, t)),
        PlayersOp::Kick { name } => rules::kick_reason(w, name),
        PlayersOp::Leader { name } => rules::leader_reason(w, name),
        PlayersOp::Leave => rules::leave_reason(w),
        PlayersOp::Reply { .. } | PlayersOp::Cancel { .. } => None,
    }
}

impl App {
    /// 真人组队的请求：先查观察模式和本地预检，再发出。返回是否发出。
    pub(super) fn players_request(&mut self, op: PlayersOp) -> bool {
        if self.observer_blocks() {
            return false;
        }
        if let Some(r) = self.world.as_ref().and_then(|w| precheck(w, &op)) {
            self.log.error(r);
            return false;
        }
        let (ty, data) = match &op {
            PlayersOp::Invite { name } => ("party_invite", json!({ "name": name })),
            PlayersOp::Join { name } => ("party_join", json!({ "name": name })),
            PlayersOp::Reply { id, accept } => ("party_reply", json!({ "id": id, "accept": accept })),
            PlayersOp::Cancel { id } => ("party_cancel", json!({ "id": id })),
            PlayersOp::Leave => ("party_leave", json!({})),
            PlayersOp::Kick { name } => ("party_kick", json!({ "name": name })),
            PlayersOp::Leader { name } => ("party_leader", json!({ "name": name })),
        };
        let answering = match &op {
            PlayersOp::Reply { id, .. } => Some(id.clone()),
            _ => None,
        };
        let sent = self.send(ty, data, Pending::PartyPlayers(op));
        if let (true, Some(id), Some(w)) = (sent, answering, self.world.as_mut()) {
            if let Some(p) = w.party_requests.find_mut(&id) {
                p.answering = true;
            }
        }
        sent
    }

    /// 请求成功：邀请、申请的回复带着新请求，进发出的列表。
    fn request_sent(&mut self, data: Value, text: String) {
        let Ok(env) = serde_json::from_value::<PartyRequestEnvelope>(data) else {
            return;
        };
        if let Some(w) = self.world.as_mut() {
            w.party_requests.outgoing.push(new_request(env.request, Instant::now()));
        }
        self.log.party(text);
    }

    pub(super) fn on_players_op_ok(&mut self, op: PlayersOp, data: Value) {
        match op {
            PlayersOp::Invite { name } => {
                self.request_sent(data, format!("已向 {name} 发出组队邀请，等对方答复（30 秒）"));
            }
            PlayersOp::Join { name } => {
                self.request_sent(data, format!("已向 {name} 的队长申请加入，等对方答复（30 秒）"));
            }
            PlayersOp::Reply { id, accept } => {
                let reply = serde_json::from_value::<PartyReply>(data).ok();
                let Some(w) = self.world.as_mut() else {
                    return;
                };
                let found = w.party_requests.incoming.iter().find(|p| p.req.id == id);
                let from = found.map(|p| p.req.from.clone()).unwrap_or_default();
                let what = if found.is_some_and(|p| p.req.kind == "apply") {
                    "申请"
                } else {
                    "邀请"
                };
                w.party_requests.remove(&id);
                if accept {
                    // 接受成功的文字由 party_changed（member_joined / created）写，这里只接上队伍。
                    if let Some(p) = reply.and_then(|r| r.party) {
                        w.entry.party = Some(p);
                    }
                } else {
                    self.log.party(format!("已拒绝 {from} 的组队{what}"));
                }
            }
            PlayersOp::Cancel { id } => {
                if let Some(w) = self.world.as_mut() {
                    w.party_requests.remove(&id);
                }
                self.log.party("已撤回组队请求");
            }
            PlayersOp::Leave | PlayersOp::Kick { .. } | PlayersOp::Leader { .. } => {}
        }
    }

    /// 请求失败：答复时找不到请求就从栏里去掉，其余（如满员）只解除「处理中」，推送会告诉结果。
    pub(super) fn on_players_op_failed(&mut self, op: &PlayersOp, code: &str) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        match op {
            PlayersOp::Reply { id, .. } if code == "REQUEST_NOT_FOUND" => {
                w.party_requests.remove(id);
            }
            PlayersOp::Reply { id, .. } => {
                if let Some(p) = w.party_requests.find_mut(id) {
                    p.answering = false;
                }
            }
            PlayersOp::Cancel { id } if code == "REQUEST_NOT_FOUND" => {
                w.party_requests.remove(id);
            }
            _ => {}
        }
    }

    /// `party_changed` 入口：真人队的变化在这里写消息，分身队仍交给 `on_party_changed`（STAGE-162）。
    pub(super) fn on_party_changed_any(&mut self, data: Value) {
        let Ok(change) = serde_json::from_value::<PartyChanged>(data.clone()) else {
            self.log.error("队伍推送格式无效");
            return;
        };
        let local_players = self
            .world
            .as_ref()
            .and_then(|w| w.party())
            .is_some_and(|p| p.kind == "players");
        let players = match change.reason.as_str() {
            "created" | "disbanded" | "moved" => change.party.as_ref().map_or(local_players, |p| p.kind == "players"),
            "member_joined" | "member_left" | "leader_changed" | "member_kicked" | "kicked" | "left" => true,
            _ => false,
        };
        if !players {
            self.on_party_changed(data);
            return;
        }
        let me = self
            .world
            .as_ref()
            .map(|w| w.entry.character.name.clone())
            .unwrap_or_default();
        // 转让时原队长还在队里（收到 leader_changed）；离队时原队长已经收不到推送，所以这里只会是转让。
        let was_leader = self
            .world
            .as_ref()
            .and_then(|w| w.party())
            .is_some_and(|p| p.leader == me);
        let leader = change.party.as_ref().map(|p| p.leader.clone()).unwrap_or_default();
        let mine = change.who == me;
        let who = change.who.clone();
        let cause = change.cause.as_deref().unwrap_or_default();
        let cause_text = match change.reason.as_str() {
            "member_left" | "member_kicked" | "kicked" | "left" => member_leave_text(&who, mine, cause),
            _ => None,
        };
        let text = match change.reason.as_str() {
            _ if cause_text.is_some() => cause_text,
            "created" | "member_joined" if mine => Some(format!("你加入了 {leader} 的队伍")),
            "created" | "member_joined" => Some(format!("{who} 加入了你的队伍")),
            "member_left" => Some(format!("{who} 离开了队伍")),
            "leader_changed" if mine => Some("你成了队长".into()),
            "leader_changed" if was_leader => Some(format!("你把队长转让给了 {who}")),
            "leader_changed" => Some(format!("队长换成了 {who}")),
            "member_kicked" => Some(format!("{who} 被请出了队伍")),
            "kicked" => Some("你被请出了队伍".into()),
            "left" => Some("你离开了队伍".into()),
            "disbanded" => Some(disband_text(cause).into()),
            _ => None,
        };
        if let Some(w) = self.world.as_mut() {
            w.entry.party = change.party;
        }
        self.after_party_update();
        if let Some(t) = text {
            self.log.party(t);
        }
    }

    /// `party` 的真人组队子命令：invite / join / accept / decline / leave / kick。返回 false 表示不是这些子命令。
    pub(super) fn party_players_command(&mut self, args: &[&str]) -> bool {
        match args {
            ["invite", name] => {
                self.players_request(PlayersOp::Invite { name: (*name).into() });
            }
            ["join", name] => {
                self.players_request(PlayersOp::Join { name: (*name).into() });
            }
            ["accept"] => self.answer_request(None, true),
            ["accept", name] => self.answer_request(Some(*name), true),
            ["decline"] => self.answer_request(None, false),
            ["decline", name] => self.answer_request(Some(*name), false),
            ["leave"] => self.ask_leave_party(),
            ["leave", "yes"] => {
                self.players_request(PlayersOp::Leave);
            }
            ["kick", name] => self.ask_kick(name),
            ["kick", name, "yes"] => {
                self.players_request(PlayersOp::Kick { name: (*name).into() });
            }
            ["leader", name] => self.ask_leader(name),
            ["leader", name, "yes"] => self.transfer_leader(name),
            _ => return false,
        }
        true
    }

    /// 答复收到的请求：不带名字时只有一条才算数，多条必须带名字。
    fn answer_request(&mut self, name: Option<&str>, accept: bool) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let list: Vec<(String, String)> = w
            .party_requests
            .incoming
            .iter()
            .map(|p| (p.req.id.clone(), p.req.from.clone()))
            .collect();
        let target = match name {
            Some(n) => list.iter().find(|(_, from)| from.as_str() == n).cloned(),
            None if list.len() > 1 => {
                self.log.error("有多条组队请求，请带上名字，如 party accept 阿牛");
                return;
            }
            None => list.first().cloned(),
        };
        let Some((id, _)) = target else {
            self.log.error(if name.is_some() {
                "没有来自这个名字的组队请求"
            } else {
                "没有待处理的组队请求"
            });
            return;
        };
        self.players_request(PlayersOp::Reply { id, accept });
    }

    /// 离队先问一句，默认取消（`party leave yes` 直接离）。
    pub(super) fn ask_leave_party(&mut self) {
        if self.observer_blocks() {
            return;
        }
        if let Some(r) = self.world.as_ref().and_then(rules::leave_reason) {
            self.log.error(r);
            return;
        }
        self.ask_confirm(Confirm::PartyLeave, "离开队伍？离队后要重新组队或请队长再邀请");
    }

    /// 请出队员先问一句，默认取消（`party kick 名字 yes` 直接踢）。
    pub(super) fn ask_kick(&mut self, name: &str) {
        if self.observer_blocks() {
            return;
        }
        if let Some(r) = self.world.as_ref().and_then(|w| rules::kick_reason(w, name)) {
            self.log.error(r);
            return;
        }
        let text = format!("把 {name} 请出队伍？");
        self.ask_confirm(Confirm::PartyKick(name.to_string()), &text);
    }

    /// 社交页 Enter：请求栏开请求；真人队的成员按身份处理；其余照分身队（STAGE-162）。
    pub(super) fn social_enter(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        if on_request_column(w) {
            self.request_enter();
            return;
        }
        if super::social_settings::on_settings_column(w) {
            self.social_toggle();
            return;
        }
        self.party_open();
    }

    /// 请求栏 Enter：收到的重新打开弹窗，发出的直接撤回（不二次确认，`docs/ui.md`）。
    fn request_enter(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let Some(i) = selected_request(w) else {
            return;
        };
        let n_in = w.party_requests.incoming.len();
        let id_in = w.party_requests.incoming.get(i).map(|p| p.req.id.clone());
        let out = i
            .checked_sub(n_in)
            .and_then(|j| w.party_requests.outgoing.get(j))
            .map(|p| (p.req.id.clone(), p.req.kind == crate::app::trade::TRADE_KIND));
        if let Some(id) = id_in {
            if let Some(w) = self.world.as_mut() {
                w.party_requests.reopen(&id);
            }
        } else if let Some((id, true)) = out {
            // STAGE-295：发出的交易邀请撤回走 trade_cancel。
            self.trade_request(super::trade::TradeOp::Cancel { id });
        } else if let Some((id, false)) = out {
            self.players_request(PlayersOp::Cancel { id });
        }
    }
}
