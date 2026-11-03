#!/usr/bin/env python3
"""STAGE-279：生成练级传送目录（口径见 docs/encounter.md「练级传送口径」、docs/map.md「练级传送目录」）。

输入与 1 级宠物目录相同（server/gamedata 下的 petbase.tsv / enemies.tsv / enemygroups.tsv / encounts.tsv，
mapsizes_gen.go / mapexits_gen.go / travel_catalog_gen.go），区域判定借用 tools/gen-pet-encounters.py 的加载与
buildAreaView 镜像函数。与 1 级宠物不同的地方：
- 不挑宠物：区域里所有常态敌人都算，模板名取 petbase 的名字（缺模板时同服务端 enemyName 退回“敌人 #编号”）；
- 道具条件：只挡 GROUP_APPEARBYITEMID（持有才出现）的敌群；NOTAPPEARBYITEMID（持有才不出现）的敌群照常计入
  （和 buildAreaView 对“不持有道具”的玩家一致，数量对上 323 条的需求表）；
- Lv1 变体（等级区间恰为 1–1）不计入；
- 按 10 级分段，段内裁切等级，同段“怪物名 + 段内等级”相同的区域合并成一条。

输出：
- server/gamedata/leveling_spots_gen.go（生成文件，DO NOT EDIT）；
- build/leveling-spot-report.md（各段条目数、剔除原因统计、数据指纹）。

用法：python3 tools/gen-leveling-spots.py
"""

import hashlib
import importlib.util
import sys
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DATA = ROOT / "server/gamedata"
OUT = DATA / "leveling_spots_gen.go"
REPORT = ROOT / "build/leveling-spot-report.md"
FP_INPUTS = ["petbase.tsv", "enemies.tsv", "enemygroups.tsv", "encounts.tsv",
             "mapsizes_gen.go", "mapexits_gen.go", "travel_catalog_gen.go"]
SEGMENTS = [(i * 10 + 1, i * 10 + 10) for i in range(13)]
# 需求表（STAGE-279 第三节）：各段条目数，生成结果必须与之一致，否则报错。
# STAGE-289 在漆黑洞窟 20 层新增 125–130 级蓝雷，121-130 段从 0 变成 1 条。
EXPECT = [47, 63, 34, 24, 30, 52, 27, 8, 13, 16, 9, 0, 1]


def load_base():
    spec = importlib.util.spec_from_file_location("gen_pet", ROOT / "tools/gen-pet-encounters.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def fail(msg):
    sys.exit(f"gen-leveling-spots: {msg}")


def candidates(a, enemies, groups):
    """展开区域的敌群与敌人（同服务端 collectAreaEnemies）。返回 (全部候选, 常态可遇候选)。"""
    allc, usable = [], []
    for gid, w in a["groups"]:
        if gid == -1 or w <= 0 or gid not in groups:
            continue
        g = groups[gid]
        normal = g["appear"] == -1  # 持有道具才出现的敌群挡住；持有道具才不出现的不挡
        for eid, ew in g["enemies"]:
            if eid not in enemies or ew <= 0:
                continue
            e = enemies[eid]
            vs = [enemies[x] for x in e["randomof"] if x in enemies] if e["randomof"] else [e]
            for v in vs:
                c = {"id": v["id"], "tpl": v["tpl"], "lvmin": v["lvmin"], "lvmax": v["lvmax"], "cmax": v["cmax"]}
                allc.append(c)
                if normal:
                    usable.append(c)
    return allc, usable


def main():
    base = load_base()
    lookup = base.load_lookup()
    pets, enemies, groups, areas, sizes, exits, names = base.load_inputs()
    owners = base.build_owners(areas, sizes)
    entries = defaultdict(lambda: defaultdict(list))  # 段下标 -> 怪物组合 -> 落点
    reasons = Counter()
    for idx, a in enumerate(areas):
        allc, usable = candidates(a, enemies, groups)
        if a["zorder"] <= 0:
            reasons["zorder≤0（服务端忽略）"] += 1
            continue
        if a["cepmax"] <= 0:
            reasons["CEP 上限 0（永不遇敌的安全区）"] += 1
            continue
        if a["floor"] not in names:
            reasons["地图不在快捷传送目录"] += 1
            continue
        if not base.clamp(a["floor"], a["rect"], sizes)[4]:
            reasons["裁到地图边界后为空"] += 1
            continue
        if not allc:
            reasons["没有有效敌人"] += 1
            continue
        if base.cap_of(a["max"], allc) < 1:
            reasons["同场上限为 0"] += 1
            continue
        anchor = base.anchor_of(idx, a, owners, base.exit_cells(exits, a["floor"]), sizes)
        if not anchor:
            reasons["被更高 zorder 完全覆盖或只剩出口格，无代表落点"] += 1
            continue
        variants = [c for c in usable if not (c["lvmin"] == 1 and c["lvmax"] == 1)]
        if not usable:
            reasons["道具条件挡住（持有才出现）"] += 1
            continue
        if not variants:
            reasons["只剩 Lv1 敌人（看 1 级宠物目录）"] += 1
            continue
        cap = base.cap_of(a["max"], allc)
        for si, (s, e) in enumerate(SEGMENTS):
            combo = set()
            for c in variants:
                lo, hi = max(c["lvmin"], s), min(c["lvmax"], e)
                if lo <= hi:
                    combo.add((monster_name(c, pets), lo, hi))
            if combo:
                key = tuple(sorted(combo, key=lambda m: (m[1], m[2], m[0].encode("gbk"))))
                entries[si][key].append({
                    "area": a["key"], "number": a["number"], "floor": a["floor"],
                    "map": names[a["floor"]], "anchor": anchor,
                    "rect": base.clamp(a["floor"], a["rect"], sizes)[:4], "max": cap,
                    "steps": max(1, round(lookup.steps(a["cepmin"], a["cepmax"]))),
                })
    rows = []  # (段下标, 怪物列表, 落点列表)
    for si in range(len(SEGMENTS)):
        items = []
        for key, spots in entries[si].items():
            spots = sorted(spots, key=lambda s: (s["floor"], s["area"]))
            items.append((key, spots))
        items.sort(key=lambda it: (it[0][0][1], it[0][0][2], "\0".join(m[0] for m in it[0]).encode("gbk")))
        for key, spots in items:
            rows.append((si, key, spots))
    counts = [sum(1 for r in rows if r[0] == si) for si in range(len(SEGMENTS))]
    if counts != EXPECT:
        fail(f"各段条目数 {counts} 与需求表 {EXPECT} 对不上")
    fp = fingerprint()
    write_go(rows, fp)
    write_report(rows, reasons, fp)
    total_spots = sum(len(r[2]) for r in rows)
    print(f"练级点 {len(rows)} 条（{len(SEGMENTS)} 段）、落点 {total_spots} 个 → {OUT}")
    print(f"报告：{REPORT}")


def monster_name(c, pets):
    if c["tpl"] in pets:
        return pets[c["tpl"]]["name"]
    return f"敌人 #{c['id']}"


def fingerprint():
    h = hashlib.sha256()
    for name in FP_INPUTS:
        h.update(name.encode() + b"\0" + (DATA / name).read_bytes() + b"\0")
    return h.hexdigest()[:12]


def go_str(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def write_go(rows, fp):
    lines = ["// Code generated by tools/gen-leveling-spots.py; DO NOT EDIT.", "",
             "package gamedata", "",
             "// levelingSpotVersion 是练级传送目录的数据指纹（STAGE-279）：输入表与地图/传送目录的 sha256 前 12 位。",
             f"const levelingSpotVersion = {go_str(fp)}", "",
             "// levelingSpotRows 按段、段内等级升序排好；同段同怪物组合的区域合并成一条（STAGE-279）。",
             "var levelingSpotRows = []LevelingEntry{"]
    for si, key, spots in rows:
        mons = ", ".join(
            f"{{Name: {go_str(n)}, LevelMin: {lo}, LevelMax: {hi}}}" for n, lo, hi in key)
        sp = ", ".join(
            f"{{AreaID: {go_str(s['area'])}, Number: {s['number']}, Floor: {s['floor']}, "
            f"MapName: {go_str(s['map'])}, Anchor: LevelingPoint{{X: {s['anchor'][0]}, Y: {s['anchor'][1]}}}, "
            f"Rect: LevelingRect{{X1: {s['rect'][0]}, Y1: {s['rect'][1]}, X2: {s['rect'][2]}, Y2: {s['rect'][3]}}}, "
            f"MaxEnemies: {s['max']}, Steps: {s['steps']}}}" for s in spots)
        lines.append(f"\t{{Segment: {si * 10 + 1}, Monsters: []LevelingMonster{{{mons}}}, Spots: []LevelingSpot{{{sp}}}}},")
    lines.append("}")
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_report(rows, reasons, fp):
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    L = ["# 练级传送目录生成报告（STAGE-279）", "",
         f"- 数据指纹：`{fp}`（输入表变动后需要重跑 `tools/gen-leveling-spots.py`）",
         f"- 练级点（合并后）{len(rows)} 条，落点 {sum(len(r[2]) for r in rows)} 个", "",
         "## 各段", "", "| 段 | 条目 | 涉及地图 | 落点 |", "|---|---|---|---|"]
    for si, (s, e) in enumerate(SEGMENTS):
        rs = [r for r in rows if r[0] == si]
        maps = {sp["floor"] for r in rs for sp in r[2]}
        spots = sum(len(r[2]) for r in rs)
        L.append(f"| {s}-{e} | {len(rs)} | {len(maps)} | {spots} |")
    L += ["", "## 区域剔除（按原因统计，区域数）", ""]
    L += [f"- {k}：{v}" for k, v in sorted(reasons.items(), key=lambda kv: -kv[1])] or ["无"]
    L.append("")
    REPORT.write_text("\n".join(L) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
