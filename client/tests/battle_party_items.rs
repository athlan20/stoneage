//! STAGE-275：真人同场战斗里的目标（队友能选回复、复活类，攻击不能），以及队长、队员在战斗中离队的文案
//! （`docs/battle-ui.md`「真人队同场战斗」「离场」）。不经过网络：战场用 `BattleStart` 搭出来，离队推送直接喂给 `App`。

use serde_json::json;
use stoneage_client::app::battle::party::{disband_text, member_leave_text};
use stoneage_client::app::battle::{Act, BattleState};
use stoneage_client::app::{App, Kind, Options, Screen, WorldState};
use stoneage_client::net::NetEvent;
use stoneage_client::proto::{BattleStart, Incoming, TargetType, WorldEntry};

/// 真人队同场：队长 勇者小明（0 号，自己）、队员 路人乙（1 号）及其宠物乌力（6 号），敌方野猪（15 号）。
fn party_battle() -> BattleState {
    let start: BattleStart = serde_json::from_value(json!({
        "battle_id": "b_1", "kind": "party", "scene": "野外", "you": 0,
        "allow": {"escape": true, "capture": true},
        "units": [
            {"no": 0, "kind": "player", "name": "勇者小明", "level": 1, "hp": 44, "max_hp": 44, "hp_pct": 100,
             "mp": 100, "max_mp": 100, "status": [], "ride": false},
            {"no": 1, "kind": "player", "name": "路人乙", "level": 1, "hp": 40, "max_hp": 40, "hp_pct": 100,
             "mp": 60, "max_mp": 60, "status": [], "ride": false},
            {"no": 5, "kind": "pet", "name": "小豆", "level": 5, "hp": 30, "max_hp": 30, "hp_pct": 100, "status": [], "ride": false},
            {"no": 6, "kind": "pet", "name": "乌力", "level": 5, "hp": 35, "max_hp": 35, "hp_pct": 100, "status": [], "ride": false},
            {"no": 15, "kind": "monster", "name": "野猪", "level": 3, "hp_pct": 100, "status": [], "ride": false}
        ],
        "party": {"members": [{"no": 0, "name": "勇者小明", "leader": true}, {"no": 1, "name": "路人乙", "leader": false}]}
    }))
    .unwrap();
    BattleState::from_start(start)
}

/// 路人乙倒下（血量 0）的真人队战斗。
fn party_battle_mate_down() -> BattleState {
    let mut b = party_battle();
    b.units.get_mut(&1).unwrap().hp_pct = 0;
    b
}

/// 处于世界中的 App，队伍是 `kind` 类（`players` 真人队、`clone` 分身队）。队长是勇者小明，队员是路人乙。
fn app_in_party(kind: &str) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    a.screen = Screen::World;
    let mut entry = WorldEntry::default();
    entry.character.name = "勇者小明".into();
    entry.position.floor = 1000;
    entry.position.map_name = "萨姆吉尔村".into();
    entry.party = serde_json::from_value(json!({
        "kind": kind, "leader": "勇者小明",
        "members": [{"index": 0, "name": "勇者小明", "leader": true}, {"index": 1, "name": "路人乙", "leader": false}]
    }))
    .ok();
    a.world = Some(WorldState::new(entry));
    a
}

/// 推送一条 `party_changed`（同 chat_public.rs 的做法：直接交给网络事件入口）。
fn push_party_changed(a: &mut App, data: serde_json::Value) {
    a.on_net(NetEvent::Message {
        gen_id: 0,
        msg: Incoming::Push {
            ty: "party_changed".into(),
            data,
        },
    });
}

/// 队伍类消息里有没有这一行（完全相等）。
fn party_logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.kind == Kind::Party && m.text == text)
}

#[test]
fn heal_and_revive_items_and_spells_can_pick_a_mate_and_his_pet() {
    let b = party_battle();
    assert!(b.can_pick(TargetType::Ally, Act::Item, 1), "回复道具可以选队友");
    assert!(b.can_pick(TargetType::Ally, Act::Item, 6), "回复道具可以选队友的宠物");
    assert!(b.can_pick(TargetType::Ally, Act::Magic, 1), "回复精灵可以选队友");
    assert!(b.can_pick(TargetType::Ally, Act::Magic, 6), "回复精灵可以选队友的宠物");
}

#[test]
fn revive_items_and_spells_pick_a_downed_mate_only() {
    let alive = party_battle();
    assert!(
        !alive.can_pick(TargetType::AllyDown, Act::Item, 1),
        "活着的队友不能复活"
    );
    let down = party_battle_mate_down();
    assert!(
        down.can_pick(TargetType::AllyDown, Act::Item, 1),
        "复活道具可以选倒下的队友"
    );
    assert!(
        down.can_pick(TargetType::AllyDown, Act::Magic, 1),
        "复活精灵可以选倒下的队友"
    );
    assert!(!down.can_pick(TargetType::AllyDown, Act::Item, 15), "复活不能选怪物");
}

#[test]
fn attacks_captures_and_pet_skills_never_pick_a_mate() {
    let b = party_battle();
    assert!(!b.can_pick(TargetType::Enemy, Act::Attack, 1), "普通攻击不能选队友");
    assert!(
        !b.can_pick(TargetType::Enemy, Act::Attack, 6),
        "普通攻击不能选队友的宠物"
    );
    assert!(!b.can_pick(TargetType::Enemy, Act::Capture, 1), "捕捉不能选队友");
    assert!(!b.can_pick(TargetType::Enemy, Act::Magic, 1), "攻击精灵不能选队友");
    assert!(
        !b.can_pick(TargetType::Ally, Act::Skill, 1),
        "宠物技能不在回复类里，不能选队友"
    );
    assert!(!b.can_pick(TargetType::Any, Act::Magic, 1), "属性反转不能选队友");
    assert!(b.can_pick(TargetType::Enemy, Act::Attack, 15), "攻击敌方照常");
}

#[test]
fn disband_text_names_the_leaders_cause_and_keeps_the_plain_text_without_one() {
    assert_eq!(disband_text("battle_escape"), "队长逃跑，队伍解散");
    assert_eq!(disband_text("battle_timeout"), "队长超时未下指令，被移出战斗，队伍解散");
    assert_eq!(disband_text("battle_offline"), "队长掉线，队伍解散");
    assert_eq!(disband_text(""), "队伍已解散");
}

#[test]
fn member_leave_text_names_the_cause_and_says_you_for_yourself() {
    assert_eq!(
        member_leave_text("路人乙", false, "battle_escape").unwrap(),
        "路人乙 逃跑离队"
    );
    assert_eq!(
        member_leave_text("路人乙", false, "battle_timeout").unwrap(),
        "路人乙 超时未下指令，被移出战斗"
    );
    assert_eq!(
        member_leave_text("路人乙", false, "battle_offline").unwrap(),
        "路人乙 掉线，离开队伍"
    );
    assert_eq!(
        member_leave_text("勇者小明", true, "battle_timeout").unwrap(),
        "你超时未下指令，被移出战斗"
    );
    assert!(member_leave_text("路人乙", false, "").is_none(), "没有原因时沿用原文字");
}

#[test]
fn leader_leaving_a_battle_disbands_with_the_cause_in_the_log() {
    let mut a = app_in_party("players");
    push_party_changed(
        &mut a,
        json!({"reason": "disbanded", "who": "勇者小明", "cause": "battle_escape", "party": null}),
    );
    assert!(party_logged(&a, "队长逃跑，队伍解散"), "{:?}", a.log.all());
    assert!(!party_logged(&a, "队伍已解散"), "真人队不写分身的文案");
    assert!(a.world.as_ref().unwrap().party().is_none(), "队伍已经解散");
}

#[test]
fn leader_timeout_and_offline_disband_say_their_cause() {
    let mut a = app_in_party("players");
    push_party_changed(
        &mut a,
        json!({"reason": "disbanded", "who": "勇者小明", "cause": "battle_timeout", "party": null}),
    );
    assert!(
        party_logged(&a, "队长超时未下指令，被移出战斗，队伍解散"),
        "{:?}",
        a.log.all()
    );

    let mut a = app_in_party("players");
    push_party_changed(
        &mut a,
        json!({"reason": "disbanded", "who": "勇者小明", "cause": "battle_offline", "party": null}),
    );
    assert!(party_logged(&a, "队长掉线，队伍解散"), "{:?}", a.log.all());
}

#[test]
fn member_leaving_a_battle_says_the_cause() {
    let mut a = app_in_party("players");
    push_party_changed(
        &mut a,
        json!({"reason": "member_left", "who": "路人乙", "cause": "battle_offline", "party": {"kind": "players", "leader": "勇者小明", "members": []}}),
    );
    assert!(party_logged(&a, "路人乙 掉线，离开队伍"), "{:?}", a.log.all());

    let mut a = app_in_party("players");
    push_party_changed(
        &mut a,
        json!({"reason": "member_kicked", "who": "路人乙", "cause": "battle_timeout", "party": {"kind": "players", "leader": "勇者小明", "members": []}}),
    );
    assert!(party_logged(&a, "路人乙 超时未下指令，被移出战斗"), "{:?}", a.log.all());
}

#[test]
fn clone_party_disband_text_is_unchanged() {
    let mut a = app_in_party("clone");
    push_party_changed(&mut a, json!({"reason": "disbanded", "party": null}));
    assert!(party_logged(&a, "队伍已解散，分身消失"), "{:?}", a.log.all());
}
