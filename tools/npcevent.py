"""任务 NPC（原版 enemy=changeevent）的事件脚本转换，STAGE-90，对照 gmsv/npc/npc_exchangeman.c。

原版脚本是 data/npc 下的普通文件（GBK）：第一行 NomalMainMsg（闲聊句，逗号分句），之后每个事件块
以 EventNo 开头、以 EventEnd 结束，块内是 key:value 行。这里把每块解析成 npc-event.json 的一条记录。
用到不支持的字段（宠物、EVDEL 等）的块标记 unsupported，
运行时跳过、不放行（同 STAGE-74 传送员的做法）。台词逐字保留，只把字面的 \\n 换成换行。
分页台词（STAGE-97）：AcceptMsg1..N 等按页序收进 msg.paged，窗口一页页翻（见 docs/quest.md）。
NpcWarp（STAGE-93）：`地图.x.y,地图.x.y,…` 转成位置列表，地图必须是真实存在的地图（传入的 floors）。
"""

import re

from npcevent_now import drop_now_groups
from npcevent_pet import convert_pets, parse_pet

# 块里认识的台词字段 → 记录里的键。
MSG_FIELDS = {
    "NomalMsg": "nomal", "NomalWindowMsg": "nomal_window",
    "AcceptMsg": "accept", "ThanksMsg": "thanks", "ItemFullMsg": "item_full",
    "RequestMsg": "request", "StopMsg": "stop", "EndStopMsg": "end_stop", "NoStopMsg": "no_stop",
    "StoneLessMsg": "stone_less", "StoneFullMsg": "stone_full",
    "CleanMainMsg": "clean_main", "CleanFlgMsg": "clean_flg", "PetFullMsg": "pet_full",
}
# 块里认识的动作和条件字段。
ACTION_FIELDS = ("TYPE", "EventNo", "EVENT", "GetItem", "DelItem", "GetStone", "DelStone",
                 "GetRandItem", "EndSetFlg", "CleanFlg", "Charm", "NpcWarp", "DelPet", "GetPet", "NotDel")
SUPPORTED_FIELDS = set(MSG_FIELDS) | set(ACTION_FIELDS)
BLOCK_TYPES = ("MESSAGE", "ACCEPT", "REQUEST", "CLEAN")
# 这些类型必须有的台词：ACCEPT/REQUEST 的交易写在 ThanksMsg 那一步里（原版 npc_exchangeman.c 的 MsgDisp 8/2），
# 没有 ThanksMsg 时原版什么都不做；ACCEPT/REQUEST 没有问话时弹出空窗口，交易永远走不到；CLEAN 没有问话就没有可做的事。
REQUIRED_MSG = {"ACCEPT": ("accept", "thanks"), "REQUEST": ("request", "thanks"), "CLEAN": ("clean_main",)}
# 暗号字段：只在玩家说出暗号时才考虑（原版 NPC_TypeCheck 的 szMes 比较），普通对话跳过；记录里 keyword 是暗号原文。
KEYWORD_FIELD = "KeyWord"
# 字段名后面直接是字面 \n、没有冒号的写法（AcceptMsg\n你…，4 块）：和 `AcceptMsg:\n你…` 的内容相同，只缺冒号。
MSG_NEWLINE_RE = re.compile(r"^(" + "|".join(MSG_FIELDS) + r")\\n")
# 分页台词（STAGE-97）：原版 AcceptMsg1..N 等，窗口按页翻（NEXT/PREV）。字段名 → 记录里 msg.paged 的键。
# 第 1 页是 Key 本身或 Key1，之后是 Key2、Key3…（原版两种写法都有）。其他台词（NomalMsg 等）没有分页写法。
PAGED_FIELDS = {"AcceptMsg": "accept", "ThanksMsg": "thanks", "RequestMsg": "request",
                "NomalWindowMsg": "nomal_window"}
PAGE_KEY_RE = re.compile(r"^([A-Za-z]+?)(\d+)$")

# 条件项：标记 比较符 数值 [*数量]。ITEM 的数值是道具编号，*数量只用于 =（NPC_EventReduce）。
COND_RE = re.compile(r"^(ITEM|LV|ENDEV|NOWEV|TIME|NOEEV)(!=|=|<|>)(\d+)(?:\*(\d+))?$")
# 原版 NPC_EventBigSmallCheck 用 getStringFromIndexWithDelim 取比较符第 2 段的 atoi 前缀，
# 后面的笔误（如 event30_1 的 `ENDEV=4ENDEV=29`）会被吞掉：实际只检查 ENDEV=4。严格正则不认时用它兜底。
LEGACY_COND_RE = re.compile(r"^(ITEM|LV|ENDEV|NOWEV|TIME)(!=|=|<|>)(\d+)")
# 每个标记允许的比较符（对照 NPC_EventFreeIfCheck 的分派）。NOWEV!= 在原版 NPC_NOWEventNoCheck 的 flg=3
# 分支里无论事件是否进行中都返回 TRUE（STAGE-321 核对），即恒为真，照搬。PET 见 npcevent_pet.py；
# SP、DR 不支持，DR 在原版里没有分派，恒不成立。NOEEV 原版不认，转换时记成 NEVER（STAGE-323）。
COND_CMPS = {"ITEM": ("=", "!="), "LV": ("=", "<", ">"), "ENDEV": ("=", "!="),
             "NOWEV": ("=", "!="), "TIME": ("!=",)}
ITEM_RE = re.compile(r"^(\d+)(?:\*(\d+))?$")


def parse_main(value: str) -> list[str]:
    """NomalMainMsg：逗号分句，原版随机说其中一句（npc_exchangeman.c 的 NPC_TypeCheck 末尾）。"""
    return [p.strip() for p in value.replace("\\n", "\n").split(",") if p.strip()]


def parse_event(value: str | None) -> tuple[list | None, str]:
    """EVENT：逗号分隔的候选组任一满足即可（OR），组内 & 连接（AND）。返回 (组列表, 不支持原因)。"""
    if value is None or not value.strip():
        return None, "EVENT 为空"
    groups = []
    for alt in value.strip().split(","):
        terms = []
        for raw in alt.split("&"):
            text = raw.strip()
            if text.startswith("PET"):
                pet, why = parse_pet(text, 1)
                if why:
                    return None, why
                terms.append({"op": "PET", "pet": pet})
                continue
            m = COND_RE.match(text) or LEGACY_COND_RE.match(text)
            if not m:
                return None, f"条件 {text or '（空）'} 不支持"
            if m.group(1) == "NOEEV":
                # STAGE-323：原版 NPC_EventFreeIfCheck 不认 NOEEV（C 源码里没有这个标记），条件恒不成立。
                # 照原版处理：记成 NEVER（运行时恒假），不再拒绝整个脚本。
                terms.append({"op": "NEVER"})
                continue
            if m.group(2) not in COND_CMPS[m.group(1)]:
                return None, f"条件 {text or '（空）'} 不支持"
            op, cmp_, num = m.group(1), m.group(2), int(m.group(3))
            count = m.group(4) if m.re.groups >= 4 else None
            if count and (op != "ITEM" or cmp_ != "="):
                return None, f"条件 {text} 不支持"
            if op == "ITEM":
                terms.append({"op": op, "cmp": cmp_, "id": num, "n": int(count) if count else 1})
            else:
                terms.append({"op": op, "cmp": cmp_, "n": num})
        groups.append(terms)
    return groups, ""


def parse_items(kind: str, value: str, known: set[int], allow_unknown: bool = False) -> tuple[list | None, str]:
    """GetItem / DelItem：`编号` 或 `编号*数量`，逗号分隔。GetItem 不写数量时给 1 件；DelItem 不写数量时
    原版删掉背包里所有这个编号的道具（NPC_EventDelItem），记录里 n 为 0 表示全部。
    STAGE-219：DelItem 末尾多一个逗号（19633,…,19645,）时只跳过最后一个空项：原版把空串当编号 0 处理，背包里没有，
    什么也不做。GetItem 的末尾空项不跳过：原版 NPC_EventAddItem 对它做 ITEM_makeItemAndRegist(0)，失败后中途返回，
    前面的道具已经给了，没法照搬，仍标不支持。中间的空项两种都不支持。"""
    parts = value.split(",")
    if kind == "DelItem" and len(parts) > 1 and not parts[-1].strip():
        parts.pop()
    out = []
    for raw in parts:
        m = ITEM_RE.match(raw.strip())
        if not m:
            return None, f"{kind}:{value} 格式不支持"
        iid = int(m.group(1))
        if iid not in known and not allow_unknown:
            return None, f"{kind} 的道具 {iid} 不在道具表里"
        n = int(m.group(2)) if m.group(2) else (0 if kind == "DelItem" else 1)
        out.append({"id": iid, "n": n})
    return out, ""


def parse_int_list(kind: str, value: str, known: set[int] | None = None) -> tuple[list | None, str]:
    """EndSetFlg / CleanFlg / GetRandItem：逗号分隔的整数（GetRandItem 的编号要在道具表里）。"""
    out = []
    for raw in value.split(","):
        text = raw.strip()
        if not re.fullmatch(r"-?\d+", text):
            return None, f"{kind}:{value} 格式不支持"
        num = int(text)
        if known is not None and num not in known:
            return None, f"{kind} 的道具 {num} 不在道具表里"
        out.append(num)
    return out, ""


def parse_stone_cost(value: str) -> tuple[dict | None, str]:
    """DelStone：`数字`（固定金币）或 `LV*数字`（等级 × 数字，NPC_EventGetCost）。"""
    text = value.strip()
    if re.fullmatch(r"\d+", text):
        return {"fixed": int(text)}, ""
    m = re.fullmatch(r"LV\*(\d+)", text)
    if m:
        return {"per_lv": int(m.group(1))}, ""
    return None, f"DelStone:{text} 格式不支持"


WARP_RE = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")


def parse_warp(value: str, floors: set[int]) -> tuple[list | None, str]:
    """NpcWarp：`地图.x.y` 逗号分隔（STAGE-93，对照 npc_exchangeman.c 的 NPC_EventWarpNpc）。
    地图必须存在；坐标按原文保存，不查地图边界（原版也不查）。"""
    out = []
    for raw in value.split(","):
        m = WARP_RE.match(raw.strip())
        if not m:
            return None, f"NpcWarp:{raw.strip() or '（空）'} 格式不支持"
        floor = int(m.group(1))
        if floor not in floors:
            return None, f"NpcWarp 的地图 {floor} 不存在"
        out.append({"floor": floor, "x": int(m.group(2)), "y": int(m.group(3))})
    return out, ""


def page_of(key: str) -> tuple[str, int] | None:
    """分页台词的字段：AcceptMsg2 → ("AcceptMsg", 2)；不是 PAGED_FIELDS 里的台词返回 None。"""
    m = PAGE_KEY_RE.match(key)
    if m and m.group(1) in PAGED_FIELDS:
        return m.group(1), int(m.group(2))
    return None


def collect_pages(fields: dict[str, str]) -> tuple[dict[str, list[str]], str]:
    """块里的分页台词：返回 ({记录键: 全部页}, 不支持原因)。第 1 页是 Key 本身（有 Key2 起的分页时）或 Key1，
    之后的页号必须连续；没有分页的台词不在结果里（走普通的单页字段）。"""
    nums: dict[str, list[int]] = {}
    for key in fields:
        hit = page_of(key)
        if hit:
            nums.setdefault(hit[0], []).append(hit[1])
    out: dict[str, list[str]] = {}
    for base, ns in sorted(nums.items()):
        ns.sort()
        # STAGE-323：原版页序是 base（第 1 页）、base1（第 2 页）、base2（第 3 页）……（npc_exchangeman.c
        # 的 case 9 / case 99）。原文漏页时（garunaev3 的 AcceptMsg + AcceptMsg2），原版取不到中间页，
        # token 保留上一页，所以缺页沿用上一页的内容。
        pages = [fields[base]] if base in fields else []
        for n in range(1, max(ns) + 1):
            if f"{base}{n}" in fields:
                pages.append(fields[f"{base}{n}"])
            elif pages:
                pages.append(pages[-1])
        if not pages:
            return {}, f"{base} 的分页不连续"
        out[PAGED_FIELDS[base]] = [p.replace("\\n", "\n") for p in pages]
    return out, ""


def convert_block(lines: list[str], known: set[int], floors: set[int]) -> dict:
    """把一个块（已去掉空行的 key:value 行）转成记录；有不支持的内容时带 unsupported 原因。"""
    fields: dict[str, str] = {}
    why = ""
    last_msg = None  # 上一行是台词字段时记下它的键：紧跟的没有冒号的行是它的续行（原文断行，见下）
    for line in lines:
        if line.startswith("#"):
            why = why or "块里有注释行"
            last_msg = None
            continue
        if m := MSG_NEWLINE_RE.match(line):
            line = f"{m.group(1)}:{line[len(m.group(1)):]}"
        key, sep, value = line.partition(":")
        key = key.strip()
        if not sep and last_msg is not None:
            fields[last_msg] += line  # 台词在原文里断成两行（23 块）：第二行直接接在第一行后面（原版只读第一行）
            continue
        last_msg = None
        if not sep:
            why = why or f"无法识别的行 {line[:12]}"
        elif key in fields:
            why = why or f"重复的字段 {key}"
        elif key not in SUPPORTED_FIELDS | {KEYWORD_FIELD} and page_of(key) is None:
            why = why or f"不支持的字段 {key}"
        else:
            fields[key] = value
            if key in MSG_FIELDS:
                last_msg = key
    # STAGE-219：含 NOW=n 的候选组恒不成立，先丢掉（见 npcevent_now.py）。EVDEL 的分组也用丢掉之后的 EVENT，
    # 和条件的序号对得上。
    now_dropped: list[str] = []
    if "EVENT" in fields:
        fields["EVENT"], now_dropped = drop_now_groups(fields["EVENT"])
    paged, page_why = collect_pages(fields)
    why = why or page_why
    rec: dict = {}
    kind = fields.get("TYPE", "").strip()
    if kind not in BLOCK_TYPES:
        why = why or f"TYPE {kind or '（空）'} 不支持"
    rec["type"] = kind
    ev = fields.get("EventNo", "").strip()
    if re.fullmatch(r"-?\d+", ev):
        rec["event"] = int(ev)
    else:
        rec["event"] = -1
        why = why or "EventNo 缺失或格式不对"
    cond, cond_why = parse_event(fields.get("EVENT"))
    why = why or cond_why
    rec["cond"] = cond or []
    # 有分页的台词只写 paged（全部页），不再写单页字段，免得同一句话存两份。
    msg = {name: fields[key].replace("\\n", "\n") for key, name in MSG_FIELDS.items()
           if key in fields and name not in paged}
    if paged:
        msg["paged"] = paged
    for name in REQUIRED_MSG.get(kind, ()):
        if name not in msg and name not in paged:
            why = why or f"{kind} 缺少 {name}"
    if KEYWORD_FIELD in fields:
        # 暗号逐字保留（原版 strcmp，区分大小写和全半角）；空暗号没法匹配（玩家说的话不会是空的），标不支持。
        rec["keyword"] = fields[KEYWORD_FIELD]
        if not rec["keyword"]:
            why = why or "KeyWord 为空，没法匹配"
    if msg:
        rec["msg"] = msg
    # STAGE-324：GetItem 引用的道具原版表里也没有时（event20_2 的 2745），原版 NPC_EventAddItem 的
    # ITEM_makeItemAndRegist 返回 -1，整个块的交易失败、什么都不改。这里保留编号并标 missing_item，
    # 脚本仍可整本放开，运行时按原版让这块交易失败（见 server/character/npcevent_deal.go）。
    if "GetItem" in fields:
        got, err = parse_items("GetItem", fields["GetItem"], known, allow_unknown=True)
        why = why or err
        if got:
            rec["get_item"] = got
            if any(it["id"] not in known for it in got):
                rec["missing_item"] = True
    # STAGE-324：DelItem 引用道具表里没有的编号时，原版 NPC_EventDelItem 在背包里找不到（道具表没有就
    # 不可能持有），是空操作，不会失败。保留编号、不标不支持。
    actions = (
        # STAGE-322：DelItem 引用道具表里没有的编号（如 event30_1、giruev2_1 的 2391）时，原版
        # NPC_EventDelItem 在背包里找不到（道具表没有就不可能持有），是空操作，不标不支持。
        ("DelItem", "del_item", lambda v: parse_items("DelItem", v, known, allow_unknown=True)),
        ("GetRandItem", "get_rand", lambda v: parse_int_list("GetRandItem", v, known)),
        ("EndSetFlg", "end_set", lambda v: parse_int_list("EndSetFlg", v)),
        ("CleanFlg", "clean", lambda v: parse_int_list("CleanFlg", v)),
    )
    for key, name, parse in actions:
        if key in fields and not (key == "DelItem" and fields[key].strip() == "EVDEL"):  # EVDEL 见 convert_pets
            got, err = parse(fields[key])
            why = why or err
            if got:
                rec[name] = got
    why = why or convert_pets(fields, rec, kind, known)  # 宠物（STAGE-197，见 npcevent_pet.py）
    if "GetStone" in fields:
        if re.fullmatch(r"\d+", fields["GetStone"].strip()):
            rec["get_stone"] = int(fields["GetStone"].strip())
        else:
            why = why or f"GetStone:{fields['GetStone']} 格式不支持"
    if "DelStone" in fields:
        cost, err = parse_stone_cost(fields["DelStone"])
        why = why or err
        if cost:
            rec["del_stone"] = cost
    # REQUEST 的交易（接任务时的 GetItem / DelItem / DelStone）放弃时撤销：收回给的道具，退回收走的道具和石币
    # （server/character/npcevent_refund.go）。随机道具和给石币的撤销没有对应的原版行为，仍不支持。
    if kind == "REQUEST" and any(k in rec for k in ("get_rand", "get_stone")):
        why = why or "REQUEST 带随机道具或给金币，放弃时的撤销未实现"
    if "Charm" in fields:
        if re.fullmatch(r"\d+", fields["Charm"].strip()):
            rec["charm"] = int(fields["Charm"].strip())
        else:
            why = why or f"Charm:{fields['Charm']} 格式不支持"
    if "NpcWarp" in fields:
        warp, err = parse_warp(fields["NpcWarp"], floors)
        why = why or err
        if warp:
            rec["warp"] = warp
    if why:
        rec["unsupported"] = why
    if now_dropped:
        # 给 parse_script 用的标记（不写进 JSON）：丢掉了哪些组；全部候选组都丢掉时整块省略。
        rec["_now"] = {"dropped": now_dropped, "omit": not fields["EVENT"]}
    return rec


def finish_blocks(recs: list[dict]) -> tuple[list[dict], list[str]]:
    """去掉 convert_block 留下的 _now 标记（STAGE-219）：丢掉过 NOW 组的块记一条说明；候选组全是 NOW 组的块省略
    （原版永远选不中，省略和原版等价；块的选择只看条件，不看序号，省略不影响别的块）。块序号是脚本里的原序号，从 1 数。"""
    blocks: list[dict] = []
    notes: list[str] = []
    for no, rec in enumerate(recs, 1):
        now = rec.pop("_now", None)
        if now is None:
            blocks.append(rec)
            continue
        dropped = "；".join(now["dropped"])
        if now["omit"]:
            notes.append(f"块 {no} 省略（候选组全是 NOW 组）：{dropped}")
            continue
        notes.append(f"块 {no} 丢掉 NOW 组：{dropped}")
        blocks.append(rec)
    return blocks, notes


def has_event_no(lines: list[str]) -> bool:
    """段里有没有 EventNo 行。原版 npc_exchangeman.c 的选块循环对没有 EventNo 的段直接 continue，不算块。"""
    return any(line.partition(":")[0].strip() == "EventNo" for line in lines)


def parse_script(text: str, known: set[int], floors: set[int]) -> tuple[dict, list[str]]:
    """整个脚本：({"main": [闲聊句], "blocks": [块记录]}, 转换说明)。块保持文件顺序（原版按顺序选第一个命中的块）。

    STAGE-223，对照 gmsv/util.c 的 getStringFromIndexWithDelim_body 和 npc_exchangeman.c 的选块循环：
    原版按子串找 EventEnd，所以 EventEnd 后面同一行的文字属于下一段（poru/titan 的最后一块就是这样，
    EventEnd 后面接了一句无关的话，那段没有 EventNo，原版跳过）。这里先把 EventEnd 拆成单独一行，再按行处理；
    文件末尾没有 EventEnd 的块原版读到文件结束（最后一段也返回 TRUE），这里同样当作块结束。
    没有 EventNo 的段原版跳过，这里也跳过，并在转换说明里记一条（段号从 1 数，和原版的段序号一致）。"""
    main: list[str] = []
    recs: list[dict] = []
    skipped: list[str] = []
    cur: list[str] | None = None
    seg = 0

    def close(lines: list[str]) -> None:
        nonlocal seg
        seg += 1
        if has_event_no(lines):
            recs.append(convert_block(lines, known, floors))
        elif any(not line.startswith("#") for line in lines):
            skipped.append(f"段 {seg} 没有 EventNo，原版跳过：{lines[0][:20]}")

    for raw in text.replace("EventEnd", "\nEventEnd\n").splitlines():
        line = raw.strip()
        if line == "EventEnd":
            if cur is not None:
                close(cur)
                cur = None
            continue
        if not line:
            continue
        if cur is None:
            if line.startswith("#"):
                continue
            key, _, value = line.partition(":")
            if key.strip() == "NomalMainMsg" and not main:
                main = parse_main(value)
                continue
            cur = []
        cur.append(line)
    if cur is not None:  # 文件末尾没有 EventEnd 的段（STAGE-223）
        close(cur)
    blocks, notes = finish_blocks(recs)
    return {"main": main, "blocks": blocks}, skipped + notes


def event_record(enemy_file: str, text: str, known: set[int], floors: set[int]) -> tuple[dict, list[str]]:
    """npc-event.json 的一条（不含 id）：脚本文件相对 data/npc 的路径，加上解析结果；第二个返回值是转换说明
    （前面加上脚本文件名，给 gen-npcs.py 汇总打印，不写进 JSON）。"""
    script, notes = parse_script(text, known, floors)
    return {"file": enemy_file, **script}, [f"{enemy_file} {note}" for note in notes]
