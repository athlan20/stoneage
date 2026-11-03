#!/usr/bin/env python3
"""STAGE-1 生成客户端世界地图的海岸线数据（设计见 docs/map.md）。

读取原版服务端的 LS2MAP 地图文件，把两座主岛降采样后取出海岸线，
按 docs/map.md 的岛偏移换算成世界格坐标，写成 Rust 常量文件
client/src/ui/worldmap_data.rs（生成文件，不要手改）。

STAGE-20 同时生成 client/src/ui/worldmap_floors.rs：每个能从两座主岛走到的 floor
（村庄、室内、洞窟）对应的“岛上入口”，来源是 mapwarp.txt 和 npcgen_warp，
从岛出发做广度优先搜索，取离岛最近的入口。

用法: python3 tools/gen-worldmap.py
"""
import re
import struct
import sys
from collections import deque
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MAP_DIR = ROOT / "original-data/gmsv/data/map"
OUT = ROOT / "client/src/ui/worldmap_data.rs"
OUT_FLOORS = ROOT / "client/src/ui/worldmap_floors.rs"
GMSV_DATA = ROOT / "original-data/gmsv/data"

SEA_TILES = {0, 193, 194, 195, 196}
STRIDE = 6  # 每个采样块覆盖 6×6 格

# (名字, 文件, 岛偏移 x, y)  偏移来自 docs/map.md
ISLANDS = [
    ("SAINASU", "sainasu/sainasu", 142, 198),
    ("JALGA", "jyaruga/jalga", 669, 753),
]


def read_ground(path):
    data = path.read_bytes()
    if data[:6] != b"LS2MAP":
        sys.exit(f"{path}: 不是 LS2MAP 文件")
    w, h = struct.unpack(">HH", data[40:44])
    if len(data) != 44 + 4 * w * h:
        sys.exit(f"{path}: 文件大小与 {w}x{h} 不符")
    ground = struct.unpack(f">{w * h}H", data[44 : 44 + 2 * w * h])
    return w, h, ground


def coast(path, ox, oy):
    w, h, ground = read_ground(path)
    bw, bh = (w + STRIDE - 1) // STRIDE, (h + STRIDE - 1) // STRIDE
    land = [[False] * bw for _ in range(bh)]
    for by in range(bh):
        for bx in range(bw):
            n = total = 0
            for y in range(by * STRIDE, min((by + 1) * STRIDE, h - 1)):  # 跳过最后一行的杂点
                for x in range(bx * STRIDE, min((bx + 1) * STRIDE, w)):
                    total += 1
                    n += ground[y * w + x] not in SEA_TILES
            land[by][bx] = total > 0 and n * 2 >= total

    def is_land(bx, by):
        return 0 <= bx < bw and 0 <= by < bh and land[by][bx]

    pts = []
    for by in range(bh):
        for bx in range(bw):
            if land[by][bx] and not all(
                is_land(bx + dx, by + dy) for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1))
            ):
                pts.append((ox + bx * STRIDE + STRIDE // 2, oy + by * STRIDE + STRIDE // 2))
    return pts


def warp_edges():
    """所有传送：(源 floor, 源 x, 源 y, 目标 floor)。源坐标取区域中心。"""
    edges = []
    for line in (GMSV_DATA / "map/mapwarp.txt").read_text(errors="ignore").splitlines():
        parts = line.strip().split(":")
        if len(parts) < 4 or parts[0] != "NONE":
            continue
        try:
            sf, sx, sy = (int(v) for v in parts[2].split(","))
            df = int(parts[3].split(",")[0])
        except ValueError:
            continue
        edges.append((sf, sx, sy, df))
    block = re.compile(r"\{([^{}]*)\}")
    for path in sorted((GMSV_DATA / "npc").rglob("*.create")):
        for body in block.findall(path.read_text(errors="ignore")):
            f = re.search(r"^floorid=(\d+)", body, re.M)
            c = re.search(r"^borncorner=(\d+),(\d+),(\d+),(\d+)", body, re.M)
            e = re.search(r"^enemy=npcgen_warp\|(\d+)\|", body, re.M)
            if f and c and e:
                x1, y1, x2, y2 = (int(v) for v in c.groups())
                edges.append((int(f.group(1)), (x1 + x2) // 2, (y1 + y2) // 2, int(e.group(1))))
    return edges


def clusters(points):
    """把相邻（切比雪夫距离 ≤ 2）的入口格子归成一簇：一个村口往往是连续的几格。"""
    groups = []
    for p in sorted(points):
        for g in groups:
            if any(max(abs(p[0] - q[0]), abs(p[1] - q[1])) <= 2 for q in g):
                g.append(p)
                break
        else:
            groups.append([p])
    return groups


def floor_entrances(edges):
    """从两座岛出发做广度优先搜索：floor -> (岛 floor, 岛上入口 x, y)。
    直接相连的 floor：一个村可能有好几个口，取格子最多的一簇（同样多取靠南靠东的一簇）的中心；
    更深的 floor 继承上一层的入口，同一层有多个来路时取 (岛, x, y) 最小的，保证结果稳定。"""
    islands = {100, 200}
    adj = {}
    for sf, sx, sy, df in edges:
        adj.setdefault(sf, []).append((sx, sy, df))
    first = {}  # floor -> {岛: [入口格子]}
    for isl in sorted(islands):
        for sx, sy, df in adj.get(isl, []):
            if df not in islands:
                first.setdefault(df, {}).setdefault(isl, []).append((sx, sy))
    best = {}  # floor -> (深度, 岛, x, y)
    for df, per_island in first.items():
        cands = [(len(g), max((y, x) for x, y in g), isl, g) for isl, pts in per_island.items() for g in clusters(pts)]
        size, _, isl, g = max(cands, key=lambda c: c[:3])
        best[df] = (1, isl, round(sum(x for x, _ in g) / len(g)), round(sum(y for _, y in g) / len(g)))
    frontier = set(best)
    depth = 1
    while frontier:
        nxt = {}
        for f in frontier:
            _, isl, ex, ey = best[f]
            for _, _, df in adj.get(f, []):
                if df in islands or (df in best and best[df][0] <= depth):
                    continue
                cand = (depth + 1, isl, ex, ey)
                if df not in nxt or cand < nxt[df]:
                    nxt[df] = cand
        best.update(nxt)
        frontier = set(nxt)
        depth += 1
    return {f: (v[1], v[2], v[3]) for f, v in best.items()}


def write_floors(table):
    out = [
        "// 生成文件，由 tools/gen-worldmap.py 产生，不要手改。",
        "// floor -> 岛上入口（岛 floor, 入口 x, 入口 y），按 floor 排序，见 docs/map.md。",
        "",
        "pub const FLOOR_ENTRANCE: &[(u32, u32, i16, i16)] = &[",
    ]
    items = sorted(table.items())
    for i in range(0, len(items), 4):
        out.append("    " + " ".join(f"({f}, {isl}, {x}, {y})," for f, (isl, x, y) in items[i : i + 4]))
    out.append("];")
    OUT_FLOORS.write_text("\n".join(out) + "\n")
    print(f"写入 {OUT_FLOORS.relative_to(ROOT)}：{len(items)} 个 floor")


def main():
    out = [
        "// 生成文件，由 tools/gen-worldmap.py 产生，不要手改。",
        "// 两座主岛的海岸线采样点，世界格坐标（x 向东，y 向南），见 docs/map.md。",
        "",
    ]
    all_pts = []
    for name, rel, ox, oy in ISLANDS:
        pts = coast(MAP_DIR / rel, ox, oy)
        all_pts += pts
        out.append(f"pub const {name}: &[(i16, i16)] = &[")
        for i in range(0, len(pts), 8):
            out.append("    " + " ".join(f"({x}, {y})," for x, y in pts[i : i + 8]))
        out.append("];")
        out.append("")
    xs, ys = [p[0] for p in all_pts], [p[1] for p in all_pts]
    out.append(f"pub const BOUNDS_X: (f64, f64) = ({min(xs)}.0, {max(xs)}.0);")
    out.append(f"pub const BOUNDS_Y: (f64, f64) = ({min(ys)}.0, {max(ys)}.0);")
    OUT.write_text("\n".join(out) + "\n")
    write_floors(floor_entrances(warp_edges()))
    print(f"写入 {OUT.relative_to(ROOT)}：{len(all_pts)} 个点，x {min(xs)}–{max(xs)}，y {min(ys)}–{max(ys)}")


main()
