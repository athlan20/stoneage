//! 人物指令的重交与退回（STAGE-109，`docs/battle-ui.md`「宠物指令阶段可以退回人物指令」）：
//! 人物指令交上、宠物指令还没交时，可以在宠物指令的第一层按 Esc 回到人物指令重选；服务端用新的覆盖先交的那条。
//! 指令被接受、被拒绝的阶段变化也在这里，`net.rs` 只负责接收回复。

use super::{Actor, BattleState, Phase, Pick};

impl BattleState {
    /// 指令被接受（回复里的 `next`）：`pet` 轮到宠物指令，`wait` 等其他玩家或结算。
    /// `pet` 时人物指令已经交上、宠物还没交，之后可以退回重选；`wait` 就不能再改。
    pub fn on_accepted(&mut self, next: &str) {
        self.submitting = false;
        self.reset_pick();
        if matches!(self.phase, Phase::Input(_)) {
            self.phase = if next == "pet" {
                Phase::Input(Actor::Pet)
            } else {
                Phase::Waiting
            };
        }
        self.revisable = next == "pet";
        self.revising = false;
    }

    /// 指令被拒绝：阶段不变，玩家可以重新选。退回重选时被拒的，先交的那条仍然有效（10.5），回到宠物指令。
    pub fn on_rejected(&mut self) {
        self.submitting = false;
        self.reset_pick();
        if self.revising {
            self.revising = false;
            self.phase = Phase::Input(Actor::Pet);
        }
    }

    /// 现在能不能从宠物指令退回人物指令：人物指令交上、宠物指令还没交，没有指令在发送中，
    /// 焦点在指令条第一层（没进列表、没在选目标）。输入行是否为空由按键处理看。
    pub fn can_revert(&self) -> bool {
        self.revisable && !self.submitting && self.pick == Pick::Action && self.phase == Phase::Input(Actor::Pet)
    }

    /// 退回人物指令重选：阶段回到人物，指令条写“人物指令（改）”。
    pub fn revert_to_player(&mut self) {
        self.phase = Phase::Input(Actor::Player);
        self.revising = true;
        self.reset_pick();
    }
}
