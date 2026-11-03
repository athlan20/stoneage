//! STAGE-250：骑宠血量随战斗单位下发（`docs/protocol.md` 10.2、10.7 的 `mount`）。
//! 用 mock 服务端的用例读取和物化（和 `mock-server` 回放同一条路）走一遍 `battle_ride`：
//! `battle_start` 和每回合 `state` 里的人物都带骑宠血量，血量随分摊逐回合下降；没骑乘的用例没有 `mount`。

#[allow(dead_code)]
#[path = "../src/bin/mock_server/script.rs"]
mod script;

use std::path::Path;

use stoneage_client::proto::{BattleRound, BattleStart};

/// 按用例顺序取出 `player` 连接收到的 s2c 消息的 `data`，并按 mock 服务端的做法物化通配。
fn s2c_data(case: &str, ty: &str) -> Vec<serde_json::Value> {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../docs/protocol/cases");
    let plan = script::load(&root.join(format!("{case}.jsonl"))).unwrap();
    let mut vars = script::Vars::new();
    plan.steps
        .iter()
        .filter(|s| s.dir == script::Dir::S2c && s.ty() == ty)
        .map(|s| script::materialize(&s.msg, &mut vars)["data"].clone())
        .collect()
}

#[test]
fn battle_start_carries_the_mount_of_the_rider() {
    let start: BattleStart = serde_json::from_value(s2c_data("battle_ride", "battle_start").remove(0)).unwrap();
    let rider = start.units.iter().find(|u| u.no == 0).expect("人物单位");
    assert!(rider.ride);
    let mount = rider.mount.as_ref().expect("骑乘的人物带 mount");
    assert_eq!(
        (mount.name.as_str(), mount.hp, mount.max_hp, mount.hp_pct),
        ("金格萨贝鲁", Some(40), Some(40), 100)
    );
}

#[test]
fn round_state_refreshes_the_mount_hp_each_round() {
    let hps: Vec<Option<u32>> = s2c_data("battle_ride", "battle_round")
        .into_iter()
        .map(|d| {
            let round: BattleRound = serde_json::from_value(d).unwrap();
            let rider = round.state.iter().find(|s| s.no == 0).expect("人物的状态");
            rider.mount.as_ref().and_then(|m| m.hp)
        })
        .collect();
    // 开局 40：battle_ride 用例（STAGE-291 的怪物计划改了随机走向）两回合骑宠都没被打到，血量保持 40；字段每回合都带。
    assert_eq!(hps, vec![Some(40), Some(40)], "每回合 state 里的人物都带骑宠血量");
}

#[test]
fn plain_battle_has_no_mount() {
    let start: BattleStart = serde_json::from_value(s2c_data("battle_win", "battle_start").remove(0)).unwrap();
    assert!(start.units.iter().all(|u| u.mount.is_none()), "没骑乘的单位不带 mount");
}
