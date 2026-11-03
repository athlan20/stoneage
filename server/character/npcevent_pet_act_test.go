package character

// 任务脚本的宠物动作（STAGE-197，docs/quest.md）：收宠物（DelPet，直接写和 EVDEL）、送宠物（GetPet）、NotDel 与
// DelItem:EVDEL。纯逻辑部分（planEvent 直接算，不需要数据库）在前面；流程部分（脚本、窗口、数据库、推送）在后面，需要数据库。

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

// petsOf 造 n 只随身的 560 号宠物（栏位 0 起，等级 10，休息）。
func petsOf(n int) []petRec {
	out := make([]petRec, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, petFor(i, 560, 10, false))
	}
	return out
}

// noRand 是不应该被调用的随机源：测试里用它确认某条路径不抽随机数。
func noRand(t *testing.T) func(int) int {
	return func(n int) int {
		t.Helper()
		t.Fatalf("不应该抽随机数（上限 %d）", n)
		return 0
	}
}

// fixedRand 返回固定的随机数（取 0 到 n-1 里的 v，超出时取 0）。
func fixedRand(v int) func(int) int {
	return func(n int) int {
		if v < n {
			return v
		}
		return 0
	}
}

// planWith 用给定的宠物和块算一趟交易（纯逻辑：没有道具、金币，只看宠物）。
func planWith(r *record, blk *gamedata.EventBlock, sel eventSel, rnd func(int) int) (eventPlan, string, bool) {
	return planEvent(r, blk, sel, rnd)
}

// 直接写的收宠物：PET>0-560*4 收走 4 只（不论骑乘中还是休息中），消息栏提示“交出XX。”。骑乘中的宠物也收走
// （删掉这一行，状态就没了）。
func TestPlanDelPetTakesCountAndRidingPet(t *testing.T) {
	pets := petsOf(5)
	pets[1].State = petStateRide
	r := &record{Pets: pets, Level: 50}
	blk := &gamedata.EventBlock{Type: "ACCEPT", DelPet: []gamedata.PetAsk{{Cmp: ">", Level: 0, Template: 560, Count: 4}}}
	plan, fail, ok := planWith(r, blk, eventSel{group: -1}, noRand(t))
	if !ok {
		t.Fatalf("5 只里有 5 只符合，应该收成，失败台词 %q", fail)
	}
	if len(plan.pets) != 1 || plan.pets[0].Slot != 4 {
		t.Fatalf("应该只剩 4 号栏位（按栏位顺序收走了 0–3），实际 %+v", plan.pets)
	}
	if len(plan.notes) != 4 || plan.notes[0] != "交出"+pets[0].Name+"。" {
		t.Fatalf("应该有 4 条“交出XX。”，实际 %v", plan.notes)
	}
}

// 数量不够：什么都不收，不说话（原版 NPC_AcceptDel 直接返回 FALSE）。
func TestPlanDelPetNotEnoughChangesNothing(t *testing.T) {
	r := &record{Pets: petsOf(3), Level: 50}
	blk := &gamedata.EventBlock{Type: "ACCEPT", DelPet: []gamedata.PetAsk{{Cmp: ">", Level: 0, Template: 560, Count: 4}}}
	_, fail, ok := planWith(r, blk, eventSel{group: -1}, noRand(t))
	if ok || fail != "" {
		t.Fatalf("只有 3 只，应该失败且没有台词，实际 ok=%v fail=%q", ok, fail)
	}
	if len(r.Pets) != 3 {
		t.Fatalf("失败不应改动记录里的宠物")
	}
}

// 没写数量（Count 0）：全部收走，但至少要有 1 只；一只都没有就失败。
func TestPlanDelPetWithoutCountTakesAll(t *testing.T) {
	blk := &gamedata.EventBlock{Type: "ACCEPT", DelPet: []gamedata.PetAsk{{Cmp: ">", Level: 0, Template: 560, Count: 0}}}
	plan, _, ok := planWith(&record{Pets: petsOf(3), Level: 50}, blk, eventSel{group: -1}, noRand(t))
	if !ok || len(plan.pets) != 0 {
		t.Fatalf("没写数量应该全部收走，实际 ok=%v 剩 %d 只", ok, len(plan.pets))
	}
	if _, _, ok := planWith(&record{Level: 50}, blk, eventSel{group: -1}, noRand(t)); ok {
		t.Fatalf("一只都没有时不应成功")
	}
}

// 战斗中收不了宠物：整笔失败、什么都不说（原版 NPC_EventDelPet 的战斗检查）。
func TestPlanDelPetInBattleFails(t *testing.T) {
	r := &record{Pets: petsOf(4), Level: 50}
	blk := &gamedata.EventBlock{Type: "ACCEPT", DelPet: []gamedata.PetAsk{{Cmp: ">", Level: 0, Template: 560, Count: 4}}}
	_, fail, ok := planWith(r, blk, eventSel{group: -1, battle: true}, noRand(t))
	if ok || fail != "" {
		t.Fatalf("战斗中应该失败且没有台词，实际 ok=%v fail=%q", ok, fail)
	}
}

// EVDEL：命中第几组，就收第几组的宠物。组 0 要 2 只，组 1 要 1 只。
func TestPlanDelPetEVDELPicksHitGroup(t *testing.T) {
	blk := &gamedata.EventBlock{
		Type: "ACCEPT",
		DelPetEV: [][]gamedata.PetAsk{
			{{Cmp: ">", Level: 0, Template: 560, Count: 2}},
			{{Cmp: ">", Level: 0, Template: 560, Count: 1}},
			{},
		},
	}
	plan, _, ok := planWith(&record{Pets: petsOf(4), Level: 50}, blk, eventSel{group: 1}, noRand(t))
	if !ok || len(plan.pets) != 3 {
		t.Fatalf("命中第 2 组应该只收 1 只，实际 ok=%v 剩 %d 只", ok, len(plan.pets))
	}
	plan, _, ok = planWith(&record{Pets: petsOf(4), Level: 50}, blk, eventSel{group: 0}, noRand(t))
	if !ok || len(plan.pets) != 2 {
		t.Fatalf("命中第 1 组应该收 2 只，实际 ok=%v 剩 %d 只", ok, len(plan.pets))
	}
	// 命中的组没有宠物项：什么都不收，交易照常成立。
	plan, _, ok = planWith(&record{Pets: petsOf(4), Level: 50}, blk, eventSel{group: 2}, noRand(t))
	if !ok || len(plan.pets) != 4 {
		t.Fatalf("命中的组没有宠物项应该什么都不收，实际 ok=%v 剩 %d 只", ok, len(plan.pets))
	}
}

// 任务送的宠物不算进收宠物的数量：只有它的话 DelPet 的检查不过。
func TestPlanDelPetSkipsEventGiven(t *testing.T) {
	pets := []petRec{petFor(0, 560, 10, true)}
	blk := &gamedata.EventBlock{Type: "ACCEPT", DelPet: []gamedata.PetAsk{{Cmp: ">", Level: 0, Template: 560, Count: 1}}}
	if _, _, ok := planWith(&record{Pets: pets, Level: 50}, blk, eventSel{group: -1}, noRand(t)); ok {
		t.Fatalf("任务送的宠物不应该被收走")
	}
}

// 送宠物：收完之后找第一个空栏位；前面收走的会空出栏位。宠物栏满时说 PetFullMsg，整笔不做。
func TestPlanGetPetNeedsFreeSlot(t *testing.T) {
	blk := &gamedata.EventBlock{
		Type:    "ACCEPT",
		Msg:     gamedata.EventMsg{PetFull: "满了"},
		GetPet:  []int{44},
		GetItem: []gamedata.ItemCount{{ID: 2712, N: 1}},
		DelPet:  []gamedata.PetAsk{{Cmp: ">", Level: 0, Template: 560, Count: 1}},
	}
	full := &record{Pets: petsOf(MaxPetSlots), Level: 50}
	if _, fail, ok := planWith(full, &gamedata.EventBlock{Type: "ACCEPT", Msg: blk.Msg, GetPet: []int{44}}, eventSel{group: -1}, fixedRand(0)); ok || fail != "满了" {
		t.Fatalf("栏位满时应该说 PetFullMsg 并失败，实际 ok=%v fail=%q", ok, fail)
	}
	// 收走一只之后就有空栏位，送宠物成功，栏位是刚空出来的那个。
	plan, _, ok := planWith(full, blk, eventSel{group: -1}, fixedRand(0))
	if !ok {
		t.Fatalf("收走一只后应该能送宠物")
	}
	if len(plan.pets) != MaxPetSlots || plan.pets[0].TemplateID != 63 || plan.pets[0].Slot != 0 {
		t.Fatalf("应该在 0 号栏位得到 63 号模板的宠物，实际 %+v", plan.pets)
	}
}

// 送宠物：多个编号时随机选一个（固定随机数，选第 2 个 = 45 号怪物，模板 64）；只有一个编号时不抽随机数。
func TestPlanGetPetRandomPick(t *testing.T) {
	blk := &gamedata.EventBlock{Type: "ACCEPT", Msg: gamedata.EventMsg{PetFull: "满了"}, GetPet: []int{44, 45}}
	plan, _, ok := planWith(&record{Level: 50}, blk, eventSel{group: -1}, fixedRand(1))
	if !ok || len(plan.pets) != 1 || plan.pets[0].TemplateID != 64 {
		t.Fatalf("随机到第 2 个应得到 64 号模板的宠物，实际 ok=%v %+v", ok, plan.pets)
	}
	single := &gamedata.EventBlock{Type: "ACCEPT", Msg: gamedata.EventMsg{PetFull: "满了"}, GetPet: []int{44}}
	noPick := func(n int) int { // 只有一个编号时不挑选（挑选的上限是 1）；送宠物的其他随机数上限是 4、5 等
		if n == 1 {
			t.Fatalf("只有一个编号时不应该抽随机数选编号")
		}
		return 0
	}
	if _, _, ok := planWith(&record{Level: 50}, single, eventSel{group: -1}, noPick); !ok {
		t.Fatalf("送宠物应该成功")
	}
}

// 送出的宠物：模板决定种类、名字、技能和品质，血量满，状态休息，等级在怪物的等级范围里（45 号怪物 1–1）。
// 任务送的标记只在 REQUEST 接任务的路径上打（原版 NPC_EventAdd mode 0）；ACCEPT 送的不打（NPC_AcceptDel）。
func TestPlanGetPetFieldsAndFlag(t *testing.T) {
	enemy, ok := gamedata.EnemyByID(44)
	if !ok {
		t.Fatal("敌人 44 不在表里")
	}
	tmpl, ok := gamedata.PetByID(enemy.Template)
	if !ok {
		t.Fatalf("模板 %d 不在表里", enemy.Template)
	}
	for _, typ := range []string{"ACCEPT", "REQUEST"} {
		blk := &gamedata.EventBlock{Type: typ, Msg: gamedata.EventMsg{PetFull: "满了"}, GetPet: []int{44}}
		plan, _, ok := planWith(&record{Level: 50}, blk, eventSel{group: -1}, fixedRand(0))
		if !ok || len(plan.pets) != 1 {
			t.Fatalf("%s 送宠物应该成功", typ)
		}
		p := plan.pets[0]
		if p.Name != tmpl.Name || p.TemplateID != tmpl.ID || p.State != petStateRest || p.HP != gamedata.PetMaxHP(p.Stats) {
			t.Fatalf("%s 送出的宠物字段不对：%+v", typ, p)
		}
		if p.Skills != tmpl.Skills || p.Rank != gamedata.PetRank(tmpl) {
			t.Fatalf("%s 送出的宠物技能或品质不对", typ)
		}
		if p.Level != enemy.LVMin && p.Level != enemy.LVMax {
			t.Fatalf("%s 等级 %d 不在怪物的等级范围里", typ, p.Level)
		}
		wantFlag := typ == "REQUEST"
		if p.EventGiven != wantFlag {
			t.Fatalf("%s 送出的宠物 EventGiven 应该是 %v，实际 %v", typ, wantFlag, p.EventGiven)
		}
		if len(plan.notes) != 1 || plan.notes[0] != "拿到"+tmpl.Name+"。" {
			t.Fatalf("%s 应该提示“拿到XX。”，实际 %v", typ, plan.notes)
		}
	}
}

// NotDel 和 DelItem:EVDEL：收命中组的道具，编号在 NotDel 里的留下（已在转换时排除，这里是 DelItemEV 的内容）。
// 没写数量（N 0）的收走背包里所有这个编号的道具。
func TestPlanDelItemEVDELUsesHitGroup(t *testing.T) {
	r := &record{
		Items: []itemRec{{Slot: 0, ItemID: 2590, Count: 1}, {Slot: 1, ItemID: 2589, Count: 2}, {Slot: 2, ItemID: 2594, Count: 1}},
		Level: 50,
	}
	blk := &gamedata.EventBlock{
		Type:      "ACCEPT",
		GetItem:   []gamedata.ItemCount{{ID: 2591, N: 1}},
		DelItemEV: [][]gamedata.ItemCount{{{ID: 2590, N: 1}}, {{ID: 2589, N: 0}}},
	}
	plan, _, ok := planWith(r, blk, eventSel{group: 1}, noRand(t))
	if !ok {
		t.Fatal("交易应该成立")
	}
	if countItem(plan.items, 2589) != 0 || countItem(plan.items, 2590) != 1 || countItem(plan.items, 2594) != 1 {
		t.Fatalf("命中第 2 组应该只收走 2589（全部），实际 %+v", plan.items)
	}
	plan, _, ok = planWith(r, blk, eventSel{group: 0}, noRand(t))
	if !ok || countItem(plan.items, 2590) != 0 || countItem(plan.items, 2589) != 2 {
		t.Fatalf("命中第 1 组应该只收走 2590 一件，实际 %+v", plan.items)
	}
}

// 命中的组：EVENT 逗号是或，取第一个全部满足的组序号；都不满足返回 -1。
func TestEventMatchedGroupIndex(t *testing.T) {
	r := &record{Pets: petsOf(4), Level: 50}
	groups := [][]gamedata.EventTerm{
		{petTerm(">", 0, 560, 5)},
		{petTerm(">", 0, 560, 4)},
		{petTerm(">", 0, 560, 1)},
	}
	if got := eventMatchedGroup(r, groups, time.Now()); got != 1 {
		t.Fatalf("应该命中第 2 组，实际 %d", got)
	}
	if got := eventMatchedGroup(&record{Level: 50}, groups, time.Now()); got != -1 {
		t.Fatalf("没有宠物时应该没有命中，实际 %d", got)
	}
}

// 文本里的台词不影响判断：PetFullMsg 只在送宠物时用，这里只要确认它被保留下来。
func TestPetFullMsgIsKept(t *testing.T) {
	blk := &gamedata.EventBlock{Type: "ACCEPT", Msg: gamedata.EventMsg{PetFull: "你的宠物太多了"}, GetPet: []int{44}}
	_, fail, ok := planWith(&record{Pets: petsOf(MaxPetSlots), Level: 50}, blk, eventSel{group: -1}, fixedRand(0))
	if ok || !strings.Contains(fail, "宠物太多") {
		t.Fatalf("宠物栏满的台词应该是 PetFullMsg，实际 %q", fail)
	}
}

// ---- 以下需要数据库（流程：脚本、窗口、写库、推送）----

// petRows 数角色在数据库里的宠物行（slot → event_given）。
func (e *env) petRows(t *testing.T) map[int]bool {
	t.Helper()
	rows, err := e.chars.db.Query(context.Background(), `SELECT slot, event_given FROM character_pets
		WHERE character_id = (SELECT id FROM characters WHERE name = '勇者小明') ORDER BY slot`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var slot int
		var given bool
		if err := rows.Scan(&slot, &given); err != nil {
			t.Fatal(err)
		}
		out[slot] = given
	}
	return out
}

// clearPetsSetup 清掉角色的宠物（新角色出生时会送初始宠物，STAGE-62；测试要自己定宠物）。
func clearPetsSetup(t *testing.T) func(*env) {
	return func(e *env) {
		if err := ClearPets(context.Background(), e.chars.db, "勇者小明"); err != nil {
			t.Fatal(err)
		}
	}
}

// givePetSetup 给角色一只宠物（setup 用，同 givepet 命令）。
func givePetSetup(t *testing.T, template, level int) func(*env) {
	return func(e *env) {
		if _, err := GivePet(context.Background(), e.chars.db, DefaultPetNotifyChannel, "勇者小明", template, level, func(int) int { return 0 }); err != nil {
			t.Fatal(err)
		}
	}
}

// 流程（jaruga/event/bait_03a2 的第一块）：5 只 560 号、2590 和 2594 各一件，选“是”：EVDEL 收走 5 只宠物和
// 命中组里的 2590（2594 有 NotDel 留下）、给 2591；宠物都没了，数据库和推送都对。
func TestEvdelDelPetFlowKeepsNotDelItem(t *testing.T) {
	const file = "jaruga/event/bait_03a2"
	npc := pagedFileNPC(t, file)
	setups := []func(*env){clearPetsSetup(t), putItemSetup(t, 5, 2590, 1), putItemSetup(t, 6, 2594, 1)}
	for i := 0; i < 5; i++ {
		setups = append(setups, givePetSetup(t, 560, 10))
	}
	e, c := pagedEnv(t, npc, []string{file}, setups...)

	expectWindow(t, talkTo(c, npc.ID), "\n你所有的材料恐龙", 2) // 台词以换行开头
	pushes := choose(c, 1)
	if !hasPush(pushes, "pets_changed") || !hasPush(pushes, "inventory_changed") || !hasPush(pushes, "notice") {
		t.Fatalf("交易后应推 pets_changed、inventory_changed 和交出宠物的 notice: %v", pushes)
	}
	if rows := e.petRows(t); len(rows) != 0 {
		t.Fatalf("5 只 560 号应该全部收走，实际 %v", rows)
	}
	if got := e.itemCounts(t); got[2590] != 0 || got[2594] != 1 || got[2591] != 1 {
		t.Fatalf("应该收走 2590、留下 2594、给 2591，实际 %v", got)
	}
}

// 流程：骑乘中的宠物也收走（数据库里这一行没了）。
func TestEvdelDelPetFlowTakesRidingPet(t *testing.T) {
	const file = "jaruga/event/bait_03a2"
	npc := pagedFileNPC(t, file)
	setups := []func(*env){clearPetsSetup(t), putItemSetup(t, 5, 2590, 1), putItemSetup(t, 6, 2594, 1)}
	for i := 0; i < 5; i++ {
		setups = append(setups, givePetSetup(t, 560, 10))
	}
	setups = append(setups, func(e *env) {
		e.exec(t, `UPDATE character_pets SET state = 4 WHERE character_id = (SELECT id FROM characters WHERE name = '勇者小明') AND slot = 0`)
	})
	e, c := pagedEnv(t, npc, []string{file}, setups...)
	expectWindow(t, talkTo(c, npc.ID), "\n你所有的材料恐龙", 2) // 台词以换行开头
	choose(c, 1)
	if rows := e.petRows(t); len(rows) != 0 {
		t.Fatalf("骑乘中的宠物也应该被收走，实际剩 %v", rows)
	}
}

// 流程（seimu/event/event02_1 的接任务块）：有 221、222 号宠物（25 级）和 2607、任务 35 进行中，选“是”：
// 收走两只宠物，送 93 号怪物的宠物（ACCEPT 送的不是任务宠物，event_given 为假）。
func TestGetPetFlowGivesPetAfterDelPet(t *testing.T) {
	const file = "seimu/event/event02_1"
	npc := pagedFileNPC(t, file)
	setups := []func(*env){
		clearPetsSetup(t),
		putItemSetup(t, 5, 2607, 1),
		givePetSetup(t, 221, 25),
		givePetSetup(t, 222, 25),
		questStateSetup(t, 4, questDoing), // 任务 4（卡坦 找4只鲨鱼）的进行中事件号是 35
	}
	e, c := pagedEnv(t, npc, []string{file}, setups...)
	expectWindow(t, talkTo(c, npc.ID), "\n我拜托你捕抓类似鱼状的恐龙", 2)
	pushes := choose(c, 1)
	if !hasPush(pushes, "pets_changed") {
		t.Fatalf("交易后应推 pets_changed: %v", pushes)
	}
	rows := e.petRows(t)
	if len(rows) != 1 {
		t.Fatalf("应该剩 1 只（送出的 93 号），实际 %v", rows)
	}
	for _, given := range rows {
		if given {
			t.Fatalf("ACCEPT 送出的宠物不应该打任务标记")
		}
	}
}
