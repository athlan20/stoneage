#!/usr/bin/env python3
"""从原版服务端的 gmsv/data/magic.txt 生成 server/gamedata/magic.tsv。

magic.txt 每行（列序见 gmsv/include/magic_base.h 的枚举）：
名字,说明,函数,参数,ID,使用场所,目标,复活旗标,图标。
收进来的函数：
  MAGIC_AttMagic（攻击，STAGE-48）、MAGIC_Recovery（自愈）、MAGIC_OtherRecovery（治疗他人）；
  MAGIC_StatusChange（状态，STAGE-77）、MAGIC_StatusRecovery（解状态）、MAGIC_Ressurect（复活）；
  MAGIC_FieldAttChange（场地属性，STAGE-200）、MAGIC_AttReverse（属性反转，STAGE-200）。
  MAGIC_MagicDef（单人防御，STAGE-199：光=吸收、镜子=反射、守护=无效）、MAGIC_ResAndDef（复活 + 防御）。
  魔障、沉默、虚弱（STAGE-201）的函数 MAGIC_Barrier、MAGIC_Nocast、MAGIC_Weaken 也按状态精灵收录。
  MAGIC_MagicStatusChange（魔法防御与冰、电、火之精灵，STAGE-228：参数是“魔抗|3|20|单”，状态|回合|数值|单或全）。
  MAGIC_Metamo（变幻的精灵，STAGE-203：变身成自己的宠物，参数是秒数，战斗外使用）。
  MAGIC_ToCallDragon（召唤龙阵，STAGE-202：奔龙阵、强恩阵、奔雷阵、GM 烟火，参数是“图像号|威力”）、
  MAGIC_AttSkill（精灵版宠物技能，STAGE-202：参数是“技能函数;技能编号”，只留技能编号）。

原版没有在 magic.txt 里给 MP 消耗：MP 挂在物品上（物品表 ITEM_MAGICUSEMP），同一精灵装在不同物品上 MP 不同，
没有“精灵编号 → MP”的固定表（STAGE-224 核对，见 docs/magic-mp.md）。装备自带的精灵服务端用物品自己的 MP；
givemagic 发到人物栏位的精灵没有原版数据，这里按公式推算（STAGE-224 决定维持推算）：攻击系 MP = 等级×5+5，治疗系 MP = 威力/32×2
（取整），状态与解状态 MP = 等级×2+2（原版毒雾、石化等同族 Lv1–5 为 4–14，
这里是粗估），复活 MP 同治疗系；场地属性 MP = 等级×2+2（调和按 Lv1 算为 4），
属性反转彩虹 10、极光 16。以上是推算值，没有原版依据。

目标：原版 target 1 是单人，8 是“一方全体”。攻击系的 9（敌方某一个）、10（敌方某一排）、11（敌方所有人）
只是“选什么”，实际命中哪些格由原版 data/attmagic.bin 里每个精灵的 siField（3×5 掩码）决定：
BATTLE_MultiAttMagic 以选中的人（目标码 1/9）或选中的一排（目标码 10）为中心，用掩码在敌方两排里取格；
目标码 11 遍历敌方两排，用掩码的第 0、1 行。uiAttackType 只影响表现（单人/整排/全体的回包写法），不决定命中格。
所以 TSV 多出两列：原版目标码和掩码（15 位，行优先；解析见 tools/decode-attmagic.py）。
状态与解状态按 1 单人、8 全体收录，全体的方向由函数决定（状态打对方，解状态救己方）。
魔障、沉默的目标是 3（敌方全体，MAGIC_TARGET_ALLOTHERSIDE），同样记作全体。

召唤龙阵（STAGE-202）：威力是参数里的威力，属性写 -；目标码 9 是单体（奔龙阵），10、11 是全体（强恩阵、奔雷阵、GM 烟火），
与攻击系 9/11 的旧规则不同，这是任务书的规定。MP 推算为 威力/10 + 10（奔龙阵、强恩阵 40，奔雷阵 50），GM 烟火 0
（只能靠管理命令发得到）；STAGE-204 统一校准。精灵版宠物技能（attskill）的威力为 0，MP 为 10，目标列写 single 只作占位，
真正的目标类型由服务端按技能（petskill.tsv 的目标码）决定。
输出列：编号 名字 函数 属性 威力 MP 目标(single/all/none/any) 状态 回合 成功率 参数 原版目标码 掩码
参数列（STAGE-202）：召唤龙阵写图像号（动画编号，客户端终端不画）；精灵版宠物技能写 petskill.txt 的技能编号；其余写 -。
原版目标码、掩码（STAGE-218）：只有攻击系有掩码（15 位，siField 行优先，1 表示命中格）；其余写 -。
攻击等级（STAGE-227）：攻击系是参数第 3 段（MagicLv），其余写 0。
状态：毒 poison、麻 paralysis、眠 sleep、石 stone、醉 drunk、乱 confusion；
解状态的“全”写 all。虚 weaken、障 barrier、默 nocast 是 STAGE-201 接入的状态；剧 deep_poison 还是
引擎没有的状态，照原样输出，引擎里没有人会挂上它，解它是空效（同原版没有这种状态的目标）。
没有的字段写 -（状态）或 0（回合、成功率）。
场地属性（STAGE-200）：属性列写 地/水/火/风/无，威力、回合是 magic.txt 参数里的数字（缺省 30、3，
威力超出 0–100 按 30）；目标写 none（不用选，对整场生效）。属性反转：彩虹 single、极光 all，
目标类型都是 any（要选一个单位，极光落在它所在的一方）。
"""
import math
import os
import re
import struct
import sys

funcs = {
    "MAGIC_AttMagic": "attack",
    "MAGIC_Recovery": "heal",
    "MAGIC_OtherRecovery": "heal_other",
    "MAGIC_StatusChange": "status",
    "MAGIC_Weaken": "status",
    "MAGIC_Barrier": "status",
    "MAGIC_Nocast": "status",
    "MAGIC_StatusRecovery": "cure",
    "MAGIC_MagicStatusChange": "resist",
    "MAGIC_Ressurect": "revive",
    "MAGIC_FieldAttChange": "field",
    "MAGIC_AttReverse": "reverse",
    "MAGIC_MagicDef": "defend",
    "MAGIC_ResAndDef": "revive_defend",
    "MAGIC_Metamo": "metamo",
    "MAGIC_ToCallDragon": "dragon",
    "MAGIC_AttSkill": "attskill",
}

# 原版 aszStatus（battle_event.c）里的单字代号；“全”是解状态的全部。
status_keys = {
    "毒": "poison", "麻": "paralysis", "眠": "sleep", "石": "stone", "醉": "drunk",
    "乱": "confusion", "虚": "weaken", "剧": "deep_poison", "障": "barrier", "默": "nocast",
    "全": "all",
}

# 状态精灵的参数：“毒 turn 3 成 20”，turn 是回合数，成 是成功率的偏移（BATTLE_StatusAttackCheck 的 PerOffset）。
status_arg = re.compile(r"^(.)\s*turn\s*(\d+)\s*成\s*(\d+)$")

# 场地属性的参数：“地 60  turn 3”，或只写属性（调和的“无”）。
field_arg = re.compile(r"^(.)\s*(\d+)?\s*(?:turn\s*(\d+))?$")
field_attrs = {"地", "水", "火", "风", "无"}
# 魔抗系精灵的参数（STAGE-228）：“魔抗|3|20|单”，状态名是中文，数值是加成百分比（火、电、冰抗原版只占槽）。
resist_arg = re.compile(r"^(魔抗|冰抗|电抗|火抗)\|(\d+)\|(\d+)\|(单|全)$")
resist_keys = {"魔抗": "magic_resist", "冰抗": "ice_resist", "电抗": "thunder_resist", "火抗": "fire_resist"}
# 防御精灵的参数（STAGE-199）：“吸  turn 1”，turn 是防御次数（原版 BATTLE_GetDamageReact 的计数器，不是回合）。
# 吸 = 吸收（CHAR_WORKDAMAGEABSROB）、反 = 反射（CHAR_WORKDAMAGEREFLEC）、无 = 无效（CHAR_WORKDAMAGEVANISH）。
defense_keys = {"吸": "absorb", "反": "reflect", "无": "vanish"}
defense_arg = re.compile(r"^(.)\s*turn\s*(\d+)$")
# 复活防御精灵的参数：“100 吸 turn 1”，前面是复活的威力。
revive_defense_arg = re.compile(r"^(\d+)\s*(.)\s*turn\s*(\d+)$")


# 挂状态的精灵函数（状态参数是“毒 turn 3 成 20”的形式）。
STATUS_SPELLS = {"MAGIC_StatusChange", "MAGIC_Weaken", "MAGIC_Barrier", "MAGIC_Nocast"}


def level_of(name):
    m = re.search(r"Lv\s*(\d+)", name)
    return int(m.group(1)) if m else 1


# 原版 tagAttMagic（gmsv/include/magic_base.h，_ATTACK_MAGIC）在 64 位平台按 4 字节对齐时的大小：
# 4 个 uint + 14 个 int + int[3][5] = 132 字节；siField 在最后，偏移 72。
ATT_MAGIC_SIZE = 132
ATT_MAGIC_MASK = 72


def read_attmagic(path):
    """读 attmagic.bin，返回每个精灵的 15 位掩码（文件里每个精灵两份，敌方/玩家各一份，掩码相同）。"""
    data = open(path, "rb").read()
    if len(data) % ATT_MAGIC_SIZE:
        sys.exit(f"{path} 的长度 {len(data)} 不是 {ATT_MAGIC_SIZE} 的整数倍")
    entries = []
    for off in range(0, len(data), ATT_MAGIC_SIZE):
        entries.append(struct.unpack_from("<15i", data, off + ATT_MAGIC_MASK))
    return entries


def attack_mask(entries, idx, name):
    if idx < 0 or idx * 2 + 1 >= len(entries):
        sys.exit(f"{name}：攻击精灵索引 {idx} 超出 attmagic.bin 的条数")
    enemy, player = entries[idx * 2], entries[idx * 2 + 1]
    if enemy != player:
        sys.exit(f"{name}：attmagic.bin 里敌方/玩家两份的掩码不一致")
    return "".join("1" if x else "0" for x in enemy)


def main(src, dst):
    entries = read_attmagic(os.path.join(os.path.dirname(src), "attmagic.bin"))
    out = ["# generated by tools/gen-magic.py; DO NOT EDIT. 列：编号 名字 函数 属性 威力 MP 目标(single/all/none/any) 状态 回合 成功率 参数 原版目标码 掩码 攻击等级"]
    for line in open(src, encoding="gbk"):
        line = line.strip()
        if not line:
            continue
        f = line.split(",")
        if len(f) < 8:
            continue
        name, fn, arg = f[0], f[2], f[3]
        try:
            target_code = int(f[6])
        except ValueError:
            continue
        if fn not in funcs:
            continue
        try:
            mid = int(f[4])
        except ValueError:
            continue
        elem, power, mp = "-", 0, 0
        status, turns, success = "-", 0, 0
        argcol = "-" # 参数列（STAGE-202）：只有召唤龙阵（图像号）和精灵版宠物技能（技能编号）有
        targetcol, maskcol = str(target_code), "-" # STAGE-218：原版目标码和攻击精灵的 3×5 掩码
        lvcol = "0" # STAGE-227：攻击精灵参数第 3 段的等级（原版 BATTLE_getMagicAdjustInt 的 MagicLv），其他为 0
        if fn == "MAGIC_AttMagic":
            parts = arg.split("|")
            elem, power, lv = parts[0], int(parts[1]), int(parts[2])
            mp = lv*5 + 5
            if len(f) < 9 or not f[8].strip().isdigit():
                sys.exit(f"{name}：攻击精灵缺少 MAGIC_IDX 列（attmagic.bin 的索引）")
            maskcol = attack_mask(entries, int(f[8]), name)
            lvcol = str(lv)
            # 目标码只决定“选什么”：1/9 选一个人、10 选一排、11 不用选（敌方两排）；命中格全看掩码。
            kind = "all" if target_code in (8, 11) else "single"
        elif fn in STATUS_SPELLS:
            m = status_arg.match(arg)
            if not m or m.group(1) not in status_keys:
                sys.exit(f"{name}：状态参数不认识：{arg}")
            status, turns, success = status_keys[m.group(1)], int(m.group(2)), int(m.group(3))
            mp = level_of(name)*2 + 2
            kind = status_kind(name, target_code)
        elif fn == "MAGIC_StatusRecovery":
            if arg not in status_keys:
                sys.exit(f"{name}：解状态参数不认识：{arg}")
            status = status_keys[arg]
            mp = level_of(name)*2 + 2
            kind = status_kind(name, target_code)
        elif fn == "MAGIC_MagicStatusChange":
            # 魔抗系（STAGE-228）：数值记进威力列（原版 CHAR_OTHERSTATUSNUMS）；单是目标 1，全是目标 8（我方一方，
            # 方向由服务端的 resist 函数定，见 gamedata/target.go）。MP 同其他状态精灵的推算。
            m = resist_arg.match(arg)
            if not m:
                sys.exit(f"{name}：魔抗参数不认识：{arg}")
            want = 1 if m.group(4) == "单" else 8
            if target_code != want:
                sys.exit(f"{name}：魔抗的目标码应为 {want}，实际 {target_code}")
            status, turns, power = resist_keys[m.group(1)], int(m.group(2)), int(m.group(3))
            mp = level_of(name)*2 + 2
            kind = status_kind(name, target_code)
        elif fn == "MAGIC_FieldAttChange":
            m = field_arg.match(arg)
            if not m or m.group(1) not in field_attrs:
                sys.exit(f"{name}：场地属性参数不认识：{arg}")
            elem = m.group(1)
            power = int(m.group(2)) if m.group(2) else 30
            if power > 100:
                power = 30
            turns = int(m.group(3)) if m.group(3) else 3
            mp = level_of(name)*2 + 2
            kind = "none"
        elif fn == "MAGIC_AttReverse":
            if target_code not in (1, 8):
                sys.exit(f"{name}：属性反转的目标应为 1 或 8，实际 {target_code}")
            mp = 10 if target_code == 1 else 16
            kind = "single" if target_code == 1 else "all"
        elif fn == "MAGIC_MagicDef":
            m = defense_arg.match(arg)
            if not m or m.group(1) not in defense_keys:
                sys.exit(f"{name}：防御参数不认识：{arg}")
            status, turns = defense_keys[m.group(1)], int(m.group(2))
            mp = turns*4 + 4
            if target_code != 1:
                sys.exit(f"{name}：防御精灵应为单人（目标 1），实际 {target_code}")
            kind = "single"
        elif fn == "MAGIC_ResAndDef":
            m = revive_defense_arg.match(arg)
            if not m or m.group(2) not in defense_keys:
                sys.exit(f"{name}：复活防御参数不认识：{arg}")
            power, status, turns = int(m.group(1)), defense_keys[m.group(2)], int(m.group(3))
            mp = int(math.ceil(power/32)) * 2 + turns*4
            if target_code != 1:
                sys.exit(f"{name}：复活防御应为单人（目标 1），实际 {target_code}")
            kind = "single"
        elif fn == "MAGIC_Metamo":
            # 变幻的精灵（STAGE-203）：威力列填变身的秒数；目标列是自己，TSV 的“目标”列写 single 只为占位，
            # 服务端按函数 metamo 判断（BattleTarget 为 none，战斗外使用）。MP = 秒数÷60×4+4，上限 60（STAGE-204 统一校准）。
            if not arg.isdigit():
                sys.exit(f"{name}：变身参数不认识：{arg}")
            if target_code != 1:
                sys.exit(f"{name}：变幻的精灵应为目标 1，实际 {target_code}")
            power = int(arg)
            mp = min(60, power*4//60 + 4)
            kind = "single"
        elif fn == "MAGIC_Ressurect":
            if not arg.isdigit():
                sys.exit(f"{name}：复活参数不认识：{arg}")
            power = int(arg)
            mp = int(math.ceil(power/32)) * 2
            if target_code != 1:
                sys.exit(f"{name}：复活应为单人（目标 1），实际 {target_code}")
            kind = "single"
        elif fn == "MAGIC_ToCallDragon":
            # 召唤龙阵（STAGE-202）：参数是“图像号|威力”，图像号记进参数列，威力是这一下的攻击力。
            # 目标码 9 是单体（奔龙阵），10、11 是全体（强恩阵、奔雷阵、GM 烟火），见文件开头的说明。
            parts = arg.split("|")
            if len(parts) != 2 or not parts[0].isdigit() or not parts[1].isdigit():
                sys.exit(f"{name}：召唤龙阵参数不认识：{arg}")
            argcol, power = parts[0], int(parts[1])
            if target_code not in (9, 10, 11):
                sys.exit(f"{name}：召唤龙阵的目标应为 9、10 或 11，实际 {target_code}")
            mp = 0 if "烟火" in name else power//10 + 10
            kind = "single" if target_code == 9 else "all"
        elif fn == "MAGIC_AttSkill":
            # 精灵版宠物技能（STAGE-202）：参数是“技能函数;技能编号”，只记技能编号；威力 0，MP 10，目标列占位写 single。
            parts = arg.split(";")
            if len(parts) != 2 or not parts[1].isdigit():
                sys.exit(f"{name}：精灵版技能参数不认识：{arg}")
            argcol, mp, kind = parts[1], 10, "single"
        else:
            power = int(arg) if arg.isdigit() else 0
            mp = int(math.ceil(power/32)) * 2
            kind = "all" if target_code in (8, 9) else "single"
        out.append(f"{mid}\t{name}\t{funcs[fn]}\t{elem}\t{power}\t{mp}\t{kind}\t{status}\t{turns}\t{success}\t{argcol}\t{targetcol}\t{maskcol}\t{lvcol}")
    with open(dst, "w", encoding="utf-8") as w:
        w.write("\n".join(out) + "\n")
    print(f"{dst}: {len(out) - 1} 行")


def status_kind(name, target_code):
    if target_code not in (1, 3, 8):
        sys.exit(f"{name}：状态类精灵的目标应为 1、3 或 8，实际 {target_code}")
    return "single" if target_code == 1 else "all"


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
