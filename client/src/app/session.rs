//! 协议流程：连接、握手、登录、注册、角色请求与推送的处理。

use std::collections::HashMap;
use std::time::{Duration, Instant};

use serde_json::{Value, json};

use super::{App, CharsState, CreateState, Screen};
use crate::net::compress;
use crate::net::{Link, Net, NetEvent};
use crate::proto::{
    CLIENT_NAME, Disconnect, HelloInfo, Incoming, Notice, PROTOCOL_VERSION,
};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConnState {
    Idle,
    Connecting,
    Handshaking,
    Ready,
}

pub use super::pending::{Action, Pending};

pub struct Session {
    pub state: ConnState,
    pub hello: Option<HelloInfo>,
    pub addr: String,
    net: Net,
    link: Option<Link>,
    pending: HashMap<u64, Pending>,
    pub(super) queued: Option<Action>,
    last_sent: Instant,
}

impl Session {
    pub(super) fn has_uncertain(&self)->bool{self.pending.values().any(Pending::uncertain)}
    pub(super) fn token_busy(&self)->bool{self.pending.values().any(|p|matches!(p,Pending::TokenCreate|Pending::TokenRevoke))}
    pub(super) fn chat_busy(&self) -> bool {
        self.pending.values().any(|p| matches!(p, Pending::Chat { .. }))
    }
    pub(super) fn say_busy(&self)->bool{self.pending.values().any(|p|matches!(p,Pending::AgentSay(_)))}
    pub(super) fn scene_operation_busy(&self)->bool {
        self.pending.values().any(|p|matches!(p,Pending::UseExit|Pending::Warp(_)|Pending::NpcWarp{..}|Pending::Talk))
    }
    /// 「1级宠物」开战或传送在途（STAGE-251）：在途时不再发第二条。
    pub(super) fn pet_start_busy(&self) -> bool {
        self.pending.values().any(|p| matches!(p, Pending::PetStart { .. }))
    }
    pub(super) fn leveling_start_busy(&self) -> bool {
        self.pending.values().any(|p| matches!(p, Pending::LevelingStart { .. }))
    }
    pub(super) fn npc_open_busy(&self) -> bool {
        self.pending.values().any(|p| matches!(p, Pending::Talk))
    }
    /// 遇敌开关/开战在途（STAGE-153）：停止遇敌时要等它们收敛到关闭。
    pub(crate) fn encounter_busy(&self) -> bool {
        self.pending.values().any(|p| matches!(p, Pending::Encounter { set: Some(_) } | Pending::EncounterStart))
    }
    pub(super) fn npc_busy(&self) -> bool {
        self.pending.values().any(|p| {
            matches!(
                p,
                Pending::Talk | Pending::NpcChoose | Pending::NpcReply | Pending::NpcClose
            )
        })
    }
    /// 系统页的管理操作是否在途（令牌、私聊、控制权）。遇敌开关不在此列：停止遇敌的重复按下
    /// 由主动停机状态机去重（STAGE-153），超时后还要能重试。
    pub(super) fn system_busy(&self) -> bool {
        self.pending.values().any(|p| matches!(p, Pending::TokenCreate | Pending::TokenRevoke | Pending::AgentSay(_) | Pending::Take | Pending::Release | Pending::KickAgent))
    }
    pub fn new(addr: String) -> Self {
        Self {
            state: ConnState::Idle,
            hello: None,
            addr,
            net: Net::new(),
            link: None,
            pending: HashMap::new(),
            queued: None,
            last_sent: Instant::now(),
        }
    }

    /// 是否有请求在途，或者正在连接、握手；此时表单不接受重复提交。
    pub fn busy(&self) -> bool {
        matches!(self.state, ConnState::Connecting | ConnState::Handshaking)
            || self.pending.values().any(|p| !matches!(p, Pending::Ping))
    }

    pub fn event_rx(&self) -> &std::sync::mpsc::Receiver<NetEvent> {
        &self.net.rx
    }
}

fn disconnect_text(d: &Disconnect) -> String {
    if !d.message.is_empty() {
        return d.message.clone();
    }
    match d.reason.as_str() {
        "KICKED_DUPLICATE_LOGIN" => "账号在别处登录，你已下线".into(),
        "IDLE_TIMEOUT" => "长时间没有响应，连接已断开".into(),
        "SERVER_SHUTDOWN" => "服务器维护中，连接已断开".into(),
        "ACCOUNT_DISABLED" => "账号已被禁用，连接已断开".into(),
        _ => format!("连接已被服务端断开（{}）", d.reason),
    }
}

impl App {
    /// 切换线路：地址变了就丢掉旧连接。
    pub(super) fn use_server(&mut self, addr: &str) {
        if self.session.addr != addr {
            self.reconnect=None;self.world=None;
            self.drop_link();
            self.session.addr = addr.to_string();
        }
    }

    pub(super) fn run_action(&mut self, action: Action) {
        if let Action::Login{account,..}=&action {if self.reconnect.as_ref().is_some_and(|r|r.account!=*account){self.reconnect=None;self.world=None;}}
        let addr = self.current_route().addr.clone();
        self.use_server(&addr);
        if self.session.state == ConnState::Ready {
            self.dispatch(action);
            return;
        }
        self.session.queued = Some(action);
        if self.session.state == ConnState::Idle {
            self.session.state = ConnState::Connecting;
            self.log.system(format!("正在连接服务端 {addr}…"));
            self.session.net.connect(&addr);
        }
    }

    pub(super) fn dispatch(&mut self, action: Action) {
        match action {
            Action::Probe => {}
            Action::Login { account, password } => {
                self.log.system("正在登录…");
                self.send(
                    "login",
                    json!({"account": account, "password": password}),
                    Pending::Login { account },
                );
            }
            Action::Register { account, password } => {
                self.log.system("正在注册…");
                self.send(
                    "register",
                    json!({"account": account, "password": password}),
                    Pending::Register { account },
                );
            }
        }
    }

    pub(super) fn send(&mut self, ty: &str, data: Value, pending: Pending) -> bool {
        if !self.allow_send(ty,&pending){return false;}
        if self.world.as_ref().is_some_and(|w| w.observing()) && super::system::game_mutation(ty, &data) {
            self.log.error("agent 操控中，请先收回控制权"); return false;
        }
        let Some(link) = self.session.link.as_mut() else {
            self.handle_closed();
            return false;
        };
        match link.send(ty, &data) {
            Ok(id) => {
                self.session.pending.insert(id, pending);
                self.session.last_sent = Instant::now();
                true
            }
            Err(_) => {
                self.handle_closed();
                false
            }
        }
    }

    /// 关闭连接并清空与连接相关的状态；旧连接上还没到的事件会被作废。
    pub(super) fn drop_link(&mut self) {
        if let Some(link) = self.session.link.take() {
            link.close();
        }
        self.session.net.invalidate();
        self.session.state = ConnState::Idle;
        self.session.hello = None;
        self.session.pending.clear();
        self.session.queued = None;
    }

    /// 连接断了：在页面停留的阶段回到登录页并说明。
    fn handle_closed(&mut self) {
        let in_flight = self.session.busy();
        let was_active = self.session.state != ConnState::Idle;
        let unknown=self.session.pending.values().any(Pending::uncertain);
        if self.world.is_some(){self.offline("与服务端的连接已断开（离线只读，资料可能过期）",unknown);return;}
        self.drop_link();
        match self.screen {
            Screen::Login | Screen::Register => {
                if in_flight {
                    self.log.error("与服务端的连接已断开");
                }
            }
            _ if was_active => {
                self.back_to_login();
                self.log.error("与服务端的连接已断开");
            }
            _ => {}
        }
    }

    fn back_to_login(&mut self) {
        self.screen = Screen::Login;
        self.world = None;
        self.login.password.clear();
        self.chars = CharsState::default();
        self.create = CreateState::default();
    }

    pub fn session_tick(&mut self, now: Instant) {
        let s = &self.session;
        let interval = s.hello.as_ref().map_or(20, |h| h.heartbeat_seconds.max(1));
        if s.state == ConnState::Ready
            && now.duration_since(s.last_sent) >= Duration::from_secs(interval)
        {
            self.send("ping", json!({}), Pending::Ping);
        }
    }

    /// 取出并处理网络线程发来的所有事件。
    pub fn pump_net(&mut self) {
        while let Ok(ev) = self.session.net.rx.try_recv() {
            self.on_net(ev);
        }
    }

    pub fn on_net(&mut self, ev: NetEvent) {
        if ev.gen_id() != self.session.net.current_gen() {
            return;
        }
        match ev {
            NetEvent::Connected {
                gen_id,
                outbox,
                socket,
            } => {
                self.session.link = Some(self.session.net.link_from(outbox, socket, gen_id));
                self.session.state = ConnState::Handshaking;
                // STAGE-246：声明能接收 gzip 压缩，超过 10 KiB 的回复会以压缩行发来。
                let data = json!({
                    "protocol": PROTOCOL_VERSION,
                    "client": CLIENT_NAME,
                    "role": "player",
                    "compress": [compress::GZIP],
                });
                self.send("hello", data, Pending::Hello);
            }
            NetEvent::ConnectFailed { reason, .. } => {
                self.session.state = ConnState::Idle;
                self.session.queued = None;
                self.log.error(reason);
            }
            NetEvent::Message { msg, .. } => self.on_incoming(msg),
            NetEvent::Closed { .. } => self.handle_closed(),
        }
    }

    fn on_incoming(&mut self, msg: Incoming) {
        match msg {
            Incoming::Response { id, result, .. } => {
                let Some(pending) = self.session.pending.remove(&id) else {
                    return;
                };
                match result {
                    Ok(data) => self.on_ok(pending, data),
                    Err(err) => self.on_error(pending, err),
                }
            }
            Incoming::Push { ty, data } => match ty.as_str() {
                "disconnect" => {
                    let d: Disconnect = serde_json::from_value(data).unwrap_or_default();
                    self.handle_disconnect(&d);
                }
                "notice" => {
                    let n: Notice = serde_json::from_value(data).unwrap_or_default();
                    match n.level.as_str() {
                        "error" | "warning" => self.log.error(n.text),
                        _ => self.log.system(n.text),
                    }
                }
                "agent_presence" => { if let Some(w)=self.world.as_mut(){ w.entry.agent_online=data["online"].as_bool().unwrap_or(false); } },
                "control_changed" => self.on_control_changed(data),
                "position_changed" => self.on_position_changed(data),
                "npc_window" => self.on_npc_window(data),
                "npc_window_closed" => self.on_npc_window_closed(data),
                "npcs_changed" => self.on_npcs_changed(data),
                "players_changed" => self.on_players_changed(data),
                "chat_message" => self.on_chat_message(data),
                "inventory_changed" => self.on_inventory(data),
                "pets_changed" => self.on_pets(data),
                "party_changed" => self.on_party_changed_any(data),
                "social_changed" => self.on_social_changed(data),
                "party_request" => self.on_party_request(data),
                "party_request_result" => self.on_party_request_result(data),
                "trade_request" => self.on_trade_request(data),
                "trade_request_result" => self.on_trade_request_result(data),
                "trade_updated" => self.on_trade_updated(data),
                "trade_ended" => self.on_trade_ended(data),
                "quests_changed" => self.on_quests_changed(data),
                "status_changed" => self.on_status_changed(data),
                "agent_message" => self.on_agent_message(data),
                "battle_start" | "battle_turn" | "battle_progress" | "battle_round"
                | "battle_end" | "battle_leave" => self.on_battle_push(&ty, data),
                "encounter_changed" => self.on_encounter_changed(data),
                _ => {}
            },
        }
    }

    /// 服务端推送 `disconnect`：回登录页，说明原因。
    fn handle_disconnect(&mut self, d: &Disconnect) {
        let unknown=self.session.pending.values().any(Pending::uncertain);
        self.offline(&disconnect_text(d),unknown);
    }


}

/// 把秒数写成“15 分钟”“45 秒”这样的说法。
pub fn format_wait(seconds: u64) -> String {
    if seconds >= 3600 {
        format!("{} 小时 {} 分钟", seconds / 3600, seconds % 3600 / 60)
    } else if seconds >= 60 {
        format!("{} 分 {} 秒", seconds / 60, seconds % 60)
    } else {
        format!("{seconds} 秒")
    }
}

impl App {
    /// 取出开发快捷方式里指定的角色（只用一次）。
    pub(super) fn take_auto_character(&mut self) -> Option<String> {
        self.auto_character.take()
    }
}

/// 退出时主动关闭连接，让服务端立刻保存数据并释放登录状态。
impl Drop for Session {
    fn drop(&mut self) {
        if let Some(link) = self.link.take() {
            link.close();
        }
    }
}

#[cfg(test)]
impl App {
    /// 测试用：接一条不走网络的假连接，返回发出去的请求行（`Outgoing::Line`）的接收端。
    pub(crate) fn attach_test_link(&mut self) -> std::sync::mpsc::Receiver<crate::net::Outgoing> {
        let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind");
        let socket = std::net::TcpStream::connect(listener.local_addr().expect("addr")).expect("connect");
        let (tx, rx) = std::sync::mpsc::channel();
        self.session.link = Some(self.session.net.link_from(tx, socket, 0));
        rx
    }
}
