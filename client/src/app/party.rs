//! 分身队（STAGE-162，`docs/protocol.md` 5.57，`docs/ui.md`「社交」页）：队伍状态、`party_changed` 推送、
//! `party_clone` / `party_disband` 请求和 `/party` 命令。菜单动作流程在 `party_action.rs`，页面在 `ui/socialpage.rs`。
//!
//! 队伍只存在 `world.entry.party` 一份：进入世界、`status` 响应和 `party_changed` 推送都会换掉它，
//! 请求的回复不改状态以外的东西（推送总是先于回复到达，回复里的队伍只是兜底）。
//! 菜单、命令都先在这里查一遍“现在能不能做”：倒下、已有队伍、没有队伍、agent 操控中，
//! 都在消息栏写明原因，不发多余的请求；服务端仍是最终裁判。战斗中也能组、解散（STAGE-189）。

use serde_json::{Value, json};

use super::auto_battle::{AutoProfile, VERSION as PROFILE_VERSION};
use super::select::{Column, Item, OnEnter};
use super::session::Pending;
use super::world::SOCIAL_MENU;
use super::{App, Confirm, WorldState};
use crate::proto::{Party, PartyChanged, PartyReply};

/// 社交页唯一的栏。
pub const PARTY_COLUMN: &str = "队伍";

/// 队伍请求（`Pending::PartyOp` 携带），成功后的消息栏文字在推送里写，回复本身不再重复。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PartyOp {
    Clone,
    Disband,
}

/// 社交页的栏：没有队伍时只有“组成分身队”一项；有队伍时每个成员一项，按稳定编号记选中。
pub fn party_columns(world: &WorldState) -> Vec<Column> {
    let items = match world.party() {
        Some(p) => p
            .members
            .iter()
            .map(|m| Item {
                key: i64::from(m.index),
                usable: true,
                enter: Some(OnEnter::Open),
            })
            .collect(),
        None => vec![Item {
            key: 0,
            usable: true,
            enter: Some(OnEnter::Open),
        }],
    };
    let mut cols = vec![Column {
        name: PARTY_COLUMN,
        items,
        shown: true,
        detail: false,
        default_key: None,
    }];
    cols.push(super::party_players::request_column(world));
    cols.push(super::social_settings::settings_column());
    cols
}

impl WorldState {
    /// 当前队伍；没有队伍时为 None。
    pub fn party(&self) -> Option<&Party> {
        self.entry.party.as_ref()
    }

    /// 社交页选中的成员编号（队长 0）；没有队伍时为 None。
    pub fn selected_party_index(&self) -> Option<u8> {
        self.party()?;
        let key = super::select::selections(&party_columns(self), &self.select[SOCIAL_MENU])
            .first()
            .copied()
            .flatten()?;
        u8::try_from(key).ok()
    }

    /// 现在不能组成分身队的原因（不含 agent 操控和离线，那两条由 `observer_blocks` 统一拦）。
    /// 页面上入口灰显、命令和 Enter 写进消息栏的都是它。顺序同服务端：倒下 → 已有队伍。
    pub fn clone_blocked(&self) -> Option<&'static str> {
        if self.entry.status.hp == 0 {
            Some("角色已经倒下，不能组成分身队")
        } else if self.party().is_some() {
            Some("已经有队伍了，先解散再组")
        } else {
            None
        }
    }

    /// 现在不能解散队伍的原因（不含 agent 操控和离线）。
    pub fn disband_blocked(&self) -> Option<&'static str> {
        if self.party().is_none() {
            Some("没有队伍可解散")
        } else {
            None
        }
    }
}

/// 组队请求的数据（STAGE-195）：当前生效的自动战斗方案加上 `version`，服务端据此认方案（协议 5.57）。
pub(super) fn clone_request(profile: &AutoProfile) -> Value {
    let mut body = serde_json::to_value(profile).unwrap_or(Value::Null);
    if let Some(obj) = body.as_object_mut() {
        obj.insert("version".into(), json!(PROFILE_VERSION));
    }
    json!({ "auto_profile": body })
}

/// 服务端对组队方案的回复（STAGE-195）写进消息栏的话：`applied` 说明分身按方案出招，`ignored` 说明不认、分身走默认。
pub(super) fn profile_hint(status: &str, name: &str) -> Option<String> {
    match status {
        "applied" => Some(format!("分身会按当前自动战斗方案出招（方案：{name}）")),
        "ignored" => Some("服务端不认这份自动战斗方案，分身按默认方式出招".into()),
        _ => None,
    }
}

impl App {
    /// 推送 `party_changed`：换掉队伍，按 `reason` 在消息栏写一句。成员消失的动作流程随之作废。
    pub(super) fn on_party_changed(&mut self, data: Value) {
        let Ok(change) = serde_json::from_value::<PartyChanged>(data) else {
            self.log.error("队伍推送格式无效");
            return;
        };
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let had = w.entry.party.is_some();
        w.entry.party = change.party;
        self.after_party_update();
        match change.reason.as_str() {
            "created" => {
                if let Some(p) = self.world.as_ref().and_then(|w| w.party()) {
                    self.log.party(format!(
                        "组成分身队：你是队长，带着 {} 个分身（共 {} 人）",
                        p.clones(),
                        p.size()
                    ));
                }
            }
            "disbanded" if had => self.log.party("队伍已解散，分身消失"),
            "battle_end" => self.log.party("战斗结束，分身已恢复满血"),
            _ => {}
        }
    }

    /// 队伍换了之后收拾界面：成员没了（解散）就关掉动作流程，选中项落回有效的成员。
    pub(super) fn after_party_update(&mut self) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let stale = match (&w.party_step, w.party()) {
            (None, _) => false,
            (Some(_), None) => true,
            (Some(step), Some(p)) => p.member(step.member()).is_none(),
        };
        if stale {
            self.close_party_step();
        }
    }

    /// 请求成功。推送已经更新过队伍，这里只在回复带着队伍而推送没到时兜底。
    pub(super) fn on_party_op_ok(&mut self, op: PartyOp, data: Value) {
        if op != PartyOp::Clone {
            return;
        }
        let Ok(reply) = serde_json::from_value::<PartyReply>(data) else {
            return;
        };
        if let (Some(w), Some(party)) = (self.world.as_mut(), reply.party) {
            if w.entry.party.is_none() {
                w.entry.party = Some(party);
            }
        }
        let name = self.auto_profile().name.clone();
        if let Some(hint) = reply
            .auto_profile
            .as_deref()
            .and_then(|s| profile_hint(s, &name))
        {
            self.log.party(hint);
        }
    }

    /// `/party`：不带参数打开社交页；`clone` 组队；`disband` 解散（先确认，`disband yes` 直接解散）。
    pub(super) fn party_command(&mut self, rest: &str) {
        if let Some(text) = super::party_chat::say_text(rest) {
            self.party_say(text);
            return;
        }
        let args: Vec<&str> = rest.split_whitespace().collect();
        if self.party_players_command(&args) {
            return;
        }
        match args.as_slice() {
            [] => self.show_party_page(),
            ["clone"] => self.clone_party(),
            ["disband"] => self.ask_disband_party(),
            ["disband", "yes"] => self.disband_party(),
            _ => self.log.error(super::commands::usage_of("party")),
        }
    }

    /// 打开社交页（`/party`）。战斗中不能切页。
    fn show_party_page(&mut self) {
        if self.world.as_ref().is_some_and(|w| w.battle.is_some()) {
            self.log.error("战斗中不能切页");
            return;
        }
        self.switch_menu(SOCIAL_MENU);
    }

    /// 组成分身队：先查 agent 操控和现在能不能做，再发请求。菜单入口、`Enter`、`/party clone` 都走这里。
    pub(super) fn clone_party(&mut self) {
        if self.observer_blocks() {
            return;
        }
        if let Some(reason) = self.world.as_ref().and_then(|w| w.clone_blocked()) {
            self.log.error(reason);
            return;
        }
        let body = clone_request(self.auto_profile());
        self.send("party_clone", body, Pending::PartyOp(PartyOp::Clone));
    }

    /// 解散前先问一句，默认取消（有损失：4 个分身立即消失）。
    pub(super) fn ask_disband_party(&mut self) {
        if self.observer_blocks() {
            return;
        }
        if let Some(reason) = self.world.as_ref().and_then(|w| w.disband_blocked()) {
            self.log.error(reason);
            return;
        }
        self.ask_confirm(Confirm::PartyDisband, "解散队伍？4 个分身会立即消失");
    }

    /// 确认之后（或 `party disband yes`）真正发出解散请求。
    pub(super) fn disband_party(&mut self) {
        if self.observer_blocks() {
            return;
        }
        if let Some(reason) = self.world.as_ref().and_then(|w| w.disband_blocked()) {
            self.log.error(reason);
            return;
        }
        self.send("party_disband", json!({}), Pending::PartyOp(PartyOp::Disband));
    }

    /// 重连完成后：之前有队伍、重连后的世界里没有，说明分身随断线消失了，在消息栏说一句。
    pub(super) fn note_party_lost(&mut self, had: bool) {
        if had && self.world.as_ref().is_some_and(|w| w.party().is_none()) {
            self.log.party("分身已随断线消失，现在没有队伍");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{PartyMember, WorldEntry};

    fn party(n: u8) -> Party {
        Party {
            kind: "clone".into(),
            leader: "勇者".into(),
            members: (0..n)
                .map(|i| PartyMember {
                    index: i,
                    name: format!("成员{i}"),
                    leader: i == 0,
                    clone: i > 0,
                    ..Default::default()
                })
                .collect(),
        }
    }

    fn world(party: Option<Party>) -> WorldState {
        let mut entry = WorldEntry::default();
        entry.status.hp = 10;
        entry.party = party;
        WorldState::new(entry)
    }

    #[test]
    fn column_is_one_entry_without_a_party_and_one_row_per_member_with_one() {
        assert_eq!(party_columns(&world(None))[0].items.len(), 1);
        let cols = party_columns(&world(Some(party(5))));
        assert_eq!(
            cols[0].items.iter().map(|i| i.key).collect::<Vec<_>>(),
            vec![0, 1, 2, 3, 4]
        );
        assert!(cols[0].items.iter().all(|i| i.enter == Some(OnEnter::Open)));
    }

    #[test]
    fn clone_is_blocked_when_dead_or_with_a_party_in_that_order() {
        let mut w = world(None);
        assert_eq!(w.clone_blocked(), None);
        w.entry.status.hp = 0;
        assert_eq!(w.clone_blocked(), Some("角色已经倒下，不能组成分身队"));
        w.entry.party = Some(party(5));
        assert_eq!(
            w.clone_blocked(),
            Some("角色已经倒下，不能组成分身队"),
            "倒下先于已有队伍"
        );
        w.entry.status.hp = 10;
        assert_eq!(w.clone_blocked(), Some("已经有队伍了，先解散再组"));
    }

    #[test]
    fn disband_needs_a_party() {
        let mut w = world(None);
        assert_eq!(w.disband_blocked(), Some("没有队伍可解散"));
        w.entry.party = Some(party(5));
        assert_eq!(w.disband_blocked(), None);
    }

    #[test]
    fn selection_defaults_to_the_leader_and_follows_the_member_index() {
        let mut w = world(Some(party(5)));
        assert_eq!(w.selected_party_index(), Some(0));
        let cols = party_columns(&w);
        assert!(crate::app::select::select_key(&cols, &mut w.select[SOCIAL_MENU], 3));
        assert_eq!(w.selected_party_index(), Some(3));
        assert_eq!(world(None).selected_party_index(), None, "没有队伍时没有选中的成员");
    }

    #[test]
    fn party_helpers_count_members() {
        let p = party(5);
        assert_eq!((p.size(), p.clones()), (5, 4));
        assert!(p.member(4).is_some() && p.member(5).is_none());
    }

    #[test]
    fn clone_request_carries_the_profile_with_its_version() {
        let body = clone_request(&AutoProfile::default());
        let profile = &body["auto_profile"];
        assert_eq!(profile["version"], json!(PROFILE_VERSION));
        assert_eq!(profile["target"], json!("combo"), "方案的字段照原样带上");
        assert!(profile.get("playback").is_some());
    }

    #[test]
    fn profile_hint_says_applied_or_ignored_and_nothing_else() {
        assert_eq!(
            profile_hint("applied", "默认方案").as_deref(),
            Some("分身会按当前自动战斗方案出招（方案：默认方案）")
        );
        assert_eq!(
            profile_hint("ignored", "").as_deref(),
            Some("服务端不认这份自动战斗方案，分身按默认方式出招")
        );
        assert_eq!(profile_hint("maybe", ""), None);
    }
}
