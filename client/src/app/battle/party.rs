//! 真人队同场战斗（STAGE-275，`docs/protocol.md` 10.14、`docs/battle-ui.md`「真人队同场战斗」）：
//! 队友的识别与队长标记、队员离场和逃跑的战斗记录，以及等待、倒计时的文字（纯函数，便于测试）。
//! 队友是己方的真人人物，不能被当作分身或选作目标；只有自己和自己的宠物可操作。

use crate::app::Kind;
use crate::proto::{BattleEvent, BattleLeave, Progress};

use super::{BattleState, slots};

/// 等待其他玩家的文字：`等待其他玩家 2/3`（done 是已交齐的真人数，total 是本回合需要交指令的真人数）。
pub fn wait_text(p: Progress) -> String {
    format!("等待其他玩家 {}/{}", p.done, p.total)
}

/// 倒计时的文字：有截止时间写剩余秒数，没有（本回合还没有人交指令）写“——”。
pub fn countdown_text(left: Option<u64>) -> String {
    match left {
        Some(secs) => format!("{secs}s"),
        None => "——".to_string(),
    }
}

/// 队员离场的战斗记录（`battle_leave.cause`）：超时写被踢出，掉线写退出；其他原因不写。
pub fn leave_text(name: &str, cause: &str) -> Option<String> {
    match cause {
        "timeout" => Some(format!("{name} 超时未下指令，被踢出战斗")),
        "offline" => Some(format!("{name} 掉线，退出战斗")),
        _ => None,
    }
}

/// 队员逃跑成功的战斗记录（`escape` 事件）。自己逃跑由结果窗口说明，不写这一行。
pub fn escape_text(name: &str) -> String {
    format!("{name} 逃离了战斗，已离队")
}

/// 战斗中队长离场解散队伍的文字（`disbanded` 的 `cause`，协议 10.14）；没有原因时写“队伍已解散”。
pub fn disband_text(cause: &str) -> &'static str {
    match cause {
        "battle_escape" => "队长逃跑，队伍解散",
        "battle_timeout" => "队长超时未下指令，被移出战斗，队伍解散",
        "battle_offline" => "队长掉线，队伍解散",
        _ => "队伍已解散",
    }
}

/// 战斗中队员离队的文字（`member_left` / `member_kicked` / `kicked` / `left` 带 `cause` 时）。`mine` 是离队的人是自己。
/// 没有战斗原因（`cause` 不认识或为空）返回 None，沿用原来的文字。
pub fn member_leave_text(who: &str, mine: bool, cause: &str) -> Option<String> {
    let reason = match cause {
        "battle_escape" => "逃跑离队",
        "battle_timeout" => "超时未下指令，被移出战斗",
        "battle_offline" => "掉线，离开队伍",
        _ => return None,
    };
    Some(if mine { format!("你{reason}") } else { format!("{who} {reason}") })
}

impl BattleState {
    /// 队友：真人队同场时，己方除自己和自己的宠物以外的真人人物或他们的宠物（分身不算）。
    pub fn is_mate(&self, no: u8) -> bool {
        self.party
            && slots::cell_of(no).side == self.me_side()
            && no != self.you
            && no != self.you + 5
            && self.units.get(&no).is_some_and(|u| !u.auto)
    }

    /// 队长：队员表里 `leader` 为真的人物单位号。
    pub fn is_leader(&self, no: u8) -> bool {
        self.members.iter().any(|m| m.no == no && m.leader)
    }

    /// `battle_leave`：离场的单位不再在场，进度换成离场之后的数字，写一条队伍记录。
    pub fn on_leave(&mut self, l: BattleLeave) {
        for no in &l.left {
            self.here.remove(no);
        }
        self.progress = l.progress;
        let name = if l.name.is_empty() { self.name(l.no) } else { l.name };
        if let Some(text) = leave_text(&name, &l.cause) {
            self.note_kind(Kind::Party, text);
        }
    }

    /// 本回合的结算里队员逃跑成功的，写一条队伍记录（队员的单位已经随 `left` 离场）。
    pub(super) fn note_party_escapes(&mut self, events: &[BattleEvent]) {
        for ev in events.iter().filter(|ev| ev.kind == "escape" && ev.ok == Some(true)) {
            if let Some(actor) = ev.actor.filter(|&a| self.is_mate(a)) {
                let text = escape_text(&self.name(actor));
                self.note_kind(Kind::Party, text);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wait_text_shows_done_of_total() {
        assert_eq!(wait_text(Progress { done: 2, total: 3 }), "等待其他玩家 2/3");
        assert_eq!(wait_text(Progress { done: 0, total: 2 }), "等待其他玩家 0/2");
    }

    #[test]
    fn countdown_is_dash_until_a_deadline_exists() {
        assert_eq!(countdown_text(None), "——");
        assert_eq!(countdown_text(Some(120)), "120s");
    }

    #[test]
    fn leave_text_names_the_cause() {
        assert_eq!(
            leave_text("路人乙", "timeout").unwrap(),
            "路人乙 超时未下指令，被踢出战斗"
        );
        assert_eq!(leave_text("路人乙", "offline").unwrap(), "路人乙 掉线，退出战斗");
        assert!(leave_text("路人乙", "other").is_none());
    }
}
