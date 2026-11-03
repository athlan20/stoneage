#!/usr/bin/env python3
"""生成 server/gamedata/itemset.tsv：物品编号 → 名字、效果文字、类型（UTF-8）。

数据来自原版服务端的 gmsv/data/itemset.txt（GBK）。列的编号与
internal/gamecatalog/parse.go 的 ParseItems 一致：名字是第 0 列，效果文字是第 2 列，
物品编号（ITEM_ID）是第 16 列，类型（ItemType）是第 19 列，等级（ITEM_LEVEL）是第 22 列。

穿戴用的数值（STAGE-29，docs/ui.md 背包页 B）：攻击取第 37 列（ITEM_MODIFYATTACK），防御取第 39 列，
敏捷取第 41 列。这三项在数据里各有两列写同一个值（37/38、39/40、41/42），效果文字里的“攻 +9 防 -3 敏 -3”
与第一列对得上（统计：攻 858 条、防 850 条、敏 490 条）。
运气修正取第 47 列（ITEM_MODIFYLUCK），魅力修正取第 49 列（ITEM_MODIFYCHARM，STAGE-61）：第 49 列与效果文字
“魅 +N”在无问号的 422 条里一致 411 条（其余是“吃了会很有魅力”这类消耗品，不穿戴，不生效）；
运气列只出现在签和许愿池这类非装备物品上。地水火风暂不从装备推算。
暴击修正取第 75 列（ITEM_CRITICAL，武器加成暴击率，STAGE-60 用）。
装备自带的精灵（STAGE-190）：精灵编号取第 55 列（ITEM_MAGICID，对应 magic.tsv 的编号，-1/空表示没有），
施放所需 MP 取第 57 列（ITEM_MAGICUSEMP，以物品为准，不用魔法表里的 MP）。第 56 列 ITEM_MAGICPROB 原版服务端没有任何地方用到，不收。
效果文字里的“滋润的精灵 Lv1”只是显示；7970 条带精灵编号的物品里编号与文字对得上 7112 条，其余是文字措辞不同（如“睡眠/昏睡”）。
腕力、速度、转生的穿戴要求（ITEM_NEEDSTR 等）来自原版服务端的 itemset4.txt，仓库里的数据目录没有这个文件，
所以这里不生成，穿戴时只检查等级。

使用物品的字段（STAGE-30，docs/protocol.md 5.29）：使用函数取第 10 列（ITEM_USEFUNC），
使用参数取第 3 列（ITEM_ARGUMENT，如 “体40”“毒”），可使用场合取第 20 列（ITEM_ABLEUSEFIELD：
0 都可以、1 战斗中、2 战斗外），目标取第 21 列（ITEM_TARGET：0 自己、1 他人、2 全己方、3 全敌方、4 全体）。

合成的字段（STAGE-205，docs/synthesis.md）：可作材料取第 82 列，可作配方取第 83 列，原子名与数值取第 84–93 列
（5 组，空名字表示没有）。原子名要在原子表（tools/gen-itematom.py）里，由服务端加载时校验。

耐久（STAGE-209，docs/durability.md）：当前耐久取第 30 列（ITEM_DAMAGECRUSHE），最大耐久取第 31 列（ITEM_MAXDAMAGECRUSHE），
追加在 TSV 最后两列；最大耐久为 0 或空的物品不损坏。

精工（STAGE-297，docs/inlay.md 2.4）：精工类别码取第 4 列（ITEM_TYPECODE，字符串），初始镶嵌码取第 5 列
（ITEM_INLAYCODE，字符串）；伤取第 32 列（ITEM_OTHERDAMAGE），吸取第 33 列（ITEM_OTHERDEFC）；装备的 HP、MP 修正
取第 43、45 列（ITEM_MODIFYHP / ITEM_MODIFYMP）。原版的 int 字段成对出现（攻 37/38、防 39/40、敏 41/42、
HP 43/44、MP 45/46、运 47/48、魅 49/50），HP、MP 的列号按 item.h 的枚举顺序推出（敏 41、运 47、魅 49 已对得上效果文字）；
item.c 的 ITEM_setintdata 表里 mh/mm/mq 的顺序与枚举不同，以枚举为准。
数据里这些列大多是空的（HP 只有 7 条水灵、风灵项链，MP 全空，伤、吸全空），精工没启用，新列只是存下来。
追加在 TSV 最后六列（精工类别码、初始镶嵌码、伤、吸、HP、MP），前面各列位置不变。

用法：python3 tools/gen-itemset.py [itemset.txt 路径]
不传路径时用仓库里的 original-data/gmsv/data/itemset.txt。
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_INPUT = ROOT / "original-data/gmsv/data/itemset.txt"
OUTPUT = ROOT / "server/gamedata/itemset.tsv"

# COST 是 int 字段第 3 个（item.h 的 ITEM 枚举：ID、BASEIMAGENUMBER、COST…），对应 itemset.txt
# 的第 18 列（ID=16、BASEIMAGE=17、COST=18、TYPE=19…），是商店买卖的基价（STAGE-70，npc_itemshop.c 的
# ITEM_getcostFromITEMtabl）。
NAME, EFFECT, USEFUNC, ARGUMENT, ITEM_ID, BASEIMAGE, COST, TYPE, ABLEUSE, TARGET, LEVEL = 0, 2, 10, 3, 16, 17, 18, 19, 20, 21, 22
ATTACK, DEFENCE, AGILITY, LUCK, CHARM, CRITICAL = 37, 39, 41, 47, 49, 75
MAGICID, MAGICUSEMP = 55, 57
# 合成（STAGE-205，docs/synthesis.md 第 2 节）：第 82 列 canmergefrom（能作材料）、第 83 列 canmergeto（能作配方产物）、
# 第 84+2i 列原子名、第 85+2i 列原子数值（i = 0–4，原版 ITEM_INGNAME0..4 / ITEM_INGVALUE0..4）。
MERGEFROM, MERGETO, INGNAME0, INGVALUE0 = 82, 83, 84, 85
# STAGE-209：耐久（原版 ITEM_DAMAGECRUSHE / ITEM_MAXDAMAGECRUSHE，第 30、31 列；枚举顺序与列号的对应见 docs/durability.md）。
# 两列追加在 TSV 最后，前面各列的位置不变。
DURA, MAXDURA = 30, 31
# STAGE-297：精工（docs/inlay.md 2.4）。TYPECODE、INLAYCODE 是字符串列；伤、吸、HP、MP 是 int 列。
TYPECODE, INLAYCODE, OTHERDAMAGE, OTHERDEFC, MODIFYHP, MODIFYMP = 4, 5, 32, 33, 43, 45


def parse(path: Path):
    seen = {}
    for lineno, raw in enumerate(path.read_bytes().splitlines(), 1):
        line = raw.rstrip(b"\r").decode("gbk")
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        fields = line.split(",")
        if len(fields) <= MAGICUSEMP:
            raise SystemExit(f"{path}:{lineno}: 列数不够（{len(fields)}）")
        try:
            item_id = int(fields[ITEM_ID].strip())
            item_type = int(fields[TYPE].strip())
            cost = num(fields, COST)
            level, attack, defence, agility, luck, charm, critical = (
                num(fields, i) for i in (LEVEL, ATTACK, DEFENCE, AGILITY, LUCK, CHARM, CRITICAL))
            ableuse, target = (num(fields, i) for i in (ABLEUSE, TARGET))
            magic_id = int(fields[MAGICID].strip()) if fields[MAGICID].strip() else -1
            magic_mp = num(fields, MAGICUSEMP)
            merge = [num(fields, MERGEFROM), num(fields, MERGETO)]
            for i in range(5):
                merge += [fields[INGNAME0 + 2 * i].strip(), num(fields, INGVALUE0 + 2 * i)]
            dura = [num(fields, DURA), num(fields, MAXDURA)]
            inlay = [fields[TYPECODE].strip(), fields[INLAYCODE].strip()]
            inlay += [num(fields, i) for i in (OTHERDAMAGE, OTHERDEFC, MODIFYHP, MODIFYMP)]
        except ValueError as e:
            raise SystemExit(f"{path}:{lineno}: 数字列解析失败: {e}")
        name = " ".join(fields[NAME].split())
        effect = " ".join(fields[EFFECT].split())
        usefunc = fields[USEFUNC].strip()
        argument = fields[ARGUMENT].strip()
        if not name:
            raise SystemExit(f"{path}:{lineno}: 名字为空")
        if item_id in seen:
            raise SystemExit(f"{path}:{lineno}: 物品编号 {item_id} 重复")
        seen[item_id] = (name, effect, item_type, cost, level, attack, defence, agility, luck, charm, critical,
                         usefunc, argument, ableuse, target, magic_id, magic_mp, *merge, *dura, *inlay)
    return seen


def num(fields, i):
    """数值列：空白当作 0。"""
    text = fields[i].strip()
    return int(text) if text else 0


def main():
    src = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_INPUT
    items = parse(src)
    rows = sorted(items.items())
    out = ["\t".join([str(item_id), name, effect, str(item_type), *map(str, rest)])
           for item_id, (name, effect, item_type, *rest) in rows]
    OUTPUT.write_text(
        "# generated by tools/gen-itemset.py; DO NOT EDIT. 列：物品编号\\t名字\\t效果\\t类型\\t基价\\t等级\\t攻击\\t防御\\t敏捷\\t运气\\t魅力\\t暴击修正"
        "\\t使用函数\\t使用参数\\t可使用场合\\t目标\\t精灵编号\\t精灵MP"
        "\\t可作材料\\t可作配方\\t原子1\\t原子1数值\\t原子2\\t原子2数值\\t原子3\\t原子3数值\\t原子4\\t原子4数值\\t原子5\\t原子5数值\\t当前耐久\\t最大耐久"
        "\\t精工类别码\\t初始镶嵌码\\t伤\\t吸\\tHP\\tMP\n"
        + "\n".join(out) + "\n",
        encoding="utf-8",
    )
    print(f"{src}: {len(rows)} 条物品 → {OUTPUT}")


if __name__ == "__main__":
    main()
