//! STAGE-295：交易请求的自动应答。用例没写交易步骤时，单玩家连接找不到对方，按 `docs/protocol.md` 5.72 回复对应错误码；
//! 用例里写了这一步的，按用例走（调用方先判断）。

use serde_json::{Value, json};

pub fn is_trade_request(ty: &str) -> bool {
    matches!(
        ty,
        "trade_invite" | "trade_reply" | "trade_cancel" | "trade_offer" | "trade_lock" | "trade_unlock" | "trade_confirm"
    )
}

/// 邀请找不到对方；其余操作找不到交易或请求。
pub fn auto_reply(id: u64, ty: &str) -> Value {
    let (code, message) = match ty {
        "trade_invite" => ("PLAYER_NOT_FOUND", "没有这个在线角色"),
        _ => ("TRADE_NOT_FOUND", "这笔交易已经结束"),
    };
    json!({"id": id, "type": ty, "ok": false, "error": {"code": code, "message": message}})
}
