"""tools/npcquiz.py 的单元测试（STAGE-329）：题库、人物参数的解析规则，以及产物与原版数据一致。"""

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import npcquiz  # noqa: E402
from npcquiz import ROOT, check, eligible_count, generate, load_placements, load_questions, parse_quiz_config  # noqa: E402

QUESTIONS = """# 注释行
1,1,4,2,1,石器时代的世界的名称是?,尼斯,尼结,眯斯
2,1,1,1,2,近藏的宠物的名字是？,小卡,小胜,
3,1,1,4,1,卡鲁它那的村子是哪里？,卡鲁它那,,
4,1,1,2,1,坏行,A,B
"""

CONFIG = """Type:0
Answer:0
Level:31
Quiznum:2
Border:2,过了,1,及格,0,没过
GetItem:2,2759.2760
Warp:2,803.18.15,0,802.20.22
StartMsg:\\n跟我以猜谜比赛胜负
FailureMsg:真是可惜呢
"""


class QuestionTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = Path(self.dir.name) / "question.txt"

    def tearDown(self):
        self.dir.cleanup()

    def write(self, text):
        self.path.write_bytes(text.encode("gbk"))

    def test_skips_short_lines_and_trims(self):
        self.write(QUESTIONS)
        got = load_questions(self.path)
        self.assertEqual([q["no"] for q in got], [1, 2, 3])  # 第 4 行字段不足，跳过
        self.assertEqual(got[0]["options"], ["尼斯", "尼结", "眯斯"])  # 尾部空格去掉
        self.assertEqual(got[1]["options"], ["小卡", "小胜"])  # 答案类型 1 只取两个
        self.assertEqual(got[2]["options"], ["卡鲁它那"])  # 文字题只留正确答案

    def test_answer_type_1_with_answer_3_fails(self):
        self.write("1,1,1,1,3,题？,a,b,c\n")
        with self.assertRaises(ValueError):
            load_questions(self.path)

    def test_text_answer_must_be_first(self):
        self.write("1,1,1,4,2,题？,a,b,c\n")
        with self.assertRaises(ValueError):
            load_questions(self.path)


class ConfigTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = Path(self.dir.name) / "quz_test"

    def tearDown(self):
        self.dir.cleanup()

    def test_pairs_and_warp(self):
        self.path.write_bytes(CONFIG.encode("gbk"))
        cfg = parse_quiz_config(self.path)
        self.assertEqual(cfg["quiznum"], 2)
        self.assertEqual(cfg["border"], [{"min": 2, "msg": "过了"}, {"min": 1, "msg": "及格"}, {"min": 0, "msg": "没过"}])
        self.assertEqual(cfg["get_item"], [{"min": 2, "items": [2759, 2760]}])
        self.assertEqual(cfg["warp"][0], {"min": 2, "floor": 803, "x": 18, "y": 15})
        self.assertEqual(cfg["start_msg"], "\\n跟我以猜谜比赛胜负")  # 字面 \n 原样保留
        self.assertNotIn("entry_item", cfg)

    def test_odd_pairs_fail(self):
        self.path.write_bytes("Border:2,过了,1\nQuiznum:1\n".encode("gbk"))
        with self.assertRaises(ValueError):
            parse_quiz_config(self.path)


class PlacementTest(unittest.TestCase):
    def test_commented_block_is_not_placed(self):
        text = (
            "#{\n#floorid=7777\n#name=欧吉拉\n#enemy=npcgen_quiz|file:genout/quz_7777_20_20\n#borncorner=20,20,20,20\n"
            "#dir=4\n#graphicname=100070\n#}\n"
            "{\nfloorid=812\nborncorner=28,14,28,14\ndir=4\ngraphicname=100010\nname=猜谜大师\n"
            "enemy=npcgen_quiz|file:genout/quz_812_28_14\n}\n"
        )
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "quizman.create"
            p.write_bytes(text.encode("gbk"))
            got = load_placements(p)
        self.assertFalse(got["quz_7777_20_20"]["placed"])
        self.assertEqual(got["quz_7777_20_20"]["name"], "欧吉拉")
        self.assertTrue(got["quz_812_28_14"]["placed"])
        self.assertEqual(got["quz_812_28_14"]["corner"], [28, 14, 28, 14])


class CheckTest(unittest.TestCase):
    def test_eligible_masks(self):
        qs = [{"type": 1, "answertype": 2, "level": 1}, {"type": 1, "answertype": 1, "level": 4}]
        npc = {"type_mask": 0, "answer_mask": 0, "level_mask": 3}
        self.assertEqual(eligible_count(npc, qs), 1)

    def test_quiznum_over_eligible_fails(self):
        qs = [{"type": 1, "answertype": 2, "level": 1}]
        with self.assertRaises(ValueError):
            check(qs, [{"file": "quz_x", "quiznum": 2, "type_mask": 0, "answer_mask": 0, "level_mask": 0}])


class DataTest(unittest.TestCase):
    """真实原版数据：生成结果与提交的产物一致，关键数字对得上原版。"""

    def test_output_matches_committed(self):
        with tempfile.TemporaryDirectory() as d:
            nq, nn = generate(npcquiz.DEFAULT_DATA, Path(d))
            self.assertEqual((nq, nn), (149, 23))
            for name in ("npc-quiz-questions.json", "npc-quiz.json"):
                committed = (ROOT / "server/gamedata" / name).read_bytes()
                self.assertEqual((Path(d) / name).read_bytes(), committed, f"{name} 与重新生成的不一致")

    def test_key_numbers(self):
        npcs = json.loads((ROOT / "server/gamedata/npc-quiz.json").read_text(encoding="utf-8"))
        c = next(x for x in npcs if x["file"] == "quz_812_28_14")
        self.assertEqual(c["quiznum"], 100)
        self.assertEqual(c["get_item"], [{"min": 100, "items": [2759]}])
        self.assertEqual(len(npcs), 23)
        self.assertEqual(sum(1 for x in npcs if x["placed"]), 22)


if __name__ == "__main__":
    unittest.main()
