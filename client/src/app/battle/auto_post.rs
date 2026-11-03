//! 战后补血与丢弃（STAGE-260，`docs/auto-battle-settings.md`「战后吃肉与丢弃」）。
//! 战斗窗口收起、回到世界之后跑一轮：先吃肉（人物，人物达标后再喂出战宠），再丢肉。
//! 每一步只发一条请求，回包之后才决定下一步；每一步前先读一次背包、宠物和状态，判断用的是最新的。
//! 挂机停了、agent 接管、断线或又进了战斗，就放弃这一轮：不停挂机，也不弹窗。
//! 判定见 `auto_meat.rs`，丢弃的选法见 `auto_drop.rs`。

use std::collections::BTreeSet;

use serde_json::{Value, json};

use super::auto_drop::{self, MAX_DROP};
use super::auto_meat::{self, Body, EatStep, EatView, MAX_EAT, Meat};
use crate::app::App;
use crate::app::session::Pending;

/// 战后请求的上下文（`Pending::AutoPost` 携带）：回包时要写的内容在发出时就取好。
#[derive(Debug, Clone)]
pub enum PostOp {
    /// 读状态。排在 inventory、pets 之后发，它的回包到了，前面两个也已经生效。
    Sync,
    /// 吃一份肉：格子、名字，以及写进记录里的对象（人物或「乌力（战宠）」这样的写法）。
    Eat { slot: u8, name: String, who: String },
    /// 丢一件肉：格子、名字、数量（整格丢时的数量，只用来写记录）。
    Drop { slot: u8, name: String, count: u32 },
}

/// 一轮战后处理的进度。平时是默认值（`armed` 为假）。
#[derive(Debug, Default)]
pub struct PostBattle {
    /// 战斗收起、挂机开着：等这一轮跑完或放弃。
    armed: bool,
    /// 下一步之前要先读一次背包、宠物和状态。
    need_sync: bool,
    /// 有请求在途，等回包。
    waiting: bool,
    /// 吃肉这一段结束（肉不够、到上限、或用不着了）。
    eat_done: bool,
    eaten: u32,
    drops: u32,
    dropped: u32,
    drop_failed: u32,
    /// 本场拉黑的格子：吃肉和丢弃分开记（ITEM_UNUSABLE 的肉照样可以丢）。
    bad_eat: BTreeSet<u8>,
    bad_drop: BTreeSet<u8>,
}

impl PostBattle {
    /// 战斗窗口收起（有结算、挂机开着）时调用：从头跑一轮。
    pub(crate) fn arm(&mut self) {
        *self = Self {
            armed: true,
            need_sync: true,
            ..Self::default()
        };
    }
}

/// 这些错误码说明这一轮不该再做（死了、进战斗了、被 agent 或玩家接管）：放弃，写一句原因。
const ABORT_CODES: [&str; 4] = [
    "CHARACTER_DEAD",
    "IN_BATTLE",
    "CONTROLLED_BY_AGENT",
    "CONTROLLED_BY_PLAYER",
];

impl App {
    /// 托管的战后部分（每个刷新周期查一次）：战斗已收起、挂机还开着，才往下走一步。
    pub(crate) fn post_battle_tick(&mut self) {
        let Some(w) = self.world.as_ref() else { return };
        let (armed, waiting) = (w.post_battle.armed, w.post_battle.waiting);
        let live = w.auto_battle && w.online && !w.observing() && w.battle.is_none();
        // 人物倒下（血量 0）时不吃也不丢：道具用不了，这一轮直接收起。
        let dead = w.entry.status.hp == 0;
        if !armed {
            return;
        }
        if !live || dead || self.halt_busy() {
            return self.post_battle_reset();
        }
        if waiting {
            return;
        }
        let (heal, pct, drop_on) = {
            let r = &self.auto_profile().recover;
            (r.post_heal_item, r.post_heal_pct, r.post_drop_item)
        };
        if !heal && !drop_on {
            return self.post_battle_reset();
        }
        if self.post_need_sync() {
            return self.post_battle_sync();
        }
        if heal && !self.post_eat_done() && self.post_battle_eat(pct) {
            return;
        }
        if drop_on {
            return self.post_battle_drop();
        }
        self.post_battle_finish();
    }

    /// 发出读状态的三条请求：背包、宠物、状态（状态排最后，它的回包表示前两个也到了）。
    fn post_battle_sync(&mut self) {
        let ok = self.send("inventory", json!({}), Pending::Inventory)
            && self.send("pets", json!({}), Pending::Pets)
            && self.send("status", json!({}), Pending::AutoPost(PostOp::Sync));
        if self.post_sent(ok) {
            self.post_set(|p| p.need_sync = false);
        }
    }

    /// 吃肉的一步：人物达标后才轮到出战宠。发出了请求返回 true；否则这段吃肉结束（写好说明）。
    fn post_battle_eat(&mut self, pct: u8) -> bool {
        let (step, pet_name, note) = {
            let Some(w) = self.world.as_ref() else { return false };
            let p = &w.post_battle;
            let pet = w.pets.pets.iter().find(|x| x.state == "battle");
            let view = EatView {
                pct,
                player: Body {
                    hp: w.entry.status.hp,
                    max: w.entry.status.max_hp,
                },
                pet: pet.map(|x| {
                    (
                        x.slot + 1,
                        Body {
                            hp: x.hp,
                            max: x.max_hp,
                        },
                    )
                }),
                slots: &w.inventory.slots,
                bad: &p.bad_eat,
                eaten: p.eaten,
            };
            let step = auto_meat::next_eat(&view);
            let note = match step {
                EatStep::Short => {
                    let person = auto_meat::percent(w.entry.status.hp, w.entry.status.max_hp);
                    let to_pet = pet.map_or_else(
                        || "未出战".to_string(),
                        |x| format!("{}%", auto_meat::percent(x.hp, x.max_hp)),
                    );
                    Some(format!("战后补血：肉不足，人物 {person}% / 战宠 {to_pet}"))
                }
                EatStep::Capped => Some(format!("战后吃肉已到 {MAX_EAT} 份上限，剩下的不吃了")),
                _ => None,
            };
            (step, pet.map(|x| x.name.clone()), note)
        };
        if let Some(text) = note {
            self.log.battle(text);
        }
        match step {
            EatStep::Person(meat) => self.send_eat(meat, None, "人物".into()),
            EatStep::Pet { meat, target } => {
                let who = format!("{}（战宠）", pet_name.unwrap_or_default());
                self.send_eat(meat, Some(target), who)
            }
            _ => {
                self.post_set(|p| p.eat_done = true);
                false
            }
        }
    }

    fn send_eat(&mut self, meat: Meat, target: Option<u8>, who: String) -> bool {
        let mut data = json!({"slot": meat.slot});
        if let Some(t) = target {
            data["target"] = json!(t);
        }
        let op = PostOp::Eat {
            slot: meat.slot,
            name: meat.name,
            who,
        };
        let ok = self.send("item_use", data, Pending::AutoPost(op));
        self.post_sent(ok)
    }

    /// 丢肉的一步：挑一件发出去；没有了或到上限就结束。
    fn post_battle_drop(&mut self) {
        let (capped, pick) = {
            let Some(w) = self.world.as_ref() else { return };
            let p = &w.post_battle;
            if !p.armed {
                return;
            }
            if p.drops >= MAX_DROP {
                (true, None)
            } else {
                (false, auto_drop::pick_drop(&w.inventory.slots, &p.bad_drop))
            }
        };
        if capped {
            self.log
                .battle(format!("战后丢弃已到 {MAX_DROP} 次上限，剩下的肉留在背包"));
            return self.post_battle_finish();
        }
        let Some((slot, name, count)) = pick else {
            return self.post_battle_finish();
        };
        self.post_set(|p| p.drops += 1);
        let ok = self.send(
            "item_drop",
            json!({"slot": slot, "confirm": "yes"}),
            Pending::AutoPost(PostOp::Drop { slot, name, count }),
        );
        self.post_sent(ok);
    }

    /// 这一轮结束：有丢弃的写一句汇总，然后收起。
    fn post_battle_finish(&mut self) {
        let (dropped, failed) = self
            .world
            .as_ref()
            .map_or((0, 0), |w| (w.post_battle.dropped, w.post_battle.drop_failed));
        if dropped + failed > 0 {
            self.log
                .battle(format!("战后丢弃：丢弃 {dropped} 件，失败 {failed} 件"));
        }
        self.post_battle_reset();
    }

    fn post_battle_reset(&mut self) {
        if let Some(w) = self.world.as_mut() {
            w.post_battle = PostBattle::default();
        }
    }

    /// 回包（成功）：吃肉、丢弃的记录在这里写；状态回包顺带更新人物。
    pub(crate) fn on_post_ok(&mut self, op: PostOp, data: &Value) {
        if let PostOp::Sync = op {
            self.on_status(data.clone());
        }
        if !self.post_armed() {
            return;
        }
        match op {
            PostOp::Sync => self.post_set(|p| p.waiting = false),
            PostOp::Eat { name, who, .. } => {
                let delta = data.get("delta").and_then(Value::as_i64).unwrap_or(0);
                let mut text = format!("战后补血：对{who}使用了 {name}");
                if delta > 0 {
                    text.push_str(&format!("，血量回复 {delta}"));
                }
                self.log.battle(text);
                self.post_set(|p| {
                    p.eaten += 1;
                    p.waiting = false;
                    p.need_sync = true;
                });
            }
            PostOp::Drop { slot, name, count } => {
                self.log.battle(format!("战后丢弃：{slot} 号 {name} ×{count}"));
                self.post_set(|p| {
                    p.dropped += 1;
                    p.waiting = false;
                    p.need_sync = true;
                });
            }
        }
    }

    /// 回包（失败）：放弃类错误结束这一轮；其他错误只拉黑这一格，不重试同一格。
    pub(crate) fn on_post_failed(&mut self, op: PostOp, code: &str) {
        if !self.post_armed() {
            return;
        }
        if ABORT_CODES.contains(&code) {
            self.log.battle(format!("战后处理中止（{code}）"));
            return self.post_battle_reset();
        }
        match op {
            PostOp::Sync => self.post_battle_reset(),
            PostOp::Eat { slot, .. } => self.post_set(|p| {
                p.bad_eat.insert(slot);
                p.waiting = false;
                p.need_sync = true;
            }),
            // 格子已经空了（别处改过背包）：不算失败，下一轮读背包时自然不会再选它。
            PostOp::Drop { slot, .. } if code == "EMPTY_SLOT" => self.post_set(|p| {
                p.bad_drop.insert(slot);
                p.waiting = false;
                p.need_sync = true;
            }),
            PostOp::Drop { slot, name, .. } => {
                self.log.battle(format!("战后丢弃失败：{slot} 号 {name}（{code}）"));
                self.post_set(|p| {
                    p.bad_drop.insert(slot);
                    p.drop_failed += 1;
                    p.waiting = false;
                    p.need_sync = true;
                });
            }
        }
    }

    /// 请求发出去了就记在途；没发出去（断线）就放弃这一轮。
    fn post_sent(&mut self, ok: bool) -> bool {
        if !ok {
            self.post_battle_reset();
            return false;
        }
        self.post_set(|p| p.waiting = true);
        true
    }

    fn post_armed(&self) -> bool {
        self.world.as_ref().is_some_and(|w| w.post_battle.armed)
    }

    fn post_need_sync(&self) -> bool {
        self.world.as_ref().is_some_and(|w| w.post_battle.need_sync)
    }

    fn post_eat_done(&self) -> bool {
        self.world.as_ref().is_some_and(|w| w.post_battle.eat_done)
    }

    fn post_set(&mut self, f: impl FnOnce(&mut PostBattle)) {
        if let Some(w) = self.world.as_mut() {
            f(&mut w.post_battle);
        }
    }
}

#[cfg(test)]
#[path = "auto_post_tests.rs"]
mod tests;
