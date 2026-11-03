"""冒烟的分身队段（STAGE-162）：从 smoke-pty.py 拆出来，让它不超过 500 行。

用真实客户端走新版界面（场景 → Esc 游戏菜单 → 社交）：没有队伍时 Enter 直接组成分身队，
表里出现队长和 4 个分身、顶部状态行出现“队伍 5/5”；动作菜单里解散先确认（默认取消），确认后分身消失；
再用 /party clone、/party disband yes 走一遍命令。战斗里的分身由协议用例 party_battle 和客户端场景测试覆盖
（冒烟里的角色站在村长家，没有遇敌）。进入世界后的落点是场景页，所以先打开菜单再进社交页。
"""

ENTER, ESC, DOWN, RIGHT = "\r", "\x1b", "\x1b[B", "\x1b[C"


def open_social(c):
    """场景页 → Esc 打开游戏菜单 → 下移 5 次选「社交」→ Enter。"""
    c.send(ESC)
    c.wait_for("游戏菜单", "Esc 打开游戏菜单")
    c.send(DOWN * 5 + ENTER)
    c.wait_for("还没有队伍", "社交页：没有队伍时是入口和规则说明（STAGE-162）")
    c.wait_for("组成分身队", "社交页：入口“组成分身队”")


def party_part(c, a):
    print("   进社交页")
    open_social(c)
    c.send(ENTER)
    c.wait_for("队伍 · 1 队长 + 4 分身", "Enter 直接组成分身队：队伍表的标题")
    for n in range(1, 5):
        c.wait_for(f"{a.name}·分身{n}", f"队伍表里有分身 {n}")
    c.wait_for("[队长]", "队伍表里标出队长")
    c.wait_for("队伍 5/5", "顶部状态行出现“队伍 5/5”")
    c.wait_for("组成分身队：你是队长，带着 4 个分身（共 5 人）", "消息栏写明组成分身队")
    print("   动作菜单：解散先确认，默认取消")
    c.send(ENTER)
    c.wait_for("查看详情", "Enter 打开动作菜单")
    c.send(DOWN + ENTER)
    c.wait_for("解散队伍？4 个分身会立即消失", "解散进入确认")
    c.send(ENTER)  # 默认取消
    c.wait_for("已取消解散", "确认默认取消，不解散")
    c.wait_for("队伍 5/5", "取消后队伍还在")
    c.send(ENTER + DOWN + ENTER + RIGHT + ENTER)
    c.wait_for("队伍已解散，分身消失", "确认后队伍解散")
    c.wait_gone("队伍 5/5", "顶部状态行的队伍标记消失")
    c.wait_for("还没有队伍", "社交页回到没有队伍的入口")
    print("   命令：/party clone、/party disband yes")
    c.send("/party clone" + ENTER)
    c.wait_for("队伍 5/5", "/party clone 再次组队")
    c.send("/party disband yes" + ENTER)
    c.wait_gone("队伍 5/5", "/party disband yes 直接解散")
    c.send(ESC)  # 社交页 → 场景
