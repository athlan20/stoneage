//! STAGE-277 转让队长与队伍聊天的客户端行为（`docs/ui.md` 队员动作与命令、`docs/chat.md` 1.5、`docs/protocol.md` 5.67/5.68）：
//! 动作菜单里「转让队长」的灰显原因与确认（默认取消）、`party leader` 命令、`leader_changed` 推送的文字、
//! `party say` / 队聊、`[队]` 前缀与颜色、Tab 在没有真人队时跳过队伍频道、错误码提示、观察模式、80×24。
//! 用假连接（本地回环套接字）截获发出的请求行，再用响应或推送喂回去；不连真正的服务端。

use std::net::{TcpListener, TcpStream};
use std::sync::mpsc::{Receiver, channel};

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{Terminal, backend::TestBackend, style::Color};
use serde_json::{Value, json};
use stoneage_client::app::party_players::party_error_hint;
use stoneage_client::app::{App, Kind, Layer, Options, PartyAct, SOCIAL_MENU, Screen, WorldState, party_menu};
use stoneage_client::net::{NetEvent, Outgoing};
use stoneage_client::proto::{ChatChannel, Incoming, Party, PartyMember, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

/// 假连接：`_server` 只为保持套接字存活，`rx` 收网络线程本应写出去的行。
struct Net {
    rx: Receiver<Outgoing>,
    _server: TcpStream,
}

fn app() -> (App, Net) {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    w.entry.character.name = "我".into();
    w.entry.status.hp = 10;
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    a.world = Some(w);
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let client = TcpStream::connect(listener.local_addr().unwrap()).unwrap();
    let (server, _) = listener.accept().unwrap();
    let (outbox, rx) = channel();
    a.on_net(NetEvent::Connected {
        gen_id: 0,
        outbox,
        socket: client,
    });
    (a, Net { rx, _server: server })
}

fn world(a: &mut App) -> &mut WorldState {
    a.world.as_mut().unwrap()
}

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

fn type_str(a: &mut App, s: &str) {
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

/// 打开命令栏输入一行。上一条命令报错或在等确认时命令栏还开着（报错的原文留在栏里），先按 Esc 收起。
fn run_line(a: &mut App, line: &str) {
    for _ in 0..3 {
        if world(a).nav.top() != &Layer::Command {
            break;
        }
        key(a, KeyCode::Esc);
    }
    key(a, KeyCode::Char('/'));
    type_str(a, line);
    key(a, KeyCode::Enter);
}

/// 本次之后发出的请求：（id，类型，数据）。
fn sent(net: &Net) -> Vec<(u64, String, Value)> {
    net.rx
        .try_iter()
        .filter_map(|o| match o {
            Outgoing::Line(s) => serde_json::from_str::<Value>(s.trim()).ok(),
            Outgoing::Close(_) => None,
        })
        .filter_map(|v| {
            Some((
                v.get("id")?.as_u64()?,
                v.get("type")?.as_str()?.to_string(),
                v.get("data")?.clone(),
            ))
        })
        .collect()
}

fn sent_of(net: &Net, ty: &str) -> Vec<(u64, Value)> {
    sent(net)
        .into_iter()
        .filter(|(_, t, _)| t == ty)
        .map(|(id, _, data)| (id, data))
        .collect()
}

/// 喂一条响应错误（`id` 取自发出的请求）。
fn fail(a: &mut App, id: u64, ty: &str, code: &str, message: &str, data: Value) {
    let err = serde_json::from_value(json!({"code": code, "message": message, "data": data})).unwrap();
    a.on_net(NetEvent::Message {
        gen_id: 0,
        msg: Incoming::Response {
            id,
            ty: ty.into(),
            result: Err(err),
        },
    });
}

/// 喂一条服务端推送。
fn push(a: &mut App, ty: &str, data: Value) {
    a.on_net(NetEvent::Message {
        gen_id: 0,
        msg: Incoming::Push { ty: ty.into(), data },
    });
}

fn draw(a: &mut App, width: u16, height: u16) -> String {
    let mut t = Terminal::new(TestBackend::new(width, height)).unwrap();
    t.draw(|f| ui::render(f, a)).unwrap();
    let b = t.backend().buffer();
    (0..height)
        .map(|y| {
            let mut s = String::new();
            let mut x = 0;
            while x < width {
                let v = b[(x, y)].symbol();
                s.push_str(v);
                x += UnicodeWidthStr::width(v).max(1) as u16;
            }
            s
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn member(index: u8, name: &str, leader: bool) -> PartyMember {
    PartyMember {
        index,
        name: name.into(),
        leader,
        level: 9,
        online: true,
        ..Default::default()
    }
}

fn real_party(leader: &str, members: Vec<PartyMember>) -> Party {
    Party {
        kind: "players".into(),
        leader: leader.into(),
        members,
    }
}

/// 我是队长，队里还有 乙、丙。
fn led_by_me(a: &mut App) {
    world(a).entry.party = Some(real_party(
        "我",
        vec![member(0, "我", true), member(1, "乙", false), member(2, "丙", false)],
    ));
}

/// 乙是队长，我是队员（丙也在队里）。
fn led_by_other(a: &mut App) {
    world(a).entry.party = Some(real_party(
        "乙",
        vec![member(0, "乙", true), member(1, "我", false), member(2, "丙", false)],
    ));
}

/// 进社交页并选中第 `index` 个成员（队长是 0），Enter 打开动作菜单。
fn open_member_actions(a: &mut App, index: u8) {
    run_line(a, "party");
    assert_eq!(world(a).menu, SOCIAL_MENU);
    for _ in 0..index {
        key(a, KeyCode::Down);
    }
    assert_eq!(world(a).selected_party_index(), Some(index));
    key(a, KeyCode::Enter);
    assert!(world(a).party_step.is_some(), "Enter 打开成员动作菜单");
}

fn last_error(a: &App) -> String {
    a.log
        .all()
        .iter()
        .rev()
        .find(|m| m.kind == Kind::Error)
        .map(|m| m.text.clone())
        .unwrap_or_default()
}

fn reason_of(rows: &[(PartyAct, Option<&'static str>)], act: PartyAct) -> Option<&'static str> {
    rows.iter().find(|(a, _)| *a == act).and_then(|(_, r)| *r)
}

#[test]
fn leader_action_menu_lists_transfer_and_greys_it_with_reason() {
    let (mut a, _net) = app();
    led_by_me(&mut a);
    let w = world(&mut a);
    // 我是队长：选中队员时有「请出队伍」和「转让队长」，转让能用。
    let rows = party_menu(w, 1);
    let labels: Vec<&str> = rows.iter().map(|(act, _)| act.label()).collect();
    assert_eq!(labels, ["查看详情", "请出队伍", "转让队长"]);
    assert_eq!(reason_of(&rows, PartyAct::Leader), None);
    // 选中自己（队长）：转让灰显，写明不能转给自己。
    let rows = party_menu(w, 0);
    assert_eq!(reason_of(&rows, PartyAct::Leader), Some("不能转给自己，选一位队员"));
    assert!(rows.iter().any(|(act, _)| *act == PartyAct::Disband));

    // 我是队员：转让灰显，写明只有队长能转。
    led_by_other(&mut a);
    let rows = party_menu(world(&mut a), 2);
    assert_eq!(reason_of(&rows, PartyAct::Leader), Some("只有队长能转让队长"));
    assert!(!rows.iter().any(|(act, _)| *act == PartyAct::Kick), "队员看不到请出");

    // 分身队：灰显，写明分身队没有可转让的队长。
    world(&mut a).entry.party = Some(Party {
        kind: "clone".into(),
        leader: "我".into(),
        members: vec![member(0, "我", true), member(1, "分身", false)],
    });
    let rows = party_menu(world(&mut a), 1);
    assert_eq!(
        reason_of(&rows, PartyAct::Leader),
        Some("分身队没有可转让的队长，要换队长请先解散")
    );

    // 界面上也能看到：打开菜单后 80×24 里有「转让队长」。
    led_by_me(&mut a);
    open_member_actions(&mut a, 1);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("转让队长"), "{screen}");
}

#[test]
fn transfer_asks_first_and_cancels_by_default() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    open_member_actions(&mut a, 1);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(world(&mut a).confirm.is_some(), "先进入确认");
    assert!(world(&mut a).party_step.is_none(), "确认时关掉动作菜单");
    let screen = draw(&mut a, 80, 24);
    assert!(
        screen.contains("把队长转让给 乙") && screen.contains("[取消]"),
        "{screen}"
    );
    assert!(sent_of(&net, "party_leader").is_empty(), "确认前不发请求");
    // 默认停在取消：直接 Enter 不转让。
    key(&mut a, KeyCode::Enter);
    assert!(world(&mut a).confirm.is_none());
    assert!(a.log.all().iter().any(|m| m.text == "已取消转让队长"));
    assert!(sent_of(&net, "party_leader").is_empty(), "取消不发请求");
}

#[test]
fn party_leader_command_sends_with_yes_and_refuses_bad_targets_locally() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    run_line(&mut a, "party leader 乙 yes");
    let reqs = sent_of(&net, "party_leader");
    assert_eq!(reqs.len(), 1, "带 yes 直接发");
    assert_eq!(reqs[0].1, json!({"name": "乙"}));

    // 本地能看出的原因直接写进消息栏，不发请求。
    run_line(&mut a, "party leader 我 yes");
    assert_eq!(last_error(&a), "不能转给自己，选一位队员");
    run_line(&mut a, "party leader 丁 yes");
    assert_eq!(last_error(&a), "对方不在你的队伍里");
    assert_eq!(sent_of(&net, "party_leader").len(), 0, "被拦的不发");
}

#[test]
fn party_leader_command_without_yes_asks_first_and_esc_cancels() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    run_line(&mut a, "party leader 乙");
    assert!(world(&mut a).confirm.is_some(), "命令不带 yes 先确认");
    assert!(sent_of(&net, "party_leader").is_empty());
    key(&mut a, KeyCode::Esc);
    assert!(world(&mut a).confirm.is_none(), "确认里按 Esc 取消");
    assert!(a.log.all().iter().any(|m| m.text == "已取消转让队长"));
    assert!(sent_of(&net, "party_leader").is_empty(), "取消不发请求");
}

#[test]
fn party_leader_error_codes_are_explained_not_shown_raw() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    run_line(&mut a, "party leader 乙 yes");
    let (id, _) = sent_of(&net, "party_leader")[0].clone();
    fail(&mut a, id, "party_leader", "NOT_PARTY_LEADER", "", json!({}));
    assert_eq!(last_error(&a), "只有队长能这样做");

    for (code, want) in [
        ("NOT_IN_PARTY", "你现在没有真人队伍"),
        ("NOT_PARTY_MEMBER", "对方不是你队伍里的队员"),
        ("INVALID_TARGET", "对象不能是自己（离队请用 party leave）"),
        ("PARTY_IN_BATTLE", "队伍正在战斗中，不能变更队伍"),
    ] {
        assert_eq!(party_error_hint(code), Some(want), "{code}");
    }
}

#[test]
fn leader_changed_push_updates_the_table_and_writes_the_right_line() {
    // 原队长（我）把队长转给乙：我的消息栏写「你把队长转让给了 乙」。
    let (mut a, _net) = app();
    led_by_me(&mut a);
    let swapped = |leader: &str| {
        json!({
            "reason": "leader_changed",
            "who": "乙",
            "party": {
                "kind": "players",
                "leader": leader,
                "members": [
                    {"index": 0, "name": "乙", "leader": true, "level": 9, "online": true},
                    {"index": 1, "name": "我", "leader": false, "level": 9, "online": true},
                    {"index": 2, "name": "丙", "leader": false, "level": 9, "online": true},
                ]
            }
        })
    };
    push(&mut a, "party_changed", swapped("乙"));
    let w = world(&mut a);
    assert_eq!(w.party().unwrap().leader, "乙");
    assert_eq!(w.party().unwrap().member(0).unwrap().name, "乙");
    assert!(w.party().unwrap().member(0).unwrap().leader);
    assert!(
        a.log
            .all()
            .iter()
            .any(|m| m.kind == Kind::Party && m.text == "你把队长转让给了 乙")
    );

    // 新队长（乙）那边：「你成了队长」。
    let (mut b, _net2) = app();
    led_by_other(&mut b);
    world(&mut b).entry.character.name = "乙".into();
    push(&mut b, "party_changed", swapped("乙"));
    assert!(
        b.log
            .all()
            .iter()
            .any(|m| m.kind == Kind::Party && m.text == "你成了队长")
    );

    // 丙那边：「队长换成了 乙」。
    let (mut c, _net3) = app();
    led_by_other(&mut c);
    world(&mut c).entry.character.name = "丙".into();
    push(&mut c, "party_changed", swapped("乙"));
    assert!(
        c.log
            .all()
            .iter()
            .any(|m| m.kind == Kind::Party && m.text == "队长换成了 乙")
    );
}

#[test]
fn party_say_sends_on_the_party_channel_with_both_spellings() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    run_line(&mut a, "party say 大家好 来打怪");
    run_line(&mut a, "队聊 你好");
    let chats = sent_of(&net, "chat");
    assert_eq!(chats.len(), 2);
    assert_eq!(chats[0].1, json!({"channel": "party", "text": "大家好 来打怪"}));
    assert_eq!(chats[1].1, json!({"channel": "party", "text": "你好"}));
    // 自己的话靠推送回显，发出时不在消息栏写。
    assert!(!a.log.all().iter().any(|m| matches!(m.kind, Kind::Chat(_))));
}

#[test]
fn party_say_without_text_opens_the_box_on_the_party_channel() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    run_line(&mut a, "party say");
    assert_eq!(world(&mut a).nav.top().clone(), Layer::Chat);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Party);
    assert!(sent_of(&net, "chat").is_empty());
}

#[test]
fn party_say_is_refused_without_a_real_party_and_server_errors_are_explained() {
    let (mut a, net) = app();
    run_line(&mut a, "party say 你好");
    assert_eq!(last_error(&a), "只有真人队里能用队伍频道");
    // 分身队也不能发队聊。
    world(&mut a).entry.party = Some(Party {
        kind: "clone".into(),
        leader: "我".into(),
        members: vec![member(0, "我", true)],
    });
    run_line(&mut a, "队聊 你好");
    assert_eq!(last_error(&a), "只有真人队里能用队伍频道");
    assert!(sent_of(&net, "chat").is_empty(), "被拦的不发");

    // 本地以为在真人队里，服务端回 NOT_IN_PARTY：按服务端的情况说明。
    led_by_me(&mut a);
    run_line(&mut a, "party say 你好");
    let (id, _) = sent_of(&net, "chat")[0].clone();
    fail(&mut a, id, "chat", "NOT_IN_PARTY", "", json!({}));
    assert_eq!(last_error(&a), "只有真人队里能用队伍频道（分身队和没有队伍都不行）");
    // 限流沿用本图的文案，频道名写成队伍。
    run_line(&mut a, "party say 再来");
    let (id, _) = sent_of(&net, "chat")[0].clone();
    fail(
        &mut a,
        id,
        "chat",
        "RATE_LIMITED",
        "",
        json!({"retry_after_seconds": 1}),
    );
    let rate = last_error(&a);
    assert!(rate.starts_with("队伍频道说话太快") && rate.contains("1 秒"), "{rate}");
}

#[test]
fn party_chat_push_is_logged_green_with_a_fixed_width_tag() {
    let (mut a, _net) = app();
    led_by_me(&mut a);
    push(
        &mut a,
        "chat_message",
        json!({"channel": "party", "from": "乙", "level": 9, "text": "集合", "time_ms": 1}),
    );
    let m = a.log.all().back().unwrap().clone();
    assert_eq!(m.kind, Kind::Chat(ChatChannel::Party));
    assert_eq!(m.text, " 乙(Lv9): 集合");
    assert_eq!(Kind::Chat(ChatChannel::Party).label(), "[队]  ");
    assert_eq!(
        Kind::Chat(ChatChannel::Party).label().width(),
        Kind::Command.label().width(),
        "定宽前缀"
    );
    assert_eq!(Kind::Chat(ChatChannel::Party).color(), Color::Green);

    push(
        &mut a,
        "chat_message",
        json!({"channel": "party", "from": "我", "level": 30, "text": "来了", "time_ms": 2}),
    );
    assert_eq!(a.log.all().back().unwrap().text, " 你: 来了");
}

#[test]
fn tab_skips_the_party_channel_outside_a_real_party() {
    let (mut a, _net) = app();
    run_line(&mut a, "say");
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::World);
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Map, "没有真人队时直接回本图");
    key(&mut a, KeyCode::Esc);

    // 进了真人队：本图 → 世界 → 队伍 → 本图。
    led_by_me(&mut a);
    run_line(&mut a, "say");
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Party);
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Map);
    key(&mut a, KeyCode::Esc);

    // 正选着队伍频道时队伍没了：Tab 跳回本图，不卡在队伍上。
    run_line(&mut a, "party say");
    world(&mut a).chat.channel = ChatChannel::Party;
    world(&mut a).entry.party = None;
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Map, "队伍没了，Tab 回本图");
}

#[test]
fn observer_can_chat_but_cannot_transfer_leadership() {
    let (mut a, net) = app();
    led_by_me(&mut a);
    world(&mut a).entry.control.you = "observer".into();
    run_line(&mut a, "party leader 乙 yes");
    assert!(sent_of(&net, "party_leader").is_empty(), "agent 操控中转让被拦");
    run_line(&mut a, "party say 大家好");
    assert_eq!(sent_of(&net, "chat").len(), 1, "聊天不受控制权限制");
}

#[test]
fn party_chat_and_leader_fit_80_by_24() {
    let (mut a, _net) = app();
    led_by_me(&mut a);
    run_line(&mut a, "party say");
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("队伍") && screen.contains("Tab 切换"), "{screen}");
    key(&mut a, KeyCode::Esc);
    open_member_actions(&mut a, 1);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("转让队长"), "{screen}");
}
