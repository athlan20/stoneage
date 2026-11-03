"""宠物对话脚本（玛蕾等）的转换（STAGE-234，docs/quest.md「宠物对话」）。

原版 data/npc/pettalk/*.arg（GBK）是宠物对话：每个块以 OVER 结束，块名 TALKRUN*/EVENTRUN*/BOTH*/OTHER 决定
什么时候用它（对照 gmsv/char/pet_event.c 的 PET_Talkfunc：宠物等级 >= 个体等级上限时只看 EVENTRUN，否则看
TALKRUN，BOTH 两种都看，OTHER 是没有块命中时的台词）。条件行：FREE（玩家）、PET（宠物，LV 比较）、FLOOR（玩家所在
楼层，! 表示不在）；动作行 LimitLevel:n 把宠物的个体等级上限改成 n（PetTalk_RunEvent）。

这里把 ptalk*.arg 转成 server/gamedata/pettalk.json，运行时由 server/gamedata/pettalk.go 读取。只认能支持的条件：
FREE:LV>0（恒成立，角色等级至少 1）、PET:LV 的 = < > 与 & 组合、FLOOR、AddItem、LimitLevel、TalkMsg。含 HP 等其他条件的块
（如 BOTH2–BOTH6）跳过，并在 skipped 里列出原因；跳过的块不进候选，只在玩家的 HP 或宠物的 HP 落在它的区间时，原版
的随机候选才会多出它，这点在 docs/quest.md「宠物对话」一节说明。

用法：python3 tools/pettalk.py [--src DIR] [--out FILE]
"""

import argparse
import json
import re
import sys
from pathlib import Path

from npc_records import decode

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_SRC = ROOT / "original-data/gmsv/data/npc/pettalk"
DEFAULT_OUT = ROOT / "server/gamedata/pettalk.json"

# 要转换的宠物对话文件：(文件名, 宠物模板编号)。ptalk06（二转玛蕾）不在本需求范围内。
SOURCES = [("ptalk01.arg", 718)]

TAGS = ("TALKRUN", "EVENTRUN", "BOTH", "OTHER")
LABEL = re.compile(r"^([A-Za-z0-9_]+):$")
PET_LV = re.compile(r"^LV(=|<|>)(\d+)$")
FLOOR = re.compile(r"^(!?)(\d+)$")


def talk_text(value: str) -> str:
    """原版台词里的字面 \\n（反斜杠加 n）是换行，转成真正的换行符（STAGE-239：客户端按台词原样显示）。"""
    return value.replace("\\n", "\n")


def parse_block(name: str, lines: list[str]) -> tuple[dict | None, str]:
    """解析一个块（不含 OVER）。返回 (块, 跳过原因)；能支持时原因为空。"""
    cond = {"pet": [], "floor": None, "limit": 0, "items": []}
    text = None
    for line in lines:
        if LABEL.match(line):
            continue
        key, sep, value = line.partition(":")
        if not sep:
            return None, f"无法识别的行：{line}"
        if key == "FREE":
            if value != "LV>0":
                return None, f"FREE 条件 {value} 不支持"
        elif key == "PET":
            for part in value.split("&"):
                m = PET_LV.match(part)
                if not m:
                    return None, f"PET 条件 {part} 不支持"
                cond["pet"].append({"cmp": m.group(1), "level": int(m.group(2))})
            if "," in value:
                return None, "PET 条件里的 , 不支持"
        elif key == "FLOOR":
            m = FLOOR.match(value)
            if not m:
                return None, f"FLOOR 条件 {value} 不支持"
            cond["floor"] = {"id": int(m.group(2)), "not": m.group(1) == "!"}
        elif key == "AddItem":
            # 原版 NPC_ActionAddItem：逗号分的道具编号，每个给一件；给不下（栏位满）时整个块失败、不改上限
            if not all(x.isdigit() for x in value.split(",")):
                return None, f"AddItem {value} 不是道具编号"
            cond["items"] = [int(x) for x in value.split(",")]
        elif key == "LimitLevel":
            if not value.isdigit():
                return None, f"LimitLevel {value} 不是整数"
            cond["limit"] = int(value)
        elif key == "TalkMsg":
            text = talk_text(value)
        else:
            return None, f"{key} 不支持"
    if text is None:
        return None, "没有 TalkMsg"
    return {"name": name, "tag": next(t for t in TAGS if name.startswith(t)), **cond,
            "text": text}, ""


def parse_file(path: Path, template: int) -> dict:
    raw = decode(path.read_bytes(), str(path))
    body = []
    for line in raw.splitlines():
        s = line.strip()
        if not s or s.startswith("#") or s in ("{", "}") or s.startswith("PETTEMPNO:"):
            continue
        body.append(s)
    table = {"source": path.name, "template": template, "other": [], "blocks": [], "skipped": []}
    for chunk in "\n".join(body).split("OVER"):
        lines = [x for x in chunk.split("\n") if x.strip()]
        if not lines:
            continue
        m = LABEL.match(lines[0])
        if not m or not m.group(1).startswith(TAGS):
            table["skipped"].append({"name": lines[0], "reason": "块名无法识别"})
            continue
        name = m.group(1)
        if name.startswith("OTHER"):
            talk = [x[len("TalkMsg:"):] for x in lines if x.startswith("TalkMsg:")]
            # 原版 OTHER 的 TalkMsg 用逗号分成若干句，随机选一句
            table["other"].extend(talk_text(talk[0]).split(",") if talk else [])
            continue
        block, reason = parse_block(name, lines[1:])
        if block is None:
            table["skipped"].append({"name": name, "reason": reason})
        else:
            table["blocks"].append(block)
    return table


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--src", default=str(DEFAULT_SRC), help="原版 pettalk 目录")
    ap.add_argument("--out", default=str(DEFAULT_OUT), help="输出的 JSON 文件")
    args = ap.parse_args()
    tables = []
    for fname, template in SOURCES:
        t = parse_file(Path(args.src) / fname, template)
        for s in t["skipped"]:
            print(f"跳过 {fname} {s['name']}：{s['reason']}", file=sys.stderr)
        print(f"{fname}：{len(t['blocks'])} 个块，跳过 {len(t['skipped'])} 个", file=sys.stderr)
        tables.append(t)
    out = Path(args.out)
    out.write_text(json.dumps(tables, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
    print(f"写入 {out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
