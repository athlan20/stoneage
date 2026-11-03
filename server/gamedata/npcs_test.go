package gamedata

// 抽查 tools/gen-npcs.py 的输出（STAGE-68）：四个出生村和村长家的 NPC 与原版数据一致，
// 名字（GBK 转 UTF-8）不乱码；编号从 1 开始连续且不重复。

import (
	"sort"
	"testing"
)

func countByType(list []NPC) map[string]int {
	kinds := map[string]int{}
	for _, n := range list {
		kinds[n.Type]++
	}
	return kinds
}

func TestVillageNpcs(t *testing.T) {
	for _, tc := range []struct {
		floor int
		want  int
	}{
		// STAGE-69 收录报时 NPC 后，四个村各多了几名（1000 +10、2000 +3、3000 +1、4000 +6）；
		// STAGE-90 收录任务 NPC 后又多了（1000 +6、2000 +10、3000 +5、4000 +6），脚本没放开的不能对话。
		// STAGE-266 补主城精灵王（转生）后各多一名（1000 +1、2000 +1、3000 +1、4000 +1）。
		// STAGE-308 收录强者表（1000 +2、2000 +2、3000 +1、4000 +1，竞技场 130 不在村里）。
		{1000, 43}, {2000, 33}, {3000, 28}, {4000, 35},
	} {
		list := NPCsByFloor(tc.floor)
		if len(list) != tc.want {
			t.Errorf("地图 %d 应有 %d 个 NPC，实际 %d 个", tc.floor, tc.want, len(list))
			continue
		}
		kinds := countByType(list)
		for _, kind := range []string{"对话", "告示牌", "商店", "存档点", "传送员"} {
			if kinds[kind] == 0 {
				t.Errorf("地图 %d 应有「%s」类 NPC，实际没有", tc.floor, kind)
			}
		}
	}
	// 报时 NPC（STAGE-69）：村里的老师和酒保（原版 npcgen_timeman），三个村都有。
	for _, floor := range []int{1000, 2000, 4000} {
		if countByType(NPCsByFloor(floor))["报时"] == 0 {
			t.Errorf("地图 %d 应有「报时」类 NPC，实际没有", floor)
		}
	}
}

func TestHomeVillageNpcsByName(t *testing.T) {
	// 萨姆吉尔村（1000）：商店、存档点、告示牌的名字都来自原版 GBK 数据。
	want := map[string]string{
		"特产品贩卖员":   "商店",
		"萨姆吉尔的储存点": "存档点",
		"竞技场前的看板":  "告示牌",
		"村庄青年":     "对话",
	}
	byName := map[string]NPC{}
	for _, n := range NPCsByFloor(1000) {
		byName[n.Name] = n
	}
	for name, kind := range want {
		n, ok := byName[name]
		if !ok {
			t.Errorf("萨姆吉尔村应有一个叫「%s」的 NPC（原版数据），实际没有；名字不乱码才找得到", name)
			continue
		}
		if n.Type != kind {
			t.Errorf("「%s」应是「%s」，实际是「%s」", name, kind, n.Type)
		}
		if n.X <= 0 || n.Y <= 0 {
			t.Errorf("「%s」的坐标应是正数，实际 (%d,%d)", name, n.X, n.Y)
		}
	}
}

func TestHomeInteriorHasChiefCook(t *testing.T) {
	// 村长家（1006）有村长家的厨师（对话），还有萨姆吉尔的村长（任务，STAGE-90 收录）；没有 NPC 的地图返回 nil（当空列表用）。
	list := NPCsByFloor(1006)
	kinds := countByType(list)
	if len(list) != 2 || kinds["对话"] != 1 || kinds["任务"] != 1 {
		t.Errorf("村长家应有「村长家的厨师」（对话）和「萨姆吉尔的村长」（任务），实际 %+v", list)
	}
	if got := NPCsByFloor(5555); len(got) != 0 {
		t.Errorf("没有 NPC 的地图应返回空列表，实际 %d 个", len(got))
	}
}

// npcIDHoles 是删除的原版源 NPC 编号（STAGE-302 便利店素材集中化；1797 是 STAGE-309 删掉的斗技场寄放店）：编号分配后直接删除、不重排，留下空洞。
var npcIDHoles = map[int]bool{268: true, 317: true, 318: true, 345: true, 346: true, 490: true, 762: true, 1797: true}

func TestNpcIDsAreUniqueWithKnownHoles(t *testing.T) {
	NPCsByFloor(1000) // 触发加载
	var all []NPC
	for _, list := range npcByFloor {
		all = append(all, list...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	if len(all) == 0 {
		t.Fatal("一张 NPC 都没有，数据表是空的？")
	}
	// 编号从 1 起递增、不重复；缺号只允许是已删除的源 NPC（空洞不重排，已有编号不能变）。
	idx := 0
	for id := 1; idx < len(all); id++ {
		if all[idx].ID == id {
			idx++
			continue
		}
		if !npcIDHoles[id] {
			t.Fatalf("编号 %d 缺失，且不是已删除的源 NPC（编号应从 1 递增且不重复）", id)
		}
	}
	for i := 1; i < len(all); i++ {
		if all[i].ID == all[i-1].ID {
			t.Fatalf("编号 %d 重复", all[i].ID)
		}
	}
}
