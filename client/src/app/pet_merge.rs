//! 宠物加工、料理的合成界面（STAGE-208，`docs/synthesis.md` 第 9 节第 4、5 步，协议 5.55）：
//! 宠物页 → 动作「技能」→ 在加工或料理上 `Enter` → 背包里勾选材料（空格勾选，2–15 件）→ `Enter`
//! 进入默认取消的确认 → 看结果页（成功、失败还料、冷却、混合）。
//! 命令 `pet skill <宠物> <技能栏位> <背包格号…>` 走同一个发送函数；食物与道具混合在本地就拦下，不发请求。
//! 材料是不是能合成客户端不知道（协议没有这个字段），非材料由服务端回复 `ITEM_UNUSABLE`，同样不扣料。

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::{Value, json};

use crate::proto::InventorySlot;

use super::confirm::Confirm;
use super::pet_action::PetStep;
use super::pets_manage::PetOp;
use super::session::Pending;
use super::world::{BAG_LAST_SLOT, WorldState};
use super::{App, FIRST_BAG_SLOT};

/// 宠物技能表里的加工（`petskill.txt` 200）和料理（201）。
const SKILL_PROCESS: i32 = 200;
const SKILL_COOK: i32 = 201;
/// 一次合成的件数范围（协议 5.55）。
pub const MIN_MERGE_ITEMS: usize = 2;
pub const MAX_MERGE_ITEMS: usize = 15;
/// 物品表里食物（料理成品）的类型号，与服务端 `ItemTypeDish` 一致。
const KIND_FOOD: u32 = 20;

/// 是不是合成技能。
pub fn is_merge_skill(id: i32) -> bool {
    matches!(id, SKILL_PROCESS | SKILL_COOK)
}

/// 合成的四种结果（协议 5.55 的 `result`）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MergeKind {
    /// 合成出了配方产物。
    Created,
    /// 5 次匹配都失败，还回一件材料。
    Fallback,
    /// 冷却中，还回一件材料。
    Cooldown,
    /// 食物与道具混合，没有产物。
    Illegal,
}

/// 结果页的内容。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MergeReport {
    pub kind: MergeKind,
    pub lines: Vec<String>,
}

/// 勾选的材料里有没有食物和道具混在一起。
pub fn mixes_food_and_tools<'a>(kinds: impl IntoIterator<Item = &'a u32>) -> bool {
    let (mut food, mut other) = (false, false);
    for kind in kinds {
        if *kind == KIND_FOOD {
            food = true;
        } else {
            other = true;
        }
    }
    food && other
}

impl WorldState {
    /// 能当材料候选的格子：背包 5–19 里有东西的格子，按格号从小到大。
    pub fn merge_materials(&self) -> Vec<&InventorySlot> {
        let mut slots: Vec<&InventorySlot> = self
            .inventory
            .slots
            .iter()
            .filter(|s| (FIRST_BAG_SLOT..=BAG_LAST_SLOT).contains(&usize::from(s.slot)))
            .collect();
        slots.sort_by_key(|s| s.slot);
        slots
    }

    /// 已勾选格子里的物品（背包变化后已经空了的格子不算）。
    pub fn merge_picked<'a>(&'a self, picked: &[u8]) -> Vec<&'a InventorySlot> {
        picked
            .iter()
            .filter_map(|n| self.inventory.slots.iter().find(|s| s.slot == *n))
            .collect()
    }
}

impl App {
    /// 技能列表里的 `Enter`：加工、料理进入选材料；别的技能只说明一句。返回 true 表示已处理。
    pub(super) fn pet_merge_open(&mut self) -> bool {
        let Some((slot, index)) = self.world.as_ref().and_then(|w| match w.pet_step.as_ref() {
            Some(PetStep::Skills { slot, index }) => Some((*slot, *index)),
            _ => None,
        }) else {
            return true;
        };
        let Some(skill) = self
            .world
            .as_ref()
            .and_then(|w| w.pet_at(slot))
            .and_then(|p| p.skills.get(index).cloned())
        else {
            return true;
        };
        if !is_merge_skill(skill.id) {
            self.log.system(format!(
                "{} 不用材料；战斗外要用它请输入 pet skill {slot} {}",
                skill.name, skill.slot
            ));
            return true;
        }
        if self.observer_blocks() || !self.pet_op_allowed() {
            return true;
        }
        let have = self.world.as_ref().map_or(0, |w| w.merge_materials().len());
        if have < MIN_MERGE_ITEMS {
            self.log.system(format!(
                "背包里的东西不到 {MIN_MERGE_ITEMS} 件，凑不齐{}的材料",
                skill.name
            ));
            return true;
        }
        if let Some(world) = self.world.as_mut() {
            world.pet_step = Some(PetStep::Merge {
                slot,
                skill: skill.slot,
                cursor: 0,
                picked: Vec::new(),
                sending: false,
            });
        }
        true
    }

    /// 选材料步骤的按键：`↑↓` 移光标，空格勾选或取消，`Enter` 去确认，`Esc` 退回技能列表。
    pub(super) fn pet_merge_key(&mut self, key: KeyEvent) -> bool {
        if key.code == KeyCode::Esc {
            return self.pet_step_back();
        }
        let rows = self.world.as_ref().map_or(0, |w| w.merge_materials().len());
        let last = rows.saturating_sub(1);
        let Some(world) = self.world.as_mut() else {
            return true;
        };
        // 背包变化后，已经空掉的格子不再算勾选，光标也要落在列表里。
        let bag: Vec<u8> = world.inventory.slots.iter().map(|s| s.slot).collect();
        let Some(PetStep::Merge {
            cursor,
            picked,
            sending,
            ..
        }) = world.pet_step.as_mut()
        else {
            return true;
        };
        if *sending {
            return true;
        }
        picked.retain(|n| bag.contains(n));
        *cursor = (*cursor).min(last);
        match key.code {
            KeyCode::Up => *cursor = cursor.saturating_sub(1),
            KeyCode::Down => *cursor = (*cursor + 1).min(last),
            KeyCode::Home => *cursor = 0,
            KeyCode::End => *cursor = last,
            KeyCode::PageUp => *cursor = cursor.saturating_sub(5),
            KeyCode::PageDown => *cursor = (*cursor + 5).min(last),
            KeyCode::Char(' ') => self.pet_merge_toggle(),
            KeyCode::Enter => self.pet_merge_ask(),
            _ => {}
        }
        true
    }

    /// 勾选或取消光标所在的材料；已经勾满 15 件时只提示。
    fn pet_merge_toggle(&mut self) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let Some(PetStep::Merge { cursor, picked, .. }) = world.pet_step.as_ref() else {
            return;
        };
        let Some(item_slot) = world.merge_materials().get(*cursor).map(|s| s.slot) else {
            return;
        };
        let (full, was_picked) = (picked.len() >= MAX_MERGE_ITEMS, picked.contains(&item_slot));
        if full && !was_picked {
            self.log.system(format!("一次最多合成 {MAX_MERGE_ITEMS} 件材料"));
            return;
        }
        if let Some(PetStep::Merge { picked, .. }) = world.pet_step.as_mut() {
            if was_picked {
                picked.retain(|n| *n != item_slot);
            } else {
                picked.push(item_slot);
            }
        }
    }

    /// `Enter`：件数够、没有混合，就进入默认取消的确认；否则在消息栏说明原因。
    fn pet_merge_ask(&mut self) {
        let Some(world) = self.world.as_ref() else {
            return;
        };
        let Some(PetStep::Merge {
            slot, skill, picked, ..
        }) = world.pet_step.as_ref()
        else {
            return;
        };
        let (slot, skill, picked) = (*slot, *skill, picked.clone());
        let items = world.merge_picked(&picked);
        if items.len() < MIN_MERGE_ITEMS {
            self.log.system(format!(
                "至少要选 {MIN_MERGE_ITEMS} 件材料（空格勾选），现在 {} 件",
                items.len()
            ));
            return;
        }
        if mixes_food_and_tools(items.iter().map(|s| &s.kind)) {
            self.log
                .error("食物和道具不能混着合成（会白白扣掉材料），请取消其中一类");
            return;
        }
        let names: Vec<String> = items.iter().map(|s| s.name.clone()).collect();
        let skill_name = world
            .pet_at(slot)
            .and_then(|p| p.skills.iter().find(|s| s.slot == skill).map(|s| s.name.clone()))
            .unwrap_or_else(|| "合成".to_string());
        let text = format!(
            "用{skill_name}合成：{}（共 {} 件，材料不论成败都会用掉）",
            names.join("、"),
            names.len()
        );
        self.ask_confirm(
            Confirm::PetMerge {
                slot,
                skill,
                skill_name,
                items: picked,
                names,
            },
            &text,
        );
    }

    /// 结果页：`Enter` 再合成一次（回到空的选材料），`Esc` 退回技能列表。
    pub(super) fn pet_merge_result_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.pet_step_back(),
            KeyCode::Enter | KeyCode::Char(' ') => {
                if let Some(world) = self.world.as_mut() {
                    if let Some(PetStep::MergeResult { slot, skill, .. }) = world.pet_step.as_ref() {
                        let (slot, skill) = (*slot, *skill);
                        world.pet_step = Some(PetStep::Merge {
                            slot,
                            skill,
                            cursor: 0,
                            picked: Vec::new(),
                            sending: false,
                        });
                    }
                }
                true
            }
            _ => true,
        }
    }

    /// 发出合成请求（确认之后，或命令直接带格号）。`names` 与 `items` 一一对应，结果页用它写“用掉了什么”。
    pub(super) fn send_pet_merge(
        &mut self,
        slot: u8,
        skill: u8,
        skill_name: String,
        items: Vec<u8>,
        names: Vec<String>,
    ) {
        let sent = self.send(
            "pet_skill",
            json!({"slot": slot, "skill": skill, "items": items}),
            Pending::PetOp(PetOp::Merge {
                slot,
                skill,
                skill_name,
                names,
            }),
        );
        if sent {
            if let Some(PetStep::Merge { sending, .. }) = self.world.as_mut().and_then(|w| w.pet_step.as_mut()) {
                *sending = true;
            }
        }
    }

    /// 合成请求被拒（不扣料）：解除“等响应”，留在选材料步骤里让玩家改。错误原因已经由通用流程写进消息栏。
    pub(super) fn on_pet_merge_failed(&mut self) {
        if let Some(PetStep::Merge { sending, .. }) = self.world.as_mut().and_then(|w| w.pet_step.as_mut()) {
            *sending = false;
        }
    }

    /// 合成成功的响应：写消息栏，并把选材料步骤换成结果页（玩家已经退出去了就只写消息栏）。
    pub(super) fn on_pet_merged(&mut self, slot: u8, skill: u8, skill_name: &str, names: &[String], data: &Value) {
        let report = merge_report(skill_name, names, data);
        self.log.system(format!(
            "{skill_name}：{}",
            report.lines.first().map(String::as_str).unwrap_or("")
        ));
        if let Some(world) = self.world.as_mut() {
            if matches!(world.pet_step, Some(PetStep::Merge { slot: s, skill: k, .. }) if s == slot && k == skill) {
                world.pet_step = Some(PetStep::MergeResult { slot, skill, report });
            }
        }
    }
}

/// 按协议 5.55 的响应写结果页的几行字。
pub fn merge_report(skill_name: &str, names: &[String], data: &Value) -> MergeReport {
    let item = data.get("item").filter(|v| v.is_object());
    let got = item.map(|it| {
        let name = it.get("name").and_then(Value::as_str).unwrap_or("物品");
        match it.get("slot").and_then(Value::as_u64) {
            Some(n) => format!("{name}（放进 {n} 号格）"),
            None => name.to_string(),
        }
    });
    let message = data.get("message").and_then(Value::as_str).map(str::to_string);
    let used = format!("用掉了：{}", names.join("、"));
    let (kind, mut lines) = match data.get("result").and_then(Value::as_str) {
        Some("created") => (
            MergeKind::Created,
            vec![format!("{skill_name}成功：{}", got.unwrap_or_default())],
        ),
        Some("fallback") => (
            MergeKind::Fallback,
            vec![
                "没合成出东西（配方对不上）".to_string(),
                format!("还回一件材料：{}", got.unwrap_or_default()),
            ],
        ),
        Some("cooldown") => (
            MergeKind::Cooldown,
            vec![
                message.unwrap_or_else(|| "合成太频繁了，休息一下".to_string()),
                format!("材料照样用掉了，还回一件：{}", got.unwrap_or_default()),
            ],
        ),
        _ => (
            MergeKind::Illegal,
            vec![
                message.unwrap_or_else(|| "非法的合成方法".to_string()),
                "食物和道具混合，材料用掉了，没有产物".to_string(),
            ],
        ),
    };
    lines.push(used);
    MergeReport { kind, lines }
}

/// 命令 `pet skill <宠物> <技能栏位> <格号…>` 里的背包格号：5–19，2–15 件，不能重复。
pub fn parse_merge_items(parts: &[&str]) -> Result<Vec<u8>, String> {
    let mut items: Vec<u8> = Vec::new();
    for part in parts {
        let n = match part.parse::<u8>() {
            Ok(n) if (FIRST_BAG_SLOT..=BAG_LAST_SLOT).contains(&usize::from(n)) => n,
            _ => {
                return Err(format!(
                    "材料的背包格号要在 {FIRST_BAG_SLOT}–{BAG_LAST_SLOT} 之间：{part}"
                ));
            }
        };
        if items.contains(&n) {
            return Err(format!("{n} 号格重复了，一格只能放一次"));
        }
        items.push(n);
    }
    if !(MIN_MERGE_ITEMS..=MAX_MERGE_ITEMS).contains(&items.len()) {
        return Err(format!(
            "合成要 {MIN_MERGE_ITEMS}–{MAX_MERGE_ITEMS} 件材料，现在 {} 件",
            items.len()
        ));
    }
    Ok(items)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_process_and_cook_are_merge_skills() {
        assert!(is_merge_skill(200) && is_merge_skill(201));
        assert!(!is_merge_skill(1) && !is_merge_skill(202));
    }

    #[test]
    fn food_with_tools_is_a_mix() {
        assert!(mixes_food_and_tools([20u32, 1].iter()));
        assert!(!mixes_food_and_tools([20u32, 20].iter()));
        assert!(!mixes_food_and_tools([1u32, 3].iter()));
        assert!(!mixes_food_and_tools([].iter()));
    }

    #[test]
    fn command_items_are_checked_locally() {
        assert_eq!(parse_merge_items(&["5", "6", "19"]), Ok(vec![5, 6, 19]));
        assert!(parse_merge_items(&["5"]).is_err(), "少于 2 件");
        assert!(parse_merge_items(&["5", "5"]).is_err(), "重复");
        assert!(parse_merge_items(&["4", "6"]).is_err(), "装备格不是材料");
        assert!(parse_merge_items(&["5", "20"]).is_err(), "越界");
        assert!(parse_merge_items(&["5", "x"]).is_err(), "不是数字");
        let many: Vec<String> = (5..=19).map(|n| n.to_string()).collect();
        let refs: Vec<&str> = many.iter().map(String::as_str).collect();
        assert_eq!(parse_merge_items(&refs).map(|v| v.len()), Ok(15));
    }

    #[test]
    fn report_lines_follow_the_four_results() {
        let names = vec!["斧".to_string(), "石".to_string()];
        let r = merge_report(
            "加工",
            &names,
            &json!({"result": "created", "item": {"slot": 5, "name": "合成枪1"}, "consumed": [5, 6]}),
        );
        assert_eq!(r.kind, MergeKind::Created);
        assert_eq!(r.lines[0], "加工成功：合成枪1（放进 5 号格）");
        assert_eq!(r.lines.last().unwrap(), "用掉了：斧、石");
        let r = merge_report(
            "加工",
            &names,
            &json!({"result": "fallback", "item": {"slot": 6, "name": "斧"}}),
        );
        assert_eq!(r.kind, MergeKind::Fallback);
        assert!(r.lines[1].contains("还回一件材料：斧"));
        let r = merge_report(
            "料理",
            &names,
            &json!({"result": "cooldown", "item": {"slot": 6, "name": "肉"}, "message": "合成料理过於频繁，休息一下比较好哟。"}),
        );
        assert_eq!(r.kind, MergeKind::Cooldown);
        assert!(r.lines[0].contains("过於频繁"));
        let r = merge_report(
            "料理",
            &names,
            &json!({"result": "illegal", "message": "非法的合成方法"}),
        );
        assert_eq!(r.kind, MergeKind::Illegal);
        assert_eq!(r.lines[0], "非法的合成方法");
    }
}
