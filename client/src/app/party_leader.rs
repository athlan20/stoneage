//! STAGE-277：转让队长（`docs/protocol.md` 5.67 `party_leader`，`docs/ui.md` 队员动作）。
//! 先在消息栏问一句，默认取消；`party leader 名字 yes` 直接转。转让后的消息由 `party_changed`（leader_changed）写。

use super::party_players::PlayersOp;
use super::party_rules as rules;
use super::{App, Confirm};

impl App {
    /// 转让队长先问一句（默认取消），本地能看出的原因直接写进消息栏。
    pub(super) fn ask_leader(&mut self, name: &str) {
        if self.observer_blocks() {
            return;
        }
        if let Some(r) = self.world.as_ref().and_then(|w| rules::leader_reason(w, name)) {
            self.log.error(r);
            return;
        }
        let text = format!("把队长转让给 {name}？你会变成普通队员");
        self.ask_confirm(Confirm::PartyLeader(name.to_string()), &text);
    }

    /// 确认之后（或 `party leader 名字 yes`）真正发出转让请求。
    pub(super) fn transfer_leader(&mut self, name: &str) {
        self.players_request(PlayersOp::Leader { name: name.to_string() });
    }
}
