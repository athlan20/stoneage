package character

// 任务脚本的纯逻辑（STAGE-90）：事件位与任务进度的映射、条件、交易的计算。不碰数据库。

import (
	"github.com/athlan20/stoneage/server/battle"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 测试用的空进度。
func newProgress() progress {
	return progress{quests: map[int]int{}, events: map[int]int{}}
}

// 接任务的事件号对应任务 1（成人仪式）：进行中设上，完成后清掉进行中。
func TestApplyEventEditDoingThenDone(t *testing.T) {
	p := newProgress()
	applyEventEdit(p, editSetDoing, 4)
	if p.quests[1] != questDoing {
		t.Fatalf("NowEventSetFlg(4) 后任务 1 应进行中，实际 %v", p.quests)
	}
	endSetFlg(p, 4, []int{4})
	if p.quests[1] != questDone {
		t.Fatalf("EndSetFlg:4 后任务 1 应已完成，实际 %v", p.quests)
	}
}

// 已完成的任务不会被改回进行中（一个状态只有一种）。
func TestApplyEventEditDoingKeepsDone(t *testing.T) {
	p := newProgress()
	p.quests[1] = questDone
	applyEventEdit(p, editSetDoing, 4)
	if p.quests[1] != questDone {
		t.Fatalf("已完成的任务不应被设成进行中，实际 %v", p.quests)
	}
}

// CleanFlg 清掉完成和进行中（可以重做）；事件号 -1 什么都不做。
func TestCleanFlgClearsBothAndIgnoresMinusOne(t *testing.T) {
	p := newProgress()
	p.quests[1] = questDone
	cleanFlg(p, []int{4, -1})
	if _, ok := p.quests[1]; ok {
		t.Fatalf("CleanFlg:4 后任务 1 应被清掉，实际 %v", p.quests)
	}
	p.quests[1] = questDoing
	applyEventEdit(p, editClrDoing, -1)
	if p.quests[1] != questDoing {
		t.Fatalf("事件号 -1 不应改动任何任务，实际 %v", p.quests)
	}
}

// 共享事件号（原版一个事件完成，两个任务一起完成）：两个任务都要变。
func TestSharedEventCompletesBothQuests(t *testing.T) {
	var shared []gamedata.Quest
	counts := map[int]int{}
	for _, q := range gamedata.Quests() {
		counts[q.EndEvent]++
	}
	for _, q := range gamedata.Quests() {
		if counts[q.EndEvent] > 1 {
			shared = append(shared, q)
		}
	}
	if len(shared) < 2 {
		t.Fatalf("任务表里应有共享事件号的任务，实际 %v", shared)
	}
	ev := shared[0].EndEvent
	p := newProgress()
	applyEventEdit(p, editSetDone, ev)
	for _, q := range shared {
		if q.EndEvent == ev && p.quests[q.No] != questDone {
			t.Errorf("事件 %d 完成时任务 %d 应一起完成，实际 %v", ev, q.No, p.quests)
		}
	}
}

// 不对应任务表的事件号（造斧的 19 号，STAGE-94 问题 1）：进度记在事件表里，状态规则和任务一样，
// 不碰任务表；清除后回到没有。
func TestEventWithoutQuestKeepsItsOwnFlag(t *testing.T) {
	if hasQuest(19) {
		t.Fatalf("19 号不应对应任务表里的任务（它只是脚本里的中间步骤）")
	}
	p := newProgress()
	applyEventEdit(p, editSetDoing, 19)
	if p.events[19] != questDoing || len(p.quests) != 0 {
		t.Fatalf("NowEventSetFlg(19) 应记在事件表里，实际 events=%v quests=%v", p.events, p.quests)
	}
	r := &record{EventFlags: stateRecs(p.events)}
	if !r.eventDoing(19) || r.eventDone(19) {
		t.Fatalf("事件 19 应只是进行中，实际 done=%v doing=%v", r.eventDone(19), r.eventDoing(19))
	}
	endSetFlg(p, 19, []int{19})
	if p.events[19] != questDone {
		t.Fatalf("EndSetFlg:19 后事件 19 应已完成，实际 %v", p.events)
	}
	applyEventEdit(p, editSetDoing, 19)
	if p.events[19] != questDone {
		t.Fatalf("已完成的事件不应被设回进行中，实际 %v", p.events)
	}
	cleanFlg(p, []int{19})
	if _, ok := p.events[19]; ok {
		t.Fatalf("CleanFlg:19 后事件 19 应被清掉，实际 %v", p.events)
	}
}

// 条件：ITEM 数背包（不数穿戴），数量按堆叠累计；LV、ENDEV、NOWEV、TIME 各自的比较。
func TestEventTermHolds(t *testing.T) {
	r := &record{
		Level: 10,
		Items: []itemRec{
			{Slot: firstBagSlot, ItemID: 2417, Count: 3},
			{Slot: firstBagSlot + 1, ItemID: 2417, Count: 2},
			{Slot: 0, ItemID: 2417, Count: 9}, // 穿戴的不算
		},
		Quests: []questRec{{No: 1, State: questDoing}, {No: 2, State: questDone}},
	}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
	cases := []struct {
		term gamedata.EventTerm
		want bool
	}{
		{gamedata.EventTerm{Op: "ITEM", Cmp: "=", ID: 2417, N: 5}, true},
		{gamedata.EventTerm{Op: "ITEM", Cmp: "=", ID: 2417, N: 6}, false},
		{gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 2418, N: 1}, true},
		{gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 2417, N: 1}, false},
		{gamedata.EventTerm{Op: "LV", Cmp: ">", N: 0}, true},
		{gamedata.EventTerm{Op: "LV", Cmp: "<", N: 10}, false},
		{gamedata.EventTerm{Op: "ENDEV", Cmp: "=", N: 4}, false},
		{gamedata.EventTerm{Op: "ENDEV", Cmp: "!=", N: 4}, true},
		{gamedata.EventTerm{Op: "NOWEV", Cmp: "=", N: 4}, true},
		{gamedata.EventTerm{Op: "NOWEV", Cmp: "=", N: 3}, false},
	}
	for _, c := range cases {
		if got := eventTermHolds(r, c.term, now); got != c.want {
			t.Errorf("条件 %+v：want %v，got %v", c.term, c.want, got)
		}
	}
	// 任务 2（猜谜3兄弟，事件 44）已完成：事件 44 是完成的，但不是进行中；事件 4 两样都不是。
	if !r.eventDone(44) || r.eventDoing(44) || r.eventDone(4) || !r.eventDoing(4) {
		t.Errorf("事件 44 应只是完成、事件 4 应是进行中（任务 1），实际 done44=%v doing44=%v done4=%v doing4=%v",
			r.eventDone(44), r.eventDoing(44), r.eventDone(4), r.eventDoing(4))
	}
	// TIME!=n：时段 0–3 与游戏时间一一对应，TIME!=当前时段 为假。
	section := gamedata.EventTimeSection(now)
	if eventTermHolds(r, gamedata.EventTerm{Op: "TIME", Cmp: "!=", N: section}, now) {
		t.Errorf("TIME!=当前时段 应为假")
	}
}

// 交易：先删再给。背包满时，删掉 15 个仪玉再给 1 件，结果成立（原版按净额算空位）。
func TestPlanEventRemovesBeforeAdding(t *testing.T) {
	var items []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		items = append(items, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := &record{Items: items, Gold: 0}
	blk := &gamedata.EventBlock{
		DelItem: []gamedata.ItemCount{{ID: 2417, N: 15}},
		GetItem: []gamedata.ItemCount{{ID: 2418, N: 1}},
	}
	plan, fail, ok := planEvent(r, blk, eventSel{group: -1}, func(int) int { return 0 })
	if !ok || fail != "" {
		t.Fatalf("删 15 给 1 应成立，实际 ok=%v fail=%q", ok, fail)
	}
	if len(plan.items) != 1 || plan.items[0].ItemID != 2418 {
		t.Fatalf("结果应只有 2418，实际 %v", plan.items)
	}
}

// 空位不够时整笔不做，返回 ItemFullMsg（不改动输入的背包）。
func TestPlanEventBagFullFailsWithoutChange(t *testing.T) {
	r := &record{Items: []itemRec{{Slot: firstBagSlot, ItemID: 2417, Count: 1}}}
	blk := &gamedata.EventBlock{
		GetItem: []gamedata.ItemCount{{ID: 2417, N: 15}},
		Msg:     gamedata.EventMsg{ItemFull: "空间不够"},
	}
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, nil)
	if ok || fail != "空间不够" {
		t.Fatalf("空间不够时应返回 ItemFullMsg，实际 ok=%v fail=%q", ok, fail)
	}
	if len(r.Items) != 1 || r.Items[0].Count != 1 {
		t.Fatalf("失败时不应改动背包，实际 %v", r.Items)
	}
}

// 金币：DelStone 按等级算，不够扣说 StoneLessMsg；GetStone 到上限说 StoneFullMsg。
func TestPlanEventStones(t *testing.T) {
	r := &record{Level: 20, Gold: 1000}
	cost := &gamedata.StoneCost{PerLv: 60} // 20 × 60 = 1200，不够
	blk := &gamedata.EventBlock{DelStone: cost, Msg: gamedata.EventMsg{StoneLess: "钱不够"}}
	if _, fail, ok := planEvent(r, blk, eventSel{group: -1}, nil); ok || fail != "钱不够" {
		t.Fatalf("金币不够应说 StoneLessMsg，实际 ok=%v fail=%q", ok, fail)
	}
	blk = &gamedata.EventBlock{DelStone: &gamedata.StoneCost{Fixed: 300}, GetStone: 50}
	plan, _, ok := planEvent(r, blk, eventSel{group: -1}, nil)
	if !ok || plan.gold != 750 {
		t.Fatalf("扣 300 加 50 应剩 750，实际 ok=%v gold=%d", ok, plan.gold)
	}
	r.Gold = battle.MaxHaveGold - 10
	blk = &gamedata.EventBlock{GetStone: 100, Msg: gamedata.EventMsg{StoneFull: "装不下"}}
	if _, fail, ok := planEvent(r, blk, eventSel{group: -1}, nil); ok || fail != "装不下" {
		t.Fatalf("金币到上限应说 StoneFullMsg，实际 ok=%v fail=%q", ok, fail)
	}
}

// 随机道具：只给其中一件，用的是传入的随机源。
func TestPlanEventRandomItemUsesRand(t *testing.T) {
	r := &record{}
	blk := &gamedata.EventBlock{GetRand: []int{32, 44, 59}}
	plan, _, ok := planEvent(r, blk, eventSel{group: -1}, func(n int) int {
		if n != 3 {
			t.Errorf("随机源应收到候选数 3，实际 %d", n)
		}
		return 2
	})
	if !ok || len(plan.items) != 1 || plan.items[0].ItemID != 59 {
		t.Fatalf("应随机给第 3 件 59，实际 ok=%v items=%v", ok, plan.items)
	}
}

// 替换 %8d：原版 sprintf 的金币占位，取 DelStone 的费用。
func TestEventTextReplacesCost(t *testing.T) {
	if got := eventText("要付 %8d 金币", 300); got != "要付 300 金币" {
		t.Fatalf("费用占位替换不对：%q", got)
	}
}

// 选块（原版 NPC_TypeCheck）：暗号块普通对话跳过；不支持的块（没被完成或暗号跳过）拒绝对话；已完成的块跳过。
func TestPickEventBlockSkipsKeywordAndDoneRefusesUnsupported(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
	s := &Service{opts: Options{Now: func() time.Time { return now }}}
	anyLevel := [][]gamedata.EventTerm{{{Op: "LV", Cmp: ">", N: 0}}}
	msg := gamedata.EventBlock{Type: "MESSAGE", Event: -1, Cond: anyLevel}

	keyword := gamedata.EventBlock{Type: "ACCEPT", Event: 5, Keyword: "近藏", Cond: anyLevel, Unsupported: "不支持的字段 PetFullMsg"}
	idx, refuse := s.pickEventBlock(&record{Level: 3}, &gamedata.EventScript{Blocks: []gamedata.EventBlock{keyword, msg}}, "")
	if idx != 1 || refuse {
		t.Fatalf("暗号块应跳过、命中后面的块，实际 idx=%d refuse=%v", idx, refuse)
	}

	unsupported := gamedata.EventBlock{Type: "MESSAGE", Event: -1, Cond: anyLevel, Unsupported: "不支持的字段 NpcWarp"}
	idx, refuse = s.pickEventBlock(&record{Level: 3}, &gamedata.EventScript{Blocks: []gamedata.EventBlock{unsupported, msg}}, "")
	if idx != -1 || !refuse {
		t.Fatalf("不支持的块不能让后面的块替它说话，应拒绝，实际 idx=%d refuse=%v", idx, refuse)
	}

	doneUnsupported := gamedata.EventBlock{Type: "MESSAGE", Event: 4, Cond: anyLevel, Unsupported: "不支持的字段 NpcWarp"}
	r := &record{Level: 3, Quests: []questRec{{No: 1, State: questDone}}} // 任务 1 的完成事件是 4
	idx, refuse = s.pickEventBlock(r, &gamedata.EventScript{Blocks: []gamedata.EventBlock{doneUnsupported, msg}}, "")
	if idx != 1 || refuse {
		t.Fatalf("事件已完成的不支持块应被跳过，实际 idx=%d refuse=%v", idx, refuse)
	}
}

// 暗号（STAGE-92，原版 npc_exchangeman.c 的 strcmp）：只有说出的话逐字相同的暗号块才考虑（区分大小写、
// 不去空格），匹配了暗号但条件不满足就接着看后面的块；普通块不看说的话，照常按条件选。
func TestPickEventBlockKeywordMustMatchExactly(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
	s := &Service{opts: Options{Now: func() time.Time { return now }}}
	anyLevel := [][]gamedata.EventTerm{{{Op: "LV", Cmp: ">", N: 0}}}
	noLevel := [][]gamedata.EventTerm{{{Op: "LV", Cmp: ">", N: 99}}}
	msg := gamedata.EventBlock{Type: "MESSAGE", Event: -1, Cond: anyLevel}
	kw := gamedata.EventBlock{Type: "MESSAGE", Event: -1, Keyword: "Ab", Cond: anyLevel}
	ev := &gamedata.EventScript{Blocks: []gamedata.EventBlock{kw, msg}}
	r := &record{Level: 3}

	for _, c := range []struct {
		said string
		want int
	}{{"", 1}, {"ab", 1}, {"Ab ", 1}, {" Ab", 1}, {"Ab", 0}} {
		if idx, refuse := s.pickEventBlock(r, ev, c.said); idx != c.want || refuse {
			t.Errorf("说 %q：应命中 %d，实际 idx=%d refuse=%v", c.said, c.want, idx, refuse)
		}
	}

	failing := gamedata.EventBlock{Type: "MESSAGE", Event: -1, Keyword: "Ab", Cond: noLevel}
	ev = &gamedata.EventScript{Blocks: []gamedata.EventBlock{failing, msg}}
	if idx, _ := s.pickEventBlock(r, ev, "Ab"); idx != 1 {
		t.Errorf("暗号对了但条件不满足，应接着看后面的块，实际 idx=%d", idx)
	}

	done := gamedata.EventBlock{Type: "MESSAGE", Event: 4, Keyword: "Ab", Cond: anyLevel}
	ev = &gamedata.EventScript{Blocks: []gamedata.EventBlock{done, msg}}
	rDone := &record{Level: 3, Quests: []questRec{{No: 1, State: questDone}}} // 任务 1 的完成事件是 4
	if idx, _ := s.pickEventBlock(rDone, ev, "Ab"); idx != 1 {
		t.Errorf("事件已完成的暗号块不再考虑，实际 idx=%d", idx)
	}
}

// 有没有还没完成的暗号块决定闲聊窗口上要不要给“说暗号”（没有暗号块的 NPC 不出现这个选项）。
func TestHasPendingKeyword(t *testing.T) {
	anyLevel := [][]gamedata.EventTerm{{{Op: "LV", Cmp: ">", N: 0}}}
	plain := &gamedata.EventScript{Blocks: []gamedata.EventBlock{{Type: "MESSAGE", Event: -1, Cond: anyLevel}}}
	if hasPendingKeyword(&record{}, plain) {
		t.Errorf("没有暗号块的脚本不应该有待说的暗号")
	}
	kw := &gamedata.EventScript{Blocks: []gamedata.EventBlock{{Type: "MESSAGE", Event: 4, Keyword: "Ab", Cond: anyLevel}}}
	if !hasPendingKeyword(&record{}, kw) {
		t.Errorf("事件没完成的暗号块应该给“说暗号”")
	}
	done := &record{Quests: []questRec{{No: 1, State: questDone}}} // 任务 1 的完成事件是 4
	if hasPendingKeyword(done, kw) {
		t.Errorf("事件已完成的暗号块不应该再给“说暗号”")
	}
}
