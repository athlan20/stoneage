package character

// STAGE-315（第 3 组）：亚姆亚姆的斧头任务 12、13（事件 3、5）和玄黄洞窟任务 38、39（事件 41、42）
// 端到端（纯逻辑，不连数据库）：用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言
// 命中的块、道具增减、事件位变化。反向用例覆盖顺序不对、丢了中间道具、背包满、完成后再来。
// 协议层（对话窗口、推送）不在这里测。脚本的数据检查见本文件末尾的 TestSainasuAxeScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	axeMati     = 1249 // 马提（sainasu/event/event03_1，1300）：收斧头、写感谢信
	axeYamu     = 1253 // 亚姆亚姆（sainasu/event/event03_2，1400）：给斧头、收信换改良斧头
	genouWill   = 1362 // 威尔（sainasu/event/genou_1，11215）：任务 38 的接取、任务 39 的交付
	genouMuhasa = 1186 // 姆哈萨（sainasu/event/genou_3，100）：任务 39 的接取
	genouBoss   = 1359 // 梦德族长（sainasu/event/event12，11107）：任务 38 给风之竖琴

	axeQuest12 = 3    // 任务 12 的事件号（送斧头）
	axeQuest13 = 5    // 任务 13 的事件号（还信）
	axeGive    = 2416 // 亚姆亚姆的斧头（会消失的那把）
	axeNote    = 2423 // 马提的信
	axeBetter  = 2448 // 亚姆亚姆的斧头（改良过、不会消失的那把）
	genouQ38   = 41   // 任务 38 的事件号
	genouQ39   = 42   // 任务 39 的事件号
	genouHarp  = 2736 // 风的竖琴
	genouLock  = 2735 // 风的锁
)

// genouPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func genouPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// 斧头任务 12、13 正向：亚姆亚姆给斧头 → 马提收斧头（完成 12）→ 马提给信 → 亚姆亚姆收信换改良斧头（完成 13）。
func TestAxeQuestsWalk(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	blk := genouPick(t, s, r, axeYamu, 1)
	if blk.Type != "REQUEST" || blk.Event != axeQuest12 {
		t.Fatalf("接任务应命中事件 3 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, axeGive) != 1 || !r.eventDoing(axeQuest12) {
		t.Fatalf("接任务应拿到斧头 2416、事件 3 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, genouPick(t, s, r, axeMati, 0))
	if countItem(r.Items, axeGive) != 0 || !r.eventDone(axeQuest12) {
		t.Fatalf("马提应收走斧头、完成事件 3，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, genouPick(t, s, r, axeMati, 1))
	if countItem(r.Items, axeNote) != 1 || !r.eventDoing(axeQuest13) {
		t.Fatalf("马提应给信 2423、事件 5 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, genouPick(t, s, r, axeYamu, 0))
	if countItem(r.Items, axeBetter) != 1 || countItem(r.Items, axeNote) != 0 || !r.eventDone(axeQuest13) {
		t.Fatalf("亚姆亚姆应收信换改良斧头、完成事件 5，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	genouPick(t, s, r, axeMati, -1) // 马提没有完成后的块，只说闲聊
	genouPick(t, s, r, axeYamu, 2)  // 亚姆亚姆命中 ENDEV=3/5 的闲话
}

// 反向：没有斧头时马提没有块命中；任务 12 进行中再找亚姆亚姆是放弃问话，不会重复给斧头。
func TestAxeOrderAndRepeat(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	genouPick(t, s, r, axeMati, -1)
	if r.eventDoing(axeQuest12) || len(r.Items) != 0 {
		t.Fatalf("没有斧头时马提不应给任何东西，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	r = kingRecord(nil, nil, nil)
	kingRun(t, r, genouPick(t, s, r, axeYamu, 1))
	blk := genouPick(t, s, r, axeYamu, 1)
	if blk.Type != "REQUEST" || blk.Event != axeQuest12 {
		t.Fatalf("进行中再找亚姆亚姆应命中 REQUEST（放弃问话），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, axeGive) != 1 {
		t.Fatalf("进行中不应重复给斧头，实际 %v", r.Items)
	}
}

// 反向：背包满时亚姆亚姆给不了斧头，台词是 ItemFullMsg，事件不进入进行中。
func TestAxeFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, nil)
	blk := genouPick(t, s, r, axeYamu, 1)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(axeQuest12) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 玄黄洞窟正向：威尔接任务 38 → 姆哈萨接任务 39 → 梦德族长给风之竖琴（完成 38）→ 威尔收竖琴给风之锁（完成 39）。
func TestGenouQuestsWalk(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil) // 成人仪式完成
	blk := genouPick(t, s, r, genouWill, 3)
	if blk.Type != "REQUEST" || blk.Event != genouQ38 {
		t.Fatalf("接任务应命中事件 41 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(genouQ38) {
		t.Fatalf("接取后事件 41 应进行中，实际 %v", r.EventFlags)
	}

	kingRun(t, r, genouPick(t, s, r, genouMuhasa, 1))
	if !r.eventDoing(genouQ39) {
		t.Fatalf("接取后事件 42 应进行中，实际 %v", r.EventFlags)
	}

	blk = genouPick(t, s, r, genouBoss, 3)
	if blk.Type != "ACCEPT" || blk.Event != genouQ38 {
		t.Fatalf("族长应给风之竖琴（ACCEPT 41），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if blk.Charm != 1 {
		t.Errorf("族长给竖琴的块应加 1 点魅力，实际 %d", blk.Charm)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, genouHarp) != 1 || !r.eventDone(genouQ38) {
		t.Fatalf("应拿到风之竖琴、完成事件 41，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	blk = genouPick(t, s, r, genouWill, 0)
	if blk.Type != "ACCEPT" || blk.Event != genouQ39 {
		t.Fatalf("交竖琴应命中 ACCEPT 42，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, genouLock) != 1 || countItem(r.Items, genouHarp) != 0 || !r.eventDone(genouQ39) {
		t.Fatalf("交竖琴应换到风之锁、完成事件 42，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	genouPick(t, s, r, genouWill, 2) // 完成后命中 ENDEV=42 的闲话（没有宠物 95 时块 1 不命中）
}

// 反向：只接任务 38、没接任务 39 时族长不给竖琴；任务 38 没完成时威尔不收回竖琴；
// 丢了竖琴（事件 38 已完成、事件 39 进行中）可以找族长补一把。
func TestGenouOrderAndReissue(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil)
	kingRun(t, r, genouPick(t, s, r, genouWill, 3))
	genouPick(t, s, r, genouBoss, -1) // 事件 42 没进行中，族长不给
	if countItem(r.Items, genouHarp) != 0 {
		t.Fatalf("事件 42 未进行中不应给竖琴，实际 %v", r.Items)
	}

	kingRun(t, r, genouPick(t, s, r, genouMuhasa, 1))
	kingRun(t, r, genouPick(t, s, r, genouBoss, 3))
	// 这时事件 41 完成、42 进行中；先把竖琴丢掉，再找族长补一把（块 2 的兜底）。
	r.Items = nil
	kingRun(t, r, genouPick(t, s, r, genouBoss, 2))
	if countItem(r.Items, genouHarp) != 1 {
		t.Fatalf("丢了竖琴应能补一把，实际 %v", r.Items)
	}
	// 没有完成事件 41 时威尔只问放弃，不会收竖琴。
	r = kingRecord(kingItems(genouHarp), []int{4}, []int{genouQ39})
	blk := genouPick(t, s, r, genouWill, 3)
	if blk.Type != "REQUEST" || blk.Event != genouQ38 {
		t.Fatalf("事件 41 未完成时应命中事件 41 的 REQUEST（放弃问话），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, genouHarp) != 1 || r.eventDone(genouQ39) {
		t.Fatalf("事件 41 未完成不应收走竖琴，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：背包满时族长给不了竖琴，台词是 ItemFullMsg，事件和背包都不变。
func TestGenouHarpFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, []int{4}, []int{genouQ38, genouQ39})
	blk := genouPick(t, s, r, genouBoss, 3)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, genouHarp) != 0 || len(r.Items) != len(full) || !r.eventDoing(genouQ38) {
		t.Fatalf("失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 数据检查：四个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；四条任务的完成事件对得上；
// 斧头、信、改良斧头、风之竖琴和风之锁都在道具表里；竖琴由族长给、锁由威尔给。
func TestSainasuAxeScriptData(t *testing.T) {
	files := map[int]string{
		axeMati:     "sainasu/event/event03_1",
		axeYamu:     "sainasu/event/event03_2",
		genouWill:   "sainasu/event/genou_1",
		genouMuhasa: "sainasu/event/genou_3",
	}
	for npcID, file := range files {
		script, ok := gamedata.NpcEventOf(npcID)
		if !ok || script.File != file {
			t.Fatalf("NPC %d 应对应 %s，实际 %+v ok=%v", npcID, file, script, ok)
		}
		if !gamedata.NpcEventEnabled(script.File) {
			t.Errorf("%s 应已放开", script.File)
		}
		npc, ok := gamedata.NPCByID(npcID)
		if !ok || npc.Kind != "changeevent" || !gamedata.FormalFloor(npc.Floor) {
			t.Errorf("%s 的 NPC %d 应是任务类型、在正式地图上，实际 %+v", script.File, npcID, npc)
		}
		for i := range script.Blocks {
			b := &script.Blocks[i]
			if b.Unsupported != "" {
				t.Errorf("%s 块 %d 不支持：%s", script.File, i, b.Unsupported)
			}
			for _, g := range b.Cond {
				for _, term := range g {
					if term.Op == "ITEM" {
						checkKingItem(t, script.File, term.ID)
					}
				}
			}
			for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
				checkKingItem(t, script.File, it.ID)
			}
		}
	}
	for no, event := range map[int]int{12: axeQuest12, 13: axeQuest13, 38: genouQ38, 39: genouQ39} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	boss, _ := gamedata.NpcEventOf(genouBoss)
	if boss.Blocks[3].GetItem[0].ID != genouHarp {
		t.Errorf("族长的块 3 应给风之竖琴，实际 %+v", boss.Blocks[3].GetItem)
	}
	will, _ := gamedata.NpcEventOf(genouWill)
	if will.Blocks[0].GetItem[0].ID != genouLock {
		t.Errorf("威尔的块 0 应给风之锁，实际 %+v", will.Blocks[0].GetItem)
	}
}
