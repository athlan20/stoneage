//! 确认层（STAGE-103，`docs/interaction.md`「确认」）：放生宠物、丢弃物品、跨图找 NPC 都先在消息栏问一句
//! “输入 y 确认，其他内容取消”。下一行输入是 y、yes 或 是 就执行；其他内容取消，并把这一行照常当命令处理。
//! 输入行为空时按 Esc 也取消。同一时间只有一个确认，新的问题会替换旧的。
//! 命令末尾直接带 yes（`pet drop 2 yes`、`item drop 6 yes`）时跳过询问，不经过这里。

use crossterm::event::{KeyCode, KeyEvent};

use super::App;
use crate::proto::NpcMatch;

/// 等待玩家回答的确认，以及确认后要做的事。
#[derive(Debug, Clone)]
pub enum Confirm {
    /// 放生宠物（`pet drop`）：栏位和名字。
    PetDrop { slot: u8, name: String },
    /// 丢弃物品（`item drop`）：格子、名字和数量。
    ItemDrop { slot: u8, name: String, count: Option<u32> },
    /// 丢弃石币（`gold drop`，STAGE-328）：数量。
    GoldDrop { amount: u64 },
    /// 跨图找 NPC（`npc 名字`，STAGE-84）：确认后传送过去再对话。
    NpcVisit(NpcMatch),
    /// 解散分身队（`party disband`，STAGE-162）：确认后 4 个分身立即消失。
    PartyDisband,
    /// 离开真人队（`party leave`，STAGE-274）：确认后离队。
    PartyLeave,
    /// 请出真人队的队员（`party kick`，STAGE-274）：队员名字。
    PartyKick(String),
    /// 转让队长（`party leader`，STAGE-277）：新队长的名字。
    PartyLeader(String),
    /// 宠物合成（加工、料理，STAGE-208）：宠物栏位、技能栏位与名字、背包格号（勾选顺序）及对应的物品名。
    PetMerge { slot: u8, skill: u8, skill_name: String, items: Vec<u8>, names: Vec<String> },
    /// 精工（STAGE-299，`item inlay` 与背包动作共用）：目标格、目标名字与材料格（勾选顺序）。
    Inlay { target: u8, target_name: String, materials: Vec<u8> },
    /// 向同图的人发起决斗（STAGE-307，`docs/pk.md` 5.1）：对方名字。
    Duel(String),
}

impl Confirm {
    /// 取消时消息栏写的话。
    fn cancelled(&self) -> &'static str {
        match self {
            Confirm::PetDrop { .. } => "已取消放生",
            Confirm::ItemDrop { .. } => "已取消丢弃",
            Confirm::GoldDrop { .. } => "已取消丢弃石币",
            Confirm::NpcVisit(_) => "已取消传送",
            Confirm::PartyDisband => "已取消解散",
            Confirm::PartyLeave => "已取消离队",
            Confirm::PartyKick(_) => "已取消请出",
            Confirm::PartyLeader(_) => "已取消转让队长",
            Confirm::PetMerge { .. } => "已取消合成",
            Confirm::Inlay { .. } => "已取消精工",
            Confirm::Duel(_) => "已取消决斗",
        }
    }

    /// 答 y 时消息栏回显的等价命令：原命令末尾带 yes（STAGE-104，`docs/interaction.md`「快捷键与命令」）。
    /// 找 NPC 没有带 yes 的写法，返回 None，照常回显玩家打的 y。
    fn with_yes(&self) -> Option<String> {
        match self {
            Confirm::PetDrop { slot, .. } => Some(format!("pet drop {slot} yes")),
            Confirm::ItemDrop {
                slot, count: Some(n), ..
            } => Some(format!("item drop {slot} {n} yes")),
            Confirm::ItemDrop { slot, count: None, .. } => Some(format!("item drop {slot} yes")),
            Confirm::GoldDrop { amount } => Some(format!("gold drop {amount} yes")),
            Confirm::NpcVisit(_) => None,
            Confirm::PartyDisband => Some("party disband yes".to_string()),
            Confirm::PartyLeave => Some("party leave yes".to_string()),
            Confirm::PartyKick(name) => Some(format!("party kick {name} yes")),
            Confirm::PartyLeader(name) => Some(format!("party leader {name} yes")),
            Confirm::Duel(name) => Some(format!("duel {name} yes")),
            Confirm::PetMerge { .. } => None,
            Confirm::Inlay { target, materials, .. } => {
                let mats: String = materials.iter().map(|m| format!(" {m}")).collect();
                Some(format!("item inlay {target}{mats} yes"))
            }
        }
    }
}

/// 确认的回答：y、yes 或 是（不区分大小写）。
pub fn is_yes(line: &str) -> bool {
    matches!(line.to_ascii_lowercase().as_str(), "y" | "yes" | "是")
}

impl App {
    /// 问玩家要不要执行：记下确认，消息栏写“{text}。输入 y 确认，其他内容取消”（text 已有问号或句号时不再补句号）。
    pub(super) fn ask_confirm(&mut self, confirm: Confirm, text: &str) {
        if let Some(world) = self.world.as_mut() {
            world.confirm_native=!world.nav.is(&super::Layer::Command);world.confirm_choice=false;world.confirm_text=text.into();world.confirm_revision=(world.inventory_epoch,world.pets_epoch);
            world.confirm = Some(confirm);
        }
        if self.world.as_ref().is_some_and(|w|w.confirm_native){self.log.system(format!("{text}（默认取消）"));return;}
        let stop = if text.ends_with(['？', '?', '。']) { "" } else { "。" };
        self.log.system(format!("{text}{stop}输入 y 确认，其他内容取消"));
    }

    /// 确认层的 Esc（优先级高于弹窗和战斗）：输入行为空时取消确认，返回 true 表示已处理。
    /// 输入行有字时 Esc 只清空输入行，交给输入行处理（返回 false）。
    pub(super) fn confirm_layer_key(&mut self, key: KeyEvent) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        if world.confirm.is_some() && world.confirm_native {
            match key.code {
                KeyCode::Left|KeyCode::Right|KeyCode::Tab|KeyCode::BackTab=>world.confirm_choice=!world.confirm_choice,
                KeyCode::Char('/')=>world.nav.push(super::Layer::Command),
                KeyCode::Enter if world.confirm_choice=>{self.answer_confirm("y");}
                KeyCode::Esc|KeyCode::Enter=>{if let Some(c)=world.confirm.take(){self.log.system(c.cancelled());}world.browse.quest_visit=None;}
                _=>{}
            }return true;
        }
        if key.code != KeyCode::Esc || !world.input.is_empty() {
            return false;
        }
        let Some(confirm) = world.confirm.take() else {
            return false;
        };
        self.log.system(confirm.cancelled());
        true
    }

    /// 放生确认是否在等（STAGE-256：这时宠物动作面板留着，Esc 取消后光标还在「放生」）。
    pub(super) fn drop_confirm_pending(&self) -> bool {
        self.world
            .as_ref()
            .and_then(|w| w.confirm.as_ref())
            .is_some_and(|c| matches!(c, Confirm::PetDrop { .. }))
    }

    /// 放生确认答 y 后收掉宠物动作面板（STAGE-256）：动作流程和它的导航层一起去掉。
    fn close_pet_action_after_drop(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.pet_step = None;
            world.nav.close_action();
        }
    }

    /// 答 y 时要回显的等价命令（STAGE-104）。正在等确认、且这一行是 y、又有带 yes 的写法时才有值。
    pub(super) fn confirm_echo(&self, line: &str) -> Option<String> {
        let confirm = self.world.as_ref()?.confirm.as_ref()?;
        if is_yes(line) { confirm.with_yes() } else { None }
    }

    /// 等待确认时，下一行输入先经过这里：是 y 就执行，返回 true；其他内容取消，返回 false，
    /// 让这一行照常当命令处理。
    pub(super) fn answer_confirm(&mut self, line: &str) -> bool {
        let Some(confirm) = self.world.as_mut().and_then(|w| w.confirm.take()) else {
            return false;
        };
        if !is_yes(line) {
            if let Some(w)=self.world.as_mut(){w.browse.quest_visit=None;}
            self.log.system(confirm.cancelled());
            return false;
        }
        // STAGE-256：放生答 y 时才收掉宠物动作面板（确认期间面板一直开着）。
        if matches!(confirm, Confirm::PetDrop { .. }) {
            self.close_pet_action_after_drop();
        }
        // 观察模式下确认也是操作（STAGE-104）：拦下，确认作废。
        if self.observer_blocks() {
            return true;
        }
        let stale=self.world.as_ref().is_some_and(|w|match &confirm {
            Confirm::ItemDrop{..}|Confirm::Inlay{..}=>w.confirm_revision.0!=w.inventory_epoch,
            Confirm::PetDrop{..}=>w.confirm_revision.1!=w.pets_epoch,
            Confirm::PetMerge{..}=>w.confirm_revision.0!=w.inventory_epoch||w.confirm_revision.1!=w.pets_epoch,
            _=>false,
        });
        if stale{self.log.error("对象资料已经变化，旧确认已取消，请重新选择");return true;}
        match confirm {
            Confirm::PetDrop { slot, name } => self.send_pet_drop(slot, name),
            Confirm::ItemDrop { slot, name, count } => self.send_item_drop(slot, name, count),
            Confirm::GoldDrop { amount } => self.send_gold_drop(amount),
            Confirm::NpcVisit(npc) => self.send_npc_visit(npc),
            Confirm::PartyDisband => self.disband_party(),
            Confirm::PartyLeave => {
                self.players_request(super::party_players::PlayersOp::Leave);
            }
            Confirm::PartyKick(name) => {
                self.players_request(super::party_players::PlayersOp::Kick { name });
            }
            Confirm::PartyLeader(name) => {
                self.players_request(super::party_players::PlayersOp::Leader { name });
            }
            Confirm::Duel(name) => self.duel_challenge(name),
            Confirm::PetMerge { slot, skill, skill_name, items, names } => {
                self.send_pet_merge(slot, skill, skill_name, items, names)
            }
            Confirm::Inlay { target, target_name, materials } => self.send_inlay(target, materials, target_name),
        }
        true
    }
}

#[cfg(test)]
mod tests {
    use super::is_yes;

    #[test]
    fn accepts_y_yes_and_shi_in_any_case() {
        for ok in ["y", "Y", "yes", "YES", "Yes", "是"] {
            assert!(is_yes(ok), "{ok} 应当是确认");
        }
        for no in ["", "n", "no", "ye", "yes please", "好", "1"] {
            assert!(!is_yes(no), "{no:?} 不是确认");
        }
    }
}
