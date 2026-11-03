//! 快捷传送页每一行要显示的东西（STAGE-161，STAGE-251 加了「1级宠物」行）：`RowView` 和 `TravelState::view()`。
//! 从 `travel.rs` 拆出；UI 只读这里的结果，不碰内部索引。

use super::{Entry, TravelState};

/// 列表里一行要显示的东西。UI 只读这个，不碰内部索引。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RowView {
    /// 「传送到本图」那个虚拟首项。
    pub here: bool,
    pub name: String,
    /// 地图编号；纯分组是 None。
    pub floor: Option<u32>,
    /// 还能往下钻。
    pub has_children: bool,
    /// 下层有几个子项（含地图节点自己的下级；不包含「传送到本图」）。
    pub children: usize,
    /// 就是角色现在所在地图。
    pub current: bool,
    /// 是根层的「1级宠物」入口（STAGE-251），不是地图。
    pub pets: bool,
    /// 是根层的「练级传送」入口（STAGE-279），不是地图。
    pub leveling: bool,
}

impl TravelState {
    /// 当前层每一行的显示信息（UI 用）。
    pub fn view(&self) -> Vec<RowView> {
        self.entries()
            .into_iter()
            .map(|entry| match entry {
                Entry::Pets => RowView {
                    here: false,
                    name: "1级宠物".to_string(),
                    floor: None,
                    has_children: true,
                    children: 0,
                    current: false,
                    pets: true,
                    leveling: false,
                },
                Entry::Leveling => RowView {
                    here: false,
                    name: "练级传送".to_string(),
                    floor: None,
                    has_children: true,
                    children: 0,
                    current: false,
                    pets: false,
                    leveling: true,
                },
                Entry::Here(id) => {
                    let node = self.nodes.get(&id);
                    RowView {
                        here: true,
                        name: node.map(|n| n.name.clone()).unwrap_or_default(),
                        floor: node.and_then(|n| n.floor),
                        has_children: true,
                        children: self.children.get(&id).map_or(0, Vec::len),
                        current: node.and_then(|n| n.floor) == self.here_floor,
                        pets: false,
                        leveling: false,
                    }
                }
                Entry::Node(id) => {
                    let node = self.nodes.get(&id);
                    RowView {
                        here: false,
                        name: node.map(|n| n.name.clone()).unwrap_or_default(),
                        floor: node.and_then(|n| n.floor),
                        has_children: self.children.contains_key(&id),
                        children: self.children.get(&id).map_or(0, Vec::len),
                        current: node.and_then(|n| n.floor).is_some() && node.and_then(|n| n.floor) == self.here_floor,
                        pets: false,
                        leveling: false,
                    }
                }
            })
            .collect()
    }

    /// 目录层级里所有能传送的地图 floor（测试和调试用）。
    pub fn floors(&self) -> Vec<u32> {
        self.nodes.values().filter_map(|n| n.floor).collect()
    }
}
