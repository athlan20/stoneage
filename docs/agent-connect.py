#!/usr/bin/env python3
"""石器时代 agent 参考连接脚本（STAGE-118）。只用 Python 3 标准库，不需要 pip。

负责 WebSocket 握手、hello、agent_login、心跳，以及按行收发 JSON。
行为规则（何时做什么、怎么汇报）见服务端的 /agent.md，协议消息见 /protocol.md。
文档更新提醒（STAGE-188）：hello 返回的 docs_rev 和上次记下的不一致时，先输出 script_docs_updated 事件，
列出 /changes.md 里新增的条目，然后才输出 hello 的响应。记录在 ~/.stoneage-agent/docs-rev.json（环境变量 STONEAGE_AGENT_STATE 可改）。

用法：
  export STONEAGE_AGENT_TOKEN=sat_...                # 玩家给的令牌（或用 --token-file 指定文件）
  mkfifo agent.in                                     # 命令管道，推荐：脚本会反复读它
  python3 agent-connect.py --url wss://主机/ws --input agent.in > agent.out 2>&1 &
  echo '{"type": "status", "data": {}}' > agent.in    # 发一条请求；结果和推送按行追加到 agent.out

输出：stdout 每行一条 JSON，是服务端的响应和推送原样输出；脚本自己的事件用 script_* 类型。
脚本不会自动重连：连接结束时输出 script_closed 并退出，由 agent 按 /agent.md 的规则决定是否重启。
"""
import argparse, base64, hashlib, itertools, json, os, re, socket, ssl, stat, struct, sys, threading, urllib.request, zlib
from urllib.parse import urlsplit

GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
MAX_INFLATED = 1 << 20           # 压缩行解压后的原文上限，与服务端 protocol.MaxInflatedBytes 一致（STAGE-258）
CLIENT = "stoneage-agent/0.1.0"  # 客户端版本，须不低于服务端配置的最低版本（hello 的 min_client）
RECV_TIMEOUT = 90                # 秒；服务端每 20 秒心跳一次，超过这么久没有任何数据就视为断开
STATE_FILE = os.environ.get("STONEAGE_AGENT_STATE") or os.path.join(os.path.expanduser("~"), ".stoneage-agent", "docs-rev.json")
# /changes.md 每条的标题：## 日期 · docs_rev 修订号（见 docs/agent-changes.md）
DOC_ENTRY = re.compile(r"(?m)^## .*docs_rev ([0-9a-f]{12})\s*$")


def emit(obj):
    print(json.dumps(obj, ensure_ascii=False), flush=True)


class ProtocolError(Exception):
    """服务端发来的数据不符合协议（压缩行无法解压等）：输出 script_closed（PROTOCOL_ERROR）后退出。"""


def unpack_line(text):
    """服务端压缩的一行 {"z":"gzip","d":"<base64 of gzip(原文)>"} 还原成原样 JSON 文本；普通行原样返回。
    解压后超过 1 MiB、不支持的压缩方式、d 不是合法 base64 或 gzip 损坏都抛 ProtocolError。"""
    if '"z"' not in text:
        return text
    try:
        env = json.loads(text)
    except ValueError:
        return text
    if not isinstance(env, dict) or "z" not in env or "type" in env:
        return text
    if env["z"] != "gzip":
        raise ProtocolError(f"不支持的压缩方式：{env['z']}")
    try:
        gz = base64.b64decode(env["d"], validate=True)
        raw = zlib.decompressobj(16 + zlib.MAX_WBITS).decompress(gz, MAX_INFLATED + 1)  # gzip 格式，最多多读 1 字节用来判超限
    except (KeyError, TypeError, ValueError, zlib.error) as e:
        raise ProtocolError(f"压缩消息无法解压：{e}") from e
    if len(raw) > MAX_INFLATED:
        raise ProtocolError("压缩消息解压后超过 1 MiB")
    try:
        return raw.decode("utf-8")
    except UnicodeDecodeError as e:
        raise ProtocolError(f"压缩消息解压后不是 UTF-8：{e}") from e


def recv_msg(ws):
    """读一帧，还原压缩行，解析成消息。连接关闭时返回 None。"""
    text = ws.recv_text()
    if text is None:
        return None
    return json.loads(unpack_line(text))


def load_state(path):
    try:
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except (OSError, ValueError):
        return {}


def save_state(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False)
    os.replace(tmp, path)


def fetch_changes(url):
    parts = urlsplit(url)
    scheme = "https" if parts.scheme == "wss" else "http"
    with urllib.request.urlopen(f"{scheme}://{parts.netloc}/changes.md", timeout=10) as r:
        return r.read().decode("utf-8")


def new_entries(text, last):
    """changes.md 里比 last 新的条目（最上面的开始）；遇到 last 那一条为止。"""
    out = []
    for block in re.split(r"(?m)^(?=## )", text):
        m = DOC_ENTRY.match(block)
        if not m:
            continue
        if m.group(1) == last:
            break
        out.append(block.strip())
    return out


def check_docs(url, rev, path):
    """比对 hello 里的 docs_rev。变了就输出 script_docs_updated 并列出新增条目；读不到 changes.md 时不记下新值，下次登录再提醒。"""
    key = urlsplit(url).netloc
    state = load_state(path)
    last = state.get(key)
    if last == rev:
        return
    if last is not None:
        try:
            entries = new_entries(fetch_changes(url), last)
        except (OSError, ValueError) as e:
            emit({"type": "script_docs_updated", "data": {"from": last, "to": rev, "entries": [], "message": f"文档已更新（docs_rev {last} → {rev}），但读取 /changes.md 失败：{e}。下次登录会再提醒。"}})
            return
        emit({"type": "script_docs_updated", "data": {"from": last, "to": rev, "entries": entries, "message": f"文档已更新：docs_rev {last} → {rev}。先读下面的变更条目，再继续行动。"}})
    state[key] = rev
    try:
        save_state(path, state)
    except OSError as e:
        emit({"type": "script_error", "data": {"message": f"无法记录文档版本到 {path}：{e}"}})


class WebSocket:
    """最小的 WebSocket 客户端：文本帧收发、自动回 pong、不做分片以外的扩展。"""

    def __init__(self, url):
        u = urlsplit(url)
        if u.scheme not in ("ws", "wss") or not u.hostname:
            raise SystemExit("--url 必须是 ws:// 或 wss:// 开头的地址，例如 wss://主机/ws")
        sock = socket.create_connection((u.hostname, u.port or (443 if u.scheme == "wss" else 80)), timeout=15)
        if u.scheme == "wss":
            sock = ssl.create_default_context().wrap_socket(sock, server_hostname=u.hostname)
        self.sock, self.lock = sock, threading.Lock()
        key = base64.b64encode(os.urandom(16)).decode()
        host = u.netloc.rsplit("@", 1)[-1]
        target = (u.path or "/ws") + (f"?{u.query}" if u.query else "")
        sock.sendall((f"GET {target} HTTP/1.1\r\nHost: {host}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
                      f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
        head = self._read_until(b"\r\n\r\n").decode("latin-1")
        accept = base64.b64encode(hashlib.sha1((key + GUID).encode()).digest()).decode()
        if not head.startswith("HTTP/1.1 101") or accept not in head:
            raise SystemExit("WebSocket 握手失败:\n" + head)
        sock.settimeout(RECV_TIMEOUT)

    def _read_until(self, end):
        buf = b""
        while not buf.endswith(end):
            c = self.sock.recv(1)
            if not c:
                raise ConnectionError("连接被服务端关闭")
            buf += c
        return buf

    def _read_exact(self, n):
        buf = b""
        while len(buf) < n:
            chunk = self.sock.recv(n - len(buf))
            if not chunk:
                raise ConnectionError("连接被服务端关闭")
            buf += chunk
        return buf

    def _send_frame(self, opcode, payload):
        n, mask = len(payload), os.urandom(4)  # 客户端发出的帧必须带掩码
        if n < 126:
            header = struct.pack("!BB", 0x80 | opcode, 0x80 | n)
        elif n < 65536:
            header = struct.pack("!BBH", 0x80 | opcode, 0x80 | 126, n)
        else:
            header = struct.pack("!BBQ", 0x80 | opcode, 0x80 | 127, n)
        masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
        with self.lock:
            self.sock.sendall(header + mask + masked)

    def send_text(self, text):
        self._send_frame(0x1, text.encode("utf-8"))

    def recv_text(self):
        """返回下一条文本消息（JSON 字符串）；连接关闭时返回 None。ping 自动回 pong。"""
        message = b""
        while True:
            b1, b2 = self._read_exact(2)
            n = b2 & 0x7F
            if n == 126:
                n = struct.unpack("!H", self._read_exact(2))[0]
            elif n == 127:
                n = struct.unpack("!Q", self._read_exact(8))[0]
            payload = self._read_exact(n) if n else b""
            opcode = b1 & 0x0F
            if opcode == 0x8:
                return None
            if opcode == 0x9:
                self._send_frame(0xA, payload)
            elif opcode in (0x0, 0x1):
                message += payload
                if b1 & 0x80:
                    return message.decode("utf-8")
            elif opcode != 0xA:
                raise ConnectionError(f"不支持的帧类型 {opcode}")


def main():
    p = argparse.ArgumentParser(description="石器时代 agent 参考连接脚本，详见文件开头和 /agent.md")
    p.add_argument("--url", required=True, help="服务端 WebSocket 地址，如 wss://主机/ws")
    p.add_argument("--token-file", help="令牌所在文件；不写则读环境变量 STONEAGE_AGENT_TOKEN")
    p.add_argument("--input", help="命令来源：FIFO 管道路径（推荐，写入后可以反复使用）；不写则读标准输入")
    args = p.parse_args()
    token = os.environ.get("STONEAGE_AGENT_TOKEN", "")
    if args.token_file:
        with open(args.token_file, encoding="utf-8") as f:
            token = f.read().strip()
    if not token:
        raise SystemExit("没有令牌：设置 STONEAGE_AGENT_TOKEN 或用 --token-file 指定文件")

    ws = WebSocket(args.url)
    ids = itertools.count(1)
    lock = threading.Lock()

    def request(typ, data):
        with lock:
            rid = next(ids)
        ws.send_text(json.dumps({"id": rid, "type": typ, "data": data}, ensure_ascii=False))
        return rid

    def wait_reply(rid, show=True):
        while True:
            msg = recv_msg(ws)
            if msg is None:
                raise ConnectionError("握手期间连接被关闭")
            if msg.get("id") == rid and "ok" in msg:
                if show:
                    emit(msg)
                return msg
            emit(msg)

    def handle_line(line):
        line = line.strip()
        if not line:
            return
        try:
            cmd = json.loads(line)
            typ = cmd["type"]
        except (ValueError, KeyError, TypeError):
            emit({"type": "script_error", "data": {"message": "命令必须是 {\"type\": ..., \"data\": {...}} 形式的一行 JSON"}})
            return
        try:
            request(typ, cmd.get("data", {}))
        except OSError as e:
            emit({"type": "script_error", "data": {"message": f"发送失败: {e}"}})

    def read_commands():
        if args.input is None:
            for line in sys.stdin:
                handle_line(line)
            return
        while True:  # FIFO 每次写入结束都会 EOF，所以要重新打开，等下一次写入
            with open(args.input, encoding="utf-8") as f:
                for line in f:
                    handle_line(line)
            if not stat.S_ISFIFO(os.stat(args.input).st_mode):
                return

    def heartbeat(seconds):
        while True:
            threading.Event().wait(seconds)
            try:
                request("ping", {})
            except OSError:
                return

    try:
        # STAGE-258：声明 gzip，服务端对大消息（超过 10 KiB）压缩发送；这里由 recv_msg 还原成原样 JSON 再输出
        hello = wait_reply(request("hello", {"protocol": 1, "client": CLIENT, "role": "agent", "compress": ["gzip"]}), show=False)
        if hello["ok"] and hello["data"].get("docs_rev"):
            check_docs(args.url, hello["data"]["docs_rev"], STATE_FILE)  # 文档更新的提示排在 hello 响应前面
        emit(hello)
        if not hello["ok"]:
            raise SystemExit(f"握手失败: {hello.get('error')}")
        login = wait_reply(request("agent_login", {"token": token}))
        if not login["ok"]:
            raise SystemExit(f"登录失败: {login.get('error')}")
        threading.Thread(target=heartbeat, args=(hello["data"]["heartbeat_seconds"],), daemon=True).start()
        threading.Thread(target=read_commands, daemon=True).start()
        reason = "CLOSED"
        while True:
            msg = recv_msg(ws)
            if msg is None:
                break
            emit(msg)
            if msg.get("type") == "disconnect":
                reason = msg["data"].get("reason", reason)
                break
        emit({"type": "script_closed", "data": {"reason": reason}})
    except ProtocolError as e:
        emit({"type": "script_closed", "data": {"reason": "PROTOCOL_ERROR", "message": str(e)}})
        sys.exit(1)
    except (OSError, ConnectionError) as e:
        emit({"type": "script_closed", "data": {"reason": "CONNECTION_LOST", "message": str(e)}})
        sys.exit(1)


if __name__ == "__main__":
    main()
