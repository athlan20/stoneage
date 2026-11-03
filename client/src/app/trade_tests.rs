//! STAGE-295：断线和控制权变化时交易窗口必须清掉（`docs/trade.md` 5.2）。

use std::time::Instant;

use serde_json::{json, Value};

use super::party_requests::new_request;
use super::pending::Pending;
use super::trade::TradeOp;
use super::trade_state::TradeWindow;
use super::{App, Options, WorldState};
use crate::proto::{PartyRequest, ProtoError, Trade, WorldEntry};

fn app_with_trade() -> App {
    let mut a = App::new(Options::default());
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    w.trade = Some(TradeWindow::new(Trade {
        id: "t-1".into(),
        rev: 2,
        ..Default::default()
    }));
    a.world = Some(w);
    a
}

#[test]
fn going_offline_clears_the_trade_window_and_says_so() {
    let mut a = app_with_trade();
    a.offline("连接断开", false);
    let w = a.world.as_ref().unwrap();
    assert!(w.trade.is_none(), "断线后交易状态为空");
    assert!(a.log.all().iter().any(|m| m.text == "连接断开，交易已取消"));
}

#[test]
fn agent_taking_control_cancels_an_open_trade() {
    let mut a = app_with_trade();
    a.on_control_changed(json!({"you": "observer", "controller": "agent", "reason": "AGENT_ENTERED"}));
    assert!(a.world.as_ref().unwrap().trade.is_none());
    assert!(a.log.all().iter().any(|m| m.text == "交易因控制权变化取消"));
}

#[test]
fn trade_action_is_greyed_with_a_reason_only_when_known_locally() {
    use super::trade_rules::trade_reason;
    use crate::proto::NearbyPlayer;
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.character.name = "勇者小明".into();
    w.entry.status.hp = 10;
    let other = NearbyPlayer { name: "路人乙".into(), level: 12, ..Default::default() };
    assert_eq!(trade_reason(&w, &other), None, "可用");
    w.entry.status.hp = 0;
    assert_eq!(trade_reason(&w, &other), Some("角色已经倒下，不能交易"));
    w.entry.status.hp = 10;
    w.trade = Some(TradeWindow::new(Trade::default()));
    assert_eq!(trade_reason(&w, &other), Some("交易中，先完成或取消交易"));
    w.trade = None;
    let busy = NearbyPlayer { in_battle: true, ..other.clone() };
    assert_eq!(trade_reason(&w, &busy), Some("对方在战斗中，不能交易"));
    let me = NearbyPlayer { name: "勇者小明".into(), ..other };
    assert_eq!(trade_reason(&w, &me), Some("不能和自己交易"));
}

/// STAGE-295：对方关了交易开关，邀请的失败文案是「对方不接收交易邀请」，不套组队的「关闭了组队邀请」。
#[test]
fn target_refuses_on_a_trade_invite_speaks_of_trade() {
    let mut a = app_with_trade();
    let err = ProtoError {
        code: "TARGET_REFUSES".into(),
        message: "对方不接收交易邀请".into(),
        data: Value::Null,
    };
    a.on_error(Pending::Trade(TradeOp::Invite { name: "路人乙".into() }), err);
    let msgs: Vec<&str> = a.log.all().iter().map(|m| m.text.as_str()).collect();
    assert!(msgs.contains(&"对方不接收交易邀请"), "{msgs:?}");
    assert!(!msgs.iter().any(|m| m.contains("组队")), "{msgs:?}");
}

/// STAGE-295：取消方收到的 trade_ended 没有 code，by 是自己：消息栏只有「你取消了交易」，响应到了也不再多一句。
#[test]
fn canceller_sees_only_you_cancelled_the_trade() {
    let mut a = app_with_trade();
    a.world.as_mut().unwrap().entry.character.name = "勇者小明".into();
    a.on_trade_ended(json!({"id": "t-1", "status": "cancelled", "by": "勇者小明", "code": null, "summary": null}));
    a.on_trade_ok(TradeOp::Cancel { id: "t-1".into() }, json!({}));
    let count = |s: &str| a.log.all().iter().filter(|m| m.text == s).count();
    assert_eq!(count("你取消了交易"), 1);
    assert_eq!(count("已撤回交易邀请"), 0);
    assert_eq!(count("交易已取消"), 0);
}

/// STAGE-295：响应先到（窗口还开着）时，窗口先关掉，随后的 trade_ended 写一次「你取消了交易」。
#[test]
fn cancel_response_before_push_still_logs_the_cancel_once() {
    let mut a = app_with_trade();
    a.world.as_mut().unwrap().entry.character.name = "勇者小明".into();
    a.on_trade_ok(TradeOp::Cancel { id: "t-1".into() }, json!({}));
    assert!(a.world.as_ref().unwrap().trade.is_none());
    a.on_trade_ended(json!({"id": "t-1", "status": "cancelled", "by": "勇者小明", "code": null, "summary": null}));
    let count = |s: &str| a.log.all().iter().filter(|m| m.text == s).count();
    assert_eq!(count("你取消了交易"), 1);
    assert_eq!(count("已撤回交易邀请"), 0);
}

/// STAGE-295：真撤回的邀请（请求栏里确有这条）才说「已撤回交易邀请」，并从栏里移走。
#[test]
fn cancel_response_says_withdrawn_only_when_the_invite_was_in_the_list() {
    let mut a = App::new(Options::default());
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    let req = PartyRequest {
        id: "r-1".into(),
        kind: "trade".into(),
        from: "勇者小明".into(),
        to: "路人乙".into(),
        party: None,
        expires_in: 30,
    };
    w.party_requests.outgoing.push(new_request(req, Instant::now()));
    a.world = Some(w);
    a.on_trade_ok(TradeOp::Cancel { id: "r-1".into() }, json!({}));
    assert!(a.log.all().iter().any(|m| m.text == "已撤回交易邀请"));
    assert!(a.world.as_ref().unwrap().party_requests.outgoing.is_empty());
}

/// STAGE-295：背包推送（成交也走它）带的石币写进角色的状态，角色页读的就是这里。
#[test]
fn inventory_push_refreshes_gold_for_the_character_page() {
    let mut a = app_with_trade();
    a.world.as_mut().unwrap().entry.status.gold = 1000;
    a.on_inventory(json!({"gold": 900, "slots": []}));
    assert_eq!(a.world.as_ref().unwrap().entry.status.gold, 900);
}
