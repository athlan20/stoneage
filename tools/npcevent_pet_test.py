"""任务脚本的宠物写法转换（STAGE-197，tools/npcevent_pet.py）：PET 条件、DelPet（直接写和 EVDEL）、GetPet、
NotDel 与 DelItem:EVDEL。运行：python3 tools/npcevent_pet_test.py（或 python3 -m unittest tools/npcevent_pet_test.py）
"""
import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import npcevent  # noqa: E402
import npcevent_pet  # noqa: E402

FLOORS = {200}
KNOWN = {2590, 2589, 2594, 2596, 2758, 2607}


class ParsePetTest(unittest.TestCase):
    def test_condition_defaults_count_to_one(self):
        pet, why = npcevent_pet.parse_pet("PET=25-221", 1)
        self.assertEqual(why, "")
        self.assertEqual(pet, {"cmp": "=", "level": 25, "template": 221, "count": 1})

    def test_del_defaults_count_to_all(self):
        pet, _ = npcevent_pet.parse_pet("PET>0-560", 0)
        self.assertEqual(pet["count"], 0)

    def test_explicit_count_and_comparators(self):
        for text, cmp_, level in (("PET>0-560*4", ">", 0), ("PET<30-720*2", "<", 30), ("PET=77-12*1", "=", 77)):
            with self.subTest(text=text):
                pet, why = npcevent_pet.parse_pet(text, 1)
                self.assertEqual(why, "")
                self.assertEqual(pet["cmp"], cmp_)
                self.assertEqual(pet["level"], level)

    def test_ev_variant_is_unsupported(self):
        pet, why = npcevent_pet.parse_pet("PET>0-560*4EV", 1)
        self.assertIsNone(pet)
        self.assertIn("EV 变种", why)

    def test_bad_forms_are_unsupported(self):
        for bad in ("PET>0", "PET!=0-560", "PET>0-560*0", "PET>x-1", ""):
            with self.subTest(bad=bad):
                pet, why = npcevent_pet.parse_pet(bad, 1)
                self.assertIsNone(pet)
                self.assertTrue(why)


class ConvertBlockPetTest(unittest.TestCase):
    def block(self, *lines):
        return npcevent.convert_block(list(lines), known=KNOWN, floors=FLOORS)

    def test_condition_with_pet_is_supported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:PET>0-560*4&ITEM=2594", "NomalMsg:你好")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["cond"], [[{"op": "PET", "pet": {"cmp": ">", "level": 0, "template": 560, "count": 4}},
                                        {"op": "ITEM", "cmp": "=", "id": 2594, "n": 1}]])

    def test_ev_condition_marks_block_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:MESSAGE", "EVENT:PET>0-560*4EV", "NomalMsg:你好")
        self.assertIn("EV 变种", rec["unsupported"])

    def test_del_pet_direct_list(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:PET>0-560*4", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "DelPet:PET>0-560*4")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["del_pet"], [{"cmp": ">", "level": 0, "template": 560, "count": 4}])

    def test_del_pet_direct_without_count_means_all(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "DelPet:PET=30-720")
        self.assertEqual(rec["del_pet"][0]["count"], 0)

    def test_del_pet_evdel_is_per_group(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM=2590&PET>0-560*2,PET>0-560*1,LV>0",
                         "AcceptMsg:要吗？", "ThanksMsg:谢谢", "DelPet:EVDEL")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["del_pet_ev"], [
            [{"cmp": ">", "level": 0, "template": 560, "count": 2}],
            [{"cmp": ">", "level": 0, "template": 560, "count": 1}],
            [],
        ])

    def test_get_pet_needs_pet_full_msg(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "GetPet:44,45")
        self.assertIn("PetFullMsg", rec["unsupported"])

    def test_get_pet_with_pet_full_msg_is_supported(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "PetFullMsg:满了", "GetPet:44,45")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["get_pet"], [44, 45])
        self.assertEqual(rec["msg"]["pet_full"], "满了")

    def test_request_with_pet_actions_is_unsupported(self):
        for extra in ("DelPet:PET>0-560*1", "GetPet:44"):
            with self.subTest(extra=extra):
                rec = self.block("EventNo:19", "TYPE:REQUEST", "EVENT:LV>0", "RequestMsg:要吗？", "ThanksMsg:谢谢",
                                 "PetFullMsg:满了", extra)
                self.assertIn("REQUEST", rec["unsupported"])

    def test_not_del_excludes_items_in_evdel(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM=2590*1&ITEM=2594,ITEM=2589*2&ITEM=2594",
                         "AcceptMsg:要吗？", "ThanksMsg:谢谢", "NotDel:2594", "DelItem:EVDEL", "GetItem:2596")
        self.assertNotIn("unsupported", rec)
        self.assertEqual(rec["del_item_ev"], [[{"id": 2590, "n": 1}], [{"id": 2589, "n": 2}]])
        self.assertNotIn("del_item", rec)

    def test_evdel_item_without_count_means_all(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM=2758", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "DelItem:EVDEL")
        self.assertEqual(rec["del_item_ev"], [[{"id": 2758, "n": 0}]])

    def test_not_del_without_evdel_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:LV>0", "AcceptMsg:要吗？", "ThanksMsg:谢谢",
                         "NotDel:2594", "DelItem:2590")
        self.assertIn("NotDel", rec["unsupported"])

    def test_evdel_item_not_ne_is_unsupported(self):
        rec = self.block("EventNo:-1", "TYPE:ACCEPT", "EVENT:ITEM!=2590&ITEM=2589", "AcceptMsg:要吗？",
                         "ThanksMsg:谢谢", "DelItem:EVDEL")
        self.assertIn("ITEM!=", rec["unsupported"])


if __name__ == "__main__":
    unittest.main()
