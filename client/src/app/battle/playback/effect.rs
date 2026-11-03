//! 结算效果到画面的映射（STAGE-110，`docs/battle-ui.md`「卡片效果」）：一个效果要闪什么、抖什么、飘什么字。
//! 只看协议里的结果种类和数值，不碰单位状态；节拍和血量在 `playback.rs`。

use crate::app::battle::{FloatKind, FxKind, slots};
use crate::proto::BattleEffect;

/// 效果在目标身上要做的动画（可能有多个：受伤同时新增状态）。
pub(super) fn fx_kinds(eff: &BattleEffect, actor: Option<u8>, me_side: u8) -> Vec<FxKind> {
    let mut out = vec![];
    if eff.down {
        out.push(FxKind::Down);
    } else {
        match eff.result.as_str() {
            "crit" => out.push(FxKind::Crit),
            "hit" if eff.delta < 0 => out.push(FxKind::Hurt),
            "heal" if eff.delta > 0 => out.push(FxKind::Heal),
            "dodge" | "miss" => out.push(FxKind::Dodge(dodge_dir(actor, eff.target, me_side))),
            "block" => out.push(FxKind::Block),
            "absorb" if eff.delta > 0 => out.push(FxKind::Heal),
            "vanish" => out.push(FxKind::Block),
            "reflect" if eff.delta < 0 => out.push(FxKind::Hurt),
            _ => {}
        }
    }
    if !eff.status_add.is_empty() {
        out.push(FxKind::Status);
    }
    out
}

/// STAGE-167：按左右物理列避开出手者；没有出手者时朝本方外侧让开。
pub(super) fn dodge_dir(actor: Option<u8>, target: u8, me_side: u8) -> i8 {
    let to = slots::screen_column(me_side, target);
    let from = actor.map_or(to, |a| slots::screen_column(me_side, a));
    if to < from {
        -1
    } else if to > from {
        1
    } else if target / 10 == me_side {
        1
    } else {
        -1
    }
}

/// 一种效果占几段闪或抖（`slot` 的个数）。闪避不按段，持续到命中拍结束。
pub(super) fn slots_of(kind: FxKind) -> u32 {
    match kind {
        FxKind::Hurt | FxKind::Down => 3,
        FxKind::Crit | FxKind::Status => 4,
        FxKind::Block | FxKind::Heal => 2,
        FxKind::Dodge(_) => 0,
    }
}

/// 效果在目标身上飘出的字：伤害、暴击、闪避、未命中、全防、回复；没有变化的不飘。
pub(super) fn float_of(eff: &BattleEffect) -> Option<(String, FloatKind)> {
    match eff.result.as_str() {
        "crit" => Some((format!("{}!", eff.delta), FloatKind::Crit)),
        "hit" if eff.delta < 0 => Some((eff.delta.to_string(), FloatKind::Damage)),
        "heal" if eff.delta > 0 => Some((format!("+{}", eff.delta), FloatKind::Heal)),
        "dodge" => Some(("闪避".into(), FloatKind::Miss)),
        "miss" => Some(("miss".into(), FloatKind::Miss)),
        "block" => Some(("挡住".into(), FloatKind::Block)),
        "absorb" if eff.delta > 0 => Some((format!("+{}", eff.delta), FloatKind::Heal)),
        "vanish" => Some(("无效".into(), FloatKind::Block)),
        "reflect" if eff.delta < 0 => Some((eff.delta.to_string(), FloatKind::Damage)),
        _ => None,
    }
}

/// 合击的飘字：同一个目标挨的几下合成一个总数（有暴击的带“!”）；一下伤害都没有的目标按第一个效果的飘字。
pub(super) fn combo_floats(effects: &[BattleEffect]) -> Vec<(u8, String, FloatKind)> {
    let mut out: Vec<(u8, String, FloatKind)> = vec![];
    let mut seen: Vec<u8> = vec![];
    for e in effects {
        if seen.contains(&e.target) {
            continue;
        }
        seen.push(e.target);
        let same: Vec<&BattleEffect> = effects.iter().filter(|x| x.target == e.target).collect();
        let total: i64 = same
            .iter()
            .filter(|x| matches!(x.result.as_str(), "hit" | "crit"))
            .map(|x| x.delta)
            .sum();
        if total < 0 {
            let crit = same.iter().any(|x| x.result == "crit");
            let kind = if crit {
                FloatKind::Crit
            } else {
                FloatKind::Damage
            };
            out.push((
                e.target,
                if crit {
                    format!("{total}!")
                } else {
                    total.to_string()
                },
                kind,
            ));
        } else if let Some((text, kind)) = same.iter().find_map(|x| float_of(x)) {
            out.push((e.target, text, kind));
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn eff(result: &str, delta: i64) -> BattleEffect {
        BattleEffect {
            result: result.into(),
            delta,
            ..BattleEffect::default()
        }
    }

    /// STAGE-199：吸收飘回血，无效飘“无效”并挡住动作，反射在攻击者身上飘伤害。
    #[test]
    fn defense_results_float_and_flash() {
        assert_eq!(
            float_of(&eff("absorb", 5)),
            Some(("+5".into(), FloatKind::Heal))
        );
        assert_eq!(
            float_of(&eff("vanish", 0)),
            Some(("无效".into(), FloatKind::Block))
        );
        assert_eq!(
            float_of(&eff("reflect", -40)),
            Some(("-40".into(), FloatKind::Damage))
        );
        assert_eq!(fx_kinds(&eff("absorb", 5), Some(15), 0), vec![FxKind::Heal]);
        assert_eq!(
            fx_kinds(&eff("vanish", 0), Some(15), 0),
            vec![FxKind::Block]
        );
        assert_eq!(
            fx_kinds(&eff("reflect", -40), Some(15), 0),
            vec![FxKind::Hurt]
        );
    }
}
