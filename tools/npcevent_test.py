"""NpcWarp 的转换（STAGE-93，tools/npcevent.py）：位置串解析、地图校验、块记录里的 warp 字段。
运行：python3 -m unittest tools/npcevent_test.py
"""
import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import npcevent  # noqa: E402

FLOORS = {200, 30001, 30012}


class ParseWarpTest(unittest.TestCase):
    def test_list_keeps_order_and_duplicates(self):
        got, why = npcevent.parse_warp("200.215.1017,200.198.1001,200.115.1011,200.115.1011", FLOORS)
        self.assertEqual(why, "")
        self.assertEqual(got[0], {"floor": 200, "x": 215, "y": 1017})
        self.assertEqual(got[2], got[3])
        self.assertEqual(len(got), 4)

    def test_unknown_floor_is_unsupported(self):
        got, why = npcevent.parse_warp("200.1.2,99999.1.2", FLOORS)
        self.assertIsNone(got)
        self.assertIn("99999", why)

    def test_bad_format_is_unsupported(self):
        for bad in ("200.1", "200.1.2.3", "200.a.2", "200.-1.2", "", "200.1.2,"):
            with self.subTest(bad=bad):
                got, why = npcevent.parse_warp(bad, FLOORS)
                self.assertIsNone(got)
                self.assertTrue(why)


class ConvertBlockWarpTest(unittest.TestCase):
    def block(self, *lines):
        return npcevent.convert_block(list(lines), known={2709, 2712}, floors=FLOORS)

    def test_message_with_warp_is_supported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好", "NpcWarp:30001.10.6,30012.7.4")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["warp"], [{"floor": 30001, "x": 10, "y": 6}, {"floor": 30012, "x": 7, "y": 4}])

    def test_accept_with_warp_and_items_is_supported(self):
        rec = self.block(
            "EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0&ITEM=2709", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
            "NpcWarp:30001.10.6", "GetItem:2712", "DelItem:2709",
        )
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["warp"], [{"floor": 30001, "x": 10, "y": 6}])
        self.assertEqual(rec["get_item"], [{"id": 2712, "n": 1}])

    def test_bad_warp_marks_block_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好", "NpcWarp:30001.10")
        self.assertIn("NpcWarp", rec["unsupported"])
        self.assertNotIn("warp", rec)

    def test_block_without_warp_has_no_warp_field(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好")
        self.assertNotIn("warp", rec)


class ConvertBlockRealTest(unittest.TestCase):
    """原文里的写法（STAGE-94）：台词断行的续行、字段名后面直接是字面 \\n 的台词，以及带物品的 REQUEST。"""

    def block(self, *lines):
        return npcevent.convert_block(list(lines), known={2466, 2593}, floors=FLOORS)

    def test_message_continuation_line_joins_the_message(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0", "AcceptMsg:要吗？\\n我会付",
                         "就会付打工工资给你哦。", "ThanksMsg:谢谢")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["accept"], "要吗？\n我会付就会付打工工资给你哦。")

    def test_continuation_after_non_message_field_is_still_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好", "GetStone:10", "随便写的行")
        self.assertIn("无法识别的行", rec["unsupported"])

    def test_field_name_with_literal_newline_gets_colon(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0", "AcceptMsg\\n要吗？\\n是吧",
                         "ThanksMsg:谢谢")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["accept"], "\n要吗？\n是吧")

    def test_request_with_items_is_supported(self):
        rec = self.block("EventNo:19", "TYPE:REQUEST", "EVENT:LV>0&ITEM!=2593", "RequestMsg:要吗？",
                         "ThanksMsg:谢谢", "StopMsg:不要吗？", "EndStopMsg:好吧", "NoStopMsg:那就算了",
                         "GetItem:2593", "DelItem:2466*1", "DelStone:50")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["get_item"], [{"id": 2593, "n": 1}])
        self.assertEqual(rec["del_item"], [{"id": 2466, "n": 1}])
        self.assertEqual(rec["del_stone"], {"fixed": 50})

    def test_request_with_random_item_is_unsupported(self):
        rec = self.block("EventNo:19", "TYPE:REQUEST", "EVENT:LV>0", "RequestMsg:要吗？", "ThanksMsg:谢谢",
                         "GetRandItem:2593,2466")
        self.assertIn("随机道具", rec["unsupported"])


class ConvertBlockPagesTest(unittest.TestCase):
    """分页台词（STAGE-97）：AcceptMsg1..N（没有 AcceptMsg 本身）和 AcceptMsg + AcceptMsg2..N 两种写法。"""

    def block(self, *lines):
        return npcevent.convert_block(list(lines), known={2547, 19621}, floors=FLOORS)

    def test_pages_numbered_from_one(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=19621", "AcceptMsg1:甲\\n一", "AcceptMsg2:乙",
                         "AcceptMsg3:丙", "ThanksMsg:谢谢", "GetItem:19621")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["paged"], {"accept": ["甲\n一", "乙", "丙"]})
        self.assertNotIn("accept", rec["msg"])
        self.assertEqual(rec["msg"]["thanks"], "谢谢")

    def test_plain_key_is_page_one(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=19621", "AcceptMsg:甲", "AcceptMsg2:乙",
                         "ThanksMsg1:谢1", "ThanksMsg2:谢2", "GetItem:19621")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["paged"], {"accept": ["甲", "乙"], "thanks": ["谢1", "谢2"]})

    def test_single_numbered_page_is_one_page(self):
        rec = self.block("EventNo:-1", "TYPE:REQUEST", "EVENT:ITEM!=19621", "RequestMsg1:问", "ThanksMsg:谢谢")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["paged"], {"request": ["问"]})

    def test_message_nomal_window_pages(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:ENDEV=4", "NomalWindowMsg:一", "NomalWindowMsg2:二")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["paged"], {"nomal_window": ["一", "二"]})

    def test_gap_in_pages_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=19621", "AcceptMsg1:甲", "AcceptMsg3:丙",
                         "ThanksMsg:谢谢", "GetItem:19621")
        self.assertIn("分页不连续", rec["unsupported"])

    def test_plain_key_with_page_one_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=19621", "AcceptMsg:甲", "AcceptMsg1:乙",
                         "ThanksMsg:谢谢", "GetItem:19621")
        self.assertIn("分页不连续", rec["unsupported"])

    def test_accept_pages_without_thanks_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=19621", "AcceptMsg1:甲", "AcceptMsg2:乙")
        self.assertIn("缺少 thanks", rec["unsupported"])

    def test_paged_key_of_other_message_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:ENDEV=4", "NomalMsg1:甲", "NomalMsg2:乙")
        self.assertIn("不支持的字段 NomalMsg1", rec["unsupported"])

    def test_pet_full_msg_with_pages_is_supported_without_get_pet(self):
        # STAGE-197：PetFullMsg 是台词，没有 GetPet 时只是一句没用到的台词，块本身支持。
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=19621", "AcceptMsg1:甲", "AcceptMsg2:乙",
                         "ThanksMsg:谢谢", "PetFullMsg:满了")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["msg"]["pet_full"], "满了")


class NowGroupTest(unittest.TestCase):
    """NOW=n 条件（STAGE-219，对照 NPC_EventFreeIfCheck）：含 NOW=n 的候选组恒不成立，整组丢掉。"""

    def block(self, *lines):
        return npcevent.convert_block(list(lines), known={2709, 2712}, floors=FLOORS)

    def test_now_group_is_dropped_and_others_kept(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:ENDEV=4&NOW=72,LV>0&ITEM=2709", "NomalMsg:你好")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(len(rec["cond"]), 1)
        self.assertEqual(rec["cond"][0], [{"op": "LV", "cmp": ">", "n": 0},
                                          {"op": "ITEM", "cmp": "=", "id": 2709, "n": 1}])
        self.assertEqual(rec["_now"], {"dropped": ["ENDEV=4&NOW=72"], "omit": False})

    def test_now_term_anywhere_in_group_drops_the_group(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:ENDEV=4&ENDEV=69&ENDEV=71&NOW=72&ENDEV!=180,LV>0",
                         "NomalMsg:你好")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(len(rec["cond"]), 1)
        self.assertEqual(rec["cond"][0][0], {"op": "LV", "cmp": ">", "n": 0})

    def test_block_with_only_now_groups_is_marked_for_omit(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:ENDEV=4&NOW=72", "NomalMsg:你好")
        self.assertEqual(rec["_now"], {"dropped": ["ENDEV=4&NOW=72"], "omit": True})

    def test_other_now_forms_stay_unsupported(self):
        for event in ("NOWEV!=72", "NOW=72*2", "NOW>72"):
            with self.subTest(event=event):
                rec = self.block("EventNo:-1", "TYPE:MESSAGE", f"EVENT:{event}", "NomalMsg:你好")
                self.assertIn("不支持", rec["unsupported"])
                self.assertNotIn("_now", rec)

    def test_evdel_group_index_follows_the_filtered_groups(self):
        # DelItem:EVDEL 的分组和条件一样按丢掉 NOW 组之后的序号对齐：第 2 组是 ITEM=2712。
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:ITEM=2709,NOW=5,ITEM=2712", "NomalMsg:你好",
                         "DelItem:EVDEL")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(len(rec["cond"]), 2)
        self.assertEqual(rec["del_item_ev"], [[{"id": 2709, "n": 0}], [{"id": 2712, "n": 0}]])

    def test_parse_script_omits_unreachable_block_and_keeps_numbering(self):
        text = "\n".join([
            "NomalMainMsg:你好",
            "EventNo:-1", "TYPE:MESSAGE", "EVENT:NOW=72", "NomalMsg:甲", "EventEnd",
            "EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:乙", "EventEnd",
        ])
        script, notes = npcevent.parse_script(text, known={2709}, floors=FLOORS)
        self.assertEqual(script["main"], ["你好"])
        self.assertEqual([b["msg"]["nomal"] for b in script["blocks"]], ["乙"])
        self.assertEqual(notes, ["块 1 省略（候选组全是 NOW 组）：NOW=72"])

    def test_event_record_prefixes_notes_with_file(self):
        text = "\n".join(["EventNo:-1", "TYPE:MESSAGE", "EVENT:ENDEV=4&NOW=72,LV>0", "NomalMsg:甲", "EventEnd"])
        rec, notes = npcevent.event_record("king/event69_7", text, known={2709}, floors=FLOORS)
        self.assertEqual(rec["file"], "king/event69_7")
        self.assertNotIn("_now", rec["blocks"][0])
        self.assertEqual(notes, ["king/event69_7 块 1 丢掉 NOW 组：ENDEV=4&NOW=72"])


class ItemListTrailingCommaTest(unittest.TestCase):
    """DelItem 末尾的空项（STAGE-219，对照 NPC_EventDelItem）：只跳过最后一个空项；GetItem 和中间的空项仍不支持。"""

    def block(self, *lines):
        return npcevent.convert_block(list(lines), known={2709, 2712}, floors=FLOORS)

    def test_del_item_trailing_comma_is_skipped(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0&ITEM=2709", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "DelItem:2709,2712,")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["del_item"], [{"id": 2709, "n": 0}, {"id": 2712, "n": 0}])

    def test_del_item_only_a_comma_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好", "DelItem:,")
        self.assertIn("DelItem:, 格式不支持", rec["unsupported"])

    def test_del_item_middle_empty_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好", "DelItem:2709,,2712")
        self.assertIn("格式不支持", rec["unsupported"])

    def test_get_item_trailing_comma_stays_unsupported(self):
        # 原版 GetItem 的空项会走 ITEM_makeItemAndRegist(0)，失败后中途返回，不照搬。
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:LV>0", "NomalMsg:你好", "GetItem:2712,")
        self.assertIn("GetItem:2712, 格式不支持", rec["unsupported"])


if __name__ == "__main__":
    unittest.main()
