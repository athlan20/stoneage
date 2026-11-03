"""任务脚本里的宠物写法（STAGE-197，对照 gmsv/npc/npc_exchangeman.c）：宠物条件 PET、收宠物 DelPet（直接写和 EVDEL 两种）、
送宠物 GetPet、暗号无关的 NotDel 与 DelItem:EVDEL。转换结果写进 npc-event.json 的块记录，运行时在
server/character/npcevent_pet*.go 执行，规则见 docs/quest.md。

- PET 条件：PET<比较符><等级>-<宠物编号>[*数量]，比较的是宠物等级（NPC_PetLvCheck）；数量缺省在条件里是 1，在 DelPet 里是 0
  （全部收走，但仍要求至少 1 只）。带 EV 的变种（只数任务宠物）不支持。
- DelPet:EVDEL：收走 EVENT 里命中的那一组（逗号分的第几项）里的 PET 项；DelItem:EVDEL 同理收道具，NotDel 的编号排除。
- GetPet：怪物编号（enemy.txt 的 ENEMY_ID）逗号分隔，随机选一个；块里必须有 PetFullMsg。
"""

import re

# 宠物项：PET<比较符><等级>-<宠物编号>[*数量]。
PET_RE = re.compile(r"^PET(=|<|>)(\d+)-(\d+)(?:\*(\d+))?$")
# 道具项（EVDEL 里用）：ITEM=编号[*数量]，没有数量时收走背包里所有这个编号的道具（NPC_EventDelItemEVDEL）。
EV_ITEM_RE = re.compile(r"^ITEM=(\d+)(?:\*(\d+))?$")


def parse_pet(text: str, default: int) -> tuple[dict | None, str]:
    """宠物条件或收宠物的一项。数量缺省时用 default：条件是 1（至少 1 只），DelPet 是 0（全部收走，
    但仍要求至少有 1 只，NPC_PetLvCheck 的 loop 缺省 1）。带 EV 的变种（只数任务宠物）不支持。"""
    if "EV" in text:
        return None, f"条件 {text} 是 EV 变种（只数任务宠物），不支持"
    m = PET_RE.match(text)
    if not m:
        return None, f"条件 {text or '（空）'} 不支持"
    count = int(m.group(4)) if m.group(4) else default
    if m.group(4) is not None and count == 0:
        return None, f"条件 {text} 的数量是 0，不支持"
    return {"cmp": m.group(1), "level": int(m.group(2)), "template": int(m.group(3)), "count": count}, ""


def parse_pet_list(value: str, default: int) -> tuple[list | None, str]:
    """DelPet 的直接写法：逗号分隔的宠物项，每一项都要求有效。"""
    out = []
    for raw in value.split(","):
        pet, why = parse_pet(raw.strip(), default)
        if why:
            return None, why
        out.append(pet)
    return out, ""


def event_groups(event: str | None) -> tuple[list[list[str]] | None, str]:
    """EVENT 按逗号分候选组，组内按 & 分项；返回每组的原始项文本（EVDEL 要按原文看有没有数量）。"""
    if event is None or not event.strip():
        return None, "EVENT 为空"
    return [[raw.strip() for raw in alt.split("&")] for alt in event.strip().split(",")], ""


def del_pet_ev(event: str | None) -> tuple[list | None, str]:
    """DelPet:EVDEL：每个候选组一份宠物项（没有 PET 项的组是空列表），运行时取命中的那一组。"""
    groups, why = event_groups(event)
    if why:
        return None, why
    out = []
    for terms in groups:
        pets = []
        for text in terms:
            if not text.startswith("PET"):
                continue
            pet, err = parse_pet(text, 0)
            if err:
                return None, err
            pets.append(pet)
        out.append(pets)
    return out, ""


def del_item_ev(event: str | None, not_del: set[int], known: set[int]) -> tuple[list | None, str]:
    """DelItem:EVDEL：每个候选组一份道具（{id, n}，n 为 0 表示收走全部），编号在 not_del 里的不收（NotDel）。
    原版对 ITEM!= 也会删（取 = 后面的编号，NPC_EventDelItemEVDEL），这个写法的意思不明，标不支持。"""
    groups, why = event_groups(event)
    if why:
        return None, why
    out = []
    for terms in groups:
        items = []
        for text in terms:
            if not text.startswith("ITEM"):
                continue
            if text.startswith("ITEM!="):
                return None, f"EVDEL 的条件 {text} 不支持（原版也会收走）"
            m = EV_ITEM_RE.match(text)
            if not m:
                return None, f"条件 {text} 不支持"
            iid = int(m.group(1))
            if iid not in known:
                return None, f"DelItem:EVDEL 的道具 {iid} 不在道具表里"
            if iid in not_del:
                continue
            items.append({"id": iid, "n": int(m.group(2)) if m.group(2) else 0})
        out.append(items)
    return out, ""


def get_pet_ids(value: str) -> tuple[list | None, str]:
    """GetPet：怪物编号（ENEMY_ID）逗号分隔；查表在 Go 的加载时做（怪物和宠物模板都要在表里）。"""
    out = []
    for raw in value.split(","):
        text = raw.strip()
        if not re.fullmatch(r"\d+", text):
            return None, f"GetPet:{value} 格式不支持"
        out.append(int(text))
    return out, ""


def not_del_ids(value: str) -> tuple[set | None, str]:
    """NotDel：要排除的道具编号，逗号分隔。"""
    out = set()
    for raw in value.split(","):
        text = raw.strip()
        if not re.fullmatch(r"\d+", text):
            return None, f"NotDel:{value} 格式不支持"
        out.add(int(text))
    return out, ""


def convert_pets(fields: dict[str, str], rec: dict, kind: str, known: set[int]) -> str:
    """把块里的宠物和 EVDEL 字段写进 rec（del_pet、del_pet_ev、del_item_ev、get_pet），返回不支持的原因，空表示支持。
    REQUEST 的宠物交易放弃时的撤销没有对应的原版行为，整块标不支持（同带随机道具的 REQUEST）。"""
    evdel_item = fields.get("DelItem", "").strip() == "EVDEL"
    not_del: set[int] = set()
    if "NotDel" in fields:
        ids, why = not_del_ids(fields["NotDel"])
        if why:
            return why
        if not evdel_item:
            return "NotDel 只能和 DelItem:EVDEL 一起用"
        not_del = ids
    if "DelPet" in fields:
        if kind == "REQUEST":
            return "REQUEST 带 DelPet，放弃时的撤销未实现"
        value = fields["DelPet"].strip()
        if value == "EVDEL":
            groups, why = del_pet_ev(fields.get("EVENT"))
            if why:
                return why
            rec["del_pet_ev"] = groups
        else:
            pets, why = parse_pet_list(value, 0)
            if why:
                return why
            rec["del_pet"] = pets
    if "GetPet" in fields:
        if kind == "REQUEST":
            return "REQUEST 带 GetPet，放弃时的撤销未实现"
        if "PetFullMsg" not in fields:
            return "GetPet 没有 PetFullMsg（宠物栏满时的台词）"
        ids, why = get_pet_ids(fields["GetPet"])
        if why:
            return why
        rec["get_pet"] = ids
    if evdel_item:
        groups, why = del_item_ev(fields.get("EVENT"), not_del, known)
        if why:
            return why
        rec["del_item_ev"] = groups
    return ""
