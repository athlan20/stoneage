import importlib.util
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('exits',Path(__file__).with_name('gen-mapexits.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class ExitTests(unittest.TestCase):
    maps={1:(100,100),2:(100,100),3:(100,100)}
    def row(self,x,y,tx=10,ty=20,to=2,gate='NULL',source=1):
        return f'NONE:{gate}:{source},{x},{y}:{to},{tx},{ty}:NULL'
    def test_components_mapping_identity_and_order(self):
        rows=[self.row(5,5),self.row(5,6,11,21),self.row(6,7),self.row(20,20),self.row(5,5),self.row(5,5,gate='M')]
        exits,diags=m.generate(rows,self.maps)
        self.assertEqual(len(exits),4)
        door=next(e for e in exits if len(e['cells'])==2)
        self.assertEqual(door['cells'],[(5,5,10,20),(5,6,11,21)])
        self.assertEqual(sum(d[1]=='duplicate' for d in diags),1)
        rev,_=m.generate(list(reversed(rows)),self.maps)
        self.assertEqual(exits,rev)
        more,_=m.generate([self.row(30,30,to=3),*rows],self.maps)
        self.assertEqual(exits,[e for e in more if e['target']==2])
    def test_conflicts_disable_entire_component_and_all_targets(self):
        rows=[self.row(5,5),self.row(5,6),self.row(5,5,to=3),self.row(5,6,12,13)]
        exits,diags=m.generate(rows,self.maps)
        self.assertTrue(all(e['conflict'] for e in exits))
        self.assertEqual(len(exits),2)
        self.assertEqual(len(exits[0]['cells']),3)
        self.assertTrue(any(d[1]=='conflict' for d in diags))
    def test_validation_and_no_same_map_or_invented_reverse(self):
        rows=[self.row(1,1,to=9),self.row(100,1),self.row(1,1,tx=100),self.row(1,1,gate='X'),'WHAT:NULL:1,1,1:2,1,1:NULL','NONE:NULL:1,1,1:2,1,1:OTHER', self.row(1,1,to=1),self.row(2,2)]
        exits,diags=m.generate(rows,self.maps)
        self.assertEqual(len(exits),1)
        self.assertEqual(len(diags),6)
        self.assertTrue(exits[0]['one_way'])
        back=self.row(3,3,to=1,gate='N',source=2)
        exits,_=m.generate([self.row(2,2),back],self.maps)
        self.assertFalse(any(e['one_way'] for e in exits))
    def test_real_village_and_jiru(self):
        exits,diags=m.generate((m.MAPS/'mapwarp.txt').read_text(encoding='gbk').splitlines(),m.read_maps(m.MAPS))
        village=[e for e in exits if e['source']==2000]
        self.assertEqual(len(village),13)
        self.assertEqual(sum(len(e['cells']) for e in village),27)
        self.assertEqual([(e['cells'][0]) for e in village if e['target']==100],[(74,33,79,614),(108,79,114,660)])
        self.assertNotIn(2007,[e['target'] for e in village])
        self.assertEqual({e['target']:e['gate'] for e in village if e['target'] in (2011,2014)},{2011:'M',2014:'A'})
        self.assertEqual(len([e for e in exits if e['source']==300]),31)
        self.assertTrue(any(e['conflict'] for e in exits if e['source']==300))
        # STAGE-261: 27 same-map near-landings open; only the 5 truly ambiguous exits stay conflicted.
        self.assertEqual(sorted((e['source'],e['target']) for e in exits if e['conflict']),
                         [(300,31201),(300,31205),(810,809),(810,829),(21016,21011)])
        self.assertEqual(sum(not e['conflict'] for e in exits),1777-5)
        opened=[e for e in exits if e['source']==31901 and e['target']==300]
        self.assertEqual([(e['conflict'],e['cells']) for e in opened],[(False,[(0,43,239,98),(0,44,239,98)])])
        # STAGE-261: resolved landings follow file order, so only ids and conflict flags must be order-independent.
        rev=m.generate(list((m.MAPS/'mapwarp.txt').read_text(encoding='gbk').splitlines())[::-1],m.read_maps(m.MAPS))[0]
        self.assertEqual([(e['id'],e['conflict']) for e in exits],[(e['id'],e['conflict']) for e in rev])
    def test_adjacent_landings_on_one_map_keep_earliest_line(self):
        exits,diags=m.generate([self.row(5,5,11,21),self.row(5,5,10,21)],self.maps)
        self.assertEqual(len(exits),1)
        self.assertFalse(exits[0]['conflict'])
        self.assertEqual(exits[0]['cells'],[(5,5,11,21)])
        self.assertEqual([d for d in diags if d[1]=='conflict-resolved'],
                         [(2,'conflict-resolved','(1, 5, 5) -> (2, 10, 21); kept line 1')])
        exits,_=m.generate([self.row(5,5,10,21),self.row(5,5,10,20)],self.maps)
        self.assertEqual(exits[0]['cells'],[(5,5,10,21)])
    def test_far_or_cross_map_landings_stay_conflict(self):
        for rows in ([self.row(5,5,10,20),self.row(5,5,10,23)],
                     [self.row(5,5,10,20),self.row(5,5,10,21),self.row(5,5,11,21)],
                     [self.row(5,5,10,20),self.row(5,5,10,21),self.row(5,5,10,20,to=3)]):
            exits,diags=m.generate(rows,self.maps)
            self.assertTrue(all(e['conflict'] for e in exits))
            self.assertFalse(any(d[1]=='conflict-resolved' for d in diags))

if __name__=='__main__': unittest.main()
