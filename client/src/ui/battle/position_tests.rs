use super::*;
use crate::{
    app::battle::{Motion, SwapFx},
    proto::{BattleEffect, BattleEvent, BattleStart, BattleUnit},
};
use std::time::{Duration, Instant};

fn scene() -> (BattleState, BTreeMap<u8, Rect>, Rect, Instant) {
    let b = BattleState::from_start(BattleStart {
        you: 0,
        units: [0, 5, 10, 14, 15]
            .into_iter()
            .map(|no| BattleUnit {
                no,
                hp_pct: 100,
                ..Default::default()
            })
            .collect(),
        ..Default::default()
    });
    let area = Rect::new(0, 0, 100, 30);
    let home = [
        (0, Rect::new(82, 12, 14, 5)),
        (5, Rect::new(62, 12, 14, 5)),
        (10, Rect::new(3, 12, 14, 5)),
        (14, Rect::new(3, 22, 14, 5)),
        (15, Rect::new(25, 12, 14, 5)),
    ]
    .into();
    (b, home, area, Instant::now())
}

fn look(now: Instant) -> Look {
    Look {
        now,
        unicode: true,
        focus: None,
        reduce_motion: false,
    }
}

#[test]
fn melee_crosses_the_field_reaches_the_target_and_returns_home() {
    let (mut b, home, area, start) = scene();
    let hit = start + Duration::from_millis(250);
    let end = start + Duration::from_millis(700);
    let ev = BattleEvent {
        actor: Some(0),
        weapon: "axe".into(),
        effects: vec![BattleEffect {
            target: 15,
            ..Default::default()
        }],
        ..Default::default()
    };
    b.motions
        .push(Motion::from_event(&ev, start, hit, end).unwrap());
    let mid = poses(&b, &home, area, &look(start + Duration::from_millis(125)));
    let contact = poses(&b, &home, area, &look(hit));
    assert!(mid[&0].x < home[&0].x - 15, "向左跨过中场");
    assert!(contact[&0].y.abs_diff(home[&15].y) <= 1, "抵达目标所在排");
    assert!((center(&contact[&0]).0 - center(&home[&15]).0).abs() <= 10.0);
    assert_eq!(poses(&b, &home, area, &look(end))[&0], home[&0]);
}

#[test]
fn ranged_and_reduced_motion_keep_the_body_at_its_station() {
    let (mut b, home, area, start) = scene();
    let ev = BattleEvent {
        actor: Some(0),
        weapon: "bow".into(),
        target: Some(15),
        ..Default::default()
    };
    b.motions.push(
        Motion::from_event(
            &ev,
            start,
            start + Duration::from_millis(250),
            start + Duration::from_millis(700),
        )
        .unwrap(),
    );
    assert_eq!(
        poses(&b, &home, area, &look(start + Duration::from_millis(120)))[&0],
        home[&0]
    );
    let mut reduced = look(start + Duration::from_millis(120));
    reduced.reduce_motion = true;
    assert_eq!(poses(&b, &home, area, &reduced), home);
}

#[test]
fn enemy_pet_reaches_its_owner_and_new_pet_leaves_the_same_owner() {
    let (mut b, home, area, start) = scene();
    let hit = start + Duration::from_millis(250);
    let end = start + Duration::from_millis(700);
    b.swaps.push(SwapFx {
        owner: 14,
        old: Some(15),
        new: Some(5),
        start,
        hit,
        end,
    });
    let recalled = poses(&b, &home, area, &look(hit - Duration::from_millis(1)))[&15];
    assert!(recalled.x.abs_diff(home[&14].x) <= 1);
    assert!(recalled.y.abs_diff(home[&14].y) <= 1);
    let released = poses(&b, &home, area, &look(hit))[&5];
    assert_eq!((released.x, released.y), (home[&14].x, home[&14].y));
    assert_eq!(poses(&b, &home, area, &look(end))[&5], home[&5]);
}

#[test]
fn flown_body_does_not_reappear_at_home_after_leaving() {
    let (mut b, home, area, start) = scene();
    b.flown
        .insert(15, crate::app::battle::Flown { at: start, dir: 1 });
    assert!(poses(&b, &home, area, &look(start + Duration::from_millis(200)))[&15].x > home[&15].x);
    assert!(!poses(&b, &home, area, &look(start + Duration::from_millis(500))).contains_key(&15));
}

#[test]
fn a_guarding_pet_moves_toward_the_intended_victim_instead_of_hitting_itself() {
    let (mut b, home, area, start) = scene();
    let hit = start + Duration::from_millis(250);
    let ev = BattleEvent {
        actor: Some(0),
        weapon: "axe".into(),
        effects: vec![BattleEffect {
            target: 15,
            intent: Some(10),
            result: "hit".into(),
            ..Default::default()
        }],
        ..Default::default()
    };
    b.motions
        .push(Motion::from_event(&ev, start, hit, start + Duration::from_millis(700)).unwrap());
    let at = poses(&b, &home, area, &look(hit));
    assert!(at[&15].x < home[&15].x, "代受者走到主人面前");
    assert!(at[&0].y.abs_diff(home[&10].y) <= 1, "攻击方向仍是原目标");
    assert_eq!(
        b.motions.of(0).unwrap().contacts[0].0,
        15,
        "效果落在真正代受者身上"
    );
}

#[test]
fn counter_happens_at_the_contact_scene_then_the_attacker_returns() {
    let (mut b, home, area, start) = scene();
    let hit = start + Duration::from_millis(250);
    let end = start + Duration::from_millis(1400);
    let ev = BattleEvent {
        actor: Some(0),
        weapon: "axe".into(),
        target: Some(15),
        ..Default::default()
    };
    b.motions
        .push(Motion::from_event_holding(&ev, start, hit, end, true).unwrap());
    let ev = BattleEvent {
        kind: "counter".into(),
        actor: Some(15),
        weapon: "claw".into(),
        target: Some(0),
        ..Default::default()
    };
    b.motions.push(
        Motion::from_event(
            &ev,
            start + Duration::from_millis(700),
            start + Duration::from_millis(950),
            end,
        )
        .unwrap(),
    );
    let contact = poses(&b, &home, area, &look(start + Duration::from_millis(900)))[&0];
    let returning = poses(&b, &home, area, &look(start + Duration::from_millis(1200)))[&0];
    assert!(contact.y.abs_diff(home[&15].y) <= 1);
    assert!(returning.x > contact.x, "反击接触后再走回去");
    assert_eq!(poses(&b, &home, area, &look(end))[&0], home[&0]);
}
