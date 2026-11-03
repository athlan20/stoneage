package character

// 任务 NPC 的撤销（STAGE-94，npcevent_refund.go）。用 20401 图的「乌力斯凯」（脚本 event07_1）：接任务给 2425，
// 触发条件 ITEM=2414 不会被收走，放弃后这个块仍然命中。退款记录由 setup 直接写库（接任务时收走东西的写法见
// TestPlanEventRecordsTakenItems）。编号和台词都从数据里取，不写死。

import (
	"context"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	refundFloor    = 20401 // 乌力斯凯所在的地图
	refundGiveItem = 2425  // event07_1 接任务时给的道具（放弃时收回）
	refundCondItem = 2414  // event07_1 的触发条件（背包里要有，接任务和放弃都不收走）
	refundEvent    = 7     // event07_1 的事件号（进行中事件）
	refundQuest    = 24    // 任务表里进行中事件是 7 的任务「哥亚山洞窟 把花和信给乌力美」（进度按任务编号存）
)

// refundEnv 准备勇者小明站在乌力斯凯旁边，setup 在进入世界之前执行（只能写数据库），然后进入世界。
func refundEnv(t *testing.T, setup ...func(e *env)) (*env, *client, int) {
	t.Helper()
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	npc := npcNamed(t, refundFloor, "乌力斯凯")
	info, ok := gamedata.NPCByID(npc)
	if !ok {
		t.Fatalf("NPC %d 不在表里", npc)
	}
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", refundFloor, info.X, info.Y); err != nil {
		t.Fatal(err)
	}
	for _, f := range setup {
		f(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c, npc
}

// refundBlock 取脚本里事件号为 ev 的 REQUEST 块（测试用它的台词）。
func refundBlock(t *testing.T, npc, ev int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	for i := range script.Blocks {
		if b := &script.Blocks[i]; b.Type == "REQUEST" && b.Event == ev {
			return b
		}
	}
	t.Fatalf("脚本里没有事件 %d 的 REQUEST 块", ev)
	return nil
}

// refundHead 取台词的第一行（窗口正文按行显示，测试只比对开头）。
func refundHead(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// refundSetup 写一条退款记录（setup 用）：事件号 ev 上接任务时收走的道具和石币。
func refundSetup(t *testing.T, ev int, items string, gold int) func(*env) {
	return func(e *env) {
		e.exec(t, `INSERT INTO character_event_refunds (character_id, event_no, items, gold)
			VALUES ((SELECT id FROM characters WHERE name = '勇者小明'), $1, $2::jsonb, $3)`, ev, items, gold)
	}
}

// questDoingNo 让角色已经接了任务编号 no（setup 用）。
func questDoingNo(t *testing.T, no int) func(*env) {
	return func(e *env) {
		e.exec(t, `INSERT INTO character_quests (character_id, quest_no, state)
			VALUES ((SELECT id FROM characters WHERE name = '勇者小明'), $1, $2)`, no, questDoing)
	}
}

// goldSetup 设角色的石币（setup 用）。
func goldSetup(t *testing.T, gold int) func(*env) {
	return func(e *env) {
		e.exec(t, `UPDATE characters SET gold = $1 WHERE name = '勇者小明'`, gold)
	}
}

// pushedGold 从推送里取 status_changed 的石币（石币只在内存里改，离开世界时才写库，见 docs/quest.md）。
func pushedGold(pushes []map[string]any) (int64, bool) {
	for _, p := range pushes {
		if p["type"] != "status_changed" {
			continue
		}
		status := p["data"].(map[string]any)["status"].(map[string]any)
		return int64(status["gold"].(float64)), true
	}
	return 0, false
}

// refundRows 数数据库里事件 ev 的退款记录行数。
func (e *env) refundRows(t *testing.T, ev int) int {
	t.Helper()
	var n int
	if err := e.chars.db.QueryRow(context.Background(), `SELECT count(*)::int FROM character_event_refunds cer
		JOIN characters c ON c.id = cer.character_id WHERE c.name = '勇者小明' AND cer.event_no = $1`, ev).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// 放弃（StopMsg 选是）：收回接任务时给的 2425，退回退款记录里的 2466 两件和 300 石币，任务清掉，记录删掉。
func TestEventGiveUpRecallsGivenItemAndRefunds(t *testing.T) {
	e, c, npc := refundEnv(t,
		putItemSetup(t, 5, refundCondItem, 1),
		putItemSetup(t, 6, refundGiveItem, 1),
		questDoingNo(t, refundQuest),
		refundSetup(t, refundEvent, `[{"id":2466,"n":2}]`, 300),
		goldSetup(t, 1000),
	)
	blk := refundBlock(t, npc, refundEvent)

	expectWindow(t, talkTo(c, npc), refundHead(blk.Msg.Stop), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, refundHead(blk.Msg.EndStop), 0)

	got := e.itemCounts(t)
	if got[refundGiveItem] != 0 || got[refundCondItem] != 1 || got[2466] != 2 {
		t.Fatalf("应收回 2425、保留 2414、退回 2466 两件，实际 %v", got)
	}
	if qs := e.questRows(t); qs[refundQuest] != 0 {
		t.Fatalf("放弃后任务 %d 应被清掉，实际 %v", refundQuest, qs)
	}
	if n := e.refundRows(t, refundEvent); n != 0 {
		t.Fatalf("放弃后退款记录应删掉，还剩 %d 行", n)
	}
	if gold, ok := pushedGold(pushes); !ok || gold != 1300 {
		t.Fatalf("石币应退回 300，推送的 status_changed 应是 1300，实际 %d（有推送 %v）", gold, ok)
	}
	if !hasPush(pushes, "inventory_changed") || !hasPush(pushes, "quests_changed") {
		t.Fatalf("放弃后应推 inventory_changed 和 quests_changed: %v", pushes)
	}
}

// 背包放不下退回的道具：放弃不成立（原版 NPC_ItemFullCheck），什么都不动，任务和退款记录都还在。
// 背包 15 格全占（收回 2425 后只空出 1 格，要退 2 件）。
func TestEventGiveUpWaitsForBagRoom(t *testing.T) {
	setup := []func(*env){
		putItemSetup(t, 5, refundCondItem, 1),
		putItemSetup(t, 6, refundGiveItem, 1),
	}
	for slot := 7; slot <= MaxItemSlot; slot++ {
		setup = append(setup, putItemSetup(t, slot, refundCondItem, 1))
	}
	setup = append(setup,
		questDoingNo(t, refundQuest),
		refundSetup(t, refundEvent, `[{"id":2466,"n":2}]`, 300),
		goldSetup(t, 1000),
	)
	e, c, npc := refundEnv(t, setup...)
	blk := refundBlock(t, npc, refundEvent)

	expectWindow(t, talkTo(c, npc), refundHead(blk.Msg.Stop), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	if hasPush(pushes, "quests_changed") || hasPush(pushes, "inventory_changed") || hasPush(pushes, "status_changed") {
		t.Fatalf("背包放不下时不应有任何变化: %v", pushes)
	}
	if got := e.itemCounts(t); got[refundGiveItem] != 1 || got[2466] != 0 {
		t.Fatalf("背包不应变化（仍有 2425、没有 2466），实际 %v", got)
	}
	if qs := e.questRows(t); qs[refundQuest] != questDoing {
		t.Fatalf("放弃不成立，任务 %d 应仍是进行中，实际 %v", refundQuest, qs)
	}
	if n := e.refundRows(t, refundEvent); n != 1 {
		t.Fatalf("放弃不成立，退款记录应保留，实际 %d 行", n)
	}
}

// 退款记录的写入、读取和删除（接任务时的写法）：写进事务、读回来一致，空的记录删掉这一行。
func TestRefundRecordRoundTrip(t *testing.T) {
	e, _, _ := refundEnv(t)
	ctx := context.Background()
	var id int64
	if err := e.chars.db.QueryRow(ctx, `SELECT id FROM characters WHERE name = '勇者小明'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	write := func(w *refundWrite) {
		t.Helper()
		tx, err := e.chars.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := writeRefundDiff(ctx, tx, id, w); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want := eventRefund{Items: []refundItem{{ID: 2486, N: 2}, {ID: 2487, N: 1}}, Gold: 50}
	write(&refundWrite{Event: 20, eventRefund: want})
	got, err := e.chars.loadRefund(ctx, id, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Gold != 50 || len(got.Items) != 2 || got.Items[0] != want.Items[0] || got.Items[1] != want.Items[1] {
		t.Fatalf("读回的退款应和写入一致，实际 %+v", got)
	}
	write(&refundWrite{Event: 20}) // 道具和石币都空：删掉这一行
	if got, err := e.chars.loadRefund(ctx, id, 20); err != nil || got.Gold != 0 || len(got.Items) != 0 {
		t.Fatalf("删除后应读到空的退款，实际 %+v，错误 %v", got, err)
	}
	if n := e.refundRows(t, 20); n != 0 {
		t.Fatalf("删除后不应还有记录，实际 %d 行", n)
	}
}

// planEvent 记下 DelItem 实际删掉的件数（DelItem 不写数量时删光，按删前删后的差算）和 DelStone 扣的石币。
func TestPlanEventRecordsTakenItems(t *testing.T) {
	r := &record{
		Items: []itemRec{{Slot: 5, ItemID: 2486, Count: 1}, {Slot: 6, ItemID: 2486, Count: 1}, {Slot: 7, ItemID: 9, Count: 1}},
		Gold:  500,
		Level: 10,
	}
	blk := &gamedata.EventBlock{
		Type:     "REQUEST",
		Event:    20,
		DelItem:  []gamedata.ItemCount{{ID: 2486, N: 0}, {ID: 2487, N: 0}},
		DelStone: &gamedata.StoneCost{Fixed: 50},
	}
	plan, fail, ok := planEvent(r, blk, eventSel{group: -1}, nil)
	if !ok {
		t.Fatalf("交易应成立，失败台词 %q", fail)
	}
	if len(plan.taken) != 1 || plan.taken[0] != (refundItem{ID: 2486, N: 2}) {
		t.Fatalf("应记下收走 2486 两件（2487 没有，不记），实际 %v", plan.taken)
	}
	if plan.takenGold != 50 || plan.gold != 450 {
		t.Fatalf("应收走 50 石币、剩 450，实际 takenGold=%d gold=%d", plan.takenGold, plan.gold)
	}
	if len(plan.items) != 1 || plan.items[0].ItemID != 9 {
		t.Fatalf("背包应只剩 9，实际 %v", plan.items)
	}
}
