"""smoke-pty.py 的 agent 协同一段（STAGE-6/118）：玩家在「Agent」页生成令牌（STAGE-119 起按 0），原始 TCP 连接扮演 agent，
参考脚本的令牌留给 smoke.sh（tools/smoke-pty.py 调用 agent_part）。"""
import json
import re
import socket
import sys
import time

ENTER = "\r"
ESC = "\x1b"
END = "\x1b[F"


class RawAgent:
    """最小的 agent 客户端：用原始 TCP 连接按协议收发 JSON Lines（agent 就是这样接入的）。"""

    def __init__(self, addr):
        host, port = addr.rsplit(":", 1)
        self.sock = socket.create_connection((host, int(port)), timeout=5)
        self.f = self.sock.makefile("rwb")
        self.next = 1
        self.call("hello", {"protocol": 1, "client": "smoke-agent", "role": "agent"})

    def call(self, ty, data):
        """发请求，读到它的响应为止；返回（响应, 之前收到的推送）。"""
        rid = self.next
        self.next += 1
        self.f.write((json.dumps({"id": rid, "type": ty, "data": data}) + "\n").encode())
        self.f.flush()
        pushes = []
        while True:
            m = json.loads(self.f.readline())
            if m.get("id") == rid:
                return m, pushes
            pushes.append(m)

    def wait_push(self, ty):
        while True:
            m = json.loads(self.f.readline())
            if m.get("id") is None and m.get("type") == ty:
                return m["data"]


def command(c, line):
    """输入一条命令（同 client/tests/e2e/agent.rs 的 command）：命令栏没开就先按 /，已经开着就直接输入。
    命令被拒时命令栏会留着（面包屑显示“› 命令”），成功才关闭，所以不能每次都按 /。"""
    if "› 命令" not in c.screen.text():
        c.send("/")
    c.send(line + ENTER)


def agent_part(c, server, token_file, script_token_file):
    """玩家在「系统」页生成令牌 → agent 用令牌登录并接管 → 观察模式 → take / release → 吊销令牌；
    最后再生成一个令牌写到 script_token_file，留给 smoke.sh 的参考脚本步骤（STAGE-118）。
    STAGE-139 起令牌和控制权都在「系统」页（游戏菜单最后一项）：进入后用 / 命令栏输入命令，同 client/tests/e2e/agent.rs。"""
    print("-- agent 协同")
    c.send(ESC)  # 游戏菜单
    c.send(END)  # 光标到最后一项「系统」
    c.send(ENTER)
    c.wait_for("系统 · agent", "系统页")
    command(c, "token new 冒烟 30")
    token = wait_new_token(c, "")
    if token_file:
        with open(token_file, "w") as f:
            f.write(token)
    agent = RawAgent(server)
    resp, _ = agent.call("agent_login", {"token": token})
    assert resp.get("ok"), resp
    resp, _ = agent.call("enter_world", {"slot": 0})
    assert resp["data"]["control"] == {"controller": "agent", "you": "controller"}, resp
    c.wait_for("agent 操控中", "系统页标题显示 agent 操控中")
    command(c, "turn 3")
    c.wait_for("输入 take 收回控制权", "观察模式下操作命令被拦下")
    command(c, "take")
    c.wait_for("你已收回控制权", "take 收回控制权")
    resp, _ = agent.call("turn", {"direction": 1})
    assert resp["error"]["code"] == "CONTROLLED_BY_PLAYER", resp
    print("   OK   服务端拒绝观察方 agent 的操作（CONTROLLED_BY_PLAYER）")
    command(c, "ag 你好")
    assert agent.wait_push("agent_message")["text"] == "你好"
    print("   OK   私聊送达 agent")
    command(c, "revoke 1")
    assert agent.wait_push("disconnect")["reason"] == "AGENT_TOKEN_REVOKED"
    print("   OK   吊销令牌后 agent 被断开")
    if script_token_file:
        command(c, "token new 冒烟脚本 30")
        script_token = wait_new_token(c, token)
        with open(script_token_file, "w") as f:
            f.write(script_token)
    c.send("1")  # 回到地图页（功能页里数字键仍切页）


def wait_new_token(c, old, timeout=15):
    """等屏幕上出现和 old 不同的令牌原文，返回它。"""
    end = time.time() + timeout
    while time.time() < end:
        c.pump(0.1)
        for tok in re.findall(r"sat_[A-Za-z0-9_-]{20,}", c.screen.text()):
            if tok != old:
                return tok
    print("   FAIL 屏幕上没有出现第二个令牌")
    sys.exit(1)
