#!/usr/bin/env python3
"""英雄岛“萨姆吉尔的勾玉”（STAGE-333）：在 gen-npcs.py 转换之后叠一层覆盖，原版 GBK 脚本只读、不改。

两件事（设计见 docs/quest.md「萨姆吉尔的勾玉」）：

1. NPC_OVERRIDES：避难洞窟 5500 的长老（NPC 640，原版只闲聊）改成任务 NPC，脚本来自
   tools/npc_overlay/events/gouyu_elder.txt（UTF-8，写法同原版脚本），脚本文件名 overlay/gouyu_elder。
   只改 kind / type / script，编号不变（覆盖在排序之后做）。
2. GOUYU_PATCHES：改写原版 poru 脚本里跟红暴（任务 40，事件 63）绑死的块，换成新事件号 GOUYU_EVENT：
   - poru/samugiru 的五个勾玉发放块：NOWEV=63 → NOWEV=GOUYU_EVENT；
   - poru/samugiru04 块 0 的条件是原版笔误（`ITEM18539&ITEM=18340&18541`），整段改成五块勾玉齐；
     块 1 的五个“持有勾玉”提示 ENDEV=63 → NOWEV=GOUYU_EVENT，块 2 的完成提示 ENDEV=63 → ENDEV=GOUYU_EVENT；
   - poru/girl 只留五块勾玉齐的兑换块（块 0）：条件 ENDEV=63 → NOWEV=GOUYU_EVENT，兑换后设完成；删掉逐块单换的块 1–5。
   每条都校验原值，命中次数不对就报错（原版脚本一变、补丁没命中都不能悄悄过去）。
"""

from pathlib import Path

from npc_records import talk_arg
import npc_overlay

ROOT = Path(__file__).resolve().parent
GOUYU_EVENT = 185  # 萨姆吉尔的勾玉的事件号（任务表任务 111，tools/gen-quests.py 的 EXTRA_QUESTS）
ELDER_FILE = "overlay/gouyu_elder"  # 长老脚本在 npc-event.json 里的 file，白名单按它放开
ELDER_SCRIPT = ROOT / "npc_overlay" / "events" / "gouyu_elder.txt"
ELDER_ENTRY = {"floor": 5500, "x": 32, "y": 22, "name": "长老"}  # NPC 640，npcs.tsv 里原为 npcgen_man

HOLD_ITEMS = (18538, 18539, 18540, 18541, 18542)  # 萨姆吉尔的勾玉（五块）
BLESSING_ITEM = 18543  # 英雄的祝福
DRAGON_EVENT = 186  # STAGE-336：龙王的守护只给一次的事件号（任务表里没有它，不登记，只作开关用）


def apply_npc_overrides(npcs: list, npc_types: dict) -> None:
    """把长老改成任务 NPC（在排序之后调用，编号和批次不动）。"""
    idx = npc_overlay.find_one(npcs, ELDER_ENTRY)
    n = npcs[idx - 1]
    if n["kind"] != "npcgen_man":
        raise SystemExit(f"STAGE-333：长老 NPC {idx} 的类型是 {n['kind']}，应为 npcgen_man")
    n["kind"] = "changeevent"
    n["type"] = npc_types["changeevent"]
    n["script"] = (ELDER_FILE, ELDER_SCRIPT.read_text(encoding="utf-8"))


def script_of(n: dict, data: Path) -> tuple:
    """changeevent NPC 的脚本：覆盖层给的（文件名, 文本），否则读原版文件（原来 gen-npcs 里的做法）。"""
    if "script" in n:
        return n["script"]
    _, _, arg = n["enemy"].partition("|")
    arg = arg.strip()
    rel = arg[len("file:"):] if arg.startswith("file:") else ""
    return rel, talk_arg(n["enemy"], data)


def _cond(*pairs) -> list:
    """一组与条件（一个 OR 组）：pairs 为 ("NOWEV", 185) 或 ("ITEM", 编号)。"""
    out = []
    for op, v in pairs:
        if op == "ITEM":
            out.append({"op": "ITEM", "cmp": "=", "id": v, "n": 1})
        else:
            out.append({"op": op, "cmp": "=", "n": v})
    return out


GOUYU_PATCHES = [
    # 五个勾玉发放块（ACCEPT，块 1–5）：红暴进行中 → 萨姆吉尔的勾玉进行中
    *[{"kind": "rewrite", "file": "poru/samugiru", "block": b, "from": ("NOWEV", 63), "to": ("NOWEV", GOUYU_EVENT),
       "count": 1, "note": "五个勾玉发放块的进行中条件"} for b in (1, 2, 3, 4, 5)],
    # samugiru 的四个提示块（块 0、6、7、9，MESSAGE）：同样绑红暴，新任务下才看得到提示（块 9 是第一步的药品提示）
    *[{"kind": "rewrite", "file": "poru/samugiru", "block": b, "from": ("NOWEV", 63), "to": ("NOWEV", GOUYU_EVENT),
       "count": 1, "note": "提示块的进行中条件"} for b in (0, 6, 7, 9)],
    # girl 块 6：完成后的“你好棒”，ENDEV=63 → 完成萨姆吉尔的勾玉
    {"kind": "rewrite", "file": "poru/girl", "block": 6, "from": ("ENDEV", 63), "to": ("ENDEV", GOUYU_EVENT),
     "count": 1, "note": "完成后夸奖的条件（完成）"},
    # samugiru04 块 0：原版笔误 ITEM18539&ITEM=18340&18541&18542，整段改成五块勾玉齐
    {"kind": "set_cond", "file": "poru/samugiru04", "block": 0, "expect": "哦～～你全程参与了英雄萨姆吉尔的故事",
     "cond": [_cond(("NOWEV", GOUYU_EVENT), *[("ITEM", i) for i in HOLD_ITEMS])],
     "note": "原版笔误：ITEM18539&ITEM=18340&18541&18542"},
    # samugiru04 块 1：持有勾玉的提示（五个 OR 组，每组 ENDEV=63 & ITEM）→ 进行中
    {"kind": "rewrite", "file": "poru/samugiru04", "block": 1, "from": ("ENDEV", 63), "to": ("NOWEV", GOUYU_EVENT),
     "count": 5, "note": "持有勾玉提示的条件（进行中）"},
    # samugiru04 块 2：完成后的夸奖 → 完成
    {"kind": "rewrite", "file": "poru/samugiru04", "block": 2, "from": ("ENDEV", 63), "to": ("ENDEV", GOUYU_EVENT),
     "count": 1, "note": "完成后夸奖的条件（完成）"},
    # girl 块 0：五块齐的兑换块，条件改成进行中，兑换后设完成
    {"kind": "rewrite", "file": "poru/girl", "block": 0, "from": ("ENDEV", 63), "to": ("NOWEV", GOUYU_EVENT),
     "count": 1, "note": "五块齐兑换的条件（进行中）"},
    {"kind": "end_set", "file": "poru/girl", "block": 0, "value": [GOUYU_EVENT], "note": "兑换后完成萨姆吉尔的勾玉"},
    # girl 块 1–5：逐块单换（原版给英雄的祝福 18543），删掉；五块齐的兑换块已经包含它们
    *[{"kind": "delete", "file": "poru/girl", "block": b, "give": BLESSING_ITEM,
       "take": HOLD_ITEMS[b - 1], "note": "逐块单换的块（原版给英雄的祝福）"} for b in (1, 2, 3, 4, 5)],
]

# STAGE-336（有意偏离原版）：龙王（poru/boy04）块 0 原本是 ENDEV=63&ITEM!=18544，手上没守护就又给两颗，
# 换成挖掘物再换圣石就能无限转。这里给它加上 ENDEV!=DRAGON_EVENT，发完设上事件，之后只给一次，丢了也不补。
DRAGON_PATCHES = [
    {"kind": "set_cond", "file": "poru/boy04", "block": 0, "expect": "\n我会守护着你",
     "cond": [[{"op": "ENDEV", "cmp": "=", "n": 63}, {"op": "ITEM", "cmp": "!=", "id": 18544, "n": 1},
               {"op": "ENDEV", "cmp": "!=", "n": DRAGON_EVENT}]],
     "note": "两颗守护只给一次（STAGE-336 堵循环）"},
    {"kind": "end_set", "file": "poru/boy04", "block": 0, "value": [DRAGON_EVENT], "note": "发过守护后设完成"},
]

# STAGE-345（有意偏离原版）：采伐许可证 2478 只有一张，涛线（poru/team02 ACCEPT 56）要用，岚线的赛尚（20501ev5 块 1，
# 2468×14 换杉的手环 2481）原版也收走它。这里让手环块不再收 2478。块的 del_item 原为 2468、2478、2467（2467 保持原样）。
# 不收 2478 以后，手环块可以再次命中，所以加两个条件：没有手环（ITEM!=2481），和一次性事件 LICENSE_EVENT（手环可以丢，
# 只靠前者能重复换，每次 14 个良木换 20000 石币）。发过手环后设 LICENSE_EVENT，之后只给一次。
LICENSE_EVENT = 187  # STAGE-345：手环只给一次的事件号（任务表里没有它，不登记，只作开关用）
LICENSE_FILE = "jaruga/event/20501ev5"
TAO_DONE_EVENT = 56  # STAGE-349：涛之圣石（任务 45）的完成事件，poru/team02 ACCEPT 56
LICENSE_PATCHES = [
    {"kind": "del_drop", "file": LICENSE_FILE, "block": 1, "item": 2478, "take": [2468, 2478, 2467],
     "note": "手环块不收采伐许可证 2478（STAGE-345）"},
    {"kind": "add_cond", "file": LICENSE_FILE, "block": 1, "expect": "啊！你带了这么多来啊",
     "add": [{"op": "ITEM", "cmp": "!=", "id": 2481, "n": 1}, {"op": "ENDEV", "cmp": "!=", "n": LICENSE_EVENT}],
     "note": "没有手环且没发过（STAGE-345 防重复）"},
    {"kind": "end_set", "file": LICENSE_FILE, "block": 1, "value": [LICENSE_EVENT], "note": "发过手环后设一次性事件"},
    # STAGE-348：块 15（台词“怎么连一根都没有”）条件是有 2478、没有良木，原版选「是」就收走 2478，只回一句台词，
    # 没有任何奖励。许可证补不回来，涛线从此交不了。这一块不再收 2478；块 2~14（带 1~13 个良木换石币）有收益，保持原版。
    {"kind": "del_drop", "file": LICENSE_FILE, "block": 15, "item": 2478, "take": [2478],
     "note": "没有良木的块不收采伐许可证 2478（STAGE-348）"},
    # STAGE-349：先交涛（事件 56）把许可证交掉的角色，手环块因为“要持有 2478”再也命中不了，岚线的木竿敌群（持有手环才出现）
    # 也就出不来。手环块加第二组条件：涛已交付 + 14 个良木 + 没有手环 + 没发过手环，不要求许可证；块 0（没有许可证的提示）
    # 在涛已交付时不再抢先命中。手环仍只给一次（LICENSE_EVENT）。
    {"kind": "or_group", "file": LICENSE_FILE, "block": 1, "expect": "啊！你带了这么多来啊",
     "group": [{"op": "ENDEV", "cmp": "=", "n": TAO_DONE_EVENT}, {"op": "ITEM", "cmp": "=", "id": 2468, "n": 14},
               {"op": "ITEM", "cmp": "!=", "id": 2481, "n": 1}, {"op": "ENDEV", "cmp": "!=", "n": LICENSE_EVENT}],
     "note": "涛已交付、没有许可证也能换手环（STAGE-349）"},
    {"kind": "add_cond0", "file": LICENSE_FILE, "block": 0, "expect": "你有很好的木材嘛",
     "add": [{"op": "ENDEV", "cmp": "!=", "n": TAO_DONE_EVENT}],
     "note": "涛已交付后，没有许可证的提示块不再抢先命中（STAGE-349）"},
]


def _rec(records: list, file: str) -> dict:
    for r in records:
        if r["file"] == file:
            return r
    raise SystemExit(f"STAGE-333 覆盖：没有 {file} 的任务记录")


def _take(block: dict, p: dict) -> None:
    """校验并改一块（rewrite / set_cond / end_set）；delete 由调用方最后统一删。"""
    kind = p["kind"]
    if kind == "rewrite":
        (fop, fn), (top, tn) = p["from"], p["to"]
        hits = [c for group in block["cond"] for c in group if c["op"] == fop and c.get("n") == fn]
        if len(hits) != p["count"]:
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：{fop}={fn} 应命中 {p['count']} 次，实际 {len(hits)}")
        for c in hits:
            c["op"], c["n"] = top, tn
    elif kind == "set_cond":
        msg = block.get("msg", {}).get("nomal_window", "")
        if not msg.startswith(p["expect"]):
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：台词不符：{msg[:20]!r}")
        block["cond"] = p["cond"]
        block.pop("unsupported", None)
    elif kind == "del_drop":
        dels = block.get("del_item") or []
        if [d["id"] for d in dels] != p["take"]:
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：del_item 不符 {[d['id'] for d in dels]}")
        block["del_item"] = [d for d in dels if d["id"] != p["item"]]
    elif kind == "add_cond":
        msg = block.get("msg", {}).get("accept", "")
        if not msg.startswith(p["expect"]):
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：台词不符：{msg[:20]!r}")
        if len(block["cond"]) != 1 or sorted(c["id"] for c in block["cond"][0]) != [2468, 2478]:
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：条件组不是 2478 与 2468 两项")
        block["cond"][0].extend(p["add"])
    elif kind == "or_group":
        msg = block.get("msg", {}).get("accept", "")
        if not msg.startswith(p["expect"]):
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：台词不符：{msg[:20]!r}")
        if len(block["cond"]) != 1:
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：应只有一组条件，实际 {len(block['cond'])}")
        block["cond"].append(p["group"])
    elif kind == "add_cond0":
        msg = block.get("msg", {}).get("nomal_window", "")
        if not msg.startswith(p["expect"]):
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：台词不符：{msg[:20]!r}")
        if len(block["cond"]) != 1:
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：应只有一组条件，实际 {len(block['cond'])}")
        block["cond"][0].extend(p["add"])
    elif kind == "end_set":
        if block.get("end_set"):
            raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：已有 end_set {block['end_set']}")
        block["end_set"] = p["value"]


# STAGE-338：钓竿专家的脚本 sainasu/event/nevent01_2 有一块暗号“楍”（KeyWord），只要求 LV>0，不查任何道具，
# 直接给深恋戒指 2291（使用后经验 +10000000%），块上没有完成事件，说一次给一次，是无限领取。原版也是这样。
# 放开时把这一块从转换结果里拿掉（原版 GBK 脚本不动），其余 9 块照原版。
KEYWORD_DELETES = [
    {"file": "sainasu/event/nevent01_2", "keyword": "楍", "give": [2291], "take": [2570, 2573]},
]


def delete_keyword_blocks(records: list) -> None:
    """删掉 KEYWORD_DELETES 里的暗号块；同一脚本可能有多条记录（每个 NPC 一条），每条都删，都要校验到。"""
    for p in KEYWORD_DELETES:
        recs = [r for r in records if r["file"] == p["file"]]
        if not recs:
            raise SystemExit(f"STAGE-338 覆盖：没有 {p['file']} 的任务记录")
        for rec in recs:
            hits = [i for i, b in enumerate(rec["blocks"]) if b.get("keyword") == p["keyword"]]
            if len(hits) != 1:
                raise SystemExit(f"{p['file']}：暗号 {p['keyword']} 应命中 1 块，实际 {len(hits)}")
            block = rec["blocks"][hits[0]]
            gets = [g["id"] for g in block.get("get_item") or []]
            dels = [d["id"] for d in block.get("del_item") or []]
            if gets != p["give"] or dels != p["take"]:
                raise SystemExit(f"{p['file']} 暗号块交易不符：{gets} {dels}")
            del rec["blocks"][hits[0]]


def apply_event_patches(records: list) -> None:
    """改写 poru 脚本（在 event_record 之后、写 npc-event.json 之前调用）。"""
    delete_keyword_blocks(records)
    deletes: dict = {}
    for p in GOUYU_PATCHES + DRAGON_PATCHES + LICENSE_PATCHES:
        rec = _rec(records, p["file"])
        block = rec["blocks"][p["block"]]
        if p["kind"] == "delete":
            gets = block.get("get_item") or []
            dels = block.get("del_item") or []
            if [g["id"] for g in gets] != [p["give"]] or [d["id"] for d in dels] != [p["take"]]:
                raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：交易不符 {gets} {dels}")
            deletes.setdefault(p["file"], []).append(p["block"])
            continue
        _take(block, p)
    for file, idxs in deletes.items():
        rec = _rec(records, file)
        for b in sorted(idxs, reverse=True):
            del rec["blocks"][b]
