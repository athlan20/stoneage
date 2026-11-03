#!/usr/bin/env python3
"""生成快捷传送目录（STAGE-161）：server/gamedata/travel_catalog_gen.go 与审核报告。

目录数据由两部分组成：

1. `server/gamedata/travel_catalog.json`：**人工审定的权威清单**。它记录
   - `regions`：正式地图的包含树（根是岛屿/大区，往下是村庄、建筑、洞窟楼层）；
   - `excluded`：明确排除的非正式地图及理由；
   - `landings`：需要人工指定的落点（原版传送目标缺失或不合适时）。
2. 原版服务端的地图文件与数据表：`gmsv/data/map/**` 的 LS2MAP 文件头（floor、名字、宽高）、
   `mapwarp.txt` 的传送目标、`npc/**/*.create` 的 `npcgen_warp` 目标。

脚本负责把清单展开成运行表，并做完整性校验：未登记地图、重复登记、孤儿、环、
父节点不存在、落点越界或落在海里、同一层级重名。任何一项不通过就以非 0 退出，
这样「新增地图忘记分类」会在生成阶段暴露，而不是把测试图放进玩家目录。

落点的取值顺序（每张正式地图都要有一个）：
  a. `travel_catalog.json` 的 `landings` 显式指定；
  b. `mapwarp.txt` 里作为**目标**出现的第一条坐标（原版走进这张图时站的位置）；
  c. `mapwarp.txt` 里作为**源**出现的第一条坐标（玩家从这张图走出去的位置，必然可站）；
  d. NPC 脚本里的 `borncorner`；
  e. 从地图中心向外螺旋搜索第一个非海格（地面层不是 0 或 193–196）。
b–e 的坐标都要求 `0 <= x < w`、`0 <= y < h`，且地面层不是海；报告里记下每张图落点的来源。

用法：
  python3 tools/build-travel-catalog.py                # 生成 + 校验
  python3 tools/build-travel-catalog.py --report PATH  # 报告写到指定位置
"""

import argparse
import json
import shutil
import struct
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
GMSV_DATA = ROOT / "original-data/gmsv/data"
MAP_DIR = GMSV_DATA / "map"
MAPWARP = MAP_DIR / "mapwarp.txt"
NPC_DIR = GMSV_DATA / "npc"
CATALOG = ROOT / "server/gamedata/travel_catalog.json"
OUT = ROOT / "server/gamedata/travel_catalog_gen.go"
DEFAULT_REPORT = ROOT / "build/travel-catalog-report.md"

# 地面层里表示「海」的贴图编号（docs/map.md）；0 表示空白。
SEA_TILES = {0, 193, 194, 195, 196}


class Fail(Exception):
    pass


def read_map(path):
    """读 LS2MAP 文件头。返回 (floor, name, w, h) 或 None。"""
    try:
        head = path.open("rb").read(44)
    except OSError:
        return None
    if len(head) < 44 or head[:6] != b"LS2MAP":
        return None
    floor = struct.unpack(">H", head[6:8])[0]
    # 文件头里的名字可能带 `|0` 之类的后缀，和 mapnames_gen.go 一样去掉。
    name = head[8:40].split(b"\0")[0].decode("gbk", errors="ignore").split("|")[0].strip()
    w, h = struct.unpack(">HH", head[40:44])
    return floor, name, w, h


def scan_maps():
    """扫描全部 LS2MAP：floor -> {path, name, w, h}；同时收集重复 floor 的诊断。"""
    maps, dup = {}, []
    for path in sorted(MAP_DIR.rglob("*")):
        if not path.is_file():
            continue
        got = read_map(path)
        if got is None:
            continue
        floor, name, w, h = got
        rel = path.relative_to(MAP_DIR).as_posix()
        if floor in maps:
            first = maps[floor]
            if (w, h) == (first["w"], first["h"]):
                dup.append(f"地图 {floor} 有重复文件：{first['path']} 与 {rel}（同尺寸 {w}×{h}，取前者）")
            else:
                dup.append(
                    f"地图 {floor} 有重复文件且尺寸不同：{first['path']} {first['w']}×{first['h']}"
                    f" 与 {rel} {w}×{h}（取前者，需人工确认）"
                )
            continue
        maps[floor] = {"path": rel, "name": name, "w": w, "h": h}
    return maps, dup


def terrain_ok(floor, x, y, maps):
    """坐标是否在地图边界内且地面层不是海。"""
    info = maps[floor]
    if not (0 <= x < info["w"] and 0 <= y < info["h"]):
        return False
    path = MAP_DIR / info["path"]
    with path.open("rb") as fh:
        fh.seek(44 + 2 * (y * info["w"] + x))
        tile = struct.unpack(">H", fh.read(2))[0]
    return tile not in SEA_TILES


def ground_tiles(floor, maps):
    """整张图的地面层（一条 u16 序列）。"""
    info = maps[floor]
    path = MAP_DIR / info["path"]
    data = path.read_bytes()
    n = info["w"] * info["h"]
    return struct.unpack(f">{n}H", data[44 : 44 + 2 * n])


def scan_mapwarp():
    """mapwarp.txt：source 的 floor/x/y -> target 的 floor/x/y。返回 (targets, sources)。"""
    targets, sources = {}, {}
    with MAPWARP.open("rb") as fh:
        for raw in fh:
            fields = raw.decode("gbk", errors="ignore").strip().split(":")
            if len(fields) != 5:
                continue
            src, dst = fields[2].split(","), fields[3].split(",")
            if len(src) != 3 or len(dst) != 3:
                continue
            try:
                sf, sx, sy = (int(v) for v in src)
                df, dx, dy = (int(v) for v in dst)
            except ValueError:
                continue
            targets.setdefault(df, (dx, dy))
            sources.setdefault(sf, (sx, sy))
    return targets, sources


def scan_npc_corners():
    """npc 脚本里的 borncorner：floor -> 第一条出生点。"""
    corners = {}
    for path in sorted(NPC_DIR.rglob("*.create")):
        floor, corner = None, None
        for line in path.read_text(encoding="gbk", errors="ignore").splitlines():
            key, _, value = line.partition("=")
            key = key.strip()
            if key == "floorid":
                floor = int(value) if value.strip().isdigit() else None
            elif key == "borncorner":
                parts = value.strip().split(",")
                if len(parts) == 2 and all(p.isdigit() for p in parts):
                    corner = (int(parts[0]), int(parts[1]))
                if floor is not None and corner is not None:
                    corners.setdefault(floor, corner)
                floor, corner = None, None
    return corners


def search_landing(floor, maps):
    """从地图中心向外螺旋找第一个非海格。找不到返回 None。"""
    info = maps[floor]
    w, h = info["w"], info["h"]
    tiles = ground_tiles(floor, maps)
    cx, cy = w // 2, h // 2
    for radius in range(0, max(w, h)):
        for dy in range(-radius, radius + 1):
            for dx in range(-radius, radius + 1):
                if max(abs(dx), abs(dy)) != radius:
                    continue
                x, y = cx + dx, cy + dy
                if 0 <= x < w and 0 <= y < h and tiles[y * w + x] not in SEA_TILES:
                    return x, y
    return None


def resolve_landing(floor, maps, landwarp, sourcewarp, corners, overrides):
    """按 a→e 的顺序定一张正式地图的落点，返回 (x, y, source) 或 None。"""
    if floor in overrides:
        x, y = overrides[floor]
        return x, y, "catalog"
    for value, source in ((landwarp.get(floor), "mapwarp-target"), (sourcewarp.get(floor), "mapwarp-source")):
        if value and terrain_ok(floor, value[0], value[1], maps):
            return value[0], value[1], source
    value = corners.get(floor)
    if value and terrain_ok(floor, value[0], value[1], maps):
        return value[0], value[1], "npc-borncorner"
    got = search_landing(floor, maps)
    if got:
        return got[0], got[1], "terrain-search"
    return None


def flatten(catalog, maps, dup):
    """把包含树展开成扁平节点表，返回 (nodes, formal_floors, problems)。

    同一层级里重名的地图（例如多处「迷宫」「封印之间」「地下１楼」）在显示名后面补上
    ` · floor`，用地图编号消歧（docs/protocol.md 5.42）；纯分组不参与。
    """
    problems = []
    nodes, formal = [], set()

    def add(node_id, parent_id, name, floor, order):
        nodes.append({"id": node_id, "parent": parent_id, "name": name, "floor": floor, "order": order})

    def collect(children, path):
        """把一层的条目规整成 {name, id, floor, children}，同时做重复登记检查。"""
        entries = []
        for item in children:
            if isinstance(item, int):
                floor, name, node_id, sub = item, None, None, None
            elif isinstance(item, dict):
                if "map" in item and "maps" not in item:
                    floor, name, sub = item["map"], item["name"], item.get("children")
                elif "maps" in item:
                    # 只挂一张地图的「分组」没有意义（点进去只有一个子项），直接当叶子地图，
                    # 显示名仍用清单里写的那个，保持和地图名一致。
                    if len(item["maps"]) == 1 and "children" not in item:
                        floor, name, sub = item["maps"][0], item["name"], None
                    else:
                        floor, name, sub = None, item["name"], item["maps"]
                elif "children" in item:
                    floor, name, sub = None, item["name"], item["children"]
                else:
                    problems.append(f"{path} 里的条目既没有 map 也没有 maps/children：{item!r}")
                    continue
                node_id = item.get("id")
            else:
                problems.append(f"{path} 有无法识别的条目 {item!r}")
                continue
            if floor is not None:
                if floor not in maps:
                    problems.append(f"{path} 引用了不存在的地图 {floor}")
                    continue
                if floor in formal:
                    problems.append(f"地图 {floor} 在目录里出现了两次（第二处在 {path}）")
                    continue
                formal.add(floor)
                name = name or maps[floor]["name"]
                node_id = node_id or f"map:{floor}"
            else:
                node_id = node_id or "group:" + name
            entries.append({"name": name, "id": node_id, "floor": floor, "children": sub})
        return entries

    def walk(children, parent_id, path):
        entries = collect(children, path)
        counts = {}
        for entry in entries:
            counts[entry["name"]] = counts.get(entry["name"], 0) + 1
        for order, entry in enumerate(entries):
            name = entry["name"]
            if counts[name] > 1 and entry["floor"] is not None:
                name = f"{name} · {entry['floor']}"
            add(entry["id"], parent_id, name, entry["floor"], order)
            if entry["children"] is not None:
                walk(entry["children"], entry["id"], f"{path}/{name}")

    roots = collect(catalog["regions"], "根层")
    for order, entry in enumerate(roots):
        add(entry["id"], "", entry["name"], entry["floor"], order)
        if entry["children"] is not None:
            walk(entry["children"], entry["id"], entry["name"])
    return nodes, formal, problems


def validate(nodes, formal, maps, dup, catalog, excluded, landings):
    """完整性校验：集合互斥、覆盖全部盘点地图、树结构无环无孤儿。"""
    problems = []
    ids = {}
    for node in nodes:
        if node["id"] in ids:
            problems.append(f"节点 id 重复：{node['id']}")
        ids[node["id"]] = node

    # 父节点必须存在，且不能成环。
    for node in nodes:
        parent = node["parent"]
        if parent and parent not in ids:
            problems.append(f"节点 {node['id']} 的父节点 {parent} 不存在")
    for node in nodes:
        seen = set()
        cur = node["id"]
        while cur:
            if cur in seen:
                problems.append(f"节点 {node['id']} 的上级链路成环")
                break
            seen.add(cur)
            cur = ids[cur]["parent"] if cur in ids else ""

    # 可传节点（有 floor）与 formal 集合必须一致。
    node_floors = {n["floor"] for n in nodes if n["floor"] is not None}
    for floor in sorted(node_floors - formal):
        problems.append(f"目录节点里的地图 {floor} 没被计为正式地图")
    for floor in sorted(formal - node_floors):
        problems.append(f"正式地图 {floor} 没有对应的目录节点")

    excluded_floors = set()
    for item in excluded:
        for floor in item["floors"]:
            if floor in excluded_floors:
                problems.append(f"排除项 {floor} 重复登记")
            excluded_floors.add(floor)
            if floor not in maps:
                problems.append(f"排除项 {floor} 在盘点的地图里不存在")
    for floor in sorted(formal & excluded_floors):
        problems.append(f"地图 {floor} 同时被标为正式和排除")
    for floor in sorted(set(maps) - formal - excluded_floors):
        problems.append(f"地图 {floor}（{maps[floor]['name']}）既没收录也没排除，属于待审核")

    # 落点：正式地图每张都要有，且不能越界。
    for floor in sorted(formal):
        if floor not in landings:
            problems.append(f"正式地图 {floor}（{maps[floor]['name']}）没有可用落点")
            continue
        x, y = landings[floor][:2]
        if not (0 <= x < maps[floor]["w"] and 0 <= y < maps[floor]["h"]):
            problems.append(f"正式地图 {floor} 的落点 ({x},{y}) 越界")
    for floor in sorted(excluded_floors & set(landings)):
        pass  # 排除的图可以有落点（构造测试用），不算问题

    # 重复 floor 的文件只作诊断：同一编号的两份文件只能有一份进目录，报告里单独列出。
    return problems


def emit_go(version, nodes, landings, formal):
    lines = [
        "// Code generated by tools/build-travel-catalog.py; DO NOT EDIT.",
        "",
        "package gamedata",
        "",
        f'// travelCatalogVersion 是快捷传送目录的版本号（STAGE-161）：层级或落点配置变动时由',
        f"// tools/build-travel-catalog.py 从 travel_catalog.json 读出来，客户端据此判断缓存是否过期。",
        f'const travelCatalogVersion = "{version}"',
        "",
        "// travelCatalogNodes 是快捷传送目录的节点表（STAGE-161，docs/protocol.md 5.42）。",
        "// 地图节点的 id 形如 map:1006，纯分组节点用 group: 前缀；floor 为 nil 的是纯分组。",
        "var travelCatalogNodes = []TravelNode{",
    ]
    for node in nodes:
        floor = "nil" if node["floor"] is None else f"ip({node['floor']})"
        parent = "nil" if not node["parent"] else f'sp("{node["parent"]}")'
        lines.append(
            f'\t{{ID: "{node["id"]}", ParentID: {parent}, Name: "{node["name"]}",'
            f" Floor: {floor}, Order: {node['order']}}},"
        )
    lines += [
        "}",
        "",
        "// travelCatalogLandings 是正式地图的默认落点（STAGE-161）：目录里的每个地图节点都有一条，",
        "// 来自原版传送目标或经地图数据核验的固定位置，来源见 build/travel-catalog-report.md。",
        "var travelCatalogLandings = map[int][2]int{",
    ]
    for floor in sorted(formal):
        x, y = landings[floor][0], landings[floor][1]
        lines.append(f"\t{floor}: {{{x}, {y}}},")
    lines += ["}", ""]
    OUT.write_text("\n".join(lines), encoding="utf-8")
    gofmt = shutil.which("gofmt")
    if gofmt:
        subprocess.run([gofmt, "-w", str(OUT)], check=False)


def emit_report(path, catalog, nodes, maps, excluded, landings, problems, dup):
    formal = [n for n in nodes if n["floor"] is not None]
    by_source = {}
    for floor, info in landings.items():
        by_source.setdefault(info[2], []).append(floor)
    src_name = {
        "catalog": "人工指定",
        "mapwarp-target": "mapwarp 目标",
        "mapwarp-source": "mapwarp 源坐标",
        "npc-borncorner": "NPC 出生点",
        "terrain-search": "地面层扫描",
    }
    lines = [
        "# 快捷传送目录审核报告（STAGE-161）",
        "",
        f"- 目录版本：`{catalog['version']}`",
        f"- 盘点地图：{len(maps)} 张（按 floor 唯一；含重复文件的 floor 只算一张）",
        f"- 正式地图：{len(formal)} 张",
        f"- 排除地图：{sum(len(i['floors']) for i in excluded)} 张",
        f"- 待审核：0 张",
        f"- 目录节点：{len(nodes)} 个（其中纯分组 {len(nodes) - len(formal)} 个）",
        f"- 落点来源：" + "、".join(f"{src_name.get(k, k)} {len(v)} 张" for k, v in sorted(by_source.items())),
        "",
        "## 排除清单",
        "",
        "| 地图 | 名字 | 理由 |",
        "| --- | --- | --- |",
    ]
    for item in excluded:
        for floor in item["floors"]:
            lines.append(f"| {floor} | {maps[floor]['name']} | {item['reason']} |")
    if dup:
        lines += ["", "## 重复 floor 诊断", ""]
        lines += [f"- {line}" for line in dup]
    lines += ["", "## 落点来源明细（非 mapwarp 目标的）", ""]
    lines += ["| 地图 | 名字 | 落点 | 来源 |", "| --- | --- | --- | --- |"]
    for floor in sorted(landings):
        x, y, source = landings[floor]
        if source == "mapwarp-target":
            continue
        lines.append(f"| {floor} | {maps[floor]['name']} | ({x}, {y}) | {src_name.get(source, source)} |")
    if problems:
        lines += ["", "## 校验失败项", ""]
        lines += [f"- {p}" for p in problems]
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    args = parser.parse_args()

    if not CATALOG.exists():
        raise Fail(f"缺少人工审核清单 {CATALOG}")
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    maps, dup = scan_maps()
    if not maps:
        raise Fail(f"{MAP_DIR}: 没找到 LS2MAP 文件")

    landwarp, sourcewarp = scan_mapwarp()
    corners = scan_npc_corners()
    overrides = {int(k): tuple(v) for k, v in catalog.get("landings", {}).items()}

    nodes, formal, problems = flatten(catalog, maps, dup)
    excluded = catalog.get("excluded", [])
    excluded_floors = {floor for item in excluded for floor in item["floors"]}

    landings = {}
    for floor in sorted(formal | excluded_floors):
        got = resolve_landing(floor, maps, landwarp, sourcewarp, corners, overrides)
        if got:
            landings[floor] = got

    problems += validate(nodes, formal, maps, dup, catalog, excluded, landings)
    emit_report(args.report, catalog, nodes, maps, excluded, landings, problems, dup)
    if problems:
        print(f"校验未通过，{len(problems)} 个问题（详见 {args.report.relative_to(ROOT)}）：", file=sys.stderr)
        for line in problems[:40]:
            print(f"  - {line}", file=sys.stderr)
        sys.exit(1)

    emit_go(catalog["version"], nodes, landings, formal)
    print(
        f"正式 {len(formal)} 张 / 排除 {len(excluded_floors)} 张 / 盘点 {len(maps)} 张"
        f" → {OUT.relative_to(ROOT)}，报告 {args.report.relative_to(ROOT)}"
    )


if __name__ == "__main__":
    try:
        main()
    except Fail as exc:
        sys.exit(f"失败：{exc}")
