//! 遇敌开关（STAGE-40）：`encounter` 命令的响应和 `encounter_changed` 推送。
//! STAGE-153 起停止遇敌交给主动停机协调（`battle/halt.rs`）：权威状态一到就收尾，
//! 旧开启响应不能翻掉玩家按下的停止意图。

use serde_json::Value;

use super::App;
use crate::proto::EncounterChanged;

impl App {
    /// `encounter on|off` 的响应：更新开关状态；手动开关写进消息栏（查询也报一下当前状态）。
    pub(super) fn on_encounter_ok(&mut self, set: Option<bool>, data: Value) {
        let on = data
            .get("encounter")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        if let Some(w) = self.world.as_mut() {
            w.entry.encounter = on;
        }
        // 停止意图在途：以权威状态为准收尾，不在这里写“已开启”（旧响应晚到也一样）。
        if self.world.as_ref().is_some_and(|w| w.halt.stopping) {
            self.on_halt_state(on);
            return;
        }
        match set {
            Some(true) if on => self.log.system("自动遇敌已开启"),
            Some(false) => self.log.system("自动遇敌已关闭，这场打完就停"),
            _ => self.log.system(if on { "自动遇敌：开" } else { "自动遇敌：关" }),
        }
    }

    /// 推送 `encounter_changed`：只更新状态；自动关闭的原因由随附的 notice 说明，不重复写。
    /// 停止意图在途时用它收尾（服务端确认关了就把“正在关闭”改成“已关闭”）。
    pub(super) fn on_encounter_changed(&mut self, data: Value) {
        let ev: EncounterChanged = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            w.entry.encounter = ev.encounter;
        }
        if self.world.as_ref().is_some_and(|w| w.halt.stopping) {
            self.on_halt_state(ev.encounter);
        }
    }
}
