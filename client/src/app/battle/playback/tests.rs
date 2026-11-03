use super::effect::dodge_dir;
use super::*;
use crate::app::battle::{FloatKind, FxKind, Part};
use crate::proto::{BattleAllow, BattleStart, BattleTurn, BattleUnit, Mount};

fn unit(no: u8, name: &str, pct: u32) -> BattleUnit {
    BattleUnit { no, name: name.into(), level: 3, hp_pct: pct, ..BattleUnit::default() }
}

fn effect(target: u8, result: &str, delta: i64, pct: u32, down: bool) -> BattleEffect {
    BattleEffect { target, result: result.into(), delta, hp_pct: Some(pct), down, ..BattleEffect::default() }
}

fn battle() -> BattleState {
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow::default(),
        units: vec![unit(0, "勇者", 100), unit(5, "乌力", 100), unit(15, "野猪", 100), unit(16, "野猪", 100)],
        ..BattleStart::default()
    };
    let mut s = BattleState::from_start(start);
    s.on_turn(BattleTurn { turn: 1, needs: vec!["player".into()], ..BattleTurn::default() }, Instant::now());
    s
}

fn round(events: Vec<BattleEvent>, state: Vec<BattleUnitState>) -> BattleRound {
    BattleRound { turn: 1, events, state, ..BattleRound::default() }
}

fn at(t0: Instant, millis: u64) -> Instant {
    t0 + Duration::from_millis(millis)
}

fn text(parts: &[Part]) -> String {
    parts.iter().map(|p| p.text.as_str()).collect()
}

fn attack(actor: u8, target: u8, delta: i64, pct: u32) -> BattleEvent {
    BattleEvent {
        kind: "attack".into(),
        actor: Some(actor),
        effects: vec![effect(target, "hit", delta, pct, false)],
        ..BattleEvent::default()
    }
}

#[test]
fn two_beats_per_event_and_hp_changes_on_the_hit_beat() {
    let t0 = Instant::now();
    let mut s = battle();
    let state = vec![BattleUnitState { no: 15, hp_pct: 40, ..BattleUnitState::default() }];
    s.on_round(round(vec![attack(0, 15, -23, 70), attack(5, 15, -10, 40)], state), t0);

    assert!(!s.advance(t0), "起手拍：行动者亮出来");
    assert_eq!(s.acting.map(|(no, _)| no), Some(0));
    assert_eq!(s.units[&15].hp_pct, 100, "命中拍之前血量不变");
    assert_eq!(text(&s.caption), "我8 勇者 ━━ 攻击 ━━▶ 敌3 野猪", "起手拍的字幕还没有结果");

    assert!(!s.advance(at(t0, 249)));
    assert_eq!(s.units[&15].hp_pct, 100);
    assert!(!s.advance(at(t0, 250)), "命中拍：血量和字幕结果");
    assert_eq!(s.units[&15].hp_pct, 70);
    assert_eq!(text(&s.caption), "我8 勇者 ━━ 攻击 ━━▶ 敌3 野猪  -23");

    assert!(!s.advance(at(t0, 700)), "第二条开始，这条还没命中");
    assert_eq!(s.units[&15].hp_pct, 70);
    assert!(!s.advance(at(t0, 950)));
    assert_eq!(s.units[&15].hp_pct, 40);
    assert!(s.advance(at(t0, 1400)), "两条都走完两拍，用最终状态校正，并报告播完");
    assert_eq!(s.units[&15].hp_pct, 40);
    assert!(s.playback.is_none() && s.caption.is_empty());
}

#[test]
fn floats_fx_and_records_follow_the_result_kind() {
    let t0 = Instant::now();
    let mut s = battle();
    let crit = BattleEvent {
        kind: "attack".into(),
        actor: Some(15),
        effects: vec![effect(5, "crit", -46, 0, true)],
        ..BattleEvent::default()
    };
    s.on_round(round(vec![crit], vec![]), t0);
    assert!(!s.advance(t0));
    assert!(s.floats.is_empty(), "飘字在命中拍才出现");
    assert!(!s.advance(at(t0, 250)));
    let f = &s.floats[0];
    assert_eq!((f.no, f.text.as_str(), f.kind), (5, "-46!", FloatKind::Crit));
    let kinds: Vec<FxKind> = s.fx.iter().filter(|f| f.no == 5).map(|f| f.kind).collect();
    assert_eq!(kinds, [FxKind::Down], "倒下只播倒下的效果（红色闪），不再另外播受伤");
    assert_eq!(s.units[&5].hp_pct, 0, "倒下的单位血量为 0");
    let lines: Vec<String> = s.records.iter().cloned().collect();
    assert!(lines.contains(&"野猪 攻击 乌力 -46!".to_string()), "{lines:?}");
    assert!(lines.contains(&"乌力 倒下了".to_string()), "{lines:?}");
}

#[test]
fn damage_leaves_a_ghost_bar_that_shrinks_and_heal_fills_up() {
    let t0 = Instant::now();
    let mut s = battle();
    let heal = BattleEvent {
        kind: "item".into(),
        actor: Some(0),
        name: Some("回复药".into()),
        effects: vec![effect(15, "heal", 30, 70, false)],
        ..BattleEvent::default()
    };
    s.units.get_mut(&15).unwrap().hp_pct = 40;
    s.on_round(round(vec![attack(0, 15, -30, 10), heal], vec![]), t0);
    assert!(!s.advance(t0));
    assert!(!s.advance(at(t0, 250)));
    let hurt = s.hp_moves[0];
    assert_eq!((hurt.no, hurt.from, hurt.to), (15, 40, 10));
    assert_eq!(hurt.until, at(t0, 250 + 300), "残影 300ms 缩掉");
    assert!(!s.advance(at(t0, 700)), "第二条（回复）从 700ms 开始");
    assert!(!s.advance(at(t0, 950)));
    let healed = s.hp_moves.last().copied().unwrap();
    assert_eq!((healed.from, healed.to), (10, 70));
    assert_eq!(healed.until, at(t0, 950 + 350), "回复在命中拍内涨完");
}

#[test]
fn caption_names_the_group_and_stays_short_for_many_targets() {
    let t0 = Instant::now();
    let mut s = battle();
    let group = BattleEvent {
        kind: "magic".into(),
        actor: Some(0),
        name: Some("陨石".into()),
        effects: vec![effect(15, "hit", -10, 90, false), effect(16, "hit", -10, 90, false)],
        ..BattleEvent::default()
    };
    s.on_round(round(vec![group], vec![]), t0);
    assert!(!s.advance(at(t0, 250)));
    assert_eq!(text(&s.caption), "我8 勇者 ━━【陨石】━━▶ 敌方全体 ×2");
}

#[test]
fn capture_pauses_before_the_verdict() {
    let t0 = Instant::now();
    let mut s = battle();
    let catch = BattleEvent {
        kind: "capture".into(),
        actor: Some(0),
        target: Some(15),
        ok: Some(true),
        ..BattleEvent::default()
    };
    s.on_round(round(vec![catch], vec![]), t0);
    assert!(!s.advance(t0));
    assert!(!s.advance(at(t0, 250)));
    assert_eq!(text(&s.caption), "我8 勇者 ━━ 捕捉 ━━▶ 敌3 野猪  · · ·", "起手后停一拍，还没有结果");
    assert!(!s.advance(at(t0, 650)));
    assert_eq!(text(&s.caption), "我8 勇者 ━━ 捕捉 ━━▶ 敌3 野猪  成功！");
}

#[test]
fn faster_speed_halves_every_beat() {
    let t0 = Instant::now();
    let mut s = battle();
    s.speed = 2;
    s.on_round(round(vec![attack(0, 15, -23, 70)], vec![]), t0);
    assert!(!s.advance(t0));
    assert!(!s.advance(at(t0, 125)));
    assert_eq!(s.units[&15].hp_pct, 70, "2x 时命中拍在 125ms");
    assert!(s.advance(at(t0, 350)), "2x 时整条 350ms 就走完");
}

#[test]
fn replay_progress_counts_the_events_started() {
    let t0 = Instant::now();
    let mut s = battle();
    s.on_round(round(vec![attack(0, 15, -1, 99), attack(5, 15, -1, 98)], vec![]), t0);
    assert_eq!(s.replay_progress(), Some((0, 2)));
    s.advance(t0);
    assert_eq!(s.replay_progress(), Some((1, 2)));
    s.advance(at(t0, 700));
    assert_eq!(s.replay_progress(), Some((2, 2)));
}

#[test]
fn dodge_pushes_the_body_away_from_the_actor_on_both_sides() {
    for me_side in [0, 1] {
        let me = me_side * 10;
        let enemy = (1 - me_side) * 10;
        for offset in 0..10 {
            assert_eq!(dodge_dir(Some(me), enemy + offset, me_side), -1);
            assert_eq!(dodge_dir(Some(enemy), me + offset, me_side), 1);
        }
        assert_eq!(dodge_dir(None, me, me_side), 1);
        assert_eq!(dodge_dir(None, enemy, me_side), -1);
    }
}

#[test]
fn skip_applies_everything_at_once_without_animation() {
    let t0 = Instant::now();
    let mut s = battle();
    let swap = BattleEvent {
        kind: "swap_pet".into(),
        actor: Some(0),
        pet_slot: Some(1),
        left: vec![5],
        joined: vec![unit(5, "波利", 100)],
        ..BattleEvent::default()
    };
    let miss = BattleEvent {
        kind: "attack".into(),
        actor: Some(15),
        effects: vec![effect(0, "miss", 0, 100, false)],
        ..BattleEvent::default()
    };
    s.on_round(round(vec![swap, miss], vec![]), t0);
    assert!(s.skip());
    assert!(s.floats.is_empty() && s.acting.is_none() && s.fx.is_empty(), "跳过不做动画");
    assert!(s.caption.is_empty() && s.current_lines.is_none());
    assert_eq!(s.units[&5].name, "波利", "换上的宠物用新的名字");
    assert!(s.here.contains(&5), "换上的宠物在场");
    assert!(s.records.iter().any(|l| l == "勇者 换宠：乌力 → 波利"), "{:?}", s.records);
}

#[test]
fn skip_finishes_the_event_that_is_playing_without_animation() {
    let t0 = Instant::now();
    let mut s = battle();
    s.on_round(round(vec![attack(0, 15, -23, 70)], vec![]), t0);
    assert!(!s.advance(t0), "正在起手");
    assert!(s.skip());
    assert_eq!(s.units[&15].hp_pct, 70, "正在播的那条直接命中");
}

#[test]
fn unknown_kinds_still_apply_their_effects_without_a_caption() {
    let t0 = Instant::now();
    let mut s = battle();
    let weird = BattleEvent {
        kind: "future_kind".into(),
        effects: vec![effect(15, "hit", -7, 60, false)],
        ..BattleEvent::default()
    };
    s.on_round(round(vec![weird], vec![]), t0);
    assert!(!s.advance(t0));
    assert_eq!(s.units[&15].hp_pct, 60, "不认识的事件立即应用效果");
    assert!(s.caption.is_empty() && s.fx.is_empty() && s.floats.is_empty(), "不写字幕、不播动画");
}

/// STAGE-126：合击是一个事件，actors 里的出手者一起亮，字幕把几个人写在前面，飘字合成一个总数，记录写成一行。
#[test]
fn combo_is_one_beat_with_all_actors_and_one_summed_float() {
    let t0 = Instant::now();
    let mut s = battle();
    let combo = BattleEvent {
        kind: "combo".into(),
        actor: Some(5),
        actors: vec![5, 0],
        effects: vec![effect(15, "hit", -5, 64, false), effect(15, "crit", -6, 21, false)],
        ..BattleEvent::default()
    };
    let state = vec![BattleUnitState {
        no: 15,
        hp_pct: 21,
        ..BattleUnitState::default()
    }];
    s.on_round(round(vec![combo], state), t0);

    assert!(!s.advance(t0), "起手拍");
    assert_eq!(s.acting.map(|(no, _)| no), Some(5));
    assert_eq!(s.acting_with, vec![0], "起头以外的出手者也亮");
    assert_eq!(s.motions.all().len(), 2, "合击每个单位都有独立动作");
    assert_eq!(s.motions.of(0).unwrap().contacts[0].1, crate::app::battle::Outcome::Crit);
    assert_eq!(s.motions.of(5).unwrap().contacts[0].1, crate::app::battle::Outcome::Hit);
    assert_eq!(text(&s.caption), "我3 乌力 + 我8 勇者 ━━ 合击 ━━▶ 敌3 野猪");

    assert!(!s.advance(at(t0, 250)), "命中拍");
    assert_eq!(s.units[&15].hp_pct, 21, "两下依次生效");
    assert_eq!(
        text(&s.caption),
        "我3 乌力 + 我8 勇者 ━━ 合击 ━━▶ 敌3 野猪  -5 -6 暴击！"
    );
    let floats: Vec<(String, FloatKind)> = s
        .floats
        .iter()
        .filter(|f| f.no == 15)
        .map(|f| (f.text.clone(), f.kind))
        .collect();
    assert_eq!(
        floats,
        vec![("-11!".to_string(), FloatKind::Crit)],
        "同一个目标的飘字合成总数"
    );
    assert_eq!(
        s.records.back().map(String::as_str),
        Some("乌力 + 勇者 合击 野猪 -5 -6!")
    );

    assert!(s.advance(at(t0, 700)));
    assert!(s.acting_with.is_empty(), "播完清掉");
}

/// STAGE-250：回合末的 `state` 校正骑宠血量（每回合刷新）；没骑乘的人物项没有 `mount`，校正时清掉。
#[test]
fn round_end_state_refreshes_the_mount_hp() {
    let mut s = battle();
    let pet = |hp| Mount {
        name: "贝鲁卡".into(),
        hp: Some(hp),
        max_hp: Some(40),
        hp_pct: hp * 100 / 40,
    };
    s.units.get_mut(&0).unwrap().mount = Some(pet(40));
    let t0 = Instant::now();
    let state = vec![BattleUnitState {
        no: 0,
        hp_pct: 100,
        mount: Some(pet(39)),
        ..BattleUnitState::default()
    }];
    s.on_round(round(vec![], state), t0);
    assert!(s.advance(t0));
    assert_eq!(s.units[&0].mount.as_ref().and_then(|m| m.hp), Some(39));

    let state = vec![BattleUnitState { no: 0, hp_pct: 100, ..BattleUnitState::default() }];
    s.on_round(round(vec![], state), t0);
    assert!(s.advance(t0));
    assert_eq!(s.units[&0].mount, None, "没骑乘的 state 项清掉 mount");
}
