//! STAGE-6 联调：真实的客户端（玩家）+ 一个讲同一份协议的“agent”（测试里用原始 TCP 连接扮演）+ 真实的服务端。

use std::io::{BufRead, BufReader, Write};
use std::net::TcpStream;
use std::time::Duration;

use crossterm::event::KeyCode::*;
use serde_json::{Value, json};
use stoneage_client::app::{Kind, Screen};

use crate::PASSWORD;
use crate::driver::{Client, unique};
use crate::server::Server;

/// 最小的 agent 客户端：按协议收发 JSON Lines。
struct Agent {
    stream: TcpStream,
    reader: BufReader<TcpStream>,
    next: u64,
}

impl Agent {
    fn connect(addr: &str) -> Self {
        let stream = TcpStream::connect(addr).expect("agent 连不上服务端");
        stream.set_read_timeout(Some(Duration::from_secs(5))).unwrap();
        let reader = BufReader::new(stream.try_clone().unwrap());
        let mut a = Agent { stream, reader, next: 1 };
        let (hello, _) = a.call("hello", json!({"protocol": 1, "client": "e2e-agent", "role": "agent"}));
        assert_eq!(hello["ok"], true, "{hello}");
        a
    }

    fn send(&mut self, ty: &str, data: Value) -> u64 {
        let id = self.next;
        self.next += 1;
        writeln!(self.stream, "{}", json!({"id": id, "type": ty, "data": data})).unwrap();
        id
    }

    fn recv(&mut self) -> Value {
        let mut line = String::new();
        self.reader.read_line(&mut line).expect("等 agent 的消息超时");
        serde_json::from_str(&line).unwrap_or_else(|e| panic!("服务端发来的不是 JSON：{line:?} {e}"))
    }

    /// 发请求，读到它的响应为止；响应之前到达的推送一并返回。
    fn call(&mut self, ty: &str, data: Value) -> (Value, Vec<Value>) {
        let id = self.send(ty, data);
        let mut pushes = vec![];
        loop {
            let m = self.recv();
            if m["id"].as_u64() == Some(id) {
                return (m, pushes);
            }
            pushes.push(m);
        }
    }

    /// 等一条指定类型的推送（中间收到别的推送就跳过）。
    fn wait_push(&mut self, ty: &str) -> Value {
        loop {
            let m = self.recv();
            if m["id"].is_null() && m["type"] == ty {
                return m["data"].clone();
            }
        }
    }
}

fn command(c: &mut Client, line: &str) {
    if c.app.world.as_ref().is_some_and(|w|!w.nav.is(&stoneage_client::app::Layer::Command)){c.key(Char('/'));}
    c.text(line);
    c.key(Enter);
}

/// 玩家生成令牌，agent 用它登录并接管；玩家在界面上看到“agent 操控中”，操作被拦下；
/// agent 操作的变化玩家实时看到；玩家收回控制权后 agent 的操作被服务端拒绝；私聊双向；吊销令牌后 agent 被断开。
#[test]
fn agent_takes_over_player_takes_back() {
    let Some(mut server) = Server::start() else { return };
    let account = unique("ag_");

    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    c.submit_password(PASSWORD);
    c.wait_screen(Screen::Characters);
    c.create_character("勇者小明");
    c.wait("创建成功", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 1);
    c.key(Enter);

    c.key(Enter);

    c.wait_screen(Screen::World);

    // 在「系统」页生成令牌（STAGE-86 起系统是第 8 页）。
    c.key(Esc);c.key(End);c.key(Enter);
    for _ in 0..5 {c.key(Down);}c.key(Enter);
    c.text("我的助手");c.key(Tab);c.key(Tab);c.key(Right);c.key(Enter);
    c.wait("令牌生成", 15, |a| a.world.as_ref().is_some_and(|w| w.agent.fresh.is_some() && w.agent.tokens.len() == 1));
    let token = c.app.world.as_ref().unwrap().agent.fresh.as_ref().unwrap().token.clone();
    assert!(token.starts_with("sat_"), "{token}");
    assert!(c.screen_text().contains(&token), "令牌原文应显示在系统页");

    // agent 登录并进入同一个角色：接管控制权。
    let mut agent = Agent::connect(&server.addr);
    let (login, _) = agent.call("agent_login", json!({"token": token}));
    assert_eq!(login["ok"], true, "{login}");
    let (entered, _) = agent.call("enter_world", json!({"slot": 0}));
    assert_eq!(entered["data"]["control"], json!({"controller": "agent", "you": "controller"}), "{entered}");
    c.wait("观察模式", 10, |a| a.world.as_ref().is_some_and(|w| w.observing()));
    let screen = c.screen_text();
    assert!(screen.contains("agent 操控中"), "菜单栏应显示 agent 操控中：\n{screen}");

    // 玩家的“操作”类命令被客户端拦下；“查看”类照常。
    command(&mut c, "turn 3");
    assert!(c.last_error().contains("输入 take 收回控制权"), "{}", c.last_error());
    command(&mut c, "warp 3");
    assert!(c.last_error().contains("输入 take 收回控制权"), "{}", c.last_error());
    command(&mut c, "status");
    c.run_ms(300);
    assert!(c.app.world.as_ref().unwrap().observing());

    // agent 转向，玩家实时看到。
    let (turned, _) = agent.call("turn", json!({"direction": 5}));
    assert_eq!(turned["ok"], true, "{turned}");
    c.wait("看到 agent 的变化", 10, |a| a.world.as_ref().is_some_and(|w| w.entry.position.direction == 5));

    // 服务端判断权限，不靠客户端：玩家收回后，agent 的操作被服务端拒绝。
    c.key(Esc);c.key(Home);c.key(Enter);
    c.wait("收回控制权", 10, |a| a.world.as_ref().is_some_and(|w| !w.observing()));
    let (denied, pushes) = agent.call("turn", json!({"direction": 1}));
    assert_eq!(denied["error"]["code"], "CONTROLLED_BY_PLAYER", "{denied} {pushes:?}");
    command(&mut c, "turn 6");
    c.wait("玩家操作", 10, |a| a.world.as_ref().is_some_and(|w| w.entry.position.direction == 6));
    assert_eq!(agent.wait_push("position_changed")["direction"], 6);

    // STAGE-22：玩家 warp 到加加村，地图页的位置面板和 agent 都跟着变；agent 现在是观察方，warp 被服务端拒绝。
    command(&mut c, "warp 加加村");
    c.wait("传送", 10, |a| a.world.as_ref().is_some_and(|w| (w.entry.position.floor, w.entry.position.x, w.entry.position.y) == (3000, 74, 54)));
    let seen = agent.wait_push("position_changed");
    assert_eq!((seen["floor"].as_i64(), seen["x"].as_i64(), seen["y"].as_i64()), (Some(3000), Some(74), Some(54)), "{seen}");
    c.key(Char('1')); // 切到「地图」页看位置面板
    // STAGE-40：村 warp 进村内地图，地图页把村内位置标在村的入口上。
    let screen = c.screen_text();
    assert!(screen.contains("加鲁卡岛") && screen.contains("74,54") && screen.contains("入口位置"), "{screen}");
    let (denied, _) = agent.call("warp", json!({"place": 1}));
    assert_eq!(denied["error"]["code"], "CONTROLLED_BY_PLAYER", "{denied}");

    // 私聊：玩家 → agent，agent → 玩家。
    command(&mut c, "ag 先去加加村，别打架");
    let msg = agent.wait_push("agent_message");
    assert_eq!((msg["from"].as_str(), msg["text"].as_str()), (Some("player"), Some("先去加加村，别打架")));
    let (said, _) = agent.call("agent_say", json!({"text": "收到，出发"}));
    assert_eq!(said["ok"], true);
    c.wait("agent 的回复", 10, |a| a.log.all().iter().any(|m| m.kind == Kind::Agent && m.text == "收到，出发"));

    // 交还后 agent 恢复操控，吊销令牌后 agent 被断开、控制权回到玩家。
    command(&mut c, "release");
    c.wait("交还", 10, |a| a.world.as_ref().is_some_and(|w| w.observing()));
    command(&mut c, "revoke 1");
    c.wait("恢复操控", 10, |a| a.world.as_ref().is_some_and(|w| !w.observing() && w.agent.tokens.is_empty()));
    let d = agent.wait_push("disconnect");
    assert_eq!(d["reason"], "AGENT_TOKEN_REVOKED", "{d}");
    let screen = c.screen_text();
    assert!(!screen.contains("agent 操控中"), "控制权回到玩家后不应再显示 agent 操控中：\n{screen}");

    server.stop();
    server.assert_no_plaintext(&[PASSWORD, &token]);
}
