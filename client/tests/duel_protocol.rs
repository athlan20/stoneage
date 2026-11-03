//! STAGE-307：决斗的协议解析。用 mock 服务端的做法（`--conn` 扮演某个连接）读 `duel_basic` 用例，
//! 检查客户端能解析应战方收到的 `battle_start`（kind 为 duel、单位号 10）、决斗结束的 `battle_end`（duel_point、rewards 为 null），
//! 以及附近玩家的 `duel` 标记和 `enter_world` 里的 `duel_point`、`social.duel`。界面见 `app/duel.rs` 和 `ui/battle/result.rs` 的单元测试。

#[allow(dead_code)]
#[path = "../src/bin/mock_server/script.rs"]
mod script;

use std::path::Path;

use serde_json::Value;
use stoneage_client::proto::{BattleEnd, BattleStart, DuelPoint, PlayerList, PlayersChanged, Social};

/// 扮演 `conn` 连接时收到的 `ty` 类 s2c 消息的 `data`（按用例顺序，已物化通配）。
fn s2c(case: &str, conn: &str, ty: &str) -> Vec<Value> {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../docs/protocol/cases");
    let plan = script::load_as(&root.join(format!("{case}.jsonl")), conn).unwrap();
    let mut vars = script::Vars::new();
    plan.steps
        .iter()
        .filter(|s| s.dir == script::Dir::S2c && s.ty() == ty)
        .map(|s| script::materialize(&s.msg, &mut vars)["data"].clone())
        .collect()
}

#[test]
fn challenged_side_gets_a_duel_start_with_its_own_number_10() {
    let start: BattleStart = serde_json::from_value(s2c("duel_basic", "alice", "battle_start").remove(0)).unwrap();
    assert_eq!(start.kind, "duel");
    assert_eq!(start.you, 10, "应战方的单位号是 10–14");
    let data = s2c("duel_basic", "alice", "battle_start").remove(0);
    assert_eq!(data["duel"]["sides"].as_array().map(Vec::len), Some(2));
    assert_eq!(data["duel"]["sides"][0]["leader"], "路人乙", "发起方在前");
    assert_eq!(data["duel"]["sides"][1]["leader"], "勇者小明");
    assert!(start.party.is_none(), "决斗不带 party");
}

#[test]
fn duel_end_carries_duel_point_and_no_rewards() {
    let end: BattleEnd = serde_json::from_value(s2c("duel_basic", "alice", "battle_end").remove(0)).unwrap();
    assert_eq!(end.result, "lose");
    assert_eq!(end.duel_point, Some(DuelPoint { change: -10, total: 90 }));
    assert!(end.rewards.exp.is_empty() && end.rewards.gold == 0 && end.rewards.items.is_empty());
}

#[test]
fn nearby_players_carry_the_duel_flag() {
    let list: PlayerList = serde_json::from_value(s2c("duel_basic", "bob", "players").remove(0)).unwrap();
    assert!(!list.players[0].duel, "用例开场时对方没开");
    let pushes: Vec<PlayersChanged> = s2c("duel_basic", "bob", "players_changed")
        .into_iter()
        .map(|d| serde_json::from_value(d).unwrap())
        .collect();
    let update = pushes
        .iter()
        .flat_map(|p| p.changes.iter())
        .find(|c| c.op == "update")
        .and_then(|c| c.player.as_ref())
        .expect("开关打开后推 update");
    assert!(update.duel);
}

#[test]
fn enter_world_carries_duel_point_and_social_duel_off() {
    let data = s2c("duel_basic", "alice", "enter_world").remove(0);
    assert_eq!(data["duel_point"], 100);
    let social: Social = serde_json::from_value(data["social"].clone()).unwrap();
    assert!(!social.duel, "接受决斗默认关");
    assert!(social.visible && social.invite && social.trade);
}
