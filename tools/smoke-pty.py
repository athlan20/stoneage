#!/usr/bin/env python3
"""冒烟测试：在伪终端里运行真实的客户端二进制，连真实的服务端，完整走一遍
注册 → 登录 → 建角 → 进入游戏 → 角色页 → leave → 再进入 → quit。

客户端的输出是终端控制序列，这里带一个很小的终端模拟器，把屏幕还原成文本再检查，
所以检查的是玩家真正看到的内容（含中文宽字符）。用法：

    tools/smoke-pty.py --client client/target/debug/stoneage --server 127.0.0.1:7065

服务端用 scripts/smoke.sh 起（临时 schema，不碰开发库）。步骤和手工检查见 docs/smoke.md。
"""
import argparse
import codecs
import fcntl
import os
import pty
import select
import struct
import sys
import termios
import time
import unicodedata

from smoke_agent import agent_part
from smoke_npc import battle_npc_part
from smoke_party import party_part
from smoke_term import COLS, ROWS, Screen



class Client:
    def __init__(self, argv):
        self.screen = Screen()
        pid, fd = pty.fork()
        if pid == 0:
            os.environ["TERM"] = "xterm-256color"
            os.execv(argv[0], argv)
        self.pid, self.fd = pid, fd
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
        self.exited = None

    def pump(self, seconds=0.1):
        end = time.time() + seconds
        while time.time() < end:
            r, _, _ = select.select([self.fd], [], [], 0.05)
            if not r:
                continue
            try:
                data = os.read(self.fd, 65536)
            except OSError:
                data = b""
            if not data:
                self.reap()
                return
            self.screen.feed(data)

    def reap(self):
        if self.exited is None:
            try:
                pid, status = os.waitpid(self.pid, os.WNOHANG)
                if pid:
                    self.exited = os.waitstatus_to_exitcode(status)
            except ChildProcessError:
                self.exited = 0

    def send(self, s):
        os.write(self.fd, s.encode())
        self.pump(0.15)

    def wait_for(self, text, what, timeout=15):
        end = time.time() + timeout
        while time.time() < end:
            self.pump(0.1)
            if text in self.screen.text():
                print(f"   OK   {what}")
                return
            if self.exited is not None and text not in self.screen.text():
                break
        print(f"   FAIL {what}：等待“{text}”超时。当时的屏幕：\n{self.screen.text()}")
        self.kill()
        sys.exit(1)

    def wait_gone(self, text, what, timeout=15):
        end = time.time() + timeout
        while time.time() < end:
            self.pump(0.1)
            self.reap()
            if text not in self.screen.text():
                print(f"   OK   {what}")
                return
        print(f"   FAIL {what}：等“{text}”消失超时。当时的屏幕：\n{self.screen.text()}")
        self.kill()
        sys.exit(1)

    def wait_exit(self, what, timeout=10):
        end = time.time() + timeout
        while time.time() < end:
            self.pump(0.1)
            self.reap()
            if self.exited is not None:
                print(f"   OK   {what}（退出码 {self.exited}）")
                return self.exited
        print(f"   FAIL {what}：客户端没有退出")
        self.kill()
        sys.exit(1)

    def kill(self):
        try:
            os.kill(self.pid, 9)
        except ProcessLookupError:
            pass


TAB, ENTER, RIGHT, ESC = "\t", "\r", "\x1b[C", "\x1b"


def quest_part(c, a):
    """任务页（STAGE-86）：新角色是灰色空提示；管理命令设成进行中后页面不用刷新就出现，消息栏提示；设成已完成后计数变化。"""
    c.send("5")
    c.wait_for("还没有接到任务", "任务页：新角色的空提示")
    import subprocess

    if not (a.admin and a.admin_db):
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        return
    env = dict(os.environ, STONEAGE_DATABASE_URL=a.admin_db)
    r = subprocess.run([a.admin, "-env", "/nonexistent.env", "quest", "Hero1", "1", "doing"], env=env, capture_output=True, text=True)
    if r.returncode != 0:
        print("   FAIL quest 管理命令失败：\n" + r.stdout + r.stderr)
        sys.exit(1)
    c.wait_for("任务【成人仪式】进行中", "在线推送：消息栏提示进行中")
    c.wait_for("进行中 1 · 已完成 0", "任务页：标题计数不用刷新就更新")
    r = subprocess.run([a.admin, "-env", "/nonexistent.env", "quest", "Hero1", "1", "done"], env=env, capture_output=True, text=True)
    if r.returncode != 0:
        print("   FAIL quest 管理命令失败：\n" + r.stdout + r.stderr)
        sys.exit(1)
    c.wait_for("任务【成人仪式】已完成", "在线推送：消息栏提示已完成")
    c.wait_for("进行中 0 · 已完成 1", "任务页：完成后计数变化")
    c.send("1")
    c.wait_for("入口位置", "切回地图页")


def ceremony_part(c, a):
    """任务 NPC（STAGE-90）：村长家的「萨姆吉尔的村长」是任务 NPC，列表里类型是「任务」；它的脚本没放开，
    talk 回“还不能对话”。成人仪式的完整流程（接任务、领仪玉、交仪玉）由服务端测试和协议用例 npc_quest_ceremony 覆盖，
    冒烟里没有走路，传不到 10204 图（warp 只能去有落点的地图）。"""
    c.wait_for("萨姆吉尔的村长", "地图页：任务 NPC 出现在本地图 NPC 列表里（STAGE-90）")
    c.send("/talk 1238" + ENTER)
    c.wait_for("萨姆吉尔的村长 还不能对话", "任务 NPC 脚本没放开，对话回还不能对话（STAGE-90）")


def points_part(c, a):
    """加点（STAGE-24）：角色页显示未分配点数；管理命令发 3 点后页面不用刷新就出现；addpoint 加点后消息栏写明结果，点数不够时报错。"""
    import subprocess
    c.send("2")
    c.wait_for("未分配", "角色页：未分配点数")
    if not (a.admin and a.admin_db):
        print("   SKIP 没有管理命令或数据库串，跳过加点")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        return
    print("   管理命令发 3 点未分配点数")
    env = dict(os.environ, STONEAGE_DATABASE_URL=a.admin_db)
    r = subprocess.run([a.admin, "-env", "/nonexistent.env", "givepoint", "Hero1", "3"],
                       env=env, capture_output=True, text=True, timeout=30)
    if r.returncode != 0:
        print("   FAIL 管理命令失败：\n" + r.stdout + r.stderr)
        sys.exit(1)
    c.wait_for("未分配 3", "在线推送：角色页不用刷新就显示未分配 3")
    c.wait_for("获得 3 点未分配点数", "在线推送：消息栏提示怎么加点")
    c.send("/addpoint 体力 2" + ENTER)
    c.wait_for("已加点：体力 +2，未分配点数还剩 1", "addpoint 加点成功：消息栏写明结果")
    c.wait_for("未分配 1", "角色页：加点后未分配点数减少")
    c.send("/addpoint 体力 9" + ENTER)
    c.wait_for("未分配的点数不够，现在有 1 点", "点数不够时消息栏报错")
    c.send("1")
    c.wait_for("入口位置", "切回地图页")


def bag_part(c, a):
    """背包页（STAGE-28）：空格子与石币；管理命令发一把小斧头后页面不用刷新就出现它。"""
    import subprocess
    c.send("4")
    c.wait_for("装备", "背包页：装备面板")
    c.wait_for("物品", "背包页：物品面板")
    c.wait_for("石币 1000000", "背包页：石币写在物品面板标题里")
    c.wait_for("饰品", "背包页：装备部位")
    if not (a.admin and a.admin_db):
        print("   SKIP 没有管理命令或数据库串，跳过发物品")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        return
    print("   管理命令发一把小斧头（item_id 0）")
    env = dict(os.environ, STONEAGE_DATABASE_URL=a.admin_db)
    r = subprocess.run([a.admin, "-env", "/nonexistent.env", "give", "Hero1", "0"],
                       env=env, capture_output=True, text=True, timeout=30)
    if r.returncode != 0:
        print("   FAIL 管理命令失败：\n" + r.stdout + r.stderr)
        sys.exit(1)
    c.wait_for("小斧头", "在线推送：背包页不用刷新就出现小斧头")
    c.wait_for("攻 +9 防 -3 敏 -3", "背包页：效果文字")
    c.send("1")
    c.wait_for("入口位置", "切回地图页")


def pet_part(c, a):
    """宠物页（STAGE-31/62）：新建角色出生就带 0 号出生地宠物、1 号配套虎、2 号配套雷龙；
    管理命令给一只乌力（放进 3 号栏）后页面不用刷新就出现它，pet status 切换详情。"""
    import subprocess
    c.send("3")
    c.wait_for("宠物栏位", "宠物页：栏位表")
    c.wait_for("乌力乌力", "宠物页：0 号是出生地宠物乌力乌力（STAGE-62）")
    c.wait_for("格鲁西斯", "宠物页：1 号是配套虎格鲁西斯（STAGE-62）")
    c.wait_for("布林帖斯", "宠物页：2 号是配套雷龙布林帖斯（STAGE-62）")
    if not (a.admin and a.admin_db):
        print("   SKIP 没有管理命令或数据库串，跳过发宠物")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        return
    print("   管理命令给一只乌力（编号 1，Lv 5，放进 3 号栏）")
    env = dict(os.environ, STONEAGE_DATABASE_URL=a.admin_db)
    r = subprocess.run([a.admin, "-env", "/nonexistent.env", "givepet", "Hero1", "1", "5"],
                       env=env, capture_output=True, text=True, timeout=30)
    if r.returncode != 0:
        print("   FAIL 管理命令失败：\n" + r.stdout + r.stderr)
        sys.exit(1)
    c.send("/pet status 3" + ENTER)
    c.wait_for("乌力 · 3 号", "pet status 切换到管理命令给的乌力（在线推送）")
    c.wait_for("种族", "宠物页：详情有种族（资质随机，只看字段名）")
    c.wait_for("忠诚", "宠物页：详情有忠诚")
    c.wait_for("休息", "宠物页：新宠物是休息状态")
def enter_world(c, expect, what):
    """从角色选择页进入世界（STAGE-137）：选中角色按 Enter 打开角色动作菜单，
    选“进入游戏”再按 Enter 才发送 enter_world（docs/login.md）。"""
    c.send(ENTER)
    c.wait_for("角色动作", "角色动作菜单弹出")
    c.send(ENTER)
    c.wait_for(expect, what)


def shown_value(text, label):
    """角色页的一行是“腕力 4 攻击 5”这样的形式：取标签后面的那个数字（没有就返回 None）。"""
    for line in text.split("\n"):
        tokens = line.split()
        if label in tokens:
            i = tokens.index(label)
            if i + 1 < len(tokens) and tokens[i + 1].isdigit():
                return int(tokens[i + 1])
    return None


def wait_value(c, label, want, what, timeout=15):
    end = time.time() + timeout
    got = None
    while time.time() < end:
        c.pump(0.1)
        got = shown_value(c.screen.text(), label)
        if got == want:
            print(f"   OK   {what}")
            return
    print(f"   FAIL {what}：{label} 应为 {want}，屏幕上是 {got}\n" + c.screen.text())
    sys.exit(1)


def equip_part(c, a):
    """穿脱整理（STAGE-29）：背包里的小斧头穿到手上，角色页攻击 +9；卸下后恢复。依赖 bag_part 发的小斧头（第 5 格）。"""
    if not (a.admin and a.admin_db):
        print("   SKIP 没有管理命令或数据库串，跳过穿脱")
        return
    c.send("2")
    c.wait_for("攻击", "角色页：攻击")
    base = shown_value(c.screen.text(), "攻击")
    if base is None:
        print("   FAIL 角色页没有攻击数值\n" + c.screen.text())
        sys.exit(1)
    c.send("1")
    c.wait_for("入口位置", "切回地图页")
    print("   item equip 5：小斧头穿到手上，攻击 +9")
    c.send("/item equip 5" + ENTER)
    c.wait_for("已穿上 小斧头", "穿上的消息")
    c.send("2")
    wait_value(c, "攻击", base + 9, "角色页攻击不用刷新就变成 +9")
    c.send("1")
    c.wait_for("入口位置", "切回地图页")
    print("   item unequip 2：卸下，攻击恢复")
    c.send("/item unequip 2" + ENTER)
    c.wait_for("已卸下 小斧头", "卸下的消息")
    c.send("2")
    wait_value(c, "攻击", base, "卸下后攻击恢复")
    c.send("1")
    c.wait_for("入口位置", "切回地图页")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--client", required=True)
    ap.add_argument("--server", required=True)
    ap.add_argument("--account", default=f"smoke_{os.getpid() % 100000}")
    ap.add_argument("--password", default="Zq9-plain-secret")
    ap.add_argument("--name", default="Hero1", help="新建角色的名字（同一个库里名字不能重复，同一个服务端跑第二遍要换一个）")
    ap.add_argument("--token-file", help="把生成的 agent 令牌原文写到这个文件，供脚本检查日志和数据库里没有它")
    ap.add_argument("--script-token-file", help="再生成一个 agent 令牌写到这个文件，供参考脚本步骤登录（agent 段结束时吊销了第一个）")
    ap.add_argument("--no-agent", action="store_true", help="跳过 agent 协同这一段")
    ap.add_argument("--party-only", action="store_true",
                    help="只走分身队这一段（STAGE-162）：注册 → 登录 → 建角 → 进入游戏 → 社交页组队、解散 → 退出；其余各段不跑")
    ap.add_argument("--admin", help="stoneage-admin 二进制：传了就在背包段用它发一把小斧头（STAGE-28）")
    ap.add_argument("--admin-db", help="管理命令用的数据库连接串（指向服务端的临时 schema）")
    ap.add_argument("--idle-seconds", type=int, default=0,
                    help="进入游戏后什么都不做，空闲这么多秒，再确认连接还在（验证心跳；服务端空闲超时是 60 秒，要验证就填 75）")
    a = ap.parse_args()

    c = Client([a.client, "--server", a.server])
    print("-- 登录页")
    c.wait_for("石器时代", "登录页")
    c.wait_for("本地", "线路")
    print("-- 注册")
    c.send(TAB + TAB + RIGHT + ENTER)
    c.wait_for("注册账号", "注册页")
    time.sleep(0.5)
    c.send(a.account + TAB + a.password + TAB + a.password + TAB + ENTER)
    c.wait_for("注册成功", "注册成功提示")
    print("-- 登录")
    c.send(a.password + ENTER)
    c.wait_for("选择角色", "角色选择页")
    print("-- 建角")
    c.send(ENTER)
    c.wait_for("外形", "创建角色：外形页")
    c.send(ENTER)
    c.wait_for("名字", "创建角色：名字与属性页")
    c.send(a.name)
    c.send("\x1b[B" * 8 + "\x1b[C" * 0)
    c.send(ENTER)
    c.wait_for("出生地", "创建角色：出生地页")
    c.send(ENTER)
    c.wait_for(a.name, "角色列表里有新角色")
    if a.party_only:
        print("-- 进入游戏（选角 Enter → 角色动作 Enter）")
        enter_world(c, "萨姆吉尔的村长家", "世界页显示地图名")
        print("-- 分身队（STAGE-162）")
        party_part(c, a)
        print("-- 退出")
        c.send("/quit" + ENTER)
        c.wait_for("退出游戏", "退出确认")
        c.send(RIGHT + ENTER)
        code = c.wait_exit("客户端正常退出")
        if code != 0:
            print(f"   FAIL 退出码应为 0，实际 {code}")
            sys.exit(1)
        print("冒烟测试通过（分身队）")
        return
    print("-- 进入游戏")
    enter_world(c, "萨姆吉尔的村长家", "世界页显示地图名")
    c.wait_for("15,22", "世界页显示出生点坐标")
    # STAGE-131 起进入世界落在场景页，地图页要从游戏菜单进：Esc 打开菜单，Enter 进功能页，1 切到「地图」（同 client/tests/support 的 enter_page）
    c.send(ESC)
    c.send(ENTER)
    c.send("1")
    c.wait_for("入口位置", "地图页：出生点在室内，@ 标在村口并注明入口位置")
    c.wait_for("本地图 NPC（2）", "地图页：村长家有本地图 NPC 表（STAGE-68，STAGE-90 多了任务 NPC 村长）")
    c.wait_for("村长家的厨师", "地图页：NPC 列表里有村长家的厨师，名字不乱码（STAGE-68）")

    print("-- 和 NPC 对话（STAGE-69）")
    # talk 现在走 NPC 菜单 v2（docs/npc-menu-v2.md，STAGE-135）：窗口带 window_id，底部提示是
    # “↑↓ 选择 · Enter 确认 … Esc 关闭”，不再是旧的“按数字或 Esc 关闭”，所以按窗口正文判断弹出和关闭。
    npc_text = "村长家里的饭菜"
    c.send("/talk 225" + ENTER)
    c.wait_for(npc_text, "对话窗口弹出（村长家的厨师）")
    c.send(ESC)
    c.wait_gone(npc_text, "Esc 关掉对话窗口")
    c.send("/talk 225" + ENTER)
    c.wait_for(npc_text, "再次弹出对话窗口")
    c.send("5")  # 没有选项的窗口按数字就是关（v2 窗口同样）
    c.wait_gone(npc_text, "数字键关掉对话窗口")
    print("-- 角色页（STAGE-23）")
    c.send("2")
    c.wait_for("基本信息", "角色页：基本信息面板")
    c.wait_for("攻击", "角色页：能力与衍生数值")
    c.wait_for("未分配", "角色页：未分配点数")
    c.wait_for("地水火风", "角色页：属性面板")
    c.send("1")  # 回到地图页
    c.wait_for("入口位置", "切回地图页")
    print("-- 加点（STAGE-24）")
    points_part(c, a)
    print("-- 背包（STAGE-28）")
    bag_part(c, a)
    print("-- 宠物（STAGE-31）")
    pet_part(c, a)
    print("-- 穿脱整理（STAGE-29）")
    equip_part(c, a)
    print("-- 任务（STAGE-86）")
    quest_part(c, a)
    print("-- 任务 NPC（STAGE-90）")
    ceremony_part(c, a)
    if not a.no_agent:
        agent_part(c, a.server, a.token_file, a.script_token_file)
    if a.idle_seconds:
        print(f"-- 空闲 {a.idle_seconds} 秒（心跳应让连接保持）")
        end = time.time() + a.idle_seconds
        while time.time() < end:
            c.pump(0.5)
            if c.exited is not None or "连接已断开" in c.screen.text() or "空闲" in c.screen.text():
                print("   FAIL 空闲期间连接被断开：\n" + c.screen.text())
                sys.exit(1)
        c.wait_for("15,22", "空闲之后仍在世界中")
    print("-- leave 后再进入")
    c.send("/leave" + ENTER)
    c.wait_for("选择角色", "回到角色选择页")
    enter_world(c, "15,22", "再次进入，位置不变")
    if a.admin and a.admin_db:
        c.send(ESC)  # 进入后落在场景页，数字键要在功能页里才切页：先 Esc、Enter 进游戏菜单的功能页
        c.send(ENTER)
        c.send("2")
        c.wait_for("未分配 1", "重新进入后未分配点数还在（STAGE-24）")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        c.send("4")
        c.wait_for("小斧头", "重新进入后物品还在（STAGE-28）")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        c.send("3")
        c.wait_for("乌力乌力", "重新进入后出生送的宠物还在（STAGE-31/62）")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
        c.send("5")
        c.wait_for("已完成 1", "重新进入后任务进度还在（STAGE-86）")
        c.send("1")
        c.wait_for("入口位置", "切回地图页")
    print("-- 战斗 NPC（STAGE-96）")
    battle_npc_part(c, a)
    print("-- 传送（STAGE-22）")
    c.send(ESC)  # 战斗 NPC 段结束时在场景页，先进功能页的地图
    c.send(ENTER)
    c.send("1")
    c.wait_for("X3", "地图页：地点编号直接标在世界图上（STAGE-81 去掉了地点表）")
    for gone in ("地点 · warp", "朝向"):
        assert gone not in c.screen.text(), f"{gone} 不应再出现在地图页（STAGE-81）"
    c.send("/warp 加加村" + ENTER)
    c.wait_for("74,54", "warp 后标题里的坐标更新")
    c.wait_for("加鲁卡岛", "位置面板写所在岛")
    c.wait_for("已传送到 加加村", "消息栏提示")
    c.wait_for("本地图 NPC（28）", "warp 换地图后 NPC 列表跟着换（STAGE-68/69/90，加加村多了摆设和任务 NPC；STAGE-266 多了精灵王；STAGE-308 多了强者表）")
    print("-- 商店买卖（STAGE-70）")
    c.send("/talk 417" + ENTER)  # 加加村的巫女（有 18 件商品）
    c.wait_for("我有卖很神奇的东西喔", "商店菜单弹出（欢迎语是店里的原话）")
    c.send("1")  # 选「买」看商品列表
    c.wait_for("金币）", "商品列表带单价（buy_rate 计价）")
    # Esc 逐层返回（docs/npc-menu-v2.md）：商品列表 → 买卖菜单 → 关闭。不实际买入（买入流程在 casetest 与场景测试里验证）
    c.send(ESC)
    c.wait_for("我有卖很神奇的东西喔", "Esc 回到买卖菜单")
    c.send(ESC)
    c.wait_gone("我有卖很神奇的东西喔", "再按 Esc 关掉商店窗口")
    c.send("/warp 99" + ENTER)
    c.wait_for("没有编号为 99", "编号不存在时给出提示")
    c.wait_for("地点编号见地图上的 X 标记", "提示里说明编号去哪里看（STAGE-81，STAGE-82 起由服务端给）")
    c.wait_for("74,54", "出错后位置不应变化")
    c.send("/leave" + ENTER)
    c.wait_for("选择角色", "离开后回到角色选择页")
    enter_world(c, "74,54", "再次进入，位置是传送后的位置")
    print("-- 退出")
    c.send("/quit" + ENTER)
    c.wait_for("退出游戏", "退出确认")  # 退出确认默认是取消，右移到「退出」再回车（同分身队那一支）
    c.send(RIGHT + ENTER)
    code = c.wait_exit("客户端正常退出")
    if code != 0:
        print(f"   FAIL 退出码应为 0，实际 {code}")
        sys.exit(1)
    print("冒烟测试通过")


if __name__ == "__main__":
    main()
