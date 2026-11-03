#!/usr/bin/env python3
"""生成遇敌数据表（STAGE-40，docs/protocol.md 第 10 章）。

从原版服务端的四张 GBK 表生成三个 UTF-8 的 TSV，编译进服务端（server/gamedata）：
- encounts.tsv：遇敌区域（encount.txt 的行，去掉用不到的概率与事件列）；
- enemygroups.tsv：敌群（setup.cf 的 groupfile 指向 group1.txt，不是 group.txt）；
- enemies.tsv：遇敌用的敌人（enemy.txt 的行，含等级、同场上限、经验、掉落，
  以及 enemy.c 里 RandomEnemyTbl 的随机替换候选）。

字段顺序与含义对照 gmsv/char/enemy.c 的 ENEMY_initEnemy / ENEMY_initGroup /
ENCOUNT_initEncount（enemey.h 的枚举），以及 internal/aiknowledge 的 coreparse.go：
- encount.txt：区域编号、地图、x1、y1、x2、y2、CEP 上下限（STAGE-152 起保留）、最多敌人数、
  zorder、10 个敌群编号、10 个敌群权重、3 个事件字段（不用）；
- group1.txt：敌群名、编号、持有才出现道具、持有不出现道具、10 个敌人编号、10 个生成权重；
- enemy.txt：敌人名、战术串（怪物 AI 的选项）、行为条件（不用）、编号、模板、等级下上限、
  同场上限/下限、战术模式（1 = ai_normal）、经验、DP（不用）、Style（武器样式，1–7 给怪物
  配对应武器）、PetFlag、10 个掉落物品、10 个掉落概率。

处理规则（都按原版加载器的行为）：
- 数值一律 atoi（取开头的整数）；等级下限 0 时取上限，然后小大排序（ENEMY_initEnemy）；
- 敌群与敌人的空槽写 -1；
- enemies.tsv 的 random_of 列：enemy.c 的 ENEMY_RandomEnemyArray 会把编号 945–956、
  964–969 的敌人换成 RandomEnemyTbl 里的随机一个候选，这里把候选表展开成
  逗号分隔的一列（空表示普通敌人）；战术串与样式按原文（GBK 转 UTF-8）；
- encounts.tsv 保持原文件顺序（重叠区域平局时文件靠前的行胜出，ENCOUNT_getEncountAreaArray）。

用法：python3 tools/gen-encounters.py [数据目录]
不传目录时用 original-data/gmsv/data。
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_DATA = ROOT / "original-data/gmsv/data"
OUT_DIR = ROOT / "server/gamedata"
ITEMSET = OUT_DIR / "itemset.tsv"

# enemy.c 的 RandomEnemyTbl：编号 → 候选敌人编号列表。
RANDOM_ENEMY = {
    945: [739, 742, 745, 748],
    946: [859, 860, 861, 868, 869, 870, 881, 882, 883, 884],
    947: [740, 743, 746, 749],
    948: [864, 871, 872, 873, 885, 886, 889, 890, 891, 892, 893, 894],
    949: [741, 744, 747, 750],
    950: [862, 863, 865, 866, 867, 874, 875, 876, 877, 878, 879, 880],
    951: [895, 898, 901, 904],
    952: [912, 913, 914, 915, 916, 917, 918, 919, 920, 928],
    953: [896, 899, 902, 905],
    954: [929, 930, 931, 932, 933, 934, 935, 936, 937, 938, 939, 940],
    955: [897, 900, 903, 906],
    956: [907, 908, 909, 910, 911, 921, 922, 923, 924, 925, 926, 927],
    964: [859, 860, 861, 868, 869, 870, 881, 882, 883, 884],
    965: [864, 871, 872, 873, 885, 886, 889, 890, 891, 892, 893, 894],
    966: [862, 863, 865, 866, 867, 874, 875, 876, 877, 878, 879, 880],
    967: [912, 913, 914, 915, 916, 917, 918, 919, 920, 928],
    968: [929, 930, 931, 932, 933, 934, 935, 936, 937, 938, 939, 940],
    969: [907, 908, 909, 910, 911, 921, 922, 923, 924, 925, 926, 927],
}


def atoi(text: str) -> int:
    """与 C 的 atoi 相同：取开头的整数，开头不是数字时为 0。"""
    m = text and __import__("re").match(r"\s*([+-]?\d+)", text)
    return int(m.group(1)) if m else 0


def read_rows(path: Path):
    for lineno, raw in enumerate(path.read_bytes().splitlines(), 1):
        line = raw.rstrip(b"\r")
        if lineno == 1:
            line = line.removeprefix(b"\xef\xbb\xbf")
        text = line.decode("gbk")
        if not text.strip() or text.lstrip().startswith("#"):
            continue
        yield lineno, text.split(",")


def opt(text: str, default: int) -> int:
    """空槽：encount/group 的敌群、敌人槽空时按原版记 -1。"""
    return atoi(text) if text.strip() else default


def parse_encounts(path: Path):
    """解析 encount.txt，并给出与原版加载器一致的诊断。

    STAGE-152 起多保留：CEP 上下限（静态判断“永不遇敌的安全区”）、原始行号与原始序号
    （区域编号 430/639 在不同地图上重复，需要能区分），以及一个稳定区域 ID。
    """
    rows, diag = [], []
    seen_exact: dict = {}
    base_ids: dict = {}
    seq = 0
    for lineno, f in read_rows(path):
        if len(f) != 33:
            raise SystemExit(f"{path}:{lineno}: encount 行应有 33 列，实际 {len(f)}")
        orig, floor = atoi(f[0]), atoi(f[1])
        x1, y1, x2, y2 = (atoi(f[i]) for i in (2, 3, 4, 5))
        cep_a, cep_b = atoi(f[6]), atoi(f[7])
        maxnum, zorder = atoi(f[8]), atoi(f[9])
        # 原版 ENCOUNT_initEncount 对这两种行整行跳过（只打一行日志）。
        if not 1 <= maxnum <= 10:
            diag.append(f"{path.name}:{lineno}: 最多敌人数 {maxnum} 不在 1–10，整行跳过")
            continue
        slots = []
        for i in range(10):
            slots += [str(opt(f[10 + i], -1)), str(opt(f[20 + i], -1))]
        gids = [g for g in slots[0::2] if g != "-1"]
        if len(gids) != len(set(gids)):
            diag.append(f"{path.name}:{lineno}: 敌群编号重复，整行跳过（原版 checkRedundancy）")
            continue
        exact = (floor, orig, x1, y1, x2, y2, tuple(slots))
        if exact in seen_exact:
            diag.append(f"{path.name}:{lineno}: 与第 {seen_exact[exact]} 行完全相同，去重")
            continue
        seen_exact[exact] = lineno
        # 稳定 ID：地图 + 原编号 + 矩形；同键再来一条时追加序号，不会串号。
        base = f"{floor}-{orig}-{x1}-{y1}-{x2}-{y2}"
        n = base_ids.get(base, 0) + 1
        base_ids[base] = n
        aid = base if n == 1 else f"{base}-{n}"
        rows.append(
            [
                aid, str(orig), str(floor),
                str(x1), str(y1), str(x2), str(y2),
                str(zorder), str(maxnum),
                str(min(cep_a, cep_b)), str(max(cep_a, cep_b)),
                str(lineno), str(seq), *slots,
            ]
        )
        seq += 1
    return rows, diag


# 敌群 780（500 五十年前波拉岛 659/660/661 的蓝雷龙 Lv105–120）原版要持有 18557 涛之圣石才出现；
# 本服去掉这个条件，直接可遇（编号 → 覆盖后的“持有才出现道具”）。
#
# STAGE-350：敌群 414（敌人 563“木”，良木 2468 的唯一来源）原版要持有 2478 采伐许可证才出现。许可证全角色只有一张，
# 交给涛（任务 45）后就没了，打不出木，赛尚的手环（STAGE-349 起涛后不要许可证）就换不到。这里同样去掉条件。
# 良木只有赛尚用：石币块仍要许可证，手环块一次性（事件 187），去掉条件不会多出刷奖路径。
APPEAR_ITEM_OVERRIDE = {780: -1, 414: -1}


def parse_groups(path: Path):
    rows = []
    for lineno, f in read_rows(path):
        if len(f) != 24:
            raise SystemExit(f"{path}:{lineno}: group 行应有 24 列，实际 {len(f)}")
        slots = []
        for i in range(10):
            slots += [str(opt(f[4 + i], -1)), str(opt(f[14 + i], -1))]
        appear = APPEAR_ITEM_OVERRIDE.get(int(f[1]), f[2])
        rows.append([f[1], str(appear), f[3], *slots])
    return rows


def parse_enemies(path: Path):
    rows = []
    for lineno, f in read_rows(path):
        if len(f) != 34:
            raise SystemExit(f"{path}:{lineno}: enemy 行应有 34 列，实际 {len(f)}")
        nums = f[3:]  # 3 个字符串列之后是 31 个整数列
        lv_min, lv_max = atoi(nums[2]), atoi(nums[3])
        if lv_min == 0:  # ENEMY_initEnemy：下限 0 时取上限
            lv_min = lv_max
        lv_min, lv_max = min(lv_min, lv_max), max(lv_min, lv_max)
        drops = []
        for i in range(10):
            drops += [str(opt(nums[11 + i], -1)), str(opt(nums[21 + i], 0))]
        rand = RANDOM_ENEMY.get(atoi(nums[0]), [])
        rows.append(
            [
                str(atoi(nums[0])),
                str(atoi(nums[1])),
                str(lv_min),
                str(lv_max),
                str(atoi(nums[4])),
                str(atoi(nums[5])),
                str(atoi(nums[7])),
                str(atoi(nums[10])),
                *drops,
                ",".join(str(x) for x in rand),
                " ".join(f[1].split()),  # 战术串（怪物 AI 选项，STAGE-60）
                str(atoi(nums[6])),      # 战术模式：1 = ai_normal
                str(atoi(nums[9])),      # 武器样式：1–7 给怪物配武器
            ]
        )
    return rows


def write_tsv(out: Path, header: str, rows):
    body = "\n".join("\t".join(row) for row in rows)
    out.write_text(
        f"# generated by tools/gen-encounters.py; DO NOT EDIT. {header}\n{body}\n",
        encoding="utf-8",
    )


def missing_drop_items(enemies):
    """掉落表（生效的掉落位，概率不为 0）引用了 itemset.tsv 里没有的编号：返回 {编号: 处数}。

    服务端发放掉落时会跳过这些编号（STAGE-191），这里只报出来，数据要不要改由人决定。"""
    if not ITEMSET.exists():
        return None
    known = {int(line.split("\t")[0]) for line in ITEMSET.read_text(encoding="utf-8").splitlines() if line[:1].isdigit()}
    missing = {}
    for row in enemies:
        for i in range(10):
            item, pct = int(row[8 + 2 * i]), int(row[9 + 2 * i])
            if item >= 0 and pct != 0 and item not in known:
                missing[item] = missing.get(item, 0) + 1
    return missing


def main():
    data = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_DATA
    encounts, diag = parse_encounts(data / "encount.txt")
    groups = parse_groups(data / "group1.txt")
    enemies = parse_enemies(data / "enemy.txt")
    write_tsv(
        OUT_DIR / "encounts.tsv",
        "遇敌区域，保持原文件顺序（重叠平局取靠前）。列：区域ID 区域编号 地图 x1 y1 x2 y2 zorder "
        "最多敌人数 CEP下限 CEP上限 原始行号 原始序号 敌群1 权重1 .. 敌群10 权重10"
        "（-1 为空；zorder≤0 的行运行时忽略；CEP 上限 0 = 永不遇敌的安全区）",
        encounts,
    )
    write_tsv(
        OUT_DIR / "enemygroups.tsv",
        "敌群（setup.cf 的 group1.txt）。列：敌群编号 持有才出现道具 持有不出现道具 "
        "敌人1 生成权重1 .. 敌人10 生成权重10（-1 为空）",
        groups,
    )
    write_tsv(
        OUT_DIR / "enemies.tsv",
        "遇敌用的敌人（enemy.txt，等级已按原版归一）。列：敌人编号 模板编号 等级下限 等级上限 "
        "同场上限 同场下限 经验 PetFlag 掉落1 概率1 .. 掉落10 概率10 随机替换候选（逗号分隔，空为普通敌人） "
        "战术串 战术模式 武器样式",
        enemies,
    )
    print(f"{data}: {len(encounts)} 个遇敌区域、{len(groups)} 个敌群、{len(enemies)} 个敌人 → {OUT_DIR}")
    for line in diag:
        print(f"诊断: {line}")
    missing = missing_drop_items(enemies)
    if missing is None:
        print("诊断: 没有 itemset.tsv，跳过掉落编号检查（先运行 tools/gen-itemset.py）")
    elif missing:
        top = "、".join(f"{k}×{v}" for k, v in sorted(missing.items(), key=lambda kv: -kv[1])[:10])
        print(f"诊断: 掉落表引用了物品表里没有的编号 {len(missing)} 种（{sum(missing.values())} 处生效的掉落），"
              f"服务端不发放：{top}")


if __name__ == "__main__":
    main()
