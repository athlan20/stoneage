#!/usr/bin/env python3
# 遇敌查询（只读工具）：某个坐标会遇到什么敌人、等级多少、几只、怎么站位。
#
# 数据源是 server/gamedata/ 下由 tools/gen-encounters.py 从原版服务端 GBK 表生成的 TSV：
#   encounts.tsv    = gmsv/data/encount.txt   （遇敌矩形区域）
#   enemygroups.tsv = gmsv/data/group1.txt    （敌群，setup.cf 的 groupfile）
#   enemies.tsv     = gmsv/data/enemy.txt     （敌人变体：等级范围、同场上限、掉落）
#   petbase.tsv     = 敌人模板（名字、体型）
# 算法照 gmsv/char/enemy.c 的 ENEMY_getEnemy 与 gmsv/char/char_walk.c 的 CEP 判定。
#
# 用法：
#   python tools/encounter-lookup.py --floor 100 --x 650 --y 500   # 这个点会遇到什么
#   python tools/encounter-lookup.py --floor 100                   # 这张图所有遇敌区域
#   python tools/encounter-lookup.py --name 乌力                    # 哪些区域能遇到它
#   python tools/encounter-lookup.py --lv 1                        # 哪些区域有 1 级怪

import argparse
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DATA = os.path.join(ROOT, "server", "gamedata")

ENEMYACT = 1  # gmsv setup.cf 的 ENEMYACTION，缺省 1；遇敌分母 = 120 * ENEMYACT


def load(name):
    rows = []
    with open(os.path.join(DATA, name), encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if not line or line[0] == "#":
                continue
            rows.append(line.split("\t"))
    return rows


def num(s):
    try:
        return int(s)
    except ValueError:
        return -1


class Data:
    def __init__(self):
        self.areas = []
        for r in load("encounts.tsv"):
            # encounts.tsv 的列（STAGE-152 起）：稳定 ID、原编号、地图、x1、y1、x2、y2、
            # zorder、最多敌人数、CEP 下限、CEP 上限、原始行号、序号、10 组（敌群, 权重）。
            a = {
                "id": num(r[1]), "floor": num(r[2]),
                "x1": num(r[3]), "y1": num(r[4]), "x2": num(r[5]), "y2": num(r[6]),
                "zorder": num(r[7]), "max": num(r[8]), "groups": [],
            }
            if a["x1"] > a["x2"]:
                a["x1"], a["x2"] = a["x2"], a["x1"]
            if a["y1"] > a["y2"]:
                a["y1"], a["y2"] = a["y2"], a["y1"]
            for i in range(10):
                a["groups"].append((num(r[13 + i * 2]), num(r[14 + i * 2])))
            self.areas.append(a)
        self.groups = {}
        for r in load("enemygroups.tsv"):
            g = {"id": num(r[0]), "appear": num(r[1]), "notappear": num(r[2]), "enemies": []}
            for i in range(10):
                eid = num(r[3 + i * 2])
                if eid != -1:
                    g["enemies"].append((eid, num(r[4 + i * 2])))
            self.groups[g["id"]] = g
        self.variants = {}
        for r in load("enemies.tsv"):
            v = {
                "id": num(r[0]), "tpl": num(r[1]), "lvmin": num(r[2]), "lvmax": num(r[3]),
                "cmax": num(r[4]), "exp": num(r[6]),
                "randomof": [num(x) for x in r[28].split(",") if x.strip()],
                "tactics": num(r[30]),
            }
            self.variants[v["id"]] = v
        self.tpl = {}
        for r in load("petbase.tsv"):
            self.tpl[num(r[0])] = {"name": r[1], "size": num(r[5])}

    def area_at(self, floor, x, y):
        best = None
        for a in self.areas:
            if a["floor"] != floor or a["zorder"] <= 0:
                continue
            if not (a["x1"] <= x <= a["x2"] and a["y1"] <= y <= a["y2"]):
                continue
            if best is None or a["zorder"] > best["zorder"]:
                best = a
        return best

    def name_of(self, vid):
        v = self.variants.get(vid)
        if not v:
            return "?%d" % vid
        t = self.tpl.get(v["tpl"])
        return t["name"] if t else "模板%d" % v["tpl"]

    def is_big(self, vid):
        v = self.variants.get(vid)
        t = self.tpl.get(v["tpl"]) if v else None
        return bool(t and t["size"] == 1)

    def lv_text(self, vid):
        v = self.variants[vid]
        return "Lv%d" % v["lvmin"] if v["lvmin"] == v["lvmax"] else "Lv%d-%d" % (v["lvmin"], v["lvmax"])


def steps(minep, maxep):
    """原版 CEP 的期望遇敌步数：每走一格 rand()%(120*act) < cep，不中则 cep+1（上限 maxep），
    触发后 cep 回到 minep。maxep<=0 表示该区域不遇敌。"""
    if maxep <= 0:
        return None
    denom = 120.0 * ENEMYACT
    e = denom / maxep
    for c in range(maxep - 1, minep - 1, -1):
        p = min(1.0, c / denom)
        e = 1 + (1 - p) * e
    return e


def group_pool(d, area):
    """区域里可参与抽签的敌群（跳过空槽、表里缺失、道具条件不满足的按“无条件”处理）。"""
    pool, total, missing = [], 0, []
    for gid, w in area["groups"]:
        if gid == -1:
            continue
        total += w  # 缺失的敌群在原版也参与抽签（抽中 = 空场），所以权重算进分母
        g = d.groups.get(gid)
        if not g:
            missing.append((gid, w))
            continue
        if g["appear"] != -1 or g["notappear"] != -1:
            pool.append((g, w, "有道具条件"))
        else:
            pool.append((g, w, ""))
    return pool, total, missing


def show_point(d, floor, x, y):
    area = d.area_at(floor, x, y)
    mname = ""
    print("地图 %d · 坐标 (%d,%d)" % (floor, x, y))
    if not area:
        print("  不在任何遇敌区域里 → 走多少格都不会遇敌（城镇、室内、安全区都是这种）")
        near = [a for a in d.areas if a["floor"] == floor]
        if near:
            print("  这张图有 %d 个遇敌区域，用 --floor %d 看分布" % (len(near), floor))
        return
    print("  命中区域 #%d  矩形 (%d,%d)-(%d,%d)  zorder=%d  队伍上限 %d 只" % (
        area["id"], area["x1"], area["y1"], area["x2"], area["y2"], area["zorder"], area["max"]))
    overlap = [a for a in d.areas if a["floor"] == floor and a["zorder"] > 0 and a["id"] != area["id"]
               and a["x1"] <= x <= a["x2"] and a["y1"] <= y <= a["y2"]]
    if overlap:
        print("  重叠区域 %s → 取 zorder 最大的这块，平局取表里靠前的行" %
              ", ".join("#%d(z=%d)" % (a["id"], a["zorder"]) for a in overlap))

    mn = mx = None
    for a in d.areas:
        if a["id"] == area["id"]:
            pass
    mn, mx = area_minmax(area)
    st = steps(mn, mx)
    if st is None:
        print("\n  这个区域不遇敌（CEP 上限 0）")
    else:
        print("\n  遇敌节奏：CEP %d→%d，每格概率 %d/%d…%d/%d，期望约 %.0f 格一场（enemyact=%d）" % (
            mn, mx, mn, 120 * ENEMYACT, mx, 120 * ENEMYACT, st, ENEMYACT))
        print("  （CEP 是累积的：不遇敌就 +1，遇敌后回到下限；所以“久走必遇”，不是固定概率）")

    pool, total, missing = group_pool(d, area)
    print("\n  敌群抽签（区域这一行给的权重）")
    if not pool:
        print("    没有可用敌群 → 走进去也开不出战斗")
    ok = [p for p in pool if p[1] > 0]
    wsum = total
    for g, w, note in pool:
        prob = ("%5.1f%%" % (100.0 * w / wsum)) if wsum else "    -"
        comp = "、".join("%s %s" % (d.name_of(eid), d.lv_text(eid)) for eid, _ in g["enemies"]) or "空敌群"
        print("    权重 %4d  %s  敌群 %-5d %s%s" % (w, prob, g["id"], comp, (" [" + note + "]") if note else ""))
    for gid, w in missing:
        prob = ("%5.1f%%" % (100.0 * w / wsum)) if wsum else "    -"
        print("    权重 %4d  %s  敌群 %-5d 表里缺失（原版抽中这一场就是空场，新服务端直接剔除）" % (w, prob, gid))

    print("\n  逐敌群展开（等级 / 同场上限 / 群内权重）")
    for g, w, _ in ok:
        print("    敌群 %d：" % g["id"])
        ew = sum(x[1] for x in g["enemies"]) or 1
        for eid, ewgt in g["enemies"]:
            v = d.variants.get(eid)
            if not v:
                print("      敌人 %-5d 表里缺失（这一只不入场）" % eid)
                continue
            cand = ""
            if v["randomof"]:
                cand = "  ← 随机替换: " + "、".join(d.name_of(c) for c in v["randomof"])
            print("      %-12s %-9s 同场上限 %-2d 群内权重 %3d (%4.1f%%) %s%s" % (
                d.name_of(eid), d.lv_text(eid), v["cmax"], ewgt, 100.0 * ewgt / ew,
                "【大型】" if d.is_big(eid) else "普通", cand))
        create = sum(d.variants[e]["cmax"] for e, _ in g["enemies"] if e in d.variants)
        print("      这一群一场最多出 %d 只" % create)

    # 队伍规模与队形
    sizes = []
    for g, _, _ in ok:
        create = sum(d.variants[e]["cmax"] for e, _ in g["enemies"] if e in d.variants)
        cap = min(area["max"], create)
        sizes.append((g["id"], cap))
    caps = sorted({c for _, c in sizes})
    print("\n  队伍规模")
    print("    一场的敌人只数 = 1 .. min(区域上限 %d, 该敌群各变体同场上限之和)" % area["max"])
    for gid, cap in sizes:
        print("      敌群 %-5d → 1..%d 只" % (gid, cap))
    print("    同一变体的只数另有上限：同场上限 × 它在敌群里占的槽位数")
    print("    大型怪全场最多 5 只；前 5 只（前排 15-19 号格）优先给大型，第 6 只大型会顶掉前排的普通怪")

    # 1 级判定
    lv1 = []
    allmin = 999
    for g, _, _ in ok:
        for eid, _ in g["enemies"]:
            v = d.variants.get(eid)
            if not v:
                continue
            ids = [eid] + v["randomof"]
            for i in ids:
                vv = d.variants.get(i)
                if not vv:
                    continue
                allmin = min(allmin, vv["lvmin"])
                if vv["lvmin"] <= 1 <= vv["lvmax"]:
                    lv1.append(d.name_of(i))
    print("\n  等级")
    print("    最低等级 %d；1 级怪：%s" % (allmin, "可能出现（%s）" % "、".join(sorted(set(lv1))) if lv1 else "不会出现"))
    print("    等级在 [下限,上限] 里均匀随机；两只同种怪等级可以不同")


def area_minmax(area):
    """encount 行的第 7/8 列（CEP 下限/上限）。TSV 没带这两列，回原始表按区域号查。"""
    p = os.path.join(ROOT, "original-data", "gmsv", "data", "encount.txt")
    if not os.path.exists(p):
        return 1, 5
    with open(p, encoding="gbk", errors="replace") as f:
        for line in f:
            line = line.strip()
            if not line or line[0] == "#":
                continue
            c = line.split(",")
            if num(c[0]) == area["id"]:
                a, b = num(c[6]), num(c[7])
                return min(a, b), max(a, b)
    return 1, 5


def show_floor(d, floor):
    rows = [a for a in d.areas if a["floor"] == floor]
    if not rows:
        print("地图 %d 没有遇敌区域表" % floor)
        return
    print("地图 %d 共 %d 个遇敌区域" % (floor, len(rows)))
    print("  区域  矩形               z          CEP    步/场  上限  敌群(权重)  最低等级  含1级")
    for a in sorted(rows, key=lambda a: a["id"]):
        mn, mx = area_minmax(a)
        st = steps(mn, mx)
        pool, _, _ = group_pool(d, a)
        gs = ",".join("%d(%d)" % (g["id"], w) for g, w, _ in pool) or "-"
        lvs = []
        for g, _, _ in pool:
            for eid, _ in g["enemies"]:
                v = d.variants.get(eid)
                if v:
                    lvs.append(v["lvmin"])
        lo = min(lvs) if lvs else 0
        has1 = any(d.variants[e]["lvmin"] <= 1 <= d.variants[e]["lvmax"]
                   for g, _, _ in pool for e, _ in g["enemies"] if e in d.variants)
        print("  #%-4d (%3d,%3d)-(%3d,%3d) %3d   %3d-%-3d  %5s   %3d  %s  %6s  %s" % (
            a["id"], a["x1"], a["y1"], a["x2"], a["y2"], a["zorder"], mn, mx,
            ("%.0f" % st) if st else "不遇敌", a["max"], gs, ("Lv%d" % lo) if lo else "-",
            "是" if has1 else "否"))


def find(d, name=None, lv=None):
    """反查：某个敌人 / 某个等级出现在哪些区域。"""
    hit = set()
    for vid, v in d.variants.items():
        ids = [vid] + v["randomof"]
        for i in ids:
            vv = d.variants.get(i)
            if not vv:
                continue
            if name and name in d.name_of(i):
                hit.add(i)
            if lv is not None and vv["lvmin"] <= lv <= vv["lvmax"]:
                hit.add(i)
    if not hit:
        print("没找到")
        return
    print("命中 %d 个敌人变体：%s" % (len(hit), "、".join(sorted({d.name_of(i) for i in hit}))[:200]))
    # 敌人 → 敌群 → 区域
    places = []
    for a in d.areas:
        if a["zorder"] <= 0:
            continue
        for gid, w in a["groups"]:
            g = d.groups.get(gid)
            if not g:
                continue
            if any(e in hit for e, _ in g["enemies"]):
                places.append((a, gid, w))
    print("出现地点（%d 个区域×敌群）：" % len(places))
    seen = {}
    for a, gid, w in places:
        seen.setdefault(a["floor"], []).append((a["id"], gid, w, a["zorder"]))
    for floor in sorted(seen):
        print("  地图 %-5d %s" % (floor, "  ".join("#%d/%d(z%d,w%d)" % x for x in sorted(seen[floor]))))


def main():
    ap = argparse.ArgumentParser(description="遇敌查询（只读）")
    ap.add_argument("--floor", type=int, help="地图编号")
    ap.add_argument("--x", type=int, help="坐标 x")
    ap.add_argument("--y", type=int, help="坐标 y")
    ap.add_argument("--name", help="按敌人名字反查")
    ap.add_argument("--lv", type=int, help="按等级反查（该等级在范围内就算）")
    args = ap.parse_args()
    d = Data()
    if args.name or args.lv is not None:
        find(d, args.name, args.lv)
    elif args.floor is not None and args.x is not None and args.y is not None:
        show_point(d, args.floor, args.x, args.y)
    elif args.floor is not None:
        show_floor(d, args.floor)
    else:
        ap.print_help()


if __name__ == "__main__":
    sys.exit(main())
