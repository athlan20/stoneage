//! 一条连接上的收发。TCP 每行一条 JSON，WebSocket 每个文本帧一条 JSON（docs/protocol.md 第 1 节）。
//! 对上层两者一样：写一行、读一行。

use std::io::{self, BufRead, BufReader, Write};
use std::net::{Shutdown, TcpStream};

use tungstenite::stream::MaybeTlsStream;
use tungstenite::{Message, WebSocket};

pub trait Transport: Send {
    /// 发出一行（不含换行）。
    fn write_line(&mut self, line: &str) -> io::Result<()>;
    /// 读一条消息，最多等到套接字设定的轮询时间。没有消息返回 `Ok(None)`；连接断了返回 `Err`。
    fn poll_text(&mut self) -> io::Result<Option<String>>;
    /// 关闭连接（尽量礼貌地）。
    fn close(&mut self);
}

/// 读超时在不同平台上表现不同（Unix 是 WouldBlock，Windows 是 TimedOut），两个都算超时。
fn is_timeout(e: &io::Error) -> bool {
    matches!(
        e.kind(),
        io::ErrorKind::WouldBlock | io::ErrorKind::TimedOut
    )
}

pub struct TcpTransport {
    reader: BufReader<TcpStream>,
    /// 还没凑齐一行的字节。读超时时保留，下次接着读，不会丢掉半行。
    partial: Vec<u8>,
}

impl TcpTransport {
    pub fn new(stream: TcpStream) -> Self {
        Self {
            reader: BufReader::new(stream),
            partial: Vec::new(),
        }
    }
}

impl Transport for TcpTransport {
    fn write_line(&mut self, line: &str) -> io::Result<()> {
        let stream = self.reader.get_mut();
        stream.write_all(line.as_bytes())?;
        stream.write_all(b"\n")?;
        stream.flush()
    }

    fn poll_text(&mut self) -> io::Result<Option<String>> {
        match self.reader.read_until(b'\n', &mut self.partial) {
            Ok(0) => Err(io::ErrorKind::UnexpectedEof.into()),
            // 读到了末尾却没有换行：对端在一行中间断开了。
            Ok(_) if self.partial.last() != Some(&b'\n') => {
                Err(io::ErrorKind::UnexpectedEof.into())
            }
            Ok(_) => {
                let bytes = std::mem::take(&mut self.partial);
                String::from_utf8(bytes)
                    .map(Some)
                    .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
            }
            Err(e) if is_timeout(&e) => Ok(None),
            Err(e) => Err(e),
        }
    }

    fn close(&mut self) {
        let _ = self.reader.get_ref().shutdown(Shutdown::Both);
    }
}

pub struct WsTransport {
    pub ws: WebSocket<MaybeTlsStream<TcpStream>>,
}

impl Transport for WsTransport {
    fn write_line(&mut self, line: &str) -> io::Result<()> {
        self.ws.send(Message::Text(line.into())).map_err(to_io)
    }

    fn poll_text(&mut self) -> io::Result<Option<String>> {
        match self.ws.read() {
            Ok(Message::Text(text)) => Ok(Some(text.to_string())),
            Ok(Message::Close(_)) => Err(io::ErrorKind::ConnectionAborted.into()),
            Ok(Message::Binary(_)) => Err(io::Error::new(
                io::ErrorKind::InvalidData,
                "服务端发来了二进制帧",
            )),
            // ping 等控制帧：库会自动排队 pong，这里 flush 一下让它尽快发出去。
            Ok(_) => {
                let _ = self.ws.flush();
                Ok(None)
            }
            Err(tungstenite::Error::Io(e)) if is_timeout(&e) => Ok(None),
            Err(e) => Err(to_io(e)),
        }
    }

    fn close(&mut self) {
        let _ = self.ws.close(None);
        let _ = self.ws.flush();
    }
}

fn to_io(e: tungstenite::Error) -> io::Error {
    match e {
        tungstenite::Error::Io(e) => e,
        other => io::Error::other(other.to_string()),
    }
}
