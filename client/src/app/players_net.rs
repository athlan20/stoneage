//! STAGE-273：`players_changed` 推送（`docs/protocol.md` 5.66）。`players` 请求本身走场景列的通用流程（scene_net）。

use super::App;
use crate::proto::PlayersChanged;
use serde_json::Value;

impl App {
    /// 推送的地图不是角色现在所在的地图（换图前后的旧推送）时不动；列表以下一次整表为准。
    pub(super) fn on_players_changed(&mut self, data: Value) {
        let changed: PlayersChanged = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            if changed.floor == w.entry.position.floor {
                w.scene.players.apply(changed.changes);
            }
        }
    }
}
