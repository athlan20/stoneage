//! 精工（STAGE-299，`docs/inlay.md` 2.8、`docs/protocol.md` 5.73）：背包动作「精工」和命令 `item inlay` 共用这里的函数。
//! 目标是可镶的武器或防具（`inlay.role = gear`，在背包或穿着都行），材料是背包里 `role = material` 的格子；
//! 每次最多 3 件材料，且不超过目标还剩的空格。请求发出前先问一句（确认窗口默认取消）；
//! 成功、部分成功的文字写进消息栏，格子和数值由 `inventory_changed` 等推送更新。

use serde_json::{Value, json};

use super::App;
use super::confirm::Confirm;
use super::items::{parse_slot, usage};
use super::messages::Kind;
use super::session::Pending;
use crate::proto::{Inlay, InlayReply, InventorySlot, ProtoError};

/// 每次最多几件材料（目标加材料共 4 件，`docs/protocol.md` 5.73）。
pub const MAX_MATERIALS: usize = 3;

/// 精工请求（`Pending::ItemInlay` 携带）：目标名字在发出时取好，成功后写消息栏用。
#[derive(Debug, Clone)]
pub struct InlayOp {
    pub target_name: String,
}

/// 目标格还剩的空格；不是可镶装备返回 None。
pub fn gear_free(it: &InventorySlot) -> Option<u8> {
    it.inlay.as_ref().filter(|i| i.is_gear()).map(|i| i.free)
}

/// 材料格（`role = material`）。
pub fn is_material(it: &InventorySlot) -> bool {
    it.inlay.as_ref().is_some_and(Inlay::is_material)
}

/// 最多能勾几件材料：剩余空格与 3 件取小。
pub fn max_pick(free: u8) -> usize {
    usize::from(free).min(MAX_MATERIALS)
}

impl App {
    /// 精工的一次请求（动作面板与命令共用）：`confirmed` 为真直接发出，否则先问一句（默认取消，`Confirm::Inlay`）。
    /// 格子是否有东西在这里查；类别码、格数、镶满等由服务端判断。
    pub(super) fn request_inlay(&mut self, target: u8, materials: Vec<u8>, confirmed: bool) {
        let Some(target_item) = self.item_at(target) else {
            return self.log.error(format!("{target} 号格子没有物品"));
        };
        let mut names = Vec::with_capacity(materials.len());
        for &slot in &materials {
            let Some(it) = self.item_at(slot) else {
                return self.log.error(format!("{slot} 号格子没有物品"));
            };
            names.push(it.name);
        }
        if confirmed {
            return self.send_inlay(target, materials, target_item.name);
        }
        let text = format!(
            "将 {} 镶到 {} 上，永久占用 {} 格、可能覆盖原有精灵",
            names.join("、"),
            target_item.name,
            materials.len()
        );
        self.ask_confirm(
            Confirm::Inlay {
                target,
                target_name: target_item.name,
                materials,
            },
            &text,
        );
    }

    /// 发出精工请求；推送先于响应，响应只写消息栏。
    pub(super) fn send_inlay(&mut self, target: u8, materials: Vec<u8>, target_name: String) {
        self.send(
            "item_inlay",
            json!({"target": target, "materials": materials}),
            Pending::ItemInlay(InlayOp { target_name }),
        );
    }

    /// 全部镶上（`ok: true`）：绿色一句话。
    pub(super) fn on_inlay_ok(&mut self, op: InlayOp, data: &Value) {
        let applied = reply_of(data).applied;
        self.log
            .push(Kind::Inlay(true), format!("{} 镶上了 {applied} 件材料", op.target_name));
    }

    /// 失败（含中途镶满）：镶满且已经镶上几件时写“镶了 N 件，其余没有镶上：…”，其余直接用服务端台词；都是黄色。
    pub(super) fn on_inlay_error(&mut self, err: &ProtoError) {
        let msg = if err.message.is_empty() {
            format!("请求失败（{}）", err.code)
        } else {
            err.message.clone()
        };
        let applied = if err.code == "INLAY_FULL" {
            reply_of(&err.data).applied
        } else {
            0
        };
        let text = if applied > 0 {
            format!("镶了 {applied} 件，其余没有镶上：{msg}")
        } else {
            msg
        };
        self.log.push(Kind::Inlay(false), text);
    }
}

/// `item inlay` 的参数：目标格、材料格、是不是带了 yes。
#[derive(Debug, PartialEq, Eq)]
pub struct InlayArgs {
    pub target: u8,
    pub materials: Vec<u8>,
    pub confirmed: bool,
}

/// 解析 `inlay <目标格> <材料格…> [yes]`；yes 可以放在最后。格子是否有东西、类别码等由服务端判断。
pub fn parse_inlay(args: &[&str]) -> Result<InlayArgs, String> {
    let (confirmed, args) = match args.split_last() {
        Some((&"yes", rest)) => (true, rest),
        _ => (false, args),
    };
    let [target, mats @ ..] = args else {
        return Err(usage());
    };
    if mats.is_empty() {
        return Err(usage());
    }
    let target = parse_slot(target)?;
    let mut materials = Vec::with_capacity(mats.len());
    for text in mats {
        let slot = parse_slot(text)?;
        if !(5..=19).contains(&slot) {
            return Err("材料要选背包格（5–19）".to_string());
        }
        if slot == target {
            return Err("目标和材料不能是同一格".to_string());
        }
        if materials.contains(&slot) {
            return Err("材料格不能重复".to_string());
        }
        materials.push(slot);
    }
    if materials.len() > MAX_MATERIALS {
        return Err("精工每次只能选择四个物品。".to_string());
    }
    Ok(InlayArgs {
        target,
        materials,
        confirmed,
    })
}

impl App {
    /// `item inlay <目标格> <材料格…> [yes]`（别名 `精工`，命令表 `item` 与 `inlay` 共用）：没有 yes 先问一句，带 yes 直接发出。
    pub(super) fn inlay_command(&mut self, args: &[&str]) {
        match parse_inlay(args) {
            Ok(a) => self.request_inlay(a.target, a.materials, a.confirmed),
            Err(msg) => self.log.error(msg),
        }
    }
}

/// 响应或错误里的 `applied` 等字段；读不出来就当 0。
fn reply_of(data: &Value) -> InlayReply {
    serde_json::from_value(data.clone()).unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_target_materials_and_yes() {
        assert_eq!(
            parse_inlay(&["5", "6", "7"]),
            Ok(InlayArgs {
                target: 5,
                materials: vec![6, 7],
                confirmed: false
            })
        );
        assert_eq!(
            parse_inlay(&["5", "6", "yes"]),
            Ok(InlayArgs {
                target: 5,
                materials: vec![6],
                confirmed: true
            })
        );
        assert_eq!(
            parse_inlay(&["2", "6", "7", "8", "yes"]),
            Ok(InlayArgs {
                target: 2,
                materials: vec![6, 7, 8],
                confirmed: true
            }),
            "穿着的装备也能当目标（0–4）"
        );
    }

    #[test]
    fn rejects_bad_shapes_before_sending() {
        assert_eq!(parse_inlay(&[]), Err(usage()));
        assert_eq!(parse_inlay(&["5"]), Err(usage()));
        assert_eq!(parse_inlay(&["5", "yes"]), Err(usage()));
        assert!(parse_inlay(&["20", "6"]).unwrap_err().contains("0–19"));
        assert!(parse_inlay(&["5", "4"]).unwrap_err().contains("背包格"));
        assert!(parse_inlay(&["5", "6", "6"]).unwrap_err().contains("重复"));
        assert!(parse_inlay(&["5", "5"]).unwrap_err().contains("同一格"));
        assert_eq!(
            parse_inlay(&["5", "6", "7", "8", "9"]),
            Err("精工每次只能选择四个物品。".to_string())
        );
    }

    #[test]
    fn gear_and_material_are_read_from_inlay_role() {
        let gear: InventorySlot = serde_json::from_value(serde_json::json!({"slot": 5, "name": "斧", "inlay": {"role": "gear", "free": 2, "codes": [null, null, null]}})).unwrap();
        let mat: InventorySlot =
            serde_json::from_value(serde_json::json!({"slot": 6, "name": "红宝石", "inlay": {"role": "material"}}))
                .unwrap();
        let plain: InventorySlot = serde_json::from_value(serde_json::json!({"slot": 7, "name": "药"})).unwrap();
        assert_eq!(gear_free(&gear), Some(2));
        assert_eq!(gear_free(&mat), None);
        assert!(is_material(&mat) && !is_material(&plain));
        assert_eq!(max_pick(2), 2);
        assert_eq!(max_pick(3), 3);
    }
}
