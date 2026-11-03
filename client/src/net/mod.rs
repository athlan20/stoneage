//! 服务端连接：每条连接一个网络线程，独占套接字。线程循环里先把待发的行写出去，再用短超时轮询读取，
//! 收到的消息解析后经 channel 交给主循环。主线程只往 channel 里投递，不直接写套接字。
//! 地址可以是 `host:port`（裸 TCP，一行一条 JSON）或 `ws://` / `wss://`（WebSocket，一帧一条），见 `endpoint.rs`。

pub mod compress;
mod endpoint;
mod transport;

use std::net::{Shutdown, TcpStream};
use std::sync::mpsc::{Receiver, Sender, TryRecvError, channel};
use std::thread;
use std::time::Duration;

use serde_json::Value;

use crate::proto::{Incoming, request_line};

pub(crate) use endpoint::{Endpoint, open};
use transport::Transport;

/// 读取轮询间隔：网络线程每隔这么久检查一次有没有要发的行。
const POLL_INTERVAL: Duration = Duration::from_millis(20);
/// 关闭连接时最多等网络线程把已排队的行写完这么久，超时就直接关套接字。
const CLOSE_WAIT: Duration = Duration::from_secs(1);

/// 主线程发给网络线程的东西：一行，或者“写完排队的行后关闭连接，完成后通知我”。
pub enum Outgoing {
    Line(String),
    Close(Sender<()>),
}

pub enum NetEvent {
    Connected {
        gen_id: u64,
        outbox: Sender<Outgoing>,
        socket: TcpStream,
    },
    ConnectFailed {
        gen_id: u64,
        reason: String,
    },
    Message {
        gen_id: u64,
        msg: Incoming,
    },
    Closed {
        gen_id: u64,
    },
}

impl NetEvent {
    pub fn gen_id(&self) -> u64 {
        match self {
            Self::Connected { gen_id, .. }
            | Self::ConnectFailed { gen_id, .. }
            | Self::Message { gen_id, .. }
            | Self::Closed { gen_id } => *gen_id,
        }
    }
}

/// 一条已建立的连接（发送端）。`id` 在连接内从 1 递增。
/// 发送只是把一行投递给网络线程；`close` 等排队的行写完后再关掉套接字，网络线程随之结束。
pub struct Link {
    outbox: Sender<Outgoing>,
    socket: TcpStream,
    next_id: u64,
    pub gen_id: u64,
}

impl Link {
    fn new(outbox: Sender<Outgoing>, socket: TcpStream, gen_id: u64) -> Self {
        Self {
            outbox,
            socket,
            next_id: 1,
            gen_id,
        }
    }

    pub fn send(&mut self, ty: &str, data: &Value) -> std::io::Result<u64> {
        let id = self.next_id;
        self.next_id += 1;
        let line = request_line(id, ty, data);
        self.outbox
            .send(Outgoing::Line(line))
            .map_err(|_| std::io::Error::new(std::io::ErrorKind::BrokenPipe, "连接已断开"))?;
        Ok(id)
    }

    /// 先让网络线程把已排队的行写完、关好连接（最多等 CLOSE_WAIT），再强行关套接字，防止它卡在写入上。
    /// 退出程序时用它，保证最后发出的请求（如 npc_close）不会因为进程结束而丢掉。
    pub fn close(&self) {
        let (done_tx, done_rx) = channel();
        if self.outbox.send(Outgoing::Close(done_tx)).is_ok() {
            let _ = done_rx.recv_timeout(CLOSE_WAIT);
        }
        let _ = self.socket.shutdown(Shutdown::Both);
    }
}

pub struct Net {
    tx: Sender<NetEvent>,
    pub rx: Receiver<NetEvent>,
    gen_id: u64,
}

impl Default for Net {
    fn default() -> Self {
        Self::new()
    }
}

impl Net {
    pub fn new() -> Self {
        let (tx, rx) = channel();
        Self { tx, rx, gen_id: 0 }
    }

    /// 当前有效的连接代数；旧代数的事件一律丢弃。
    pub fn current_gen(&self) -> u64 {
        self.gen_id
    }

    /// 作废当前连接代数（收到的旧事件会被丢弃）。
    pub fn invalidate(&mut self) {
        self.gen_id += 1;
    }

    pub fn link_from(&self, outbox: Sender<Outgoing>, socket: TcpStream, gen_id: u64) -> Link {
        Link::new(outbox, socket, gen_id)
    }

    pub fn connect(&mut self, addr: &str) -> u64 {
        self.gen_id += 1;
        let gen_id = self.gen_id;
        let tx = self.tx.clone();
        let addr = addr.to_string();
        thread::spawn(move || run_connection(&addr, gen_id, &tx));
        gen_id
    }
}

fn run_connection(addr: &str, gen_id: u64, tx: &Sender<NetEvent>) {
    let fail = |reason: String| {
        let _ = tx.send(NetEvent::ConnectFailed { gen_id, reason });
    };
    let endpoint = match Endpoint::parse(addr) {
        Ok(ep) => ep,
        Err(reason) => return fail(reason),
    };
    let (transport, socket) = match open(&endpoint) {
        Ok(opened) => opened,
        Err(reason) => return fail(reason),
    };
    let (outbox, outgoing) = channel();
    if tx
        .send(NetEvent::Connected {
            gen_id,
            outbox,
            socket,
        })
        .is_err()
    {
        return;
    }
    pump(transport, gen_id, tx, &outgoing);
}

/// 网络线程的主循环：先写完待发的行，再读一次。连接断了、收到无法解析的数据、或主线程不再需要时结束。
fn pump(
    mut transport: Box<dyn Transport>,
    gen_id: u64,
    tx: &Sender<NetEvent>,
    outgoing: &Receiver<Outgoing>,
) {
    loop {
        loop {
            match outgoing.try_recv() {
                Ok(Outgoing::Line(line)) => {
                    if transport.write_line(&line).is_err() {
                        return finish(transport, gen_id, tx);
                    }
                }
                // 主线程主动断开：排队的行已经在前面写完了，关闭后静默结束，不报 Closed。
                Ok(Outgoing::Close(done)) => {
                    transport.close();
                    let _ = done.send(());
                    return;
                }
                Err(TryRecvError::Empty) => break,
                // Link 已经被丢弃，没有人再需要这条连接。
                Err(TryRecvError::Disconnected) => return transport.close(),
            }
        }
        match transport.poll_text() {
            Ok(None) => {}
            Ok(Some(text)) => {
                let text = text.trim();
                if text.is_empty() {
                    continue;
                }
                // STAGE-246：压缩行先解压；损坏的压缩行和无法解析的数据一样按协议错误断开。
                let plain = match compress::unwrap_line(text) {
                    Ok(plain) => plain,
                    Err(_) => break,
                };
                match Incoming::parse(&plain) {
                    Ok(msg) => {
                        if tx.send(NetEvent::Message { gen_id, msg }).is_err() {
                            return transport.close();
                        }
                    }
                    Err(_) => break, // 无法解析的数据：按协议错误处理，断开
                }
            }
            Err(_) => break,
        }
    }
    finish(transport, gen_id, tx);
}

fn finish(mut transport: Box<dyn Transport>, gen_id: u64, tx: &Sender<NetEvent>) {
    transport.close();
    let _ = tx.send(NetEvent::Closed { gen_id });
}
