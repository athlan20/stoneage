//! STAGE-274：真人组队的客户端预检（`docs/social-players.md` 2.1–2.5，`docs/protocol.md` 5.67）。
//! 只拦住能从本地状态确定的情况（倒下、分身队、队员身份、满员、对方状态、对方队伍）；
//! 同图、对方是否在线、请求是否仍有效由服务端判断。这里给的灰显原因就是界面上写的那句话。

use serde_json::Value;

use super::WorldState;
use crate::proto::{MAX_PARTY_SIZE, NearbyPlayer};

/// 队员的换图、遇敌类入口灰显时写的原因（与服务端 `PARTY_FOLLOWER` 的提示一致）。
pub const FOLLOWER_REASON: &str = "跟随队长，离队后才能自己行动";

fn my_name(w: &WorldState) -> &str {
    &w.entry.character.name
}

/// 角色倒下、已有分身队：邀请和申请的共同前提。
fn own_block(w: &WorldState) -> Option<&'static str> {
    if w.entry.status.hp == 0 {
        return Some("角色已经倒下，不能组队");
    }
    if w.party().is_some_and(|p| p.kind == "clone") {
        return Some("已经有分身队，先解散再组真人队");
    }
    None
}

/// 自己是真人队的队员（不是队长）：换图、遇敌类入口灰显。
pub fn follower_reason(w: &WorldState) -> Option<&'static str> {
    let p = w.party()?;
    (p.kind == "players" && p.leader != my_name(w)).then_some(FOLLOWER_REASON)
}

/// 邀请对方：自己是队长（或无队伍）、队伍没满、对方没战斗、对方没有队伍。
pub fn invite_reason(w: &WorldState, t: &NearbyPlayer) -> Option<&'static str> {
    if let Some(r) = own_block(w) {
        return Some(r);
    }
    if t.name == my_name(w) {
        return Some("不能邀请自己");
    }
    if let Some(p) = w.party() {
        if p.leader != my_name(w) {
            return Some("只有队长能邀请，队员不能邀请");
        }
        if p.size() >= MAX_PARTY_SIZE {
            return Some("队伍已满（5 人）");
        }
    }
    if t.in_battle {
        return Some("对方在战斗中，不能邀请");
    }
    if t.party.is_some() {
        return Some("对方已经有队伍了");
    }
    None
}

/// 申请加入对方的队伍：自己没有队伍，对方是队长且队伍没满，对方没战斗。
pub fn apply_reason(w: &WorldState, t: &NearbyPlayer) -> Option<&'static str> {
    if let Some(r) = own_block(w) {
        return Some(r);
    }
    if w.party().is_some() {
        return Some("你已经有队伍了");
    }
    if t.name == my_name(w) {
        return Some("不能申请加入自己的队伍");
    }
    if t.in_battle {
        return Some("对方在战斗中，不能申请");
    }
    let Some(v) = t.party.as_ref() else {
        return Some("对方还没有队伍，想组队就用邀请");
    };
    if v.get("leader").and_then(Value::as_str) != Some(t.name.as_str()) {
        return Some("对方不是队长，只能申请加入队长的队伍");
    }
    let size = v.get("size").and_then(Value::as_u64).unwrap_or(0);
    let max = v.get("max").and_then(Value::as_u64).unwrap_or(MAX_PARTY_SIZE as u64);
    if size >= max {
        return Some("对方队伍已满（5 人）");
    }
    None
}

/// 请出队员：自己是真人队的队长，对方是队员。
pub fn kick_reason(w: &WorldState, name: &str) -> Option<&'static str> {
    let Some(p) = w.party() else {
        return Some("你没有队伍");
    };
    if p.kind != "players" {
        return Some("分身队用 party disband 解散，不能请出分身");
    }
    if p.leader != my_name(w) {
        return Some("只有队长能请人出队");
    }
    if !p.members.iter().any(|m| !m.leader && m.name == name) {
        return Some("对方不在你的队伍里");
    }
    None
}

/// 离队：有真人队才能离，分身队用解散。
pub fn leave_reason(w: &WorldState) -> Option<&'static str> {
    let Some(p) = w.party() else {
        return Some("你没有队伍");
    };
    (p.kind != "players").then_some("分身队用 party disband 解散")
}

/// 转让队长：自己是真人队的队长，对方是队员且不是自己（分身队没有转让，分身没有连接）。
pub fn leader_reason(w: &WorldState, name: &str) -> Option<&'static str> {
    let Some(p) = w.party() else {
        return Some("你没有队伍");
    };
    if p.kind != "players" {
        return Some("分身队没有可转让的队长，要换队长请先解散");
    }
    if p.leader != my_name(w) {
        return Some("只有队长能转让队长");
    }
    if name == my_name(w) {
        return Some("不能转给自己，选一位队员");
    }
    if !p.members.iter().any(|m| !m.leader && m.name == name) {
        return Some("对方不在你的队伍里");
    }
    None
}

/// 队伍频道能不能用：在真人队里（分身队和没有队伍都不行，`docs/chat.md` 1.5）。
pub fn party_chat_ok(w: &WorldState) -> bool {
    w.party().is_some_and(|p| p.kind == "players")
}

/// 发队伍聊天前的本地检查；能发返回 None。
pub fn chat_reason(w: &WorldState) -> Option<&'static str> {
    (!party_chat_ok(w)).then_some("只有真人队里能用队伍频道")
}
