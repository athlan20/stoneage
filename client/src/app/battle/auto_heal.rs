//! 托管的恢复动作（STAGE-173 M06 补血、M07 补气，取代 STAGE-42 写死的 30%）。
//!
//! 顺序照需求「基础执行规则」：人物自己补血 → 战宠补血（人物喂药）→ 人物补气。
//! 来源顺序由方案里的「先道具 / 先精灵」决定；道具按回复量贴着缺口选，精灵只看名字与 MP。
//! 同一回合只出一条指令：这个模块返回第一条能用的，返回 None 就交给行动模块。

use super::BattleState;
use super::auto::AutoMove;
use super::command::Act;
use crate::app::WorldState;
use crate::app::auto_battle::AutoProfile;
use crate::proto::{MagicView, TargetType};

/// 人物自己补血、存活战宠补血。
pub(super) fn heal_move(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<AutoMove> {
    if let Some(mv) = heal_self(b, w, cfg) {
        return Some(mv);
    }
    if cfg.recover.pet_heal {
        return heal_pet(b, w, cfg);
    }
    None
}

/// 人物补气（M07）：MP 百分比 ≤ 门槛且背包里有补气道具时用一片。
pub(super) fn mp_move(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<AutoMove> {
    if !cfg.recover.player_mp {
        return None;
    }
    let st = &w.entry.status;
    if st.max_mp == 0 || u64::from(st.mp) * 100 > u64::from(st.max_mp) * u64::from(cfg.recover.player_mp_pct) {
        return None;
    }
    let (slot, name) = pick_mp_item(w, b, cfg)?;
    Some(AutoMove {
        act: Act::Item,
        target: Some(b.you),
        arg: Some(i64::from(slot)),
        reason: format!("用 {name} 补气"),
    })
}

fn heal_self(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<AutoMove> {
    if !cfg.recover.player_heal {
        return None;
    }
    let me = b.units.get(&b.you)?;
    let (hp, max_hp) = (me.hp?, me.max_hp?);
    if max_hp == 0 || u64::from(hp) * 100 > u64::from(max_hp) * u64::from(cfg.recover.player_heal_pct) {
        return None;
    }
    let missing = max_hp.saturating_sub(hp);
    let by_item = || {
        pick_heal_item(w, b, missing, false).map(|(slot, name, _, kind)| AutoMove {
            act: Act::Item,
            target: heal_target(kind, b.you),
            arg: Some(i64::from(slot)),
            reason: format!("用 {name} 回复"),
        })
    };
    let by_magic = || {
        pick_heal_magic(w, cfg).map(|m| AutoMove {
            act: Act::Magic,
            target: heal_target(m.target_type, b.you),
            arg: Some(i64::from(m.slot)),
            reason: format!("用 {} 回复", m.name),
        })
    };
    if cfg.recover.item_first { by_item().or_else(by_magic) } else { by_magic().or_else(by_item) }
}

/// 存活的出战宠低血时由人物处理：喂药，或对宠物放精灵（来源顺序同人物补血，看 `item_first`）。
/// 宠物自己只能攻击/技能/防御，补血必须人物出手。
fn heal_pet(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<AutoMove> {
    let pet_no = b.you + 5;
    let pet = b.units.get(&pet_no)?;
    if pet.hp_pct == 0 {
        return None; // 倒下的宠物不喂药：由自动复活（auto_revive）先救起来
    }
    let (hp, max_hp) = (pet.hp?, pet.max_hp?);
    if max_hp == 0 || u64::from(hp) * 100 > u64::from(max_hp) * u64::from(cfg.recover.pet_heal_pct) {
        return None;
    }
    let missing = max_hp.saturating_sub(hp);
    let by_item = || {
        pick_heal_item(w, b, missing, true).map(|(slot, name, _, _)| AutoMove {
            act: Act::Item,
            target: Some(pet_no),
            arg: Some(i64::from(slot)),
            reason: format!("用 {name} 回复 {}", pet.name),
        })
    };
    let by_magic = || {
        pick_pet_heal_magic(w, cfg).map(|m| AutoMove {
            act: Act::Magic,
            // 己方全体的（恩惠）不带目标，单体治疗他人的（滋润）指定宠物。
            target: heal_target(m.target_type, pet_no),
            arg: Some(i64::from(m.slot)),
            reason: format!("用 {} 回复 {}", m.name, pet.name),
        })
    };
    if cfg.recover.item_first { by_item().or_else(by_magic) } else { by_magic().or_else(by_item) }
}

/// 能给战宠补血的精灵：沿用 `pick_heal_magic` 的取舍，但「只能对自己」的（治愈的精灵）落不到宠物身上，
/// 直接不要它。玩家把补血精灵指定成这类时也返回 None（不偷偷换成别的），退回喂药。
fn pick_pet_heal_magic<'a>(w: &'a WorldState, cfg: &AutoProfile) -> Option<&'a MagicView> {
    pick_heal_magic(w, cfg).filter(|m| m.target_type != TargetType::SelfOnly)
}

/// 回复指令带不带目标：要选己方一个的（治疗他人）带上目标，其余（自愈、全体）不带。
fn heal_target(kind: TargetType, unit: u8) -> Option<u8> {
    kind.picks_unit().then_some(unit)
}

/// 背包里的 HP 回复道具。`for_pet` 为真时只要“能选己方一个/全体”的（`self` 的只能自己吃）。
/// 选回复量最贴近缺口的：够补的最小，都不够补的最大；本场被拒过的槽位与战斗中不能用的跳过。
fn pick_heal_item<'a>(
    w: &'a WorldState,
    b: &BattleState,
    missing: u32,
    for_pet: bool,
) -> Option<(u8, &'a str, u32, TargetType)> {
    let mut best: Option<(u8, &'a str, u32, TargetType)> = None;
    for s in w.inventory.slots.iter().filter(|s| (5..=19).contains(&s.slot) && !b.bad_slots.contains(&s.slot)) {
        // 复活道具（ally_down）归自动复活，不当回复用：它的效果文字“气绝回复成耐力100”会被误认成回血。
        if matches!(s.target_type, TargetType::None | TargetType::AllyDown) {
            continue;
        }
        if for_pet && !matches!(s.target_type, TargetType::Ally | TargetType::AllyAll) {
            continue;
        }
        let Some(power) = heal_power(&s.effect) else { continue };
        let better = best.is_none_or(|(_, _, cur, _)| heal_better(power, cur, missing));
        if better {
            best = Some((s.slot, s.name.as_str(), power, s.target_type));
        }
    }
    best
}

/// 背包里的补气道具：效果文字里带“气”的回复量、战斗中能用，且在白名单里（M07，空名单表示不限）。
/// 槽位最靠前的优先。
fn pick_mp_item(w: &WorldState, b: &BattleState, cfg: &AutoProfile) -> Option<(u8, String)> {
    let mut best: Option<(u8, &str)> = None;
    for s in w.inventory.slots.iter().filter(|s| (5..=19).contains(&s.slot) && !b.bad_slots.contains(&s.slot)) {
        if matches!(s.target_type, TargetType::None | TargetType::AllyDown) || !s.effect.contains("回复") {
            continue;
        }
        if !cfg.recover.mp_item_allowed(&s.name) {
            continue; // 不在白名单里
        }
        if mp_power(&s.effect).is_none() {
            continue;
        }
        if best.is_none_or(|(slot, _)| s.slot < slot) {
            best = Some((s.slot, s.name.as_str()));
        }
    }
    best.map(|(slot, name)| (slot, name.to_string()))
}

/// 还有没有补给（M11「补给不足」）：背包里剩战斗内可用的回复/补气道具，或用得起的治愈系精灵。
/// 判定只看“还有没有”，不看缺口多少——门槛留给补血补气自己决定用哪一件。
pub(super) fn has_supply(w: &WorldState, cfg: &AutoProfile) -> bool {
    let item = w.inventory.slots.iter().any(|s| {
        (5..=19).contains(&s.slot)
            && !matches!(s.target_type, TargetType::None | TargetType::AllyDown)
            && s.effect.contains("回复")
            && (heal_power(&s.effect).is_some() || mp_power(&s.effect).is_some())
    });
    item || pick_heal_magic(w, cfg).is_some()
}

/// 补血用的精灵（STAGE-173，玩家可指定）。
///
/// 方案里指定了「补血精灵」就**只认那一个**：栏位和名字都对得上才算数（装备换了、精灵换了就失效），
/// MP 不够时也不偷偷换成别的治愈精灵——直接不用精灵，交给道具。
/// 没指定（或指定的已经不在列表里）就自动认回血精灵：己方全体的（恩惠）优先，其次取栏位最靠前、MP 够的那个。
/// 全体优先是 STAGE-254 的取舍：一次回人物和宠物，省回合；代价是低等级全体精灵的回复量可能小于单体的。
fn pick_heal_magic<'a>(w: &'a WorldState, cfg: &AutoProfile) -> Option<&'a MagicView> {
    let mp = w.entry.status.mp;
    if !cfg.recover.heal_magic_name.is_empty() {
        let picked = w
            .entry
            .status
            .magic
            .iter()
            .find(|m| m.slot == cfg.recover.heal_magic_slot && m.name == cfg.recover.heal_magic_name);
        if let Some(m) = picked {
            // 选成攻击/复活类精灵就不拿它补血（服务端会拒），也别每回合白发一条指令。
            if matches!(m.target_type, TargetType::Enemy | TargetType::EnemyAll | TargetType::AllyDown) {
                return None;
            }
            return (m.mp <= mp).then_some(m);
        }
    }
    w.entry
        .status
        .magic
        .iter()
        .filter(|m| is_heal_magic(m) && m.mp <= mp)
        .min_by_key(|m| (m.target_type != TargetType::AllyAll, m.slot))
}

/// 回血精灵（STAGE-254）。认效果文字：服务端 `magic_effect.go` 给 heal / heal_other 写的是
/// 「回复自己 HP N」「回复己方一个 HP N」「回复己方全体 HP N」，与名字无关（「恩惠的精灵」就靠这个认出来）。
/// 没有效果文字时（旧服务端）才退回按名字关键字认。
fn is_heal_magic(m: &MagicView) -> bool {
    if m.effect.is_empty() {
        return HEAL_MAGIC_NAMES.iter().any(|k| m.name.contains(k));
    }
    HEAL_MAGIC_EFFECTS.iter().any(|p| m.effect.starts_with(p))
}

const HEAL_MAGIC_NAMES: [&str; 5] = ["治愈", "滋润", "恩惠", "回复", "恢复"];
const HEAL_MAGIC_EFFECTS: [&str; 3] = ["回复自己 HP", "回复己方一个 HP", "回复己方全体 HP"];

/// 候选药效比现选的好不好：能覆盖缺口的优先；都能覆盖时小的优先（大药不花在小伤上），
/// 都覆盖不了时大的优先。
fn heal_better(candidate: u32, current: u32, missing: u32) -> bool {
    let (c, m) = (candidate >= missing, current >= missing);
    if c != m {
        c
    } else if c {
        candidate < current
    } else {
        candidate > current
    }
}

/// 效果文字里解析 HP 回复量（旧 `itemset.txt` 的写法见 `power_after`）。
pub(super) fn heal_power(effect: &str) -> Option<u32> {
    power_after(effect, &['耐', '体'])
}

/// 效果文字里解析 MP 回复量：“气力60前後回复”“气10耐100回复”里的“气”那个数字。
fn mp_power(effect: &str) -> Option<u32> {
    power_after(effect, &['气'])
}

/// 按给定关键字取它后面的数字，取最大。物品表的写法有“耐久力100前後回复”“耐50回复(1体)”
/// “体1000回复 (自己)”“气10耐100回复(1体)”“耐久力回复5”（数字写在“回复”后面），
/// 都按“关键字（可带‘久力’‘力’‘量’）后面的数字”取；一处数字都没有时退回“回复”后面的数字。
fn power_after(effect: &str, keys: &[char]) -> Option<u32> {
    if !effect.contains("回复") {
        return None;
    }
    let chars: Vec<char> = effect.chars().collect();
    let digits_at = |mut j: usize| -> Option<u32> {
        let start = j;
        while matches!(chars.get(j), Some(c) if c.is_ascii_digit()) {
            j += 1;
        }
        (j > start).then(|| chars[start..j].iter().collect::<String>().parse().ok()).flatten()
    };
    let mut best = 0;
    for (i, c) in chars.iter().enumerate() {
        if !keys.contains(c) {
            continue;
        }
        let mut j = i + 1;
        if chars.get(j) == Some(&'久') {
            j += 1;
        }
        if chars.get(j) == Some(&'力') || chars.get(j) == Some(&'量') {
            j += 1;
        }
        if let Some(n) = digits_at(j) {
            best = best.max(n);
        }
    }
    if best == 0 && let Some(i) = effect.find("回复") {
        let j = effect[..i].chars().count() + "回复".chars().count();
        best = digits_at(j).unwrap_or(0);
    }
    (best > 0).then_some(best)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn heal_power_reads_the_itemset_wordings() {
        assert_eq!(heal_power("耐久力20前後回复"), Some(20));
        assert_eq!(heal_power("耐100回复(1体)"), Some(100));
        assert_eq!(heal_power("体1000 回复(自己)"), Some(1000));
        assert_eq!(heal_power("气10耐100回复(1体)"), Some(100), "混合药取耐的量");
        assert_eq!(heal_power("耐久力回复5"), Some(5), "数字写在回复后面的");
        assert_eq!(heal_power("耐久力1000前後回复"), Some(1000));
        assert_eq!(heal_power("气力60前後回复"), None, "纯 MP 药不算 HP 药");
        assert_eq!(heal_power("攻 +9 防 -3 敏 -3"), None, "装备效果不算");
        assert_eq!(heal_power("宠物的忠诚度+30上升"), None);
        assert_eq!(heal_power(""), None);
    }

    #[test]
    fn mp_power_reads_the_mp_wordings() {
        assert_eq!(mp_power("气力60前後回复"), Some(60));
        assert_eq!(mp_power("气10耐100回复(1体)"), Some(10));
        assert_eq!(mp_power("耐100回复(1体)"), None);
    }

    #[test]
    fn heal_target_is_only_given_when_a_unit_is_picked() {
        assert_eq!(heal_target(TargetType::Ally, 0), Some(0), "治疗他人要选己方一个，选自己");
        assert_eq!(heal_target(TargetType::SelfOnly, 0), None, "只能对自己的药不带目标");
        assert_eq!(heal_target(TargetType::AllyAll, 0), None, "全体不带目标");
        assert_eq!(heal_target(TargetType::None, 0), None);
    }

    /// STAGE-173 M07：补气物品白名单。空名单＝不限（按槽位先后的自动挑）。
    #[test]
    fn mp_item_allowlist_limits_what_is_used() {
        use crate::app::WorldState;
        use crate::proto::{BattleStart, InventorySlot};

        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.entry.status.mp = 5;
        w.entry.status.max_mp = 100;
        for (slot, name, effect) in
            [(5u8, "气药", "气30回复(1人)"), (6u8, "大汽水", "气力60回复(1人)"), (7u8, "肉", "耐100回复(1体)")]
        {
            w.inventory.slots.push(InventorySlot {
                slot,
                name: name.into(),
                effect: effect.into(),
                target_type: TargetType::Ally,
                ..InventorySlot::default()
            });
        }
        let b = BattleState::from_start(BattleStart::default());
        let mut cfg = AutoProfile::default();
        cfg.recover.player_mp = true;

        assert_eq!(mp_move(&b, &w, &cfg).and_then(|m| m.arg), Some(5), "不限时用槽位最靠前的补气药");
        cfg.recover.mp_items = vec!["汽水".into()];
        assert_eq!(mp_move(&b, &w, &cfg).and_then(|m| m.arg), Some(6), "白名单外的不吃");
        cfg.recover.mp_items = vec!["不存在的药".into()];
        assert!(mp_move(&b, &w, &cfg).is_none(), "白名单里没有可用物品就不补气");
    }

    /// M11「补给不足」：背包里有可用的回复/补气道具，或用得起治愈精灵，就不算用完。
    #[test]
    fn has_supply_looks_at_items_and_heal_magic() {
        use crate::app::WorldState;
        use crate::proto::{BattleStart, InventorySlot};

        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        assert!(!has_supply(&w, &AutoProfile::default()), "空背包没有补给");
        w.inventory.slots.push(InventorySlot {
            slot: 5,
            name: "肉".into(),
            effect: "耐久力20前後回复".into(),
            target_type: TargetType::Ally,
            ..InventorySlot::default()
        });
        assert!(has_supply(&w, &AutoProfile::default()), "有回复药就算有补给");
        w.inventory.slots.clear();
        w.entry.status.mp = 30;
        w.entry.status.magic = vec![MagicView { slot: 1, name: "治愈的精灵 Lv1".into(), mp: 10, ..MagicView::default() }];
        assert!(has_supply(&w, &AutoProfile::default()), "治愈精灵也算补给");
        w.entry.status.magic = vec![MagicView { slot: 1, name: "火球的精灵 Lv1".into(), mp: 10, ..MagicView::default() }];
        assert!(!has_supply(&w, &AutoProfile::default()), "攻击精灵不算补给");
        let _ = BattleStart::default();
    }

    /// STAGE-173：方案里指定了补血精灵就只用它——哪怕栏位更靠后的那个才是「自动」会选的。
    #[test]
    fn the_chosen_heal_magic_wins_over_the_automatic_pick() {
        use crate::app::WorldState;

        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.entry.status.mp = 100;
        // 两个都能补血：0 号「治愈的精灵 Lv1」、3 号「滋润的精灵 Lv3」。
        w.entry.status.magic = vec![
            MagicView { slot: 0, name: "治愈的精灵 Lv1".into(), mp: 10, ..MagicView::default() },
            MagicView { slot: 3, name: "滋润的精灵 Lv3".into(), mp: 30, ..MagicView::default() },
        ];
        let b = BattleState::from_start(crate::proto::BattleStart::default());
        let mut cfg = AutoProfile::default();
        cfg.recover.player_heal = true;

        // 没指定：按栏位取 0 号。
        assert_eq!(pick_heal_magic(&w, &cfg).map(|m| m.slot), Some(0));
        // 指定 3 号：就用它。
        cfg.recover.heal_magic_slot = 3;
        cfg.recover.heal_magic_name = "滋润的精灵 Lv3".into();
        assert_eq!(pick_heal_magic(&w, &cfg).map(|m| m.slot), Some(3));

        // 指定的那个 MP 不够：不偷偷换回 0 号，这一回合不用精灵。
        w.entry.status.mp = 5;
        assert!(pick_heal_magic(&w, &cfg).is_none());

        // 指定失效（装备换了，栏位上是别的精灵）：回落到自动认。
        w.entry.status.mp = 100;
        w.entry.status.magic[1].name = "火球的精灵 Lv3".into();
        assert_eq!(pick_heal_magic(&w, &cfg).map(|m| m.slot), Some(0));
        let _ = b;
    }

    /// 指定了补血精灵且 MP 够，算「有补给」；MP 不够就不算。
    #[test]
    fn supply_counts_the_chosen_magic_only_when_affordable() {
        use crate::app::WorldState;

        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.entry.status.magic =
            vec![MagicView { slot: 1, name: "治愈的精灵 Lv1".into(), mp: 20, ..MagicView::default() }];
        let mut cfg = AutoProfile::default();
        cfg.recover.heal_magic_slot = 1;
        cfg.recover.heal_magic_name = "治愈的精灵 Lv1".into();

        w.entry.status.mp = 30;
        assert!(has_supply(&w, &cfg));
        w.entry.status.mp = 10;
        assert!(!has_supply(&w, &cfg), "MP 不够就不算补给");
    }

    /// 补血精灵选成了攻击类：不拿它补血（服务端会拒），也不用别的精灵顶替。
    #[test]
    fn an_attack_spell_chosen_as_heal_is_not_used() {
        use crate::app::WorldState;

        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.entry.status.mp = 100;
        w.entry.status.magic = vec![
            MagicView {
                slot: 0,
                name: "火球的精灵 Lv3".into(),
                mp: 10,
                target_type: TargetType::Enemy,
                ..MagicView::default()
            },
            MagicView { slot: 2, name: "治愈的精灵 Lv1".into(), mp: 10, ..MagicView::default() },
        ];
        let mut cfg = AutoProfile::default();
        cfg.recover.heal_magic_slot = 0;
        cfg.recover.heal_magic_name = "火球的精灵 Lv3".into();
        assert!(pick_heal_magic(&w, &cfg).is_none(), "指定的精灵不能补血就不用精灵");
    }

    /// STAGE-254：回血精灵按效果文字认；「恩惠的精灵」名字里没有治愈/滋润，也要认得出来。
    #[test]
    fn heal_magic_is_told_by_its_effect_not_only_its_name() {
        let spell = |name: &str, effect: &str| MagicView {
            name: name.into(),
            effect: effect.into(),
            mp: 1,
            ..MagicView::default()
        };
        assert!(is_heal_magic(&spell("恩惠的精灵 Lv3", "回复己方全体 HP 200")));
        assert!(is_heal_magic(&spell("恩惠的精靈 Lv100", "回复己方全体 HP 10000")));
        assert!(is_heal_magic(&spell("滋润的精灵 Lv1", "回复己方一个 HP 65")));
        assert!(is_heal_magic(&spell("治愈的精灵 Lv1", "回复自己 HP 80")));
        assert!(!is_heal_magic(&spell("火球的精灵 Lv1", "攻击敌方一个，威力 80，火属性")));
        assert!(
            !is_heal_magic(&spell("某某的精灵 Lv1", "己方一个挨下的 3 次物理攻击改为吸收（回复伤害值的血量）")),
            "效果里提到“回复”的不算回血"
        );
        // 没有效果文字（旧服务端）：退回按名字认，恩惠也认。
        assert!(is_heal_magic(&spell("恩惠的精灵 Lv3", "")));
        assert!(!is_heal_magic(&spell("火球的精灵 Lv1", "")));
    }

    /// STAGE-254：没指定补血精灵时，己方全体的（恩惠）优先于单体的（治愈），哪怕它栏位更靠后；它 MP 不够才退回单体。
    #[test]
    fn automatic_pick_prefers_the_ally_all_heal_magic() {
        use crate::app::WorldState;

        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.entry.status.mp = 100;
        w.entry.status.magic = vec![
            MagicView {
                slot: 0,
                name: "治愈的精灵 Lv3".into(),
                effect: "回复自己 HP 320".into(),
                mp: 20,
                target_type: TargetType::SelfOnly,
                ..MagicView::default()
            },
            MagicView {
                slot: 2,
                name: "恩惠的精灵 Lv3".into(),
                effect: "回复己方全体 HP 200".into(),
                mp: 30,
                target_type: TargetType::AllyAll,
                ..MagicView::default()
            },
        ];
        let cfg = AutoProfile::default();
        assert_eq!(pick_heal_magic(&w, &cfg).map(|m| m.slot), Some(2));

        w.entry.status.mp = 25;
        assert_eq!(pick_heal_magic(&w, &cfg).map(|m| m.slot), Some(0), "恩惠 MP 不够就用治愈");
    }

    // ---- 战宠补血的精灵分支（本轮修复：原来只有喂药一条路）----

    /// 一只 10/100 的战宠 + 血满的人物；精灵按参数放进去。
    fn battle_with_low_pet(pet_hp: u32) -> BattleState {
        use crate::proto::{BattleStart, BattleUnit};
        let unit = |no: u8, name: &str, hp: u32, max_hp: u32, kind: &str| BattleUnit {
            no,
            name: name.into(),
            kind: kind.into(),
            hp: Some(hp),
            max_hp: Some(max_hp),
            hp_pct: hp * 100 / max_hp,
            ..BattleUnit::default()
        };
        BattleState::from_start(BattleStart {
            you: 0,
            units: vec![
                unit(0, "勇者", 100, 100, "player"),
                unit(5, "乌力", pet_hp, 100, "pet"),
                unit(15, "野猪", 100, 100, "monster"),
            ],
            ..BattleStart::default()
        })
    }

    fn world_with_magic(spells: Vec<MagicView>) -> WorldState {
        let mut w = WorldState::new(Default::default());
        w.entry.status.mp = 100;
        w.entry.status.magic = spells;
        w
    }

    fn spell(slot: u8, name: &str, effect: &str, mp: u32, target_type: TargetType) -> MagicView {
        MagicView { slot, name: name.into(), effect: effect.into(), mp, target_type, ..MagicView::default() }
    }

    /// 人物血满、宠物低血：用己方全体的精灵（恩惠），不带目标。
    #[test]
    fn pet_is_healed_by_the_ally_all_magic() {
        let b = battle_with_low_pet(10);
        let w = world_with_magic(vec![spell(2, "恩惠的精灵 Lv3", "回复己方全体 HP 200", 30, TargetType::AllyAll)]);
        let cfg = AutoProfile::default();
        let mv = heal_move(&b, &w, &cfg).expect("应当给战宠补血");
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Magic, Some(2), None), "全体精灵不指定目标");
        assert!(mv.reason.contains("乌力"), "{}", mv.reason);
    }

    /// 单体治疗他人的精灵（滋润）要把目标指到宠物身上，不能落在自己。
    #[test]
    fn single_target_heal_magic_targets_the_pet() {
        let b = battle_with_low_pet(10);
        let w = world_with_magic(vec![spell(1, "滋润的精灵 Lv1", "回复己方一个 HP 65", 10, TargetType::Ally)]);
        let cfg = AutoProfile::default();
        let mv = heal_move(&b, &w, &cfg).expect("应当给战宠补血");
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Magic, Some(1), Some(5)), "目标是被喂的宠（5 号）");
    }

    /// 只能对自己的治愈精灵落不到宠物身上：退回没有可用手段，这回合不出补血指令。
    #[test]
    fn self_only_heal_magic_is_not_used_on_the_pet() {
        let b = battle_with_low_pet(10);
        let w = world_with_magic(vec![spell(0, "治愈的精灵 Lv3", "回复自己 HP 320", 20, TargetType::SelfOnly)]);
        assert!(heal_move(&b, &w, &AutoProfile::default()).is_none(), "治愈只治自己，不能算宠物的补血");
    }

    /// 回复来源顺序也管战宠：先道具时优先喂药。
    #[test]
    fn pet_heal_follows_the_item_first_order() {
        use crate::proto::InventorySlot;
        let b = battle_with_low_pet(10);
        let mut w = world_with_magic(vec![spell(2, "恩惠的精灵 Lv3", "回复己方全体 HP 200", 30, TargetType::AllyAll)]);
        w.inventory.slots.push(InventorySlot {
            slot: 5,
            name: "普通的肉".into(),
            effect: "耐久力40前後回复".into(),
            target_type: TargetType::Ally,
            ..InventorySlot::default()
        });
        let cfg = AutoProfile::default(); // 先精灵
        assert_eq!(heal_move(&b, &w, &cfg).map(|m| m.act), Some(Act::Magic));
        let mut item_first = cfg.clone();
        item_first.recover.item_first = true;
        assert_eq!(heal_move(&b, &w, &item_first).map(|m| m.act), Some(Act::Item));
    }
}
