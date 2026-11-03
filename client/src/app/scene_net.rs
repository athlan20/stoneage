//! STAGE-148/152: request ids carry map generations; the transport rejects old connections.
use super::{
    App,
    scene_state::{LoadState, SceneColumn, SceneQuery},
    session::Pending,
};
use crate::proto::{AreaList, ExitList, NpcList, PlayerList};
use serde_json::{Value, json};
use std::time::{Duration, Instant};

impl App {
    pub(super) fn refresh_scene(&mut self) {
        self.request_scene_list(SceneColumn::Exit);
        self.request_scene_list(SceneColumn::Npc);
        // STAGE-273：附近玩家列与 NPC 列一样，进场景就拉一次；之后靠 players_changed 增量。
        self.request_scene_list(SceneColumn::Players);
        // STAGE-160：遇敌列是常驻的第三列，进场景（进世界、换地图、重连）就一起拉。
        // STAGE-152 原本按需加载（切到该列才请求），但面板一直画着，玩家看到的是永远不动的「加载中…」。
        self.request_scene_list(SceneColumn::Encounter);
    }
    pub(super) fn request_scene_list(&mut self, column: SceneColumn) {
        let Some(w) = self.world.as_mut() else { return };
        let s = &mut w.scene;
        let query = match column {
            SceneColumn::Exit => {
                s.exit_query += 1;
                s.exit_requested_at = Some(Instant::now());
                s.exit_load = LoadState::Loading;
                s.refresh_due = None;
                s.exit_query
            }
            SceneColumn::Npc => {
                s.npc_query += 1;
                s.npc_load = LoadState::Loading;
                s.npc_query
            }
            SceneColumn::Players => {
                s.players.query += 1;
                s.players.load = LoadState::Loading;
                s.players.query
            }
            SceneColumn::Encounter => {
                s.area_query += 1;
                s.area_load = LoadState::Loading;
                s.area_query
            }
        };
        let context = SceneQuery {
            floor: w.entry.position.floor,
            epoch: s.epoch,
            query,
        };
        let (ty, pending) = match column {
            SceneColumn::Exit => ("exits", Pending::Exits(context)),
            SceneColumn::Npc => ("npcs", Pending::Npcs(context)),
            SceneColumn::Players => ("players", Pending::Players(context)),
            SceneColumn::Encounter => ("encounter_areas", Pending::EncounterAreas(context)),
        };
        self.send(ty, json!({}), pending);
    }
    pub(super) fn scene_query_current(&self, c: SceneQuery, column: SceneColumn) -> bool {
        self.world
            .as_ref()
            .is_some_and(|w| w.scene.accepts(c, w.entry.position.floor, column))
    }
    pub(super) fn scene_list_ok(&mut self, c: SceneQuery, column: SceneColumn, data: Value) -> bool {
        if !self.scene_query_current(c, column) {
            return false;
        }
        if data.get("floor").and_then(Value::as_u64) != Some(u64::from(c.floor)) {
            self.scene_list_error(c, column, "响应地图不一致，请重试");
            return false;
        }
        let w = self.world.as_mut().unwrap();
        match column {
            SceneColumn::Players => {
                let Ok(list) = serde_json::from_value::<PlayerList>(data) else {
                    self.scene_list_error(c, column, "玩家数据无效");
                    return false;
                };
                w.scene.players.replace(list.players, list.truncated);
            }
            SceneColumn::Npc => {
                let Ok(list) = serde_json::from_value::<NpcList>(data) else {
                    self.scene_list_error(c, column, "NPC 数据无效");
                    return false;
                };
                let id = w.npcs.get(w.scene.npc_index).map(|n| n.id);
                w.npcs = list.npcs;
                w.scene.npc_index = id
                    .and_then(|id| w.npcs.iter().position(|n| n.id == id))
                    .unwrap_or(w.scene.npc_index)
                    .min(w.npcs.len().saturating_sub(1));
                w.scene.npc_load = LoadState::Ready;
            }
            SceneColumn::Encounter => {
                let Ok(list) = serde_json::from_value::<AreaList>(data) else {
                    self.scene_list_error(c, column, "遇敌区域数据无效");
                    return false;
                };
                // 刷新时按稳定 ID 保住当前选择：列表顺序可能因为敌人配置变化而变。
                let id = w.scene.areas.get(w.scene.area_index).map(|a| a.id.clone());
                w.scene.areas = list.areas;
                w.scene.area_index = id
                    .and_then(|id| w.scene.areas.iter().position(|a| a.id == id))
                    .unwrap_or(w.scene.area_index)
                    .min(w.scene.areas.len().saturating_sub(1));
                w.scene.area_load = LoadState::Ready;
            }
            SceneColumn::Exit => {
                let Ok(list) = serde_json::from_value::<ExitList>(data) else {
                    self.scene_list_error(c, column, "出口数据无效");
                    return false;
                };
                let old_count = w.scene.exits.len();
                let id = w.scene.exits.get(w.scene_index).map(|e| e.id.clone());
                let fixed = w.scene_index.checked_sub(old_count);
                w.scene.exits = list.exits;
                w.scene_index = if let Some(fixed) = fixed {
                    w.scene.exits.len() + fixed.min(2)
                } else {
                    id.and_then(|id| w.scene.exits.iter().position(|e| e.id == id))
                        .unwrap_or(w.scene_index)
                        .min(w.scene.exits.len().saturating_sub(1))
                };
                // First load starts at the first exit, rather than a previously empty fixed entry.
                if w.scene.exit_query == 1 && old_count == 0 && !w.scene.left_touched {
                    w.scene_index = 0;
                }
                w.scene.exit_load = LoadState::Ready;
                w.scene.refresh_due = list.next_change_ms.map(|ms| {
                    Instant::now()
                        + Duration::from_millis(
                            ms.saturating_sub(list.server_time_ms)
                                .saturating_sub(w.scene.exit_requested_at.map_or(0, |at| {
                                    at.elapsed().as_millis().min(i64::MAX as u128) as i64
                                }))
                                .max(1) as u64,
                        )
                });
            }
        }
        true
    }
    pub(super) fn scene_list_error(&mut self, c: SceneQuery, column: SceneColumn, message: &str) {
        if !self.scene_query_current(c, column) {
            return;
        }
        let s = &mut self.world.as_mut().unwrap().scene;
        match column {
            SceneColumn::Npc => s.npc_load = LoadState::Failed(message.into()),
            SceneColumn::Players => s.players.load = LoadState::Failed(message.into()),
            SceneColumn::Encounter => s.area_load = LoadState::Failed(message.into()),
            SceneColumn::Exit => {
                s.exit_load = LoadState::Failed(message.into());
                s.refresh_due = None;
            }
        }
        // 附近玩家不在重连的刷新清单里（STAGE-273）：它失败只在列里显示，不把连接打成离线。
        if column != SceneColumn::Players && self.reconnect.as_ref().is_some_and(|r| r.refreshing()) {
            self.offline(&format!("资料刷新失败：{message}"), false);
        }
    }
    pub(super) fn scene_tick(&mut self, now: Instant) {
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.online && w.scene.refresh_due.is_some_and(|due| now >= due))
        {
            self.request_scene_list(SceneColumn::Exit);
        }
    }
    pub(super) fn use_scene_exit(&mut self) {
        if self.observer_blocks() {
            return;
        }
        let Some(w) = self.world.as_ref() else { return };
        if w.scene.using_exit || self.session.npc_busy() || self.session.scene_operation_busy() {
            return;
        }
        if w.scene.exit_load != LoadState::Ready {
            self.log.error("出口资料尚未就绪，按 F5 重试");
            return;
        }
        let Some(exit) = w.scene.exits.get(w.scene_index) else {
            return;
        };
        if !exit.available {
            self.log.error(
                exit.unavailable_reason
                    .as_deref()
                    .unwrap_or("出口当前不可用"),
            );
            return;
        }
        let data = json!({"source_floor":w.entry.position.floor,"exit_id":exit.id});
        if self.send("use_exit", data, Pending::UseExit) {
            self.world.as_mut().unwrap().scene.using_exit = true;
        }
    }
    /// 停止遇敌（STAGE-152）：走统一的停止动作（STAGE-153），当前这场照常打完。
    pub(super) fn stop_encounter(&mut self) {
        self.stop_encounter_only();
        if self.halt_busy() {
            if let Some(w) = self.world.as_mut() {
                w.scene.area_index = 0; // 停下来之后光标本就没有意义，回到第一个区域
            }
        }
    }
    /// 提交按区域遇敌（STAGE-152）：客户端不改坐标，等服务端的 position_changed 与 battle_start。
    pub(super) fn start_encounter(&mut self, area_id: String, mode: &'static str) {
        let Some(w) = self.world.as_ref() else { return };
        if w.scene.starting_encounter {
            return;
        }
        // 关闭中不允许重新开启遇敌（STAGE-153）：先等停止确认，再由玩家主动开。
        if w.halt.busy() {
            self.log.system("正在关闭自动遇敌，请稍候");
            return;
        }
        let data = json!({
            "source_floor": w.entry.position.floor,
            "area_id": area_id,
            "mode": mode,
        });
        if self.send("encounter_start", data, Pending::EncounterStart) {
            if let Some(w) = self.world.as_mut() {
                w.scene.starting_encounter = true;
            }
        }
    }
}
