package character

// STAGE-222：塔姆塔姆村的药草（任务 30，事件 1）的端到端（纯逻辑，用真实白名单、真实选块和交易，写法同
// npcevent_king_test.go）。链路：村庄小姑娘（1280，3110 塔姆塔姆的民家，event01_1）接任务 → 咍罗山的医生
// （1364，20105 森林洞窟山顶，event01_2）给草药 2401 → 村庄小姑娘收草药、给护身符 2427、结束事件 1。
// 原版 event01_2 里的磨药（1983 + 1982 → 1984）也在这里测。前置：没有（事件 1 的块只有 LV>0）。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 药草链里的 NPC 和道具编号（台词对应 event01_1、event01_2）。
const (
	herbGirl   = 1280 // 村庄小姑娘（3110 塔姆塔姆的民家，event01_1）
	herbDoctor = 1364 // 咍罗山的医生（20105 森林洞窟山顶，event01_2）
	herbLeaf   = 2401 // 草药（医生给，送给小姑娘的父亲）
	herbCharm  = 2427 // 库伊的护身符（小姑娘给的奖励，事件 1 完成时发）
	herbPowder = 1984 // 药粉（磨好的爱困草）
	herbGrass  = 1983 // 爱困草（原版 DelItem:1983，磨药时收走）
	herbTool   = 1982 // 磨药的器具（原版只检查有没有，不收）
)

// TestHerbChainWalksToCharm 从头走完任务 30：接任务 → 拿草药 → 交草药拿护身符 → 再找医生和小姑娘只剩闲谈。
func TestHerbChainWalksToCharm(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)

	// 1. 村庄小姑娘：还没接，接事件 1（REQUEST，不交易，设进行中）。
	blk := kingPick(t, s, r, herbGirl, 1)
	if blk.Type != "REQUEST" || blk.Event != 1 {
		t.Fatalf("小姑娘的第一块应是接事件 1（REQUEST），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(1) {
		t.Fatalf("接了任务，事件 1 应进行中，实际 %v", r.EventFlags)
	}

	// 2. 咍罗山的医生：事件 1 进行中、身上没草药，给草药（ACCEPT，没有事件号，不结束 1）。
	blk = kingPick(t, s, r, herbDoctor, 3)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("医生应给草药（ACCEPT 没有事件），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, herbLeaf) != 1 || !r.eventDoing(1) || r.eventDone(1) {
		t.Fatalf("应拿到草药且事件 1 仍进行中，实际 items=%v", r.Items)
	}

	// 3. 村庄小姑娘：交草药（收走 2401，给护身符 2427，结束事件 1）。
	blk = kingPick(t, s, r, herbGirl, 0)
	if blk.Type != "ACCEPT" || blk.Event != 1 {
		t.Fatalf("小姑娘应收草药（ACCEPT 事件 1），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, herbLeaf) != 0 || countItem(r.Items, herbCharm) != 1 {
		t.Fatalf("交完应收走草药、给护身符，实际 %v", r.Items)
	}
	if !r.eventDone(1) || r.eventDoing(1) {
		t.Fatalf("交完草药事件 1 应完成且不再进行中，实际 %v", r.EventFlags)
	}

	// 4. 之后两人都只闲谈：小姑娘说感谢（MESSAGE ENDEV=1），不再给东西。
	kingTalk(t, s, r, herbGirl, 2)
	if countItem(r.Items, herbCharm) != 1 || len(r.Items) != 1 {
		t.Fatalf("完成后闲谈不应改动背包，实际 %v", r.Items)
	}
	kingTalk(t, s, r, herbDoctor, -1)
}

// TestHerbGrindingExchange 医生的磨药（原版 event01_2 块 2）：有爱困草 1983 和器具 1982 时，收走 1983、给药粉 1984，
// 器具留下。没有器具时磨不了（块不命中，只剩闲谈）。
func TestHerbGrindingExchange(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(herbGrass, herbTool), []int{4}, nil)
	blk := kingPick(t, s, r, herbDoctor, 2)
	kingRun(t, r, blk)
	if countItem(r.Items, herbGrass) != 0 || countItem(r.Items, herbPowder) != 1 || countItem(r.Items, herbTool) != 1 {
		t.Fatalf("磨药后应收走爱困草、给药粉、器具留下，实际 %v", r.Items)
	}

	// 没有器具：不命中磨药块，只闲谈，爱困草留着。
	r = kingRecord(kingItems(herbGrass), nil, nil)
	kingTalk(t, s, r, herbDoctor, -1)
	if countItem(r.Items, herbGrass) != 1 || countItem(r.Items, herbPowder) != 0 {
		t.Fatalf("没有器具时不应磨药，实际 %v", r.Items)
	}
}

// TestHerbDoctorNeedsQuest 事件 1 没接时医生不给草药（块 3 要求 NOWEV=1），只闲谈。
func TestHerbDoctorNeedsQuest(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	kingTalk(t, s, r, herbDoctor, -1)
	if len(r.Items) != 0 || r.eventDoing(1) {
		t.Fatalf("没接任务时不应拿到草药，实际 items=%v", r.Items)
	}
}

// TestHerbDoctorBagFullChangesNothing 事件 1 进行中、背包满：医生说 ItemFullMsg，整笔不做，事件 1 仍进行中。
func TestHerbDoctorBagFullChangesNothing(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1}) // 2417 是仪玉，这里只占格子
	}
	r := kingRecord(full, nil, []int{1})

	blk := kingPick(t, s, r, herbDoctor, 3)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, herbLeaf) != 0 || !r.eventDoing(1) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d 事件1进行中=%v", len(r.Items), r.eventDoing(1))
	}
}

// TestHerbLeafNotRegrantedAfterDone 事件 1 完成后，草药不会再领（医生块 3 的 NOWEV=1 不成立，ENDEV=87 从没有脚本设置，
// 不会走到）；医生只剩闲谈。原版 event01_2 块 3 的 NOWEV=87 / ENDEV=87 分支也是这样，照搬，不会造成重复发奖。
func TestHerbLeafNotRegrantedAfterDone(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(herbCharm), []int{1}, nil)
	kingTalk(t, s, r, herbDoctor, -1)
	if countItem(r.Items, herbLeaf) != 0 || len(r.Items) != 1 {
		t.Fatalf("完成后医生不应再给草药，实际 %v", r.Items)
	}
}

// TestHerbHasLeafOnlyReminds 身上已经有草药：医生只提醒（块 0 的 MESSAGE），不再给第二株。
func TestHerbHasLeafOnlyReminds(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(herbLeaf), nil, []int{1})
	kingTalk(t, s, r, herbDoctor, 0)
	if countItem(r.Items, herbLeaf) != 1 || len(r.Items) != 1 {
		t.Fatalf("身上有草药时不应再给，实际 %v", r.Items)
	}
}

// TestHerbWhitelisted 任务 30 的两个脚本都在白名单里，而且整本支持（没有不支持的块）。
func TestHerbWhitelisted(t *testing.T) {
	for _, file := range []string{"jaruga/event/event01_1", "jaruga/event/event01_2"} {
		if !gamedata.NpcEventEnabled(file) {
			t.Errorf("%s 应已放开", file)
		}
	}
	for _, npc := range []int{herbGirl, herbDoctor} {
		script, ok := gamedata.NpcEventOf(npc)
		if !ok {
			t.Fatalf("NPC %d 没有任务脚本", npc)
		}
		for i := range script.Blocks {
			if u := script.Blocks[i].Unsupported; u != "" {
				t.Errorf("%s 第 %d 块不支持（%s），不该放开", script.File, i, u)
			}
		}
	}
}
