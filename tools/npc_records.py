"""NPC 参数记录（STAGE-69 起）：从创建文件的 enemy 参数读出台词和配置，供 tools/gen-npcs.py 调用。

对话（npcgen_man、npcgen_signboard、npcgen_timeman）、商店、护士、存档点、宠物店、传送员的参数格式对照
gmsv/npc/ 下对应的 C 源码，各记录的字段见 server/gamedata 里的 npc-*.json。
"""

import re
import sys
from pathlib import Path

OUT_DIR = Path(__file__).resolve().parent.parent / "server/gamedata"


# 有对话参数、要生成 npc-talk.json 记录的类型（摆设没有参数，不在其中）。
TALK_KINDS = {"npcgen_man": "lines", "npcgen_signboard": "text", "npcgen_timeman": "timeman"}

# 护士（STAGE-71）：治疗参数 level|hp_rate|mp_rate|range（npc_windowhealer.c，range 不用）。
# 商店台词参数 → 记录字段（npc_itemshop.c 读取的 key，逐字保留）。
SHOP_MSGS = {"main_msg": "main", "buy_main": "buy_main", "sell_main": "sell_main",
             "what_msg": "what", "level_msg": "level", "realy_msg": "realy",
             "itemfull_msg": "itemfull", "stone_msg": "stone"}


def atoi(text: str) -> int:
    """与 C 的 atoi 相同：取开头的整数，开头不是数字时为 0。"""
    m = re.match(r"\s*([+-]?\d+)", text)
    return int(m.group(1)) if m else 0


def decode(raw: bytes, where: str) -> str:
    try:
        return raw.decode("gbk")
    except UnicodeDecodeError as e:
        print(f"警告：{where} 不是完整的 GBK 文本（{e}），坏字节用替换字符保留", file=sys.stderr)
        return raw.decode("gbk", errors="replace")


def decode_name(raw: bytes, where: str) -> str:
    """NPC 名字（name= 行）：绝大多数是 GBK；sb_gd、sb_nusu、sb_hag 是 Big5，sb_sinsa 带私用区字形、要 GB18030（STAGE-96）。"""
    for codec in ("gbk", "big5", "gb18030"):
        try:
            return raw.decode(codec)
        except UnicodeDecodeError:
            pass
    print(f"警告：{where} 的名字既不是 GBK 也不是 Big5，坏字节用替换字符保留", file=sys.stderr)
    return raw.decode("gbk", errors="replace")


def talk_arg(enemy: str, data: Path) -> str:
    """取 enemy 参数的参数段（模板名|参数），`file:genout/xxx` 读回原文件（GBK）。

    引用的文件缺失时返回空串（原版加载器也会跳过，那份 2.5 数据快照里偶尔缺文件），
    生成的表照常收录这个 NPC，运行时按没有参数的「……」处理。
    """
    _, _, arg = enemy.partition("|")
    arg = arg.strip()
    if arg.startswith("file:"):
        path = data / "npc" / arg[len("file:"):]
        if not path.is_file():
            return ""
        return decode(path.read_bytes(), str(path))
    return arg


def split_lines(text: str) -> list[str]:
    """按半角逗号分成多句，同原版 npc_townpeople.c（空段丢弃，空白收紧成一句话）。"""
    out = []
    for part in text.split(","):
        part = part.strip()
        if part:
            out.append(part)
    return out


def parse_timeman(text: str) -> dict | None:
    """解析 tman 参数文件的 `key:value` 行（npc_timeman.c 的 TimeManInit）。

    只收 time、main_msg、change_msg；change_no 是变身图形，新服务端不用。
    time 不认识或没有 main_msg/change_msg 的返回 None，运行时按「……」处理。
    """
    fields = {}
    for line in text.splitlines():
        key, sep, value = line.strip().partition(":")
        if sep:
            fields[key.strip().lower()] = value.strip()
    if "time" not in fields:
        return None
    record: dict = {"time": fields["time"]}
    if "main_msg" in fields:
        record["lines"] = split_lines(fields["main_msg"])
    if "change_msg" in fields:
        record["change"] = split_lines(fields["change_msg"])
    if "lines" not in record and "change" not in record:
        return None
    return record


def talk_record(kind: str, enemy: str, data: Path) -> dict | None:
    """一个 NPC 块的对话参数（npc-talk.json 的一条记录），没有内容返回 None。"""
    if kind not in TALK_KINDS:
        return None
    arg = talk_arg(enemy, data)
    if not arg:
        return None
    shape = TALK_KINDS[kind]
    if shape == "timeman":
        return parse_timeman(arg)
    if shape == "text":  # 告示牌：原文保存，`\n` 保持字面，协议发送时才转成换行
        text = arg.strip()
        return {"text": text} if text else None
    lines = split_lines(arg)  # 村民：按逗号分句
    return {"lines": lines} if lines else None


def load_known_items() -> set[int]:
    """物品编号集合（itemset.tsv 第一列），商店商品的有效性按它过滤。"""
    known = set()
    path = OUT_DIR / "itemset.tsv"
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line[0] == "#":
            continue
        known.add(atoi(line.split("\t", 1)[0]))
    return known


KNOWN_ITEMS = load_known_items()


def warpman_record(enemy: str, data: Path) -> dict | None:
    """传送员参数（npc-warpman.json 的一条记录），对照 gmsv/npc/npc_warpman.c（NEWWARPMAN）。

    WARP 是落点 `floor,x,y`；FREE 是通行条件（逗号分隔的候选组任一满足即可，组内 `&`
    连接；`ALLFREE` 恒真）；MONEY 是免费条件不满足时的费用（数字、`LV*N` 按等级算、
    `-1` 表示不能付费进入）；DelItem 是传送成功后收走的门票编号（角色持有其一）。
    条件标记只支持 LV/TRANS/ITEM（本期的范围）；ENDEV/NOWEV（任务事件）、MANOR（庄园）、
    FM（家族）等标记的数据 unsupported=true，运行时跳过、不放行。台词逐字保留。
    """
    fields = shop_fields(talk_arg(enemy, data))
    warp = fields.get("warp", "")
    f = [atoi(x) for x in warp.split(",")]
    if len(f) != 3:
        return None
    record: dict = {"warp": f}
    if fields.get("free"):
        record["free"] = fields["free"]
    if fields.get("money"):
        record["money"] = fields["money"]
    if fields.get("delitem"):
        record["del_item"] = [atoi(x) for x in fields["delitem"].split(",") if atoi(x)]
    for param, key in (("freemsg", "free_msg"), ("paymsg", "pay_msg"), ("moneymsg", "money_msg"),
                       ("partymsg", "party_msg"), ("nomalmsg", "nomal")):
        if fields.get(param):
            record[key] = fields[param]
    record["unsupported"] = not free_supported(record.get("free", ""))
    return record


def free_supported(free: str) -> bool:
    """FREE 条件是否全部由本期支持的标记组成（对照 server/gamedata/warpcond.go 的 passCond）。

    支持：LV、TRANS（任意比较）、ITEM（= 和 !=）、ENDEV（= 和 !=）、NOWEV（只有 =）、ALLFREE；
    空条件视为支持（没有 FREE 的传送员直接可传）。引用庄园、家族，或者 ITEM/ENDEV/NOWEV 用了别的比较符、
    ENDEV/NOWEV 带上限的，返回 False，运行时跳过以免误放行。
    """
    tokens = set(re.findall(r"[A-Z]+", free))
    if free == "" or "ALLFREE" in tokens:
        return True
    if not tokens <= {"LV", "TRANS", "ITEM", "ENDEV", "NOWEV", "AND"}:
        return False
    if re.search(r"ITEM(?:<|>)|NOWEV(?!=)|ENDEV(?:<|>)", free):
        return False
    return not re.search(r"(?:ENDEV|NOWEV|ITEM)[!=<>]+\d+-", free)


def petshop_record(enemy: str, data: Path) -> dict | None:
    """宠物店参数（npc-petshop.json 的一条记录），对照 gmsv/npc/npc_petshop.c。

    nomal_rate 是卖宠倍率、special_rate 配 special_pet（高价回收的模板编号列表）；
    pool_flg=1 的店支持寄放/领取（pool_cost 参数原版注释掉了没有使用，寄放费是
    50 + 等级×4，见 NPC_GETPOOLCOST 宏）；台词逐字保留。
    """
    fields = shop_fields(talk_arg(enemy, data))
    record: dict = {}
    for key in ("nomal_rate", "special_rate"):
        if fields.get(key):
            record[key] = float(fields[key])
    if fields.get("special_pet"):
        record["special_pet"] = [atoi(x) for x in fields["special_pet"].split(",") if atoi(x)]
    record["pool"] = atoi(fields.get("pool_flg", "")) == 1
    for param, key in (("main_msg", "main"), ("realy_msg", "realy"), ("over_msg", "over"),
                       ("thanks_msg", "thanks"), ("cost_msg", "cost"), ("pooltanks_msg", "poolthanks"),
                       ("poolfull_msg", "poolfull"), ("getfull_msg", "getfull")):
        if fields.get(param):
            record[key] = fields[param]
    return record


def petskillshop_record(enemy: str, data: Path) -> dict | None:
    """宠物技能店参数（npc-petskillshop.json 的一条记录），对照 gmsv/npc/npc_petskillshop.c。

    pet_skill 是这家店教的技能编号（逗号分隔，原版按顺序上架，行首 T 的非法技能在运行时跳过）；
    skill_rate 是学习价格的倍率（原版 atof，缺省 1.0）；main_msg 是欢迎语，逐字保留（`\\n` 保持字面）。
    没有 pet_skill 的参数文件返回 None，运行时这家店不上架任何技能。
    """
    fields = shop_fields(talk_arg(enemy, data))
    if not fields.get("pet_skill"):
        return None
    record: dict = {
        "skill_rate": float(fields["skill_rate"]) if fields.get("skill_rate") else 1.0,
        # 0 号（待机）是合法技能，原版照常上架；不能按“0 为假”把它过滤掉。
        "skills": [atoi(x) for x in fields["pet_skill"].split(",") if x.strip()],
    }
    if fields.get("main_msg"):
        record["main"] = fields["main_msg"]
    return record


def savepoint_record(enemy: str, data: Path) -> dict | None:
    """存档点参数（npc-savepoint.json 的一条记录），对照 gmsv/npc/npc_savepoint.c。

    参数文件（genout/sp_*）的 key:value：Born 是战败送回的落点 `floor,x,y`；
    GetItem 是记录要交的物品：逗号分隔多个候选，候选之间任一满足即可（原版 NPC_UsedCheck
    的 OR）；候选里 `&` 连接的几件都要有、收物品时一起收（NPC_AndReduce 的 AND 组）。
    每件是 `编号*数量`，缺数量的写法按数量 0 处理——原版 NPC_SavePointItemCheck 数到 0 永远
    不成立，所以含它的组整组丢弃（数据里有 sp_200_449_982 这种写法）。
    台词逐字保留（RequestMsg 要物品、OkMsg 记录成功、RealyMsg 收走物品、NomalMsg 已记录过；
    参数文件里的键是 RealyMsg / NomalMsg 这两个拼写，21 个文件全部一致）。
    """
    fields = shop_fields(talk_arg(enemy, data))
    born = fields.get("born", "")
    f = born.split(",")
    if len(f) != 3:
        return None
    groups = []
    for part in fields.get("getitem", "").split(","):
        if not part.strip():
            continue
        group = []
        for member in part.split("&"):
            no, star, cnt = member.strip().partition("*")
            group.append({"item": atoi(no), "count": atoi(cnt) if star else 1})
        if all(m["item"] > 0 and m["count"] > 0 for m in group):
            groups.append(group)
    record = {"born": [atoi(f[0]), atoi(f[1]), atoi(f[2])]}
    if groups:
        record["groups"] = groups
    for key, name in (("requestmsg", "request"), ("okmsg", "ok"), ("realymsg", "really"), ("nomalmsg", "normal")):
        if fields.get(key):
            record[name] = fields[key]
    return record


def healer_record(enemy: str, data: Path) -> dict | None:
    """护士的治疗参数（npc-healer.json 的一条记录）：level|hp_rate|mp_rate|range。

    对照 npc_windowhealer.c 的 NPC_WindowHealerInit：level 是免费线的等级（角色等级低于它免费），
    hp_rate/mp_rate 是每级费用倍率（费用 = 角色等级 × 倍率，向下取整，至少 1）；
    range 是走近自动弹窗的距离，新服务端不自动弹窗，不收。
    """
    arg = talk_arg(enemy, data).strip()
    if not arg:
        return None
    parts = [atoi(p) if "." not in p else float(p) for p in arg.split("|")]
    if len(parts) < 3 or not parts[0]:
        return None
    return {"level": int(parts[0]), "hp_rate": float(parts[1]), "mp_rate": float(parts[2])}


def shop_fields(text: str) -> dict[str, str]:
    """商店参数的 `key:value` 行（同 tman 的格式，npc_itemshop.c 用 NPC_Util_GetStrFromStrWithDelim 读）。"""
    fields = {}
    for line in text.splitlines():
        key, sep, value = line.strip().partition(":")
        if sep:
            fields[key.strip().lower()] = value.strip()
    return fields


def expand_itemlist(text: str, known: set[int], limit: int = 60) -> list[int]:
    """展开 ItemList（逗号分隔的编号或 `A-B` 范围），只留物品表里有的，上限 60（STAGE-302 由原版 MAXSHOPITEM=33 提高）。

    范围端点大小写照原版处理（start > end 时交换）；单个编号也要在物品表里（原版按
    ITEM_getNameFromNumber 是否为空过滤）。
    """
    out: list[int] = []
    for part in text.split(","):
        part = part.strip()
        if not part:
            continue
        if "-" in part:
            a, _, b = part.partition("-")
            start, end = atoi(a), atoi(b)
            if start > end:
                start, end = end, start
            for item_id in range(start, end + 1):
                if item_id in known:
                    out.append(item_id)
                    if len(out) >= limit:
                        return out
        elif (item_id := atoi(part)) in known:
            out.append(item_id)
            if len(out) >= limit:
                return out
    return out


def shop_record(kind: str, enemy: str, data: Path) -> dict | None:
    """一个商店 NPC 的买卖参数（npc-shop.json 的一条记录），没有商品也没有回收价返回 None。"""
    fields = shop_fields(talk_arg(enemy, data))
    record: dict = {}
    for key in ("buy_rate", "sell_rate"):
        if fields.get(key):
            record[key] = float(fields[key])
    record["items"] = expand_itemlist(fields.get("itemlist", ""), KNOWN_ITEMS)
    for param, key in SHOP_MSGS.items():
        if fields.get(param):
            record[key] = fields[param]
    if fields.get("special_item"):
        record["special_items"] = expand_itemlist(fields["special_item"], KNOWN_ITEMS)
        if fields.get("special_rate"):
            record["special_rate"] = float(fields["special_rate"])
    if record["items"] or record.get("sell_rate"):
        return record
    return None
