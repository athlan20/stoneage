//! STAGE-278 公开聊天（`docs/chat.md`、`docs/interaction.md`「聊天输入框」）：命令 `say`/`world` 与别名、
//! 附近玩家列的「聊天…」入口、输入框的 Tab/Enter/Esc 与草稿、`chat_message` 推送的前缀和颜色、
//! 三种错误的提示文字、80×24 绘制。不连服务端：请求的线上流程由 `docs/protocol/cases/chat_*.jsonl` 回放覆盖。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{Terminal, backend::TestBackend, style::Color};
use serde_json::json;
use stoneage_client::app::{App, Kind, Layer, Options, SceneColumn, Screen, WorldState, chat_error_text};
use stoneage_client::net::NetEvent;
use stoneage_client::proto::{ChatChannel, Incoming, NearbyPlayer, WorldEntry};
use unicode_width::UnicodeWidthStr;

/// 进世界、场景页默认层，附近玩家列加载完毕。
fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.entry.position.floor = 1000;
    w.entry.position.map_name = "萨姆吉尔村".into();
    w.entry.character.name = "勇者小明".into();
    w.scene.players.load = stoneage_client::app::LoadState::Ready;
    a.world = Some(w);
    a
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

fn top(a: &mut App) -> Layer {
    world(a).nav.top().clone()
}

/// 喂一条服务端推送（连接代数 0，同 keys_battle_observe）。
fn push(a: &mut App, ty: &str, data: serde_json::Value) {
    a.on_net(NetEvent::Message {
        gen_id: 0,
        msg: Incoming::Push { ty: ty.into(), data },
    });
}

fn draw(a: &mut App, width: u16, height: u16) -> String {
    let mut t = Terminal::new(TestBackend::new(width, height)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, a)).unwrap();
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

/// `/` 打开命令栏，输入命令和 Enter。
fn run_line(a: &mut App, line: &str) {
    key(a, KeyCode::Char('/'));
    type_str(a, line);
    key(a, KeyCode::Enter);
}

#[test]
fn say_and_world_without_text_open_the_box_on_their_channel() {
    for (line, channel) in [
        ("say", ChatChannel::Map),
        ("s", ChatChannel::Map),
        ("说", ChatChannel::Map),
        ("world", ChatChannel::World),
        ("w", ChatChannel::World),
        ("喊", ChatChannel::World),
    ] {
        let mut a = app();
        run_line(&mut a, line);
        assert_eq!(top(&mut a), Layer::Chat, "命令 {line} 应打开聊天框");
        assert_eq!(world(&mut a).chat.channel, channel, "命令 {line} 的频道");
        assert!(!world(&mut a).nav.contains(&Layer::Command), "命令栏应当收起");
    }
}

#[test]
fn players_chat_row_opens_the_box_even_when_list_is_empty() {
    let mut a = app();
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    assert_eq!(world(&mut a).scene.column, SceneColumn::Players);
    // 空列表时光标在 0 就是「聊天…」行。
    key(&mut a, KeyCode::Enter);
    assert_eq!(top(&mut a), Layer::Chat);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Map);
}

#[test]
fn players_chat_row_sits_below_the_players_and_is_reachable() {
    let mut a = app();
    world(&mut a).scene.players.replace(
        vec![NearbyPlayer {
            name: "甲".into(),
            level: 30,
            ..Default::default()
        }],
        false,
    );
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Down);
    assert_eq!(world(&mut a).scene.players.index, 1, "下移一格到「聊天…」");
    assert!(world(&mut a).scene.players.selected().is_none(), "聊天行不是玩家");
    key(&mut a, KeyCode::Enter);
    assert_eq!(top(&mut a), Layer::Chat);
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&mut a), Layer::Scene);
}

#[test]
fn tab_switches_channel_esc_keeps_the_draft() {
    let mut a = app();
    run_line(&mut a, "say");
    type_str(&mut a, "你好");
    key(&mut a, KeyCode::Tab);
    assert_eq!(world(&mut a).chat.channel, ChatChannel::World);
    key(&mut a, KeyCode::Esc);
    assert_eq!(top(&mut a), Layer::Scene, "Esc 只收起输入框");
    assert_eq!(world(&mut a).chat.input.text(), "你好", "草稿保留");
    run_line(&mut a, "say");
    assert_eq!(world(&mut a).chat.input.text(), "你好", "再次打开还在");
    assert_eq!(world(&mut a).chat.channel, ChatChannel::Map, "命令里的频道生效");
}

#[test]
fn enter_on_empty_draft_sends_nothing_and_box_stays_open() {
    let mut a = app();
    run_line(&mut a, "world");
    key(&mut a, KeyCode::Enter);
    assert_eq!(top(&mut a), Layer::Chat, "空文字不发，框保持打开");
    assert!(!a.chat_busy());
    // 字符进框，不走场景页的按键。
    type_str(&mut a, "ab");
    assert_eq!(world(&mut a).chat.input.text(), "ab");
}

#[test]
fn draft_stops_at_seventy_characters() {
    let mut a = app();
    run_line(&mut a, "say");
    type_str(&mut a, &"字".repeat(75));
    assert_eq!(world(&mut a).chat.input.text().chars().count(), 70);
}

#[test]
fn chat_message_push_is_logged_with_channel_prefix_and_color() {
    let mut a = app();
    push(
        &mut a,
        "chat_message",
        json!({"channel": "world", "from": "阿牛", "level": 30, "text": "有人去加加村吗", "time_ms": 1760000000000u64}),
    );
    let m = a.log.all().back().unwrap().clone();
    assert_eq!(m.kind, Kind::Chat(ChatChannel::World));
    assert_eq!(m.text, " 阿牛(Lv30): 有人去加加村吗");
    assert_eq!(Kind::Chat(ChatChannel::World).label(), "[世界]");
    assert_eq!(Kind::Chat(ChatChannel::World).color(), Color::LightBlue);
    assert_eq!(Kind::Chat(ChatChannel::Map).label(), "[本图]");
    assert_eq!(Kind::Chat(ChatChannel::Map).color(), Color::Reset, "本图是默认色");
    assert_eq!(
        Kind::Chat(ChatChannel::Map).label().width(),
        Kind::Command.label().width(),
        "定宽前缀"
    );

    // 自己说的话（推送里的角色名与自己一致）写「你」。
    push(
        &mut a,
        "chat_message",
        json!({"channel": "map", "from": "勇者小明", "level": 5, "text": "大家好", "time_ms": 1}),
    );
    let m = a.log.all().back().unwrap().clone();
    assert_eq!(m.kind, Kind::Chat(ChatChannel::Map));
    assert_eq!(m.text, " 你: 大家好");
}

#[test]
fn chat_error_hints_say_the_real_reason() {
    let mute = chat_error_text(
        ChatChannel::Map,
        "CHAT_MUTED",
        "",
        &json!({"until_ms": 1_000_000 + 125_000}),
        1_000_000,
    );
    assert!(mute.contains("禁言") && mute.contains("2 分 5 秒"), "{mute}");
    let lv = chat_error_text(ChatChannel::World, "CHAT_LEVEL_LOW", "", &json!({"min_level": 5}), 0);
    assert!(lv.contains("世界频道") && lv.contains("5 级"), "{lv}");
    let rl = chat_error_text(
        ChatChannel::World,
        "RATE_LIMITED",
        "",
        &json!({"retry_after_seconds": 12}),
        0,
    );
    assert!(rl.contains("世界频道") && rl.contains("12 秒"), "{rl}");
    let map = chat_error_text(
        ChatChannel::Map,
        "RATE_LIMITED",
        "",
        &json!({"retry_after_seconds": 1}),
        0,
    );
    assert!(map.contains("本图") && map.contains("1 秒"), "{map}");
}

#[test]
fn chat_box_and_players_row_fit_80_by_24() {
    let mut a = app();
    world(&mut a).scene.players.replace(
        vec![NearbyPlayer {
            name: "甲".into(),
            level: 30,
            ..Default::default()
        }],
        false,
    );
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let scene = draw(&mut a, 80, 24);
    assert!(scene.contains("聊天…"), "附近玩家列底部有「聊天…」\n{scene}");
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    type_str(&mut a, "有人去加加村吗");
    let boxed = draw(&mut a, 80, 24);
    assert!(boxed.contains("聊天") && boxed.contains("Tab 切换"), "{boxed}");
    assert!(boxed.contains("> 有人去加加村吗"), "{boxed}");
    assert!(boxed.contains("Enter 发送"), "{boxed}");
    assert!(boxed.contains("7/70"), "计数在 80×24 下可见\n{boxed}");
}
