//! STAGE-186：给分身和分身的宠物指定攻击目标（`docs/battle-ui.md`「分身队」，`docs/protocol.md` 10.4 / 10.5）。
//! 分身仍由服务端自动出招，指定目标是可选的：回合里按 `f` 打开「分身」面板，给每个能行动的分身
//! （`clone:N`）或分身的宠物（`clone_pet:N`）各选一个敌方目标；不选、来不及选的，STAGE-312 起服务端
//! 按合击顺序打第一个（面板里写成“默认→ 敌N”）。
//! 分身指令不进 `needs`，队长（和宠物）交齐指令就结算，所以要在队长最后一条指令之前发出。

use serde_json::json;

use super::command::{Act, parse_target};
use super::issue::check_target;
use super::slots::{self, Cell};
use super::{BattleState, Phase, Pick};
use crate::app::App;
use crate::app::session::Pending;
use crate::proto::TargetType;

/// 面板里的一行：哪个分身、现在指定了谁。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CloneRow {
    /// 面板里的编号：分身 1–4，分身的宠物 5–8，数字键直选。
    pub key: u8,
    /// 单位号（分身 N 占 N 号，它的宠物占 5+N 号）。
    pub who: u8,
    pub name: String,
    /// 本回合已经指定的目标单位号；没指定为 `None`（服务端按合击顺序打第一个）。
    pub target: Option<u8>,
    /// 没指定时服务端默认会打的敌人（合击顺序的第一个）；没有敌人为 `None`。
    pub default_target: Option<u8>,
}

/// `clone:N` / `clone_pet:N` → 单位号。
pub fn unit_of(actor: &str) -> Option<u8> {
    let (rest, pet) = match actor.strip_prefix("clone_pet:") {
        Some(rest) => (rest, true),
        None => (actor.strip_prefix("clone:")?, false),
    };
    let n: u8 = rest.parse().ok().filter(|n| (1..=4).contains(n))?;
    Some(if pet { 5 + n } else { n })
}

/// 单位号 → 协议的 `actor`。
pub fn actor_of(who: u8) -> String {
    if who >= 5 {
        format!("clone_pet:{}", who - 5)
    } else {
        format!("clone:{who}")
    }
}

/// 面板里的编号（分身 1–4，宠物 5–8）→ 单位号。
pub fn who_of_key(key: u8) -> Option<u8> {
    match key {
        1..=4 => Some(key),
        5..=8 => Some(key + 1),
        _ => None,
    }
}

/// 单位号 → 面板里的编号。
pub fn key_of(who: u8) -> u8 {
    if who >= 5 { who - 1 } else { who }
}

impl BattleState {
    /// 这回合有没有可以指定目标的分身。
    pub fn has_optional(&self) -> bool {
        !self.optional.is_empty()
    }

    /// 面板的行：服务端这回合列出的分身（`optional`），按服务端给的顺序。
    pub fn clone_rows(&self) -> Vec<CloneRow> {
        let default_target = super::auto::enemy_order(self).first().copied();
        self.optional
            .iter()
            .filter_map(|a| unit_of(a))
            .map(|who| CloneRow {
                key: key_of(who),
                who,
                name: if who >= 5 {
                    format!("{}（分身{}）", self.name(who), who - 5)
                } else {
                    self.name(who)
                },
                target: self.clone_targets.get(&who).copied(),
                default_target,
            })
            .collect()
    }

    /// 分身指令被接受：记下目标，回到面板继续选下一个。
    pub fn on_clone_accepted(&mut self, who: u8, target: u8) {
        self.submitting = false;
        self.clone_targets.insert(who, target);
        let text = format!("{} → {}", self.name(who), slots::short_label(self.me_side(), target));
        self.note(text);
        if matches!(self.pick, Pick::CloneAim { .. }) {
            self.pick = Pick::Clones;
        }
    }

    /// 分身指令被拒绝：错误在消息栏里，留在选目标，玩家可以改选。
    pub fn on_clone_rejected(&mut self) {
        self.submitting = false;
    }

    /// 面板上下移动高亮（到头不循环）。
    pub(super) fn clone_move(&mut self, down: bool) {
        let last = self.clone_rows().len().saturating_sub(1);
        self.clone_focus = if down {
            (self.clone_focus + 1).min(last)
        } else {
            self.clone_focus.saturating_sub(1)
        };
    }
}

/// 文字命令 `clone <1-4|p1-p4> <目标>`（简写 `f`）：`2` 是分身 2，`p2` 是分身 2 的宠物；目标写法同 `attack`。
pub fn parse(line: &str) -> Option<Result<(u8, (Option<bool>, u8)), String>> {
    let mut words = line.split_whitespace();
    if !matches!(words.next()?, "clone" | "f") {
        return None;
    }
    const USAGE: &str =
        "用法：clone <1-4|p1-p4> <目标>，例如 clone 2 e3（分身 2 打敌方 3 号）、clone p2 e3（分身 2 的宠物）";
    let who = words.next().and_then(|w| {
        let w = w.to_ascii_lowercase();
        let (pet, digits) = w.strip_prefix('p').map_or((false, w.as_str()), |d| (true, d));
        let n: u8 = digits.parse().ok().filter(|n| (1..=4).contains(n))?;
        Some(if pet { 5 + n } else { n })
    });
    Some(match (who, words.next()) {
        (Some(who), Some(target)) => parse_target(target).map(|t| (who, t)),
        _ => Err(USAGE.to_string()),
    })
}

impl App {
    /// `f`：打开分身面板。这回合没有能指定的分身时说明原因。
    pub(super) fn open_clones(&mut self) {
        let Some(b) = self.battle_mut() else {
            return;
        };
        if !b.has_optional() {
            self.log.error("这回合没有可以指定目标的分身");
            return;
        }
        b.clone_focus = 0;
        b.pick = Pick::Clones;
    }

    /// 面板里 Enter：给高亮的这一行选目标。
    pub(super) fn clones_enter(&mut self) {
        let Some(b) = self.battle_ref() else {
            return;
        };
        if let Some(row) = b.clone_rows().get(b.clone_focus) {
            let who = row.who;
            self.start_clone_aim(who);
        }
    }

    /// 面板里的数字键：1–4 分身，5–8 分身的宠物。
    pub(super) fn clones_digit(&mut self, number: u8) {
        let Some(who) = who_of_key(number) else {
            self.log.error("分身编号是 1–4，宠物是 5–8");
            return;
        };
        let listed = self
            .battle_ref()
            .is_some_and(|b| b.clone_rows().iter().any(|r| r.who == who));
        if listed {
            self.start_clone_aim(who);
        } else {
            self.log.error(format!(
                "{} 这回合不能行动",
                self.battle_ref().map_or_else(String::new, |b| b.name(who))
            ));
        }
    }

    fn start_clone_aim(&mut self, who: u8) {
        if let Some(b) = self.battle_mut() {
            b.cursor = b.default_cursor(TargetType::Enemy);
            b.pick = Pick::CloneAim { who };
        }
    }

    /// 在格子上确认分身的目标：只能选敌方活着的单位，不合法的留在选目标里说明原因。
    pub(super) fn confirm_clone_cell(&mut self, who: u8, cell: Cell) {
        let no = slots::no_at(cell);
        let check = match self.battle_ref() {
            Some(b) => check_target(b, TargetType::Enemy, Act::Attack, no),
            None => return,
        };
        match check {
            Ok(()) => {
                self.send_clone(who, no);
            }
            Err(msg) => self.log.error(msg),
        }
    }

    /// 文字命令 `clone 2 e3`：只在下指令的阶段可用（队长交齐就结算，之后再发就晚了）。
    pub(super) fn run_clone_typed(&mut self, who: u8, target: (Option<bool>, u8)) {
        let Some(b) = self.battle_ref() else {
            return;
        };
        if !matches!(b.phase, Phase::Input(_)) {
            self.log.error("现在不用下指令");
            return;
        }
        if !b.clone_rows().iter().any(|r| r.who == who) {
            let text = format!("{} 这回合不能指定目标", b.name(who));
            self.log.error(text);
            return;
        }
        let (side, number) = target;
        if side == Some(true) {
            self.log.error("分身只能攻击敌方，目标写 e3 或直接写数字");
            return;
        }
        let Some(cell) = slots::cell_of_number(slots::other_side(b.me_side()), number) else {
            return;
        };
        self.confirm_clone_cell(who, cell);
    }

    /// 发出分身的攻击目标；上一条还没有回复时不再发。返回是否发出去了。
    pub(super) fn send_clone(&mut self, who: u8, target: u8) -> bool {
        if self.observer_blocks() {
            return false;
        }
        let Some(b) = self.battle_mut() else {
            return false;
        };
        if b.submitting {
            return false;
        }
        let turn = b.turn;
        b.submitting = true;
        let data = json!({"turn": turn, "actor": actor_of(who), "action": "attack", "target": target});
        self.send("battle_command", data, Pending::BattleClone { turn, who, target })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn actor_names_round_trip() {
        for who in [1, 2, 3, 4, 6, 7, 8, 9] {
            assert_eq!(unit_of(&actor_of(who)), Some(who));
            assert_eq!(who_of_key(key_of(who)), Some(who));
        }
        assert_eq!(actor_of(7), "clone_pet:2");
        for bad in ["clone:0", "clone:5", "clone_pet:9", "clone:", "player", "clone:01x"] {
            assert_eq!(unit_of(bad), None, "{bad}");
        }
    }

    #[test]
    fn typed_command_names_the_clone_and_the_target() {
        assert_eq!(parse("clone 2 e3"), Some(Ok((2, (Some(false), 3)))));
        assert_eq!(parse("f p4 5"), Some(Ok((9, (None, 5)))));
        assert!(parse("clone 5 e3").unwrap().is_err());
        assert!(parse("clone 2").unwrap().is_err());
        assert!(parse("attack e3").is_none());
    }
}
