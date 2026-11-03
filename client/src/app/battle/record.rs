//! 战斗记录的文字（`docs/battle-ui.md`「结算回放」）：一条事件写成一行或几行，右侧栏和消息栏共用。

use super::caption::disobey_text;
use super::BattleState;
use crate::proto::{BattleEffect, BattleEvent, BattleField};

impl BattleState {
    /// 一条事件的记录文字（右侧栏和消息栏共用）。
    pub(super) fn describe(&self, ev: &BattleEvent) -> Vec<String> {
        let who = ev.actor.map(|n| self.name(n)).unwrap_or_default();
        let mut out = match ev.kind.as_str() {
            "attack" => self.outcomes(&who, "攻击", ev),
            "counter" => self.outcomes(&who, "反击", ev),
            "combo" => self.combo(ev),
            "magic" => self.uses(&who, ev.name.as_deref().unwrap_or("精灵"), ev),
            "item" => self.uses(&who, ev.name.as_deref().unwrap_or("道具"), ev),
            "skill" => self.uses(&who, ev.name.as_deref().unwrap_or("技能"), ev),
            "guard" => vec![format!("{who} 防御")],
            // 资质筛选放生（STAGE-264）：不是抓到，记录写“已放生”和原因。
            "capture" if ev.discarded.is_some() => {
                vec![ev.discarded.as_ref().map(crate::app::auto_battle::capture_log::battle_line).unwrap_or_default()]
            }
            "capture" => {
                let target = ev.target.map(|n| self.name(n)).unwrap_or_default();
                vec![format!("{who} 捕捉 {target}：{}", verdict(ev.ok))]
            }
            "escape" => vec![format!("{who} 逃跑：{}", verdict(ev.ok))],
            "swap_pet" => {
                let left: Vec<String> = ev.left.iter().map(|n| self.name(*n)).collect();
                let joined: Vec<String> = ev.joined.iter().map(|u| u.name.clone()).collect();
                let text = match (left.is_empty(), joined.is_empty()) {
                    (_, true) => format!("{who} 收回宠物"),
                    (true, false) => format!("{who} 放出 {}", joined.join("、")),
                    (false, false) => format!("{who} 换宠：{} → {}", left.join("、"), joined.join("、")),
                };
                vec![text]
            }
            "skip" => vec![format!("{who} 没有行动（{}）", skip_reason(ev.reason.as_deref()))],
            "disobey" => vec![format!("{who} {}", disobey_text(ev.reason.as_deref()))],
            "pet_escape" => vec![format!("{who} 不听话，跑掉了")],
            "status_tick" => ev.effects.iter().map(|e| self.outcome(&self.name(e.target), e)).collect(),
            "field" => vec![field_text(ev.field.as_ref())],
            _ => vec![format!("{who} 行动")],
        };
        out.extend(ev.effects.iter().filter(|e| e.down).map(|e| format!("{} 倒下了", self.name(e.target))));
        out
    }

    /// 攻击、精灵、道具、技能这类动作：每个目标一行；没有目标时只写动作本身。
    fn outcomes(&self, who: &str, verb: &str, ev: &BattleEvent) -> Vec<String> {
        if ev.effects.is_empty() {
            return vec![format!("{who} {verb}")];
        }
        ev.effects
            .iter()
            .map(|e| format!("{who} {verb} {}", self.outcome(&self.name(e.target), e)).trim_end().to_string())
            .collect()
    }

    /// 合击（STAGE-126）：`乌力 + 勇者 合击 野猪 -5 -6`，几个人打同一个目标合成一行；打到不同目标时按目标分行。
    fn combo(&self, ev: &BattleEvent) -> Vec<String> {
        let names: Vec<String> = ev.actor_list().iter().map(|n| self.name(*n)).collect();
        let who = names.join(" + ");
        let mut targets: Vec<u8> = vec![];
        for e in &ev.effects {
            if !targets.contains(&e.target) {
                targets.push(e.target);
            }
        }
        if targets.is_empty() {
            return vec![format!("{who} 合击")];
        }
        targets
            .into_iter()
            .map(|t| {
                let hits = ev.effects.iter().filter(|e| e.target == t);
                let texts: Vec<String> = hits.map(effect_text).filter(|s| !s.is_empty()).collect();
                let line = format!("{who} 合击 {} {}", self.name(t), texts.join(" "));
                line.trim_end().to_string()
            })
            .collect()
    }

    /// 精灵、道具、技能：`勇者 用回复药 → 勇者 +10`；没有目标时只写用了什么。
    fn uses(&self, who: &str, name: &str, ev: &BattleEvent) -> Vec<String> {
        if ev.effects.is_empty() {
            return vec![format!("{who} 用{name}")];
        }
        ev.effects.iter().map(|e| format!("{who} 用{name} → {}", self.outcome(&self.name(e.target), e))).collect()
    }

    fn outcome(&self, target: &str, e: &BattleEffect) -> String {
        let text = effect_text(e);
        if text.is_empty() { target.to_string() } else { format!("{target} {text}") }
    }
}

/// 记录里的效果文字：`-23`、`-46!`、`miss`、`+80`；属性反转写“反转开”或“反转关”；没有变化是空串。
fn effect_text(e: &BattleEffect) -> String {
    if e.result == "none" {
        if e.status_add.iter().any(|s| s == "reversed") {
            return "反转开".into();
        }
        if e.status_remove.iter().any(|s| s == "reversed") {
            return "反转关".into();
        }
    }
    match e.result.as_str() {
        "crit" => format!("{}!", e.delta),
        "hit" if e.delta != 0 => e.delta.to_string(),
        "heal" if e.delta > 0 => format!("+{}", e.delta),
        "miss" | "dodge" => "miss".into(),
        "block" => "全防".into(),
        "absorb" if e.delta > 0 => format!("吸收+{}", e.delta),
        "reflect" => "反弹".into(),
        "vanish" => "无效".into(),
        _ => String::new(),
    }
}

/// 场地变化或恢复的记录（STAGE-200）：`场地变成火属性（60%，3 回合）`，无属性时 `场地恢复成无属性`。
fn field_text(f: Option<&BattleField>) -> String {
    match f {
        Some(f) if f.active() => format!("场地变成{}属性（{}%，{} 回合）", f.attr_text(), f.pow, f.left),
        _ => "场地恢复成无属性".into(),
    }
}

fn verdict(ok: Option<bool>) -> &'static str {
    if ok == Some(true) { "成功" } else { "失败" }
}

fn skip_reason(reason: Option<&str>) -> &'static str {
    match reason {
        Some("timeout") => "超时",
        Some("offline") => "掉线",
        Some("sleep") => "睡着",
        Some("stone") => "石化",
        Some("paralysis") => "麻痹",
        Some("confusion") => "混乱",
        Some("barrier") => "魔障",
        Some("disobey") => "不听话",
        _ => "无法行动",
    }
}
