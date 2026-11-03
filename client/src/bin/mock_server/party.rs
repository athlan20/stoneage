//! STAGE-274：真人组队请求的自动应答。单玩家连接时对方不存在，按 `docs/protocol.md` 5.67 回复对应错误码；
//! 用例里写了这一步的，按用例走（调用方先判断）。

use serde_json::{Value, json};

pub fn is_party_request(ty: &str) -> bool {
    matches!(
        ty,
        "party_invite" | "party_join" | "party_reply" | "party_cancel" | "party_leave" | "party_kick" | "party_leader"
    )
}

/// STAGE-277：队伍频道的聊天。单玩家连接没有队伍，按 docs/protocol.md 5.68 回 `NOT_IN_PARTY`。
pub fn is_party_chat(data: &Value) -> bool {
    data.get("channel").and_then(Value::as_str) == Some("party")
}

/// 邀请、申请找不到对方；答复、撤回找不到请求；离队、踢人没有队伍。
pub fn auto_reply(id: u64, ty: &str) -> Value {
    let (code, message) = match ty {
        "party_invite" | "party_join" => ("PLAYER_NOT_FOUND", "没有这个在线角色"),
        "party_reply" | "party_cancel" => ("REQUEST_NOT_FOUND", "这条组队请求已经结束"),
        _ => ("NOT_IN_PARTY", "你没有队伍"),
    };
    json!({"id": id, "type": ty, "ok": false, "error": {"code": code, "message": message}})
}
