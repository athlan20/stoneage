//! STAGE-274 真人组队的客户端界面与预检：附近玩家动作面板的启用与灰显原因、请求弹窗（默认拒绝、Esc 稍后、
//! 倒计时、过期自动关）、请求栏、真人队成员表、队员灰显、错误码提示、80×24。直接构造状态，不连服务端。

use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{Terminal, backend::TestBackend, style::{Color, Style}};
use serde_json::json;
use stoneage_client::app::party_requests::PopupFocus;
use stoneage_client::app::{
    App, LoadState, Options, SOCIAL_MENU, SceneColumn, Screen, WorldState, party_players, party_requests, party_rules,
};
use stoneage_client::app::Kind;
use stoneage_client::proto::{
    EncounterArea, MapExit, NearbyPlayer, Party, PartyMember, PartyRequest, PartyRequestResult, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    w.entry.character.name = "我".into();
    w.entry.status.hp = 10;
    w.entry.position.floor = 100;
    w.entry.position.map_name = "萨伊那斯".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    w.scene.area_load = LoadState::Ready;
    w.scene.area_query = 1;
    w.scene.players.load = LoadState::Ready;
    a.world = Some(w);
    a
}

fn world(a: &mut App) -> &mut WorldState {
    a.world.as_mut().unwrap()
}

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
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

fn nearby(name: &str, level: u32) -> NearbyPlayer {
    NearbyPlayer {
        name: name.into(),
        level,
        ..Default::default()
    }
}

fn request(id: &str, kind: &str, from: &str) -> PartyRequest {
    PartyRequest {
        id: id.into(),
        kind: kind.into(),
        from: from.into(),
        to: "我".into(),
        party: None,
        expires_in: 30,
    }
}

fn member(index: u8, name: &str, leader: bool, level: u32, in_battle: Option<bool>) -> PartyMember {
    PartyMember {
        index,
        name: name.into(),
        leader,
        level,
        in_battle,
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

/// 进社交页（数字键 6，输入行为空时直接切页）。
fn open_social(a: &mut App) {
    key(a, KeyCode::Char('/'));
    for c in "party".chars() {
        key(a, KeyCode::Char(c));
    }
    key(a, KeyCode::Enter);
    assert_eq!(world(a).menu, SOCIAL_MENU);
}

#[test]
fn invite_is_enabled_for_a_free_player_and_grey_with_a_reason_otherwise() {
    let mut a = app();
    let w = world(&mut a);
    let free = nearby("阿牛", 30);
    assert_eq!(party_rules::invite_reason(w, &free), None);
    assert_eq!(
        party_rules::apply_reason(w, &free),
        Some("对方还没有队伍，想组队就用邀请")
    );
    assert_eq!(party_rules::kick_reason(w, "阿牛"), Some("你没有队伍"));

    let busy = NearbyPlayer {
        in_battle: true,
        ..nearby("小红", 12)
    };
    assert_eq!(party_rules::invite_reason(w, &busy), Some("对方在战斗中，不能邀请"));

    let member_of_other = NearbyPlayer {
        party: Some(json!({"leader": "乙", "size": 2, "max": 5})),
        ..nearby("丙", 9)
    };
    assert_eq!(
        party_rules::invite_reason(w, &member_of_other),
        Some("对方已经有队伍了")
    );
    assert_eq!(
        party_rules::apply_reason(w, &member_of_other),
        Some("对方不是队长，只能申请加入队长的队伍")
    );
    let leader = NearbyPlayer {
        party: Some(json!({"leader": "乙", "size": 2, "max": 5})),
        ..nearby("乙", 9)
    };
    assert_eq!(party_rules::apply_reason(w, &leader), None);
}

#[test]
fn action_panel_greys_invite_with_its_reason_on_screen() {
    let mut a = app();
    let mut busy = nearby("乙", 9);
    busy.party = Some(json!({"leader": "乙", "size": 2, "max": 5}));
    world(&mut a).scene.players.replace(vec![busy], false);
    world(&mut a).scene.column = SceneColumn::Players;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("邀请组队 · 对方已经有队伍了"), "{screen}");
    assert!(!screen.contains("组队功能尚未开放"), "{screen}");
}

#[test]
fn popup_defaults_to_reject_and_esc_keeps_the_request_in_the_list() {
    let mut a = app();
    world(&mut a).party_requests.incoming.push(party_requests::new_request(
        request("r-1", "invite", "阿牛"),
        Instant::now(),
    ));
    assert_eq!(world(&mut a).party_requests.focus, PopupFocus::Reject);
    let shown = draw(&mut a, 80, 24);
    assert!(shown.contains("阿牛 邀请你加入队伍"), "{shown}");
    assert!(shown.contains("[拒绝]"), "{shown}");
    assert!(shown.contains("剩余 30 秒"), "{shown}");

    key(&mut a, KeyCode::Esc);
    let w = world(&mut a);
    assert!(w.party_requests.popup().is_none(), "稍后处理后不再自动弹");
    assert_eq!(w.party_requests.incoming.len(), 1, "请求留在请求栏");
    let after = draw(&mut a, 80, 24);
    assert!(!after.contains("邀请你加入队伍"), "{after}");
}

#[test]
fn expired_request_closes_the_popup_and_leaves_the_list() {
    let mut a = app();
    world(&mut a).party_requests.incoming.push(party_requests::new_request(
        request("r-2", "apply", "小红"),
        Instant::now(),
    ));
    assert!(draw(&mut a, 80, 24).contains("申请加入你的队伍"));
    a.party_requests_tick(Instant::now() + Duration::from_secs(31));
    let w = world(&mut a);
    assert!(w.party_requests.incoming.is_empty());
    assert!(!draw(&mut a, 80, 24).contains("小红 申请加入"));
}

#[test]
fn result_messages_match_the_protocol_wording() {
    let expired = PartyRequestResult {
        id: "r-3".into(),
        kind: "invite".into(),
        status: "expired".into(),
        from: "阿牛".into(),
        to: "我".into(),
        ..Default::default()
    };
    assert_eq!(
        party_requests::result_message(&expired, true).as_deref(),
        Some("阿牛的组队邀请已过期")
    );
    let declined = PartyRequestResult {
        status: "declined".into(),
        to: "阿牛".into(),
        from: "我".into(),
        ..expired.clone()
    };
    assert_eq!(
        party_requests::result_message(&declined, false).as_deref(),
        Some("阿牛拒绝了你的组队邀请")
    );
    assert_eq!(party_requests::result_message(&declined, true), None);
}

#[test]
fn request_column_lists_incoming_and_outgoing_with_countdown_and_actions() {
    let mut a = app();
    let w = world(&mut a);
    w.party_requests.incoming.push(party_requests::new_request(
        request("r-4", "invite", "阿牛"),
        Instant::now(),
    ));
    w.party_requests.outgoing.push(party_requests::new_request(
        PartyRequest {
            to: "小红".into(),
            ..request("r-5", "apply", "我")
        },
        Instant::now(),
    ));
    w.party_requests.snoozed.insert("r-4".into());
    open_social(&mut a);
    key(&mut a, KeyCode::Right);
    assert!(party_players::on_request_column(world(&mut a)));
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("请求 · 收到 1 · 发出 1"), "{screen}");
    assert!(screen.contains("Enter 打开弹窗"), "{screen}");
    assert!(screen.contains("Enter 撤回"), "{screen}");
    assert!(screen.contains("阿牛") && screen.contains("小红"), "{screen}");
}

#[test]
fn empty_request_column_says_so() {
    let mut a = app();
    open_social(&mut a);
    key(&mut a, KeyCode::Right);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("没有待处理的组队请求"), "{screen}");
}

#[test]
fn real_team_table_marks_leader_level_and_battle_state() {
    let mut a = app();
    let members = vec![
        member(0, "我", true, 30, None),
        member(1, "乙", false, 12, Some(true)),
        member(2, "丙", false, 8, Some(false)),
    ];
    world(&mut a).entry.party = Some(real_party("我", members));
    open_social(&mut a);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("队伍 · 3/5 真人"), "{screen}");
    assert!(screen.contains("[队长]") && screen.contains("[队员]"), "{screen}");
    assert!(screen.contains("战斗中"), "{screen}");
    assert!(!screen.contains("[分身]"), "{screen}");
}

#[test]
fn follower_is_grey_and_the_leader_is_not() {
    let mut a = app();
    let w = world(&mut a);
    w.entry.party = Some(real_party(
        "乙",
        vec![member(0, "乙", true, 9, None), member(1, "我", false, 30, None)],
    ));
    assert_eq!(party_rules::follower_reason(w), Some(party_rules::FOLLOWER_REASON));
    w.entry.party = Some(real_party(
        "我",
        vec![member(0, "我", true, 30, None), member(1, "乙", false, 9, None)],
    ));
    assert_eq!(party_rules::follower_reason(w), None);
}

#[test]
fn follower_gets_a_reason_when_entering_a_map_exit_is_refused_locally() {
    let mut a = app();
    world(&mut a).entry.party = Some(real_party(
        "乙",
        vec![member(0, "乙", true, 9, None), member(1, "我", false, 30, None)],
    ));
    key(&mut a, KeyCode::Enter);
    // 队员按 Enter 不发请求、不切换场景：界面仍在场景页上。
    assert_eq!(world(&mut a).scene.column, SceneColumn::Exit);
}

#[test]
fn every_party_error_code_has_a_chinese_hint() {
    let codes = [
        "PLAYER_NOT_FOUND",
        "PLAYER_NOT_NEARBY",
        "TARGET_IN_BATTLE",
        "TARGET_IN_PARTY",
        "TARGET_NOT_LEADER",
        "TARGET_BUSY",
        "TARGET_REFUSES",
        "PARTY_FULL",
        "NOT_PARTY_LEADER",
        "NOT_PARTY_MEMBER",
        "PARTY_IN_BATTLE",
        "PARTY_FOLLOWER",
        "REQUEST_NOT_FOUND",
        "REQUEST_PENDING",
    ];
    for code in codes {
        let hint = party_players::party_error_hint(code).unwrap_or_else(|| panic!("{code} 没有提示"));
        assert!(!hint.is_empty());
    }
    assert_eq!(party_players::party_error_hint("NO_SUCH_CODE"), None);
    assert!(party_players::party_error_hint("PARTY_FULL").unwrap().contains("5 人"));
}

#[test]
fn popup_and_request_column_fit_in_80_by_24() {
    let mut a = app();
    world(&mut a).party_requests.incoming.push(party_requests::new_request(
        request("r-6", "invite", "一个名字比较长的玩家"),
        Instant::now(),
    ));
    let screen = draw(&mut a, 80, 24);
    assert_eq!(screen.lines().count(), 24);
    assert!(screen.contains("[拒绝]"), "{screen}");
    for line in screen.lines() {
        assert!(UnicodeWidthStr::width(line) <= 80, "超宽：{line}");
    }
}

#[test]
fn party_leave_and_kick_refuse_locally_without_a_party() {
    let mut a = app();
    let w = world(&mut a);
    assert_eq!(party_rules::leave_reason(w), Some("你没有队伍"));
    assert_eq!(party_rules::kick_reason(w, "乙"), Some("你没有队伍"));
    assert_eq!(party_rules::invite_reason(w, &nearby("乙", 1)), None);
    let _ = SceneColumn::Players;
    let _ = LoadState::Ready;
    let _ = Duration::from_secs(1);
    let _ = PopupFocus::Accept;
}

/// 屏幕上第一次出现 `needle` 的那个字的样式（按显示宽度逐格对齐，和 `draw` 一致）。
fn style_of(a: &mut App, width: u16, height: u16, needle: &str) -> Style {
    let mut t = Terminal::new(TestBackend::new(width, height)).unwrap();
    t.draw(|f| ui::render(f, a)).unwrap();
    let b = t.backend().buffer().clone();
    for y in 0..height {
        let mut cells: Vec<(u16, String)> = Vec::new();
        let mut x = 0;
        while x < width {
            let v = b[(x, y)].symbol().to_string();
            cells.push((x, v.clone()));
            x += UnicodeWidthStr::width(v.as_str()).max(1) as u16;
        }
        let text: String = cells.iter().map(|(_, v)| v.as_str()).collect();
        let Some(byte) = text.find(needle) else { continue };
        let mut off = 0;
        for (x, v) in &cells {
            if off == byte {
                return b[(*x, y)].style();
            }
            off += v.len();
        }
    }
    panic!("屏幕上没有「{needle}」：\n{}", draw(a, width, height));
}

/// 场景页放一个出口和一个遇敌区域（编号 7），焦点停在出口列。
fn scene_with_exit_and_area(a: &mut App) {
    let w = world(a);
    w.scene.exits = vec![MapExit {
        id: "e1".into(),
        to_name: "西门".into(),
        time_gate: "all".into(),
        available: true,
        ..Default::default()
    }];
    w.scene.areas = vec![EncounterArea {
        id: "a7".into(),
        number: 7,
        available: true,
        max_enemies: 3,
        ..Default::default()
    }];
}

#[test]
fn follower_exit_column_and_fixed_entries_are_drawn_grey() {
    let mut a = app();
    scene_with_exit_and_area(&mut a);
    world(&mut a).entry.party = Some(real_party(
        "乙",
        vec![member(0, "乙", true, 9, None), member(1, "我", false, 30, None)],
    ));
    // 焦点在出口列第一行：高亮也保持灰字，不变成黄底黑字。
    assert_eq!(style_of(&mut a, 80, 24, "西门").fg, Some(Color::DarkGray));
    assert_eq!(style_of(&mut a, 80, 24, "快捷传送").fg, Some(Color::DarkGray));
    assert_eq!(style_of(&mut a, 80, 24, "回城").fg, Some(Color::DarkGray));
    // 「查找并直接对话」不受组队限制，保持青色。
    assert_eq!(style_of(&mut a, 80, 24, "查找并直接对话").fg, Some(Color::Cyan));
    // 底部说明写出灰显原因。
    assert!(draw(&mut a, 80, 24).contains("跟随队长，离队后才能自己行动"));
}

#[test]
fn follower_encounter_column_is_drawn_grey() {
    let mut a = app();
    scene_with_exit_and_area(&mut a);
    world(&mut a).entry.party = Some(real_party(
        "乙",
        vec![member(0, "乙", true, 9, None), member(1, "我", false, 30, None)],
    ));
    world(&mut a).scene.column = SceneColumn::Encounter;
    assert_eq!(style_of(&mut a, 80, 24, "#7 无敌人").fg, Some(Color::DarkGray));
    assert!(draw(&mut a, 80, 24).contains("跟随队长，离队后才能自己行动"));
}

#[test]
fn leader_and_free_player_exit_column_is_not_grey() {
    let mut a = app();
    scene_with_exit_and_area(&mut a);
    // 没有队伍：焦点行是黄底黑字，其余出口是默认色。
    assert_eq!(style_of(&mut a, 80, 24, "西门").fg, Some(Color::Black));
    assert_ne!(style_of(&mut a, 80, 24, "快捷传送").fg, Some(Color::DarkGray));
    assert!(!draw(&mut a, 80, 24).contains("跟随队长，离队后才能自己行动"));
}

#[test]
fn party_leave_command_echo_keeps_the_space() {
    let mut a = app();
    world(&mut a).entry.party = Some(real_party(
        "乙",
        vec![member(0, "乙", true, 9, None), member(1, "我", false, 30, None)],
    ));
    key(&mut a, KeyCode::Char('/'));
    for c in "party leave".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Enter);
    let echoes: Vec<String> = a
        .log
        .all()
        .iter()
        .filter(|m| m.kind == Kind::Command)
        .map(|m| m.text.clone())
        .collect();
    assert_eq!(echoes, vec!["party leave".to_string()]);
    for (w, h) in [(80, 24), (120, 30), (100, 40)] {
        let screen = draw(&mut a, w, h);
        assert!(screen.contains("命令  party leave"), "{w}x{h}\n{screen}");
    }
}

