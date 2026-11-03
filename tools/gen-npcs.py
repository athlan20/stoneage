#!/usr/bin/env python3
"""生成 NPC 数据表（STAGE-68，docs/protocol.md 5.32）与对话参数（STAGE-69，5.33）。

从原版服务端的 NPC 创建文件（data/npc/**/*.create，GBK 编码）生成一个 UTF-8 的 TSV，
编译进服务端（server/gamedata），只收录后续需求会实现的几类 NPC：
对话（npcgen_man）、告示牌（npcgen_signboard）、商店（npcgen_shop）、护士（npcgen_winhealer）、
存档点（npcgen_savepoint）、宠物店（npcgen_petshop）、传送员（npcgen_warpman），
以及 STAGE-69 的报时（npcgen_timeman）和摆设（npcgen_mugon），STAGE-90 的任务 NPC（changeevent，
事件脚本转成 npc-event.json，见 tools/npcevent.py 与 docs/quest.md），STAGE-143 的宠物技能店
（npcgen_petskillshop，参数转成 npc-petskillshop.json，见 docs/pet-skill.md）。
道场、赌博、家族等类型暂不收；`npcgen_warp`（2904 个）是踩上去自动传送的
地图出入口，不是 NPC，也不收。表里保留原版类型名，以后增量补类型。

解析规则对照 gmsv/npc/npccreate.c 的 NPC_readCreateFile：
- 递归扫描 data/npc 下所有文件，首行是 NPCCREATE 的才算创建文件（文件名和扩展名不挑），
  跳过 ~ 结尾和 .bak 的备份；
- `#` 开头是注释，`{` 开块、`}` 收块，块内是 `key=value`（key 大小写不敏感，value 可含 =）；
- 一块至少要有一条 `enemy=`（模板名|参数）、`born` 位置齐全、floorid 是真实存在的地图，
  否则原版加载器会把这块丢掉，这里同样丢弃；
- born 位置两种写法：`borncorner=lx,ly,rx,ry`（对角两点）或 `borncenter=cx,cy,w,h`（中心加宽高），
  统一换算成矩形后取中心点当作 NPC 的展示位置（本期不做走路，坐标只用来展示和以后做地图用）；
- NPC 的类型取第一条 enemy= 的模板名（npcgen_* 的功能映射见 genout/npcgen.template），
  createnum 大于 1 时原版会生成多个分身，这里一块只记一行。
- 数值一律 atoi（取开头的整数），同 gen-encounters.py。

编号按地图、坐标、类型、名字排序后从 1 顺序分配，保证重新生成时编号稳定。
STAGE-69 新增的报时和摆设排在全部 STAGE-68 类型之后（排序键多一档“批次”），
已有 1061 个 NPC 的编号保持不变，协议用例和已上线的数据不受影响。

对话参数另外写进 server/gamedata/npc-talk.json（docs/protocol.md 5.33）：编号 → 说的内容。
商店（npcgen_shop）和限量商店（npcgen_limitshop）的买卖参数写进 server/gamedata/npc-shop.json
（STAGE-70，docs/protocol.md 5.37）：倍率（buy_rate/sell_rate）、商品（ItemList 展开成编号列表，
最多 60 个（STAGE-302 由 33 提高，原版 MAXSHOPITEM=33 只是说明性约束；有效性按 itemset.tsv 的物品表）、台词（main_msg 等，逐字保留）。
STAGE-302 便利店素材集中化：tools/npc_overlay/convenience_shops.json 覆盖表在原版解析之上改 1009/4009 的素材店
（商品换成整组、新增 2 家、删除 7 个源 NPC），原版数据不动；新增 NPC 放进末尾批次，删除的留编号空洞，已有编号不变。
解析对照 gmsv/npc/npc_itemshop.c；限量店的 LimitItemType/LimitItemNo 收购范围本期不实现（全店统一
按 sell_rate 收），special_item/special_rate 的高价回收实现。
- 对话（npcgen_man）：enemy 参数是一段话或 file:genout/msg_<地图>_<x>_<y>（GBK），
  按原版 npc_townpeople.c 的规则按半角逗号分成多句，运行时随机取一句；
- 告示牌（npcgen_signboard）：参数同上（signb_<地图>_<x>_<y>），原文保存，不按逗号分句，
  里面的 `\n` 是字面两个字符，协议发送时才转成换行（npc_signboard.c）；
- 报时（npcgen_timeman）：参数是 tman_<地图>_<x>_<y>，`key:value` 行（npc_timeman.c），
  保存 time 时段、main_msg 和 change_msg 的分句，`change_no` 变身图形不收；
- 摆设（npcgen_mugon）：没有参数，不生成记录，服务端固定回「……」。
- 战斗（npcenemy，STAGE-96）：enemy 的模板在 .template 里是 functionset=NPCEnemy 的 NPC（道场弟子也在内，
  标不支持），参数文件转成 server/gamedata/npc-enemy.json，逐项见 tools/npcenemy.py 与 docs/battle-npc.md（STAGE-91）。
- 查不到参数的对话类 NPC 同样按「……」处理，服务端不依赖 npc-talk.json 一定有记录。

用法：python3 tools/gen-npcs.py [数据目录]
不传目录时用 original-data/gmsv/data。
"""

import json
import re
import sys
from pathlib import Path

from npc_records import (KNOWN_ITEMS, atoi, decode, decode_name, healer_record, petshop_record,
                         petskillshop_record, savepoint_record, shop_record, talk_arg, talk_record,
                         warpman_record)

from npcenemy import enemy_ids, enemy_record, enemy_templates, summarize
from npcevent import event_record
from npcriderman import riderman_records, write_json
import npc_overlay  # STAGE-302：便利店素材店覆盖表
import npc_event_overlay  # STAGE-333：萨姆吉尔的勾玉（长老 NPC 与 poru 块改写）
ROOT = Path(__file__).resolve().parent.parent
DEFAULT_DATA = ROOT / "original-data/gmsv/data"
OUT_DIR = ROOT / "server/gamedata"

# 本期收录的类型 → 显示用的类型名（docs/protocol.md 5.32 的 type 字段）。
# NPC_BATCHES 里的顺序就是编号排序的先后（STAGE-68 的在前，STAGE-69 新增的在后）。
NPC_BATCHES = [
    {  # STAGE-68
        "npcgen_man": "对话",
        "npcgen_signboard": "告示牌",
        "npcgen_shop": "商店",
        "npcgen_winhealer": "护士",
        "npcgen_savepoint": "存档点",
        "npcgen_petshop": "宠物店",
        "npcgen_warpman": "传送员",
    },
    {  # STAGE-69：报时（按游戏时间段说话）和摆设（不说话的装饰 NPC）
        "npcgen_timeman": "报时",
        "npcgen_mugon": "摆设",
    },
    {  # STAGE-90：任务 NPC（事件脚本，原版 npc_exchangeman.c）
        "changeevent": "任务",
    },
    {  # STAGE-96：战斗 NPC（functionset=NPCEnemy 的模板，参数见 npc-enemy.json）
        "npcenemy": "战斗",
    },
    {  # STAGE-143：宠物技能店（花钱给宠物学技能，参数见 npc-petskillshop.json）
        "npcgen_petskillshop": "宠物技能店",
    },
    {  # STAGE-122：骑乘训练师（riderman，对话窗口见 npc-riderman.json，docs/ride.md §2）
        "riderman": "骑乘训练师",
    },
    {  # STAGE-236：宠物转生（npc_transmigration，精灵王/玛蕾菲雅，台词在 data/npc/777/ten.arg 的 PETTRANS 段）
        "npc_transmigration": "宠物转生",
    },
    {  # STAGE-255：美容师（npcgen_charm，花石币买魅力，原版 npc_charm.c；四张医院各一个）
        "npcgen_charm": "美容师",
    },
    {  # STAGE-266：主城精灵王（npc_tensei，人物转生，窗口流程见 server/character/npc_tensei.go）
        "npc_tensei": "转生",
    },
    {  # STAGE-309：道具仓库（寄存、领取道具，原版 npc_poolitemshop.c；四家便利店各一位，docs/protocol.md 5.60）
        "npcgen_poolitemshop": "道具仓库",
    },
    {},  # STAGE-302：便利店覆盖表新增的素材店（tools/npc_overlay.py，kind 仍是 npcgen_shop，单独成批以保持编号）
    {  # STAGE-308：强者表（竞技场和各村，原版 npc_duelranking.c，窗口见 docs/protocol.md 5.75）；排在覆盖表之后，已有编号不变
        "duelranking": "强者表",
    },
    {  # STAGE-332：猜谜大师（原版 npc_quiz.c，只收认定场 812 的 quz_812_28_14，其余 22 个猜谜人物另起需求）
        "npcgen_quiz": "猜谜",
    },
]
# 覆盖表那一批是空字典（见上），按位置找，不写死下标。
OVERLAY_BATCH = NPC_BATCHES.index({})
QUIZ_NPC_FILE = "file:genout/quz_812_28_14"  # STAGE-332：猜谜大师（认定场 812 的 28,14）的参数文件
NPC_TYPES = {kind: name for batch in NPC_BATCHES for kind, name in batch.items()}
# STAGE-266：原版数据里没有主城的人物转生精灵王（只有宠物转生的精灵王，见 STAGE-236），这里按坐标补四个。
# 坐标是存档点旁的可站立格（已用 LS2MAP 障碍数据核对：MAP_walkAbleFromPoint，原版障碍格不可站），
# 任务书建议的格子在 1000/2000/3000/4000 分别是障碍，取离建议格最近的可站立空格：
# 1000 (95,99) 障碍 → (96,99)；2000 (68,58) 障碍 → (69,58)；3000 (95,97) 障碍 → (94,97)；4000 (69,55) 障碍 → (69,57)。
EXTRA_NPCS = [
    {"floor": 1000, "x": 96, "y": 99, "kind": "npc_tensei", "name": "精灵王"},
    {"floor": 2000, "x": 69, "y": 58, "kind": "npc_tensei", "name": "精灵王"},
    {"floor": 3000, "x": 94, "y": 97, "kind": "npc_tensei", "name": "精灵王"},
    {"floor": 4000, "x": 69, "y": 57, "kind": "npc_tensei", "name": "精灵王"},
]
# 有买卖参数、要生成 npc-shop.json 记录的类型（STAGE-70）。
SHOP_KINDS = ("npcgen_shop", "npcgen_limitshop")
# STAGE-280：成人仪式改为只收发 1 个仪玉。原版 GBK 脚本（original-data）是行为对照，不改；
# 这里在生成时覆盖解析结果。每条补丁都校验原值（整数相等；文案是子串且必须恰好出现一次），
# 原版脚本一变或补丁没命中就直接报错，生成报告逐条列出改了什么。
# 字段：脚本相对路径、块序号（从 0 开始，即 npc-event.json 里 blocks 的下标）、字段路径、原值、新值、说明。
EVENT_PATCHES = [
    {"file": "jaruga/event/event04_1", "block": 0, "path": ("cond", 0, 0, "n"), "old": 15, "new": 1,
     "note": "审判官交任务条件：仪玉 2417 个数"},
    {"file": "jaruga/event/event04_1", "block": 0, "path": ("del_item", 0, "n"), "old": 15, "new": 1,
     "note": "审判官扣除仪玉 2417 个数"},
    {"file": "jaruga/event/event04_1", "block": 1, "path": ("msg", "thanks"),
     "old": "取回１５个来", "new": "取回１个来", "note": "审判官 REQUEST ThanksMsg"},
    {"file": "jaruga/event/event04_2", "block": 0, "path": ("get_item", 0, "n"), "old": 15, "new": 1,
     "note": "使者发放仪玉 2417 个数"},
    {"file": "jaruga/event/event04_2", "block": 0, "path": ("msg", "thanks"),
     "old": "给你１５个仪玉", "new": "给你１个仪玉", "note": "使者 ThanksMsg"},
    {"file": "jaruga/event/event04_2", "block": 0, "path": ("msg", "item_full"),
     "old": "如果想要全部的仪玉，就要空出15个空间才可", "new": "你的道具袋没有空间了", "note": "使者 ItemFullMsg"},
]


def trunc_div(a: int, b: int) -> int:
    """C 语义的整数除法：向零取整（坐标可能出现负差值）。"""
    q = abs(a) // abs(b)
    return q if (a >= 0) == (b >= 0) else -q


def map_floors(data: Path) -> set[int]:
    """收集真实存在的地图编号（LS2MAP 文件头，见 tools/gen-mapnames.py）。"""
    floors = set()
    for path in (data / "map").rglob("*"):
        if not path.is_file():
            continue
        head = path.read_bytes()[:8]
        if head[:6] == b"LS2MAP":
            floors.add(int.from_bytes(head[6:8], "big"))
    return floors


def read_blocks(path: Path):
    """按 npccreate.c 的逻辑读一个文件，返回块的列表（每块是 key → value 的字典，
    enemy 是列表）；不是创建文件返回 None。"""
    raw = path.read_bytes()
    lines = raw.split(b"\n")
    if not lines or lines[0].rstrip(b"\r") != b"NPCCREATE":
        return None
    blocks = []
    block, started = None, False
    for lineno, rawline in enumerate(lines[1:], 2):
        line = rawline.rstrip(b"\r")
        if not line or line[0:1] == b"#":
            continue
        ch = line[0:1]
        if ch == b"{":
            if started:
                raise SystemExit(f"{path}:{lineno}: 块还没结束又遇到 {{")
            block, started = {}, True
        elif ch == b"}":
            if not started:
                raise SystemExit(f"{path}:{lineno}: 没有 {{ 却遇到 }}")
            block["_file"], block["_line"] = str(path), lineno
            blocks.append(block)
            block, started = None, False
        elif started:
            key_raw, sep, value_raw = line.partition(b"=")  # "=" 是 ASCII，不会落在 GBK 的字节里
            if not sep:
                continue
            key = key_raw.decode("ascii", "replace").strip().lower()
            if key == "name":
                value = decode_name(value_raw, f"{path}:{lineno}").strip()
            else:
                value = decode(value_raw, f"{path}:{lineno}").strip()
            if key == "enemy":
                block.setdefault("enemy", []).append(value)
            else:
                block[key] = value
    return blocks


def born_center(block: dict) -> tuple[int, int] | None:
    """把 born 位置换算成展示坐标（矩形中心）。两种写法都没有时返回 None。"""
    if "borncorner" in block:
        f = block["borncorner"].split(",")
        if len(f) != 4:
            return None
        lx, ly, rx, ry = (atoi(v) for v in f)
        x1, x2 = min(lx, rx), max(lx, rx)
        y1, y2 = min(ly, ry), max(ly, ry)
    elif "borncenter" in block:
        f = block["borncenter"].split(",")
        if len(f) != 4:
            return None
        cx, cy, w, h = (atoi(v) for v in f)
        x1, x2 = cx - trunc_div(w, 2), cx + trunc_div(w, 2)
        y1, y2 = cy - trunc_div(h, 2), cy + trunc_div(h, 2)
    else:
        return None
    return trunc_div(x1 + x2, 2), trunc_div(y1 + y2, 2)


def apply_event_patches(record: dict, hits: list[int], log: list[str]) -> None:
    """按 EVENT_PATCHES 改一条任务记录（STAGE-280）：原值不符就报错；命中的补丁计数并写进 log。"""
    for i, p in enumerate(EVENT_PATCHES):
        if record["file"] != p["file"]:
            continue
        node = record["blocks"][p["block"]]
        for key in p["path"][:-1]:
            node = node[key]
        last = p["path"][-1]
        old, new = p["old"], p["new"]
        if isinstance(old, str):
            cur = node[last]
            if cur.count(old) != 1:
                raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：原文 {old!r} 应恰好出现一次，实际 {cur!r}")
            node[last] = cur.replace(old, new)
        else:
            if node[last] != old:
                raise SystemExit(f"{p['file']} 块 {p['block']} {p['note']}：原值应为 {old}，实际 {node[last]}")
            node[last] = new
        hits[i] += 1
        log.append(f"{p['file']} 块{p['block']} {p['note']}：{old} → {new}")


def print_event_summary(events: list[dict], out: Path, notes: list[str]) -> None:
    """任务脚本的转换结果：块总数、支持和不支持的块，以及不支持的原因（按出现次数排），最后列转换说明（STAGE-219）。"""
    blocks = [b for e in events for b in e["blocks"]]
    bad = [b["unsupported"] for b in blocks if b.get("unsupported")]
    reasons: dict[str, int] = {}
    for why in bad:
        key = re.sub(r"[0-9]+", "N", why)
        reasons[key] = reasons.get(key, 0) + 1
    top = sorted(reasons.items(), key=lambda kv: -kv[1])[:8]
    print(f"任务脚本 {len(events)} 个 NPC、{len(blocks)} 个事件块（支持 {len(blocks) - len(bad)}，"
          f"不支持 {len(bad)}）→ {out}")
    for why, count in top:
        print(f"    {count:5d}  {why}")
    if notes:
        print(f"  NOW 条件转换说明（{len(notes)} 条，原版恒假，见 docs/quest.md）：")
        for note in notes:
            print(f"    {note}")


def main():
    data = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_DATA
    floors = map_floors(data)
    if not floors:
        raise SystemExit(f"{data}/map 下没有找到地图文件")
    templates = enemy_templates(data)  # 战斗 NPC 的模板名（STAGE-96）
    ids = enemy_ids(OUT_DIR / "enemies.tsv")

    npcs = []
    shop = []
    events = []
    event_notes: list[str] = []  # 任务脚本转换说明（STAGE-219：丢掉的 NOW 组、省略的块），打印给人复核
    patch_hits = [0] * len(EVENT_PATCHES)  # STAGE-280：每条事件补丁命中的次数，必须各为 1
    patch_log: list[str] = []
    dropped = {"no_enemy": 0, "no_born": 0, "bad_floor": 0, "other_type": 0}
    files = 0
    for path in sorted((data / "npc").rglob("*")):
        if not path.is_file() or path.name.endswith("~") or path.suffix == ".bak":
            continue
        blocks = read_blocks(path)
        if blocks is None:
            continue
        files += 1
        for b in blocks:
            enemy = b.get("enemy") or []
            if not enemy:
                dropped["no_enemy"] += 1
                continue
            kind = enemy[0].split("|", 1)[0].strip()
            if kind.lower() in templates:
                kind = "npcenemy"
            if kind not in NPC_TYPES:
                dropped["other_type"] += 1
                continue
            if kind == "npcgen_quiz" and not enemy[0].endswith(QUIZ_NPC_FILE):
                dropped["quiz_not_yet"] = dropped.get("quiz_not_yet", 0) + 1  # STAGE-332：本期只收录认定场的猜谜大师
                continue
            floor = atoi(b.get("floorid", ""))
            if floor not in floors:
                dropped["bad_floor"] += 1
                continue
            pos = born_center(b)
            if pos is None:
                dropped["no_born"] += 1
                continue
            npcs.append({
                "floor": floor, "x": pos[0], "y": pos[1],
                "kind": kind, "type": NPC_TYPES[kind],
                "batch": next(i for i, batch in enumerate(NPC_BATCHES) if kind in batch),
                "name": re.sub(r"[\t\r\n]+", " ", b.get("name", "")).strip(),
                "enemy": enemy[0], "file": b["_file"], "line": b["_line"],
            })
    # STAGE-266：补进来的 NPC（不在原版数据里），来源记为本脚本，行号按列表顺序。
    for line_no, e in enumerate(EXTRA_NPCS, 1):
        if e["floor"] not in floors:
            raise SystemExit(f"EXTRA_NPCS 第 {line_no} 条的地图 {e['floor']} 不存在")
        npcs.append({
            "floor": e["floor"], "x": e["x"], "y": e["y"],
            "kind": e["kind"], "type": NPC_TYPES[e["kind"]],
            "batch": next(i for i, batch in enumerate(NPC_BATCHES) if e["kind"] in batch),
            "name": e["name"], "enemy": e["kind"], "file": "tools/gen-npcs.py", "line": line_no,
        })

    # STAGE-302：覆盖表的新增店进末尾批次（排序后编号接在最后，已有编号不动）。
    overlay = npc_overlay.load()
    added_npcs = npc_overlay.add_npcs(npcs, overlay, floors, NPC_TYPES["npcgen_shop"], OVERLAY_BATCH)
    # 编号要稳定：先按收录批次分档（STAGE-69 新增的两类排在全部 STAGE-68 类型之后，
    # 已有编号保持不变），档内按地图、坐标、类型、名字排，来源文件和行号做最后的平局裁决。
    npcs.sort(key=lambda n: (n["batch"], n["floor"], n["x"], n["y"], n["kind"], n["name"], n["file"], n["line"]))
    npc_event_overlay.apply_npc_overrides(npcs, NPC_TYPES)  # STAGE-333：长老改成任务 NPC（排序之后，编号不动）
    rows = []
    talk = []
    removed = npc_overlay.removed_ids(npcs, overlay)  # STAGE-302：删除的源 NPC 留编号空洞，不输出
    for i, n in enumerate(npcs, 1):
        if i in removed:
            continue
        rows.append([str(i), str(n["floor"]), str(n["x"]), str(n["y"]), n["kind"], n["type"], n["name"]])
        record = talk_record(n["kind"], n["enemy"], data)
        if record is not None:
            talk.append({"id": i, **record})
        if n["kind"] in SHOP_KINDS:
            if record := shop_record(n["kind"], n["enemy"], data):
                shop.append({"id": i, **record})
        if n["kind"] == "changeevent":
            rel, text = npc_event_overlay.script_of(n, data)
            record, notes = event_record(rel, text, KNOWN_ITEMS, floors)
            apply_event_patches(record, patch_hits, patch_log)
            events.append({"id": i, **record})
            event_notes.extend(notes)
    shop_changed, shop_added = npc_overlay.apply_shops(npcs, shop, overlay, KNOWN_ITEMS)
    pet_shops = npc_overlay.apply_item_overlay(npcs, shop, KNOWN_ITEMS)  # STAGE-326：药师店加安抚之素与覆盖买价
    print(f"宠物忠诚物品（STAGE-326）：改 {pet_shops} 家药师店的商品与买价")
    print(f"便利店覆盖表（STAGE-302）：改商品 {shop_changed} 家、新增 {shop_added} 家（NPC {added_npcs} 个）、"
          f"删除 {len(removed)} 个源 NPC（编号留空洞）")
    body = "\n".join("\t".join(row) for row in rows)
    out = OUT_DIR / "npcs.tsv"
    out.write_text(
        "# generated by tools/gen-npcs.py; DO NOT EDIT. "
        "NPC 数据表（STAGE-68，docs/protocol.md 5.32），只收录已实现和接下来要实现的类型。"
        "列：编号 地图 x y 原版类型 显示类型 名字（编号按收录批次、地图、坐标、类型、名字排序，从 1 开始；"
        "STAGE-69 的报时和摆设在 STAGE-68 全部类型之后）\n"
        f"{body}\n",
        encoding="utf-8",
    )
    talk_out = OUT_DIR / "npc-talk.json"
    talk_out.write_text(
        json.dumps(talk, ensure_ascii=False, indent=0, sort_keys=False) + "\n",
        encoding="utf-8",
    )
    npc_event_overlay.apply_event_patches(events)  # STAGE-333：poru 红暴块改成萨姆吉尔的勾玉
    event_out = OUT_DIR / "npc-event.json"
    event_out.write_text("[\n" + ",\n".join(json.dumps(e, ensure_ascii=False, separators=(",", ":")) for e in events)
                         + "\n]\n", encoding="utf-8")
    enemy_list = [enemy_record(i, n["enemy"], data, floors, ids)
                  for i, n in enumerate(npcs, 1) if n["kind"] == "npcenemy"]
    enemy_out = OUT_DIR / "npc-enemy.json"
    enemy_out.write_text("[\n" + ",\n".join(json.dumps(e, ensure_ascii=False, separators=(",", ":")) for e in enemy_list)
                         + "\n]\n", encoding="utf-8")
    write_json(riderman_records(data), OUT_DIR / "npc-riderman.json")  # 骑乘训练师的窗口（STAGE-122）
    shop_out = OUT_DIR / "npc-shop.json"
    shop_out.write_text(json.dumps(shop, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")
    healers = []
    for i, n in enumerate(npcs, 1):
        if n["kind"] == "npcgen_winhealer":
            if rec := healer_record(n["enemy"], data):
                healers.append({"id": i, **rec})
    healer_out = OUT_DIR / "npc-healer.json"
    healer_out.write_text(json.dumps(healers, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")
    saves = []
    for i, n in enumerate(npcs, 1):
        if n["kind"] == "npcgen_savepoint":
            if rec := savepoint_record(n["enemy"], data):
                saves.append({"id": i, **rec})
    save_out = OUT_DIR / "npc-savepoint.json"
    save_out.write_text(json.dumps(saves, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")
    petshops = []
    for i, n in enumerate(npcs, 1):
        if n["kind"] == "npcgen_petshop":
            if rec := petshop_record(n["enemy"], data):
                petshops.append({"id": i, **rec})
    petshop_out = OUT_DIR / "npc-petshop.json"
    petshop_out.write_text(json.dumps(petshops, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")
    petskillshops = []
    for i, n in enumerate(npcs, 1):
        if n["kind"] == "npcgen_petskillshop":
            if rec := petskillshop_record(n["enemy"], data):
                petskillshops.append({"id": i, **rec})
    petskillshop_out = OUT_DIR / "npc-petskillshop.json"
    petskillshop_out.write_text(json.dumps(petskillshops, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")
    warpmen = []
    for i, n in enumerate(npcs, 1):
        if n["kind"] == "npcgen_warpman":
            if rec := warpman_record(n["enemy"], data):
                warpmen.append({"id": i, **rec})
    warp_out = OUT_DIR / "npc-warpman.json"
    warp_out.write_text(json.dumps(warpmen, ensure_ascii=False, indent=0) + "\n", encoding="utf-8")
    skipped = sum(1 for w in warpmen if w.get("unsupported"))
    print(f"{data}/npc: 扫描 {files} 个创建文件，收录 {len(npcs)} 个 NPC → {out}，对话参数 {len(talk)} 条 → {talk_out}，"
          f"商店 {len(shop)} 家 → {shop_out}，护士 {len(healers)} 位 → {healer_out}，存档点 {len(saves)} 处 → {save_out}，"
          f"宠物店 {len(petshops)} 家 → {petshop_out}，宠物技能店 {len(petskillshops)} 家 → {petskillshop_out}，传送员 {len(warpmen)} 名（{skipped} 名依赖任务/家族，本期跳过）→ {warp_out}")
    missed = [EVENT_PATCHES[i]["note"] for i, c in enumerate(patch_hits) if c != 1]
    if missed:
        raise SystemExit(f"事件补丁没有恰好命中一次：{missed}")
    print(f"事件补丁 {len(patch_log)} 处（STAGE-280）：")
    for line in patch_log:
        print(f"    {line}")
    print_event_summary(events, event_out, event_notes)
    bad_enemy = sum(1 for e in enemy_list if e["unsupported"])
    print(f"战斗 NPC {len(enemy_list)} 个（支持 {len(enemy_list) - bad_enemy}，不支持 {bad_enemy}）→ {enemy_out}")
    for why, count in summarize(enemy_list).items():
        print(f"    {count:5d}  {why}")
    print(f"  丢弃：{dropped['other_type']} 个未收录类型、{dropped['bad_floor']} 个地图编号无效、"
          f"{dropped['no_born']} 个缺 born 位置、{dropped['no_enemy']} 个没有 enemy")
    for floor in (1000, 2000, 3000, 4000, 1006, 2006, 3006, 4006, 100, 200):
        on_floor = [n for n in npcs if n["floor"] == floor]
        kinds = {}
        for n in on_floor:
            kinds[n["type"]] = kinds.get(n["type"], 0) + 1
        print(f"  地图 {floor}: {len(on_floor)} 个 {kinds}")


if __name__ == "__main__":
    main()
