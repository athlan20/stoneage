//! 应用状态与按键分发。各页面的按键处理在同名子模块里，协议流程在 `session`。

pub mod alert;
mod agent;
mod agent_guide;
pub mod agent_mcp;
pub mod auto_battle;
pub mod reconnect;
#[cfg(test)]
mod reconnect_tests;
mod responses;
pub mod system;
mod system_keys;
mod travel;
mod travel_keys;
pub mod pet_dir;
mod pet_dir_app;
pub mod leveling_dir;
mod leveling_dir_app;
mod bag_action;
mod bag_select;
pub mod battle;
pub mod char_select;
mod char_action;
mod char_magic;
mod characters;
pub mod commands;
mod confirm;
pub mod party_players;
pub mod party_requests;
pub mod trade;
pub mod trade_command;
pub mod trade_keys;
pub mod trade_rules;
pub mod trade_state;
pub mod trade_text;
#[cfg(test)]
mod trade_tests;
pub mod party_rules;
mod party_chat;
mod party_leader;
pub mod social_settings;
mod cursor;
mod create;
mod destination;
mod dialog;
mod encounter;
mod forms;
mod hint;
mod history;
mod input;
mod gold;
mod items;
pub mod inventory_inlay;
mod magic_use;
mod messages;
mod nav;
mod npc_visit;
mod observe;
mod page;
mod page_keys;
mod pending;
pub mod msgview;
pub mod pet_action;
pub mod pet_merge;
pub mod pet_ride;
mod pets;
mod pets_manage;
mod party;
mod party_action;
mod select;
mod points;
mod quest_guide;
mod quests;
pub mod scroll;
mod scene_action;
mod scene_state;
mod scene_net;
mod players_net;
mod players_state;
#[cfg(test)]
mod scene_net_tests;
mod scene_keys;
mod players_keys;
mod duel;
mod chat;
mod chat_keys;
pub use chat::{CHAT_MAX, ChatState, chat_error_text};
pub use players_keys::{PLAYER_ACTIONS, PlayerAction};
pub use scene_state::{EncounterChoice, EncounterPrompt, LoadState, SceneColumn, SceneQuery, SceneState};
mod session;
mod update_notice;
mod wiki;
mod world;
mod exit;
pub use exit::{ExitDialog, ExitKind};
mod browse;
pub use browse::{BrowseState, BrowseStep};
mod back_key;
mod world_keys;
mod world_nav;
mod world_command;
mod scene_catalog;

use std::sync::mpsc::Receiver;
use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyModifiers};
use semver::Version;

pub use agent::AgentState;
pub use bag_action::{Act, BagStep, UseTargets, actions, disabled_reason, use_targets};
pub use bag_select::{FIRST_BAG_SLOT, MAX_EQUIP_SLOT, can_equip};
pub use char_action::CharStep;
pub use characters::{CharsState, DeleteConfirm};
pub use create::{CreateFocus, CreateState, CreateStep};
pub use hint::key_hint;
pub use forms::{LoginFocus, LoginForm, RegisterFocus, RegisterForm};
pub use history::History;
pub use input::TextInput;
pub use messages::{Kind, Message, MessageLog};
pub use msgview::{MessageView, LABEL_WIDTH, wrap_text};
pub use pet_merge::{MergeKind, MergeReport, is_merge_skill};
pub use pet_action::{PET_ACTS, PetAct, PetStep, disabled_reason as pet_disabled_reason};
pub use pet_ride::RideContext;
pub use party::{PARTY_COLUMN, PartyOp, party_columns};
pub use party_action::{PartyAct, PartyStep, detail_rows as party_detail_rows, party_menu};
pub use scene_action::{CatalogScope, SceneStep};
pub use nav::{Layer, Nav};
pub use confirm::{Confirm, is_yes};
pub use session::{ConnState, Session};
pub use wiki::{Opened, WikiState};
pub use quest_guide::QuestGuide;
pub use quests::QuestFilter;
pub use select::page_cursor;
pub use travel::{Entry as TravelEntry, RowView as TravelRow, TravelAction, TravelNav, TravelState, TravelStatus};
pub use world::{AUTO_MENU, BAG_LAST_SLOT, BAG_MENU, CHAR_MENU, EQUIP_PARTS, GAME_MENU, MAP_MENU, MENU_TITLES, MESSAGES_MENU, PETS_MENU, QUEST_MENU, SIDE_BY_SIDE, SOCIAL_MENU, SYSTEM_MENU, WIKI_MENU, WorldState};

/// 默认服务端地址。发布版在编译时由环境变量 `STONEAGE_DEFAULT_SERVER` 指定线上地址（STAGE-98，见 docs/client.md「发布」）；
/// 没设置时是本机开发用的 127.0.0.1:7065。
pub const DEFAULT_SERVER: &str = match option_env!("STONEAGE_DEFAULT_SERVER") {
    Some(addr) => addr,
    None => "127.0.0.1:7065",
};

/// 命令行参数换算出来的启动选项。
#[derive(Debug, Clone, Default)]
pub struct Options {
    pub server: String,
    pub account: Option<String>,
    /// 来自环境变量 `STONEAGE_PASSWORD`，只在内存里，不落盘。
    pub password: Option<String>,
    pub character: Option<String>,
    pub unicode: bool,
    /// 启动后台查一次新版本（STAGE-99）；测试和离线场景保持关闭。
    pub update_check: bool,
    /// 减少动态效果（STAGE-167）：不画卡片位移和飞行物，只留身份、方向与接触结果。
    pub reduce_motion: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Screen {
    Login,
    Register,
    Characters,
    Create,
    World,
}

/// 线路列表项。一期只有一条线路，名字由地址决定（见 `route_name`），以后由配置文件提供。
#[derive(Debug, Clone)]
pub struct Route {
    pub name: String,
    pub addr: String,
}

/// 线路名：回环地址叫「本地」，其他地址叫「线上」（STAGE-98：发布版默认连线上服务端）。
/// STAGE-101 起地址可以是 `ws://`、`wss://`，所以先去掉 scheme 和路径，再看主机名。
fn route_name(addr: &str) -> &'static str {
    match route_host(addr).as_str() {
        "127.0.0.1" | "localhost" | "::1" => "本地",
        _ => "线上",
    }
}

/// 从 `host:port`、`ws://host:port/ws`、`wss://[::1]:443/ws` 里取出主机名（转小写）。
fn route_host(addr: &str) -> String {
    let rest = addr.split_once("://").map_or(addr, |(_scheme, rest)| rest);
    let rest = rest.split(['/', '?', '#']).next().unwrap_or("");
    let host = match rest.find(']') {
        // [::1]:7067 → ::1
        Some(end) => rest.get(1..end).unwrap_or(""),
        None => rest.rsplit_once(':').map_or(rest, |(host, _port)| host),
    };
    host.to_ascii_lowercase()
}

pub struct App {
    pub screen: Screen,
    pub routes: Vec<Route>,
    pub login: LoginForm,
    pub register: RegisterForm,
    pub chars: CharsState,
    pub create: CreateState,
    pub world: Option<WorldState>,
    pub log: MessageLog,
    pub session: Session,
    pub unicode: bool,
    /// 减少动态效果（STAGE-167）：战斗回放里不画卡片位移和飞行物。
    pub reduce_motion: bool,
    pub default_battle_speed: u8,
    /// 结算回放直接跳过（STAGE-173）：选「跳过」时下完指令就出结果、进下一回合，不播动画。
    pub skip_replay: bool,
    /// 战斗日志的档位（`battle-log` 命令）：自动时观察模式简略、自己操控完整。
    pub battle_log: battle::brief::LogMode,
    /// 结果窗口是否自动关闭（STAGE-173 M16）：跟着当前方案走，默认开。
    pub auto_continue: bool,
    /// 当前服务器与角色的自动战斗方案（STAGE-173 M15）：进世界时按角色打开，退出世界后保留。
    pub auto_store: Option<auto_battle::store::Store>,
    pub should_quit: bool,
    pub tick: u64,
    auto_character: Option<String>,
    pub exit_dialog: Option<ExitDialog>,
    pub reconnect: Option<reconnect::Reconnect>,
    /// 后台版本检查的结果通道，收到新版本后置空。
    update_rx: Option<Receiver<Version>>,
    /// 本次运行是否已经在消息栏提示过新版本（后台检查和握手响应共用，只提示一次）。
    update_announced: bool,
    /// 等着写进系统剪贴板的文本（STAGE-171）：主循环取走并交给 `clipboard::copy`。
    clipboard_out: Option<String>,
}

impl App {
    pub fn new(opts: Options) -> Self {
        let addr = if opts.server.is_empty() { DEFAULT_SERVER.to_string() } else { opts.server.clone() };
        let routes = vec![Route { name: route_name(&addr).into(), addr: addr.clone() }];
        let mut app = Self {
            screen: Screen::Login,
            routes,
            login: LoginForm::default(),
            register: RegisterForm::default(),
            chars: CharsState::default(),
            create: CreateState::default(),
            world: None,
            log: MessageLog::default(),
            session: Session::new(addr),
            unicode: opts.unicode,
            reduce_motion: opts.reduce_motion,
            default_battle_speed: 1,
            skip_replay: false,
            battle_log: Default::default(),
            auto_continue: true,
            auto_store: None,
            should_quit: false,
            tick: 0,
            auto_character: opts.character.clone(),
            exit_dialog: None,
            reconnect: None,
            update_rx: opts.update_check.then(crate::update::spawn_check),
            update_announced: false,
            clipboard_out: None,
        };
        if let Some(account) = &opts.account {
            app.login.account.set(account);
        }
        if let Some(password) = &opts.password {
            app.login.password.set(password);
        }
        if opts.account.is_some() && opts.password.is_some() {
            app.submit_login();
        }
        app
    }

    /// 结算回放的档位标签（系统页显示）：1× / 2× / 跳过。
    pub fn playback_label(&self) -> &'static str {
        if self.skip_replay {
            "跳过"
        } else if self.default_battle_speed == 2 {
            "2×"
        } else {
            "1×"
        }
    }

    pub fn current_route(&self) -> &Route {
        &self.routes[self.login.route.min(self.routes.len() - 1)]
    }

    pub fn on_key(&mut self, key: KeyEvent) {
        if key.kind == KeyEventKind::Release {
            return;
        }
        if key.kind == KeyEventKind::Repeat && key.code == KeyCode::Enter { return; }
        let hold = self.auto_hold();
        if let Some(b)=self.world.as_mut().and_then(|w|w.battle.as_mut()){b.defer_auto(Instant::now(), hold);}
        if self.exit_dialog.is_some() { self.exit_key(key); return; }
        if key.modifiers.contains(KeyModifiers::CONTROL) && key.code == KeyCode::Char('c') {
            self.on_ctrl_c();
            return;
        }
        match self.screen {
            Screen::Login => self.login_key(key),
            Screen::Register => self.register_key(key),
            Screen::Characters => self.characters_key(key),
            Screen::Create => self.create_key(key),
            Screen::World => self.world_key(key),
        }
    }

    /// 所有屏幕都打开默认取消的可见退出确认。
    fn on_ctrl_c(&mut self) {
        self.request_exit(ExitKind::Quit);
    }

    pub fn on_tick(&mut self, now: Instant) {
        self.tick += 1;
        self.session_tick(now);
        self.update_tick();
        self.scene_tick(now);
        self.party_requests_tick(now);
        if self.world.as_ref().is_some_and(|w|w.online){self.battle_tick(now);self.auto_tick(now);}
        // 主动停机的看门狗（STAGE-153）：停止请求等太久就先查权威状态，不假设请求没执行。
        self.halt_tick(now);
    }

    /// 取走待复制的文本（主循环每圈调一次）。
    pub fn take_clipboard(&mut self) -> Option<String> {
        self.clipboard_out.take()
    }

    /// 刷新间隔：平时 250ms；战斗中缩到 33ms，让身体移动和结算回放流畅（`docs/battle-ui.md`）。
    pub fn tick_rate(&self) -> Duration {
        if self.world.as_ref().is_some_and(|w| w.battle.is_some()) { Duration::from_millis(33) } else { Duration::from_millis(250) }
    }
}

/// 在 `n` 项之间循环移动焦点。
pub(crate) fn cycle(i: usize, n: usize, forward: bool) -> usize {
    if forward { (i + 1) % n } else { (i + n - 1) % n }
}

#[cfg(test)]
mod tests {
    use super::route_name;

    #[test]
    fn route_name_is_local_only_for_loopback() {
        for addr in ["127.0.0.1:7065", "localhost:7065", "[::1]:7065"] {
            assert_eq!(route_name(addr), "本地", "{addr}");
        }
        for addr in ["game.example.com:7065", "10.0.0.8:7065", "[2001:db8::1]:7065"] {
            assert_eq!(route_name(addr), "线上", "{addr}");
        }
    }

    /// STAGE-101：WebSocket 地址（带 scheme 和路径）也要判得出本地/线上。
    #[test]
    fn route_name_understands_websocket_addresses() {
        for addr in [
            "ws://127.0.0.1:7067/ws",
            "ws://127.0.0.1",
            "wss://localhost/ws",
            "ws://[::1]:7067/ws",
            "ws://LOCALHOST:7067/ws",
        ] {
            assert_eq!(route_name(addr), "本地", "{addr}");
        }
        for addr in [
            "ws://game.example.com:7067/ws",
            "wss://game.example.com/ws",
            "wss://203.0.113.9/ws",
            "wss://[2001:db8::1]:443/ws",
        ] {
            assert_eq!(route_name(addr), "线上", "{addr}");
        }
    }
}
