package character

// STAGE-337：涛之圣石（任务 45、46）端到端（纯逻辑，不连数据库）。涉及迪欧训练师（poru/mary06，NPC 1327）、
// 交付（poru/team02，NPC 1326：任务 46 接取、任务 45 交付）和迪欧闲聊（poru/mary16，NPC 1325）。
// 用真实白名单、真实选块和真实交易，断言命中的块、道具和宠物增减、事件位变化。
// 真实走法：接任务 46（REQUEST 67）→ 迪欧训练师交付（ACCEPT 67，给迪欧，事件 56 由 keepDoing 进入进行中）
// → 练到 30 级 → 交付（ACCEPT 56，给涛之圣石）。事件 55（任务 43）按需求直接设成完成。
// 反向：宠物不到 30 级、没有采伐许可证、宠物栏满、已完成再来、没接任务、丢了圣石的补发（原版也能补，见 docs/quest.md）。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	s337Mary06 = 1327  // 迪欧训练师（poru/mary06）：交付后给迪欧（任务 46 完成）
	s337Team   = 1326  // 交付（poru/team02）：任务 46 接取，交采伐许可证给涛之圣石（任务 45 完成）
	s337Chat   = 1325  // 迪欧闲聊（poru/mary16）
	s337Ev45   = 56    // 任务 45 的完成事件（涛之圣石）
	s337Ev46   = 67    // 任务 46 的完成事件（迪欧）
	s337Ev41   = 64    // 任务 41 进行中：任务 45、46 的前置（NOWEV=64）
	s337Ev43   = 55    // 任务 43 完成：任务 46 接取的前置（ENDEV=55）
	s337Cert   = 2478  // 采伐许可证
	s337Stone  = 18557 // 涛之圣石
	s337Pet    = 719   // 迪欧（宠物模板）
	s337Getpet = 1481  // 迪欧训练师给迪欧用的怪物编号（GetPet）
	s337Filler = 18556 // 占位道具（只占格子）
)

// s337Mary06 造一只迪欧（不是任务送的，等级由调用方设）。
func s337Pets(level int) []petRec {
	return []petRec{{Slot: 0, TemplateID: s337Pet, Name: "迪欧", Level: level}}
}

// s337FullBag 造满背包：第一格放 first，其余放占位道具。
func s337FullBag(first int) []itemRec {
	out := make([]itemRec, 0, MaxItemSlot-firstBagSlot+1)
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		id := s337Filler
		if slot == firstBagSlot {
			id = first
		}
		out = append(out, itemRec{Slot: slot, ItemID: id, Count: 1})
	}
	return out
}

// s337Idx 取 NPC 按真实选块命中的块序号（-1 表示闲聊），用于确认某个块不会命中。
func s337Idx(t *testing.T, s *Service, r *record, npc int) int {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	idx, refuse := s.pickEventBlock(r, script, "")
	if refuse {
		t.Fatalf("NPC %d 应该能对话，实际拒绝", npc)
	}
	return idx
}

// 真实走法：接任务 46 → 迪欧训练师交付（事件 56 因 keepDoing 进入进行中）→ 练到 30 级 → 交付拿涛之圣石。
// 交付后两个 NPC 只说话，迪欧和圣石都不再给。
func TestStage337RealWalk(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s337Cert), []int{s337Ev43}, []int{s337Ev41})

	// 接任务 46：REQUEST 67。此时 56 还没进行中，交付块（块 3）不会命中。
	kingRun(t, r, poruPick(t, s, r, s337Team, 6))
	if !r.eventDoing(s337Ev46) || r.eventDoing(s337Ev45) {
		t.Fatalf("接任务 46 后应是 67 进行中、56 没进行中，实际 %v", r.EventFlags)
	}
	kingTalk(t, s, r, s337Team, 5) // 去找小队长（块 5，NOWEV=67 的闲聊）

	// 迪欧训练师交付：ACCEPT 67 给迪欧，67 完成，56 由 keepDoing 设成进行中。
	kingRun(t, r, poruPick(t, s, r, s337Mary06, 4))
	if !r.eventDone(s337Ev46) || !r.eventDoing(s337Ev45) {
		t.Fatalf("迪欧交付后应是 67 完成、56 进行中，实际 %v", r.EventFlags)
	}
	if len(r.Pets) != 1 || r.Pets[0].TemplateID != s337Pet || r.Pets[0].EventGiven {
		t.Fatalf("应给一只迪欧（模板 %d，非任务送的），实际 %+v", s337Pet, r.Pets)
	}

	// 练到 30 级（练级本身不在这里测）。交付处：ACCEPT 56，收 2478 和 30 级迪欧，给涛之圣石，56 完成。
	r.Pets[0].Level = 30
	kingRun(t, r, poruPick(t, s, r, s337Team, 3))
	if !r.eventDone(s337Ev45) || countItem(r.Items, s337Stone) != 1 || countItem(r.Items, s337Cert) != 0 {
		t.Fatalf("交付后应得到涛之圣石、收走采伐许可证、56 完成，实际 items=%v flags=%v", r.Items, r.EventFlags)
	}
	if len(r.Pets) != 0 {
		t.Fatalf("交付后 30 级迪欧应被收走，实际 %+v", r.Pets)
	}

	// 之后再找两人：迪欧训练师只说“有了迪欧的加入”（块 1），交付处只说“迪欧是最优秀的伐木工人”（块 1）。
	kingTalk(t, s, r, s337Mary06, 1)
	kingTalk(t, s, r, s337Team, 1)
	if len(r.Pets) != 0 || countItem(r.Items, s337Stone) != 1 {
		t.Fatalf("交付后再找两人不应再给迪欧或圣石，实际 pets=%+v items=%v", r.Pets, r.Items)
	}
}

// 交付要事件 56 进行中：没有迪欧训练师那一步（56 没进行中），即使身上有许可证、宠物 30 级，交付块也不命中。
func TestStage337TeamNeedsEvent56Doing(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s337Cert), []int{s337Ev43, s337Ev46}, []int{s337Ev41})
	r.Pets = s337Pets(30)
	if idx := s337Idx(t, s, r, s337Team); idx == 3 {
		t.Fatalf("56 没进行中时交付块不应命中")
	}
}

// 迪欧只给一次：67 完成、56 进行中，再找训练师只说话（块 3），不再给第二只；交付处也只说话。
// 67 完成后任务 46 的 REQUEST 67 不能再接（事件 67 已完成，选块跳过），再接不会重新打开迪欧的块。
func TestStage337PetOnlyOnce(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})
	r.Pets = s337Pets(1)
	kingTalk(t, s, r, s337Mary06, 3)
	kingTalk(t, s, r, s337Team, 4) // 迪欧还没到 30 级：PET<30 的台词
	if len(r.Pets) != 1 {
		t.Fatalf("再找训练师不应再给迪欧，实际 %+v", r.Pets)
	}

	// 56 完成后（迪欧已收走）：训练师说“有了迪欧的加入”，交付处说圣石的台词；REQUEST 67 不会再命中。
	done := kingRecord(kingItems(s337Stone), []int{s337Ev43, s337Ev46, s337Ev45}, []int{s337Ev41})
	kingTalk(t, s, done, s337Mary06, 1)
	kingTalk(t, s, done, s337Team, 1)
	if len(done.Pets) != 0 || countItem(done.Items, s337Stone) != 1 {
		t.Fatalf("已完成再来不应给迪欧或圣石，实际 pets=%+v items=%v", done.Pets, done.Items)
	}
	if idx := s337Idx(t, s, done, s337Team); idx == 6 {
		t.Fatalf("67 完成后 REQUEST 67 不应再命中")
	}
}

// 没接任务 46（67 不在进行中）时，训练师不给迪欧；没有 43 的完成事件（55）时交付处也不给任务 46。
func TestStage337NeedsQuest46(t *testing.T) {
	s := kingService()
	kingTalk(t, s, kingRecord(kingItems(s337Cert), []int{s337Ev43}, []int{s337Ev41}), s337Mary06, -1)
	kingTalk(t, s, kingRecord(kingItems(s337Cert), nil, []int{s337Ev41}), s337Team, -1)
}

// 反向：宠物不到 30 级，交付处只说“迪欧你要记得”（块 4），不给圣石、不收许可证、事件 56 不完成。
func TestStage337PetBelow30TalksOnly(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(s337Cert), []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})
	r.Pets = s337Pets(29)
	kingRun(t, r, poruPick(t, s, r, s337Team, 4))
	if countItem(r.Items, s337Stone) != 0 || countItem(r.Items, s337Cert) != 1 || len(r.Pets) != 1 || !r.eventDoing(s337Ev45) {
		t.Fatalf("29 级时不应交易，实际 items=%v pets=%+v flags=%v", r.Items, r.Pets, r.EventFlags)
	}
}

// 反向：没有采伐许可证（2478），即使迪欧 30 级也没有交付块命中，交付处闲聊。
func TestStage337NoCertNoTrade(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})
	r.Pets = s337Pets(30)
	kingTalk(t, s, r, s337Team, -1)
	if countItem(r.Items, s337Stone) != 0 || len(r.Pets) != 1 {
		t.Fatalf("没有许可证不应给圣石、不应收迪欧，实际 items=%v pets=%+v", r.Items, r.Pets)
	}
}

// 反向：宠物栏满时训练师给不了迪欧（PetFullMsg），67 仍是进行中、56 不进行，交易不成立。
func TestStage337PetFullKeepsQuest(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{s337Ev43}, []int{s337Ev41, s337Ev46})
	r.Pets = petsOf(MaxPetSlots)
	blk := poruPick(t, s, r, s337Mary06, 4)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail == "" {
		t.Fatalf("宠物栏满时应交易失败并给台词，实际 ok=%v fail=%q", ok, fail)
	}
	if len(r.Pets) != MaxPetSlots || !r.eventDoing(s337Ev46) || r.eventDoing(s337Ev45) || r.eventDone(s337Ev46) {
		t.Fatalf("宠物栏满时宠物和进度都不应变，实际 pets=%d flags=%v", len(r.Pets), r.EventFlags)
	}
}

// 背包满但许可证在包里：交易先收 2478 再给圣石，空出的格子够放圣石，照样交付。
func TestStage337FullBagWithCertStillTrades(t *testing.T) {
	s := kingService()
	r := kingRecord(s337FullBag(s337Cert), []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})
	r.Pets = s337Pets(30)
	kingRun(t, r, poruPick(t, s, r, s337Team, 3))
	if countItem(r.Items, s337Stone) != 1 || countItem(r.Items, s337Cert) != 0 || !r.eventDone(s337Ev45) {
		t.Fatalf("背包满但许可证在包里应照样交付，实际 items=%v flags=%v", r.Items, r.EventFlags)
	}
}

// 丢了圣石的补发（块 2，NOWEV=64&ENDEV=56&ITEM!=18557&ENDEV!=64，原版逐行一致）：56 完成、包里没有圣石时给一颗；
// 包里有了就不再给；任务 41（64）完成后不再补。背包满时补不上，什么都不改。
func TestStage337LostStoneReissue(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{s337Ev43, s337Ev46, s337Ev45}, []int{s337Ev41})
	kingRun(t, r, poruPick(t, s, r, s337Team, 2))
	if countItem(r.Items, s337Stone) != 1 {
		t.Fatalf("丢了圣石后应补一颗，实际 %v", r.Items)
	}
	kingTalk(t, s, r, s337Team, 1) // 包里有了：只说话
	if countItem(r.Items, s337Stone) != 1 {
		t.Fatalf("包里已有圣石时不应再给，实际 %v", r.Items)
	}

	// 背包满：补不上，包里没有圣石，也没有进度变化。
	full := kingRecord(s337FullBag(s337Filler), []int{s337Ev43, s337Ev46, s337Ev45}, []int{s337Ev41})
	blk := poruPick(t, s, full, s337Team, 2)
	if _, fail, ok := planEvent(full, blk, eventSel{group: -1}, kingRand); ok || fail == "" {
		t.Fatalf("背包满时补发应失败并给台词，实际 ok=%v fail=%q", ok, fail)
	}

	// 任务 41 完成（64 完成）后不再补：块 0 只说话。
	off := kingRecord(nil, []int{s337Ev43, s337Ev46, s337Ev45, s337Ev41}, nil)
	kingTalk(t, s, off, s337Team, 0)
	if countItem(off.Items, s337Stone) != 0 {
		t.Fatalf("64 完成后不应补圣石，实际 %v", off.Items)
	}
}

// 迪欧闲聊（poru/mary16）：全部块都是 MESSAGE、没有任何奖励或进度动作，任何状态下说话都不改东西。
func TestStage337ChatterGivesNothing(t *testing.T) {
	script, ok := gamedata.NpcEventOf(s337Chat)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", s337Chat)
	}
	for i, b := range script.Blocks {
		if b.Type != "MESSAGE" || b.Event != -1 || len(b.GetItem)+len(b.DelItem)+len(b.GetPet)+len(b.DelPet)+
			len(b.EndSet)+len(b.Clean)+len(b.GetRand)+len(b.DelItemEV)+len(b.DelPetEV) != 0 ||
			b.GetStone != 0 || b.DelStone != nil || b.Charm != 0 {
			t.Fatalf("mary16 块 %d 应是纯闲聊，实际 %+v", i, b)
		}
	}
	states := []*record{
		kingRecord(nil, nil, nil),
		kingRecord(kingItems(18556, 18557, 18559, 18558), []int{s337Ev43, s337Ev45}, []int{s337Ev41}),
		kingRecord(kingItems(s337Cert), nil, []int{s337Ev41, s337Ev45}),
	}
	s := kingService()
	for _, r := range states {
		before := len(r.Items)
		if idx := s337Idx(t, s, r, s337Chat); idx >= 0 && script.Blocks[idx].Type != "MESSAGE" {
			t.Fatalf("mary16 命中了非 MESSAGE 块 %d：%s", idx, script.Blocks[idx].Type)
		}
		if len(r.Items) != before || len(r.Pets) != 0 {
			t.Fatalf("迪欧闲聊不应改东西，实际 items=%v pets=%+v", r.Items, r.Pets)
		}
	}
}

// 数据检查：三个脚本在白名单里；任务 45、46 的完成事件对得上（56、67）；迪欧由交付块给、模板 719；
// 交付块的圣石、补发块的条件和原版一致（补发块：NOWEV=64&ENDEV=56&ITEM!=18557&ENDEV!=64）。
func TestStage337ScriptData(t *testing.T) {
	for _, c := range []struct {
		npc  int
		file string
	}{{s337Mary06, "poru/mary06"}, {s337Team, "poru/team02"}, {s337Chat, "poru/mary16"}} {
		script, ok := gamedata.NpcEventOf(c.npc)
		if !ok || script.File != c.file || !gamedata.NpcEventEnabled(c.file) {
			t.Fatalf("NPC %d 应对应放开的 %s，实际 ok=%v", c.npc, c.file, ok)
		}
	}
	for no, event := range map[int]int{45: s337Ev45, 46: s337Ev46} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	dio, _ := gamedata.NpcEventOf(s337Mary06)
	if blk := dio.Blocks[4]; blk.Type != "ACCEPT" || len(blk.GetPet) != 1 || blk.GetPet[0] != s337Getpet || len(blk.EndSet) != 1 || blk.EndSet[0] != s337Ev46 {
		t.Errorf("迪欧训练师的块 4 应是 ACCEPT 67、给怪物 1481 的迪欧，实际 %+v", blk)
	}
	if e, ok := gamedata.EnemyByID(s337Getpet); !ok || e.Template != s337Pet {
		t.Errorf("怪物 %d 的宠物模板应是 %d，实际 %+v ok=%v", s337Getpet, s337Pet, e, ok)
	}
	team, _ := gamedata.NpcEventOf(s337Team)
	if team.Blocks[3].Type != "ACCEPT" || team.Blocks[3].GetItem[0].ID != s337Stone || team.Blocks[3].EndSet[0] != s337Ev45 {
		t.Errorf("交付处的块 3 应是 ACCEPT 56、给圣石 18557，实际 %+v", team.Blocks[3])
	}
	if team.Blocks[2].GetItem[0].ID != s337Stone || len(team.Blocks[2].Cond[0]) != 4 {
		t.Errorf("补发块应给圣石 18557 且 4 个条件（NOWEV=64&ENDEV=56&ITEM!=18557&ENDEV!=64），实际 %+v", team.Blocks[2])
	}
}
