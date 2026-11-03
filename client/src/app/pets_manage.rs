//! 「宠物」页的管理命令（STAGE-32，`docs/ui.md` 宠物页 B、`docs/protocol.md` 5.22–5.24）：
//! `pet battle|standby|rest <编号>` 设状态，`pet rename <编号> [名字]` 改名（不带名字恢复种族名），
//! `pet drop <编号>` 放生：先在消息栏问一句，输入 y 才真正发出（确认见 `confirm.rs`）；`pet drop <编号> yes` 直接放生。
//! `pet skill <编号> <栏位>` 战斗外使用宠物的技能（STAGE-144，协议 5.50）：栏位 1–7，结果由服务端回复。
//! 数据变化由 `pets_changed` 推送告诉界面，这里只负责发请求和写消息栏。

use serde_json::{Value, json};

use super::commands;
use super::confirm::Confirm;
use super::pet_merge::{is_merge_skill, mixes_food_and_tools, parse_merge_items};
use super::pets::MAX_PET_SLOT;
use super::session::Pending;
use super::App;

/// 宠物名字的字符数上限（与服务端 `normalizePetName` 一致）。改名的菜单表单也用它。
pub const MAX_PET_NAME_CHARS: usize = 16;

/// 用法提示，从命令表取（STAGE-26）。
fn usage() -> String {
    commands::usage_of("pet")
}

/// 宠物管理请求（`Pending::PetOp` 携带）：成功后写进消息栏的内容。
#[derive(Debug, Clone)]
pub enum PetOp {
    /// 设为出战、待命或休息；label 是界面上的说法。
    State { slot: u8, label: &'static str },
    /// 改名；name 为空表示恢复种族名。
    Rename { slot: u8, name: String },
    /// 放生；name 是放生前的名字。
    Drop { slot: u8, name: String },
    /// 战斗外使用技能；name 是技能名，成功时写进消息栏。
    Skill { slot: u8, name: String },
    /// 加工、料理的合成（STAGE-208）：`names` 与请求的背包格号一一对应，结果页写“用掉了什么”。
    Merge { slot: u8, skill: u8, skill_name: String, names: Vec<String> },
    /// 骑上（STAGE-123）。
    Ride { slot: u8 },
    /// 下骑（STAGE-123）。
    Unride,
    /// 和宠物对话（STAGE-239）：台词进消息栏。
    Talk { slot: u8 },
}

/// 解析栏位编号（0–19，STAGE-346）。
fn parse_slot(text: &str) -> Result<u8, String> {
    match text.parse::<u8>() {
        Ok(slot) if slot <= MAX_PET_SLOT => Ok(slot),
        _ => Err("宠物栏位要在 0–19 之间".to_string()),
    }
}

/// 解析宠物技能栏位（1–7，宠物页上的编号）。
fn parse_skill_slot(text: &str) -> Result<u8, String> {
    match text.parse::<u8>() {
        Ok(skill) if (1..=7).contains(&skill) => Ok(skill),
        _ => Err("技能栏位要在 1–7 之间".to_string()),
    }
}

/// 解析改名的名字：没有参数表示恢复种族名（返回空串）；名字不能有空白，最多 16 个字符。
fn parse_name(parts: &[&str]) -> Result<String, String> {
    match parts {
        [] => Ok(String::new()),
        [name] if name.chars().count() <= MAX_PET_NAME_CHARS => Ok((*name).to_string()),
        [_] => Err(format!("宠物名字最多 {MAX_PET_NAME_CHARS} 个字符")),
        _ => Err("宠物名字中间不能有空白".to_string()),
    }
}

impl App {
    /// `pet` 的管理子命令。返回 false 表示没有认出管理子命令，交给 `pet_command` 处理（如 `pet status`）。
    pub(super) fn pet_manage_command(&mut self, rest: &str) -> bool {
        let args: Vec<&str> = rest.split_whitespace().collect();
        let Some((&sub, tail)) = args.split_first() else {
            return false;
        };
        match (sub, tail) {
            ("battle", [num]) => self.set_pet_state(num, "battle", "出战"),
            ("standby", [num]) => self.set_pet_state(num, "standby", "待命"),
            ("rest", [num]) => self.set_pet_state(num, "rest", "休息"),
            ("ride", ["off"]) => self.unride_pet(),
            ("ride", [num]) => self.ride_pet(num),
            ("rename", [num, name @ ..]) => self.rename_pet(num, name),
            ("drop", [num]) => self.drop_pet(num, false),
            ("drop", [num, "yes"]) => self.drop_pet(num, true),
            ("skill", [num, skill, items @ ..]) => self.use_pet_skill(num, skill, items),
            ("talk", [num]) => self.talk_pet(num),
            ("battle" | "standby" | "rest" | "ride" | "rename" | "drop" | "skill" | "talk", _) => self.log.error(usage()),
            _ => return false,
        }
        true
    }

    /// 管理命令能不能发：战斗中不能做。观察模式的拦截由命令表驱动（STAGE-26）。
    pub(super) fn pet_op_allowed(&mut self) -> bool {
        if self.world.as_ref().is_some_and(|w| w.battle.is_some()) {
            self.log.error("战斗中不能管理宠物");
            return false;
        }
        true
    }

    /// `pet battle|standby|rest <编号>`：设状态，原来的出战宠由服务端自动变成休息。
    fn set_pet_state(&mut self, num: &str, state: &'static str, label: &'static str) {
        let slot = match parse_slot(num) {
            Ok(slot) => slot,
            Err(msg) => return self.log.error(msg),
        };
        self.set_pet_state_slot(slot, state, label);
    }

    /// 设宠物状态（命令和 STAGE-133 的菜单动作共用）。
    pub(super) fn set_pet_state_slot(&mut self, slot: u8, state: &'static str, label: &'static str) {
        if !self.pet_op_allowed() {
            return;
        }
        self.send("pet_state", json!({"slot": slot, "state": state}), Pending::PetOp(PetOp::State { slot, label }));
    }

    /// `pet ride <编号>`：骑上去（服务端按 5.47 检查，不满足时回复原因）。
    fn ride_pet(&mut self, num: &str) {
        match parse_slot(num) {
            Ok(slot) => self.ride_pet_slot(slot),
            Err(msg) => self.log.error(msg),
        }
    }

    /// 骑上这只宠物（命令和宠物动作菜单共用，STAGE-243）。
    pub(super) fn ride_pet_slot(&mut self, slot: u8) {
        if !self.pet_op_allowed() {
            return;
        }
        self.send("pet_ride", json!({"slot": slot}), Pending::PetOp(PetOp::Ride { slot }));
    }

    /// `pet talk <编号>`：和宠物对话（STAGE-239）。
    fn talk_pet(&mut self, num: &str) {
        match parse_slot(num) {
            Ok(slot) => self.talk_pet_slot(slot),
            Err(msg) => self.log.error(msg),
        }
    }

    /// 和宠物对话（命令和宠物动作菜单共用）。台词由 `on_pet_op` 写进消息栏。
    pub(super) fn talk_pet_slot(&mut self, slot: u8) {
        if !self.pet_op_allowed() {
            return;
        }
        self.send("pet_talk", json!({"slot": slot}), Pending::PetOp(PetOp::Talk { slot }));
    }

    /// `pet ride off`：骑下来。没有在骑时服务端也回复成功，什么都不改。
    pub(super) fn unride_pet(&mut self) {
        if !self.pet_op_allowed() {
            return;
        }
        self.send("pet_unride", json!({}), Pending::PetOp(PetOp::Unride));
    }

    /// `pet rename <编号> [名字]`：不带名字就恢复种族名（服务端收到空串时这样处理）。
    fn rename_pet(&mut self, num: &str, parts: &[&str]) {
        let slot = match parse_slot(num) {
            Ok(slot) => slot,
            Err(msg) => return self.log.error(msg),
        };
        let name = match parse_name(parts) {
            Ok(name) => name,
            Err(msg) => return self.log.error(msg),
        };
        self.rename_pet_slot(slot, name);
    }

    /// 改名（命令和 STAGE-133 的菜单表单共用）：名字为空表示恢复种族名。
    pub(super) fn rename_pet_slot(&mut self, slot: u8, name: String) {
        if !self.pet_op_allowed() {
            return;
        }
        self.send("pet_rename", json!({"slot": slot, "name": name}), Pending::PetOp(PetOp::Rename { slot, name }));
    }

    /// `pet drop <编号> [yes]`：没有 yes 就先问一句，等玩家确认（`Confirm::PetDrop`）；带 yes 直接放生。
    fn drop_pet(&mut self, num: &str, confirmed: bool) {
        let slot = match parse_slot(num) {
            Ok(slot) => slot,
            Err(msg) => return self.log.error(msg),
        };
        if confirmed {
            if !self.pet_op_allowed() {
                return;
            }
            let name = self.world.as_ref().and_then(|w| w.pets.pets.iter().find(|p| p.slot == slot).map(|p| p.name.clone()));
            let Some(name) = name else {
                return self.log.error(format!("{slot} 号栏位没有宠物"));
            };
            return self.send_pet_drop(slot, name);
        }
        self.ask_drop_pet(slot);
    }

    /// 问一句再放生（命令和 STAGE-133 的菜单动作共用）：栏位没有宠物时在本地就拦下。
    pub(super) fn ask_drop_pet(&mut self, slot: u8) {
        if !self.pet_op_allowed() {
            return;
        }
        let name = self.world.as_ref().and_then(|w| w.pets.pets.iter().find(|p| p.slot == slot).map(|p| p.name.clone()));
        let Some(name) = name else {
            return self.log.error(format!("{slot} 号栏位没有宠物"));
        };
        let text = format!("放生后不能恢复：{name}（{slot} 号）");
        self.ask_confirm(Confirm::PetDrop { slot, name }, &text);
    }

    /// `pet skill <编号> <栏位> [背包格号…]`：栏位要是有技能的栏位。战斗中不能用（服务端也会回复 IN_BATTLE）。
    /// 加工、料理（STAGE-208）必须带 2–15 个背包格号当材料，别的技能不能带。
    fn use_pet_skill(&mut self, num: &str, skill: &str, items: &[&str]) {
        let slot = match parse_slot(num) {
            Ok(slot) => slot,
            Err(msg) => return self.log.error(msg),
        };
        let skill_slot = match parse_skill_slot(skill) {
            Ok(skill_slot) => skill_slot,
            Err(msg) => return self.log.error(msg),
        };
        if !self.pet_op_allowed() {
            return;
        }
        let pet = self
            .world
            .as_ref()
            .and_then(|w| w.pets.pets.iter().find(|p| p.slot == slot));
        let Some(pet) = pet else {
            return self.log.error(format!("{slot} 号栏位没有宠物"));
        };
        let skill = pet.skills.iter().find(|s| s.slot == skill_slot);
        let Some((name, id)) = skill.map(|s| (s.name.clone(), s.id)) else {
            return self.log.error(format!("{slot} 号宠物的 {skill_slot} 号技能栏位是空的"));
        };
        if is_merge_skill(id) {
            return self.use_merge_skill(slot, skill_slot, name, items);
        }
        if !items.is_empty() {
            return self.log.error(format!("{name} 不用材料，pet skill {slot} {skill_slot} 后面不要带背包格号"));
        }
        self.send(
            "pet_skill",
            json!({"slot": slot, "skill": skill_slot}),
            Pending::PetOp(PetOp::Skill { slot, name }),
        );
    }

    /// 命令形式的合成：材料在本地先查（格号范围、重复、件数、食物与道具混合），查过才发。
    fn use_merge_skill(&mut self, slot: u8, skill: u8, skill_name: String, items: &[&str]) {
        if items.is_empty() {
            return self.log.error(format!(
                "{skill_name}要带材料：pet skill {slot} {skill} <背包格号…>（2–15 个，格号 5–19）；也可以在宠物页的技能里勾选"
            ));
        }
        let items = match parse_merge_items(items) {
            Ok(items) => items,
            Err(msg) => return self.log.error(msg),
        };
        let Some(world) = self.world.as_ref() else {
            return;
        };
        let found = world.merge_picked(&items);
        if mixes_food_and_tools(found.iter().map(|s| &s.kind)) {
            return self.log.error("食物和道具不能混着合成（会白白扣掉材料），请只留一类");
        }
        let names = items
            .iter()
            .map(|n| found.iter().find(|s| s.slot == *n).map_or_else(|| format!("{n} 号格"), |s| s.name.clone()))
            .collect();
        self.send_pet_merge(slot, skill, skill_name, items, names);
    }

    /// 发出放生请求（直接带 yes，或者确认之后）。
    pub(super) fn send_pet_drop(&mut self, slot: u8, name: String) {
        self.send("pet_drop", json!({"slot": slot, "confirm": "yes"}), Pending::PetOp(PetOp::Drop { slot, name }));
    }

    /// 管理请求成功：状态、名字已经由 `pets_changed` 推送更新，这里只写一句话到消息栏。
    pub(super) fn on_pet_op(&mut self, op: PetOp, data: &Value) {
        match op {
            PetOp::State { slot, label } => self.log.system(format!("{slot} 号宠物已设为{label}")),
            PetOp::Ride { slot } => self.log.system(format!("{slot} 号宠物已骑上")),
            PetOp::Unride => self.log.system("宠物已骑下来".to_string()),
            PetOp::Talk { slot } => {
                // 台词里的换行在消息栏里改成空格；没有对话的宠物（text 为空）原版什么也不说。
                let text = data["text"].as_str().unwrap_or("").replace('\n', " ");
                let name = self.world.as_ref().and_then(|w| w.pet_at(slot)).map_or_else(|| format!("{slot} 号宠物"), |p| p.name);
                if text.is_empty() {
                    self.log.system(format!("{name}没有反应"));
                } else {
                    self.log.system(format!("{name}：{text}"));
                }
            }
            PetOp::Rename { slot, name } if name.is_empty() => self.log.system(format!("{slot} 号宠物已恢复种族名")),
            PetOp::Rename { slot, name } => self.log.system(format!("{slot} 号宠物改名为 {name}")),
            PetOp::Drop { slot, name } => self.log.system(format!("{name}（{slot} 号）已放生")),
            PetOp::Skill { slot, name } => self.log.system(format!("{slot} 号宠物使用了{name}")),
            PetOp::Merge { slot, skill, skill_name, names } => self.on_pet_merged(slot, skill, &skill_name, &names, data),
        }
    }

    /// 管理请求被拒：只有合成要善后（解除“等响应”），错误原因已经由通用流程写进消息栏。
    pub(super) fn on_pet_op_failed(&mut self, op: &PetOp) {
        if matches!(op, PetOp::Merge { .. }) {
            self.on_pet_merge_failed();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_slots() {
        assert_eq!(parse_slot("0"), Ok(0));
        assert_eq!(parse_slot("4"), Ok(4));
        assert_eq!(parse_slot("19"), Ok(19));
        assert!(parse_slot("20").is_err());
        assert!(parse_slot("-1").is_err());
        assert!(parse_slot("一").is_err());
    }

    #[test]
    fn parses_names() {
        assert_eq!(parse_name(&[]), Ok(String::new()), "没有名字就恢复种族名");
        assert_eq!(parse_name(&["小白"]), Ok("小白".to_string()));
        assert_eq!(parse_name(&["一二三四五六七八九十一二三四五六"]), Ok("一二三四五六七八九十一二三四五六".to_string()), "16 个字符");
        assert!(parse_name(&["一二三四五六七八九十一二三四五六七"]).unwrap_err().contains("16"));
        assert!(parse_name(&["小", "白"]).unwrap_err().contains("空白"));
    }

    #[test]
    fn parses_skill_slots() {
        assert_eq!(parse_skill_slot("1"), Ok(1));
        assert_eq!(parse_skill_slot("7"), Ok(7));
        assert!(parse_skill_slot("0").is_err());
        assert!(parse_skill_slot("8").is_err());
        assert!(parse_skill_slot("二").is_err());
    }

    #[test]
    fn usage_comes_from_the_command_table() {
        assert!(usage().starts_with("用法：pet "));
    }
}
