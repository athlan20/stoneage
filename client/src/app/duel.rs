//! STAGE-307：决斗的客户端入口（`docs/pk.md` 5.1、5.2，`docs/protocol.md` 5.74）：附近玩家的动作「决斗」、
//! 命令 `duel 名字` / `duel on|off`（别名「决斗」）。开关就是同意：对方开着「接受决斗」就直接开战，没有答复步骤。

use serde_json::json;

use super::confirm::Confirm;
use super::party_rules;
use super::session::Pending;
use super::social_settings::SocialField;
use super::{App, WorldState};
use crate::proto::NearbyPlayer;

/// 能不能向这个人发起决斗（灰掉时的原因）：自己在别人的队里先说（只有队长能发起），再看对方。
pub fn duel_reason(w: &WorldState, target: &NearbyPlayer) -> Option<&'static str> {
    if party_rules::follower_reason(w).is_some() {
        return Some("你在队伍里，只有队长能发起");
    }
    target_reason(target)
}

/// 只看对方（与 `docs/protocol.md` 5.74 的校验顺序一致：先开关，再战斗）。对方是队员时开关看队长，
/// 列表里只有他自己的开关，这里不拦，交给服务端判断（`TARGET_REFUSES`）。
fn target_reason(target: &NearbyPlayer) -> Option<&'static str> {
    let is_member =
        target.party.as_ref().and_then(|p| p["leader"].as_str()).is_some_and(|leader| leader != target.name);
    if !target.duel && !is_member {
        return Some("对方没有接受决斗");
    }
    target.in_battle.then_some("对方在战斗中")
}

/// 确认框的问句（动作面板和 `duel 名字` 共用）：写清对方的等级和单独或队伍，以及输赢的代价。
fn question(t: &NearbyPlayer) -> String {
    let side = if t.party.is_some() { "队伍" } else { "单独" };
    format!("向 {}（Lv{}，{side}）发起决斗？不掉经验和物品；倒下丢一成决斗点数，逃跑丢两成。", t.name, t.level)
}

impl App {
    /// 动作面板的「决斗」（能不能打已在面板里查过）：弹确认，默认取消。
    pub(super) fn ask_duel(&mut self, target: &NearbyPlayer) {
        self.ask_confirm(Confirm::Duel(target.name.clone()), &question(target));
    }

    /// `duel 名字 [yes]`：先按列表预检，再弹确认；末尾 yes 不问。名字不在附近列表里时不预检，交给服务端。
    fn duel_target(&mut self, name: String, yes: bool) {
        let found = self.world.as_ref().and_then(|w| w.scene.players.list.iter().find(|p| p.name == name).cloned());
        if let (Some(w), Some(t)) = (self.world.as_ref(), found.as_ref())
            && let Some(reason) = duel_reason(w, t)
        {
            self.log.error(reason);
            return;
        }
        if yes {
            self.duel_challenge(name);
            return;
        }
        let text = match found.as_ref() {
            Some(t) => question(t),
            None => format!("向 {name} 发起决斗？不掉经验和物品；倒下丢一成决斗点数，逃跑丢两成。"),
        };
        self.ask_confirm(Confirm::Duel(name), &text);
    }

    /// 发出挑战（确认后，或 `duel 名字 yes`）。开战由 `battle_start` 推送接管，响应只回 battle_id。
    pub(super) fn duel_challenge(&mut self, name: String) {
        self.send("duel_challenge", json!({ "name": name }), Pending::DuelChallenge);
    }

    /// `duel` 命令（STAGE-307）：`duel 名字 [yes]` 发起，`duel on|off` 打开或关闭接受决斗。
    pub(super) fn duel_command(&mut self, rest: &str) {
        let mut args: Vec<&str> = rest.split_whitespace().collect();
        let yes = args.last() == Some(&"yes");
        if yes {
            args.pop();
        }
        match args.as_slice() {
            [] => self.log.error("用法：duel 名字 | duel on|off（接受决斗的开关）"),
            ["on"] => self.social_set(SocialField::Duel, true),
            ["off"] => self.social_set(SocialField::Duel, false),
            names => self.duel_target(names.join(" "), yes),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn player(name: &str, duel: bool, in_battle: bool, party: Option<serde_json::Value>) -> NearbyPlayer {
        NearbyPlayer { name: name.into(), level: 30, duel, in_battle, party, ..Default::default() }
    }

    #[test]
    fn target_not_accepting_is_refused() {
        assert_eq!(target_reason(&player("阿牛", false, false, None)), Some("对方没有接受决斗"));
        assert_eq!(target_reason(&player("阿牛", true, false, None)), None);
    }

    #[test]
    fn refusal_is_checked_before_battle() {
        assert_eq!(target_reason(&player("阿牛", true, true, None)), Some("对方在战斗中"));
        assert_eq!(target_reason(&player("阿牛", false, true, None)), Some("对方没有接受决斗"));
    }

    #[test]
    fn party_member_defers_to_leader_on_server() {
        let member = Some(json!({"leader": "小明", "size": 2, "max": 5}));
        assert_eq!(target_reason(&player("阿牛", false, false, member)), None);
        let leader = Some(json!({"leader": "阿牛", "size": 2, "max": 5}));
        assert_eq!(target_reason(&player("阿牛", false, false, leader)), Some("对方没有接受决斗"));
    }

    #[test]
    fn question_names_side_and_cost() {
        let t = player("阿牛", true, false, None);
        assert_eq!(question(&t), "向 阿牛（Lv30，单独）发起决斗？不掉经验和物品；倒下丢一成决斗点数，逃跑丢两成。");
    }
}
