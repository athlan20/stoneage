//! 托管的自动复活（STAGE-262 B 段）：出战宠倒下、来源可用时，用复活精灵或复活道具把它救起来。
//!
//! 只管出战宠（单位号 `you + 5`）；人物倒下是战斗失败，不归这里。顺序在 `auto.rs` 里排在自动逃跑之后、
//! 人物补血之前（宠物倒下少一个输出，比回几点血更紧急）。
//!
//! - 精灵：`target_type == ally_down` 且 MP 够，按这个字段认，不按名字；「复活后优先带防御的精灵」开着时，
//!   防御类（复活光 / 镜 / 守护，效果文字带「次物理攻击」）排前面，同组里取栏位最靠前的。
//! - 道具：`target_type == ally_down` 的，取效果文字里数字最小的一份（复活药 100 → 300 → 500，回魂之玉 600）。
//! - 每只宠每场最多试 2 次；被服务端拒绝的来源本场拉黑（精灵记栏位，道具记槽位，沿用 `bad_slots`）。

use std::collections::{BTreeMap, BTreeSet};

use super::BattleState;
use super::auto::AutoMove;
use super::command::Act;
use crate::app::WorldState;
use crate::app::auto_battle::{AutoProfile, ReviveSource};
use crate::proto::{BattleUnit, MagicView, TargetType};

/// 同一只宠每场最多试几次复活（B5）。
pub(super) const MAX_TRIES: u32 = 2;

/// 一场战斗里的复活记录（整场一个，随 `BattleState` 新建）。
#[derive(Debug, Default)]
pub struct ReviveLog {
    /// 每只宠已经发出过几次复活。
    tries: BTreeMap<u8, u32>,
    /// 本场被服务端拒绝过的复活精灵栏位。
    bad_magic: BTreeSet<u8>,
    /// 上一条托管指令是不是复活（被拒时据此拉黑精灵栏位）。
    pending: bool,
    /// 已经写过「复活失败」的宠物（每只宠每场只写一次）。
    noted: BTreeSet<u8>,
}

impl ReviveLog {
    fn tries(&self, pet: u8) -> u32 {
        self.tries.get(&pet).copied().unwrap_or(0)
    }
}

/// 出战宠的单位号（协议 10.5：己方的宠物是人物编号 +5）。
fn pet_unit(b: &BattleState) -> u8 {
    b.you + 5
}

/// 出战宠倒下、人物还活着时，返回这只宠（人物倒下就不处理）。
fn down_pet(b: &BattleState) -> Option<&BattleUnit> {
    if b.units.get(&b.you).is_some_and(|u| u.hp_pct == 0) {
        return None;
    }
    b.units.get(&pet_unit(b)).filter(|u| u.hp_pct == 0)
}

/// 自动复活的一招。开关关着、宠物没倒下、试满了或没有可用来源时返回 `None`（行为与改动前一致）。
pub(super) fn revive_move(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<AutoMove> {
    if !cfg.recover.revive_on {
        return None;
    }
    let pet = down_pet(b)?;
    let pet_no = pet_unit(b);
    if b.revive.tries(pet_no) >= MAX_TRIES {
        return None;
    }
    let source = cfg.recover.revive_source;
    if source != ReviveSource::ItemOnly
        && let Some(m) = pick_magic(b, w, cfg)
    {
        return Some(AutoMove {
            act: Act::Magic,
            target: Some(pet_no),
            arg: Some(i64::from(m.slot)),
            reason: format!("复活 {}（{}）", pet.name, m.name),
        });
    }
    if source != ReviveSource::MagicOnly
        && let Some((slot, name)) = pick_item(b, w)
    {
        return Some(AutoMove {
            act: Act::Item,
            target: Some(pet_no),
            arg: Some(i64::from(slot)),
            reason: format!("复活 {}（{}）", pet.name, name),
        });
    }
    None
}

/// 复活精灵：`ally_down`、MP 够、本场没被拒过的；防御类优先（开关开着时），再取栏位最靠前的。
fn pick_magic<'a>(b: &BattleState, w: &'a WorldState, cfg: &AutoProfile) -> Option<&'a MagicView> {
    let mp = w.entry.status.mp;
    let defend_first = cfg.recover.revive_defend;
    w.entry
        .status
        .magic
        .iter()
        .filter(|m| m.target_type == TargetType::AllyDown && m.mp <= mp && !b.revive.bad_magic.contains(&m.slot))
        .min_by_key(|m| (defend_first && !is_defend_revive(m), m.slot))
}

/// 防御类复活精灵（复活光 / 镜 / 守护）：效果文字是「…复活…；…N 次物理攻击…」，只有它带「次物理攻击」
/// （`server/gamedata/magic_effect.go` 的 `defendText`）。没有效果文字（旧服务端）时按名字认。
pub(super) fn is_defend_revive(m: &MagicView) -> bool {
    if m.effect.is_empty() {
        return ["复活光", "复活镜", "复活守护"].iter().any(|k| m.name.contains(k));
    }
    m.effect.contains("次物理攻击")
}

/// 复活道具：`ally_down`、本场没被拒过的，效果文字里的数字最小的先用（大药不救小伤）；同数字取槽位靠前的。
fn pick_item<'a>(b: &BattleState, w: &'a WorldState) -> Option<(u8, &'a str)> {
    w.inventory
        .slots
        .iter()
        .filter(|s| (5..=19).contains(&s.slot) && !b.bad_slots.contains(&s.slot))
        .filter(|s| s.target_type == TargetType::AllyDown)
        .filter_map(|s| revive_power(&s.effect).map(|p| (p, s.slot, s.name.as_str())))
        .min_by_key(|(p, slot, _)| (*p, *slot))
        .map(|(_, slot, name)| (slot, name))
}

/// 复活道具的数值：效果文字里第一个数字（「气绝回复成耐力100」「从气绝回复成600的耐久力」都取到 100 / 600）。
/// 不用 `heal_power`：它只认“耐 / 体”后面的数字，认不出“回复成600”。
pub(super) fn revive_power(effect: &str) -> Option<u32> {
    let digits: String = effect
        .chars()
        .skip_while(|c| !c.is_ascii_digit())
        .take_while(|c| c.is_ascii_digit())
        .collect();
    digits.parse().ok().filter(|n| *n > 0)
}

/// 这条指令是不是复活（对出战宠用道具或精灵，且它确实倒下）。
pub(super) fn is_revive(b: &BattleState, mv: &AutoMove) -> bool {
    matches!(mv.act, Act::Item | Act::Magic) && mv.target == Some(pet_unit(b)) && down_pet(b).is_some()
}

/// 发出去之后记账：本次是不是复活（供被拒时拉黑精灵），复活的话这只宠的试数加一。
pub(super) fn note_issued(b: &mut BattleState, mv: &AutoMove) {
    let reviving = is_revive(b, mv);
    b.revive.pending = reviving;
    if reviving {
        let pet = pet_unit(b);
        *b.revive.tries.entry(pet).or_default() += 1;
    }
}

/// 复活被服务端拒了：精灵栏位记入本场黑名单（道具由 `bad_slots` 处理）。
pub(super) fn note_rejected(b: &mut BattleState, act: Act, arg: Option<i64>) {
    let pending = std::mem::take(&mut b.revive.pending);
    if pending
        && act == Act::Magic
        && let Some(slot) = arg.and_then(|n| u8::try_from(n).ok())
    {
        b.revive.bad_magic.insert(slot);
    }
}

/// 出战宠倒下、复活开着，但这回合没有可用的复活（没来源或试满了）：返回宠物号和一句「复活失败」，
/// 每只宠每场只说一次（B6）。有能用的复活时返回 `None`。
pub(super) fn give_up_note(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<(u8, String)> {
    if !cfg.recover.revive_on {
        return None;
    }
    let pet = down_pet(b)?;
    let pet_no = pet_unit(b);
    if b.revive.noted.contains(&pet_no) || revive_move(b, w, cfg).is_some() {
        return None;
    }
    let why = if b.revive.tries(pet_no) >= MAX_TRIES {
        format!("已试 {MAX_TRIES} 次，{}仍倒下", pet.name)
    } else {
        format!("没有可用的复活来源（{}仍倒下）", pet.name)
    };
    Some((pet_no, format!("复活失败：{why}")))
}

/// 「复活失败」写过了，这只宠不再重复写。
pub(super) fn mark_noted(b: &mut BattleState, pet: u8) {
    b.revive.noted.insert(pet);
}

#[cfg(test)]
mod tests {
    use super::super::Phase;
    use super::super::auto::plan;
    use super::super::command::Actor;
    use super::*;
    use crate::proto::{BattleStart, InventorySlot};

    /// 人物 0（血量满）、出战宠 5（`pet_pct` 为 0 就是倒下）、敌人 15。
    fn battle(player_pct: u32, pet_pct: u32) -> BattleState {
        let unit = |no: u8, name: &str, pct: u32| BattleUnit {
            no,
            kind: if no == 0 { "player" } else { "pet" }.into(),
            name: name.into(),
            hp_pct: pct,
            ..BattleUnit::default()
        };
        let units = vec![
            unit(0, "勇者", player_pct),
            unit(5, "贝鲁卡", pet_pct),
            unit(15, "野猪", 100),
        ];
        let mut b = BattleState::from_start(BattleStart {
            scene: "野外".into(),
            you: 0,
            units,
            ..BattleStart::default()
        });
        b.phase = Phase::Input(Actor::Player);
        b
    }

    /// 复活精灵：`ally_down`，效果文字照服务端 `magic_effect.go` 写。
    fn magic(slot: u8, name: &str, mp: u32, effect: &str) -> MagicView {
        MagicView {
            slot,
            name: name.into(),
            mp,
            target_type: TargetType::AllyDown,
            effect: effect.into(),
            ..Default::default()
        }
    }

    fn plain(slot: u8) -> MagicView {
        magic(slot, "复活精灵 Lv1", 8, "复活己方一个倒下的单位，HP 约 100")
    }

    fn defend(slot: u8) -> MagicView {
        magic(
            slot,
            "复活守护精灵 Lv1",
            12,
            "复活己方一个倒下的单位，HP 约 100，己方一个接下的 2 次物理攻击无效",
        )
    }

    /// 背包复活道具（`ally_down`），效果文字照物品表写。
    fn item(slot: u8, name: &str, effect: &str) -> InventorySlot {
        InventorySlot {
            slot,
            name: name.into(),
            effect: effect.into(),
            target_type: TargetType::AllyDown,
            ..InventorySlot::default()
        }
    }

    fn world(mp: u32, magics: Vec<MagicView>, items: Vec<InventorySlot>) -> WorldState {
        let mut w = WorldState::new(Default::default());
        w.entry.status.mp = mp;
        w.entry.status.magic = magics;
        w.inventory.slots = items;
        w
    }

    fn cfg_with(f: impl FnOnce(&mut AutoProfile)) -> AutoProfile {
        let mut c = AutoProfile::default();
        f(&mut c);
        c
    }

    #[test]
    fn revive_prefers_magic_then_falls_back_to_items() {
        let b = battle(100, 0);
        let w = world(50, vec![plain(2)], vec![item(6, "复活药(100)", "气绝回复成耐力100")]);
        let mv = revive_move(&b, &w, &AutoProfile::default()).expect("有精灵该用精灵");
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Magic, Some(2), Some(5)));
        assert!(mv.reason.contains("贝鲁卡"), "{}", mv.reason);
        // MP 不够：精灵不用，退回道具。
        let w = world(5, vec![plain(2)], vec![item(6, "复活药(100)", "气绝回复成耐力100")]);
        let mv = revive_move(&b, &w, &AutoProfile::default()).expect("MP 不够退道具");
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Item, Some(6), Some(5)));
    }

    #[test]
    fn only_magic_or_only_item_source_is_respected() {
        let b = battle(100, 0);
        let both = || world(50, vec![plain(2)], vec![item(6, "复活药(100)", "气绝回复成耐力100")]);
        let magic_only = cfg_with(|c| c.recover.revive_source = ReviveSource::MagicOnly);
        assert_eq!(revive_move(&b, &both(), &magic_only).map(|m| m.act), Some(Act::Magic));
        let no_magic = world(50, vec![], vec![item(6, "复活药(100)", "气绝回复成耐力100")]);
        assert!(
            revive_move(&b, &no_magic, &magic_only).is_none(),
            "只用精灵：包里有药也不吃"
        );
        let item_only = cfg_with(|c| c.recover.revive_source = ReviveSource::ItemOnly);
        assert_eq!(revive_move(&b, &both(), &item_only).map(|m| m.act), Some(Act::Item));
        let no_item = world(50, vec![plain(2)], vec![]);
        assert!(
            revive_move(&b, &no_item, &item_only).is_none(),
            "只用道具：有精灵也不用"
        );
    }

    #[test]
    fn defend_revive_comes_first_only_when_the_switch_is_on() {
        let b = battle(100, 0);
        let w = world(50, vec![plain(2), defend(4)], vec![]);
        assert_eq!(
            revive_move(&b, &w, &AutoProfile::default()).unwrap().arg,
            Some(4),
            "开着：防御类优先"
        );
        let off = cfg_with(|c| c.recover.revive_defend = false);
        assert_eq!(revive_move(&b, &w, &off).unwrap().arg, Some(2), "关着：只看栏位");
        assert!(is_defend_revive(&defend(4)) && !is_defend_revive(&plain(2)));
        // 防御类 MP 不够时取能用的普通复活精灵。
        let poor = world(10, vec![plain(2), defend(4)], vec![]);
        assert_eq!(revive_move(&b, &poor, &AutoProfile::default()).unwrap().arg, Some(2));
    }

    #[test]
    fn items_take_the_smallest_power_first() {
        assert_eq!(revive_power("气绝回复成耐力300"), Some(300));
        assert_eq!(revive_power("从气绝回复成600的耐久力"), Some(600));
        assert_eq!(revive_power("复活光精灵 气绝回复100耐久力"), Some(100));
        assert_eq!(revive_power("复活守精灵 气绝回复500耐久力"), Some(500));
        assert_eq!(
            revive_power("耐久力20前後回复"),
            Some(20),
            "数字写法只是取第一个，回复道具不会走到这里"
        );
        let b = battle(100, 0);
        let mut w = world(
            0,
            vec![],
            vec![
                item(8, "回魂之玉", "从气绝回复成600的耐久力"),
                item(7, "复活药(300)", "气绝回复成耐力300"),
                item(6, "复活药(100)", "气绝回复成耐力100"),
            ],
        );
        let c = AutoProfile::default();
        assert_eq!(revive_move(&b, &w, &c).unwrap().arg, Some(6), "100 的先用");
        w.inventory.slots.retain(|s| s.slot != 6);
        assert_eq!(revive_move(&b, &w, &c).unwrap().arg, Some(7), "没有 100 了用 300");
        w.inventory.slots.retain(|s| s.slot != 7);
        assert_eq!(revive_move(&b, &w, &c).unwrap().arg, Some(8), "最后才用回魂之玉");
    }

    #[test]
    fn revive_is_skipped_when_off_or_nothing_is_down() {
        let w = world(50, vec![plain(2)], vec![item(6, "复活药(100)", "气绝回复成耐力100")]);
        let off = cfg_with(|c| c.recover.revive_on = false);
        assert!(revive_move(&battle(100, 0), &w, &off).is_none());
        assert_eq!(
            plan(&battle(100, 0), &w, Actor::Player, &off).act,
            Act::Attack,
            "关着与改动前一致"
        );
        assert!(give_up_note(&battle(100, 0), &w, &off).is_none());
        assert!(
            revive_move(&battle(100, 100), &w, &AutoProfile::default()).is_none(),
            "宠物没倒下不产出"
        );
        assert!(
            revive_move(&battle(0, 0), &w, &AutoProfile::default()).is_none(),
            "人物倒下不处理"
        );
    }

    #[test]
    fn revive_comes_before_healing_the_player_and_items_do_not_heal_by_mistake() {
        let mut b = battle(100, 0);
        if let Some(me) = b.units.get_mut(&0) {
            me.hp = Some(10);
            me.max_hp = Some(100);
            me.hp_pct = 10;
        }
        let w = world(0, vec![], vec![item(6, "复活药(100)", "气绝回复成耐力100")]);
        // 宠物倒下：先复活宠物，不拿药回自己。
        assert_eq!(plan(&b, &w, Actor::Player, &AutoProfile::default()).target, Some(5));
        // 宠物活着：复活药不当回复药用（服务端会拒，且它是复活道具）。
        let alive = battle_alive_low_player();
        assert_eq!(
            plan(&alive, &w, Actor::Player, &AutoProfile::default()).act,
            Act::Attack
        );
    }

    fn battle_alive_low_player() -> BattleState {
        let mut b = battle(10, 100);
        if let Some(me) = b.units.get_mut(&0) {
            me.hp = Some(10);
            me.max_hp = Some(100);
        }
        b
    }

    #[test]
    fn a_rejected_magic_is_not_tried_again_this_battle() {
        let mut b = battle(100, 0);
        let w = world(50, vec![plain(2), plain(3)], vec![]);
        let c = AutoProfile::default();
        let first = revive_move(&b, &w, &c).unwrap();
        assert_eq!(first.arg, Some(2));
        note_issued(&mut b, &first);
        note_rejected(&mut b, first.act, first.arg);
        assert_eq!(revive_move(&b, &w, &c).unwrap().arg, Some(3), "被拒的栏位本场不再选");
    }

    #[test]
    fn a_rejected_item_is_blacklisted_through_bad_slots() {
        let mut b = battle(100, 0);
        let w = world(
            0,
            vec![],
            vec![
                item(6, "复活药(100)", "气绝回复成耐力100"),
                item(7, "复活药(300)", "气绝回复成耐力300"),
            ],
        );
        let c = AutoProfile::default();
        b.bad_slots.insert(6);
        assert_eq!(revive_move(&b, &w, &c).unwrap().arg, Some(7));
    }

    #[test]
    fn each_pet_gets_at_most_two_tries_per_battle() {
        let mut b = battle(100, 0);
        let w = world(50, vec![plain(2)], vec![]);
        let c = AutoProfile::default();
        for _ in 0..MAX_TRIES {
            let mv = revive_move(&b, &w, &c).expect("试数没满就该有复活");
            note_issued(&mut b, &mv);
        }
        assert!(revive_move(&b, &w, &c).is_none(), "第 3 次不再试");
        let (pet, text) = give_up_note(&b, &w, &c).expect("试满了要说一句");
        assert_eq!(pet, 5);
        assert!(text.starts_with("复活失败：已试 2 次"), "{text}");
    }

    #[test]
    fn no_source_says_so_once_and_attacks() {
        let b = battle(100, 0);
        let w = world(0, vec![], vec![]);
        let c = AutoProfile::default();
        let (pet, text) = give_up_note(&b, &w, &c).expect("没有来源要说一句");
        assert_eq!(pet, 5);
        assert_eq!(text, "复活失败：没有可用的复活来源（贝鲁卡仍倒下）");
        let mut b = b;
        mark_noted(&mut b, pet);
        assert!(give_up_note(&b, &w, &c).is_none(), "同一只宠每场只说一次");
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Attack, "照常攻击");
    }

    #[test]
    fn a_non_revive_move_is_not_counted_as_a_revive() {
        let b = battle(100, 0);
        let mv = AutoMove {
            act: Act::Attack,
            target: Some(15),
            arg: None,
            reason: String::new(),
        };
        assert!(!is_revive(&b, &mv));
        let mut b = b;
        note_issued(&mut b, &mv);
        assert!(!b.revive.pending);
    }
}
