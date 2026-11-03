"""便利店素材集中化的覆盖表应用（STAGE-302，tools/npc_overlay/convenience_shops.json）。

原版数据（original-data）一字不改，生成时在原版解析之上应用这份覆盖表：
- shops：原有的 8 家素材店（1009/4009 各 4 家）商品换成整组 40 件，buy_rate 设为 1.0，台词与名字保留；
  带 copy_from 的是新增店（1009/4009 各 2 家，kind npcgen_shop），参数复制自 copy_from 那家，再换成整组商品；
- remove：删除原版源 NPC（3109/1102/1302/1402/10408 的旧素材店，共 7 个），编号留空洞，不重排。

编号稳定：新增 NPC 在排序前追加进末尾批次（gen-npcs.py 的 NPC_BATCHES 最后一档），排序后编号接在最后；
删除的 NPC 照常参与编号，输出时跳过。所有条目都按 (地图, x, y, 名字) 精确匹配，必须恰好命中一次。
"""

import copy
import json
from pathlib import Path

from npc_records import expand_itemlist

OVERLAY_FILE = Path(__file__).resolve().parent / "npc_overlay" / "convenience_shops.json"
SHOP_ITEM_COUNT = 40  # 每家素材店的商品数（每组 4 类 × 10 级）
SHOP_ITEM_LIMIT = 60  # 商品数上限（docs/protocol.md 5.38），与 expand_itemlist 默认值一致


def load(path: Path = OVERLAY_FILE) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def _pos(entry: dict) -> tuple:
    return (entry["floor"], entry["x"], entry["y"], entry["name"])


def find_one(npcs: list, entry: dict) -> int:
    """按 (地图, x, y, 名字) 找唯一的 NPC，返回编号（npcs 已排序编号，编号 = 下标 + 1）。"""
    hits = [i for i, n in enumerate(npcs, 1) if (n["floor"], n["x"], n["y"], n["name"]) == _pos(entry)]
    if len(hits) != 1:
        raise SystemExit(f"STAGE-302 覆盖表：{_pos(entry)} 命中 {len(hits)} 个 NPC，应为恰好 1 个")
    return hits[0]


def add_npcs(npcs: list, overlay: dict, floors: set, kind_type: str, batch: int) -> int:
    """把覆盖表里的新增店追加进 npcs（排序前调用，batch 为末尾批次），返回新增个数。"""
    added = 0
    for line_no, e in enumerate(overlay["shops"], 1):
        if "copy_from" not in e:
            continue
        if e["floor"] not in floors:
            raise SystemExit(f"STAGE-302 覆盖表第 {line_no} 条的地图 {e['floor']} 不存在")
        npcs.append({
            "floor": e["floor"], "x": e["x"], "y": e["y"], "kind": e["kind"], "type": kind_type,
            "batch": batch, "name": e["name"], "enemy": e["kind"],
            "file": OVERLAY_FILE.name, "line": line_no,
        })
        added += 1
    return added


def removed_ids(npcs: list, overlay: dict) -> set:
    """要删除的 NPC 编号（编号已分配，删除的只是不输出，编号留空洞）。"""
    return {find_one(npcs, e) for e in overlay["remove"]}


def apply_shops(npcs: list, shop: list, overlay: dict, known: set) -> tuple:
    """改写商店记录：原有店换成整组商品，新增店复制 copy_from 店的参数。返回 (改商品家数, 新增家数)。"""
    by_id = {r["id"]: r for r in shop}
    changed = added = 0
    for e in overlay["shops"]:
        nid = find_one(npcs, e)
        if "copy_from" in e:
            src = find_one(npcs, e["copy_from"])
            if src not in by_id:
                raise SystemExit(f"STAGE-302 覆盖表：copy_from {_pos(e['copy_from'])} 没有商店参数")
            record = {"id": nid, **copy.deepcopy({k: v for k, v in by_id[src].items() if k != "id"})}
            shop.append(record)
            by_id[nid] = record
            added += 1
        else:
            if nid not in by_id:
                raise SystemExit(f"STAGE-302 覆盖表：{_pos(e)} 没有商店参数，无法改商品")
            record = by_id[nid]
            changed += 1
        items = expand_itemlist(overlay["groups"][e["group"]], known, limit=SHOP_ITEM_LIMIT)
        if len(items) != SHOP_ITEM_COUNT:
            raise SystemExit(f"STAGE-302 覆盖表：{_pos(e)} 的 {e['group']} 展开成 {len(items)} 件，应为 {SHOP_ITEM_COUNT}")
        record["buy_rate"] = 1.0
        record["items"] = items
    return changed, added


PET_SHOP_FILE = Path(__file__).resolve().parent / "npc_overlay" / "pet_shop_items.json"
SHOP_ITEM_LIMIT_PET = 60


def apply_item_overlay(npcs: list, shop: list, known: set, path: Path = PET_SHOP_FILE) -> int:
    """STAGE-326：给指定药师店追加商品并写入覆盖买价（record["prices"]，物品编号字符串为键）。返回改了几家店。"""
    overlay = load(path)
    by_id = {r["id"]: r for r in shop}
    for e in overlay["shops"]:
        nid = find_one(npcs, e)
        if nid not in by_id:
            raise SystemExit(f"STAGE-326 覆盖表：{_pos(e)} 没有商店参数，无法加商品")
        record = by_id[nid]
        for item in e["add"]:
            if item not in known:
                raise SystemExit(f"STAGE-326 覆盖表：物品 {item} 不在物品表里")
            if item not in record["items"]:
                record["items"].append(item)
        if len(record["items"]) > SHOP_ITEM_LIMIT_PET:
            raise SystemExit(f"STAGE-326 覆盖表：{_pos(e)} 商品超过 {SHOP_ITEM_LIMIT_PET} 件")
        record["prices"] = {str(k): int(v) for k, v in e["prices"].items()}
    return len(overlay["shops"])

