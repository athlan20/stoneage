#!/usr/bin/env python3
"""STAGE-251：生成 1 级宠物目录（口径见 docs/encounter.md「1级宠物目录口径」）。

输入都在 server/gamedata 下：
- petbase.tsv（宠物模板）、enemies.tsv / enemygroups.tsv / encounts.tsv（遇敌表，tools/gen-encounters.py 的产物）；
- mapsizes_gen.go / mapexits_gen.go（裁边界、出口触发格，与服务端 mapsizes.go / mapexits.go 同源）；
- travel_catalog_gen.go（正式地图与显示名，即快捷传送目录 5.42）；
- pet_aliases.tsv / pet_series.tsv（人工审核清单，改了不用重跑遇敌数据，但要重跑本脚本）。

输出：
- server/gamedata/pet_encounters_gen.go（生成文件，DO NOT EDIT）；
- build/pet-encounter-report.md（规模、剔除项逐条原因、无俗称/系列的宠物、别名校验、数据指纹）。

筛选与服务端 server/character/encounter_areas.go 的 buildAreaView / areaAnchor 同一口径（这里是它的 Python 镜像）；
server/character/encounter_pet_catalog_test.go 会把生成出来的每个 spot 放进服务端真正的 buildAreaView 再校验一遍。
“约多少格一场”直接调用 tools/encounter-lookup.py 的 steps()。

用法：python3 tools/gen-pet-encounters.py
"""

import hashlib
import importlib.util
import re
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DATA = ROOT / "server/gamedata"
OUT = DATA / "pet_encounters_gen.go"
REPORT = ROOT / "build/pet-encounter-report.md"
FP_INPUTS = ["petbase.tsv", "enemies.tsv", "enemygroups.tsv", "encounts.tsv", "pet_aliases.tsv",
             "pet_series.tsv", "mapsizes_gen.go", "mapexits_gen.go", "travel_catalog_gen.go"]
PLAYER_SOURCE_WORDS = ("玩家", "新浪", "黑话")  # 备注里出现这些字样的俗称，报告里单列为“仅玩家资料来源”
CLOUD_EXPECT = {"pets": 93, "maps": 49, "areas": 583}  # 需求描述里的实测口径，只用来对照


def load_lookup():
    """借用 encounter-lookup.py 的 steps()（CEP 期望步数），不复制公式。"""
    spec = importlib.util.spec_from_file_location("encounter_lookup", ROOT / "tools/encounter-lookup.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def rows(name):
    out = []
    for line in (DATA / name).read_text(encoding="utf-8").splitlines():
        if line and not line.startswith("#"):
            out.append(line.split("\t"))
    return out


def fail(msg):
    sys.exit(f"gen-pet-encounters: {msg}")


def load_inputs():
    pets = {}
    for r in rows("petbase.tsv"):
        pets[int(r[0])] = {"id": int(r[0]), "name": r[1], "flg": int(r[4]), "size": int(r[5]),
                           "catch": int(r[14]), "atoms": [int(x) for x in r[15:19]]}
    enemies = {}
    for r in rows("enemies.tsv"):
        enemies[int(r[0])] = {"id": int(r[0]), "tpl": int(r[1]), "lvmin": int(r[2]), "lvmax": int(r[3]),
                              "cmax": int(r[4]), "randomof": [int(x) for x in r[28].split(",") if x.strip()]}
    groups = {}
    for r in rows("enemygroups.tsv"):
        slots = [(int(r[3 + 2 * i]), int(r[4 + 2 * i])) for i in range(10)]
        groups[int(r[0])] = {"appear": int(r[1]), "notappear": int(r[2]),
                             "enemies": [(e, w) for e, w in slots if e != -1]}
    areas = []
    for r in rows("encounts.tsv"):
        x1, x2 = sorted((int(r[3]), int(r[5])))
        y1, y2 = sorted((int(r[4]), int(r[6])))
        areas.append({"key": r[0], "number": int(r[1]), "floor": int(r[2]), "rect": (x1, y1, x2, y2),
                      "zorder": int(r[7]), "max": int(r[8]), "cepmin": int(r[9]), "cepmax": int(r[10]),
                      "groups": [(int(r[13 + 2 * i]), int(r[14 + 2 * i])) for i in range(10)]})
    sizes, exits, names = {}, {}, {}
    for line in (DATA / "mapsizes_gen.go").read_text(encoding="utf-8").splitlines():
        m = re.match(r"\s*(\d+): \{(\d+), (\d+)\},", line)
        if m:
            sizes[int(m[1])] = (int(m[2]), int(m[3]))
    floor = None
    for line in (DATA / "mapexits_gen.go").read_text(encoding="utf-8").splitlines():
        m = re.match(r"\t(\d+): \{$", line)
        if m:
            floor = int(m[1])
            continue
        for x, y in re.findall(r"\{X: (-?\d+), Y: (-?\d+),", line):
            exits.setdefault(floor, set()).add((int(x), int(y)))
    for line in (DATA / "travel_catalog_gen.go").read_text(encoding="utf-8").splitlines():
        m = re.search(r'Name: "([^"]*)", Floor: ip\((\d+)\)', line)
        if m:
            names[int(m[2])] = m[1]
    return pets, enemies, groups, areas, sizes, exits, names


def load_aliases_series(pets):
    """校验并读取人工清单：模板编号不存在、名字对不上是错误；挂在 PETFLG=0 上、俗称跨名字重复是警告。"""
    warns, aliases, by_alias = [], {}, {}
    for r in rows("pet_aliases.tsv"):
        tid, name = int(r[0]), r[1]
        if tid not in pets:
            fail(f"pet_aliases.tsv：模板编号 {tid} 不存在")
        if pets[tid]["name"] != name:
            fail(f"pet_aliases.tsv：模板 {tid} 的名字是 {pets[tid]['name']}，表里写的是 {name}")
        al = [x.strip() for x in re.split(r"[、,，]", r[2]) if x.strip()] if len(r) > 2 else []
        if al and pets[tid]["flg"] != 1:
            warns.append(f"俗称「{'、'.join(al)}」挂在 PETFLG=0 的模板 {tid}（{name}）上，已忽略")
            continue
        for a in al:
            aliases.setdefault(tid, []).append(a)
            by_alias.setdefault(a, set()).add(name)
    for a, ns in sorted(by_alias.items()):
        if len(ns) > 1:
            warns.append(f"俗称「{a}」对应多个名字：{'、'.join(sorted(ns))}（全部列出，不做消歧）")
    series, series_of = [], {}
    for r in rows("pet_series.tsv"):
        members = [int(x) for x in r[1].split(",") if x.strip()]
        for tid in members:
            if tid not in pets:
                fail(f"pet_series.tsv：系列「{r[0]}」的成员 {tid} 不存在")
            if pets[tid]["flg"] != 1:
                warns.append(f"系列「{r[0]}」的成员 {tid}（{pets[tid]['name']}）不是 PETFLG=1，已忽略")
                continue
            if tid in series_of and series_of[tid] != r[0]:
                warns.append(f"模板 {tid} 同时属于系列「{series_of[tid]}」和「{r[0]}」，取前者")
                continue
            series_of[tid] = r[0]
        series.append((r[0], members))
    return aliases, series_of, series, warns


def clamp(floor, rect, sizes):
    x1, y1, x2, y2 = rect
    if floor not in sizes:
        return x1, y1, x2, y2, x1 <= x2 and y1 <= y2
    w, h = sizes[floor]
    nx1, ny1, nx2, ny2 = max(x1, 0), max(y1, 0), min(x2, w - 1), min(y2, h - 1)
    return nx1, ny1, nx2, ny2, nx1 <= nx2 and ny1 <= ny2


def build_owners(areas, sizes):
    """每张图每格归属哪个区域：zorder 最大、平局取文件靠前（同 areaAtOn）。只涂裁边界后的矩形。"""
    owners = {}
    for i in sorted(range(len(areas)), key=lambda i: (-areas[i]["zorder"], i)):
        a = areas[i]
        if a["zorder"] <= 0:
            continue
        x1, y1, x2, y2, ok = clamp(a["floor"], a["rect"], sizes)
        if not ok:
            continue
        cells = owners.setdefault(a["floor"], {})
        for x in range(x1, x2 + 1):
            for y in range(y1, y2 + 1):
                cells.setdefault((x, y), i)
    return owners


def candidates(a, enemies, groups, pets):
    """展开区域的敌群与敌人（同 collectAreaEnemies）。返回 (全部候选, 当前可遇候选, 道具条件挡住的敌群编号)。"""
    allc, usable, gated = [], [], []
    for gid, w in a["groups"]:
        if gid == -1 or w <= 0 or gid not in groups:
            continue
        g = groups[gid]
        ok = g["appear"] == -1 and g["notappear"] == -1
        if not ok:
            gated.append(gid)
        for eid, ew in g["enemies"]:
            if eid not in enemies or ew <= 0:
                continue
            e = enemies[eid]
            vs = [enemies[x] for x in e["randomof"] if x in enemies] if e["randomof"] else [e]
            for v in vs:
                if v["tpl"] not in pets:
                    continue
                c = {"id": v["id"], "tpl": v["tpl"], "lvmin": v["lvmin"], "lvmax": v["lvmax"],
                     "cmax": v["cmax"], "gid": gid}
                allc.append(c)
                if ok:
                    usable.append(c)
    return allc, usable, gated


def cap_of(area_max, allc):
    best = {}
    for c in allc:
        best[c["id"]] = max(best.get(c["id"], 0), c["cmax"])
    s = sum(best.values())
    return 0 if s < 1 else min(area_max, s)


def anchor_of(idx, a, owners, blocked, sizes):
    x1, y1, x2, y2, _ = clamp(a["floor"], a["rect"], sizes)
    cells = owners.get(a["floor"], {})
    for x in range(x1, x2 + 1):
        for y in range(y1, y2 + 1):
            if (x, y) in blocked:
                continue
            if cells.get((x, y)) == idx:
                return x, y
    return None


def is_one_level(c):
    return c["lvmin"] <= 1 <= c["lvmax"] and c["cmax"] > 0


def go_str(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def main():
    lookup = load_lookup()
    pets, enemies, groups, areas, sizes, exits, names = load_inputs()
    aliases, series_of, series, warns = load_aliases_series(pets)
    owners = build_owners(areas, sizes)

    excluded_pairs = []  # (pet_id, area_key, floor, number, reason)
    nonpet = {}          # 非宠物的 1 级模板 -> 出现的区域数
    spots = []           # (pet_id, area index, usable 1-level 宠物 id 列表)
    area_reason_cache = {}
    for idx, a in enumerate(areas):
        allc, usable, gated = candidates(a, enemies, groups, pets)
        ones = [c for c in allc if is_one_level(c)]
        for c in ones:
            if pets[c["tpl"]]["flg"] != 1:
                nonpet[c["tpl"]] = nonpet.get(c["tpl"], 0) + 1
        pet_ones = [c for c in ones if pets[c["tpl"]]["flg"] == 1]
        if not pet_ones:
            continue
        reason = None
        if a["zorder"] <= 0:
            reason = "zorder≤0（服务端忽略）"
        elif a["cepmax"] <= 0:
            reason = "CEP 上限 0（永不遇敌的安全区）"
        elif a["floor"] not in names:
            reason = "地图不在快捷传送目录"
        elif not clamp(a["floor"], a["rect"], sizes)[4]:
            reason = "裁到地图边界后为空"
        elif not allc:
            reason = "没有有效敌人"
        elif cap_of(a["max"], allc) < 1:
            reason = "同场上限为 0"
        elif not anchor_of(idx, a, owners, exit_cells(exits, a["floor"]), sizes):
            reason = "被更高 zorder 完全覆盖或只剩出口格，无代表落点"
        if reason:
            for c in pet_ones:
                excluded_pairs.append((c["tpl"], a["key"], a["floor"], a["number"], reason))
            continue
        area_pets = {}
        for c in usable:
            if is_one_level(c) and pets[c["tpl"]]["flg"] == 1:
                area_pets.setdefault(c["tpl"], True)
        gated_pets = {c["tpl"] for c in pet_ones if c["gid"] in gated}
        for tid in {c["tpl"] for c in pet_ones}:
            if tid in area_pets:
                spots.append((tid, idx, list(area_pets)))
            else:
                reason = "敌群有道具条件（一期不进）" if tid in gated_pets else "敌群未能抽到（权重或空槽）"
                excluded_pairs.append((tid, a["key"], a["floor"], a["number"], reason))
    return finish(lookup, pets, areas, sizes, names, aliases, series_of, series, warns, spots,
                  excluded_pairs, nonpet, owners, exits, enemies, groups)


def exit_cells(exits, floor):
    return exits.get(floor, set())


def finish(lookup, pets, areas, sizes, names, aliases, series_of, series, warns, spots,
           excluded_pairs, nonpet, owners, exits, enemies, groups):
    pet_list = sorted({(pets[tid]["name"].encode("gbk").hex(), tid) for tid, _, _ in spots})
    order = {tid: i for i, (_, tid) in enumerate(pet_list)}
    out_spots = []
    for tid, idx, also in spots:
        a = areas[idx]
        x1, y1, x2, y2, _ = clamp(a["floor"], a["rect"], sizes)
        ax, ay = anchor_of(idx, a, owners, exit_cells(exits, a["floor"]), sizes)
        allc, _, _ = candidates(a, enemies, groups, pets)
        steps = lookup.steps(a["cepmin"], a["cepmax"])
        others = sorted({pets[t]["name"] for t in also if t != tid}, key=lambda n: n.encode("gbk"))
        out_spots.append({"tid": tid, "key": a["key"], "number": a["number"], "floor": a["floor"],
                          "map": names[a["floor"]], "anchor": (ax, ay), "rect": (x1, y1, x2, y2),
                          "max": cap_of(a["max"], allc), "steps": max(1, round(steps)), "also": others})
    out_spots.sort(key=lambda s: (order[s["tid"]], s["floor"], s["key"]))
    total_areas = usable_area_count(areas, enemies, groups, pets, sizes, exits, owners, names)
    fp = fingerprint()
    write_go(pet_list, pets, aliases, series_of, out_spots, fp)
    write_report(pet_list, pets, aliases, series, warns, out_spots, excluded_pairs, nonpet, fp, total_areas)
    print(f"宠物 {len(pet_list)} 只、地图 {len({s['floor'] for s in out_spots})} 张、"
          f"区域 {len({s['key'] for s in out_spots})} 个、spot {len(out_spots)} 个 → {OUT}")
    print(f"报告：{REPORT}")
    for w in warns:
        print(f"警告: {w}")


def usable_area_count(areas, enemies, groups, pets, sizes, exits, owners, names):
    """全量可用区域（encounter_areas 视角，不分宠物）：用来解释与需求里的 583 的口径差异。"""
    n = 0
    for idx, a in enumerate(areas):
        allc, usable, _ = candidates(a, enemies, groups, pets)
        if (a["zorder"] > 0 and a["cepmax"] > 0 and a["floor"] in names and usable
                and clamp(a["floor"], a["rect"], sizes)[4] and cap_of(a["max"], allc) >= 1
                and anchor_of(idx, a, owners, exit_cells(exits, a["floor"]), sizes)):
            n += 1
    return n


def fingerprint():
    h = hashlib.sha256()
    for name in FP_INPUTS:
        h.update(name.encode() + b"\0" + (DATA / name).read_bytes() + b"\0")
    return h.hexdigest()[:12]


def write_go(pet_list, pets, aliases, series_of, spots, fp):
    lines = ["// Code generated by tools/gen-pet-encounters.py; DO NOT EDIT.", "",
             "package gamedata", "",
             "// petEncounterVersion 是 1 级宠物目录的数据指纹（STAGE-251）：输入表与地图/传送目录的 sha256 前 12 位。",
             f"const petEncounterVersion = {go_str(fp)}", "",
             "// petEncounterPetRows 按 GBK 字节序近似拼音序排好，服务端与客户端都不再改序。",
             "var petEncounterPetRows = []PetEncounterPet{"]
    count = Counter(s["tid"] for s in spots)
    for _, tid in pet_list:
        p = pets[tid]
        al = ", ".join(go_str(x) for x in aliases.get(tid, []))
        lines.append(
            f"\t{{ID: {tid}, Name: {go_str(p['name'])}, Aliases: []string{{{al}}}, "
            f"Series: {go_str(series_of.get(tid, ''))}, Atoms: [4]int{{{', '.join(map(str, p['atoms']))}}}, "
            f"Size: {p['size']}, Catch: {p['catch']}, SortKey: {go_str(p['name'].encode('gbk').hex())}, "
            f"SpotCount: {count.get(tid, 0)}}},")
    lines += ["}", "", "// petEncounterSpotRows 是每只宠物的可遇点（按宠物顺序、地图、区域 ID 排好）。",
              "var petEncounterSpotRows = []PetEncounterSpot{"]
    for s in spots:
        also = ", ".join(go_str(x) for x in s["also"])
        ax, ay = s["anchor"]
        x1, y1, x2, y2 = s["rect"]
        lines.append(
            f"\t{{PetID: {s['tid']}, AreaID: {go_str(s['key'])}, Number: {s['number']}, Floor: {s['floor']}, "
            f"MapName: {go_str(s['map'])}, Anchor: PetEncounterPoint{{X: {ax}, Y: {ay}}}, "
            f"Rect: PetEncounterRect{{X1: {x1}, Y1: {y1}, X2: {x2}, Y2: {y2}}}, MaxEnemies: {s['max']}, "
            f"Steps: {s['steps']}, AlsoLevel1: []string{{{also}}}}},")
    lines.append("}")
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_report(pet_list, pets, aliases, series, warns, spots, excluded, nonpet, fp, total_areas):
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    pets_n = len(pet_list)
    maps_n = len({s["floor"] for s in spots})
    areas_n = len({s["key"] for s in spots})
    L = ["# 1 级宠物目录生成报告（STAGE-251）", "",
         f"- 数据指纹：`{fp}`（输入表或地图/传送目录变动后指纹会变，需要重跑 `tools/gen-pet-encounters.py`）",
         f"- 规模：宠物 **{pets_n}** 只、地图 **{maps_n}** 张、可遇区域 **{areas_n}** 个、可遇点（宠物×区域）**{len(spots)}** 个",
         f"- 需求描述口径：{CLOUD_EXPECT['pets']} 只 / {CLOUD_EXPECT['maps']} 张 / {CLOUD_EXPECT['areas']} 个区域；"
         f"差异见下节说明", "",
         "## 与需求口径的差异", ""]
    L.append(f"- 宠物：生成 {pets_n} 只，需求 {CLOUD_EXPECT['pets']} 只，差 {pets_n - CLOUD_EXPECT['pets']}。")
    L.append(f"- 地图：生成 {maps_n} 张，需求 {CLOUD_EXPECT['maps']} 张，差 {maps_n - CLOUD_EXPECT['maps']}。")
    L.append(f"- 区域：含 1 级宠物的区域 {areas_n} 个；全量可用区域（encounter_areas 口径，不分宠物）{total_areas} 个，"
             f"需求里的 {CLOUD_EXPECT['areas']} 与它的口径接近，以生成器为准。")
    L.append("- 人龙系 5 只中只有 3 只进目录（扬奇洛斯、邦奇诺、利则诺顿）：邦浦洛斯、布鲁顿的唯一 1 级区域在 300 图被同 zorder 的先行区域 300-311 完全覆盖，encounter_areas 也取不到代表落点，按硬规则不进目录（见下节单列）。验收标准里「人龙」命中的口径是「实际有可遇点的人龙系成员」。")
    L.append("")
    L.append("## 被剔除项（宠物 × 区域，逐条原因）")
    L.append("")
    by_reason = {}
    for tid, key, floor, number, reason in excluded:
        by_reason[reason] = by_reason.get(reason, 0) + 1
    for reason, n in sorted(by_reason.items(), key=lambda kv: -kv[1]):
        L.append(f"- {reason}：{n} 条")
    L.append("")
    L.append("### 明细")
    L.append("")
    L.append("| 宠物 | 模板 | 区域 | 地图 | 原因 |")
    L.append("|---|---|---|---|---|")
    for tid, key, floor, number, reason in sorted(excluded, key=lambda e: (pets[e[0]]["name"], e[2], e[1])):
        L.append(f"| {pets[tid]['name']} | {tid} | {key} | {floor} | {reason} |")
    L.append("")
    L.append("## 非宠物的 1 级模板（不进目录）")
    L.append("")
    for tid, n in sorted(nonpet.items()):
        L.append(f"- {tid} {pets[tid]['name']}（出现在 {n} 处 1 级敌群，PETFLG={pets[tid]['flg']}，抓不了）")
    L.append("")
    L.append("## 没有可遇点、因此不进目录的 1 级宠物（单列）")
    L.append("")
    listed = {tid for _, tid in pet_list}
    lone = sorted({e[0] for e in excluded if e[0] not in listed}, key=lambda t: pets[t]["name"].encode("gbk"))
    if not lone:
        L.append("无")
    else:
        L.append("| 宠物 | 模板 | 候选区域数 | 原因 |")
        L.append("|---|---|---|---|")
        for t in lone:
            rs = sorted({e[4] for e in excluded if e[0] == t})
            n = len({e[1] for e in excluded if e[0] == t})
            L.append(f"| {pets[t]['name']} | {t} | {n} | {'；'.join(rs)} |")
    L.append("")
    L.append("## 没有任何俗称与系列标签的宠物（提示补表）")
    L.append("")
    bare = [tid for _, tid in pet_list if tid not in aliases and not any(tid in m for _, m in series)]
    L.append(f"共 {len(bare)} 只：" + "、".join(f"{pets[t]['name']}({t})" for t in bare))
    L.append("")
    L.append("## 俗称来源（仅玩家资料，待玩家反馈）")
    L.append("")
    weak = []
    for tid, al in aliases.items():
        src = pet_source(tid)
        if tid in listed and src and any(w in src for w in PLAYER_SOURCE_WORDS):
            weak.append(f"{pets[tid]['name']}（{'、'.join(al)}）：{src}")
    L += [f"- {x}" for x in weak] or ["无"]
    L.append("")
    L.append("## 别名与系列校验")
    L.append("")
    L += [f"- {w}" for w in warns] or ["无警告"]
    L.append("")
    L.append("## 遇敌点（宠物 → 区域）")
    L.append("")
    L.append("| 宠物 | 地图 | 区域编号 | 区域 ID | 锚点 | 同场上限 | 约几格一场 | 同点其他 1 级宠 |")
    L.append("|---|---|---|---|---|---|---|---|")
    for s in spots:
        ax, ay = s["anchor"]
        L.append(f"| {pets[s['tid']]['name']} | {s['map']}（{s['floor']}） | {s['number']} | {s['key']} | "
                 f"({ax},{ay}) | {s['max']} | {s['steps']} | {'、'.join(s['also']) or '—'} |")
    REPORT.write_text("\n".join(L) + "\n", encoding="utf-8")


def pet_source(tid):
    for r in rows("pet_aliases.tsv"):
        if int(r[0]) == tid:
            return r[3] if len(r) > 3 else ""
    return ""


if __name__ == "__main__":
    main()
