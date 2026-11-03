package character

// STAGE-336：炎之圣石的端到端（纯逻辑，不连数据库，样板见 npcevent_poru_test.go）。
// 原版链：龙王的守护 18544（poru/boy04，任务 40 完成后给两颗）→ 埋宝箱的 poru/mary13（1319，任务 41 进行中）
// 换挖掘物 18561（一颗守护换一个）/ 18562（两颗守护换一个）→ 考古队员 poru/mary12（1329）换炎之圣石 18558
// （18561 换 1 颗，18562 换 2 颗）。任务 41 的完成块在 poru/mary01（未放开），只要 1 颗 18558。
// 正向：从龙王拿守护到圣石全程；反向：条件不满足、背包满、任务已完成再来、事件 64 之外。
// 龙王的两颗守护只给一次（有意偏离原版，见 tools/npc_event_overlay.py 的 DRAGON_PATCHES 和 TestPoruFireStoneDragonOnce）。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	poruBurier     = 1319  // 埋宝箱的（poru/mary13）：任务 41 进行中，守护换挖掘物
	poruArchaeo    = 1329  // 考古队员（poru/mary12）：挖掘物换炎之圣石
	poruDigA       = 18561 // 挖掘物 A：一颗守护换的，换 1 颗圣石
	poruDigB       = 18562 // 挖掘物 B：两颗守护换的，换 2 颗圣石
	poruFireSt     = 18558 // 炎之圣石（任务 41 只要 1 颗，mary01 的 DelItem:18558*1）
	poruFiller     = 2417  // 仪玉，只用来占背包格子
	poruDragonOnce = 186   // 龙王守护只给一次的事件（overlay 的 DRAGON_EVENT，任务表里没有它）
)

// poruFireCycle 走一圈：龙王没有守护就给两颗 → 埋宝箱的换挖掘物 B → 考古队员换两颗圣石。
// 每一步都经过真实白名单和真实选块，并断言这一步命中的块与交易结果。
func poruFireCycle(t *testing.T, s *Service, r *record) {
	t.Helper()
	before := countItem(r.Items, poruFireSt)
	kingRun(t, r, poruPick(t, s, r, poruDragon, 0))
	if countItem(r.Items, poruGuard) != 2 {
		t.Fatalf("龙王应给两颗守护，实际 %d", countItem(r.Items, poruGuard))
	}
	kingRun(t, r, poruPick(t, s, r, poruBurier, 2))
	if countItem(r.Items, poruGuard) != 0 || countItem(r.Items, poruDigB) != 1 {
		t.Fatalf("两颗守护应换一个挖掘物 B，实际守护 %d、挖掘物 B %d", countItem(r.Items, poruGuard), countItem(r.Items, poruDigB))
	}
	kingRun(t, r, poruPick(t, s, r, poruArchaeo, 0))
	if countItem(r.Items, poruDigB) != 0 || countItem(r.Items, poruFireSt) != before+2 {
		t.Fatalf("挖掘物 B 应换两颗圣石，实际挖掘物 B %d、圣石 %d（之前 %d）",
			countItem(r.Items, poruDigB), countItem(r.Items, poruFireSt), before)
	}
}

// 正向（手上已有两颗守护）：埋宝箱的换挖掘物 B（事件 64 不结束），考古队员换两颗圣石，任务 41 仍进行中。
func TestPoruFireStoneFullChain(t *testing.T) {
	s := kingService()
	r := kingRecord([]itemRec{{Slot: firstBagSlot, ItemID: poruGuard, Count: 2}}, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruBurier, 2))
	if countItem(r.Items, poruDigB) != 1 || countItem(r.Items, poruGuard) != 0 {
		t.Fatalf("两颗守护应换挖掘物 B，实际 %v", r.Items)
	}
	kingRun(t, r, poruPick(t, s, r, poruArchaeo, 0))
	if countItem(r.Items, poruFireSt) != 2 || countItem(r.Items, poruDigB) != 0 {
		t.Fatalf("挖掘物 B 应换两颗炎之圣石，实际圣石 %d、挖掘物 B %d", countItem(r.Items, poruFireSt), countItem(r.Items, poruDigB))
	}
	if !r.eventDoing(poruQ41) {
		t.Fatalf("换圣石后任务 41 应仍进行中（完成块在 poru/mary01，交付见 STAGE-339），实际 %v", r.EventFlags)
	}
}

// 正向（一颗守护）：埋宝箱的换挖掘物 A（第 1 块，不是两颗的第 2 块），考古队员换一颗圣石。
func TestPoruFireStoneOneGuard(t *testing.T) {
	s := kingService()
	r := kingRecord([]itemRec{{Slot: firstBagSlot, ItemID: poruGuard, Count: 1}}, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruBurier, 3))
	if countItem(r.Items, poruDigA) != 1 || countItem(r.Items, poruGuard) != 0 {
		t.Fatalf("一颗守护应换挖掘物 A，实际 %v", r.Items)
	}
	kingRun(t, r, poruPick(t, s, r, poruArchaeo, 1))
	if countItem(r.Items, poruFireSt) != 1 || countItem(r.Items, poruDigA) != 0 {
		t.Fatalf("挖掘物 A 应换一颗炎之圣石，实际圣石 %d、挖掘物 A %d", countItem(r.Items, poruFireSt), countItem(r.Items, poruDigA))
	}
}

// 正向（从龙王开始）：任务 40 完成后龙王给两颗守护，走完一圈得两颗圣石。
func TestPoruFireStoneFromDragon(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, []int{poruQ41})
	poruFireCycle(t, s, r)
	if countItem(r.Items, poruFireSt) != 2 {
		t.Fatalf("从龙王拿两颗守护应得两颗圣石，实际 %d", countItem(r.Items, poruFireSt))
	}
}

// 只给一次（有意偏离原版）：第一圈从龙王拿到两颗，事件 186 设完成。之后换掉守护、手上再没有，龙王也只闲聊，
// 圣石停在两颗；任务 41 仍进行中（mary13、mary12 没有 EndSetFlg）。
func TestPoruFireStoneDragonOnce(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, []int{poruQ41})
	poruFireCycle(t, s, r)
	if countItem(r.Items, poruFireSt) != 2 || !r.eventDone(poruDragonOnce) {
		t.Fatalf("第一圈后应有两颗圣石且事件 186 已完成，实际圣石 %d、186 完成 %v", countItem(r.Items, poruFireSt), r.eventDone(poruDragonOnce))
	}
	for i := 0; i < 3; i++ {
		if blk := poruPick(t, s, r, poruDragon, -1); blk != nil {
			t.Fatalf("龙王已给过守护，不应再命中块，实际 %s", blk.Type)
		}
	}
	if countItem(r.Items, poruGuard) != 0 || countItem(r.Items, poruFireSt) != 2 {
		t.Fatalf("龙王只给一次，实际守护 %d、圣石 %d", countItem(r.Items, poruGuard), countItem(r.Items, poruFireSt))
	}
	if !r.eventDoing(poruQ41) {
		t.Fatalf("任务 41 应仍进行中，实际 %v", r.EventFlags)
	}
}

// 丢了守护也不补：第一次拿到两颗守护后守护没了（丢、卖、被别的任务收走），龙王不再给（原版会再给两颗，见 docs/quest.md）。
func TestPoruFireStoneLostGuardNotRefilled(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruDragon, 0))
	r.Items = nil
	if blk := poruPick(t, s, r, poruDragon, -1); blk != nil {
		t.Fatalf("守护丢了也不应再给，实际命中 %s", blk.Type)
	}
	if len(r.Items) != 0 {
		t.Fatalf("不应有任何补发，实际 %v", r.Items)
	}
}

// 反向：任务 41 没接（事件 64 既不进行也没完成）时，埋宝箱的不收守护，只闲谈；守护原样保留。
func TestPoruFireStoneNeedsQuest41(t *testing.T) {
	s := kingService()
	r := kingRecord([]itemRec{{Slot: firstBagSlot, ItemID: poruGuard, Count: 2}}, []int{poruQ40}, nil)
	if blk := poruPick(t, s, r, poruBurier, -1); blk != nil {
		t.Fatalf("没接任务 41 时不应命中块，实际 %s", blk.Type)
	}
	if countItem(r.Items, poruGuard) != 2 || countItem(r.Items, poruDigB) != 0 {
		t.Fatalf("没接任务 41 不应换东西，实际 %v", r.Items)
	}
}

// 反向：任务 41 已完成（ENDEV=64）再来，埋宝箱的只说一句台词（第 0 块），守护不换。
func TestPoruFireStoneDoneQuestTalksOnly(t *testing.T) {
	s := kingService()
	r := kingRecord([]itemRec{{Slot: firstBagSlot, ItemID: poruGuard, Count: 2}}, []int{poruQ40, poruQ41}, nil)
	kingRun(t, r, poruPick(t, s, r, poruBurier, 0))
	if countItem(r.Items, poruGuard) != 2 || countItem(r.Items, poruDigB) != 0 || countItem(r.Items, poruDigA) != 0 {
		t.Fatalf("任务 41 已完成时不应换东西，实际 %v", r.Items)
	}
}

// 反向：任务 41 进行中但手上没有守护，埋宝箱的只说话（第 4 块）。
func TestPoruFireStoneNoGuardTalksOnly(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruBurier, 4))
	if len(r.Items) != 0 {
		t.Fatalf("没有守护时不应得到东西，实际 %v", r.Items)
	}
}

// 反向：手上已有挖掘物 A 时，埋宝箱的先命中“只有你的钥匙才可以打开”的块（第 1 块），守护不换，
// 要先去考古队员那里把挖掘物换掉，才会换下一批（原版块顺序如此）。
func TestPoruFireStoneDigHoldsBlocksExchange(t *testing.T) {
	s := kingService()
	r := kingRecord([]itemRec{
		{Slot: firstBagSlot, ItemID: poruGuard, Count: 2},
		{Slot: firstBagSlot + 1, ItemID: poruDigA, Count: 1},
	}, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruBurier, 1))
	if countItem(r.Items, poruGuard) != 2 || countItem(r.Items, poruDigA) != 1 {
		t.Fatalf("手上有挖掘物 A 时不应换守护，实际 %v", r.Items)
	}
}

// 反向：背包满时龙王给不了两颗守护（ItemFullMsg），整笔不做，任务 40 的状态不变。
func TestPoruFireStoneFullBagDragon(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: poruFiller, Count: 1})
	}
	r := kingRecord(full, []int{poruQ40}, []int{poruQ41})
	blk := poruPick(t, s, r, poruDragon, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("背包满时龙王应说 ItemFullMsg 且不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, poruGuard) != 0 || len(r.Items) != len(full) {
		t.Fatalf("背包满失败后不应有任何改动，实际 %v", r.Items)
	}
}

// 背包满但守护在包里：埋宝箱的先删两颗守护再给挖掘物（planEvent 先删后给，同原版），满包也能换。
func TestPoruFireStoneFullBagStillExchanges(t *testing.T) {
	s := kingService()
	items := []itemRec{{Slot: firstBagSlot, ItemID: poruGuard, Count: 2}}
	for slot := firstBagSlot + 1; slot <= MaxItemSlot; slot++ {
		items = append(items, itemRec{Slot: slot, ItemID: poruFiller, Count: 1})
	}
	r := kingRecord(items, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruBurier, 2))
	if countItem(r.Items, poruDigB) != 1 || countItem(r.Items, poruGuard) != 0 || len(r.Items) != len(items) {
		t.Fatalf("满包时守护换挖掘物应成立且格子数不变，实际 %v", r.Items)
	}
}

// 反向：任务 41 已完成后，手上还有挖掘物 B 的，考古队员照样换圣石（mary12 不看事件 64，原版如此）。
// 挖掘物只能由守护换来，守护又有数量，所以这里能换的也是有限的；记在这里，改之前先看 docs/quest.md。
func TestPoruFireStoneArchaeoIgnoresQuest(t *testing.T) {
	s := kingService()
	r := kingRecord([]itemRec{{Slot: firstBagSlot, ItemID: poruDigB, Count: 1}}, []int{poruQ40, poruQ41}, nil)
	kingRun(t, r, poruPick(t, s, r, poruArchaeo, 0))
	if countItem(r.Items, poruFireSt) != 2 || countItem(r.Items, poruDigB) != 0 {
		t.Fatalf("挖掘物 B 应照样换两颗圣石，实际 %v", r.Items)
	}
}

// 水源线（poru/mary11 给 18560，mary07/08/09 收 18560 换石币）不在炎之圣石的路线上：
// 18560 只有这四个脚本用到，任务 41 和 42/44 的链也都不碰它。四个都不放。
// 它们也有刷钱问题（mary11 DelStone:50 换 18560、mary07 换 100 石，没有任务条件），放开前要单独设计。
func TestPoruWaterLineStaysClosed(t *testing.T) {
	for _, file := range []string{"poru/mary11", "poru/mary07", "poru/mary08", "poru/mary09"} {
		if gamedata.NpcEventEnabled(file) {
			t.Fatalf("%s 不属于炎之圣石路线，不应放开", file)
		}
	}
}
