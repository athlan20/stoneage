//! STAGE-140：旧视图只读；全新认证和完整刷新成功后才能恢复操作。
use super::{App, LoginFocus, Screen, WorldState, session::Pending};
use serde_json::json;

pub struct Reconnect {
    pub account: String,
    pub name: String,
    pub old: Option<Box<WorldState>>,
    pub remaining: u8,
}
impl Reconnect {
    pub fn refreshing(&self) -> bool {
        self.old.is_some()
    }
}
impl Pending {
    pub(super) fn refresh_bit(&self) -> u8 {
        match self {
            Self::Places => 1,
            Self::Maps => 2,
            Self::Inventory => 4,
            Self::Pets => 8,
            Self::Quests => 16,
            Self::Npcs(_) => 32,
            Self::Exits(_) => 128,
            Self::BattleQuery => 64,
            _ => 0,
        }
    }
    pub(super) fn uncertain(&self) -> bool {
        matches!(
            self,
            Self::ItemOp(_)
            | Self::GoldDrop { .. }
                | Self::ItemInlay(_)
                | Self::AutoPost(super::battle::auto_post::PostOp::Eat { .. } | super::battle::auto_post::PostOp::Drop { .. })
                | Self::PetOp(_)
                | Self::PartyOp(_)
                | Self::PartyPlayers(_)
                | Self::Trade(_)
                | Self::SocialSettings { .. }
                | Self::AddPoint { .. }
                | Self::UseExit
                | Self::Warp(_)
                | Self::NpcWarp { .. }
                | Self::Talk
                | Self::NpcChoose
                | Self::NpcReply
                | Self::BattleCommand { .. }
                | Self::BattleClone { .. }
                | Self::Encounter { set: Some(_) }
                | Self::TokenCreate
                | Self::TokenRevoke
                | Self::Take
                | Self::Release
                | Self::KickAgent
                | Self::Create
                | Self::Delete
                | Self::Enter
                | Self::Leave
        )
    }
}
impl WorldState {
    pub(super) fn readonly(&mut self) {
        self.online = false;
        self.scene.epoch+=1;
        self.scene.using_exit=false;
        self.scene.refresh_due=None;
        self.auto_battle = false;
        self.agent.fresh = None;
        self.npc_window = None;
        self.dialog = Default::default();
        self.confirm = None;
        self.bag_step = None;
        self.pet_step = None;
        self.char_step = None;
        self.party_step = None;
        self.scene_step = None;
        self.browse.step = None;
        self.browse.quest_visit = None;
        self.system = Default::default();
        self.input.clear();
        while self.nav.top().is_modal() {
            self.nav.pop();
        }
        if let Some(b) = self.battle.as_mut() {
            b.submitting = false;
            b.prepared_row = None;
            b.pick = super::battle::Pick::Action;
            b.auto_due = None;
        }
    }
}
impl App {
    pub(super) fn offline(&mut self, reason: &str, unknown: bool) {
        let unknown=unknown || self.session.has_uncertain();
        self.drop_link();
        // STAGE-295：断线即取消交易，重连也不恢复（docs/trade.md 5.2）。
        let had_trade = self.world.as_ref().is_some_and(|w| w.trade.is_some());
        self.login.password.clear();
        self.register.password.clear();
        self.register.confirm.clear();
        self.exit_dialog = None;
        if let Some(r) = self.reconnect.as_mut() {
            if let Some(old) = r.old.take() {
                self.world = Some(*old);
            }
            r.remaining = 0;
        }
        if let Some(w) = self.world.as_mut() {
            w.trade = None;
            w.readonly();
            self.screen = Screen::World;
        } else if !matches!(self.screen, Screen::Login | Screen::Register) {
            self.screen = Screen::Login;
        }
        if had_trade {
            self.log.system("连接断开，交易已取消");
        }
        self.log.error(reason);
        if unknown {
            self.log
                .error("部分已发送操作的结果未知；不会自动重试，请重新登录后查看实际结果");
        }
    }
    pub(super) fn begin_reconnect(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let account = self.login.account.text().to_string();
        let name = w.entry.character.name.clone();
        self.offline("正在重新登录；Esc 返回只读页面", false);
        self.reconnect = Some(Reconnect {
            account,
            name,
            old: None,
            remaining: 0,
        });
        self.login.focus = LoginFocus::Password;
        self.login.password.clear();
        self.screen = Screen::Login;
    }
    pub(super) fn cancel_reconnect(&mut self) -> bool {
        if self.reconnect.is_none() {
            return false;
        }
        let account=self.reconnect.as_ref().unwrap().account.clone();
        self.offline("已返回离线只读页面", false);
        self.login.account.set(&account);
        self.reconnect = None;
        true
    }
    pub(super) fn reconnect_characters(&mut self) -> bool {
        let Some(r) = self.reconnect.as_ref() else {
            return false;
        };
        let slot = self
            .chars
            .list
            .characters
            .iter()
            .find(|c| c.name == r.name)
            .map(|c| c.slot);
        if let Some(slot) = slot {
            self.chars.selected = usize::from(slot);
            self.send("enter_world", json!({"slot":slot}), Pending::Enter);
        } else {
            self.log
                .error("原角色已不存在；Esc 返回只读资料，或选择其他角色");
        }
        true
    }
    pub(super) fn stage_reconnect(&mut self, name: &str, battle: bool) -> bool {
        let Some(r) = self.reconnect.as_mut() else {
            return false;
        };
        if r.name != name || r.account != self.login.account.text() {
            self.reconnect = None;
            self.world = None;
            return false;
        }
        r.old = self.world.take().map(Box::new);
        r.remaining = 191 | if battle { 64 } else { 0 };
        true
    }
    pub(super) fn refreshed(&mut self, bit: u8) {
        if bit == 0 {
            return;
        }
        let Some(r) = self.reconnect.as_mut().filter(|r| r.refreshing()) else {
            return;
        };
        r.remaining &= !bit;
        if r.remaining != 0 {
            return;
        }
        let mut r = self.reconnect.take().unwrap();
        let had_party = r.old.as_ref().is_some_and(|old| old.entry.party.is_some());
        if let (Some(old), Some(w)) = (r.old.take(), self.world.as_mut()) {
            w.online = true;
            w.wiki = old.wiki;
            w.msg_view = old.msg_view;
            w.select = old.select;
            w.quest_filter = old.quest_filter;
            w.quest_scroll = old.quest_scroll;
            w.bag_scroll = old.bag_scroll;
            if w.battle.is_none() {
                w.menu = old.menu;
                w.nav = old.nav;
                while w.nav.top().is_modal() {
                    w.nav.pop();
                }
            } else {
                w.battle_return = Some(old.nav);
            }
        }
        self.screen = Screen::World;
        self.log
            .system("重新登录与资料刷新完成，已恢复操作；自动战斗保持关闭");
        self.note_party_lost(had_party);
        if self
            .world
            .as_ref()
            .and_then(|w| w.battle.as_ref())
            .is_some_and(|b| b.phase == super::battle::Phase::Replay)
        {
            self.after_replay();
        }
    }
    pub(super) fn allow_send(&mut self, ty: &str, pending: &Pending) -> bool {
        if self.world.as_ref().is_none_or(|w| w.online) {
            return true;
        }
        if matches!(
            ty,
            "hello"
                | "login"
                | "register"
                | "characters"
                | "enter_world"
                | "create_character"
                | "delete_character"
                | "ping"
        ) {
            return true;
        }
        if self.reconnect.as_ref().is_some_and(|r| r.refreshing()) && pending.refresh_bit() != 0 {
            return true;
        }
        self.log
            .error("连接已断开，当前资料可能过期；请从系统菜单重新连接");
        false
    }
}
