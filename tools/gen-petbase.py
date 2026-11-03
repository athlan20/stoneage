#!/usr/bin/env python3
"""生成宠物模板表与宠物技能表（STAGE-31，docs/ui.md 宠物页）。

输出两个 UTF-8 的 TSV，编译进服务端（server/gamedata）：
- server/gamedata/petbase.tsv：enemybase.txt 的宠物模板（952 行，含 PETFLG 为 0 的非宠物）；
- server/gamedata/petskill.tsv：petskill.txt 的技能名。

列的编号与 internal/gamecatalog/parse.go 的 ParsePets、ParsePetSkills 一致，并与
gmsv/include/enemy.h 的 ENEMYTEMP_DATAINT 对照：整数列从第 6 列开始（E_T_TEMPNO = 第 6 列），
每个整数列的下标 = 6 + 枚举值。数值一律用 atoi 的规则（取开头的整数，小数部分舍去），
和原版服务端 ENEMYTEMP_setInt(atoi) 一致，例如 LVUPPOINT 写的 4.50 按 4 算。

空的技能栏位写 -1（原版服务端 atoi 会变成 0，即“待机”；这里保留“没有”和“待机”的区别）。

宠物的固定原子（STAGE-205，docs/synthesis.md 第 2、6 节）写到第三个文件 server/gamedata/petatom.tsv：
每个固定原子一行（模板编号、序号 1–5、原子名、BASEADD、FIXMIN、FIXMAX、在原子表里 1/0）。原子名不在原子表里的
（原版 ITEM_merge_getPetFix 打印 "fucking atom" 后跳过）照原样保留、标 0，不丢：合成时按标记跳过，并在输出里列出来。

用法：python3 tools/gen-petbase.py [数据目录]
不传目录时用 original-data/gmsv/data。
"""

import re
import sys
from pathlib import Path

from atomtable import load_atoms

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_DATA = ROOT / "original-data/gmsv/data"
OUT_DIR = ROOT / "server/gamedata"

# enemybase.txt 的列（枚举 ENEMYTEMP_DATAINT 的下标 + 6）。
NAME = 0
TEMPNO = 6
INITNUM = 7
LVUPPOINT = 8
BASEVITAL, BASESTR, BASETGH, BASEDEX = 9, 10, 11, 12
MODAI = 13
GET = 14  # E_T_GET：捕捉难度（BATTLE_CaptureCheck 的 Df_Ge，越小越好抓）
EARTHAT, WATERAT, FIREAT, WINDAT = 15, 16, 17, 18
PETSKILL_FIRST, PETSKILL_COUNT = 25, 7
RARE = 32  # E_T_RARE：稀有度 0/1/2，宠物店卖价用（不是品质，品质是 CHAR_PETRANK 由资质算）
SLOT = 35
IMGNUMBER = 36
PETFLG = 37
SIZE = 38
LIMITLEVEL = 54
# 固定原子（E_T_ATOMFIXNAME1..5 是第 1–5 列；E_T_ATOMBASEADD1 起每组三列 BASEADD、FIXMIN、FIXMAX，第 39+3(n−1) 列起）。
FIXED_NAME_FIRST = 1
FIXED_FIRST = 39

# petskill.txt 的列（与 gamecatalog 的 petSkill*Field 一致）。
SKILL_NAME = 0
SKILL_COMMENT = 1  # 说明（PETSKILL_COMMENT），宠物技能店的列表里显示
SKILL_ID = 6
SKILL_FIELD = 7   # 场合（PETSKILL_FIELD）：1 战斗中用，2 战斗外（地图上）用
SKILL_TARGET = 8  # 目标（PETSKILL_TARGET，原版 PETSKILL_TARGETTYPE 的枚举值）
SKILL_COST = 10   # 学习价格（PETSKILL_COST）：宠物技能店的收费，乘店的 skill_rate（STAGE-143）

INT_PREFIX = re.compile(r"\s*([+-]?\d+)")


def atoi(text: str) -> int:
    """与 C 的 atoi 相同：取开头的整数，开头不是数字时为 0。"""
    m = INT_PREFIX.match(text)
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


def clean_name(text: str) -> str:
    return " ".join(text.split())


def parse_petbase(path: Path):
    seen = {}
    for lineno, fields in read_rows(path):
        if len(fields) <= LIMITLEVEL:
            raise SystemExit(f"{path}:{lineno}: 列数不够（{len(fields)}）")
        name = clean_name(fields[NAME])
        if not name:
            raise SystemExit(f"{path}:{lineno}: 名字为空")
        tempno = atoi(fields[TEMPNO])
        if tempno in seen:
            raise SystemExit(f"{path}:{lineno}: 模板编号 {tempno} 重复")
        skills = []
        for i in range(PETSKILL_COUNT):
            cell = fields[PETSKILL_FIRST + i].strip()
            skills.append(str(atoi(cell)) if cell else "-1")
        seen[tempno] = [
            str(tempno),
            name,
            str(atoi(fields[IMGNUMBER])),
            str(atoi(fields[SLOT])),
            str(atoi(fields[PETFLG])),
            str(atoi(fields[SIZE])),
            str(atoi(fields[LIMITLEVEL])),
            str(atoi(fields[INITNUM])),
            str(atoi(fields[LVUPPOINT])),
            str(atoi(fields[BASEVITAL])),
            str(atoi(fields[BASESTR])),
            str(atoi(fields[BASETGH])),
            str(atoi(fields[BASEDEX])),
            str(atoi(fields[MODAI])),
            str(atoi(fields[GET])),
            str(atoi(fields[EARTHAT])),
            str(atoi(fields[WATERAT])),
            str(atoi(fields[FIREAT])),
            str(atoi(fields[WINDAT])),
            *skills,
            str(atoi(fields[RARE])),
        ]
    return [seen[k] for k in sorted(seen)]


def parse_petskill(path: Path):
    seen = {}
    for lineno, fields in read_rows(path):
        if len(fields) <= SKILL_TARGET:
            raise SystemExit(f"{path}:{lineno}: 列数不够（{len(fields)}）")
        name = clean_name(fields[SKILL_NAME])
        skill_id = atoi(fields[SKILL_ID])
        if not name:
            raise SystemExit(f"{path}:{lineno}: 名字为空")
        if skill_id in seen:
            raise SystemExit(f"{path}:{lineno}: 技能编号 {skill_id} 重复")
        fn = fields[2] if len(fields) > 2 else ""
        arg = fields[3] if len(fields) > 3 else ""
        # 原版 pet_skill.c：行首是 T 的技能标为 PETSKILL_ILLEGAL，不在宠物技能店上架。
        illegal = fields[SKILL_NAME].startswith("T")
        seen[skill_id] = [str(skill_id), name, fn, arg,
                          str(atoi(fields[SKILL_FIELD])), str(atoi(fields[SKILL_TARGET])),
                          str(atoi(fields[SKILL_COST])), clean_name(fields[SKILL_COMMENT]),
                          "1" if illegal else "0"]
    return [seen[k] for k in sorted(seen)]


def parse_petatom(path: Path, atom_names: set[str]):
    """返回 (行, 原子表里没有的 [(模板编号, 原子名)])。行是 [模板编号, 序号, 原子名, BASEADD, FIXMIN, FIXMAX, 在原子表里]。"""
    rows, missing = [], []
    for lineno, fields in read_rows(path):
        if len(fields) <= FIXED_FIRST + 3 * 5 - 1:
            raise SystemExit(f"{path}:{lineno}: 列数不够（{len(fields)}）")
        tempno = atoi(fields[TEMPNO])
        for n in range(1, 6):
            name = fields[FIXED_NAME_FIRST + n - 1].strip()
            if not name:
                continue
            known = name in atom_names
            if not known:
                missing.append((tempno, name))
            base = FIXED_FIRST + 3 * (n - 1)
            rows.append([str(tempno), str(n), name,
                         str(atoi(fields[base])), str(atoi(fields[base + 1])), str(atoi(fields[base + 2])),
                         "1" if known else "0"])
    return rows, missing


def write_tsv(out: Path, header: str, rows):
    body = "\n".join("\t".join(row) for row in rows)
    out.write_text(f"# generated by tools/gen-petbase.py; DO NOT EDIT. {header}\n{body}\n", encoding="utf-8")


def main():
    data = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_DATA
    petbase = parse_petbase(data / "enemybase.txt")
    petskill = parse_petskill(data / "petskill.txt")
    atom_names = {name for name, _, _ in load_atoms(data / "itematom.txt")}
    petatom, missing = parse_petatom(data / "enemybase.txt", atom_names)
    write_tsv(
        OUT_DIR / "petbase.tsv",
        "列：模板编号 名字 图像编号 技能栏位数 PETFLG 体型 等级上限 INITNUM LVUPPOINT "
        "资质体力 资质腕力 资质耐久 资质速度 MODAI 捕捉难度 地 水 火 风 技能1..技能7（-1 为空） 稀有度",
        petbase,
    )
    write_tsv(OUT_DIR / "petskill.tsv",
              "列：技能编号\t名字\t函数\t参数\t场合（1 战斗、2 战斗外）\t目标码（PETSKILL_TARGET）\t学习价格\t说明\t非法（1 不上架，原版行首 T）",
              petskill)
    write_tsv(OUT_DIR / "petatom.tsv",
              "列：模板编号 序号（1–5，即原版 E_T_ATOMFIXNAME1..5） 原子名 BASEADD FIXMIN FIXMAX 在原子表里（1 是，0 否：原版数据里的缺口，合成时跳过）",
              petatom)
    print(f"{data}: {len(petbase)} 个宠物模板、{len(petskill)} 个宠物技能、{len(petatom)} 条固定原子 → {OUT_DIR}")
    if missing:
        names = "、".join(f"{t}:{n}" for t, n in missing)
        print(f"诊断: {len(missing)} 条固定原子的名字不在原子表里（原版同样跳过），标记为 0：{names}")


if __name__ == "__main__":
    main()
