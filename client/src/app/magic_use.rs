//! `magic <栏位> [宠物编号]`（STAGE-203，`docs/protocol.md` 5.51）：战斗外使用精灵。一期只有变幻的精灵能用：
//! 带宠物编号是变成那只宠物，不带是变回自己。变身成功的提示和角色页的变身行由服务端推送告诉界面，这里只发请求。
//! `magic use <栏位> [self|pet <宠物编号>]`（STAGE-231）：通用写法，对象和角色页「使用」的对象选择是同一条请求。

use serde_json::{Value, json};

use crate::proto::MagicView;

use super::App;
use super::commands;
use super::pets::MAX_PET_SLOT;
use super::session::Pending;

/// 精灵栏位的最大编号（0–4，同 `docs/protocol.md` 5.8 的 `status.magic`）。
const MAX_MAGIC_SLOT: u8 = 4;

/// 精灵的对象（`docs/protocol.md` 5.51 的 `to`）：自己，或某个宠物栏位。队友一期服务端一律无效，这里不列。
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MagicTo {
    SelfOnly,
    Pet(u8),
}

/// 精灵在战斗外怎么用（按魔法表编号分，`docs/protocol.md` 5.51）。
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MagicUse {
    /// 变幻的精灵：角色页只把 `magic <栏位> ` 填进命令栏，变身流程保持命令的写法。
    Metamo,
    /// 滋润的精灵：要选对象，自己或宠物。
    Pick,
    /// 其他（治愈、恩惠只回自己，复活、解状态、防御没有效果，战斗精灵不能用）：直接发，不选对象。
    Direct,
}

/// 按魔法表编号判断精灵的战斗外用法（`docs/protocol.md` 5.51）：变幻 350、351、352、355、410；滋润 10–15。
pub fn magic_use_kind(m: &MagicView) -> MagicUse {
    match m.id {
        350 | 351 | 352 | 355 | 410 => MagicUse::Metamo,
        10..=15 => MagicUse::Pick,
        _ => MagicUse::Direct,
    }
}

/// `magic_use` 的请求体：不带对象是自己（治愈等），`to` 为 self 或 pet。
pub fn use_data(slot: u8, to: Option<MagicTo>) -> Value {
    match to {
        None => json!({"slot": slot}),
        Some(MagicTo::SelfOnly) => json!({"slot": slot, "to": "self"}),
        Some(MagicTo::Pet(pet)) => json!({"slot": slot, "to": "pet", "target": pet}),
    }
}

/// 服务端没给中文原因（`message` 为空）时，`magic_use` 错误码的中文提示；服务端的原因优先。
pub fn magic_use_error_hint(code: &str) -> Option<&'static str> {
    match code {
        "NOT_ENOUGH_MP" => Some("MP 不够，用不了这个精灵"),
        "INVALID_TARGET" => Some("这个对象不能接受这个精灵的效果"),
        "MAGIC_UNUSABLE" => Some("这个精灵不能在战斗外使用"),
        "BAD_REQUEST" => Some("精灵的参数不对，没有发出"),
        _ => None,
    }
}

/// 解析精灵栏位（0–4）。
fn parse_magic_slot(text: &str) -> Result<u8, String> {
    match text.parse::<u8>() {
        Ok(slot) if slot <= MAX_MAGIC_SLOT => Ok(slot),
        _ => Err("精灵栏位要在 0–4 之间".to_string()),
    }
}

/// 解析宠物栏位（0–19，STAGE-346）。
fn parse_pet_slot(text: &str) -> Result<u8, String> {
    match text.parse::<u8>() {
        Ok(slot) if slot <= MAX_PET_SLOT => Ok(slot),
        _ => Err("宠物栏位要在 0–19 之间".to_string()),
    }
}

/// 解析 `magic` 的参数：`(精灵栏位, 宠物栏位)`，宠物栏位为 None 表示变回自己。
fn parse_args(args: &[&str]) -> Result<(u8, Option<u8>), String> {
    match args {
        [slot] => Ok((parse_magic_slot(slot)?, None)),
        [slot, pet] => Ok((parse_magic_slot(slot)?, Some(parse_pet_slot(pet)?))),
        _ => Err(commands::usage_of("magic")),
    }
}

/// 解析 `magic use` 的参数（`use` 之后的部分）：`<栏位>`、`<栏位> self`、`<栏位> pet <宠物编号>`。
fn parse_use_args(args: &[&str]) -> Result<(u8, Option<MagicTo>), String> {
    match args {
        [slot] => Ok((parse_magic_slot(slot)?, None)),
        [slot, "self"] => Ok((parse_magic_slot(slot)?, Some(MagicTo::SelfOnly))),
        [slot, "pet", pet] => Ok((parse_magic_slot(slot)?, Some(MagicTo::Pet(parse_pet_slot(pet)?)))),
        _ => Err(commands::usage_of("magic")),
    }
}

impl App {
    /// 文字命令 `magic`：参数不对时在消息栏提示用法。`magic use` 走通用写法。
    pub(super) fn magic_command(&mut self, rest: &str) {
        let args: Vec<&str> = rest.split_whitespace().collect();
        if args.first() == Some(&"use") {
            match parse_use_args(&args[1..]) {
                Ok((slot, to)) => self.send_magic_use(slot, to),
                Err(msg) => self.log.error(msg),
            }
            return;
        }
        match parse_args(&args) {
            Ok((slot, pet)) => self.use_magic(slot, pet),
            Err(msg) => self.log.error(msg),
        }
    }

    /// 变幻的精灵的命令写法（`magic <栏位> [宠物编号]`）：宠物编号要换成对象。
    fn use_magic(&mut self, slot: u8, pet: Option<u8>) {
        self.send_magic_use(slot, pet.map(MagicTo::Pet));
    }

    /// 发出战斗外使用精灵的请求（协议 5.51）。战斗中不能用；栏位、宠物是否存在、能不能变身都由服务端判断并回复原因。
    pub(super) fn send_magic_use(&mut self, slot: u8, to: Option<MagicTo>) {
        if self.world.as_ref().is_some_and(|w| w.battle.is_some()) {
            return self.log.error("战斗中不能这样做");
        }
        self.send("magic_use", use_data(slot, to), Pending::MagicUse);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_slot_and_optional_pet() {
        assert_eq!(parse_args(&["0", "1"]), Ok((0, Some(1))), "带宠物编号是变成那只宠物");
        assert_eq!(parse_args(&["4"]), Ok((4, None)), "不带宠物编号是变回自己");
    }

    #[test]
    fn rejects_bad_slots_and_extra_args() {
        assert!(parse_args(&["5"]).is_err(), "精灵栏位最多到 4");
        assert!(parse_args(&["0", "20"]).is_err(), "宠物栏位最多到 19");
        assert!(parse_args(&["-1"]).is_err());
        assert!(parse_args(&["一"]).is_err());
        assert!(parse_args(&[]).is_err(), "没有参数要给用法");
        assert!(parse_args(&["0", "1", "2"]).is_err());
    }

    #[test]
    fn use_command_takes_self_or_pet() {
        assert_eq!(parse_use_args(&["1"]), Ok((1, None)), "不带对象就是默认写法");
        assert_eq!(parse_use_args(&["1", "self"]), Ok((1, Some(MagicTo::SelfOnly))));
        assert_eq!(parse_use_args(&["1", "pet", "2"]), Ok((1, Some(MagicTo::Pet(2)))));
        assert!(parse_use_args(&["1", "pet"]).is_err(), "pet 要带宠物编号");
        assert!(parse_use_args(&["1", "member", "0"]).is_err(), "一期没有队友写法");
        assert!(parse_use_args(&["1", "pet", "20"]).is_err());
        assert!(parse_use_args(&[]).is_err());
    }

    #[test]
    fn request_body_matches_protocol() {
        assert_eq!(use_data(2, None), json!({"slot": 2}));
        assert_eq!(use_data(1, Some(MagicTo::SelfOnly)), json!({"slot": 1, "to": "self"}));
        assert_eq!(use_data(1, Some(MagicTo::Pet(2))), json!({"slot": 1, "to": "pet", "target": 2}));
    }

    #[test]
    fn error_hints_cover_the_codes_the_page_meets() {
        assert_eq!(magic_use_error_hint("NOT_ENOUGH_MP"), Some("MP 不够，用不了这个精灵"));
        assert!(magic_use_error_hint("INVALID_TARGET").is_some());
        assert!(magic_use_error_hint("MAGIC_UNUSABLE").is_some());
        assert_eq!(magic_use_error_hint("SOMETHING_ELSE"), None);
    }
}
