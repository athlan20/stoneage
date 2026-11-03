//! STAGE-295：附近玩家动作「交易」的灰显原因（`docs/trade.md` 5.1）。只拦住能从本地状态确定的情况；
//! 同图、对方是否关了交易、对方是否在交易中由服务端判断（错误码的说明在 `trade::error_hint`）。

use super::WorldState;
use crate::proto::NearbyPlayer;

/// 邀请对方交易的灰显原因；`None` 表示可用。
pub fn trade_reason(w: &WorldState, t: &NearbyPlayer) -> Option<&'static str> {
    if w.entry.status.hp == 0 {
        return Some("角色已经倒下，不能交易");
    }
    if w.trade.is_some() {
        return Some("交易中，先完成或取消交易");
    }
    if w.battle.is_some() {
        return Some("战斗中不能交易");
    }
    if w.party().is_some_and(|p| p.kind == "players") {
        return Some("组队中不能交易，先离开真人队");
    }
    if t.name == w.entry.character.name {
        return Some("不能和自己交易");
    }
    if t.in_battle {
        return Some("对方在战斗中，不能交易");
    }
    if t.party.as_ref().is_some_and(|p| p["kind"] == "players") {
        return Some("对方在真人队中，不能交易");
    }
    None
}
