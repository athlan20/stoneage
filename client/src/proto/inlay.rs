//! 精工（STAGE-299，`docs/protocol.md` 5.73）：背包格子对象的 `inlay` 字段，以及 `item_inlay` 的响应。
//! 只有类别码非空的物品才带 `inlay`；当前数据下没有任何物品带它，界面据此决定要不要出现「精工」入口。

use serde::Deserialize;

use super::InventorySlot;

/// 格子上的精工信息。`role` 为 `gear`（可镶装备）或 `material`（材料）。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct Inlay {
    #[serde(default)]
    pub role: String,
    /// 还能镶几格（0–3）；只对装备有意义。
    #[serde(default)]
    pub free: u8,
    /// 三个镶嵌格的类别码，空格是 `null`；只对装备有意义。
    #[serde(default)]
    pub codes: Vec<Option<String>>,
}

impl Inlay {
    /// 可镶的武器或防具。
    pub fn is_gear(&self) -> bool {
        self.role == "gear"
    }

    /// 精工材料。
    pub fn is_material(&self) -> bool {
        self.role == "material"
    }
}

/// `item_inlay` 的响应（成功）或错误的 `error.data`（`INLAY_FULL`）：`applied` 是实际镶上的件数，
/// `consumed` 是被扣掉的材料格（按请求顺序），`item` 是镶完之后的目标。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct InlayReply {
    #[serde(default)]
    pub applied: u32,
    #[serde(default)]
    pub consumed: Vec<u8>,
    #[serde(default)]
    pub item: Option<InventorySlot>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn inventory_slot_reads_inlay_and_keeps_old_slots_without_it() {
        let gear: InventorySlot = serde_json::from_value(serde_json::json!({
            "slot": 5, "item_id": 20, "name": "小斧头", "effect": "攻+11", "type": 1, "count": 1,
            "inlay": {"role": "gear", "free": 1, "codes": ["RUBY", "RUBY", null]}
        }))
        .unwrap();
        let inlay = gear.inlay.clone().expect("带 inlay");
        assert!(inlay.is_gear() && !inlay.is_material());
        assert_eq!(inlay.free, 1);
        assert_eq!(
            inlay.codes,
            vec![Some("RUBY".to_string()), Some("RUBY".to_string()), None]
        );

        let plain: InventorySlot =
            serde_json::from_value(serde_json::json!({"slot": 6, "name": "药草", "count": 2})).unwrap();
        assert!(plain.inlay.is_none(), "没有 inlay 字段时保持 None");
    }

    #[test]
    fn reply_reads_applied_consumed_and_item() {
        let reply: InlayReply = serde_json::from_value(serde_json::json!({
            "applied": 2, "consumed": [6, 7],
            "item": {"slot": 5, "name": "轻的斧头", "inlay": {"role": "gear", "free": 1, "codes": ["RUBY", "RUBY", null]}}
        }))
        .unwrap();
        assert_eq!(reply.applied, 2);
        assert_eq!(reply.consumed, vec![6, 7]);
        assert_eq!(reply.item.map(|it| it.slot), Some(5));

        let material: Inlay = serde_json::from_value(serde_json::json!({"role": "material"})).unwrap();
        assert!(material.is_material() && !material.is_gear());
    }
}
