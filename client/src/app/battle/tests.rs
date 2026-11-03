//! 战斗状态的单元测试：开局、回合、续接、目标默认位置、回放期间的结束。

use super::*;

/// 取走本次新增的消息，只留文字（类型另外测）。
fn texts(s: &mut BattleState) -> Vec<String> {
    s.take_pending().into_iter().map(|(_, text)| text).collect()
}

fn unit(no: u8, name: &str, pct: u32) -> BattleUnit {
    BattleUnit { no, name: name.into(), level: 3, hp_pct: pct, ..BattleUnit::default() }
}

fn start() -> BattleStart {
    BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![unit(0, "勇者", 100), unit(5, "乌力", 100), unit(15, "野猪", 100), unit(16, "野猪", 0)],
        ..BattleStart::default()
    }
}

fn turn(turn: u32, needs: &[&str]) -> BattleTurn {
    BattleTurn {
        turn,
        deadline_ms: 1_030_000,
        server_time_ms: 1_000_000,
        needs: needs.iter().map(|s| s.to_string()).collect(),
        progress: Progress { done: 0, total: 1 },
        ..BattleTurn::default()
    }
}

#[test]
fn start_puts_every_unit_on_the_field() {
    let s = BattleState::from_start(start());
    assert_eq!(s.here.len(), 4);
    assert_eq!(s.me_side(), 0);
    assert_eq!(s.records.back().map(String::as_str), Some("进入战斗：野外"));
    assert_eq!(s.name(15), "野猪");
    assert_eq!(s.name(19), "#19", "没见过的单位用协议号");
}

#[test]
fn turn_sets_phase_by_needs_and_countdown_from_server_clock() {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(1, &["player", "pet"]), now);
    assert_eq!(s.phase, Phase::Input(Actor::Player));
    assert_eq!(s.remaining_secs(now), Some(30));
    s.on_turn(turn(2, &["pet"]), now);
    assert_eq!(s.phase, Phase::Input(Actor::Pet), "人物睡着或倒下时直接轮到宠物");
    s.on_turn(turn(3, &[]), now);
    assert_eq!(s.phase, Phase::Waiting);
}

#[test]
fn resume_respects_submitted_commands() {
    let now = Instant::now();
    let snap = BattleSnapshot {
        in_battle: true,
        turn: 2,
        phase: "command".into(),
        needs: vec!["player".into(), "pet".into()],
        submitted: Submitted { player: true, pet: false },
        ..BattleSnapshot::default()
    };
    let s = BattleState::resume(snap, now);
    assert_eq!(s.phase, Phase::Input(Actor::Pet), "人物指令已交，接着等宠物指令");
}

#[test]
fn default_cursor_picks_first_living_enemy_in_front_row() {
    let s = BattleState::from_start(start());
    // 敌方前排左起：16 号（倒下）在 2 号位，15 号在 3 号位；第一个活着的是 15。
    assert_eq!(slots::no_at(s.default_cursor(TargetType::Enemy)), 15);
    assert_eq!(slots::no_at(s.default_cursor(TargetType::Ally)), 0, "己方一个默认落在自己身上");
}

#[test]
fn revive_starts_on_the_first_fallen_ally_and_only_takes_those() {
    let mut s = BattleState::from_start(start());
    s.units.get_mut(&5).unwrap().hp_pct = 0; // 乌力倒下
    assert_eq!(slots::no_at(s.default_cursor(TargetType::AllyDown)), 5, "复活默认落在倒下的己方身上");
    assert!(issue::check_target(&s, TargetType::AllyDown, Act::Magic, 5).is_ok());
    assert!(issue::check_target(&s, TargetType::AllyDown, Act::Magic, 0).is_err(), "活着的不能复活");
    assert!(issue::check_target(&s, TargetType::AllyDown, Act::Magic, 15).is_err(), "敌方不能复活");
    assert!(issue::check_target(&s, TargetType::Ally, Act::Item, 5).is_err(), "倒下的己方不能被治疗");
    s.units.get_mut(&5).unwrap().hp_pct = 100;
    assert_eq!(slots::no_at(s.default_cursor(TargetType::AllyDown)), 0, "没有倒下的己方就落在自己身上");
}

#[test]
fn end_during_replay_waits_for_playback() {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(1, &["player"]), now);
    s.on_round(BattleRound::default(), now);
    s.on_end(BattleEnd { duel_point: None, result: "win".into(), ..BattleEnd::default() });
    assert!(s.result.is_none(), "回放没播完前不弹结果");
    assert!(s.end_after_replay.is_some());
}

#[test]
fn records_read_like_the_battle_log() {
    use crate::proto::{BattleEffect, BattleEvent};
    let s = BattleState::from_start(BattleStart {
        scene: "野外".into(),
        you: 0,
        units: vec![unit(0, "勇者", 100), unit(5, "乌力", 100), unit(15, "野猪", 100)],
        ..BattleStart::default()
    });
    let effect =
        |target, result: &str, delta| BattleEffect { target, result: result.into(), delta, ..BattleEffect::default() };
    let event = |kind: &str, actor| BattleEvent { kind: kind.into(), actor, ..BattleEvent::default() };
    let text = |ev: &BattleEvent| s.describe(ev);

    let heal =
        BattleEvent { name: Some("回复药".into()), effects: vec![effect(0, "heal", 10)], ..event("item", Some(0)) };
    assert_eq!(text(&heal), ["勇者 用回复药 → 勇者 +10"]);
    assert_eq!(text(&event("guard", Some(5))), ["乌力 防御"]);
    let caught = BattleEvent { target: Some(15), ok: Some(true), ..event("capture", Some(0)) };
    assert_eq!(text(&caught), ["勇者 捕捉 野猪：成功"]);
    let missed = BattleEvent { target: Some(15), ok: Some(false), ..event("capture", Some(0)) };
    assert_eq!(text(&missed), ["勇者 捕捉 野猪：失败"]);
    let fled = BattleEvent { ok: Some(true), ..event("escape", Some(0)) };
    assert_eq!(text(&fled), ["勇者 逃跑：成功"]);
    let failed = BattleEvent { ok: Some(false), ..event("escape", Some(0)) };
    assert_eq!(text(&failed), ["勇者 逃跑：失败"]);
    let recall = BattleEvent { pet_slot: Some(-1), left: vec![5], ..event("swap_pet", Some(0)) };
    assert_eq!(text(&recall), ["勇者 收回宠物"]);
    let out = BattleEvent { left: vec![5], joined: vec![unit(6, "波利", 100)], ..event("swap_pet", Some(0)) };
    assert_eq!(text(&out), ["勇者 换宠：乌力 → 波利"]);
    let asleep = BattleEvent { reason: Some("sleep".into()), ..event("skip", Some(15)) };
    assert_eq!(text(&asleep), ["野猪 没有行动（睡着）"]);
    let tick = BattleEvent { effects: vec![effect(0, "hit", -5)], ..event("status_tick", None) };
    assert_eq!(text(&tick), ["勇者 -5"]);
    assert_eq!(text(&event("some_future_kind", Some(15))), ["野猪 行动"], "不认识的事件也有一行");
}

/// 宠物不听话（STAGE-127）：记录写出它为什么这样做，跑掉的宠物写“跑掉了”，两种事件都是认识的种类（有字幕）。
#[test]
fn disobey_reads_in_the_log_and_is_known() {
    use crate::proto::BattleEvent;
    let s = BattleState::from_start(start());
    let ev = |kind: &str| BattleEvent { kind: kind.into(), actor: Some(5), ..BattleEvent::default() };
    let dis = BattleEvent { reason: Some("owner".into()), ..ev("disobey") };
    assert_eq!(s.describe(&dis), ["乌力 不听话：打主人"]);
    let idle = BattleEvent { reason: Some("no_skill".into()), ..ev("disobey") };
    assert_eq!(s.describe(&idle), ["乌力 不听话：没有技能，不出手"]);
    let left = BattleEvent { left: vec![5], ..ev("pet_escape") };
    assert_eq!(s.describe(&left), ["乌力 不听话，跑掉了"]);
    let skipped = BattleEvent { reason: Some("disobey".into()), ..ev("skip") };
    assert_eq!(s.describe(&skipped), ["乌力 没有行动（不听话）"]);
    assert!(super::caption::known("disobey") && super::caption::known("pet_escape"));
}

/// NPC 发起的战斗（STAGE-96）：开战台词排在战斗记录的第一行。
#[test]
fn npc_battle_writes_the_opening_line_first() {
    let npc = BattleNpc {
        id: 1735,
        name: "里昂蛙王".into(),
        say: "看我怎么修理你们这些人类！".into(),
    };
    let mut s = BattleState::from_start(BattleStart {
        npc: Some(npc),
        ..start()
    });
    assert_eq!(
        texts(&mut s),
        ["里昂蛙王：看我怎么修理你们这些人类！", "进入战斗：野外"]
    );
}

/// 赢了 NPC 的结束台词写在结果行之前；没有台词（或输了）就不写。
#[test]
fn npc_end_line_comes_before_the_result() {
    let mut s = BattleState::from_start(start());
    texts(&mut s);
    let npc = BattleNpc {
        id: 1735,
        name: "里昂蛙王".into(),
        say: "我不甘心啊！…".into(),
    };
    s.show_result(BattleEnd { duel_point: None,
        result: "win".into(),
        npc: Some(npc),
        ..BattleEnd::default()
    });
    assert_eq!(texts(&mut s), ["里昂蛙王：我不甘心啊！…", "战斗胜利"]);

    let mut lost = BattleState::from_start(start());
    texts(&mut lost);
    lost.show_result(BattleEnd { duel_point: None,
        result: "lose".into(),
        ..BattleEnd::default()
    });
    assert_eq!(texts(&mut lost), ["战斗失败"]);
}

/// STAGE-109：人物指令交上、宠物还没交时可以退回重选，重交的覆盖先交的；宠物交上或换宠以后不能再改。
#[test]
fn player_command_can_be_revised_until_the_pet_submits() {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(1, &["player", "pet"]), now);
    s.on_accepted("pet");
    assert!(s.can_revert(), "人物攻击交上，宠物还没交");
    s.revert_to_player();
    assert_eq!(s.phase, Phase::Input(Actor::Player));
    assert!(s.revising && s.pick == Pick::Action);
    assert!(!s.can_revert(), "重选人物指令时不能再退回");
    s.on_accepted("pet");
    assert_eq!(s.phase, Phase::Input(Actor::Pet), "重交还是 next=pet，回到宠物指令");
    assert!(s.can_revert() && !s.revising);
    s.on_accepted("wait");
    assert_eq!(s.phase, Phase::Waiting);
    assert!(!s.can_revert(), "宠物指令交上以后不能再改");
}

/// 重选被拒绝时，先交的那条仍然有效，回到宠物指令，还能再退回。
#[test]
fn rejected_revision_goes_back_to_the_pet_command() {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(1, &["player", "pet"]), now);
    s.on_accepted("pet");
    s.revert_to_player();
    s.on_rejected();
    assert_eq!(s.phase, Phase::Input(Actor::Pet));
    assert!(s.can_revert() && !s.revising);
}

/// 换宠交上的人物指令：宠物本回合不需要指令，人物指令不能再改。
#[test]
fn swapping_pet_locks_the_player_command() {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(1, &["player", "pet"]), now);
    s.on_accepted("wait");
    assert!(!s.can_revert());
    assert_eq!(s.phase, Phase::Waiting);
}

#[test]
fn field_shows_from_start_and_clears_when_a_turn_has_none() {
    use crate::proto::BattleField;
    let now = Instant::now();
    let fire = BattleField { attr: "fire".into(), pow: 60, left: 3 };
    let s = BattleState::from_start(BattleStart { field: Some(fire.clone()), ..start() });
    assert_eq!(s.field, Some(fire.clone()), "开战时带着的场地要记下来");
    let mut s = BattleState::from_start(start());
    s.field = Some(fire);
    s.on_turn(turn(2, &["player"]), now);
    assert_eq!(s.field, None, "回合里没有 field 就是无属性（服务端省略了）");
    let none = BattleField { attr: "none".into(), ..BattleField::default() };
    assert!(!none.active(), "none 是无属性");
}

#[test]
fn field_and_reversal_read_in_the_log() {
    use crate::proto::{BattleEffect, BattleEvent, BattleField};
    let s = BattleState::from_start(start());
    let field = |attr: &str, pow, left| BattleField { attr: attr.into(), pow, left };
    let set = BattleEvent { kind: "field".into(), field: Some(field("fire", 60, 3)), ..BattleEvent::default() };
    assert_eq!(s.describe(&set), ["场地变成火属性（60%，3 回合）"]);
    let restored = BattleEvent { kind: "field".into(), field: Some(field("none", 0, 0)), ..BattleEvent::default() };
    assert_eq!(s.describe(&restored), ["场地恢复成无属性"]);
    let flip = BattleEffect { target: 15, result: "none".into(), status_add: vec!["reversed".into()], ..BattleEffect::default() };
    let back = BattleEffect { target: 15, result: "none".into(), status_remove: vec!["reversed".into()], ..BattleEffect::default() };
    let cast = |effect: BattleEffect| BattleEvent {
        kind: "magic".into(),
        actor: Some(0),
        name: Some("彩虹的精灵".into()),
        effects: vec![effect],
        ..BattleEvent::default()
    };
    assert_eq!(s.describe(&cast(flip)), ["勇者 用彩虹的精灵 → 野猪 反转开"]);
    assert_eq!(s.describe(&cast(back)), ["勇者 用彩虹的精灵 → 野猪 反转关"]);
}

#[test]
fn any_target_takes_either_side_but_not_the_fallen_or_empty_cells() {
    let s = BattleState::from_start(start());
    for no in [0, 5, 15] {
        assert!(issue::check_target(&s, TargetType::Any, Act::Magic, no).is_ok(), "{no} 号活着，任一方都能选");
    }
    assert!(issue::check_target(&s, TargetType::Any, Act::Magic, 16).is_err(), "16 号倒下了");
    assert!(issue::check_target(&s, TargetType::Any, Act::Magic, 1).is_err(), "空格子");
}

/// STAGE-199：防御结果的字幕（吸收回血、反射指回弹开它的那一方、无效）。
#[test]
fn defense_results_read_in_the_caption() {
    use crate::proto::{BattleEffect, BattleEvent};
    let s = BattleState::from_start(start());
    let effect = |target: u8, result: &str, delta: i64| BattleEffect {
        target,
        result: result.into(),
        delta,
        ..BattleEffect::default()
    };
    let text = |ev: &BattleEvent| -> String {
        caption::build(&s, ev, true)
            .iter()
            .map(|p| p.text.as_str())
            .collect()
    };
    let absorb = BattleEvent {
        kind: "attack".into(),
        actor: Some(15),
        effects: vec![effect(0, "absorb", 5)],
        ..BattleEvent::default()
    };
    assert!(
        text(&absorb).contains("吸收伤害，回复 5"),
        "{}",
        text(&absorb)
    );
    let mut reflect = effect(15, "reflect", -40);
    reflect.intent = Some(0);
    let reflect = BattleEvent {
        kind: "attack".into(),
        actor: Some(15),
        effects: vec![reflect],
        ..BattleEvent::default()
    };
    assert!(
        text(&reflect).contains("把伤害反弹给 勇者"),
        "{}",
        text(&reflect)
    );
    let vanish = BattleEvent {
        kind: "attack".into(),
        actor: Some(15),
        effects: vec![effect(0, "vanish", 0)],
        ..BattleEvent::default()
    };
    assert!(text(&vanish).contains("攻击无效"), "{}", text(&vanish));
}

/// STAGE-202：召唤龙阵的字幕是“XX 召唤了龙，打向 YY”；一个目标写名字和结果，多个目标写“敌方全体 ×N”。
#[test]
fn dragon_call_reads_as_summoning_in_the_caption() {
    use crate::proto::{BattleEffect, BattleEvent, SpellView};
    let s = BattleState::from_start(start());
    let effect = |target: u8, result: &str, delta: i64| BattleEffect {
        target,
        result: result.into(),
        delta,
        ..BattleEffect::default()
    };
    let cast = |effects: Vec<BattleEffect>| BattleEvent {
        kind: "magic".into(),
        actor: Some(0),
        name: Some("奔龙阵".into()),
        magic: Some(SpellView {
            func: "dragon".into(),
            ..SpellView::default()
        }),
        effects,
        ..BattleEvent::default()
    };
    let text = |ev: &BattleEvent, hit: bool| -> String {
        caption::build(&s, ev, hit)
            .iter()
            .map(|p| p.text.as_str())
            .collect()
    };
    let one = cast(vec![effect(15, "hit", -40)]);
    let start_beat = text(&one, false);
    assert!(
        start_beat.contains("召唤了龙，打向") && start_beat.contains("野猪"),
        "{start_beat}"
    );
    assert!(!start_beat.contains("-40"), "起手拍不写结果: {start_beat}");
    assert!(text(&one, true).contains("-40"), "{}", text(&one, true));
    let dodge = cast(vec![effect(15, "dodge", 0)]);
    assert!(
        text(&dodge, true).contains("闪避"),
        "{}",
        text(&dodge, true)
    );
    let all = cast(vec![effect(15, "hit", -40), effect(16, "hit", -20)]);
    assert!(
        text(&all, false).contains("敌方全体 ×2"),
        "{}",
        text(&all, false)
    );
}

/// STAGE-229：魔抗系状态（STAGE-228）的字幕写成中文名，挂上是“魔抗”，解除是“解除魔抗”；火、电、冰抗同样写名字。
#[test]
fn resist_statuses_read_in_the_caption() {
    use crate::proto::{BattleEffect, BattleEvent};
    let s = BattleState::from_start(start());
    let text = |ev: &BattleEvent| -> String {
        caption::build(&s, ev, true)
            .iter()
            .map(|p| p.text.as_str())
            .collect()
    };
    let cast = |effect: BattleEffect| BattleEvent {
        kind: "magic".into(),
        actor: Some(0),
        name: Some("魔法防御".into()),
        effects: vec![effect],
        ..BattleEvent::default()
    };
    let add = |st: &str| BattleEffect {
        target: 0,
        result: "none".into(),
        status_add: vec![st.into()],
        ..BattleEffect::default()
    };
    let remove = |st: &str| BattleEffect {
        target: 0,
        result: "none".into(),
        status_remove: vec![st.into()],
        ..BattleEffect::default()
    };
    let cases = [
        (add("magic_resist"), "魔抗"),
        (remove("magic_resist"), "解除魔抗"),
        (add("fire_resist"), "火抗"),
        (add("thunder_resist"), "电抗"),
        (add("ice_resist"), "冰抗"),
    ];
    for (effect, name) in cases {
        let line = text(&cast(effect));
        assert!(line.contains(name), "{line}");
    }
}

/// 打到结果窗口弹出的状态（阶段 Ended）：第 2 回合出招后胜利。
fn ended_state() -> BattleState {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(2, &["player"]), now);
    s.on_end(BattleEnd { duel_point: None, result: "win".into(), turn: 2, ..BattleEnd::default() });
    assert_eq!(s.phase, Phase::Ended);
    s
}

/// STAGE-282：结果窗口弹出后，迟到的旧回合（`battle_round`）和新回合（`battle_turn`）都不改阶段，
/// 否则界面又回到“结算中 / 指令”，倒计时重新跑起来，像卡在战斗里。
#[test]
fn late_round_or_turn_after_end_keeps_phase_ended() {
    let now = Instant::now();
    let mut s = ended_state();
    s.on_round(BattleRound { turn: 2, ..BattleRound::default() }, now);
    assert_eq!(s.phase, Phase::Ended, "迟到的 battle_round 不能把阶段打回回放");
    assert!(s.playback.is_none(), "也不重播");
    s.on_turn(turn(3, &["player"]), now);
    assert_eq!(s.phase, Phase::Ended, "迟到的 battle_turn 不能把阶段打回指令");
    assert_eq!(s.turn, 2, "回合号不变");
}

/// 一个带世界和战斗的离线 `App`：没有连接，所以任何一次真正的发送都会走 `handle_closed` 把世界置为离线，
/// 测试用 `world.online` 是否还在来判断“有没有发出请求”。
fn app_with(battle: BattleState) -> crate::app::App {
    let mut a = crate::app::App::new(crate::app::Options::default());
    let mut w = crate::app::WorldState::new(crate::proto::WorldEntry::default());
    w.battle = Some(battle);
    a.world = Some(w);
    a
}

fn online(a: &crate::app::App) -> bool {
    a.world.as_ref().is_some_and(|w| w.online)
}

/// STAGE-282：先 `battle_end` 再迟到 `battle_round`——客户端不再发 `battle_ack`、阶段保持 Ended。
#[test]
fn no_ack_after_end_even_if_a_late_round_arrives() {
    let mut a = app_with(ended_state());
    a.on_battle_push("battle_round", serde_json::json!({"battle_id": "b_1", "turn": 2, "events": [], "state": []}));
    assert_eq!(a.world.as_ref().unwrap().battle.as_ref().unwrap().phase, Phase::Ended);
    a.battle_tick(Instant::now() + std::time::Duration::from_secs(5));
    a.after_replay(); // 即使有人直接调“回放播完”也不能确认
    assert!(online(&a), "结束之后发了 battle_ack（离线应用里发送会把世界置为离线）");
    assert_eq!(a.world.as_ref().unwrap().battle.as_ref().unwrap().phase, Phase::Ended);
}

/// STAGE-282：回放播完时战斗还没结束才确认；这一步原样保留（对照上一条）。
#[test]
fn ack_is_still_sent_when_a_replay_finishes_mid_battle() {
    let now = Instant::now();
    let mut s = BattleState::from_start(start());
    s.on_turn(turn(1, &["player"]), now);
    s.on_round(BattleRound { turn: 1, ..BattleRound::default() }, now);
    let mut a = app_with(s);
    a.after_replay();
    assert!(!online(&a), "回放播完应该确认（离线应用里发送会把世界置为离线）");
}

/// STAGE-282：结果窗口显示中，指令（含托管的自动出招）一律不发。
#[test]
fn commands_are_not_sent_after_the_battle_has_ended() {
    let mut a = app_with(ended_state());
    let sent = a.send_command(Actor::Player, Act::Attack, Some(15), None);
    assert!(!sent, "战斗已结束，不该发 battle_command");
    assert!(online(&a));
}
