//! `npc 名字`（STAGE-84，`docs/ui.md`「命令」）：按名字找 NPC 并对话。服务端 `npc_find`（协议 5.43）
//! 已经把结果排好（当前地图在前、近的在前）；这里决定怎么做：同图直接 `talk`；在别的地图上先问
//! 玩家“要传送过去吗”，输入 `y` 确认后 `warp` 过去再 `talk`（确认见 `confirm.rs`）；同名 NPC 分布在好几张地图上时只列出地图。

use serde_json::{Value, json};

use super::App;
use super::confirm::Confirm;
use super::session::Pending;
use crate::proto::{NpcMatch, NpcMatchList, NpcsChanged};

/// 同名 NPC 分布的地图超过这么多张时，消息栏里只列前几张。
const MAP_LIST_LIMIT: usize = 5;

/// 找到的同名 NPC 怎么处理。
#[derive(Debug, PartialEq)]
pub enum Choice {
    /// 就在当前地图上：直接对话。
    Here(NpcMatch),
    /// 只在另一张地图上：问玩家要不要过去。
    Travel(NpcMatch),
    /// 分布在好几张地图上：列出地图（编号、名字），让玩家先去其中一张。
    Ambiguous(Vec<(u32, String)>),
    /// 找不到。
    Nothing,
}

/// 决定怎么做。`matches` 已经是服务端排好的顺序，当前地图的第一个就是离角色最近的。
pub fn choose(matches: &[NpcMatch], here: u32) -> Choice {
    if let Some(n) = matches.iter().find(|m| m.floor == here) {
        return Choice::Here(n.clone());
    }
    let Some(first) = matches.first() else {
        return Choice::Nothing;
    };
    let mut maps: Vec<(u32, String)> = vec![];
    for m in matches {
        if !maps.iter().any(|(floor, _)| *floor == m.floor) {
            maps.push((m.floor, m.map_name.clone()));
        }
    }
    if maps.len() == 1 {
        Choice::Travel(first.clone())
    } else {
        Choice::Ambiguous(maps)
    }
}

impl App {
    /// `npc 名字`：发查找请求，结果在 `on_npc_find` 里处理。名字为空时给出用法。
    pub(super) fn npc_command(&mut self, rest: &str) {
        let name = rest.trim();
        if name.is_empty() {
            self.log.error(super::commands::usage_of("npc"));
            return;
        }
        self.send(
            "npc_find",
            json!({"name": name}),
            Pending::NpcFind(name.to_string()),
        );
    }

    /// `npcs_changed` 推送（STAGE-93，`docs/protocol.md` 5.46）：任务脚本移动了 NPC，整张列表替换。
    /// 推送的地图不是角色现在所在的地图（换图前后的旧推送）时不动。
    pub(super) fn on_npcs_changed(&mut self, data: Value) {
        let changed: NpcsChanged = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            if changed.floor == w.entry.position.floor {
                let id=w.npcs.get(w.scene.npc_index).map(|n|n.id);
                w.scene.npc_query+=1;
                w.npcs = changed.npcs;
                w.scene.npc_index=id.and_then(|id|w.npcs.iter().position(|n|n.id==id)).unwrap_or(w.scene.npc_index).min(w.npcs.len().saturating_sub(1));
                w.scene.npc_load=super::LoadState::Ready;
            }
        }
    }

    /// `npc_find` 响应：按 `choose` 的结果对话、问是否传送，或列出地图。
    pub(super) fn on_npc_find(&mut self, name: &str, data: Value) {
        let list: NpcMatchList = serde_json::from_value(data).unwrap_or_default();
        let here = self.world.as_ref().map_or(0, |w| w.entry.position.floor);
        match choose(&list.npcs, here) {
            Choice::Here(npc) => {
                self.open_npc(npc.id);
            }
            Choice::Travel(npc) => {
                let text = format!(
                    "{} 在 {}，不在这张地图上。要传送过去吗？",
                    npc.name, npc.map_name
                );
                self.ask_confirm(Confirm::NpcVisit(npc), &text);
            }
            Choice::Ambiguous(maps) => {
                let shown: Vec<String> = maps
                    .iter()
                    .take(MAP_LIST_LIMIT)
                    .map(|(floor, map)| format!("{map}（{floor}）"))
                    .collect();
                let more = if maps.len() > MAP_LIST_LIMIT {
                    "等"
                } else {
                    ""
                };
                self.log.error(format!(
                    "同名的 NPC 在 {} 张地图上：{}{more}。先 warp 到其中一张，再输入 npc {name}",
                    maps.len(),
                    shown.join("、")
                ));
            }
            Choice::Nothing => self.log.error(format!("没有叫 {name} 的 NPC")),
        }
    }

    /// 确认传送（`Confirm::NpcVisit`）：发 `warp`，成功后对话（`on_warp` 里处理 `NpcWarp`）。
    pub(super) fn send_npc_visit(&mut self, npc: NpcMatch) {
        self.send(
            "warp",
            json!({"floor": npc.floor}),
            Pending::NpcWarp {
                npc: npc.id,
                map_name: npc.map_name,
            },
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn m(id: u32, floor: u32, map: &str, x: i32) -> NpcMatch {
        NpcMatch {
            id,
            name: "看板".into(),
            type_: "告示牌".into(),
            floor,
            map_name: map.into(),
            x,
            y: 0,
            ..NpcMatch::default()
        }
    }

    #[test]
    fn same_map_talks_directly_even_if_other_maps_match() {
        // 服务端把当前地图的排在前面；这里只要当前地图上有，就直接对话，不管后面还有几张图。
        let list = [m(7, 1006, "村长家", 3), m(8, 1000, "萨姆吉尔村", 0)];
        assert_eq!(choose(&list, 1006), Choice::Here(list[0].clone()));
    }

    #[test]
    fn single_other_map_asks_to_travel() {
        let list = [m(635, 5500, "避难洞窟", 24)];
        assert_eq!(choose(&list, 1006), Choice::Travel(list[0].clone()));
    }

    #[test]
    fn several_maps_are_listed_without_duplicates() {
        // 同一张图上有两个同名 NPC 只列一次地图。
        let list = [
            m(1, 1000, "萨姆吉尔村", 0),
            m(2, 1000, "萨姆吉尔村", 9),
            m(3, 2000, "玛丽娜丝渔村", 0),
        ];
        assert_eq!(
            choose(&list, 1006),
            Choice::Ambiguous(vec![
                (1000, "萨姆吉尔村".into()),
                (2000, "玛丽娜丝渔村".into())
            ])
        );
    }

    #[test]
    fn nothing_found() {
        assert_eq!(choose(&[], 1006), Choice::Nothing);
    }
}
