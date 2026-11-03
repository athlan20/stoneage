package character

// STAGE-324：强恩洞窟的任务 27、28、29（事件 20、21、22）端到端（纯逻辑，不连数据库）：用真实白名单、
// 真实选块 pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具增减、事件位变化。
// 顺带验证块 4 的 2745（原版道具表里也没有）按原版处理：块能命中，但整块交易失败、什么都不改。
// 反向用例覆盖顺序不对、道具不齐、背包满、完成后再来。协议层（对话窗口、推送）不在这里测。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	jingKai  = 1288 // 吉德的朋友凯伊（jaruga/event/event20_1，3400）：给发饰 2486
	jingGide = 1291 // 吉德（jaruga/event/event20_2，3406）：任务 27 接取、任务 28 交信（STAGE-324 放开）
	jingSon  = 1292 // 吉德的儿子（jaruga/event/event20_3，3406）：给发饰 2487
	jingXion = 1389 // 强恩（jaruga/event/event20_4，20607）：任务 27 完成、任务 29 给随机奖励
	jingWife = 1390 // 强恩的妻子（jaruga/event/event20_5，20607）：只说话
	jingSara = 1391 // 沙拉（jaruga/event/event20_6，20608）：任务 28 接取、给信 2488

	jingQ27    = 20   // 任务 27（找到强恩）的完成事件
	jingQ28    = 21   // 任务 28（找到吉德的女儿）的完成事件
	jingQ29    = 22   // 任务 29（把家书给吉德）的完成事件
	jingFlw    = 2486 // 凯伊给的母亲的遗物发饰
	jingSonF   = 2487 // 儿子给的发饰
	jingLetter = 2488 // 沙拉写的信
	jingFlute  = 2489 // 吉德给的木笛（交信的奖励）
	jingRand   = 2490 // 强恩给的随机奖励里 kingRand 取到的第一件
	jingEye    = 2744 // 块 4 的条件道具（原版道具表里没有）
	jingEye2   = 2745 // 块 4 的奖励道具（原版道具表里也没有）
)

// jingPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func jingPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Fatalf("%s 不在白名单里，NPC %d 对话会说“还不能对话”", script.File, npc)
	}
	return kingPick(t, s, r, npc, want)
}

// 任务 27、28、29 正向：凯伊给发饰 → 吉德接任务 27 → 儿子给第二个发饰 → 强恩完成 27 →
// 沙拉接任务 28、强恩完成 29 → 沙拉给信 → 吉德交信完成 28。
func TestJingQuestsWalk(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	kingRun(t, r, jingPick(t, s, r, jingKai, 2))
	if countItem(r.Items, jingFlw) != 1 {
		t.Fatalf("凯伊应给发饰 2486，实际 %v", r.Items)
	}

	blk := jingPick(t, s, r, jingGide, 1)
	if blk.Type != "REQUEST" || blk.Event != jingQ27 {
		t.Fatalf("吉德应接任务 27（REQUEST 20），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, jingFlw) != 0 || !r.eventDoing(jingQ27) {
		t.Fatalf("接任务应收走 2486、事件 20 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, jingPick(t, s, r, jingSon, 0))
	if countItem(r.Items, jingSonF) != 1 {
		t.Fatalf("儿子应给发饰 2487，实际 %v", r.Items)
	}
	kingRun(t, r, jingPick(t, s, r, jingXion, 0))
	if !r.eventDone(jingQ27) {
		t.Fatalf("强恩应完成事件 20（任务 27），实际 %v", r.EventFlags)
	}

	blk = jingPick(t, s, r, jingSara, 2)
	if blk.Type != "REQUEST" || blk.Event != jingQ28 {
		t.Fatalf("沙拉应接任务 28（REQUEST 21），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, jingSonF) != 0 || !r.eventDoing(jingQ28) {
		t.Fatalf("接任务应收走 2487、事件 21 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, jingPick(t, s, r, jingXion, 1))
	if countItem(r.Items, jingRand) != 1 || !r.eventDone(jingQ29) {
		t.Fatalf("强恩应完成事件 22（任务 29）并给随机奖励，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, jingPick(t, s, r, jingSara, 0))
	if countItem(r.Items, jingLetter) != 1 {
		t.Fatalf("沙拉应给信 2488，实际 %v", r.Items)
	}
	kingRun(t, r, jingPick(t, s, r, jingGide, 0))
	if countItem(r.Items, jingLetter) != 0 || countItem(r.Items, jingFlute) != 1 || !r.eventDone(jingQ28) {
		t.Fatalf("吉德应交信给木笛、完成事件 21，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	jingPick(t, s, r, jingWife, 2) // 老婆在任务完成后只说闲话
}

// 反向：没有发饰时吉德接不了任务 27；任务进行中再找他只会问是否放弃，不会重复收东西。
func TestJingQuest27NeedsFlower(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	jingPick(t, s, r, jingGide, -1)
	if r.eventDoing(jingQ27) {
		t.Fatalf("没有 2486 不应接到任务 27，实际 %v", r.EventFlags)
	}

	r = kingRecord(kingItems(jingFlw), nil, []int{jingQ27}) // 进行中又拿到一枚发饰
	blk := jingPick(t, s, r, jingGide, 1)
	if blk.Type != "REQUEST" || blk.Event != jingQ27 {
		t.Fatalf("进行中再找吉德应命中 REQUEST（放弃问话），实际 %s 事件 %d", blk.Type, blk.Event)
	}
}

// 反向：还没完成事件 22 时沙拉不给信（块 0 要 ENDEV=22），只命中提示块；先交信也交不了。
func TestJingQuest28Order(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{jingQ27}, []int{jingQ28})
	blk := jingPick(t, s, r, jingSara, 3)
	if blk.Type != "MESSAGE" || blk.Event != -1 {
		t.Fatalf("事件 22 未完成时沙拉只提示（MESSAGE），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, jingLetter) != 0 {
		t.Fatalf("事件 22 未完成不应给信，实际 %v", r.Items)
	}
}

// 反向：背包满时凯伊给不了发饰，台词是 ItemFullMsg，事件和背包都不变。
func TestJingFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, nil)
	blk := jingPick(t, s, r, jingKai, 2)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, jingFlw) != 0 || len(r.Items) != len(full) {
		t.Fatalf("失败后不应给发饰，实际 %v", r.Items)
	}
}

// 块 4（心眼支线）的条件道具 2744 和奖励 2745 在原版道具表里都没有：块能命中，但整块交易失败，
// 什么都不改（对照 NPC_EventAddItem 的 ITEM_makeItemAndRegist 返回 -1）。这就是 STAGE-324 的修法。
func TestJingMissingItemBlockFails(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{jingQ27, jingQ28, 160}, nil) // 事件 160 完成，跳过块 2
	r.Level = 80
	r.Pets = nil
	// 2744 无法从正常途径获得，测试直接放进背包；原版背包里也不可能有，这里只验证块的处理。
	r.Items = append(r.Items, itemRec{Slot: firstBagSlot, ItemID: jingEye, Count: 1})
	r.EventFlags = append(r.EventFlags, questRec{No: 161, State: questDoing}) // NOWEV=161：转前事件（不对应任务表）
	blk := jingPick(t, s, r, jingGide, 4)
	if !blk.MissingItem || len(blk.GetItem) != 1 || blk.GetItem[0].ID != jingEye2 {
		t.Fatalf("块 4 应标 missing_item 且引用 2745，实际 %+v", blk)
	}
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != "" {
		t.Fatalf("缺道具的块应交易失败且不说话，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, jingEye2) != 0 || countItem(r.Items, jingEye) != 1 {
		t.Fatalf("失败后不应给 2745、也不应收走 2744，实际 %v", r.Items)
	}
}

// 数据检查：event20_2 在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 27、28、29 的完成事件对得上；
// 块 4 的 2744/2745 原版道具表里也没有（标 missing_item），不是转换错误。
func TestJingScriptData(t *testing.T) {
	script, ok := gamedata.NpcEventOf(jingGide)
	if !ok || script.File != "jaruga/event/event20_2" {
		t.Fatalf("NPC %d 应对应 jaruga/event/event20_2，实际 %+v ok=%v", jingGide, script, ok)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Errorf("%s 应已放开", script.File)
	}
	npc, ok := gamedata.NPCByID(jingGide)
	if !ok || npc.Kind != "changeevent" || !gamedata.FormalFloor(npc.Floor) {
		t.Errorf("NPC %d 应是任务类型、在正式地图上，实际 %+v", jingGide, npc)
	}
	missing := map[int]bool{jingEye: true, jingEye2: true} // 原版道具表里也没有
	for i := range script.Blocks {
		b := &script.Blocks[i]
		if b.Unsupported != "" {
			t.Errorf("%s 块 %d 不支持：%s", script.File, i, b.Unsupported)
		}
		for _, g := range b.Cond {
			for _, term := range g {
				if term.Op == "ITEM" && !missing[term.ID] {
					checkKingItem(t, script.File, term.ID)
				}
			}
		}
		for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
			if !missing[it.ID] {
				checkKingItem(t, script.File, it.ID)
			}
		}
	}
	for no, event := range map[int]int{27: jingQ27, 28: jingQ28, 29: jingQ29} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	// 2744/2745 原版也没有：块 4 标 missing_item 而不是 unsupported。
	for _, id := range []int{jingEye, jingEye2} {
		if _, ok := gamedata.ItemByID(id); ok {
			t.Errorf("道具 %d 应在原版道具表里不存在（测试假设已过时）", id)
		}
	}
	if !script.Blocks[4].MissingItem {
		t.Errorf("块 4 应标 missing_item（GetItem 2745 不在道具表）")
	}
	for _, id := range []int{jingFlw, jingSonF, jingLetter, jingFlute, jingRand} {
		if _, ok := gamedata.ItemByID(id); !ok {
			t.Errorf("道具 %d 应在道具表里", id)
		}
	}
}
