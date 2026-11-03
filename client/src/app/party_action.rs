//! 社交页的对象动作（STAGE-162，`docs/cli-ui-redesign.md`「对象、动作、参数」）：队伍表里选中一个成员，
//! `Enter` 打开动作菜单（查看详情、解散队伍）。「查看详情」压一层只读的 `Layer::Detail`，`Esc` 回动作菜单；
//! 「解散队伍」有损失，进入默认取消的确认。动作调用 `party.rs` 里和 `/party` 命令同一批函数。
//!
//! 和宠物、背包一样，动作流程只在导航栈压一层 `Layer::Action`，详情再压一层 `Layer::Detail`；
//! 对分身不出现“私聊、交易、邀请”这类动作（它们不是真人）。

use crossterm::event::{KeyCode, KeyEvent};

use crate::proto::PartyMember;

use super::party_rules as rules;
use super::world::WorldState;
use super::{App, Layer};

/// 动作流程当前停在哪一步。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PartyStep {
    /// 动作列表：`member` 是成员稳定编号，`index` 是选中的动作。
    Actions { member: u8, index: usize },
    /// 成员详情（只读）：`scroll` 是顶部隐藏的行数。
    Detail { member: u8, scroll: usize },
}

impl PartyStep {
    pub fn member(&self) -> u8 {
        match self {
            PartyStep::Actions { member, .. } | PartyStep::Detail { member, .. } => *member,
        }
    }
}

/// 社交页成员动作菜单能做的事。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum PartyAct {
    Detail,
    Disband,
    Leave,
    Kick,
    Leader,
}

impl PartyAct {
    pub fn label(self) -> &'static str {
        match self {
            PartyAct::Detail => "查看详情",
            PartyAct::Disband => "解散队伍",
            PartyAct::Leave => "离开队伍",
            PartyAct::Kick => "请出队伍",
            PartyAct::Leader => "转让队长",
        }
    }
}

/// 选中成员时的动作菜单（顺序即显示顺序）：查看详情；中间一项按身份（分身队与队长自己是解散，队员自己是离开，
/// 队长对队员是请出）；转让队长永远在最后，不能用时灰显并写原因（`docs/ui.md` 队员动作）。
pub fn menu_for(w: &WorldState, m: &PartyMember) -> Vec<PartyAct> {
    let Some(p) = w.party() else {
        return Vec::new();
    };
    let me = w.entry.character.name.as_str();
    let mut acts = vec![PartyAct::Detail];
    if p.kind == "clone" {
        acts.push(PartyAct::Disband);
    } else if m.name == me {
        acts.push(if p.leader == me { PartyAct::Disband } else { PartyAct::Leave });
    } else if p.leader == me {
        acts.push(PartyAct::Kick);
    }
    acts.push(PartyAct::Leader);
    acts
}

/// 动作不能做的原因（保留显示并解释）：离线、agent 操控中先于队伍本身的限制。看详情不受限制。
pub fn disabled_reason(act: PartyAct, world: &WorldState, m: &PartyMember) -> Option<&'static str> {
    if act == PartyAct::Detail {
        return None;
    }
    if !world.online {
        return Some("连接已断开");
    }
    if world.observing() {
        return Some("agent 正在操控，输入 take 收回控制权");
    }
    match act {
        PartyAct::Detail => None,
        PartyAct::Disband => world.disband_blocked(),
        PartyAct::Leave => rules::leave_reason(world),
        PartyAct::Kick => rules::kick_reason(world, &m.name),
        PartyAct::Leader => rules::leader_reason(world, &m.name),
    }
}

/// 某个成员的动作菜单和每一项的灰显原因（界面和按键共用）。成员不在队里时为空。
pub fn party_menu(w: &WorldState, member: u8) -> Vec<(PartyAct, Option<&'static str>)> {
    let Some(m) = w.party().and_then(|p| p.member(member)) else {
        return Vec::new();
    };
    menu_for(w, m)
        .into_iter()
        .map(|act| (act, disabled_reason(act, w, m)))
        .collect()
}

/// 详情页的行：（栏名，内容）。整页滚动的行数也按它算。
pub fn detail_rows(m: &PartyMember) -> Vec<(&'static str, String)> {
    let role = if m.leader {
        "队长"
    } else {
        "分身（创建时的快照，只会普通攻击，不领奖励）"
    };
    let mut rows = vec![
        ("名字", format!("{}  {role}", m.name)),
        (
            "状态",
            format!("Lv{}  HP {}/{}  MP {}/{}", m.level, m.hp, m.max_hp, m.mp, m.max_mp),
        ),
        (
            "能力",
            format!(
                "体力 {}  腕力 {}  耐久力 {}  速度 {}",
                m.stats.vital, m.stats.strength, m.stats.toughness, m.stats.dexterity
            ),
        ),
        (
            "衍生",
            format!(
                "攻击 {}  防御 {}  敏捷 {}",
                m.derived.attack, m.derived.defense, m.derived.agility
            ),
        ),
    ];
    rows.push((
        "装备",
        if m.equipment.is_empty() {
            "无".to_string()
        } else {
            m.equipment
                .iter()
                .map(|e| format!("{}号 {}", e.slot, e.name))
                .collect::<Vec<_>>()
                .join("  ")
        },
    ));
    rows.push((
        "精灵",
        if m.magic.is_empty() {
            "无".to_string()
        } else {
            m.magic
                .iter()
                .map(|g| format!("{}号 {}", g.slot, g.name))
                .collect::<Vec<_>>()
                .join("  ")
        },
    ));
    rows.push((
        "宠物",
        match &m.pet {
            None => "无出战宠物".to_string(),
            Some(p) => {
                let skills = if p.skills.is_empty() {
                    "无技能".to_string()
                } else {
                    p.skills.join("、")
                };
                format!("{} Lv{}  HP {}/{}  技能 {skills}", p.name, p.level, p.hp, p.max_hp)
            }
        },
    ));
    rows.push((
        "位置",
        format!("{} ({},{})", m.position.map_name, m.position.x, m.position.y),
    ));
    rows
}

impl App {
    /// 社交页 `Enter`：没有队伍就直接组成分身队（无损失，不二次确认）；有队伍打开选中成员的动作菜单。
    pub(super) fn party_open(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        if world.party().is_none() {
            self.clone_party();
            return true;
        }
        let Some(member) = world.selected_party_index() else {
            return false;
        };
        if let Some(world) = self.world.as_mut() {
            world.party_step = Some(PartyStep::Actions { member, index: 0 });
            world.nav.push(Layer::Action);
        }
        true
    }

    /// 动作流程里的按键。返回 true 表示已处理。
    pub(super) fn party_action_key(&mut self, key: KeyEvent) -> bool {
        let Some(step) = self.world.as_ref().and_then(|w| w.party_step.clone()) else {
            return false;
        };
        if key.code == KeyCode::Char('/') {
            if let Some(world) = self.world.as_mut() {
                world.nav.push(Layer::Command);
            }
            return true;
        }
        match step {
            PartyStep::Actions { member, index } => self.party_actions_key(key, member, index),
            PartyStep::Detail { member, scroll } => self.party_detail_key(key, member, scroll),
        }
        true
    }

    fn party_actions_key(&mut self, key: KeyEvent, member: u8, index: usize) {
        match key.code {
            KeyCode::Esc => self.close_party_step(),
            KeyCode::Up | KeyCode::Down => {
                let len = self.world.as_ref().map_or(0, |w| party_menu(w, member).len());
                let next = if key.code == KeyCode::Down {
                    (index + 1).min(len.saturating_sub(1))
                } else {
                    index.saturating_sub(1)
                };
                if let Some(world) = self.world.as_mut() {
                    world.party_step = Some(PartyStep::Actions { member, index: next });
                }
            }
            KeyCode::Enter => self.party_run_selected(member, index),
            _ => {}
        }
    }

    fn party_detail_key(&mut self, key: KeyEvent, member: u8, scroll: usize) {
        let rows = self
            .world
            .as_ref()
            .and_then(|w| w.party())
            .and_then(|p| p.member(member))
            .map_or(0, |m| detail_rows(m).len());
        let next = match key.code {
            KeyCode::Esc => {
                if let Some(world) = self.world.as_mut() {
                    world.party_step = Some(PartyStep::Actions { member, index: 0 });
                    if world.nav.is(&Layer::Detail) {
                        world.nav.pop();
                    }
                }
                return;
            }
            KeyCode::Up => scroll.saturating_sub(1),
            KeyCode::Down => (scroll + 1).min(rows.saturating_sub(1)),
            KeyCode::PageUp => scroll.saturating_sub(5),
            KeyCode::PageDown => (scroll + 5).min(rows.saturating_sub(1)),
            _ => return,
        };
        if let Some(world) = self.world.as_mut() {
            world.party_step = Some(PartyStep::Detail { member, scroll: next });
        }
    }

    /// 执行动作列表里选中的那一项。成员已经不在队伍里（解散了）时动作作废，退回社交页。
    fn party_run_selected(&mut self, member: u8, index: usize) {
        let found = self.world.as_ref().and_then(|w| {
            let m = w.party()?.member(member)?;
            let act = *menu_for(w, m).get(index)?;
            Some((act, m.name.clone(), disabled_reason(act, w, m)))
        });
        let Some((act, name, reason)) = found else {
            if self.world.as_ref().and_then(|w| w.party()).and_then(|p| p.member(member)).is_none() {
                self.close_party_step();
            }
            return;
        };
        if let Some(reason) = reason {
            self.log.error(reason);
            return;
        }
        match act {
            PartyAct::Detail => {
                if let Some(world) = self.world.as_mut() {
                    world.party_step = Some(PartyStep::Detail { member, scroll: 0 });
                    world.nav.push(Layer::Detail);
                }
            }
            PartyAct::Disband => {
                self.close_party_step();
                self.ask_disband_party();
            }
            PartyAct::Leave => {
                self.close_party_step();
                self.ask_leave_party();
            }
            PartyAct::Kick => {
                self.close_party_step();
                self.ask_kick(&name);
            }
            PartyAct::Leader => {
                self.close_party_step();
                self.ask_leader(&name);
            }
        }
    }

    /// 关掉动作流程（动作完成、成员消失、`Esc`），并弹掉导航栈上的动作层和详情层。
    pub(super) fn close_party_step(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.party_step = None;
            while matches!(world.nav.top(), Layer::Action | Layer::Detail) {
                world.nav.pop();
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{PartyEquipment, PartyPet, WorldEntry};

    fn member() -> PartyMember {
        PartyMember {
            index: 1,
            name: "勇者·分身1".into(),
            clone: true,
            level: 5,
            hp: 40,
            max_hp: 44,
            mp: 100,
            max_mp: 100,
            pet: Some(PartyPet {
                name: "乌力".into(),
                level: 5,
                hp: 30,
                max_hp: 36,
                skills: vec!["攻击".into()],
            }),
            equipment: vec![PartyEquipment {
                slot: 2,
                name: "小木棒".into(),
            }],
            ..Default::default()
        }
    }

    #[test]
    fn detail_rows_cover_state_equipment_and_pet() {
        let rows = detail_rows(&member());
        let text: String = rows.iter().map(|(k, v)| format!("{k}:{v}\n")).collect();
        assert!(text.contains("Lv5  HP 40/44  MP 100/100"), "{text}");
        assert!(text.contains("2号 小木棒"), "{text}");
        assert!(text.contains("乌力 Lv5  HP 30/36  技能 攻击"), "{text}");
        assert!(text.contains("分身（创建时的快照"), "{text}");
        assert_eq!(
            rows.iter().find(|(k, _)| *k == "精灵").map(|(_, v)| v.as_str()),
            Some("无")
        );
    }

    #[test]
    fn disband_explains_why_it_cannot_run() {
        let mut entry = WorldEntry::default();
        let mut w = WorldState::new(entry.clone());
        assert_eq!(disabled_reason(PartyAct::Disband, &w, &member()), Some("没有队伍可解散"));
        assert_eq!(disabled_reason(PartyAct::Detail, &w, &member()), None, "看详情不受限制");
        entry.party = Some(crate::proto::Party {
            members: vec![member()],
            ..Default::default()
        });
        w = WorldState::new(entry);
        assert_eq!(disabled_reason(PartyAct::Disband, &w, &member()), None);
        w.entry.control.you = "observer".into();
        assert_eq!(
            disabled_reason(PartyAct::Disband, &w, &member()),
            Some("agent 正在操控，输入 take 收回控制权")
        );
        assert_eq!(disabled_reason(PartyAct::Detail, &w, &member()), None, "观察模式也能看详情");
    }
}
