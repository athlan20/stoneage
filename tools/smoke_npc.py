"""冒烟的战斗 NPC 段（STAGE-96）：从 smoke-pty.py 拆出来，让它不超过 500 行。

各段用到的按键和 smoke-pty.py 里的 TAB、ENTER 等一致（回车是 "\\r"）。
"""

import os
import subprocess
import sys

ENTER = "\r"


def battle_npc_part(c, a):
    """战斗 NPC（STAGE-96）：里昂蛙王在蛙洞地下5楼（30605）。管理命令把角色搬过去（角色要先离开世界，搬完再进入）；
    没有金珠时 talk 回拒绝台词，数字键关窗；给一颗金珠后 talk 弹两行确认，选 2 否只关窗口。开战和胜负由服务端的
    协议用例 npc_enemy_* 和客户端场景测试覆盖：冒烟里的战斗没法自己打完，之后的步骤要回村。"""
    if not (a.admin and a.admin_db):
        print("   跳过：搬地图和发道具要 --admin 与 --admin-db")
        return
    env = dict(os.environ, STONEAGE_DATABASE_URL=a.admin_db)

    def admin(*args):
        r = subprocess.run([a.admin, "-env", "/nonexistent.env", *args], env=env, capture_output=True, text=True)
        if r.returncode != 0:
            print("   FAIL 管理命令失败：\n" + r.stdout + r.stderr)
            sys.exit(1)

    c.send("/leave" + ENTER)
    c.wait_for("选择角色", "离开世界，回到角色选择页")
    admin("move", "Hero1", "30605", "7", "9")
    # 同 smoke-pty.py 的 enter_world：选中角色 Enter 打开角色动作菜单，再 Enter 选“进入游戏”
    c.send(ENTER)
    c.wait_for("角色动作", "角色动作菜单弹出")
    c.send(ENTER)
    c.wait_for("蛙洞地下5楼", "再次进入：角色在蛙洞地下5楼（管理命令搬过去的）")
    c.wait_for("里昂蛙王", "地图页：战斗 NPC 出现在本地图 NPC 列表里（STAGE-96）")
    c.send("/talk 1735" + ENTER)
    c.wait_for("这里不是你们人类该来的地方，快滚！", "没有金珠：对话回拒绝台词（STAGE-96）")
    c.send("1")  # 没有选项的窗口，数字键就是关掉
    c.wait_gone("这里不是你们人类该来的地方，快滚！", "拒绝窗口关掉")
    admin("give", "Hero1", "19622")
    c.send("/talk 1735" + ENTER)
    c.wait_for("该死的人类，偷了我的东西还敢回来？", "有金珠：弹出两行确认（STAGE-96）")
    c.send("2")  # 否：只关窗口，不开战
    c.wait_gone("该死的人类，偷了我的东西还敢回来？", "选否：窗口关掉，不开战")
