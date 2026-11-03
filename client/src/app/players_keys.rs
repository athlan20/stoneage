//! STAGE-273：附近玩家列的动作面板（Enter 打开）、按键与 `/players` 命令。
//! STAGE-274：邀请组队、申请加入、请出队伍可用；灰掉时的原因由 `party_rules` 给出（`docs/social-players.md` 2.1、2.2）。

use super::party_players::PlayersOp;
use super::party_rules as rules;
use super::{App, SceneColumn, WorldState, observe::OBSERVER_BLOCKED};
use crate::proto::NearbyPlayer;
use crossterm::event::{KeyCode, KeyEvent};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PlayerAction {
    Detail,
    Invite,
    Trade,
    Duel,
    Apply,
    Kick,
    Cancel,
}

pub const PLAYER_ACTIONS: [PlayerAction; 7] = [
    PlayerAction::Detail,
    PlayerAction::Invite,
    PlayerAction::Trade,
    PlayerAction::Duel,
    PlayerAction::Apply,
    PlayerAction::Kick,
    PlayerAction::Cancel,
];

impl PlayerAction {
    pub fn label(self) -> &'static str {
        match self {
            Self::Detail => "查看资料",
            Self::Invite => "邀请组队",
            Self::Trade => "交易",
            Self::Duel => "决斗",
            Self::Apply => "申请加入",
            Self::Kick => "请出队伍",
            Self::Cancel => "取消",
        }
    }

    /// 灰掉时的原因；`None` 表示可用。观察方（agent 操控中）先说操控权的事。
    pub fn reason(self, w: &WorldState, target: &NearbyPlayer) -> Option<&'static str> {
        match self {
            Self::Detail | Self::Cancel => None,
            _ if w.observing() => Some(OBSERVER_BLOCKED),
            Self::Invite => rules::invite_reason(w, target),
            Self::Trade => super::trade_rules::trade_reason(w, target),
            Self::Duel => super::duel::duel_reason(w, target),
            Self::Apply => rules::apply_reason(w, target),
            Self::Kick => rules::kick_reason(w, &target.name),
        }
    }
}

impl App {
    /// 动作面板开着时吃掉所有按键（同遇敌面板）：上下选，Enter 执行，Esc 关面板。
    /// 灰掉的动作只在消息栏说明原因，不发请求，面板保持打开。
    pub(super) fn players_prompt_key(&mut self, key: KeyEvent) -> bool {
        let Some(w) = self.world.as_mut() else {
            return false;
        };
        let Some(sel) = w.scene.players.prompt else {
            return false;
        };
        let last = PLAYER_ACTIONS.len() - 1;
        let mut blocked = None;
        let mut run = None;
        match key.code {
            KeyCode::Esc => w.scene.players.prompt = None,
            KeyCode::Up => w.scene.players.prompt = Some(sel.saturating_sub(1)),
            KeyCode::Down => w.scene.players.prompt = Some((sel + 1).min(last)),
            KeyCode::Enter => {
                let action = PLAYER_ACTIONS[sel];
                match action {
                    PlayerAction::Cancel => w.scene.players.prompt = None,
                    PlayerAction::Detail => {
                        w.scene.players.prompt = None;
                        w.scene.details = true;
                        w.scene.detail_scroll = 0;
                    }
                    _ => {
                        if let Some(t) = w.scene.players.selected().cloned() {
                            match action.reason(w, &t) {
                                Some(reason) => blocked = Some(reason),
                                None => {
                                    w.scene.players.prompt = None;
                                    run = Some((action, t));
                                }
                            }
                        }
                    }
                }
            }
            _ => {}
        }
        if let Some((action, t)) = run {
            match action {
                PlayerAction::Invite => {
                    self.players_request(PlayersOp::Invite { name: t.name });
                }
                PlayerAction::Trade => {
                    self.trade_request(super::trade::TradeOp::Invite { name: t.name });
                }
                PlayerAction::Duel => self.ask_duel(&t),
                PlayerAction::Apply => {
                    self.players_request(PlayersOp::Join { name: t.name });
                }
                PlayerAction::Kick => self.ask_kick(&t.name),
                _ => {}
            }
        }
        if let Some(reason) = blocked {
            self.log.system(reason);
        }
        true
    }

    /// `/players`：把焦点切到附近玩家列并刷新（STAGE-273，设计 1.4）。查看类，观察方也能用。
    pub(super) fn players_command(&mut self) {
        if let Some(w) = self.world.as_mut() {
            w.scene.column = SceneColumn::Players;
        }
        self.request_scene_list(SceneColumn::Players);
    }
}
