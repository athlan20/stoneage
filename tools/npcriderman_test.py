"""tools/npcriderman.py 的单元测试：选项与正文的切分、窗口的可达性（STAGE-122）。"""

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from npcriderman import reachable, riderman_records, window_record, windows_from_conf  # noqa: E402

CONF = """# 测试用
winno=1
message=2
message=我是训练师。
message=你想办什么吗？
message=
message=关于
message=学习
selected=
gotowin=2
endbutton=
selected=
gotowin=100
endbutton=
endwin=

winno=2
message=一段正文
message=
message=第一条
selected=
gotowin=3
endbutton=
endwin=

winno=3
message=第一段
message=
message=第二段
prevpressed=
gotowin=2
endwin=

winno=999
message=走不到的窗口
endwin=
"""


class RidermanTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        path = Path(self.dir.name) / "npc/family"
        path.mkdir(parents=True)
        (path / "riderman.conf").write_bytes(CONF.encode("gbk"))
        self.data = Path(self.dir.name)

    def tearDown(self):
        self.dir.cleanup()

    def test_options_take_the_last_lines_and_drop_count_line(self):
        win = windows_from_conf(self.data / "npc/family/riderman.conf")[0]
        rec = window_record(win)
        self.assertEqual(rec["text"], "我是训练师。\n你想办什么吗？")
        self.assertEqual([o["label"] for o in rec["options"]], ["关于", "学习"])
        self.assertEqual([o["gotowin"] for o in rec["options"]], [2, 100])

    def test_prev_button_and_paragraphs(self):
        win = windows_from_conf(self.data / "npc/family/riderman.conf")[2]
        rec = window_record(win)
        self.assertEqual(rec["text"], "第一段\n\n第二段")
        self.assertEqual(rec["prev"], 2)
        self.assertNotIn("options", rec)

    def test_unreachable_windows_are_dropped(self):
        recs = riderman_records(self.data)
        self.assertEqual(sorted(r["winno"] for r in recs), [1, 2, 3])
        self.assertEqual(reachable(windows_from_conf(self.data / "npc/family/riderman.conf")), {1, 2, 3})


if __name__ == "__main__":
    unittest.main()
