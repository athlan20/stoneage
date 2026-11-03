//! 观察模式（agent 操控时）的按键拦截（STAGE-104，`docs/interaction.md`「快捷键与命令」）：操作类按键在客户端就拦下，
//! 写一条提示，不发给服务端再等它回 `CONTROLLED_BY_AGENT`。命令表里的“操作”类命令由 `run_command` 拦（STAGE-26），
//! 这里拦的是快捷键：对话窗口的数字选项、战斗的出招、确认里的 y。

use super::App;

/// 观察模式下操作被拦时消息栏的提示（和手敲命令一样）。
pub const OBSERVER_BLOCKED: &str = "agent 正在操控，输入 take 收回控制权";

impl App {
    /// 观察模式下要拦：写一条提示并返回 true；不是观察模式返回 false，什么也不做。
    pub(super) fn observer_blocks(&mut self) -> bool {
        if self.world.as_ref().is_some_and(|w|!w.online){self.log.error("连接已断开，请从系统菜单重新连接");return true;}
        if !self.world.as_ref().is_some_and(|w| w.observing()) {
            return false;
        }
        self.log.error(OBSERVER_BLOCKED);
        true
    }
}
