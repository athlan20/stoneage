//! 战后补血与丢弃的测试（STAGE-260，`auto_post.rs` 的子模块）。
//! 用假连接收请求，假服务端按回包推进：检查发出去的 item_use / item_drop，以及战斗记录的文字。

use std::collections::HashMap;
use std::sync::mpsc::Receiver;

use serde_json::{Value, json};

use super::*;
use crate::app::auto_battle::store::Store;
use crate::app::battle::auto_meat::heal_meat_power;
use crate::app::{Options, Screen, WorldState};
use crate::net::Outgoing;
use crate::proto::{InventorySlot, Pet, TargetType, WorldEntry};

const HEAL_40: &str = "耐久力40前後回复";

/// 假服务端：背包、人物血量、出战宠血量；`refuse` 里的格子回复错误码（格子 → 错误码）。
struct Sim {
    rx: Receiver<Outgoing>,
    hp: u32,
    pet_hp: Option<u32>,
    inv: Vec<InventorySlot>,
    refuse: HashMap<u8, &'static str>,
}

fn meat(slot: u8, name: &str, effect: &str, count: u32) -> InventorySlot {
    InventorySlot {
        slot,
        name: name.into(),
        effect: effect.into(),
        target_type: TargetType::Ally,
        count,
        ..Default::default()
    }
}

/// 挂机开着、已经收起战斗、按参数设好吃肉与丢弃的 App，外加假服务端。
fn setup(heal: bool, drop_on: bool, hp: u32, pet_hp: Option<u32>, inv: Vec<InventorySlot>) -> (App, Sim) {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.auto_battle = true;
    w.online = true;
    w.post_battle.arm();
    w.entry.status.hp = hp;
    w.entry.status.max_hp = 100;
    if let Some(ph) = pet_hp {
        w.pets.pets.push(Pet {
            slot: 0,
            name: "乌力".into(),
            state: "battle".into(),
            hp: ph,
            max_hp: 100,
            ..Default::default()
        });
    }
    a.world = Some(w);
    let name = std::thread::current().name().unwrap_or("t").replace("::", "_");
    let dir = std::env::temp_dir().join(format!("stage260-{}-{name}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    a.auto_store = Some(Store::open_in(&dir, "测试服", "勇者"));
    {
        let p = a.auto_store.as_mut().unwrap().profiles.active_mut();
        p.recover.post_heal_item = heal;
        p.recover.post_heal_pct = 90;
        p.recover.post_drop_item = drop_on;
    }
    let rx = a.attach_test_link();
    (
        a,
        Sim {
            rx,
            hp,
            pet_hp,
            inv,
            refuse: HashMap::new(),
        },
    )
}

fn op_for(ty: &str, item: &InventorySlot, data: &Value) -> PostOp {
    if ty == "item_use" {
        let who = if data.get("target").is_some() {
            "乌力（战宠）"
        } else {
            "人物"
        };
        PostOp::Eat {
            slot: item.slot,
            name: item.name.clone(),
            who: who.into(),
        }
    } else {
        PostOp::Drop {
            slot: item.slot,
            name: item.name.clone(),
            count: item.count,
        }
    }
}

impl Sim {
    /// 推进到没有新请求为止：每一拍先把假服务端的状态写进世界，再跑一拍，然后按回包回答。返回这一段发出的请求。
    fn run(&mut self, a: &mut App) -> Vec<(String, Value)> {
        let mut out = Vec::new();
        for _ in 0..100 {
            {
                let w = a.world.as_mut().unwrap();
                w.inventory.slots = self.inv.clone();
                if let (Some(ph), Some(p)) = (self.pet_hp, w.pets.pets.first_mut()) {
                    p.hp = ph;
                }
            }
            a.post_battle_tick();
            let mut lines = Vec::new();
            while let Ok(msg) = self.rx.try_recv() {
                if let Outgoing::Line(line) = msg {
                    lines.push(line);
                }
            }
            if lines.is_empty() {
                break;
            }
            for line in lines {
                let v: Value = serde_json::from_str(&line).unwrap();
                let ty = v["type"].as_str().unwrap().to_string();
                let data = v["data"].clone();
                self.answer(a, &ty, &data);
                out.push((ty, data));
            }
        }
        out
    }

    fn answer(&mut self, a: &mut App, ty: &str, data: &Value) {
        if ty == "status" {
            a.on_post_ok(PostOp::Sync, &json!({"status": {"hp": self.hp, "max_hp": 100}}));
            return;
        }
        if ty != "item_use" && ty != "item_drop" {
            return;
        }
        let slot = data["slot"].as_u64().unwrap() as u8;
        let Some(item) = self.inv.iter().find(|s| s.slot == slot).cloned() else {
            return;
        };
        let op = op_for(ty, &item, data);
        if let Some(code) = self.refuse.get(&slot).copied() {
            a.on_post_failed(op, code);
            return;
        }
        if ty == "item_use" {
            let heal = heal_meat_power(&item.name, &item.effect).unwrap_or(0);
            if data.get("target").is_some() {
                self.pet_hp = self.pet_hp.map(|h| h + heal);
            } else {
                self.hp += heal;
            }
            if let Some(s) = self.inv.iter_mut().find(|s| s.slot == slot) {
                s.count -= 1;
            }
            self.inv.retain(|s| s.count > 0);
            a.on_post_ok(op, &json!({"delta": heal}));
        } else {
            self.inv.retain(|s| s.slot != slot);
            a.on_post_ok(op, &json!({}));
        }
    }
}

fn slots_of(sent: &[(String, Value)], ty: &str) -> Vec<u64> {
    sent.iter()
        .filter(|(t, _)| t == ty)
        .map(|(_, d)| d["slot"].as_u64().unwrap())
        .collect()
}

fn log_of(a: &App) -> Vec<String> {
    a.log.all().iter().map(|m| m.text.clone()).collect()
}

fn has(a: &App, text: &str) -> bool {
    log_of(a).iter().any(|t| t == text)
}

fn armed(a: &App) -> bool {
    a.world.as_ref().unwrap().post_battle.armed
}

#[test]
fn eats_for_the_person_then_drops_the_rest_and_reports_each_item() {
    let inv = vec![
        meat(5, "烤肉", HEAL_40, 2),
        meat(6, "魅力肉粽", HEAL_40, 1),
        meat(7, "气力肉", "气力60前後回复", 1),
        meat(12, "毒肉", "毒状态回复", 1),
        InventorySlot {
            slot: 3,
            name: "斧头".into(),
            effect: "攻 +9 防 -3 敏 -3".into(),
            ..Default::default()
        },
    ];
    let (mut a, mut sim) = setup(true, true, 30, None, inv);
    let sent = sim.run(&mut a);
    assert_eq!(slots_of(&sent, "item_use"), vec![5, 5], "30 → 70 → 110，两份就过门槛");
    assert!(
        sent.iter()
            .filter(|(t, _)| t == "item_use")
            .all(|(_, d)| d.get("target").is_none())
    );
    assert_eq!(
        slots_of(&sent, "item_drop"),
        vec![7, 12],
        "气力肉、毒肉丢；魅力肉粽和装备格不丢"
    );
    assert!(
        sent.iter()
            .filter(|(t, _)| t == "item_drop")
            .all(|(_, d)| d["confirm"] == "yes")
    );
    assert_eq!(sim.hp, 110);
    assert!(has(&a, "战后补血：对人物使用了 烤肉，血量回复 40"));
    assert!(has(&a, "战后丢弃：7 号 气力肉 ×1"));
    assert!(has(&a, "战后丢弃：12 号 毒肉 ×1"));
    assert!(has(&a, "战后丢弃：丢弃 2 件，失败 0 件"));
    assert!(
        sim.inv.iter().all(|s| !s.name.contains('肉') || s.name == "魅力肉粽"),
        "剩下的肉丢干净"
    );
    assert!(!armed(&a));
    assert!(a.world.as_ref().unwrap().auto_battle, "挂机不停");
}

#[test]
fn with_eating_off_meat_is_dropped_without_using_any_item() {
    let inv = vec![meat(5, "烤肉", HEAL_40, 1), meat(6, "毒肉", "毒状态回复", 1)];
    let (mut a, mut sim) = setup(false, true, 30, None, inv);
    let sent = sim.run(&mut a);
    assert!(slots_of(&sent, "item_use").is_empty(), "吃肉关着，一份都不吃");
    assert_eq!(slots_of(&sent, "item_drop"), vec![5, 6]);
    assert!(has(&a, "战后丢弃：丢弃 2 件，失败 0 件"));
    assert_eq!(sim.hp, 30);
}

#[test]
fn with_both_off_nothing_is_sent_after_a_battle() {
    let inv = vec![meat(5, "烤肉", HEAL_40, 1)];
    let (mut a, mut sim) = setup(false, false, 30, Some(40), inv);
    let sent = sim.run(&mut a);
    assert!(sent.is_empty(), "两项都关：什么请求都不发");
    assert!(!armed(&a));
    assert!(log_of(&a).iter().all(|t| !t.starts_with("战后")));
}

#[test]
fn a_stopped_or_taken_over_round_does_nothing() {
    for case in ["manual_stop", "auto_off", "agent", "not_armed"] {
        let inv = vec![meat(5, "烤肉", HEAL_40, 1)];
        let (mut a, mut sim) = setup(true, true, 30, None, inv);
        {
            let w = a.world.as_mut().unwrap();
            match case {
                "manual_stop" | "auto_off" => w.auto_battle = false,
                "agent" => w.entry.control.you = "observer".into(),
                _ => w.post_battle = PostBattle::default(),
            }
        }
        let sent = sim.run(&mut a);
        assert!(sent.is_empty(), "{case}：不应发请求");
        assert!(log_of(&a).iter().all(|t| !t.starts_with("战后")), "{case}：不应写记录");
    }
}

#[test]
fn feeds_the_pet_only_when_the_person_is_fine() {
    let inv = vec![meat(5, "烤肉", HEAL_40, 1)];
    let (mut a, mut sim) = setup(true, false, 100, Some(40), inv);
    let sent = sim.run(&mut a);
    let uses: Vec<&Value> = sent.iter().filter(|(t, _)| t == "item_use").map(|(_, d)| d).collect();
    assert_eq!(uses.len(), 1);
    assert_eq!(uses[0]["target"], json!(1), "出战宠在栏位 0，目标编号是 1");
    assert!(has(&a, "战后补血：对乌力（战宠）使用了 烤肉，血量回复 40"));
}

#[test]
fn short_of_meat_writes_one_line_and_keeps_the_auto_on() {
    let (mut a, mut sim) = setup(true, false, 30, Some(40), vec![]);
    let sent = sim.run(&mut a);
    assert!(slots_of(&sent, "item_use").is_empty());
    assert!(has(&a, "战后补血：肉不足，人物 30% / 战宠 40%"));
    assert!(a.world.as_ref().unwrap().auto_battle, "肉不够不停挂机");
}

#[test]
fn eating_stops_at_ten_portions_with_a_note() {
    let inv: Vec<InventorySlot> = (5..=15).map(|s| meat(s, "生肉", "耐久力5前后回复", 1)).collect();
    let (mut a, mut sim) = setup(true, false, 10, None, inv);
    let sent = sim.run(&mut a);
    assert_eq!(slots_of(&sent, "item_use").len(), MAX_EAT as usize);
    assert_eq!(sim.hp, 10 + 5 * MAX_EAT);
    assert!(has(&a, &format!("战后吃肉已到 {MAX_EAT} 份上限，剩下的不吃了")));
}

#[test]
fn dropping_stops_at_the_cap_with_a_note() {
    let inv = vec![meat(5, "毒肉", "毒状态回复", 1)];
    let (mut a, mut sim) = setup(false, true, 30, None, inv);
    a.world.as_mut().unwrap().post_battle.drops = MAX_DROP;
    let sent = sim.run(&mut a);
    assert!(slots_of(&sent, "item_drop").is_empty(), "到上限就不再发丢弃");
    assert!(has(&a, "战后丢弃已到 20 次上限，剩下的肉留在背包"));
    assert!(!armed(&a));
}

#[test]
fn an_empty_slot_is_skipped_and_not_counted_as_failed() {
    let inv = vec![meat(7, "气力肉", "气力60前後回复", 1), meat(8, "毒肉", "毒状态回复", 1)];
    let (mut a, mut sim) = setup(false, true, 30, None, inv);
    sim.refuse.insert(7, "EMPTY_SLOT");
    let sent = sim.run(&mut a);
    assert_eq!(slots_of(&sent, "item_drop"), vec![7, 8], "7 号被拒后跳过，继续丢 8 号");
    assert!(has(&a, "战后丢弃：丢弃 1 件，失败 0 件"));
}

#[test]
fn another_refused_drop_counts_as_failed_and_is_not_retried() {
    let inv = vec![meat(7, "气力肉", "气力60前後回复", 1), meat(8, "毒肉", "毒状态回复", 1)];
    let (mut a, mut sim) = setup(false, true, 30, None, inv);
    sim.refuse.insert(7, "BAD_REQUEST");
    let sent = sim.run(&mut a);
    assert_eq!(slots_of(&sent, "item_drop"), vec![7, 8], "7 号只发一次，不重试");
    assert!(has(&a, "战后丢弃失败：7 号 气力肉（BAD_REQUEST）"));
    assert!(has(&a, "战后丢弃：丢弃 1 件，失败 1 件"));
}

#[test]
fn a_meat_the_game_refuses_to_eat_is_not_tried_again() {
    let inv = vec![meat(5, "烤肉", HEAL_40, 1), meat(6, "烤肉", HEAL_40, 1)];
    let (mut a, mut sim) = setup(true, false, 30, None, inv);
    sim.refuse.insert(5, "ITEM_UNUSABLE");
    let sent = sim.run(&mut a);
    assert_eq!(slots_of(&sent, "item_use"), vec![5, 6], "5 号拒绝后只试一次，换 6 号");
    assert!(has(&a, "战后补血：肉不足，人物 70% / 战宠 未出战"));
}

#[test]
fn abort_codes_write_one_line_and_end_the_round() {
    for code in [
        "CHARACTER_DEAD",
        "IN_BATTLE",
        "CONTROLLED_BY_AGENT",
        "CONTROLLED_BY_PLAYER",
    ] {
        let inv = vec![meat(5, "烤肉", HEAL_40, 2)];
        let (mut a, mut sim) = setup(true, true, 30, None, inv);
        sim.refuse.insert(5, code);
        let sent = sim.run(&mut a);
        assert_eq!(slots_of(&sent, "item_use"), vec![5], "{code}：不再发下一步");
        assert!(slots_of(&sent, "item_drop").is_empty(), "{code}：不丢");
        assert!(has(&a, &format!("战后处理中止（{code}）")), "{code}：要写一句原因");
        assert!(!armed(&a));
    }
}

#[test]
fn going_offline_drops_the_round_silently() {
    let inv = vec![meat(5, "烤肉", HEAL_40, 1)];
    let (mut a, mut sim) = setup(true, true, 30, None, inv);
    a.world.as_mut().unwrap().online = false;
    let sent = sim.run(&mut a);
    assert!(sent.is_empty());
    assert!(!armed(&a));
    assert!(log_of(&a).iter().all(|t| !t.starts_with("战后")));
}

#[test]
fn answers_after_the_round_ended_are_ignored() {
    let (mut a, _sim) = setup(true, true, 30, None, vec![]);
    a.world.as_mut().unwrap().post_battle = PostBattle::default();
    a.on_post_ok(
        PostOp::Eat {
            slot: 5,
            name: "烤肉".into(),
            who: "人物".into(),
        },
        &json!({"delta": 34}),
    );
    assert_eq!(a.world.as_ref().unwrap().post_battle.eaten, 0);
    assert!(log_of(&a).iter().all(|t| !t.starts_with("战后")));
}
