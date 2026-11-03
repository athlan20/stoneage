"""战斗 NPC（原版 functionset=NPCEnemy，STAGE-96，对照 gmsv/npc/npc_npcenemy.c）的参数转换。

NPC 创建文件里 enemy=模板名|file:参数文件 的模板，只要在某个 .template 块里是 functionset=NPCEnemy 就算战斗 NPC
（一个 .template 文件可以有好几个 {…} 块，逐块读）。参数文件是 key:value 行（GBK），这里把每个战斗 NPC 转成
npc-enemy.json 的一条记录，运行时由 server/gamedata/npcenemy.go 读取。

记录的字段（协议见 docs/protocol.md 5.33、10.3、10.9）：
- ask：askbattlemsg1..6，按行给出（窗口正文用换行连起来）；denied / already：拒绝和占用的台词；
- start / end：开战和赢了时 NPC 说的话（startmsg / endmsg）；
- enemies：敌人编号（enemy.txt）按顺序入场，最多 10 个；items：开战要全部持有的道具（item）；
- steal：前置道具什么时候收（-1 不收，0 开战时收，1 赢了才收）；one_battle：onebattle，同一个 NPC 同时只能一场；
- die_act：1 赢了传送到 warp，0 消失 revive_secs 秒（time，没写取 120）；
- unsupported 非空的运行时不放行：道场（gym）、分路传送（NEWNPCENEMY）、消失与复活（dieact=0，STAGE-D）、
  参数文件缺失、敌人不在敌人表、落点地图不存在，以及认不出的参数键。
"""

import re
from pathlib import Path

from npc_records import atoi, decode

# 第一期认识的参数键（小写）。认不出的键运行时不放行，免得漏掉原版的规则。
ASK_KEYS = tuple(f"askbattlemsg{i}" for i in range(1, 7))
TEXT_KEYS = {"deniedmsg": "denied", "alreadymsg": "already", "startmsg": "start", "endmsg": "end"}
KNOWN_KEYS = set(ASK_KEYS) | set(TEXT_KEYS) | {"entype", "dieact", "warpfl", "warpx", "warpy", "enemyno",
                                               "item", "steal", "onebattle", "time"}
# 道场和分路传送各有专门的原因（它们的参数键不在 KNOWN_KEYS 里）。
GYM_KEYS = ("gym", "enemypetno")
# 原版数据里的拼错键（"end msg:"、"start msg:"、"star tmsg:"）：原版按准确的键名读，这些行不起作用，忽略。
IGNORED_KEYS = {"end msg", "start msg", "star tmsg"}
ENEMY_LIMIT = 10  # 同一场最多 10 个敌人（原版 MAXENEMY 的 enemyno 个数上限）


def enemy_templates(data: Path) -> set[str]:
    """functionset=NPCEnemy 的模板名（小写）。一个 .template 文件里可以有好几个 {…} 块，逐块读。"""
    names: set[str] = set()
    for path in (data / "npc").rglob("*.template"):
        for body in re.findall(rb"\{([^{}]*)\}", path.read_bytes()):
            fields: dict[bytes, bytes] = {}
            for line in body.splitlines():
                key, sep, value = line.partition(b"=")
                if sep:
                    fields[key.strip().lower()] = value.strip()
            if fields.get(b"functionset", b"").lower() == b"npcenemy" and b"templatename" in fields:
                names.add(fields[b"templatename"].decode("ascii", "replace").lower())
    return names


def enemy_ids(enemies_tsv: Path) -> set[int]:
    """敌人表（server/gamedata/enemies.tsv，tools/gen-encounters.py 生成）里的敌人编号。"""
    ids = set()
    for line in enemies_tsv.read_text(encoding="utf-8").splitlines():
        if line and not line.startswith("#"):
            ids.add(int(line.split("\t", 1)[0]))
    return ids


def read_args(path: Path) -> dict[str, str]:
    """读一个参数文件：key:value 行，键小写；同名的取第一次出现（同 gmsv 的按键取值）。

    NEWNPCENEMY 是单独一行（没有冒号），记成键 newnpcenemy。NEWEVENTn 块（分路传送）到 OVER 为止整块跳过，
    块里的 FREE、WARP、endmsg 不是这个 NPC 的参数。
    """
    out: dict[str, str] = {}
    in_event = False
    for lineno, raw in enumerate(path.read_bytes().split(b"\n"), 1):
        line = decode(raw.rstrip(b"\r"), f"{path}:{lineno}").strip()
        if not line or line.startswith("#"):
            continue
        word = line.upper()
        if word == "OVER":
            in_event = False
            continue
        if word.startswith("NEWEVENT"):
            in_event = True
            continue
        if in_event:
            continue
        if word == "NEWNPCENEMY":
            out["newnpcenemy"] = ""
            continue
        key, sep, value = line.partition(":")
        if sep and key.strip().lower() not in IGNORED_KEYS:
            out.setdefault(key.strip().lower(), value.strip())
    return out


def enemy_record(npc_id: int, enemy_ref: str, data: Path, floors: set[int], ids: set[int]) -> dict:
    """把一个战斗 NPC 的 enemy 参数（模板名|file:参数文件）转成记录；参数缺失或不支持的写进 unsupported。"""
    _, _, arg = enemy_ref.partition("|")
    rel = arg.strip()[len("file:"):] if arg.strip().startswith("file:") else ""
    path = data / "npc" / rel
    reasons: list[str] = []
    a = read_args(path) if path.is_file() else {}
    if not a:
        reasons.append("参数文件缺失")
    unknown = sorted(k for k in a if k not in KNOWN_KEYS and k not in GYM_KEYS and k != "newnpcenemy"
                     and not k.startswith("newevent"))
    gym = any(k in a for k in GYM_KEYS)
    if gym:
        reasons.append("道场（未实现）")  # 道场的 enemyno 是候选，不按一场的阵容检查
    if "newnpcenemy" in a:
        reasons.append("分路传送（未实现）")
    if unknown:
        reasons.append("未实现的参数 " + ",".join(unknown))

    enemies = [atoi(v) for v in a.get("enemyno", "").split(",") if v.strip()]
    if not gym and (not enemies or len(enemies) > ENEMY_LIMIT or any(e not in ids for e in enemies)):
        reasons.append("敌人数据不完整")
    items = [atoi(v) for v in a.get("item", "").split(",") if v.strip()]
    steal = atoi(a["steal"]) if "steal" in a else -1
    die_act = atoi(a.get("dieact", "0"))
    if die_act == 0:
        reasons.append("消失与复活（未实现）")
    warp = None
    if die_act == 1:
        point = {"floor": atoi(a.get("warpfl", "")), "x": atoi(a.get("warpx", "")), "y": atoi(a.get("warpy", ""))}
        if point["floor"] not in floors:
            reasons.append("落点地图不存在")
        warp = point

    texts = {field: a.get(key, "") for key, field in TEXT_KEYS.items()}
    return {
        "id": npc_id,
        "file": rel[:-len(".arg")] if rel.endswith(".arg") else rel,
        "ask": [a[k] for k in ASK_KEYS if a.get(k)],
        **texts,
        "enemies": enemies,
        "items": items,
        "steal": steal,
        "one_battle": atoi(a.get("onebattle", "0")) == 1,
        "die_act": die_act,
        "warp": warp,
        "revive_secs": 0 if die_act == 1 else atoi(a.get("time", "120")),
        "unsupported": "；".join(reasons),
    }


def summarize(records: list[dict]) -> dict[str, int]:
    """不支持的原因（去掉数字后按出现次数排）。"""
    reasons: dict[str, int] = {}
    for rec in records:
        for why in filter(None, rec["unsupported"].split("；")):
            key = re.sub(r"[0-9]+", "N", why)
            reasons[key] = reasons.get(key, 0) + 1
    return dict(sorted(reasons.items(), key=lambda kv: -kv[1]))
