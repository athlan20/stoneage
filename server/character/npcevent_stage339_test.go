package character

// STAGE-339：四圣石最终交付（任务 41，事件 64）端到端（纯逻辑，不连数据库）。交付是 poru/mary01（NPC 1316，海底萨姆吉尔）：
// 收 18556、18557、18559 全部和 18558 一颗，给宠物 310（模板 194，金格萨贝鲁），完成事件 64。
// 真实走法（每一步都经过真实白名单、真实选块和真实交易）：任务 40（老爷爷接、龙王交，事件 63）→ 41（玛丽娜丝接取）
// → 42（士兵公文、萨姆吉尔、泰坦队员的通行证，事件 65）→ 44（泰坦队长接取，小队长给鲁乌，事件 55 由 keepDoing 进行中）
// → 43（矿工给一颗矿石，最高级鉴定家换 2474，泰坦队长换岳之圣石）→ 46（迪欧训练师、涛之圣石）→ 炎（龙王、埋宝箱的、考古队员）
// → 48（加美训练师、钓竿专家换仙人钓竿、交付岚之圣石）→ 交付。
// 直接设的前置只有：宠物等级（鲁乌、迪欧、加美练到 30 或 33 级，练级不在这里测）、矿石（矿工只给一颗，其余 4 颗直接放进背包，
// 矿石是采出来的）、采伐许可证 2478（来源是 jaruga/event/20501ev5 块 16，要 2467，2467 要 2465，那条链这里不走）、
// 钓竿的材料 2569、2572（掉落物，这里直接放，钓竿本身仍是真实的 nevent01_2 换法）。
// 反向：缺任意一颗、没接任务 41、已完成再来（宠物只给一次）、宠物栏满、台词脚本只说话。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	s339Final   = 1316 // 海底萨姆吉尔（poru/mary01）：任务 41 的完成块（本需求放开）
	s339Enemy   = 310  // 金格萨贝鲁的怪物编号（GetPet:310）
	s339Pet     = 194  // 怪物 310 的宠物模板
	s339Mat1    = 2569 // 曼模竿（钓竿材料，掉落）
	s339Mat2    = 2572 // 超级姆依（钓竿材料，掉落）
	s339PetWant = 1    // 交付后金格萨贝鲁只能有一只
)

// s339Stones 是四颗圣石各一颗（岳、涛、炎、岚）。
func s339Stones() []int { return []int{poruStone, s337Stone, poruFireSt, s338Stone} }

// s339Add 把 n 件 id 放进背包里还空着的格子（从已占格子之后接着放）。
func s339Add(r *record, id, n int) {
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

// s339PetOf 数背包外的宠物里模板为 tmpl 的有几只。
func s339PetOf(r *record, tmpl int) int {
	n := 0
	for _, p := range r.Pets {
		if p.TemplateID == tmpl {
			n++
		}
	}
	return n
}

// s339Train 把模板 tmpl 的宠物练到 level（前置直接设，练级本身不测）。
func s339Train(t *testing.T, r *record, tmpl, level int) {
	t.Helper()
	for i := range r.Pets {
		if r.Pets[i].TemplateID == tmpl {
			r.Pets[i].Level = level
			return
		}
	}
	t.Fatalf("身上没有模板 %d 的宠物，没法练到 %d 级，实际 %+v", tmpl, level, r.Pets)
}

// 真实整条链：任务 41 接取到交付，每一步都用真实脚本结算。交付后金格萨贝鲁只有一只，四颗圣石只收走该收的（炎只收一颗）。
func TestStage339RealWalk(t *testing.T) {
	s338Daytime(t)
	s := kingService()
	r := kingRecord(nil, nil, nil)

	// 前置：任务 40 真实走完（老爷爷接、龙王交，给宠物 303，事件 63 完成）。
	kingRun(t, r, poruPick(t, s, r, poruGrandpa, 3))
	kingRun(t, r, poruPick(t, s, r, poruDragon, 1))
	if !r.eventDone(poruQ40) {
		t.Fatalf("任务 40 应完成，实际 %v", r.EventFlags)
	}

	// 任务 41：玛丽娜丝接取（REQUEST 64，要 ENDEV=63），事件 64 进行中。
	kingRun(t, r, poruPick(t, s, r, poruMarina, 5))
	if !r.eventDoing(poruQ41) {
		t.Fatalf("接任务 41 后事件 64 应进行中，实际 %v", r.EventFlags)
	}

	// 任务 42（事件 65）：泰坦队员接取 → 萨姆吉尔给两封公文 → 泰坦队员给通行证 18553，事件 65 完成。
	kingRun(t, r, poruPick(t, s, r, poruCourier, 7))
	kingRun(t, r, poruPick(t, s, r, poruSam, 3))
	kingRun(t, r, poruPick(t, s, r, poruSam, 2))
	kingRun(t, r, poruPick(t, s, r, poruCourier, 3))
	if !r.eventDone(poruQ42) || countItem(r.Items, poruPass) != 1 {
		t.Fatalf("任务 42 应完成并拿到通行证 %d，实际 flags=%v items=%v", poruPass, r.EventFlags, r.Items)
	}

	// 任务 44（事件 66）：泰坦队长接取（REQUEST 66，要 ENDEV=65）→ 小队长给鲁乌（模板 720），66 完成，55 由 keepDoing 进行中。
	kingRun(t, r, poruPick(t, s, r, poruTitan, 8))
	if !r.eventDoing(poruQ44) {
		t.Fatalf("接任务 44 后事件 66 应进行中，实际 %v", r.EventFlags)
	}
	kingRun(t, r, poruPick(t, s, r, poruSquad, 4))
	if !r.eventDone(poruQ44) || !r.eventDoing(poruQ43) || s339PetOf(r, poruPet44) != 1 {
		t.Fatalf("小队长应给鲁乌、66 完成、55 进行中，实际 pets=%+v flags=%v", r.Pets, r.EventFlags)
	}
	s339Train(t, r, poruPet44, 30)

	// 岳之圣石（任务 43，事件 55）：矿工给一颗矿石（没有时给 1 个），其余 4 颗前置直接放；最高级鉴定家 5 颗换 1 张 2474；
	// 泰坦队长收 2474 和 30 级鲁乌，给岳之圣石 18556，事件 55 完成。
	kingRun(t, r, poruPick(t, s, r, nevMiner, 0))
	if countItem(r.Items, nevOre3) != 1 {
		t.Fatalf("矿工应给一颗矿石，实际 %v", r.Items)
	}
	s339Add(r, nevOre3, 4)
	kingRun(t, r, poruPick(t, s, r, nevLv3, 3))
	if countItem(r.Items, nevCert3) != 1 || countItem(r.Items, nevOre3) != 0 {
		t.Fatalf("5 颗矿石应换 1 张超特级鉴定书，实际 %v", r.Items)
	}
	kingRun(t, r, poruPick(t, s, r, poruTitan, 5))
	if countItem(r.Items, poruStone) != 1 || !r.eventDone(poruQ43) || r.eventDoing(poruQ43) ||
		countItem(r.Items, nevCert3) != 0 || s339PetOf(r, poruPet44) != 0 {
		t.Fatalf("岳之圣石应到手、55 完成、2474 和鲁乌被收走，实际 items=%v pets=%+v flags=%v", r.Items, r.Pets, r.EventFlags)
	}

	// 涛之圣石（任务 46，事件 56）：交采伐许可证 2478 之前先接 67（REQUEST，要 ENDEV=55），迪欧训练师给迪欧（模板 719），
	// 练到 30 级后，许可证 + 30 级迪欧换涛之圣石 18557，事件 56 完成。
	kingRun(t, r, poruPick(t, s, r, s337Team, 6))
	kingRun(t, r, poruPick(t, s, r, s337Mary06, 4))
	if !r.eventDone(s337Ev46) || !r.eventDoing(s337Ev45) || s339PetOf(r, s337Pet) != 1 {
		t.Fatalf("迪欧交付后应是 67 完成、56 进行中、身上有迪欧，实际 pets=%+v flags=%v", r.Pets, r.EventFlags)
	}
	s339Train(t, r, s337Pet, 30)
	s339Add(r, s337Cert, 1) // 采伐许可证 2478：前置直接放（来源见文件头）
	kingRun(t, r, poruPick(t, s, r, s337Team, 3))
	if countItem(r.Items, s337Stone) != 1 || !r.eventDone(s337Ev45) || countItem(r.Items, s337Cert) != 0 || s339PetOf(r, s337Pet) != 0 {
		t.Fatalf("涛之圣石应到手、许可证和迪欧被收走、56 完成，实际 items=%v pets=%+v flags=%v", r.Items, r.Pets, r.EventFlags)
	}

	// 炎之圣石（事件 64 进行中，龙王 → 埋宝箱的 → 考古队员，两颗守护经挖掘物 B 换两颗）。
	poruFireCycle(t, s, r)
	if countItem(r.Items, poruFireSt) != 2 || !r.eventDoing(poruQ41) {
		t.Fatalf("炎之圣石应有两颗、任务 41 仍进行中，实际 items=%v flags=%v", r.Items, r.EventFlags)
	}

	// 岚之圣石（任务 48，事件 68 接取，事件 57 由 keepDoing 进行中）：加美训练师给加美（模板 721），练到 33 级；
	// 钓竿专家用两样材料换仙人钓竿 2432；交付时换岚之圣石 18559，事件 57 完成，还给一只小斯天多斯（333）。
	kingRun(t, r, poruPick(t, s, r, s338Team03, 6))
	kingRun(t, r, poruPick(t, s, r, s338Mary05, 4))
	if !r.eventDone(s338Ev48) || !r.eventDoing(s338Ev47) || s339PetOf(r, s338Pet) != 1 {
		t.Fatalf("加美交付后应是 68 完成、57 进行中、身上有加美，实际 pets=%+v flags=%v", r.Pets, r.EventFlags)
	}
	s339Train(t, r, s338Pet, 33)
	s339Add(r, s339Mat1, 1)
	s339Add(r, s339Mat2, 1)
	kingRun(t, r, poruPick(t, s, r, s338Fisher2, 0))
	if countItem(r.Items, s338Rod1) != 1 || countItem(r.Items, s339Mat1) != 0 || countItem(r.Items, s339Mat2) != 0 {
		t.Fatalf("两样材料应换一根仙人钓竿，实际 %v", r.Items)
	}
	kingRun(t, r, poruPick(t, s, r, s338Team03, 3))
	if countItem(r.Items, s338Stone) != 1 || !r.eventDone(s338Ev47) || countItem(r.Items, s338Rod1) != 0 || s339PetOf(r, s338Pet) != 0 {
		t.Fatalf("岚之圣石应到手、钓竿和加美被收走、57 完成，实际 items=%v pets=%+v flags=%v", r.Items, r.Pets, r.EventFlags)
	}

	// 交付（任务 41，事件 64）：收 18556、18557、18559 全部和 18558 一颗，给金格萨贝鲁，完成事件 64。
	if countItem(r.Items, poruFireSt) != 2 {
		t.Fatalf("交付前应有两颗炎之圣石，实际 %d", countItem(r.Items, poruFireSt))
	}
	kingRun(t, r, poruPick(t, s, r, s339Final, 0))
	if !r.eventDone(poruQ41) || r.eventDoing(poruQ41) {
		t.Fatalf("交付后任务 41 应完成，实际 %v", r.EventFlags)
	}
	for _, id := range []int{poruStone, s337Stone, s338Stone} {
		if countItem(r.Items, id) != 0 {
			t.Fatalf("交付应收走圣石 %d，实际 %v", id, r.Items)
		}
	}
	if countItem(r.Items, poruFireSt) != 1 {
		t.Fatalf("炎之圣石只收一颗，应剩 1 颗，实际 %d", countItem(r.Items, poruFireSt))
	}
	enemy, ok := gamedata.EnemyByID(s339Enemy)
	if !ok || enemy.Template != s339Pet {
		t.Fatalf("怪物 %d 应是模板 %d，实际 %+v", s339Enemy, s339Pet, enemy)
	}
	if s339PetOf(r, s339Pet) != s339PetWant {
		t.Fatalf("交付后应有一只金格萨贝鲁（模板 %d），实际 %+v", s339Pet, r.Pets)
	}

	// 交付后再找萨姆吉尔只说话：不再给宠物，圣石不再收。
	kingRun(t, r, poruPick(t, s, r, s339Final, 1))
	if s339PetOf(r, s339Pet) != s339PetWant || countItem(r.Items, poruFireSt) != 1 {
		t.Fatalf("交付后再找只能说话，实际 pets=%+v items=%v", r.Pets, r.Items)
	}
}

// 缺任意一颗（岳、涛、炎、岚，炎只缺那一颗也算）：交付块不命中，不收道具、不给宠物、事件 64 不完成。
func TestStage339MissingStoneNoReward(t *testing.T) {
	for _, miss := range s339Stones() {
		s := kingService()
		var have []int
		for _, id := range s339Stones() {
			if id != miss {
				have = append(have, id)
			}
		}
		r := kingRecord(kingItems(have...), []int{poruQ40}, []int{poruQ41})
		idx := s337Idx(t, s, r, s339Final)
		if idx == 0 {
			t.Fatalf("缺 %d 时交付块不应命中（第 0 块），实际命中 0", miss)
		}
		blk := poruPick(t, s, r, s339Final, idx)
		if blk.Type == "ACCEPT" {
			t.Fatalf("缺 %d 时不应命中交付 ACCEPT", miss)
		}
		if len(r.Pets) != 0 || r.eventDone(poruQ41) || len(r.Items) != len(have) {
			t.Fatalf("缺 %d 时不应给宠物或完成 64，实际 pets=%+v items=%v flags=%v", miss, r.Pets, r.Items, r.EventFlags)
		}
	}
}

// 没接任务 41（事件 64 没进行中）：四颗圣石都在，交付块也不命中。
func TestStage339NeedsQuest41(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s339Stones()...), []int{poruQ40}, nil)
	if idx := s337Idx(t, s, r, s339Final); idx == 0 {
		t.Fatalf("没接任务 41 时交付块不应命中")
	}
	kingRun(t, r, poruPick(t, s, r, s339Final, 4))
	if len(r.Pets) != 0 || countItem(r.Items, poruFireSt) != 1 || r.eventDone(poruQ41) {
		t.Fatalf("没接任务 41 不应收圣石或给宠物，实际 pets=%+v items=%v", r.Pets, r.Items)
	}
}

// 已完成再来：金格萨贝鲁只给一次。再凑齐四颗圣石，交付块不再命中，只说“尼斯大陆……”，宠物和圣石都不变。
func TestStage339DoneGivesPetOnce(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s339Stones()...), []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, s339Final, 0))
	if s339PetOf(r, s339Pet) != 1 {
		t.Fatalf("第一次交付应给一只金格萨贝鲁，实际 %+v", r.Pets)
	}
	for _, id := range s339Stones() {
		s339Add(r, id, 1)
	}
	kingRun(t, r, poruPick(t, s, r, s339Final, 1))
	if idx := s337Idx(t, s, r, s339Final); idx == 0 {
		t.Fatalf("完成后交付块不应命中")
	}
	if s339PetOf(r, s339Pet) != 1 || countItem(r.Items, poruFireSt) != 1 || countItem(r.Items, poruStone) != 1 {
		t.Fatalf("完成后再来不应再给宠物或收圣石，实际 pets=%+v items=%v", r.Pets, r.Items)
	}
}

// 宠物栏满：交付失败、什么都不改（圣石还在，事件 64 仍进行中）。
func TestStage339PetFullKeepsStones(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s339Stones()...), []int{poruQ40}, []int{poruQ41})
	r.Pets = petsOf(MaxPetSlots)
	blk := poruPick(t, s, r, s339Final, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail == "" {
		t.Fatalf("宠物栏满时应交易失败并给台词，实际 ok=%v fail=%q", ok, fail)
	}
	if len(r.Pets) != MaxPetSlots || countItem(r.Items, poruStone) != 1 || countItem(r.Items, poruFireSt) != 1 ||
		!r.eventDoing(poruQ41) || r.eventDone(poruQ41) {
		t.Fatalf("宠物栏满时圣石和进度都不应变，实际 pets=%d items=%v flags=%v", len(r.Pets), r.Items, r.EventFlags)
	}
}

// 数据检查：交付块是 ACCEPT 64，要 NOWEV=64 和四颗圣石，收 18556/18557/18559 全部和 18558 一颗，给 310，完成 64；
// 没有残留的转换问题。金格萨贝鲁的怪物 310 对应模板 194。
func TestStage339ScriptData(t *testing.T) {
	script, ok := gamedata.NpcEventOf(s339Final)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", s339Final)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Fatalf("%s 没在白名单里", script.File)
	}
	blk := &script.Blocks[0]
	if blk.Type != "ACCEPT" || blk.Event != poruQ41 || blk.Unsupported != "" || blk.MissingItem {
		t.Fatalf("交付块应是可用的 ACCEPT 64，实际 %+v", blk)
	}
	want := map[int]int{poruStone: 0, s337Stone: 0, poruFireSt: 1, s338Stone: 0}
	if len(blk.DelItem) != len(want) {
		t.Fatalf("交付应收 4 种道具，实际 %+v", blk.DelItem)
	}
	for _, d := range blk.DelItem {
		if n, ok := want[d.ID]; !ok || d.N != n {
			t.Fatalf("收 %d 的数量应是 %d（0 表示全部），实际 %+v", d.ID, n, blk.DelItem)
		}
	}
	if len(blk.GetPet) != 1 || blk.GetPet[0] != s339Enemy || len(blk.EndSet) != 1 || blk.EndSet[0] != poruQ41 {
		t.Fatalf("交付应给怪物 %d 并完成事件 64，实际 GetPet=%v EndSet=%v", s339Enemy, blk.GetPet, blk.EndSet)
	}
}

// 只说话的四个脚本（1313 迷路的小女孩、1314 害怕的小女孩、1218 小男孩、1221 泰坦队士兵）：全部块都是 MESSAGE、事件 -1，
// 没有道具、宠物、金币、事件位、奖励、暗号和不支持的块，说几次都不改状态。
func TestStage339TalkOnlyNpcs(t *testing.T) {
	for _, npc := range []int{1313, 1314, 1218, 1221} {
		script, ok := gamedata.NpcEventOf(npc)
		if !ok {
			t.Fatalf("NPC %d 没有任务脚本", npc)
		}
		if !gamedata.NpcEventEnabled(script.File) {
			t.Fatalf("%s 没在白名单里", script.File)
		}
		for i := range script.Blocks {
			b := &script.Blocks[i]
			changes := len(b.GetItem) + len(b.DelItem) + len(b.GetPet) + len(b.EndSet) + len(b.Clean) + len(b.DelItemEV) +
				len(b.DelPet) + len(b.DelPetEV) + len(b.GetRand) + len(b.Warp)
			if b.Type != "MESSAGE" || b.Event != -1 || changes != 0 || b.GetStone != 0 || b.DelStone != nil || b.Charm != 0 ||
				b.Unsupported != "" || b.Keyword != "" {
				t.Fatalf("%s 第 %d 块不是纯台词（类型 %s，事件 %d），不能放开", script.File, i, b.Type, b.Event)
			}
		}
	}
}
