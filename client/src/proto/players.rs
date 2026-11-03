//! STAGE-273：附近玩家（`docs/protocol.md` 5.66）。`players` 响应是同图其他在线玩家的整表；
//! `players_changed` 推送是增量（enter/update 整条替换，leave 按名字删除）。

use serde::Deserialize;
use serde_json::Value;

/// 列表里的一项。`party` 没有真人队时为 null，有时为 {"leader","size","max"}（STAGE-274），`title` 恒为空串，先按协议解析出来保留。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct NearbyPlayer {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub transmigration: u32,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub in_battle: bool,
    /// 是否接受决斗（STAGE-307，5.66）：开着时同图的人可以直接发起决斗。
    #[serde(default)]
    pub duel: bool,
    #[serde(default)]
    pub party: Option<Value>,
}

/// `players` 响应：`truncated` 为真表示同图超过 50 人，只返回了前 50 个。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct PlayerList {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub truncated: bool,
    #[serde(default)]
    pub players: Vec<NearbyPlayer>,
}

/// `players_changed` 里的一项。`enter`/`update` 带整条 `player`，`leave` 只带 `name`；
/// 不认识的 `op` 留着解析，应用时忽略。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct PlayerChange {
    #[serde(default)]
    pub op: String,
    #[serde(default)]
    pub player: Option<NearbyPlayer>,
    #[serde(default)]
    pub name: String,
}

/// `players_changed` 推送：`floor` 是这张地图的编号，`changes` 按顺序应用。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct PlayersChanged {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub changes: Vec<PlayerChange>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn players_response_parses_null_party_and_truncated() {
        let v = serde_json::json!({"floor": 1000, "truncated": true, "players": [
            {"name": "阿牛", "level": 30, "transmigration": 1, "title": "", "in_battle": true, "party": null}
        ]});
        let l: PlayerList = serde_json::from_value(v).unwrap();
        assert_eq!(l.floor, 1000);
        assert!(l.truncated);
        assert_eq!(l.players[0].name, "阿牛");
        assert_eq!(l.players[0].party, None);
        assert!(l.players[0].in_battle);
    }

    #[test]
    fn players_changed_carries_enter_update_and_leave() {
        let v = serde_json::json!({"floor": 1000, "changes": [
            {"op": "enter", "player": {"name": "小明", "level": 5, "transmigration": 0, "title": "", "in_battle": false, "party": null}},
            {"op": "leave", "name": "小红"},
            {"op": "future", "name": "x"}
        ]});
        let c: PlayersChanged = serde_json::from_value(v).unwrap();
        assert_eq!(c.changes.len(), 3);
        assert_eq!(c.changes[0].player.as_ref().unwrap().name, "小明");
        assert_eq!(c.changes[1].op, "leave");
        assert_eq!(c.changes[1].name, "小红");
        assert_eq!(c.changes[2].op, "future");
    }
}
