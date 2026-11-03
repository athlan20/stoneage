//! 列表面板的内容（STAGE-107，`docs/battle-ui.md`「编号规则」「列表面板」）：道具、精灵、换宠、宠物技能各自的行。
//! 行的号和命令参数一致；空栏位、不能用的行带原因，灰色、不能选。界面见 `ui/battle/panel.rs`，按键见 `list_keys.rs`。

use super::Pick;
use super::auto_heal::heal_power;
use crate::app::WorldState;
use crate::proto::{InventorySlot, MagicView, Pet, PetStatus, TargetType};

/// 精灵的栏位数（0–4，人物精灵栏位不随宠物栏位变）。
const SPIRIT_SLOTS: u8 = 5;
/// 宠物技能的栏位数（1–7）。
const SKILL_SLOTS: u8 = 7;
/// 收回出战宠物的命令参数（`swap -`）。
pub const RECALL: i64 = -1;

/// 列表里的一行。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Row {
    /// 界面上的号，和命令参数一致；收回那一行没有号。
    pub number: Option<u8>,
    /// 命令参数：道具槽位（5–19）、精灵栏位（0–4）、换宠栏位（0–19，收回为 -1）、技能栏位（1–7）。
    pub key: i64,
    pub name: String,
    /// 附加信息：道具数量、精灵的 MP、宠物的等级和血量。
    pub extra: String,
    /// 目标类型的大白话（“己方一个”）。
    pub target: &'static str,
    /// 现在不能选的原因；None 表示能选。
    pub blocked: Option<String>,
    /// 高亮时说明栏显示的文字，第一行是名字和效果，之后是目标或状态。
    pub desc: String,
    /// 道具是不是回复类（默认高亮用）。
    pub heals: bool,
}

/// 列表的全部行。道具只列有东西的格子；精灵、换宠、技能的栏位固定列全，空的占一行。
pub fn rows(w: &WorldState, pick: Pick) -> Vec<Row> {
    match pick {
        Pick::Item => item_rows(w),
        Pick::Magic => magic_rows(w),
        Pick::Pet => pet_rows(w),
        Pick::Skill => skill_rows(w),
        Pick::Action | Pick::Target { .. } | Pick::Clones | Pick::CloneAim { .. } => vec![],
    }
}

fn item_rows(w: &WorldState) -> Vec<Row> {
    let mut slots: Vec<&InventorySlot> = w
        .inventory
        .slots
        .iter()
        .filter(|s| (5..=19).contains(&s.slot))
        .collect();
    slots.sort_by_key(|s| s.slot);
    slots
        .into_iter()
        .map(|s| {
            let unusable = s.target_type == TargetType::None;
            let effect = if s.effect.is_empty() {
                "没有说明"
            } else {
                s.effect.as_str()
            };
            let target = if unusable {
                "不能用"
            } else {
                s.target_type.text()
            };
            Row {
                number: Some(s.slot),
                key: i64::from(s.slot),
                name: s.name.clone(),
                extra: format!("×{}", s.count),
                target,
                blocked: unusable.then(|| format!("{}战斗中不能用", s.name)),
                desc: format!("{}：{effect}\n目标：{target}", s.name),
                heals: heal_power(&s.effect).is_some(),
            }
        })
        .collect()
}

fn magic_rows(w: &WorldState) -> Vec<Row> {
    let mp = w.entry.status.mp;
    (0..SPIRIT_SLOTS)
        .map(|slot| match w.entry.status.magic.iter().find(|m| m.slot == slot) {
            Some(m) => Row {
                number: Some(slot),
                key: i64::from(slot),
                name: m.name.clone(),
                extra: format!("MP {}", m.mp),
                target: m.target_type.text(),
                blocked: magic_blocked(m, mp),
                desc: format!("{}：消耗 MP {}\n目标：{}", m.name, m.mp, m.target_type.text()),
                heals: false,
            },
            None => empty(slot, "精灵"),
        })
        .collect()
}

/// 精灵在战斗中为什么不能用（灰色、不能选）：变幻的精灵（目标 none，STAGE-203）只能战斗外用；其余看 MP 够不够。
fn magic_blocked(m: &MagicView, mp: u32) -> Option<String> {
    if m.target_type == TargetType::None {
        return Some(format!("{}只能在战斗外使用", m.name));
    }
    (m.mp > mp).then(|| format!("MP 不够，{}需要 MP {}", m.name, m.mp))
}

fn pet_rows(w: &WorldState) -> Vec<Row> {
    let mut rows: Vec<Row> = (0..w.pets.max_slots)
        .map(|slot| match w.pets.pets.iter().find(|p| p.slot == slot) {
            Some(p) => pet_row(p),
            None => empty(slot, "宠物"),
        })
        .collect();
    let has_battle = w.pets.pets.iter().any(Pet::is_battle);
    rows.push(Row {
        number: None,
        key: RECALL,
        name: "收回出战宠物".into(),
        extra: String::new(),
        target: "",
        blocked: (!has_battle).then(|| "没有出战的宠物".to_string()),
        desc: "把出战的宠物收回（命令 swap -）".into(),
        heals: false,
    });
    rows
}

fn pet_row(p: &Pet) -> Row {
    let name = if p.name.is_empty() {
        p.species.clone()
    } else {
        p.name.clone()
    };
    let blocked = if p.hp == 0 {
        Some(format!("{name}已经倒下，不能出战"))
    } else if p.is_battle() {
        Some(format!("{name}已经在场上"))
    } else {
        None
    };
    let state = match p.status() {
        PetStatus::Battle => "出战中",
        PetStatus::Standby => "待命",
        PetStatus::Ride => "骑乘",
        PetStatus::Rest => "休息",
        PetStatus::Fainted => "已倒下",
    };
    Row {
        number: Some(p.slot),
        key: i64::from(p.slot),
        name: name.clone(),
        extra: format!("Lv{} {}/{}", p.level, p.hp, p.max_hp),
        target: "",
        blocked,
        desc: format!("{name}（{}）Lv{} HP {}/{}\n{state}", p.species, p.level, p.hp, p.max_hp),
        heals: false,
    }
}

fn skill_rows(w: &WorldState) -> Vec<Row> {
    let pet = w.pets.pets.iter().find(|p| p.is_battle());
    (1..=SKILL_SLOTS)
        .map(
            |slot| match pet.and_then(|p| p.skills.iter().find(|s| s.slot == slot)) {
                Some(s) => Row {
                    number: Some(slot),
                    key: i64::from(slot),
                    name: s.name.clone(),
                    extra: String::new(),
                    target: s.target_type.text(),
                    blocked: None,
                    desc: format!("{}\n目标：{}", s.name, s.target_type.text()),
                    heals: false,
                },
                None => empty(slot, "技能"),
            },
        )
        .collect()
}

/// 空栏位：灰色占一行，选中时说明原因。`what` 是“精灵”“宠物”“技能”。
fn empty(slot: u8, what: &str) -> Row {
    Row {
        number: Some(slot),
        key: i64::from(slot),
        name: "—".into(),
        extra: String::new(),
        target: "",
        blocked: Some(format!("{slot} 号栏位没有{what}")),
        desc: format!("{slot} 号栏位是空的"),
        heals: false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{MagicView, PetSkill};

    fn item(slot: u8, name: &str, effect: &str, target: TargetType) -> InventorySlot {
        InventorySlot {
            slot,
            name: name.into(),
            effect: effect.into(),
            count: 1,
            target_type: target,
            ..Default::default()
        }
    }

    fn world() -> WorldState {
        let mut w = WorldState::new(Default::default());
        w.inventory.slots = vec![
            item(9, "小斧头", "", TargetType::None),
            item(7, "回复药", "体100回复（1体）", TargetType::Ally),
            item(12, "毒消", "解毒", TargetType::Ally),
        ];
        w
    }

    #[test]
    fn items_keep_their_bag_numbers_and_unusable_ones_are_blocked() {
        let rows = rows(&world(), Pick::Item);
        let numbers: Vec<_> = rows.iter().filter_map(|r| r.number).collect();
        assert_eq!(numbers, vec![7, 9, 12], "按槽位号排，不重新编号");
        assert!(rows[1].blocked.is_some(), "小斧头战斗中不能用");
        assert_eq!(rows[0].target, "己方一个");
    }

    #[test]
    fn magic_and_skill_always_list_every_slot() {
        let mut w = world();
        w.entry.status.mp = 5;
        w.entry.status.magic = vec![MagicView {
            slot: 2,
            name: "岩石".into(),
            mp: 10,
            target_type: TargetType::Enemy, // 战斗精灵都带目标（只有变幻的精灵是 none）
            ..Default::default()
        }];
        let magic = rows(&w, Pick::Magic);
        assert_eq!(magic.len(), 5);
        assert_eq!(magic[2].blocked.as_deref(), Some("MP 不够，岩石需要 MP 10"));
        assert_eq!(magic[0].blocked.as_deref(), Some("0 号栏位没有精灵"));
        assert_eq!(rows(&w, Pick::Skill).len(), 7, "技能 1–7 全部列出，空的占一行");
    }

    #[test]
    fn metamo_spirit_is_listed_but_refused_in_battle() {
        let mut w = world();
        w.entry.status.mp = 100;
        w.entry.status.magic = vec![MagicView {
            slot: 1,
            name: "变幻的精灵 Lv1".into(),
            mp: 8,
            target_type: TargetType::None,
            ..Default::default()
        }];
        let magic = rows(&w, Pick::Magic);
        assert_eq!(magic[1].blocked.as_deref(), Some("变幻的精灵 Lv1只能在战斗外使用"), "列出来，但战斗中不能选");
    }

    #[test]
    fn pet_list_ends_with_recall_and_refuses_the_battle_pet() {
        let mut w = world();
        w.pets.pets = vec![
            Pet {
                slot: 1,
                name: "乌力".into(),
                state: "battle".into(),
                hp: 35,
                max_hp: 35,
                level: 5,
                ..Default::default()
            },
            Pet {
                slot: 3,
                name: "波利".into(),
                state: "standby".into(),
                hp: 0,
                max_hp: 20,
                ..Default::default()
            },
        ];
        let pets = rows(&w, Pick::Pet);
        assert_eq!(pets.len(), usize::from(w.pets.max_slots) + 1, "20 个栏位加收回一行");
        assert_eq!(pets[1].blocked.as_deref(), Some("乌力已经在场上"));
        assert_eq!(pets[3].blocked.as_deref(), Some("波利已经倒下，不能出战"));
        let recall = pets.len() - 1;
        assert_eq!(pets[recall].key, RECALL);
        assert!(pets[recall].blocked.is_none(), "有出战的宠物，可以收回");
        assert_eq!(pets[2].blocked.as_deref(), Some("2 号栏位没有宠物"));
    }

    #[test]
    fn skill_rows_follow_the_battle_pet() {
        let mut w = world();
        w.pets.pets = vec![Pet {
            slot: 0,
            state: "battle".into(),
            skills: vec![PetSkill {
                slot: 2,
                name: "撞击".into(),
                ..Default::default()
            }],
            ..Default::default()
        }];
        let skills = rows(&w, Pick::Skill);
        assert_eq!(skills[1].key, 2);
        assert!(skills[1].blocked.is_none());
        assert_eq!(skills[0].blocked.as_deref(), Some("1 号栏位没有技能"));
    }
}
