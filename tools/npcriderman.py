"""骑乘训练师的对话窗口（STAGE-122，docs/ride.md §2，docs/protocol.md 5.49）。

原版参数文件 data/npc/family/riderman.conf（GBK）按 winno 分窗口：每个窗口是若干 message= 正文行，
加上选项（selected= 之后的 gotowin=）或按钮（prevpressed= 等之后的 gotowin=）。这里把它转成
server/gamedata/npc-riderman.json 的一组窗口记录，运行时由 server/gamedata/riderman.go 读取：

- text：正文，各行用换行连起来（空行就是段落之间的空行）；
- options：选项 [{label, gotowin}]，gotowin 是原版要跳到的窗口号（没写 gotowin 的 selected 行跳过）；
- prev / next / yes / no / ok：按钮的跳转窗口号，没有就不写；
- takegold：原版的学费（运行时不收，学费免费，docs/ride.md §1）。

窗口 6–9（学会的结果）和 5（查询可骑乘的宠物）不在 conf 里：前者由 riderman.go 按规则生成，后者是
原版客户端的 SHOWRIDEPET 窗口，这里改成文字页。

解析对照 gmsv/npc/npc_riderman.c 的 NPC_Riderman_readData：按 winno 找窗口，正文和选项按 message=
出现的顺序，选项的跳转按 selected=/gotowin= 成对出现。
"""

import json
import re
from pathlib import Path

from npc_records import decode

# 按钮名（原版 buttonproc 的下标顺序之外的按键）：xxxpressed= 行的前缀。
BUTTON_KEYS = {"okpressed": "ok", "cancelpressed": "cancel", "yespressed": "yes", "nopressed": "no",
               "prevpressed": "prev", "nextpressed": "next"}


def windows_from_conf(path: Path) -> list[dict]:
    """读 riderman.conf，返回窗口列表（按出现顺序）。"""
    windows = []
    cur = None
    pending = None  # 最近一个空值的按钮/选项行，等它后面的 gotowin= 来认领
    for rawline in path.read_bytes().split(b"\n"):
        line = decode(rawline.rstrip(b"\r"), str(path)).strip()
        if not line or line.startswith("#"):
            continue
        key, sep, value = line.partition("=")
        if not sep:
            continue
        key = key.strip().lower()
        value = value.strip()
        if key == "winno":
            cur = {"winno": int(value), "messages": [], "options": [], "buttons": {}, "takegold": None}
            windows.append(cur)
            pending = None
            continue
        if cur is None:
            continue
        if key == "endwin":
            cur = None
            pending = None
        elif key == "message":
            cur["messages"].append(value)
        elif key == "takegold":
            cur["takegold"] = int(value)
        elif key == "selected" and value == "":
            pending = ("option", None)
        elif key in BUTTON_KEYS and value == "":
            pending = ("button", BUTTON_KEYS[key])
        elif key == "gotowin" and pending is not None:
            kind, name = pending
            if value:
                target = int(value)
                if kind == "option":
                    cur["options"].append({"target": target})
                else:
                    cur["buttons"][name] = target
            pending = None
    return windows


def window_record(win: dict) -> dict:
    """把原始窗口拆成 text / options（选项的文字取正文末尾的几行）。"""
    msgs = list(win["messages"])
    # 第一行是数字的（"2"）是原版的行数标记，不是正文。
    if msgs and msgs[0].isdigit():
        msgs = msgs[1:]
    n = len(win["options"])
    labels = msgs[len(msgs) - n:] if n else []
    body = msgs[:len(msgs) - n] if n else msgs
    while body and body[-1] == "":  # 选项前的空行只是分隔
        body = body[:-1]
    rec = {"winno": win["winno"], "text": "\n".join(body)}
    if n:
        rec["options"] = [{"label": label, "gotowin": opt["target"]}
                          for label, opt in zip(labels, win["options"])]
    for name, target in win["buttons"].items():
        rec[name] = target
    if win["takegold"] is not None:
        rec["takegold"] = win["takegold"]
    return rec


def reachable(windows: list[dict], start: int = 1) -> set[int]:
    """从主窗口出发能走到的窗口号（家族成立的 1010、1011 走不到，不收）。"""
    by_no = {w["winno"]: w for w in windows}
    seen, todo = set(), [start]
    while todo:
        no = todo.pop()
        if no in seen or no not in by_no:
            continue
        seen.add(no)
        w = by_no[no]
        todo += [o["target"] for o in w["options"]] + list(w["buttons"].values())
    return seen


def riderman_records(data: Path) -> list[dict]:
    """data 是原版 gmsv/data 目录；返回窗口记录列表。参数文件不在时返回空列表。"""
    path = data / "npc/family/riderman.conf"
    if not path.is_file():
        return []
    windows = windows_from_conf(path)
    keep = reachable(windows)
    return [window_record(w) for w in windows if w["winno"] in keep]


def write_json(records: list[dict], out: Path) -> None:
    out.write_text(json.dumps(records, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")


if __name__ == "__main__":
    import sys
    data = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("original-data/gmsv/data")
    recs = riderman_records(data)
    print(f"骑乘训练师窗口 {len(recs)} 个")
    for r in recs:
        print(r["winno"], re.sub(r"\n", " / ", r["text"])[:40], [o["label"] for o in r.get("options", [])])
