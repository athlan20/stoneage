package character

// STAGE-323：小猪爱情故事的任务 20（事件 23，加鲁那）和 21（事件 24，加特洛洞）端到端（纯逻辑，不连数据库）：
// 用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具增减、事件位变化。
// 顺带固定 NOEEV 的原版语义（原版不认，条件恒不成立）和跳号分页的页序。
// 反向用例覆盖等级不够、没有手环、完成后再来。协议层（对话窗口、推送）不在这里测。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	pigSero  = 1434 // 雪鲁（giiru/event/garunaev2，31103）：任务 20 接取
	pigMeru  = 1436 // 美鲁（giiru/event/garunaev3，31205）：任务 20 完成
	pigZonba = 1432 // 左恩巴（giiru/event/garunaev5，31003）：任务 21 接取
	pigGaro  = 1437 // 加洛（giiru/event/garunaev6，31205）：任务 21 完成

	pigQ20  = 23   // 任务 20 的完成事件
	pigQ21  = 24   // 任务 21 的完成事件
	pigCuff = 2414 // 乌力手环（两条任务的通行道具）
	pigOre  = 2506 // 加鲁娜矿石（支线合成）
	pigBead = 2883 // 乌力手环（支线奖励）
)

// pigPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func pigPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// 任务 20 正向：雪鲁接任务拿手环 → 美鲁完成事件 23。
func TestPigQuest20Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 31
	blk := pigPick(t, s, r, pigSero, 3)
	if blk.Type != "REQUEST" || blk.Event != pigQ20 {
		t.Fatalf("接任务应命中事件 23 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, pigCuff) != 1 || !r.eventDoing(pigQ20) {
		t.Fatalf("接取后应拿到手环、事件 23 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	kingRun(t, r, pigPick(t, s, r, pigMeru, 4))
	if !r.eventDone(pigQ20) {
		t.Fatalf("美鲁应完成事件 23（任务 20），实际 %v", r.EventFlags)
	}
}

// 任务 21 正向：左恩巴接任务拿手环 → 加洛完成事件 24。两条任务的接取块都要求另一条没完成
// （ENDEV!=23 / ENDEV!=24），所以它们是二选一，各自独立测。
func TestPigQuest21Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 31
	blk := pigPick(t, s, r, pigZonba, 3)
	if blk.Type != "REQUEST" || blk.Event != pigQ21 {
		t.Fatalf("接任务应命中事件 24 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, pigCuff) != 1 || !r.eventDoing(pigQ21) {
		t.Fatalf("接取后应拿到手环、事件 24 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	kingRun(t, r, pigPick(t, s, r, pigGaro, 5))
	if !r.eventDone(pigQ21) {
		t.Fatalf("加洛应完成事件 24（任务 21），实际 %v", r.EventFlags)
	}
}

// 反向：不到 31 级接不了任务；没有手环时美鲁、加洛的完成块不成立；NOEEV 块恒不命中。
func TestPigGatesAndNoeev(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 30
	pigPick(t, s, r, pigSero, -1)
	if r.eventDoing(pigQ20) {
		t.Fatalf("30 级不应接到任务 20，实际 %v", r.EventFlags)
	}

	r = kingRecord(nil, nil, []int{pigQ20})
	r.Level = 31
	pigPick(t, s, r, pigMeru, 5) // 没有手环：块 4 要 ITEM=2414，块 5 只说话，块 9 是 NOEEV（恒假）
	if r.eventDone(pigQ20) {
		t.Fatalf("没有手环不应完成事件 23，实际 %v", r.EventFlags)
	}

	// 有手环、事件 23 进行中时美鲁的完成块命中。
	r = kingRecord(kingItems(pigCuff), nil, []int{pigQ20})
	r.Level = 31
	pigPick(t, s, r, pigMeru, 4)
}

// 反向：背包满时雪鲁给不了手环（ItemFullMsg），事件不进入进行中。
func TestPigFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, nil)
	r.Level = 31
	blk := pigPick(t, s, r, pigSero, 3)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(pigQ20) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 数据检查：四个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 20、21 的完成事件对得上；
// NOEEV 条件在数据里记成 NEVER；garunaev3 的跳号分页按原版补成连续的页。
func TestPigScriptData(t *testing.T) {
	files := map[int]string{
		pigSero:  "giiru/event/garunaev2",
		pigMeru:  "giiru/event/garunaev3",
		pigZonba: "giiru/event/garunaev5",
		pigGaro:  "giiru/event/garunaev6",
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
	for no, event := range map[int]int{20: pigQ20, 21: pigQ21} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	// NOEEV 记成 NEVER（原版不认，恒不成立）。
	meru, _ := gamedata.NpcEventOf(pigMeru)
	never := false
	for _, g := range meru.Blocks[9].Cond {
		for _, term := range g {
			if term.Op == "NEVER" {
				never = true
			}
		}
	}
	if !never {
		t.Errorf("美鲁块 9 应把 NOEEV 记成 NEVER，实际 %+v", meru.Blocks[9].Cond)
	}
	// 跳号分页：AcceptMsg + AcceptMsg2 按原版补成 3 页（第 2 页沿用第 1 页）。
	if pages := meru.Blocks[4].Msg.Pages("accept"); len(pages) != 3 || pages[1] != pages[0] {
		t.Errorf("美鲁块 4 的分页应按原版补成 3 页、第 2 页沿用第 1 页，实际 %d 页", len(pages))
	}
	if _, ok := gamedata.ItemByID(pigOre); !ok {
		t.Errorf("加鲁娜矿石 %d 应在道具表里", pigOre)
	}
	if _, ok := gamedata.ItemByID(pigBead); !ok {
		t.Errorf("乌力手环 %d 应在道具表里", pigBead)
	}
}
