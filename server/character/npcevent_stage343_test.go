package character

// STAGE-343：四圣石前置材料的真实来源（纯逻辑，不连数据库）。
// 采伐许可证 2478 只能靠 jaruga/event/20501ev1~ev3、ev5 的一条链拿到，链上每个 NPC 都用真实选块（pickEventBlock）和
// 真实结算（kingRun，同 planEvent）走一遍。结论和表格见 docs/quest.md「四圣石材料来源（STAGE-343）」。
// 原版数据下：2478 一人只能有一张（2466 只给一次），涛（team02，事件 56）和杉的手环（20501ev5 块 1）都要它，
// 赛尚在没有木材时会收走许可证（块 15），木材不到 14 个时换到的是另一档（块 2 收走许可证、只给石币）。
// STAGE-345 之后手环块不再收许可证，涛和岚两线可以同时做，整链见 npcevent_stage345_test.go。
// 掉落表和敌群挂钩在 TestStage343SourceTables 里核对（和 server/gamedata 的表一致）。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	s343Axe      = 2478 // 采伐许可证
	s343Wood     = 2468 // 良质的木材（敌人 563 掉，70%）
	s343Bracelet = 2481 // 杉的手环（赛尚块 1 给）
	s343Team02   = 1326 // 米兰达队长（poru/team02，涛之圣石，事件 56 要 2478）
	s343Barl     = 1285 // 樵夫巴尔（20501ev1/ev3，事件 18 给 2466 和推荐书 2467）
	s343Sainasu  = 1274 // 木材商人赛尚（20501ev5，块 1 手环、块 16 用 2467 换 2478）
	s343Pet      = 719  // 涛的交付要的迪欧模板
)

// s343Script 取 NPC 的脚本，并确认在白名单里（同 poruPick）。
func s343Script(t *testing.T, npc int) *gamedata.EventScript {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Fatalf("%s 不在白名单里，NPC %d 对话会说“还不能对话”", script.File, npc)
	}
	return script
}

// s343Pick 取 NPC 命中的块序号（-1 表示没有块、只说闲聊），不结算。
func s343Pick(t *testing.T, s *Service, r *record, npc int) int {
	t.Helper()
	idx, refuse := s.pickEventBlock(r, s343Script(t, npc), "")
	if refuse {
		t.Fatalf("NPC %d 应该能对话，实际拒绝", npc)
	}
	return idx
}

// s343Step 取 NPC 命中的第一块，按 kingRun 结算。没有命中块时测试失败。
func s343Step(t *testing.T, s *Service, r *record, npc int) *gamedata.EventBlock {
	t.Helper()
	script := s343Script(t, npc)
	idx := s343Pick(t, s, r, npc)
	if idx < 0 {
		t.Fatalf("NPC %d（%s）应命中一块，实际没有", npc, script.File)
	}
	blk := &script.Blocks[idx]
	kingRun(t, r, blk)
	return blk
}

// s343Add 把 n 件 id 放进背包空格（从已占格子之后接着放）。
func s343Add(r *record, id, n int) {
	slot := firstBagSlot
	for _, it := range r.Items {
		if it.Slot >= slot {
			slot = it.Slot + 1
		}
	}
	for i := 0; i < n; i++ {
		r.Items = append(r.Items, itemRec{Slot: slot + i, ItemID: id, Count: 1})
	}
}

// s343Record 造一个等级 30 的角色（可以接巴尔的 REQUEST，LV>14）。
func s343Record() *record {
	r := kingRecord(nil, nil, nil)
	r.Level = 30
	return r
}

// TestStage343AxeChainReal：采伐许可证的真实链，从巴尔的 REQUEST 到赛尚块 16 拿到 2478。
// 2466 是一次性的：事件 18 结束后巴尔只说话，哈恩的事件 19 也结束，2465 不会再出来。
func TestStage343AxeChainReal(t *testing.T) {
	s := kingService()
	r := s343Record()

	b := s343Step(t, s, r, s343Barl) // 巴尔：事件 18 REQUEST（LV>14），给 2466
	if b.Type != "REQUEST" || b.Event != 18 || countItem(r.Items, 2466) != 1 || !r.eventDoing(18) {
		t.Fatalf("巴尔应接事件 18 并给 2466，实际块 %s 事件 %d items=%v", b.Type, b.Event, r.Items)
	}
	s343Step(t, s, r, 1386) // 哈恩：事件 19 REQUEST（要 2466），给 2463
	s343Step(t, s, r, 1384) // 科巴尔特：2463 换 2464
	s343Step(t, s, r, 1386) // 哈恩：2464 + 2466 换斧头 2465（2466 收走）
	if countItem(r.Items, 2466) != 0 || countItem(r.Items, 2465) != 1 {
		t.Fatalf("2465 那一步应收走 2466、拿到 2465，实际 %v", r.Items)
	}
	s343Step(t, s, r, s343Barl) // 巴尔：2465 换 2467 + 2484，事件 18 结束
	if !r.eventDone(18) || countItem(r.Items, 2467) != 1 {
		t.Fatalf("巴尔应结束事件 18 并给 2467，实际 flags=%v items=%v", r.EventFlags, r.Items)
	}
	s343Step(t, s, r, s343Sainasu) // 赛尚块 16：2467 换许可证 2478
	if countItem(r.Items, s343Axe) != 1 || countItem(r.Items, 2467) != 0 {
		t.Fatalf("赛尚应用 2467 换到 2478，实际 %v", r.Items)
	}

	// 整条链只出一张：事件 18 结束后巴尔只说话（块 0），不会再给 2466。
	if idx := s343Pick(t, s, r, s343Barl); idx != 0 {
		t.Fatalf("事件 18 结束后巴尔应只说话（块 0），实际块 %d", idx)
	}
	// 哈恩的事件 19 也结束了：不再给 2463，也不会再收 2466 换 2465（块 1、2、4 都是事件 19 的块）。
	if idx := s343Pick(t, s, r, 1386); idx == 1 || idx == 2 || idx == 4 {
		t.Fatalf("事件 19 结束后哈恩不应再给 2463 或 2465，实际块 %d", idx)
	}
}

// TestStage343AxeNeedsFourteenWood：杉的手环要 14 个良质的木材（块 1 ≥14）。
// 13 个走块 2：收走许可证、给 16000 石币，没有手环。14 个走块 1：给手环，STAGE-345 起许可证不再被收走。
func TestStage343AxeNeedsFourteenWood(t *testing.T) {
	s := kingService()

	r := s343Record()
	r.Items = append(r.Items, itemRec{Slot: firstBagSlot, ItemID: s343Axe, Count: 1})
	s343Add(r, s343Wood, 13)
	if idx := s343Pick(t, s, r, s343Sainasu); idx != 2 {
		t.Fatalf("13 个木材应命中块 2（16000 石币档），实际块 %d", idx)
	}
	kingRun(t, r, &s343Script(t, s343Sainasu).Blocks[2])
	if countItem(r.Items, s343Bracelet) != 0 || countItem(r.Items, s343Axe) != 0 {
		t.Fatalf("13 个木材档不给手环，且把许可证收走，实际 %v", r.Items)
	}

	r = s343Record()
	r.Items = append(r.Items, itemRec{Slot: firstBagSlot, ItemID: s343Axe, Count: 1})
	s343Add(r, s343Wood, 14)
	blk := s343Step(t, s, r, s343Sainasu)
	if blk.Event != -1 || countItem(r.Items, s343Bracelet) != 1 || countItem(r.Items, s343Wood) != 0 {
		t.Fatalf("14 个木材应给手环并收走木材，实际 %v", r.Items)
	}
	if countItem(r.Items, s343Axe) != 1 {
		t.Fatalf("手环块不收许可证（STAGE-345），许可证应仍在，实际 %v", r.Items)
	}
}

// TestStage343TalkWithoutWoodKeepsAxe：拿着许可证却一根木材都没有去找赛尚（块 15），原版会收走许可证，
// STAGE-348 起不再收（没有奖励，纯属陷阱，丢了涛线就交不了）。
func TestStage343TalkWithoutWoodKeepsAxe(t *testing.T) {
	s := kingService()
	r := s343Record()
	r.Items = append(r.Items, itemRec{Slot: firstBagSlot, ItemID: s343Axe, Count: 1})
	if idx := s343Pick(t, s, r, s343Sainasu); idx != 15 {
		t.Fatalf("有许可证、没有木材应命中块 15，实际块 %d", idx)
	}
	kingRun(t, r, &s343Script(t, s343Sainasu).Blocks[15])
	if countItem(r.Items, s343Axe) != 1 || countItem(r.Items, s343Bracelet) != 0 {
		t.Fatalf("块 15 不应收走许可证（STAGE-348）也不给手环，实际 %v", r.Items)
	}
}

// TestStage343AxeOnlyOnce：涛（米兰达队长，事件 56）先交许可证。许可证交走之后，赛尚没有许可证可收；
// STAGE-349 起涛已交付的角色仍可用 14 个木材换一次手环（块 1 的第二组条件），否则岚线的木竿出不来（先手环、后涛的顺序见
// npcevent_stage345_test.go，先涛的顺序见 npcevent_stage349_test.go）。
func TestStage343AxeOnlyOnce(t *testing.T) {
	s := kingService()
	r := s343Record()
	r.Items = append(r.Items, itemRec{Slot: firstBagSlot, ItemID: s343Axe, Count: 1})
	r.Pets = append(r.Pets, petRec{TemplateID: s343Pet, Level: 30})
	kingRun(t, r, &s343Script(t, s343Team02).Blocks[3]) // 涛：收许可证和迪欧，给 18557，事件 56 完成
	if !r.eventDone(56) || countItem(r.Items, s343Axe) != 0 {
		t.Fatalf("涛应结束事件 56 并收走许可证，实际 flags=%v items=%v", r.EventFlags, r.Items)
	}
	s343Add(r, s343Wood, 14)
	if idx := s343Pick(t, s, r, s343Sainasu); idx != 1 {
		t.Fatalf("许可证已交给涛，14 个木材应走手环块（STAGE-349 第二组条件，块 1），实际块 %d", idx)
	}
}

// TestStage343SourceTables：材料的掉落、敌群挂钩、区域归属与 server/gamedata 的表一致（数据来自原版 GBK 表的转换）。
func TestStage343SourceTables(t *testing.T) {
	drops := []struct{ enemy, item, prob int }{
		{563, 2468, 700},
		{581, 2469, 800}, {582, 2470, 800}, {583, 2471, 800},
		{584, 2469, 50}, {584, 2470, 30}, {584, 2471, 10},
		{1074, 2569, 300}, {1075, 2570, 400}, {1076, 2571, 550},
		{1010, 2572, 10}, {1011, 2573, 30}, {1006, 2574, 30}, {443, 2574, 30},
	}
	for _, c := range drops {
		e, ok := gamedata.EnemyByID(c.enemy)
		if !ok {
			t.Fatalf("敌人 %d 不在表里", c.enemy)
		}
		found := false
		for _, d := range e.Drops {
			if d.Item == c.item && d.Prob == c.prob {
				found = true
			}
		}
		if !found {
			t.Fatalf("敌人 %d 应掉 %d（%d/1000），实际掉落 %+v", c.enemy, c.item, c.prob, e.Drops)
		}
	}

	// 敌群的持有条件（AppearItem）和成员。
	groups := []struct{ group, appear, enemy int }{
		{414, -1, 563}, // STAGE-350：原版要持有 2478，本服去掉（tools/gen-encounters.py）
		{539, 2481, 1074}, {540, 2481, 1075}, {541, 2481, 1076},
		{408, 2475, 581}, {409, 2476, 582}, {410, 2477, 583},
		{411, 2469, 584}, {412, 2470, 584}, {413, 2471, 584},
	}
	for _, c := range groups {
		g, ok := gamedata.EnemyGroupByID(c.group)
		if !ok {
			t.Fatalf("敌群 %d 不在表里", c.group)
		}
		if g.AppearItem != c.appear {
			t.Fatalf("敌群 %d 应持有 %d 才出现，实际 %d", c.group, c.appear, g.AppearItem)
		}
		if !groupHasEnemy(g, c.enemy) {
			t.Fatalf("敌群 %d 应含敌人 %d，实际 %+v", c.group, c.enemy, g.Enemies)
		}
	}

	// 区域：同一个坐标上命中的区域（取 zorder 最大）里要有对应的敌群。
	areas := []struct {
		floor, x, y int
		groups      []int
	}{
		{200, 700, 440, []int{414}},
		{200, 390, 250, []int{539, 540, 541}},
		{31401, 50, 30, []int{473, 475, 477}},
		{31501, 5, 5, []int{468, 471}},
		{10201, 5, 5, []int{318, 321}},
		{10603, 10, 10, []int{408, 409, 410}},
		{10605, 10, 10, []int{408, 411}},
	}
	for _, c := range areas {
		a := gamedata.EncounterAreaAt(c.floor, c.x, c.y)
		if a == nil {
			t.Fatalf("地图 %d (%d,%d) 没有遇敌区域", c.floor, c.x, c.y)
		}
		for _, g := range c.groups {
			if !areaHasGroup(a, g) {
				t.Fatalf("地图 %d (%d,%d) 的区域 %s 应含敌群 %d", c.floor, c.x, c.y, a.Key, g)
			}
		}
	}
}

func groupHasEnemy(g *gamedata.EnemyGroup, enemy int) bool {
	for _, e := range g.Enemies {
		if e.ID == enemy {
			return true
		}
	}
	return false
}

func areaHasGroup(a *gamedata.EncounterArea, group int) bool {
	for _, g := range a.Groups {
		if g.ID == group {
			return true
		}
	}
	return false
}
