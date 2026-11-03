//! 服务端地址的解析和建连。`host:port` 走裸 TCP；`ws://`、`wss://` 走 WebSocket，路径缺省为 `/ws`。
//! 不带前缀的地址仍按 TCP 连，兼容旧用法。

use std::net::{TcpStream, ToSocketAddrs};
use std::time::Duration;

use tungstenite::http::Uri;
use tungstenite::stream::MaybeTlsStream;

use super::POLL_INTERVAL;
use super::transport::{TcpTransport, Transport, WsTransport};

const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);
const WS_PATH: &str = "/ws";

pub enum Endpoint {
    Tcp(String),
    /// `url` 是完整的 `ws://` 或 `wss://` 地址（路径已补全），`host_port` 用来建立 TCP 连接。
    Ws {
        url: String,
        host_port: String,
    },
}

impl Endpoint {
    pub fn parse(addr: &str) -> Result<Self, String> {
        let addr = addr.trim();
        if !(addr.starts_with("ws://") || addr.starts_with("wss://")) {
            return Ok(Self::Tcp(addr.to_string()));
        }
        let uri: Uri = addr
            .parse()
            .map_err(|e| format!("服务端地址不对 {addr}（{e}）"))?;
        let tls = uri.scheme_str() == Some("wss");
        let authority = uri
            .authority()
            .ok_or_else(|| format!("服务端地址缺少主机 {addr}"))?;
        let host = uri.host().unwrap_or_default();
        let port = authority.port_u16().unwrap_or(if tls { 443 } else { 80 });
        let path = match uri.path() {
            "" | "/" => WS_PATH,
            p => p,
        };
        let scheme = if tls { "wss" } else { "ws" };
        Ok(Self::Ws {
            url: format!("{scheme}://{authority}{path}"),
            host_port: format!("{host}:{port}"),
        })
    }
}

/// 建立连接，返回收发端，以及同一个套接字的句柄（主线程用它强行关闭连接）。
pub fn open(endpoint: &Endpoint) -> Result<(Box<dyn Transport>, TcpStream), String> {
    match endpoint {
        Endpoint::Tcp(addr) => {
            let stream = connect_tcp(addr)?;
            let socket = stream.try_clone().map_err(|_| "无法建立连接".to_string())?;
            stream
                .set_read_timeout(Some(POLL_INTERVAL))
                .map_err(|_| "无法建立连接".to_string())?;
            Ok((Box::new(TcpTransport::new(stream)), socket))
        }
        Endpoint::Ws { url, host_port } => {
            // wss 要用 TLS；rustls 需要先选定加密实现（ring），重复安装会返回错误，忽略即可。
            let _ = rustls::crypto::ring::default_provider().install_default();
            let stream = connect_tcp(host_port)?;
            let socket = stream.try_clone().map_err(|_| "无法建立连接".to_string())?;
            // 握手期间用连接超时；握手后在实际读流上设置轮询，避免Windows克隆句柄仍沿用5秒超时。
            stream
                .set_read_timeout(Some(CONNECT_TIMEOUT))
                .map_err(|_| "无法建立连接".to_string())?;
            let (ws, _) = tungstenite::client_tls(url.as_str(), stream)
                .map_err(|e| format!("WebSocket 握手失败 {url}（{e}）"))?;
            let reader = match ws.get_ref() {
                MaybeTlsStream::Plain(s) => s,
                MaybeTlsStream::Rustls(s) => &s.sock,
                _ => return Err("不支持的 WebSocket TLS 流".into()),
            };
            reader
                .set_read_timeout(Some(POLL_INTERVAL))
                .map_err(|_| "无法建立连接".to_string())?;
            Ok((Box::new(WsTransport { ws }), socket))
        }
    }
}

/// 解析地址并按顺序尝试连接。域名可能解析出多个地址（localhost 在 macOS 上先是 ::1 再是 127.0.0.1），
/// 有一个连上就行，全部失败才报错（报最后一个错误）。STAGE-9。
fn connect_tcp(addr: &str) -> Result<TcpStream, String> {
    let socks: Vec<_> = addr
        .to_socket_addrs()
        .map_err(|e| format!("无法解析地址 {addr}: {e}"))?
        .collect();
    if socks.is_empty() {
        return Err(format!("无法解析地址 {addr}"));
    }
    let mut last_err = None;
    for sock in &socks {
        match TcpStream::connect_timeout(sock, CONNECT_TIMEOUT) {
            Ok(stream) => {
                let _ = stream.set_nodelay(true);
                return Ok(stream);
            }
            Err(e) => last_err = Some(e),
        }
    }
    let e = last_err.map(|e| e.to_string()).unwrap_or_default();
    Err(format!("连不上服务端 {addr}（{e}）"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ws(addr: &str) -> (String, String) {
        match Endpoint::parse(addr).unwrap() {
            Endpoint::Ws { url, host_port } => (url, host_port),
            Endpoint::Tcp(a) => panic!("应按 WebSocket 处理，实际是 TCP {a}"),
        }
    }

    #[test]
    fn plain_host_port_stays_tcp() {
        assert!(
            matches!(Endpoint::parse(" 127.0.0.1:7065 ").unwrap(), Endpoint::Tcp(a) if a == "127.0.0.1:7065")
        );
    }

    #[test]
    fn ws_defaults_path_and_port() {
        assert_eq!(
            ws("ws://127.0.0.1:7067"),
            ("ws://127.0.0.1:7067/ws".into(), "127.0.0.1:7067".into())
        );
        assert_eq!(
            ws("ws://example.com"),
            ("ws://example.com/ws".into(), "example.com:80".into())
        );
        assert_eq!(
            ws("wss://game.example.com/ws"),
            (
                "wss://game.example.com/ws".into(),
                "game.example.com:443".into()
            )
        );
    }

    #[test]
    fn ws_keeps_explicit_path_and_ipv6() {
        assert_eq!(
            ws("wss://game.example.com:8443/play"),
            (
                "wss://game.example.com:8443/play".into(),
                "game.example.com:8443".into()
            )
        );
        assert_eq!(
            ws("ws://[::1]:7067"),
            ("ws://[::1]:7067/ws".into(), "[::1]:7067".into())
        );
    }

    #[test]
    fn bad_ws_address_is_reported() {
        assert!(Endpoint::parse("ws://").is_err());
    }
}
