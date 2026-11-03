use serde_json::json;
use super::{App,commands};
use super::session::Pending;

impl App {
    /// 命令分发：查命令表（名字、别名），观察模式的拦截也由表里的“操作”类标记驱动（STAGE-26）。
    /// `echo` 为假时不回显（页面上 Enter 执行的查看命令，STAGE-113）。
    pub(super) fn run_command(&mut self, line: &str, raw: &str, echo: bool) {
        // 命令回显里不能带令牌原文以外的东西：目前命令里没有密码和令牌，直接回显。
        // 确认里答 y 时回显等价的原命令（带 yes），不回显 y（STAGE-104）。
        if echo {
            match self.confirm_echo(line) {
                Some(equivalent) => self.log.command(&equivalent),
                None => self.log.command(line),
            }
        }
        // 等待确认时，紧接着的输入先由确认流程处理（是 y 才执行；其他内容取消，并照常当命令处理，STAGE-103）。
        if self.answer_confirm(line) {
            return;
        }
        let (cmd, rest) = line
            .split_once(char::is_whitespace)
            .map_or((line, ""), |(a, b)| (a, b.trim()));
        let Some(spec) = commands::find(cmd) else {
            if commands::battle_find(cmd).is_some() {
                self.log
                    .error(format!("{cmd} 是战斗中的命令，战斗时才能用"));
            } else {
                self.log
                    .error(format!("未知命令 {cmd}；输入 help 查看全部命令"));
            }
            return;
        };
        let sub = rest.split_whitespace().next().unwrap_or("");
        if self.world.as_ref().is_some_and(|w|!w.online) && spec.blocks(sub){self.log.error("连接已断开，资料可能过期；请从系统菜单重新连接");return;}
        let observing = self.world.as_ref().is_some_and(|w| w.observing());
        if observing && spec.blocks(sub) {
            self.log.error(super::observe::OBSERVER_BLOCKED);
            return;
        }
        match spec.name {
            "help" => self.help_command(rest),
            "page" => self.page_command(rest),
            "status" => {
                self.send("status", json!({}), Pending::Status);
            }
            "leave" => {
                self.send("leave_world", json!({}), Pending::Leave);
            }
            "quit" => self.request_exit(super::ExitKind::Quit),
            "turn" => {
                self.turn_command(rest);
            }
            "warp" => {
                self.warp_command(rest);
            }
            "talk" => {
                self.talk_command(rest);
            }
            "npc" => self.npc_command(rest),
            "players" => self.players_command(),
            "choose" => {
                self.choose_command(rest);
            }
            "reply" => {
                self.reply_command(reply_arg(raw));
            }
            "addpoint" => self.addpoint_command(rest),
            "item" => self.item_command(rest),
            "gold" => self.gold_command(rest),
            // STAGE-299：精工（别名 精工）等同 item inlay。
            "inlay" => {
                let args: Vec<&str> = rest.split_whitespace().collect();
                self.inlay_command(&args);
            }
            "pet" => {
                if !self.pet_manage_command(rest) {
                    self.pet_command(rest);
                }
            }
            "magic" => self.magic_command(rest),
            // 任务、百科的命令打开或换了内容，焦点跟着落到新内容的栏上（`page_keys.rs` 的 `land_focus`）。
            "wiki" => {
                let errors=self.log.all().iter().filter(|m|m.kind==super::Kind::Error).count();
                self.wiki_command(rest);
                if errors==self.log.all().iter().filter(|m|m.kind==super::Kind::Error).count(){if let Some(w)=self.world.as_mut(){w.browse.step=None;w.nav.show_page(w.menu);}}
                self.land_focus();
            }
            "quest" => {
                let errors=self.log.all().iter().filter(|m|m.kind==super::Kind::Error).count();
                self.quest_command(rest);
                if errors==self.log.all().iter().filter(|m|m.kind==super::Kind::Error).count(){if let Some(w)=self.world.as_mut(){w.browse.step=None;w.nav.show_page(w.menu);if w.quest_guide.is_some(){w.nav.push(super::Layer::Detail);}}}
                self.land_focus();
                if let Some(w)=self.world.as_mut(){if w.quest_guide.is_some(){w.select[super::QUEST_MENU].column=1;w.select[super::QUEST_MENU].pinned=true;}}
            }
            "party" => self.party_command(rest),
            "trade" => self.trade_command(rest),
            "duel" => self.duel_command(rest),
            "battle" => {
                self.send("battle", json!({}), Pending::BattleQuery);
            }
            "encounter" => {
                self.encounter_command(rest);
            }
            "auto-battle" => {
                self.auto_battle_command(rest);
            }
            "battle-log" => {
                self.battle_log_command(rest);
            }
            "take" => {
                self.send("take_control", json!({}), Pending::Take);
            }
            "release" => {
                self.send("release_control", json!({}), Pending::Release);
            }
            "kick-agent" => {
                self.send("kick_agent", json!({}), Pending::KickAgent);
            }
            "say" => {
                self.chat_command(rest, crate::proto::ChatChannel::Map);
            }
            "world" => {
                self.chat_command(rest, crate::proto::ChatChannel::World);
            }
            "party-say" => self.party_say(rest),
            "ag" => {
                self.say_to_agent(rest);
            }
            "token" => {
                self.token_command(rest);
            }
            "agent" => {
                self.agent_command(rest);
            }
            "revoke" => {
                self.revoke_command(rest);
            }
            _ => unreachable!("命令表里的命令都要有分发：{}", spec.name),
        }
    }
}

/// `reply` 的文字参数（原文）：去掉命令名后分隔的那个空白，其余原样保留（行尾空格也保留，暗号要逐字相同）。
/// `raw` 是提交时只去掉行首空白的输入行；没有分隔空白时是空串。
pub fn reply_arg(raw: &str) -> &str {
    raw.split_once(char::is_whitespace).map_or("", |(_, b)| b)
}

#[cfg(test)]
mod tests {
    use super::reply_arg;

    #[test]
    fn reply_keeps_trailing_space_for_exact_keyword() {
        assert_eq!(reply_arg("reply 近藏 "), "近藏 ");
        assert_eq!(reply_arg("reply 近藏"), "近藏");
        assert_eq!(reply_arg("reply  近藏"), " 近藏");
        assert_eq!(reply_arg("reply"), "");
    }
}
