package character

// STAGE-338：钓竿专家（sainasu/event/nevent01_1、nevent01_2）、加美训练师（poru/mary05，任务 48）、交付（poru/team03，
// 任务 47）、加美闲聊（poru/mary15）端到端（纯逻辑，不连数据库）。用真实白名单、真实选块和真实交易，断言命中的块、
// 道具和宠物增减、事件位。反向覆盖：材料只有一半、暗号“楍”不再给 2291、加美等级不够、背包满、宠物栏满、已完成再来。
// 任务 47/48 的接取链在 41、43 那里断了（见 docs/quest.md「钓竿、岚之圣石与加美」），这里用种好的进度测链上每一步。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	s338Fisher1 = 1256 // 钓竿专家：nevent01_1（鱼和壳换钓竿）
	s338Fisher2 = 1260 // 钓竿专家：nevent01_2（渔村），卡鲁它那的 1299 用同一脚本
	s338Fisher3 = 1299
	s338Mary05  = 1322 // 加美训练师：给加美（模板 721，GetPet:1482）
	s338Team03  = 1323 // 交付：仙人钓竿 2432 + 33 级加美给岚之圣石 18559
	s338Mary15  = 1324 // 加美闲聊

	s338Rod1  = 2432  // 仙人钓竿
	s338Rod2  = 2433  // 工匠钓竿
	s338Rod3  = 2434  // 名人钓竿
	s338Stone = 18559 // 岚之圣石
	s338Pet   = 721   // 加美（模板）
	s338Ring  = 2291  // 深恋戒指：暗号“楍”原来给的，已删
	s338Ev47  = 57
	s338Ev48  = 68
	s338Ev41  = 64 // 任务 41（四圣石接任务）的完成事件，任务 47 的前置
	s338Ev43  = 55 // 任务 43（练鲁乌）的完成事件，任务 48 的前置
)

// s338Rods 是 9 种材料组合和它们的块序号（删掉暗号块以后的序号）。
var s338Rods = []struct {
	a, b, block, product int
}{
	{2569, 2572, 0, 2432},
	{2569, 2573, 1, 2432},
	{2569, 2574, 2, 2433},
	{2570, 2572, 3, 2433},
	{2570, 2573, 4, 2433},
	{2571, 2572, 5, 2433},
	{2570, 2574, 6, 2434},
	{2571, 2573, 7, 2434},
	{2571, 2574, 8, 2434},
}

// s338Daytime 确认测试时间是白天：nevent01 的 ACCEPT 块都带 TIME!=0（原版“工作只能在白天作啊”）。
func s338Daytime(t *testing.T) {
	t.Helper()
	if gamedata.EventTimeSection(kingNow) == 0 {
		t.Fatalf("kingNow 不在白天，TIME!=0 的块不会成立")
	}
}

// s338FullBag 造满背包：每格一件，第一格放 first，其余放占位道具 18556（只占格子）。
func s338FullBag(first int) []itemRec {
	out := make([]itemRec, 0, MaxItemSlot-firstBagSlot+1)
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		id := 18556
		if slot == firstBagSlot {
			id = first
		}
		out = append(out, itemRec{Slot: slot, ItemID: id, Count: 1})
	}
	return out
}

// 钓竿专家（1260、1299）：9 种材料组合各换出一根钓竿，材料收走，两个 NPC 都一样。
func TestNevent01_2RodFromMaterials(t *testing.T) {
	s338Daytime(t)
	s := kingService()
	for _, npc := range []int{s338Fisher2, s338Fisher3} {
		for _, c := range s338Rods {
			r := kingRecord(kingItems(c.a, c.b), nil, nil)
			blk := poruPick(t, s, r, npc, c.block)
			kingRun(t, r, blk)
			if countItem(r.Items, c.product) != 1 || countItem(r.Items, c.a) != 0 || countItem(r.Items, c.b) != 0 {
				t.Fatalf("NPC %d 用 %d+%d 应换到 %d 并收走两样材料，实际 %v", npc, c.a, c.b, c.product, r.Items)
			}
		}
	}
}

// 暗号“楍”：原版的无条件块（只要求 LV>0、给 2291）已从转换结果里删掉。说“楍”没有材料时什么都不给，
// 有材料时只按正常的交易块走（2570+2573 换 2433），绝不给 2291。
func TestNevent01_2KeywordNoLongerGivesRing(t *testing.T) {
	s := kingService()
	for _, npc := range []int{s338Fisher2, s338Fisher3} {
		script, ok := gamedata.NpcEventOf(npc)
		if !ok {
			t.Fatalf("NPC %d 没有任务脚本", npc)
		}
		for _, b := range script.Blocks {
			if b.Keyword != "" {
				t.Fatalf("%s 还有暗号块 %q，原版的无条件 2291 块应已删掉", script.File, b.Keyword)
			}
		}
		empty := kingRecord(nil, nil, nil)
		if idx, refuse := s.pickEventBlock(empty, script, "楍"); refuse || idx != -1 {
			t.Fatalf("NPC %d 没材料时说“楍”应闲聊，实际 idx=%d refuse=%v", npc, idx, refuse)
		}
		if len(empty.Items) != 0 {
			t.Fatalf("说“楍”不应给道具，实际 %v", empty.Items)
		}
		r := kingRecord(kingItems(2570, 2573), nil, nil)
		blk := poruPick(t, s, r, npc, 4)
		kingRun(t, r, blk)
		if countItem(r.Items, s338Ring) != 0 || countItem(r.Items, s338Rod2) != 1 {
			t.Fatalf("NPC %d 用 2570+2573 应换 2433 且没有 2291，实际 %v", npc, r.Items)
		}
	}
}

// 反向：只有曼模竿或只有钓鱼线，哪一块都不成立，NPC 闲聊、不给东西。
func TestNevent01_2NeedsBothMaterials(t *testing.T) {
	s := kingService()
	poruPick(t, s, kingRecord(kingItems(2569), nil, nil), s338Fisher2, -1)
	poruPick(t, s, kingRecord(kingItems(2572), nil, nil), s338Fisher2, -1)
	poruPick(t, s, kingRecord(kingItems(2570, 2571), nil, nil), s338Fisher2, -1)
}

// nevent01_1：名人钓竿 2434 换鱼 2437、壳 11872×2、红色贝壳 11863（用一根钓竿换三样，原版如此）。
func TestNevent01_1TradesRodForFish(t *testing.T) {
	s338Daytime(t)
	s := kingService()
	r := kingRecord(kingItems(s338Rod3), nil, nil)
	kingRun(t, r, poruPick(t, s, r, s338Fisher1, 0))
	if countItem(r.Items, 2437) != 1 || countItem(r.Items, 11872) != 2 || countItem(r.Items, 11863) != 1 ||
		countItem(r.Items, s338Rod3) != 0 {
		t.Fatalf("2434 应换 2437、11872×2、11863 并收走钓竿，实际 %v", r.Items)
	}
}

// 反向：背包满时换不了（换一根钓竿只空出一格，要三格）；钓竿不动，什么都不给。
func TestNevent01_1BagFullChangesNothing(t *testing.T) {
	s338Daytime(t)
	s := kingService()
	r := kingRecord(s338FullBag(s338Rod3), nil, nil)
	blk := poruPick(t, s, r, s338Fisher1, 0)
	before := countItem(r.Items, s338Rod3)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail == "" {
		t.Fatalf("背包满时应交易失败并给台词，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, s338Rod3) != before || countItem(r.Items, 2437) != 0 {
		t.Fatalf("交易失败后背包不应变，实际 %v", r.Items)
	}
}

// 任务 48：加美训练师在任务 48 进行中时给一只加美（模板 721），给的不是任务送的宠物（能计入 PET 条件）。
// 给完以后 68 完成，再来不再给。
func TestMary05GivesTrainingPetOnce(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, []int{s338Ev48})
	kingRun(t, r, poruPick(t, s, r, s338Mary05, 4))
	if len(r.Pets) != 1 || r.Pets[0].TemplateID != s338Pet || r.Pets[0].EventGiven {
		t.Fatalf("应给一只非任务送的加美（模板 %d），实际 pets=%+v", s338Pet, r.Pets)
	}
	if !r.eventDone(s338Ev48) {
		t.Fatalf("领到加美后任务 48（事件 68）应完成，实际 %v", r.EventFlags)
	}
	// STAGE-341：交付后事件 57 进入进行中，再来命中“训练加美”的提示块（第 3 块），不再给宠物。
	if blk := poruPick(t, s, r, s338Mary05, 3); blk.Type != "MESSAGE" || blk.Event != 57 {
		t.Fatalf("再来应命中事件 57 的提示块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if len(r.Pets) != 1 || !r.eventDoing(57) {
		t.Fatalf("已完成再来不应再给宠物，且事件 57 应进行中，实际 pets=%+v flags=%v", r.Pets, r.EventFlags)
	}
}

// 反向：任务 48 没接（68 不在进行中）时加美训练师不给宠物。
func TestMary05NeedsQuest48(t *testing.T) {
	s := kingService()
	poruPick(t, s, kingRecord(nil, nil, nil), s338Mary05, -1)
}

// 反向：宠物栏满时领不到加美（PetFullMsg），宠物不变、事件不完成。
func TestMary05PetFullKeepsQuest(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, []int{s338Ev48})
	r.Pets = petsOf(MaxPetSlots)
	blk := poruPick(t, s, r, s338Mary05, 4)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail == "" {
		t.Fatalf("宠物栏满时应交易失败，实际 ok=%v fail=%q", ok, fail)
	}
	if len(r.Pets) != MaxPetSlots || !r.eventDoing(s338Ev48) {
		t.Fatalf("宠物栏满时宠物和进度都不应变，实际 pets=%d flags=%v", len(r.Pets), r.EventFlags)
	}
}

// 任务 48 的接取：交付处（1323）在钓竿任务 41 进行中、任务 43 已完成（ENDEV=55）时，给出 REQUEST 68，接下去找训练师。
// 没有 43 的完成事件时不命中（任务 43 的卡点，见 docs/quest.md）。
func TestTeam03RequestStartsQuest48(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{s338Ev43}, []int{s338Ev41})
	kingRun(t, r, poruPick(t, s, r, s338Team03, 6))
	if !r.eventDoing(s338Ev48) {
		t.Fatalf("REQUEST 68 后任务 48 应进行中，实际 %v", r.EventFlags)
	}
	r2 := kingRecord(nil, nil, []int{s338Ev41})
	poruPick(t, s, r2, s338Team03, -1)
}

// 任务 47 交付：仙人钓竿 2432 + 33 级加美，收走两样，给岚之圣石 18559，事件 57 完成，另给一只小斯天多斯（333）。
// 加美是 33 级（测试里直接种成 33 级，即训练好的状态）。
func TestTeam03DeliversStoneForTrainedPet(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s338Rod1), []int{s338Ev43}, []int{s338Ev41, s338Ev47})
	r.Pets = []petRec{petFor(0, s338Pet, 33, false)}
	kingRun(t, r, poruPick(t, s, r, s338Team03, 3))
	if countItem(r.Items, s338Stone) != 1 || countItem(r.Items, s338Rod1) != 0 || !r.eventDone(s338Ev47) {
		t.Fatalf("交付应得岚之圣石、收走钓竿、事件 57 完成，实际 items=%v flags=%v", r.Items, r.EventFlags)
	}
	for _, p := range r.Pets {
		if p.TemplateID == s338Pet {
			t.Fatalf("交付后加美应被收走，实际 %+v", r.Pets)
		}
	}
	// 完成后拿着岚之圣石再来：只说闲话（块 1），不再给东西。
	poruPick(t, s, r, s338Team03, 1)
	if countItem(r.Items, s338Stone) != 1 {
		t.Fatalf("完成后再来不应多给岚之圣石，实际 %v", r.Items)
	}
}

// 反向：加美只有 32 级，PET<33 的块命中，只说话，不交付。
func TestTeam03PetBelow33Refused(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s338Rod1), []int{s338Ev43}, []int{s338Ev41, s338Ev47})
	r.Pets = []petRec{petFor(0, s338Pet, 32, false)}
	poruPick(t, s, r, s338Team03, 4)
	if countItem(r.Items, s338Stone) != 0 || countItem(r.Items, s338Rod1) != 1 || len(r.Pets) != 1 {
		t.Fatalf("加美 32 级不应交付，实际 items=%v pets=%+v", r.Items, r.Pets)
	}
}

// 反向：没有仙人钓竿 2432（只有加美）时不交付；块 3 不成立，PET<33 也不成立（33 级不小于 33），闲聊。
func TestTeam03NeedsRod(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, []int{s338Ev41, s338Ev47})
	r.Pets = []petRec{petFor(0, s338Pet, 33, false)}
	poruPick(t, s, r, s338Team03, -1)
}

// 反向：背包满时，完成后丢了岚之圣石再来拿不到（ItemFullMsg），交易整笔不做。
func TestTeam03BagFullRefusesStone(t *testing.T) {
	s := kingService()
	r := kingRecord(s338FullBag(s338Rod2), []int{s338Ev47}, []int{s338Ev41})
	blk := poruPick(t, s, r, s338Team03, 2)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail == "" {
		t.Fatalf("背包满时拿不到岚之圣石，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, s338Stone) != 0 {
		t.Fatalf("背包满时不应有岚之圣石，实际 %v", r.Items)
	}
}
