package character

// STAGE-220：精灵王的传说的协议流程（需要数据库）：真实的 talk、npc_choose，断言窗口、推送和数据库。
// 整条链的逻辑在 npcevent_king_test.go（纯逻辑）；这里只挑三段有代表性的：接任务落库、给项链和宠物、宠物栏满。

import (
	"context"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// kingEnv 准备勇者小明站在 floor 上（对话不检查距离，同 adultEnv），setup 在进入世界之前执行（只能写数据库），然后进入世界。
func kingEnv(t *testing.T, floor int, setup ...func(e *env)) (*env, *client) {
	t.Helper()
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", floor, 14, 6); err != nil {
		t.Fatal(err)
	}
	for _, f := range setup {
		f(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// kingBlock 取脚本 npc 的第 idx 块（流程测试里用来取台词，选块由协议层自己做）。
func kingBlock(t *testing.T, npc, idx int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok || idx < 0 || idx >= len(script.Blocks) {
		t.Fatalf("NPC %d 没有第 %d 块", npc, idx)
	}
	return &script.Blocks[idx]
}

// 接任务 69（愿藏的祖父，2000）：窗口问“是否帮忙找新藏”，选是后任务 49 进行中，窗口是感谢话。
func TestKingRequest69FlowDB(t *testing.T) {
	e, c := kingEnv(t, 2000, questStateSetup(t, 1, questDone))
	blk := refundBlock(t, kingGrandpa, 69)

	expectWindow(t, talkTo(c, kingGrandpa), refundHead(blk.Msg.Request), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, refundHead(blk.Msg.Thanks), 0)
	if got := e.questRows(t); got[49] != questDoing {
		t.Fatalf("接了 69，任务 49 应进行中，实际 %v", got)
	}
	if !hasPush(pushes, "quests_changed") {
		t.Fatalf("接任务后应推 quests_changed: %v", pushes)
	}
}

// 给项链和玛蕾菲雅（愿藏的祖母，2000，块 12）：69 已完成、70 进行中，选是后拿到项链 19624 和宠物 1479，
// 任务 50 完成，宠物不打任务送的标记（ACCEPT 不设）。
func TestKingNecklacePetFlowDB(t *testing.T) {
	e, c := kingEnv(t, 2000,
		clearPetsSetup(t),
		questStateSetup(t, 1, questDone),
		questStateSetup(t, 49, questDone),
		questStateSetup(t, 50, questDoing),
	)
	blk := kingBlock(t, kingGrandma, 12)

	expectWindow(t, talkTo(c, kingGrandma), refundHead(blk.Msg.Accept), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, refundHead(blk.Msg.Pages("thanks")[0]), 1) // 感谢话分页，第 1 页只有“下一页”
	if got := e.itemCounts(t); got[kingNecklace] != 1 {
		t.Fatalf("应拿到项链 19624，实际 %v", got)
	}
	if got := e.petRows(t); len(got) != 1 || got[0] {
		t.Fatalf("应有 1 只宠物在 0 号栏位、不打任务标记，实际 %v", got)
	}
	if got := e.questRows(t); got[50] != questDone {
		t.Fatalf("任务 50 应已完成，实际 %v", got)
	}
	for _, typ := range []string{"inventory_changed", "pets_changed", "quests_changed"} {
		if !hasPush(pushes, typ) {
			t.Fatalf("应推 %s: %v", typ, pushes)
		}
	}
}

// fullPetsSetup 把随身宠物栏塞满（STAGE-346 起 20 格），用来测“栏满”的分支。
func fullPetsSetup(t *testing.T) func(*env) {
	t.Helper()
	return func(e *env) {
		for i := 0; i < MaxPetSlots; i++ {
			givePetSetup(t, 560, 10)(e)
		}
	}
}

// 宠物栏满时给玛蕾菲雅（同一块）：整笔不做，说 PetFullMsg（只有一句话，没有选项），项链不给，任务 50 仍进行中。
func TestKingPetFullFlowDB(t *testing.T) {
	e, c := kingEnv(t, 2000,
		clearPetsSetup(t),
		questStateSetup(t, 1, questDone),
		questStateSetup(t, 49, questDone),
		questStateSetup(t, 50, questDoing),
		fullPetsSetup(t),
	)
	blk := kingBlock(t, kingGrandma, 12)

	expectWindow(t, talkTo(c, kingGrandma), refundHead(blk.Msg.Accept), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, refundHead(blk.Msg.PetFull), 0)
	if got := e.itemCounts(t); got[kingNecklace] != 0 {
		t.Fatalf("栏满时不应给项链，实际 %v", got)
	}
	if got := e.petRows(t); len(got) != MaxPetSlots {
		t.Fatalf("栏满时宠物应还是 %d 只，实际 %v", MaxPetSlots, got)
	}
	if got := e.questRows(t); got[50] != questDoing {
		t.Fatalf("栏满时任务 50 应仍进行中，实际 %v", got)
	}
}
