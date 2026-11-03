
    use super::*;

    fn item(kind: u32, count: u32, target: TargetType) -> InventorySlot {
        InventorySlot { slot: 7, name: "物".into(), kind, count, target_type: target, ..Default::default() }
    }

    #[test]
    fn equip_slot_offers_unequip_only() {
        let acts = actions(0, &item(7, 1, TargetType::None));
        assert_eq!(acts.first(), Some(&Act::Unequip));
        assert!(!acts.contains(&Act::Equip), "装备格里的已经穿着");
    }

    #[test]
    fn bag_slot_lists_actions_by_kind() {
        // 能穿斧头：装备 + 移动 + 丢弃 + 说明
        let acts = actions(7, &item(1, 1, TargetType::None));
        assert_eq!(acts, vec![Act::Equip, Act::Move, Act::Drop, Act::Describe]);
        // 回复药：使用 + 移动 + 丢弃 + 说明
        let acts = actions(7, &item(30, 3, TargetType::Ally));
        assert_eq!(acts, vec![Act::Use, Act::Move, Act::Drop, Act::Describe]);
        // 材料：只有移动、丢弃、说明
        let acts = actions(7, &item(5, 1, TargetType::None));
        assert_eq!(acts, vec![Act::Move, Act::Drop, Act::Describe]);
    }

    #[test]
    fn stacking_equipment_is_disabled_with_a_reason() {
        let it = item(1, 3, TargetType::None);
        assert_eq!(disabled_reason(Act::Equip, &it), Some("数量不是 1，先拆开再装备"));
        assert_eq!(disabled_reason(Act::Equip, &item(1, 1, TargetType::None)), None);
    }

    #[test]
    fn self_and_all_targets_do_not_ask() {
        assert_eq!(use_targets(&item(30, 1, TargetType::SelfOnly)), UseTargets::Direct(0));
        assert_eq!(use_targets(&item(30, 1, TargetType::AllyAll)), UseTargets::Direct(0));
        assert_eq!(use_targets(&item(30, 1, TargetType::Ally)), UseTargets::Pick { down_only: false });
        assert_eq!(use_targets(&item(30, 1, TargetType::AllyDown)), UseTargets::Pick { down_only: true });
    }
