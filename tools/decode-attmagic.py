#!/usr/bin/env python3
"""解析原版 data/attmagic.bin，列出攻击精灵的攻击类型和 3×5 命中掩码（STAGE-218）。

原版结构见 gmsv/include/magic_base.h 的 tagAttMagic（_ATTACK_MAGIC）：4 个 uint、14 个 int、
最后是 int[3][5] 的 siField 命中掩码。64 位平台按 4 字节对齐时每个 132 字节；文件里每个精灵
两份（敌方/玩家两套），magic.txt 的 MAGIC_IDX 对应 ATTMAGIC_magic[idx*2] 或 idx*2+1。

命中范围不是只看 magic.txt 的目标码，也不是只看 uiAttackType：
  - 目标码决定“选什么”：1/9 选敌方一个人、10 选敌方一排、11 选敌方全体；
  - 实战里 BATTLE_MultiAttMagic 用 siField 的 3×5 掩码在选中位置周围取格，
    目标码 9 以选中的人为中心，目标码 10 以选中的一排为中心，11 打满敌方两排；
  - uiAttackType（0 单人、1 整排、4 全体）只影响表现（BATTLE_AttMagicEffect 的回包写法），不决定命中格。

用法：
  python3 tools/decode-attmagic.py [attmagic.bin] [magic.txt]
默认读 original-data/gmsv/data 下的两份文件，输出 25 个攻击精灵的表。
"""
import os
import struct
import sys

SIZE = 132
MASK_OFFSET = 72  # siField 之前有 18 个 4 字节整数


def read_bin(path):
    data = open(path, "rb").read()
    if len(data) % SIZE:
        sys.exit(f"{path} 的长度 {len(data)} 不是 {SIZE} 的整数倍")
    out = []
    for off in range(0, len(data), SIZE):
        sprite, atype = struct.unpack_from("<II", data, off)
        mask = struct.unpack_from("<15i", data, off + MASK_OFFSET)
        out.append({"sprite": sprite, "atype": atype, "mask": mask})
    return out


def read_magic(path):
    rows = []
    for line in open(path, encoding="gbk"):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        f = line.split(",")
        if len(f) < 9 or f[2] != "MAGIC_AttMagic":
            continue
        rows.append({"name": f[0], "id": int(f[4]), "target": int(f[6]), "idx": int(f[8])})
    return rows


def shape(mask):
    cells = [(i, k) for i in range(3) for k in range(5) if mask[i * 5 + k]]
    if len(cells) == 1:
        return "单体格"
    if len(cells) == 3 and {i for i, _ in cells} == {1}:
        return "横 3 格"
    if len(cells) == 5 and {i for i, _ in cells} == {1}:
        return "整排 5 格"
    if len(cells) == 5 and {k for _, k in cells} == {2}:
        return "十字 5 格"
    if len(cells) == 15:
        return "整方 15 格"
    return f"{len(cells)} 格"


def hit(mask, rows, cols):
    """rows/cols 是这一格对应的掩码行列（-1 表示越界）。"""
    return {(r, c) for r, c in zip(rows, cols) if 0 <= r < 3 and 0 <= c < 5 and mask[r * 5 + c]}


def counts(target, mask):
    """在敌方两排五列里，按原版 BATTLE_MultiAttMagic 的取格方式数命中数。"""
    if target == 11:  # 全体：toNo 20/21，遍历敌方两排，用掩码的第 0、1 行
        return [sum(1 for r in range(2) for c in range(5) if mask[r * 5 + c])]
    if target == 10:  # 一排：toNo 23–26，选中排是掩码第 1 行，上一排/下一排用第 0/2 行
        out = []
        for tr in range(2):
            got = set()
            for mr, dr in ((0, -1), (1, 0), (2, 1)):
                rr = tr + dr
                if not 0 <= rr < 2:
                    continue
                for c in range(5):
                    if mask[mr * 5 + c]:
                        got.add((rr, c))
            out.append(len(got))
        return out
    # 1/9：选一个人，选中格是掩码第 1 行的第 2 列，周围按 3×5 取格
    out = []
    for tr in range(2):
        for tc in range(5):
            got = set()
            for mr in range(3):
                rr = tr + (mr - 1)
                if not 0 <= rr < 2:
                    continue
                for mc in range(5):
                    cc = tc + (mc - 2)
                    if 0 <= cc < 5 and mask[mr * 5 + mc]:
                        got.add((rr, cc))
            out.append(len(got))
    return out


def main(argv):
    root = "original-data/gmsv/data"
    binpath = argv[1] if len(argv) > 1 else os.path.join(root, "attmagic.bin")
    txtpath = argv[2] if len(argv) > 2 else os.path.join(root, "magic.txt")
    entries = read_bin(binpath)
    print(f"{binpath}: {len(entries)} 条，{len(entries) // 2} 个精灵")
    print()
    print("| 编号 | 名字 | 目标码 | 掩码 | uiAttackType | 原版命中（最少–最多） |")
    print("| --- | --- | --- | --- | --- | --- |")
    for m in read_magic(txtpath):
        idx = m["idx"]
        if idx * 2 + 1 >= len(entries):
            sys.exit(f"{m['name']} 的 idx {idx} 超出 bin 的条数")
        e = entries[idx * 2]  # 敌方那份；两份的掩码相同
        got = entries[idx * 2 + 1]
        if e["mask"] != got["mask"] or e["atype"] != got["atype"]:
            sys.exit(f"{m['name']}（idx {idx}）两份数据不一致")
        ns = counts(m["target"], e["mask"])
        span = f"{min(ns)}" if min(ns) == max(ns) else f"{min(ns)}–{max(ns)}"
        print(f"| {m['id']} | {m['name']} | {m['target']} | {shape(e['mask'])} | {e['atype']} | {span} |")
    print()
    print("目标码（gmsv/include/magic_base.h）：1=他人、9=敌方某一个、10=敌方某一排、11=敌方所有人。")
    print("uiAttackType（magic_base.h）：0=单人、1=整排、4=全体；只影响表现，命中格由 siField 决定。")


if __name__ == "__main__":
    main(sys.argv)