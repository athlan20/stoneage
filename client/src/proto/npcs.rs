//! `npcs` 响应（`docs/protocol.md` 5.32，STAGE-68）：角色当前所在地图上的 NPC 列表。

use serde::Deserialize;

/// 列表里的一项。`id` 是 NPC 的编号（全表唯一，以后的 NPC 交互命令沿用）；
/// `type_` 是显示用的类型名（对话、商店…，协议字段叫 `type`）；`x`/`y` 是格子坐标。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct Npc {
    #[serde(default)]
    pub id: u32,
    #[serde(default)]
    pub name: String,
    #[serde(rename = "type", default)]
    pub type_: String,
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcList {
    pub floor: u32,
    pub npcs: Vec<Npc>,
}

/// `npcs_changed` 推送（`docs/protocol.md` 5.46，STAGE-93）：任务脚本移动了 NPC，`floor` 是这张地图的编号，
/// `npcs` 是它现在的完整列表。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcsChanged {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub npcs: Vec<Npc>,
}

/// `npc_find` 响应里的一项（`docs/protocol.md` 5.43，STAGE-84）：同名 NPC 可能在好几张地图上，
/// 所以带上地图编号和地图名；`id` 给 `talk` 用，`floor` 给 `warp` 用。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct NpcMatch {
    #[serde(default)]
    pub id: u32,
    #[serde(default)]
    pub name: String,
    #[serde(rename = "type", default)]
    pub type_: String,
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub map_name: String,
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
    /// 这个 NPC 就在角色当前地图上（`npc_list` 的目录项才有，`docs/protocol.md` 5.47）。
    #[serde(default)]
    pub here: bool,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcMatchList {
    #[serde(default)]
    pub npcs: Vec<NpcMatch>,
}

/// `npc_list` 响应（STAGE-134，`docs/protocol.md` 5.47）：NPC 目录的一页。
/// `total` 是分类 + 搜索之后的全部条数，客户端靠它显示「共几条」。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct NpcCatalog {
    #[serde(default)]
    pub npcs: Vec<NpcMatch>,
    #[serde(default)]
    pub total: usize,
    #[serde(default)]
    pub offset: usize,
    #[serde(default)]
    pub limit: usize,
    /// 目录分类：`reachable`（全部可达）或 `here`（本地图），服务端回显。
    #[serde(default)]
    pub scope: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn npcs_changed_carries_floor_and_full_list() {
        let v = serde_json::json!({"floor": 30012, "npcs": [{"id": 1415, "name": "多洛加", "type": "任务", "x": 7, "y": 4}]});
        let c: NpcsChanged = serde_json::from_value(v).unwrap();
        assert_eq!(c.floor, 30012);
        assert_eq!(c.npcs.len(), 1);
        assert_eq!(c.npcs[0].id, 1415);
        assert_eq!(c.npcs[0].type_, "任务");
    }
}
