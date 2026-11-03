"""猜谜题库与猜谜人物配置（STAGE-329，原版 gmsv/npc/npc_quiz.c）。

原版服务端的题库由 setup.cf 的 quizfile=data/question.txt 指定（configfile.c 的 getQuizfile），
不是 data/racequiz.txt（C 源码里没有任何地方引用它）。这里生成两份数据，供 server/gamedata/npcquiz.go 读取：

- server/gamedata/npc-quiz-questions.json：题库。每行 9 个逗号分隔字段
  （编号,题型,难度位,答案类型,正确选项,题干,选项1,选项2,选项3，QUIZ_initQuiz 的解析）。
  与原版加载一致：字段不足 9 的行跳过（原版是 continue，不计入题库）；答案类型 1 且正确选项为 3、
  答案类型 4 且正确选项不为 1 时整个加载失败（原版 return FALSE），这里报错。
  答案类型 1 只取两个选项，2 取三个，4 是文字题（只保留正确答案 options[0]）。
- server/gamedata/npc-quiz.json：猜谜人物（genout/quz_*，原版 npcgen_quiz 的参数文件）加上
  quizman.create 里的名字、楼层、出生范围、朝向、图像。Type/Answer/Level 原样保留为位掩码（0 表示不限，
  同原版 NPC_GetQuestion），Border/GetItem/Warp 拆成 (阈值, 内容) 的有序列表（顺序即原版的判断顺序）。

台词与选项逐字保留（包括原文里的字面 "\\n"，同宠物店的做法）；只去掉选项与题干两端的空格和全角空格。

用法：python3 tools/npcquiz.py [原版 gmsv/data 目录]
"""

import glob
import json
import re
import sys
from pathlib import Path

from npc_records import atoi, decode

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_DATA = ROOT / "original-data/gmsv/data"
OUT_DIR = ROOT / "server/gamedata"

# 原版 npc_quiz.c 的 OLDNO：一场猜谜最多 100 题（NPC_QuizInit 检查 Quiznum）。
OLDNO = 100

# 猜谜人物参数里用到的键 → JSON 字段（文字台词原样保留）。
TEXT_KEYS = {"StartMsg": "start_msg", "FailureMsg": "failure_msg", "ItemFullMsg": "item_full_msg",
             "NoEntryMsg": "no_entry_msg", "Party": "party_msg", "EntryItem": "entry_item"}


def clean(text: str) -> str:
    """去掉两端的空格、全角空格和 CR；题干和选项的内部文字不动。"""
    return text.strip(" 　\t\r")


def load_questions(path: Path) -> list[dict]:
    """读题库（原版 QUIZ_initQuiz 的解析规则），返回题目列表，顺序同文件。"""
    text = decode(path.read_bytes(), str(path))
    out = []
    for lineno, raw in enumerate(text.split("\n"), 1):
        line = raw.replace("\t", " ").rstrip("\r")
        if not line or line.startswith("#"):
            continue
        fields = line.split(",")
        if len(fields) < 9:
            print(f"跳过 {path.name} 第 {lineno} 行：只有 {len(fields)} 个字段（原版加载也跳过）", file=sys.stderr)
            continue
        no, qtype, level = atoi(fields[0]), atoi(fields[1]), atoi(fields[2])
        answertype, answer = atoi(fields[3]), atoi(fields[4])
        question = clean(fields[5])
        choices = [clean(fields[6]), clean(fields[7]), clean(fields[8])]
        where = f"{path.name} 第 {lineno} 行"
        if answertype == 1:
            if answer == 3:
                raise ValueError(f"{where}：答案类型 1 的正确选项不能是 3（原版加载失败）")
            options = choices[:2]
        elif answertype == 2:
            options = choices
        elif answertype == 4:
            if answer != 1:
                raise ValueError(f"{where}：答案类型 4 的正确选项必须是 1（原版加载失败）")
            options = choices[:1]
        else:
            raise ValueError(f"{where}：未知的答案类型 {answertype}")
        if not all(options) or not question:
            raise ValueError(f"{where}：题干或选项为空")
        out.append({"no": no, "type": qtype, "level": level, "answertype": answertype,
                    "answer": answer, "question": question, "options": options})
    return out


def pairs(value: str, where: str) -> list[tuple[str, str]]:
    """Border/GetItem/Warp 的“阈值,内容,阈值,内容”拆成成对列表。"""
    parts = value.split(",")
    if len(parts) % 2:
        raise ValueError(f"{where}：逗号分隔的个数不是成对的：{value!r}")
    return [(parts[i], parts[i + 1]) for i in range(0, len(parts), 2)]


def parse_quiz_config(path: Path) -> dict:
    """读一个 genout/quz_* 参数文件（每行 键:值）。"""
    text = decode(path.read_bytes(), str(path))
    kv = {}
    for raw in text.split("\n"):
        line = raw.rstrip("\r")
        if not line.strip():
            continue
        key, sep, value = line.partition(":")
        if sep:
            kv[key.strip()] = value
    where = path.name
    cfg = {
        "type_mask": atoi(kv.get("Type", "0")),
        "answer_mask": atoi(kv.get("Answer", "0")),
        "level_mask": atoi(kv.get("Level", "0")),
        "quiznum": atoi(kv.get("Quiznum", "0")),
        "border": [{"min": atoi(t), "msg": m} for t, m in pairs(kv["Border"], where)] if "Border" in kv else [],
    }
    if "GetItem" in kv:
        cfg["get_item"] = [{"min": atoi(t), "items": [atoi(x) for x in v.split(".")]}
                           for t, v in pairs(kv["GetItem"], where)]
    if "Warp" in kv:
        warps = []
        for t, v in pairs(kv["Warp"], where):
            floor, x, y = (atoi(p) for p in (v.split(".") + ["0", "0"])[:3])
            warps.append({"min": atoi(t), "floor": floor, "x": x, "y": y})
        cfg["warp"] = warps
    for key, field in TEXT_KEYS.items():
        if key in kv:
            cfg[field] = kv[key]
    return cfg


def load_placements(path: Path) -> dict[str, dict]:
    """quizman.create 里 enemy=npcgen_quiz|file:genout/quz_* 的 NPC，按参数文件名索引。

    整块被注释掉的（如 quz_7777_20_20 的 #{ ... #}）原版不刷出来，这里记 placed=False，
    名字和坐标照注释里的写。块里的 # 只是注释前缀，去掉后解析。
    """
    text = decode(path.read_bytes(), str(path))
    found = {}
    prefix = "npcgen_quiz|file:genout/"
    for block in re.findall(r"\{(.*?)\}", text, re.S):
        kv, placed = {}, True
        for raw in block.split("\n"):
            line = raw.strip()
            if line.startswith("#") and "=" in line and "enemy=" in line:
                placed = False
            key, sep, value = line.lstrip("#").strip().partition("=")
            if sep:
                kv[key.strip()] = value.strip()
        if not kv.get("enemy", "").startswith(prefix):
            continue
        name = kv["enemy"][len(prefix):]
        if name in found:
            raise ValueError(f"{path.name}：{name} 有两个放置块")
        corner = [atoi(v) for v in kv["borncorner"].split(",")]
        found[name] = {"placed": placed, "name": kv["name"], "floor": atoi(kv["floorid"]), "corner": corner,
                       "dir": atoi(kv["dir"]), "graphic": atoi(kv["graphicname"])}
    return found


def load_npcs(data: Path) -> list[dict]:
    genout = data / "npc/genout"
    placements = load_placements(genout / "quizman.create")
    files = sorted(Path(p).name for p in glob.glob(str(genout / "quz_*")))
    if sorted(placements) != files:
        raise ValueError(f"quizman.create 的猜谜人物 {len(placements)} 个与参数文件 {len(files)} 个对不上")
    npcs = []
    for name in files:
        cfg = parse_quiz_config(genout / name)
        npcs.append({"file": name, **placements[name], **cfg})
    return npcs


def mask(value: int) -> int:
    """位掩码：<=0 当作不限（0xffff），同 NPC_GetQuestion。"""
    return value if value > 0 else 0xFFFF


def eligible_count(npc: dict, questions: list[dict]) -> int:
    """能出的题数，条件同 NPC_GetQuestion：题型、答案类型、难度都是题目位的子集。"""
    tm, am, lm = mask(npc["type_mask"]), mask(npc["answer_mask"]), mask(npc["level_mask"])
    n = 0
    for q in questions:
        if q["type"] < 1 or (tm & (1 << (q["type"] - 1))) != (1 << (q["type"] - 1)):
            continue
        if (am & q["answertype"]) != q["answertype"] or (lm & q["level"]) != q["level"]:
            continue
        n += 1
    return n


def check(questions: list[dict], npcs: list[dict]) -> None:
    """与原版 NPC_QuizInit 的检查一致：Quiznum 在 1..100 之间，且不超过能出的题数。"""
    for npc in npcs:
        if not 1 <= npc["quiznum"] <= OLDNO:
            raise ValueError(f"{npc['file']}：Quiznum={npc['quiznum']} 超出 1..{OLDNO}")
        n = eligible_count(npc, questions)
        if npc["quiznum"] > n:
            raise ValueError(f"{npc['file']}：Quiznum={npc['quiznum']} 超过能出的题数 {n}")


def write_json(records: list, out: Path) -> None:
    out.write_text(json.dumps(records, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")


def generate(data: Path, out_dir: Path) -> tuple[int, int]:
    questions = load_questions(data / "question.txt")
    npcs = load_npcs(data)
    check(questions, npcs)
    write_json(questions, out_dir / "npc-quiz-questions.json")
    write_json(npcs, out_dir / "npc-quiz.json")
    return len(questions), len(npcs)


if __name__ == "__main__":
    src = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_DATA
    nq, nn = generate(src, OUT_DIR)
    print(f"题库 {nq} 题，猜谜人物 {nn} 个")
