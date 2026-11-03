//! STAGE-295：交易的文字：错误码的中文原因、请求结束、`last_error` 和结束摘要的消息栏话（`docs/trade.md` 5.2）。
//! 只做字符串，不碰状态；窗口和请求在 `trade.rs`。

use crate::proto::{TradeEnded, TradeLastError, TradeOffer};

/// 交易的错误码（`docs/protocol.md` 5.72）各自的中文原因；其余沿用服务端的文字。
pub fn error_hint(code: &str) -> Option<&'static str> {
    Some(match code {
        "IN_TRADE" => "交易中，先完成或取消交易",
        "TARGET_IN_TRADE" => "对方正在交易中",
        "TRADE_NOT_FOUND" => "这笔交易已经结束",
        "TRADE_STALE" => "对方刚改了报价，请看清楚再操作",
        "ITEM_UNTRADABLE" => "这件物品不能交易",
        "PET_UNTRADABLE" => "这只宠物不能交易",
        "PET_FULL" => "宠物栏已满，请整理后再交易",
        "PET_TOO_STRONG" => "宠物等级太高，对方收不了",
        "TRADE_OFF" => "交易尚未开放",
        // STAGE-295：对方关了交易开关（不是组队的开关），文案与组队的 TARGET_REFUSES 区分。
        "TARGET_REFUSES" => "对方不接收交易邀请",
        _ => return None,
    })
}

/// 请求结束写进消息栏的话（组队的 `result_message` 遇到 `kind` 为 trade 时转到这里）。
pub fn request_result_message(from: &str, to: &str, status: &str, code: &str, incoming: bool) -> Option<String> {
    let tail = || {
        if code.is_empty() {
            "已撤回".to_string()
        } else {
            let why = error_hint(code)
                .or_else(|| super::party_players::party_error_hint(code))
                .unwrap_or("原因未知");
            format!("已作废：{why}")
        }
    };
    Some(match (incoming, status) {
        (true, "expired") => format!("{from}的交易邀请已过期"),
        (true, "cancelled") => format!("{from}的交易邀请{}", tail()),
        (false, "accepted") => format!("{to}接受了你的交易邀请"),
        (false, "declined") => format!("{to}拒绝了你的交易邀请"),
        (false, "expired") => format!("发给{to}的交易邀请已过期"),
        (false, "cancelled") => format!("发给{to}的交易邀请{}", tail()),
        _ => return None,
    })
}

/// 交换前或锁定时被预演拒绝的原因（`trade_updated` 的 `last_error`），`who` 为接收者视角的 me / peer。
pub fn last_error_text(e: &TradeLastError, peer: &str) -> String {
    let mine = e.who != "peer";
    let who = if mine { "你".to_string() } else { peer.to_string() };
    match e.code.as_str() {
        "BAG_FULL" => format!("{who}物品栏已满，请整理后再交易"),
        "GOLD_FULL" => format!("{who}的石币将会超过上限！"),
        "PET_FULL" => format!("{who}宠物栏已满，请整理后再交易"),
        "PET_TOO_STRONG" if mine => "你等级不够，无法照顾交易后的宠物！".into(),
        "PET_TOO_STRONG" => format!("{peer}等级不够，无法照顾交易后的宠物！"),
        "ITEM_UNTRADABLE" => "丢在地上会消失的物品无法交易！".into(),
        "PET_UNTRADABLE" => "这只宠物不能交易".into(),
        "NOT_ENOUGH_GOLD" => format!("{who}的石币不够"),
        "TRADE_STALE" => "对方刚改了报价，请看清楚再操作".into(),
        other => format!("交易失败（{other}）"),
    }
}

/// 报价的一行说法：“小斧头 x1、石币 5000、宠物 乌力 Lv5”，空的写“无”。
pub fn offer_text(o: &TradeOffer) -> String {
    let mut parts: Vec<String> = o.items.iter().map(|i| format!("{} x{}", i.name, i.count)).collect();
    if o.gold > 0 {
        parts.push(format!("石币 {}", o.gold));
    }
    if let Some(p) = &o.pet {
        parts.push(format!("宠物 {} Lv{}", p.name, p.level));
    }
    if parts.is_empty() {
        "无".into()
    } else {
        parts.join("、")
    }
}

/// 交易结束写进消息栏的话（`docs/trade.md` 5.2 的表）。返回 (文字, 是否出错用红色)。
pub fn ended_text(e: &TradeEnded, me: &str, peer: &str) -> (String, bool) {
    if e.status == "completed" {
        let text = match &e.summary {
            Some(s) => format!("交易 OK！获得：{}；付出：{}", offer_text(&s.got), offer_text(&s.gave)),
            None => "交易 OK！".into(),
        };
        return (text, false);
    }
    let text = match e.code.as_deref().unwrap_or("") {
        // STAGE-295：取消方收到的 trade_ended 的 code 为空（`docs/trade.md` 5.2），靠 by 认出是自己。
        "" | "CANCELLED" if e.by.as_deref() == Some(me) => "你取消了交易".into(),
        "CANCELLED" => format!("{peer}将交易取消了！"),
        "PEER_LEFT" => format!("{peer}已离开，交易取消"),
        "TIMEOUT" => "交易超时，已取消".into(),
        "CONTROL_CHANGED" => "交易因控制权变化取消".into(),
        "INTERNAL_ERROR" => return ("交易出错，已取消，物品和石币未变动".into(), true),
        _ => "交易已取消".into(),
    };
    (text, false)
}
