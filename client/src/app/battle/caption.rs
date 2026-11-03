//! 中线字幕（STAGE-110，`docs/battle-ui.md`「中线字幕」）：一条事件写成一行“谁 ━━ 动作 ━━▶ 谁  结果”。
//! 起手拍只写到动作和目标，命中拍再补上结果；这里只给出分段和颜色类别，颜色、ASCII 降级和截断由界面决定。

use super::slots;
use super::{BattleState, Part, Tone};
use crate::proto::{BattleEffect, BattleEvent};

/// 有字幕、会播两拍的事件种类；其余（不认识的种类）只更新状态和记录，不写字幕、不播动画（协议 10.7 的兼容规则）。
pub fn known(kind: &str) -> bool {
    matches!(
        kind,
        "attack"
            | "combo"
            | "counter"
            | "magic"
            | "item"
            | "skill"
            | "guard"
            | "capture"
            | "escape"
            | "swap_pet"
            | "skip"
            | "status_tick"
            | "disobey"
            | "pet_escape"
    )
}

/// 一条事件的字幕。`hit` 为 false 是起手拍：不写结果，捕捉和逃跑写“· · ·”。
pub fn build(b: &BattleState, ev: &BattleEvent, hit: bool) -> Vec<Part> {
    if ev.kind == "status_tick" {
        return status_tick(b, ev, hit);
    }
    let mut out = ev.actor.map(|a| who(b, a, 1)).unwrap_or_default();
    if ev.kind == "combo" {
        out = combo_actors(b, ev);
    }
    match ev.kind.as_str() {
        "guard" => out.push(Part::new("  防御", Tone::Verb)),
        "swap_pet" => out.extend(swap(ev)),
        "skip" => out.push(Part::new(format!("  {}", skip_text(ev.reason.as_deref())), Tone::Dim)),
        "disobey" => out.push(Part::new(
            format!("  {}", disobey_text(ev.reason.as_deref())),
            Tone::Verb,
        )),
        "pet_escape" => out.push(Part::new("  跑掉了", Tone::Verb)),
        "magic" if dragon_call(ev) => out.extend(dragon(b, ev, hit)),
        "capture" => out.extend(catch(b, ev, hit)),
        "escape" => {
            out.push(Part::new("  ", Tone::Plain));
            out.push(Part::new("逃跑", Tone::Verb));
            out.extend(verdict(hit, ev.ok, " "));
        }
        other => {
            let verb = match other {
                "counter" => "反击".to_string(),
                "combo" => "合击".to_string(),
                "magic" => format!("【{}】", ev.name.as_deref().unwrap_or("精灵")),
                "item" => format!("【{}】", ev.name.as_deref().unwrap_or("道具")),
                "skill" => format!("【{}】", ev.name.as_deref().unwrap_or("技能")),
                _ => "攻击".to_string(),
            };
            out.extend(act(b, ev, &verb, hit));
        }
    }
    out
}

/// 攻击、精灵、道具、技能：` ━━ 动作 ━━▶ 目标  结果`。一个目标时写名字和结果（同一目标的几次效果依次列出）；
/// 多个目标时写“敌方全体 ×3”，数字只飘在各自的卡片上；没有目标时只写动作。
fn act(b: &BattleState, ev: &BattleEvent, verb: &str, hit: bool) -> Vec<Part> {
    let targets = targets(ev);
    let mut out = vec![];
    match targets.as_slice() {
        [] => out.push(Part::new(format!("  {verb}"), Tone::Verb)),
        [one] => {
            out.extend(arrow(verb));
            if Some(*one) == ev.actor {
                out.push(Part::new("自己", Tone::Plain));
            } else {
                out.extend(who(b, *one, 2));
            }
            if hit {
                out.extend(results(b, &ev.effects));
            }
        }
        many => {
            out.extend(arrow(verb));
            let mine = slots::cell_of(many[0]).side == b.me_side();
            let side = if mine { "己方" } else { "敌方" };
            let tone = if mine { Tone::Mine } else { Tone::Foe };
            out.push(Part::new(format!("{side}全体 ×{}", many.len()), tone));
        }
    }
    out
}

/// 召唤龙阵（STAGE-202，`magic.func` 为 `dragon`）：字幕是“XX 召唤了龙，打向 YY”，不写成“用 精灵 → 目标”。
fn dragon_call(ev: &BattleEvent) -> bool {
    ev.magic.as_ref().is_some_and(|m| m.func == "dragon")
}

/// 召唤龙阵的字幕：一个目标时写名字和结果（同 `act`），多个目标写“敌方全体 ×N”，数字飘在各自的卡片上。
fn dragon(b: &BattleState, ev: &BattleEvent, hit: bool) -> Vec<Part> {
    let targets = targets(ev);
    let mut out = vec![Part::new("  召唤了龙，打向", Tone::Verb)];
    match targets.as_slice() {
        [] => {}
        [one] => {
            out.push(Part::new(" ", Tone::Plain));
            out.extend(who(b, *one, 2));
            if hit {
                out.extend(results(b, &ev.effects));
            }
        }
        many => {
            let mine = slots::cell_of(many[0]).side == b.me_side();
            let side = if mine { "己方" } else { "敌方" };
            let tone = if mine { Tone::Mine } else { Tone::Foe };
            out.push(Part::new(format!(" {side}全体 ×{}", many.len()), tone));
        }
    }
    out
}

/// 合击的出手者：`我5 乌力 + 我8 勇者`，按出手顺序（STAGE-126）；服务端没给 actors 时只写起头的。
fn combo_actors(b: &BattleState, ev: &BattleEvent) -> Vec<Part> {
    let mut out = vec![];
    for (i, no) in ev.actor_list().into_iter().enumerate() {
        if i > 0 {
            out.push(Part::new(" + ", Tone::Plain));
        }
        out.extend(who(b, no, 1));
    }
    out
}

/// 捕捉：起手写“· · ·”，命中拍补上成功或失败。
fn catch(b: &BattleState, ev: &BattleEvent, hit: bool) -> Vec<Part> {
    let mut out = arrow("捕捉");
    if let Some(t) = ev.target {
        out.extend(who(b, t, 2));
    }
    if ev.discarded.is_some() {
        // 资质筛选放生（STAGE-264）：球收回去了，不算抓到。
        out.push(Part::new("  放生", Tone::Verb));
    } else {
        out.extend(verdict(hit, ev.ok, "  "));
    }
    out
}

/// 回合末的状态结算：`毒发  敌2 野猪 -5`，每个目标一段。
fn status_tick(b: &BattleState, ev: &BattleEvent, hit: bool) -> Vec<Part> {
    let mut out = vec![Part::new("毒发", Tone::Status)];
    for e in &ev.effects {
        out.push(Part::new("  ", Tone::Plain));
        out.extend(who(b, e.target, 2));
        if hit {
            let parts = result_parts(b, e);
            if !parts.is_empty() {
                out.push(Part::new(" ", Tone::Plain));
                out.extend(parts);
            }
        }
    }
    out
}

/// 换宠：放出或换上的宠物写在后面；只收回时写“收回宠物”。
fn swap(ev: &BattleEvent) -> Vec<Part> {
    match ev.joined.first() {
        Some(u) => vec![Part::new(format!("  换上 {}", u.name), Tone::Verb)],
        None => vec![Part::new("  收回宠物", Tone::Verb)],
    }
}

/// 箭头和动作：` ━━ 攻击 ━━▶ `，后面接目标；精灵、道具、技能的名字在【】里，箭头贴着它：` ━━【火焰】━━▶ `。
fn arrow(verb: &str) -> Vec<Part> {
    let (before, after) = if verb.starts_with('【') { (" ━━", "━━▶ ") } else { (" ━━ ", " ━━▶ ") };
    vec![Part::new(before, Tone::Plain), Part::new(verb, Tone::Verb), Part::new(after, Tone::Plain)]
}

/// 捕捉和逃跑的结果：起手是“· · ·”，命中拍是成功或失败。`sep` 放在结果前面。
fn verdict(hit: bool, ok: Option<bool>, sep: &str) -> Vec<Part> {
    if !hit {
        return vec![Part::new(format!("{sep}· · ·"), Tone::Dim)];
    }
    if ok == Some(true) { vec![Part::new("  成功！", Tone::Good)] } else { vec![Part::new("  失败", Tone::Bad)] }
}

/// 一次行动的结果：每个效果的结果依次写，之间空一格；倒下的目标接在最后，`-23  倒下`。
fn results(b: &BattleState, effects: &[BattleEffect]) -> Vec<Part> {
    let mut out = vec![];
    for e in effects {
        let parts = result_parts(b, e);
        if parts.is_empty() {
            continue;
        }
        if !out.is_empty() {
            out.push(Part::new(" ", Tone::Plain));
        }
        out.extend(parts);
    }
    if !out.is_empty() {
        out.insert(0, Part::new("  ", Tone::Plain));
    }
    if effects.iter().any(|e| e.down) {
        out.push(Part::new("  倒下", Tone::Bad));
    }
    out
}

/// 单个效果的结果段：伤害、暴击、回复、闪避、未命中、全防，加上状态的增减。
/// STAGE-199：吸收（回血；协议不带被吸收的原伤害，只写回了多少）、反射（伤害弹给 intent 指的那一方）、无效。
fn result_parts(b: &BattleState, e: &BattleEffect) -> Vec<Part> {
    let mut out = match e.result.as_str() {
        "crit" => vec![Part::new(format!("{} 暴击！", e.delta), Tone::Crit)],
        "hit" if e.delta < 0 => vec![Part::new(e.delta.to_string(), Tone::Hurt)],
        "heal" if e.delta > 0 => vec![Part::new(format!("+{}", e.delta), Tone::Heal)],
        "dodge" => vec![Part::new("闪避", Tone::Dim)],
        "miss" => vec![Part::new("未命中", Tone::Dim)],
        "block" => vec![Part::new("挡住", Tone::Dim)],
        "absorb" => vec![Part::new(format!("吸收伤害，回复 {}", e.delta), Tone::Heal)],
        "reflect" => {
            let to = e.intent.map_or_else(|| "对方".to_string(), |no| b.name(no));
            vec![Part::new(format!("把伤害反弹给 {to}"), Tone::Dim)]
        }
        "vanish" => vec![Part::new("攻击无效", Tone::Dim)],
        _ => vec![],
    };
    out.extend(e.status_add.iter().map(|s| Part::new(status_name(s), Tone::Status)));
    out.extend(e.status_remove.iter().map(|s| Part::new(format!("解除{}", status_name(s)), Tone::Status)));
    out
}

/// 字幕里的目标（按出现顺序去重）。
fn targets(ev: &BattleEvent) -> Vec<u8> {
    let mut out = vec![];
    for e in &ev.effects {
        if !out.contains(&e.target) {
            out.push(e.target);
        }
    }
    out
}

/// 一个单位的编号和名字两段：编号（如“我8 ”）固定，名字放不下时按 `shrink` 截。
fn who(b: &BattleState, no: u8, shrink: u8) -> Vec<Part> {
    let tone = if slots::cell_of(no).side == b.me_side() { Tone::Mine } else { Tone::Foe };
    vec![Part::new(format!("{} ", slots::short_label(b.me_side(), no)), tone), Part::name(b.name(no), tone, shrink)]
}

/// 状态的中文名（`docs/battle-ui.md`「中线字幕」）；不认识的原样写。
fn status_name(s: &str) -> String {
    match s {
        "poison" => "中毒",
        "paralysis" => "麻痹",
        "sleep" => "睡眠",
        "stone" => "石化",
        "drunk" => "醉酒",
        "confusion" => "混乱",
        "weaken" => "虚弱",
        "barrier" => "魔障",
        "nocast" => "沉默",
        "guard" => "防御",
        "reversed" => "反转",
        "buff" => "强化",
        "debuff" => "弱化",
        "absorb" => "吸收",
        "reflect" => "反射",
        "vanish" => "无效",
        // STAGE-229：魔抗系（STAGE-228）的中文名；火、电、冰抗只占状态槽，字幕照样写出名字。
        "magic_resist" => "魔抗",
        "fire_resist" => "火抗",
        "thunder_resist" => "电抗",
        "ice_resist" => "冰抗",
        other => other,
    }
    .to_string()
}

/// `skip` 的原因写成字幕（超时写“没有出招”）。
fn skip_text(reason: Option<&str>) -> &'static str {
    match reason {
        Some("timeout") => "没有出招",
        Some("offline") => "掉线了",
        Some("sleep") => "睡着了，不能行动",
        Some("stone") => "石化了，不能行动",
        Some("paralysis") => "麻痹了，不能行动",
        Some("confusion") => "混乱了，不能行动",
        Some("barrier") => "魔障了，不能行动",
        Some("disobey") => "不听话，不出手",
        _ => "不能行动",
    }
}

/// 宠物不听话（协议 10.7 的 disobey，STAGE-127）：字幕和记录共用的说法，`reason` 说明它做了什么。
pub fn disobey_text(reason: Option<&str>) -> &'static str {
    match reason {
        Some("retarget") => "不听话：换了目标",
        Some("random") => "不听话：乱来",
        Some("owner") => "不听话：打主人",
        Some("enemy") => "不听话：打敌人",
        Some("escape") => "不听话：想跑",
        Some("no_skill") => "不听话：没有技能，不出手",
        _ => "不听话",
    }
}
