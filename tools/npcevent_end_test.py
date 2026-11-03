"""块的结束标记（STAGE-223，tools/npcevent.py 的 parse_script）：文件末尾缺 EventEnd、EventEnd 后同一行接了别的字，
以及没有 EventNo 的段。对照 gmsv/util.c 的 getStringFromIndexWithDelim_body（按子串切段，最后一段读到文件结束）
和 gmsv/npc/npc_exchangeman.c 的选块循环（没有 EventNo 的段 continue）。
运行：python3 -m unittest tools/npcevent_end_test.py
"""
import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import npcevent  # noqa: E402

KNOWN = {2466, 2593}
FLOORS = {200}

BLOCK = "EventNo:-1\nTYPE:MESSAGE\nEVENT:LV>0\nNomalWindowMsg:你好\n"


class MissingFinalEventEndTest(unittest.TestCase):
    """文件末尾的块没有 EventEnd：原版读到文件结束，所以照常读成一块。"""

    def test_last_block_without_event_end_is_read(self):
        script, notes = npcevent.parse_script("NomalMainMsg:闲聊\n" + BLOCK, KNOWN, FLOORS)
        self.assertEqual(len(script["blocks"]), 1)
        rec = script["blocks"][0]
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["nomal_window"], "你好")
        self.assertEqual(notes, [])

    def test_last_block_without_event_end_after_complete_block(self):
        text = "NomalMainMsg:闲聊\n" + BLOCK + "EventEnd\n\n" + BLOCK.replace("你好", "再见")
        script, notes = npcevent.parse_script(text, KNOWN, FLOORS)
        self.assertEqual([b["msg"]["nomal_window"] for b in script["blocks"]], ["你好", "再见"])
        self.assertEqual(notes, [])

    def test_trailing_segment_without_event_no_is_skipped_with_note(self):
        # 末尾一段只有台词，没有 EventNo：原版 continue，不算块，但要在转换说明里记下来。
        text = "NomalMainMsg:闲聊\n" + BLOCK + "EventEnd\n这是一句无关的话\n"
        script, notes = npcevent.parse_script(text, KNOWN, FLOORS)
        self.assertEqual(len(script["blocks"]), 1)
        self.assertNotIn("unsupported", script["blocks"][0])
        self.assertEqual(len(notes), 1)
        self.assertIn("段 2 没有 EventNo", notes[0])


class GluedEventEndTest(unittest.TestCase):
    """EventEnd 后面同一行接了别的字（poru/titan 的最后一块）：EventEnd 照常结束前一块，后面的字是下一段。"""

    def test_text_after_event_end_on_same_line_is_next_segment(self):
        text = "NomalMainMsg:闲聊\n" + BLOCK + "EventEnd这里是无关的话\n"
        script, notes = npcevent.parse_script(text, KNOWN, FLOORS)
        self.assertEqual(len(script["blocks"]), 1)
        self.assertNotIn("unsupported", script["blocks"][0])
        self.assertIn("段 2 没有 EventNo", notes[0])

    def test_glued_text_followed_by_another_block(self):
        text = BLOCK + "EventEnd无关的话\n" + BLOCK.replace("你好", "再见") + "EventEnd\n"
        script, _ = npcevent.parse_script(text, KNOWN, FLOORS)
        self.assertEqual([b["msg"]["nomal_window"] for b in script["blocks"]], ["你好", "再见"])


class CommentSegmentTest(unittest.TestCase):
    """注释段（整段都是 # 开头的行）不算块，也不记转换说明。"""

    def test_commented_out_block_is_silently_dropped(self):
        text = BLOCK + "EventEnd\n#EventNo:-1\n#TYPE:MESSAGE\n#EventEnd\n" + BLOCK.replace("你好", "再见") + "EventEnd\n"
        script, notes = npcevent.parse_script(text, KNOWN, FLOORS)
        self.assertEqual([b["msg"]["nomal_window"] for b in script["blocks"]], ["你好", "再见"])
        self.assertEqual(notes, [])


if __name__ == "__main__":
    unittest.main()
