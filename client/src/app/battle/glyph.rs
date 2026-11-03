//! 战斗动作层用到的字符（STAGE-167）：武器投射物、接触点、精灵属性、异常形态。
//! 全部集中在 `docs/battle-ui.md`「出手动作层」「精灵与状态」两张表里，改字符只改这一处。

use super::motion::Outcome;
use super::spell::{Elem, SpellFx, SpellKind};

/// 飞行物的字符：不同武器用不同轮廓，不能都画成一个光球。
pub fn projectile_glyph(weapon: &str, unicode: bool) -> char {
    match (weapon, unicode) {
        ("bow", true) => '➤',
        ("bow", false) => '>',
        ("throw_axe", true) => '✚',
        ("throw_axe", false) => 'x',
        ("throw_stone", true) => '●',
        ("throw_stone", false) => 'o',
        ("boomerang", true) => '◈',
        ("boomerang", false) => '*',
        (_, true) => '·',
        (_, false) => '.',
    }
}

/// 接触点的标记：五种结果各一个字符，不靠颜色区分。
/// 闪开是身体离开接触点（`»`），落空是攻击落在旁边（`´`），挡住停在盾面（`⊘`）。
pub fn contact_glyph(outcome: Outcome, unicode: bool) -> char {
    match (outcome, unicode) {
        (Outcome::Crit, true) => '✦',
        (Outcome::Crit, false) => '*',
        (Outcome::Block, true) => '⊘',
        (Outcome::Block, false) => '#',
        (Outcome::Dodge, true) => '»',
        (Outcome::Dodge, false) => '>',
        (Outcome::Miss, true) => '´',
        (Outcome::Miss, false) => '`',
        (Outcome::Hit, true) => '·',
        (Outcome::Hit, false) => '.',
    }
}

/// 地水火风的形态字符。四种必须不同，不能只靠颜色区分；关掉 Unicode 时也要能分开。
pub fn element_glyph(elem: Elem, unicode: bool) -> char {
    match (elem, unicode) {
        (Elem::Earth, true) => '▲',  // 岩柱
        (Elem::Earth, false) => '^',
        (Elem::Water, true) => '≈',  // 水珠涟漪
        (Elem::Water, false) => '~',
        (Elem::Fire, true) => '♨',   // 上升火舌
        (Elem::Fire, false) => '!',
        (Elem::Wind, true) => '≋',   // 回旋风弧
        (Elem::Wind, false) => '/',
        (Elem::None, true) => '◇',
        (Elem::None, false) => 'o',
    }
}

/// 六种异常各自的施加形态：毒滴、电纹、浮泡、硬壳、摇摆、相向箭头。
/// 这是「刚挂上」的一瞬间，卡片上的持续标记仍用 `card.rs` 的单字。
pub fn status_glyph(status: &str, unicode: bool) -> char {
    match (status, unicode) {
        ("poison", true) => '☠',
        ("poison", false) => '%',
        ("paralysis", true) => 'ϟ', // 能压在字里的宽度，Emoji 的 ⚡ 是两格，会把卡片边框挤开
        ("paralysis", false) => '!',
        ("sleep", true) => 'z',
        ("sleep", false) => 'z',
        ("stone", true) => '▓',
        ("stone", false) => '#',
        ("drunk", true) => '~',
        ("drunk", false) => '~',
        ("confusion", true) => '↔',
        ("confusion", false) => '<',
        (_, true) => '◆',
        (_, false) => '+',
    }
}

/// 这一类精灵在目标身上该画什么：治疗是往上，复活是从倒地起身，净化是分解，异常是形态。
pub fn effect_glyph(fx: &SpellFx, unicode: bool) -> char {
    match fx.kind {
        SpellKind::Damage => element_glyph(fx.elem, unicode),
        SpellKind::HealSelf | SpellKind::HealOther => {
            if unicode {
                '↑'
            } else {
                '+'
            }
        }
        SpellKind::Revive => {
            if unicode {
                '✝'
            } else {
                '*'
            }
        }
        SpellKind::Cure => {
            if unicode {
                '×'
            } else {
                'x'
            }
        }
        SpellKind::Status => status_glyph(fx.status.as_deref().unwrap_or(""), unicode),
        SpellKind::Neutral => {
            if unicode {
                '◆'
            } else {
                '+'
            }
        }
    }
}
