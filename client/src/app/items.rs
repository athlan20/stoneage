//! 「背包」页的命令（STAGE-29 的穿脱整理与 STAGE-30 的使用消耗品，
//! `docs/ui.md` 背包页 B、C，`docs/protocol.md` 5.25–5.29）：
//! `item use <槽位> [目标]` 使用消耗品（目标默认自己，1–5 是宠物，6–10 是队友、一期没有），
//! `item equip <编号>` 穿上（背包格 5–19），`item unequip <编号>` 卸下（装备格 0–4），
//! `item move <从> <到>` 移动或互换，`item drop <编号> [数量] yes` 丢弃（不带 yes 先问一句，输入 y 才发请求，见 `confirm.rs`）。
//! 格子的变化由 `inventory_changed` 推送告诉界面，血量、气力由 `status_changed`，
//! 宠物的血量由 `pets_changed` 更新。

use serde_json::json;

use super::commands;
use super::confirm::Confirm;
use super::App;
use super::session::Pending;

/// 用法提示，从命令表取（STAGE-26）。
pub(super) fn usage() -> String {
    commands::usage_of("item")
}

/// 背包格编号的上限（0–19，`docs/protocol.md` 5.25）。
const MAX_SLOT: u8 = 19;
/// 装备格的上限（0–4）。
const MAX_EQUIP_SLOT: u8 = 4;

/// 穿脱整理请求（`Pending::ItemOp` 携带）：成功后写进消息栏的内容。名字在发出时就取好，推送之后就查不到了。
#[derive(Debug, Clone)]
pub enum ItemOp {
    Use { name: String, target: u8 },
    Equip { name: String },
    Unequip { name: String },
    Move { from: u8, to: u8 },
    Drop { slot: u8, name: String, count: Option<u32> },
}

/// 解析格子编号（0–19）。
pub(super) fn parse_slot(text: &str) -> Result<u8, String> {
    match text.parse::<u8>() {
        Ok(slot) if slot <= MAX_SLOT => Ok(slot),
        _ => Err("格子编号要在 0–19 之间".to_string()),
    }
}

/// 解析丢弃的数量：正整数。
fn parse_count(text: &str) -> Result<u32, String> {
    match text.parse::<u32>() {
        Ok(n) if n >= 1 => Ok(n),
        _ => Err("数量要是正整数".to_string()),
    }
}

/// `item drop` 的参数：格子、可选的数量、是不是带了 yes。
#[derive(Debug, PartialEq, Eq)]
struct DropArgs {
    slot: u8,
    count: Option<u32>,
    confirmed: bool,
}

/// 解析 `drop <编号> [数量] [yes]`；yes 可以放在最后。
fn parse_drop(args: &[&str]) -> Result<DropArgs, String> {
    let (confirmed, args) = match args.split_last() {
        Some((&"yes", rest)) => (true, rest),
        _ => (false, args),
    };
    match args {
        [slot] => Ok(DropArgs { slot: parse_slot(slot)?, count: None, confirmed }),
        [slot, count] => Ok(DropArgs { slot: parse_slot(slot)?, count: Some(parse_count(count)?), confirmed }),
        _ => Err(usage()),
    }
}

    /// 解析 `use <槽位> [目标]`：目标 0 是自己（缺省），1–5 是宠物，6–10 是队友。
    /// 目标只在本地解析成数字，合不合法（组队、宠物栏位、物品的目标类型）由服务端判断。
    fn parse_use(args: &[&str]) -> Result<(u8, u8), String> {
        match args {
            [slot] => Ok((parse_slot(slot)?, 0)),
            [slot, target] => match target.parse::<u8>() {
                Ok(t) => Ok((parse_slot(slot)?, t)),
                Err(_) => Err("目标编号要是数字（0 自己，1–5 宠物，6–10 队友）".to_string()),
            },
            _ => Err(usage()),
        }
    }

impl App {
    /// `item` 命令：子命令不认识时给出用法。分发见 `world.rs` 的命令表（STAGE-26）。
    pub(super) fn item_command(&mut self, rest: &str) {
        let args: Vec<&str> = rest.split_whitespace().collect();
        let Some((&sub, tail)) = args.split_first() else {
            self.log.error(usage());
            return;
        };
        match (sub, tail) {
            ("use", rest) => match parse_use(rest) {
                Ok((slot, target)) => self.use_item(slot, target),
                Err(msg) => self.log.error(msg),
            },
            ("equip", [slot]) => self.equip_item(slot),
            ("unequip", [slot]) => self.unequip_item(slot),
            ("move", [from, to]) => self.move_item(from, to),
            ("inlay", tail) => self.inlay_command(tail),
            ("drop", args) => match parse_drop(args) {
                Ok(args) => self.drop_item(args),
                Err(msg) => self.log.error(msg),
            },
            _ => self.log.error(usage()),
        }
    }

    /// `item use <槽位> [目标]`：槽位要是背包格（5–19）且格子上有东西。
    /// STAGE-132 起背包页的「使用」动作也走这里。
    pub(super) fn use_item(&mut self, slot: u8, target: u8) {
        if slot <= MAX_EQUIP_SLOT {
            return self.log.error("使用物品要选背包格（5–19），装备格里的已经穿着");
        }
        let Some(name) = self.item_name_at(slot) else {
            return self.log.error(format!("{slot} 号格子没有物品"));
        };
        let mut data = json!({"slot": slot});
        if target != 0 {
            data["target"] = json!(target);
        }
        self.send("item_use", data, Pending::ItemOp(ItemOp::Use { name, target }));
    }

    /// 背包里某格的物品名字（没有物品为 None）。
    pub(super) fn item_name_at(&self, slot: u8) -> Option<String> {
        self.world.as_ref()?.inventory.slots.iter().find(|s| s.slot == slot).map(|s| s.name.clone())
    }

    /// 背包里某格的物品（没有物品为 None）。STAGE-132 的背包动作要读数量和目标类型。
    pub(super) fn item_at(&self, slot: u8) -> Option<crate::proto::InventorySlot> {
        self.world.as_ref()?.inventory.slots.iter().find(|s| s.slot == slot).cloned()
    }

    /// `item equip <编号>`：编号要是背包格（5–19）。观察模式的拦截由命令表驱动（STAGE-26）。
    fn equip_item(&mut self, slot: &str) {
        let slot = match parse_slot(slot) {
            Ok(slot) => slot,
            Err(msg) => return self.log.error(msg),
        };
        self.equip_slot(slot);
    }

    /// 穿上 `slot`（背包格 5–19）。命令和 STAGE-132 的背包动作共用。
    pub(super) fn equip_slot(&mut self, slot: u8) {
        if slot <= MAX_EQUIP_SLOT {
            return self.log.error("穿上要选背包格（5–19），装备格里的已经穿着");
        }
        let Some(name) = self.item_name_at(slot) else {
            return self.log.error(format!("{slot} 号格子没有物品"));
        };
        self.send("item_equip", json!({"slot": slot}), Pending::ItemOp(ItemOp::Equip { name }));
    }

    /// `item unequip <编号>`：编号要是装备格（0–4）。
    fn unequip_item(&mut self, slot: &str) {
        let slot = match parse_slot(slot) {
            Ok(slot) => slot,
            Err(msg) => return self.log.error(msg),
        };
        self.unequip_slot(slot);
    }

    /// 卸下 `slot`（装备格 0–4）。命令和 STAGE-132 的背包动作共用。
    pub(super) fn unequip_slot(&mut self, slot: u8) {
        if slot > MAX_EQUIP_SLOT {
            return self.log.error("卸下要选装备格（0–4）");
        }
        let Some(name) = self.item_name_at(slot) else {
            return self.log.error(format!("{slot} 号格子没有装备"));
        };
        self.send("item_unequip", json!({"slot": slot}), Pending::ItemOp(ItemOp::Unequip { name }));
    }

    /// `item move <从> <到>`：两个格子都要是 0–19；相同的格子不发请求。
    fn move_item(&mut self, from: &str, to: &str) {
        let (from, to) = match (parse_slot(from), parse_slot(to)) {
            (Ok(from), Ok(to)) => (from, to),
            (Err(msg), _) | (_, Err(msg)) => return self.log.error(msg),
        };
        self.move_slot(from, to);
    }

    /// 把 `from` 的东西移到 `to`（相同格子、源格为空时不发请求）。命令和 STAGE-132 的背包动作共用。
    pub(super) fn move_slot(&mut self, from: u8, to: u8) {
        if from == to {
            return self.log.error("源格子和目标格子是同一个");
        }
        if self.item_name_at(from).is_none() {
            return self.log.error(format!("{from} 号格子没有物品"));
        }
        self.send("item_move", json!({"from": from, "to": to}), Pending::ItemOp(ItemOp::Move { from, to }));
    }

    /// `item drop <编号> [数量] [yes]`：没有 yes 先问一句，等玩家确认（`Confirm::ItemDrop`）；带 yes 直接丢弃。
    fn drop_item(&mut self, args: DropArgs) {
        let DropArgs { slot, count, confirmed } = args;
        let Some(name) = self.item_name_at(slot) else {
            return self.log.error(format!("{slot} 号格子没有物品"));
        };
        if confirmed {
            return self.send_item_drop(slot, name, count);
        }
        let what = count.map_or_else(|| name.clone(), |n| format!("{name} ×{n}"));
        let text = format!("丢弃后不能恢复：{what}（{slot} 号）");
        self.ask_confirm(Confirm::ItemDrop { slot, name, count }, &text);
    }

    /// 发出丢弃请求（直接带 yes，或者确认之后）。
    pub(super) fn send_item_drop(&mut self, slot: u8, name: String, count: Option<u32>) {
        let mut data = json!({"slot": slot, "confirm": "yes"});
        if let Some(n) = count {
            data["count"] = json!(n);
        }
        self.send("item_drop", data, Pending::ItemOp(ItemOp::Drop { slot, name, count }));
    }

    /// 使用与穿脱整理成功：格子和数值已经由推送更新，这里只写一句话到消息栏。
    /// 使用消耗品的回复量取自响应（`delta` 是血量变化，`mp_delta` 是气力变化，STAGE-30）。
    pub(super) fn on_item_op(&mut self, op: ItemOp, data: &serde_json::Value) {
        match op {
            ItemOp::Use { name, target } => {
                let delta = data.get("delta").and_then(|v| v.as_i64()).unwrap_or(0);
                let mp = data.get("mp_delta").and_then(|v| v.as_i64()).unwrap_or(0);
                let who = match data.get("name").and_then(|v| v.as_str()) {
                    Some(n) => n.to_string(),
                    None => match target {
                        0 => "自己".to_string(),
                        t => format!("{} 号宠物", t),
                    },
                };
                let mut text = format!("对 {who} 使用了 {name}");
                if delta > 0 {
                    text.push_str(&format!("，血量回复 {delta}"));
                }
                if mp > 0 {
                    text.push_str(&format!("，气力回复 {mp}"));
                }
                let loyalty = data.get("loyalty_delta").and_then(|v| v.as_i64()).unwrap_or(0);
                if loyalty > 0 {
                    text.push_str(&format!("，忠诚度上升 {loyalty}"));
                }
                self.log.system(text);
            }
            ItemOp::Equip { name } => self.log.system(format!("已穿上 {name}")),
            ItemOp::Unequip { name } => self.log.system(format!("已卸下 {name}")),
            ItemOp::Move { from, to } => self.log.system(format!("已把 {from} 号的物品移到 {to} 号")),
            ItemOp::Drop { slot, name, count } => match count {
                Some(n) => self.log.system(format!("已丢弃 {name} ×{n}（{slot} 号）")),
                None => self.log.system(format!("已丢弃 {name}（{slot} 号）")),
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_slots() {
        assert_eq!(parse_slot("0"), Ok(0));
        assert_eq!(parse_slot("19"), Ok(19));
        assert!(parse_slot("20").is_err());
        assert!(parse_slot("-1").is_err());
    }

    #[test]
    fn parses_drop_with_optional_count_and_yes() {
        assert_eq!(parse_drop(&["6"]), Ok(DropArgs { slot: 6, count: None, confirmed: false }));
        assert_eq!(parse_drop(&["6", "yes"]), Ok(DropArgs { slot: 6, count: None, confirmed: true }));
        assert_eq!(parse_drop(&["6", "2", "yes"]), Ok(DropArgs { slot: 6, count: Some(2), confirmed: true }));
        assert_eq!(parse_drop(&["6", "2"]), Ok(DropArgs { slot: 6, count: Some(2), confirmed: false }));
        assert!(parse_drop(&["6", "0", "yes"]).unwrap_err().contains("正整数"));
        assert_eq!(parse_drop(&[]), Err(usage()));
        assert_eq!(parse_drop(&["6", "2", "3", "yes"]), Err(usage()));
    }

    #[test]
    fn parses_use_with_optional_target() {
        assert_eq!(parse_use(&["7"]), Ok((7, 0)));
        assert_eq!(parse_use(&["7", "2"]), Ok((7, 2)));
        assert_eq!(parse_use(&["7", "5"]), Ok((7, 5)));
        assert_eq!(parse_use(&["7", "9"]), Ok((7, 9))); // 合不合法由服务端判断
        assert!(parse_use(&["7", "x"]).unwrap_err().contains("数字"));
        assert_eq!(parse_use(&["7", "2", "3"]), Err(usage()));
        assert_eq!(parse_use(&[]), Err(usage()));
    }
}
