//! 真实 TCP 中继切断连接，服务端和临时数据库继续存活。
use super::{
    PASSWORD,
    driver::{Client, unique},
    server::Server,
};
use crossterm::event::KeyCode::*;
use std::{
    net::{Shutdown, TcpListener, TcpStream},
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
    },
    time::Duration,
};
use stoneage_client::app::{Layer, SYSTEM_MENU, Screen};

struct Proxy {
    addr: String,
    peers: Arc<Mutex<Vec<TcpStream>>>,
    stop: Arc<AtomicBool>,
}
impl Proxy {
    fn new(target: String) -> Self {
        let ln = TcpListener::bind("127.0.0.1:0").unwrap();
        ln.set_nonblocking(true).unwrap();
        let addr = ln.local_addr().unwrap().to_string();
        let peers = Arc::new(Mutex::new(vec![]));
        let stop = Arc::new(AtomicBool::new(false));
        let (p, s) = (peers.clone(), stop.clone());
        std::thread::spawn(move || {
            while !s.load(Ordering::SeqCst) {
                if let Ok((mut client, _)) = ln.accept() {
                    client.set_nonblocking(false).unwrap();
                    let mut server = TcpStream::connect(&target).unwrap();
                    server.set_nonblocking(false).unwrap();
                    *p.lock().unwrap() =
                        vec![client.try_clone().unwrap(), server.try_clone().unwrap()];
                    let (mut a, mut b) = (client.try_clone().unwrap(), server.try_clone().unwrap());
                    std::thread::spawn(move || {
                        let _ = std::io::copy(&mut a, &mut server);
                        let _ = server.shutdown(Shutdown::Both);
                    });
                    std::thread::spawn(move || {
                        let _ = std::io::copy(&mut b, &mut client);
                        let _ = client.shutdown(Shutdown::Both);
                    });
                } else {
                    std::thread::sleep(Duration::from_millis(10));
                }
            }
        });
        Self { addr, peers, stop }
    }
    fn cut(&self) {
        for p in self.peers.lock().unwrap().iter() {
            let _ = p.shutdown(Shutdown::Both);
        }
    }
}
impl Drop for Proxy {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::SeqCst);
        self.cut();
    }
}
fn command(c: &mut Client, line: &str) {
    if c.app
        .world
        .as_ref()
        .is_some_and(|w| !w.nav.is(&Layer::Command))
    {
        c.key(Char('/'));
    }
    c.text(line);
    c.key(Enter);
}
fn reconnect(c: &mut Client) {
    // 当前页 -> 菜单 -> 系统，选择可见的重新连接。
    c.key(Esc);
    c.key(End);
    c.key(Enter);
    assert_eq!(c.app.world.as_ref().unwrap().menu, SYSTEM_MENU);
    c.key(Home);
    c.repeat(Down, 10);
    c.key(Enter);
    assert_eq!(c.app.screen, Screen::Login);
}
#[test]
fn real_disconnect_wrong_password_and_authoritative_refresh() {
    run(false);
}
#[test]
fn websocket_disconnect_and_authoritative_refresh() {
    run(true);
}
fn run(ws: bool) {
    let Some(mut server) = Server::start() else {
        return;
    };
    let proxy = Proxy::new(if ws {
        server.ws_addr.clone()
    } else {
        server.addr.clone()
    });
    let account = unique("retry_");
    let route = if ws {
        format!("ws://{}/ws", proxy.addr)
    } else {
        proxy.addr.clone()
    };
    let mut c = Client::connect(&route);
    c.register(&account, PASSWORD);
    c.submit_password(PASSWORD);
    c.wait_screen(Screen::Characters);
    c.create_character("重连勇者");
    c.wait("建角", 15, |a| {
        a.screen == Screen::Characters && a.chars.list.characters.len() == 1
    });
    c.key(Enter);
    c.key(Enter);
    c.wait_screen(Screen::World);
    c.wait("初始资料", 10, |a| !a.session.busy());
    c.key(Esc);
    c.repeat(Down, 2);
    c.key(Enter); // 背包页
    let original = c.world_position();
    proxy.cut();
    c.wait("离线", 10, |a| {
        a.world.as_ref().is_some_and(|w| !w.online)
    });
    assert_eq!(c.app.screen, Screen::World);
    assert!(c.screen_text().contains("离线"));
    command(&mut c, "turn 3");
    assert!(c.last_error().contains("断开"));
    assert_eq!(c.world_position(), original);
    c.key(Esc); // 收起错误命令栏，背包保留
    reconnect(&mut c);
    c.submit_password("wrong-password");
    c.wait("密码错误", 15, |a| {
        a.log
            .all()
            .iter()
            .any(|m| m.text.contains("账号或密码错误"))
    });
    assert!(c.app.world.as_ref().is_some_and(|w| !w.online));
    c.key(Esc);
    assert_eq!(c.app.screen, Screen::World);
    // 另一次真实登录修改服务端位置，证明恢复后不是翻转本地布尔值。
    let mut other = Client::connect(&server.addr);
    other.login(&account, PASSWORD);
    other.wait_screen(Screen::Characters);
    other.key(Enter);
    other.key(Enter);
    other.wait_screen(Screen::World);
    other.wait("资料", 10, |a| !a.session.busy());
    command(&mut other, "warp 1");
    other.wait("移到村庄", 10, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.entry.position.floor == 1000)
    });
    let latest = other.world_position();
    command(&mut other, "leave");
    other.wait_screen(Screen::Characters);
    drop(other);
    // 上一次取消回到系统页，重新激活同一入口。
    c.app.world.as_mut().unwrap().system.index = 10;
    c.key(Enter);
    c.submit_password(PASSWORD);
    c.wait("认证与完整刷新", 15, |a| {
        a.screen == Screen::World && a.world.as_ref().is_some_and(|w| w.online)
    });
    assert_eq!(c.world_position(), latest);
    assert_ne!(latest, original);
    assert!(c.app.reconnect.is_none());
    assert!(!c.app.world.as_ref().unwrap().auto_battle);
    assert!(c.app.world.as_ref().unwrap().npc_window.is_none());
    assert!(c.app.login.password.is_empty());
    command(&mut c, "turn 5");
    c.wait("恢复后可操作", 10, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.entry.position.direction == 5)
    });
    // 场景目录真实分多页，客户端必须取完而非只显示前200条。
    c.key(Esc);c.key(Esc);c.key(End);c.key(Up);c.key(Enter);
    c.wait("完整NPC目录",15,|a|matches!(a.world.as_ref().and_then(|w|w.scene_step.as_ref()),Some(stoneage_client::app::SceneStep::Catalog{items,total,loading:false,..}) if *total>200 && items.len()==*total));
    c.key(Esc);
    server.stop();
    server.assert_no_plaintext(&[PASSWORD, "wrong-password"]);
}
