package character

// STAGE-321：马祖任务 5（事件 37）和水田迷宫任务 9（事件 38）端到端（纯逻辑，不连数据库）：
// 用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具增减、事件位变化。
// 顺带固定 NOWEV!= 的原版语义（恒为真）：没接任务时卡伊特也会给切绳石。
// 反向用例覆盖等级不够、没有信、背包满、完成后再来。协议层（对话窗口、推送）不在这里测。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	marjuPost   = 1252 // 马祖（extra/event/marju，1300）：任务 5 接取、任务 9 交付
	marjuKite   = 1196 // 卡伊特（extra/event/marju01，119）：任务 5 的收信人
	marjuQ5     = 37   // 任务 5 的完成事件
	marjuQ9     = 38   // 任务 9 的完成事件
	marjuLetter = 2687 // 马祖的信
	marjuStone  = 2697 // 切绳石
	marjuReward = 2688 // 任务 9 的随机奖励里 kingRand 取到的第一件
)

// marjuPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func marjuPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// 任务 5、9 正向：马祖接信 → 卡伊特收信给切绳石（完成 5）→ 回马祖交任务拿随机奖励（完成 9）。
func TestMarjuQuestsWalk(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 50
	blk := marjuPick(t, s, r, marjuPost, 0)
	if blk.Type != "REQUEST" || blk.Event != marjuQ5 {
		t.Fatalf("接任务应命中事件 37 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, marjuLetter) != 1 || !r.eventDoing(marjuQ5) {
		t.Fatalf("接取后应拿到信、事件 37 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, marjuPick(t, s, r, marjuKite, 0))
	if countItem(r.Items, marjuLetter) != 0 || countItem(r.Items, marjuStone) != 1 || !r.eventDone(marjuQ5) {
		t.Fatalf("卡伊特应收信给切绳石、完成事件 37，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingRun(t, r, marjuPick(t, s, r, marjuPost, 1))
	if countItem(r.Items, marjuReward) != 1 || !r.eventDone(marjuQ9) {
		t.Fatalf("回马祖交任务应拿随机奖励、完成事件 38，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	marjuPick(t, s, r, marjuPost, 2) // 完成后只说闲话
}

// 反向：不到 50 级时马祖只有闲聊；没有信时卡伊特的交信块不成立，但 NOWEV!=37 恒真，
// 40 级以上、没有切绳石时仍会命中备用块给切绳石（原版行为）。
func TestMarjuGatesAndNowEvNot(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 49
	marjuPick(t, s, r, marjuPost, -1)
	if r.eventDoing(marjuQ5) || len(r.Items) != 0 {
		t.Fatalf("49 级不应接到任务 5，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	r = kingRecord(nil, nil, nil)
	r.Level = 40
	blk := marjuPick(t, s, r, marjuKite, 2) // NOWEV!=37 恒真、ENDEV!=37、LV>39、没有 2697
	if blk.Type != "ACCEPT" {
		t.Fatalf("没接任务时卡伊特应命中备用块（NOWEV!= 恒真），实际 %s", blk.Type)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, marjuStone) != 1 {
		t.Fatalf("备用块应给切绳石 2697，实际 %v", r.Items)
	}

	r = kingRecord(nil, nil, []int{marjuQ5}) // 进行中但没有信
	r.Level = 50
	blk = marjuPick(t, s, r, marjuKite, 2) // ENDEV!=37 成立（37 没完成）、NOWEV!= 恒真
	if blk.Type != "ACCEPT" {
		t.Fatalf("进行中没信时卡伊特仍命中备用块，实际 %s", blk.Type)
	}
}

// 反向：背包满时马祖给不了信（ItemFullMsg），事件不进入进行中。
func TestMarjuFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, nil)
	r.Level = 50
	blk := marjuPick(t, s, r, marjuPost, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(marjuQ5) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 数据检查：两个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 5、9 的完成事件对得上；
// 信、切绳石和随机奖励都在道具表里，卡伊特的块 2 保留 NOWEV!=37（转换器按原版恒真放开）。
func TestMarjuScriptData(t *testing.T) {
	files := map[int]string{marjuPost: "extra/event/marju", marjuKite: "extra/event/marju01"}
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
	for no, event := range map[int]int{5: marjuQ5, 9: marjuQ9} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	kite, _ := gamedata.NpcEventOf(marjuKite)
	hasNot := false
	for _, g := range kite.Blocks[2].Cond {
		for _, term := range g {
			if term.Op == "NOWEV" && term.Cmp == "!=" && term.N == 37 {
				hasNot = true
			}
		}
	}
	if !hasNot {
		t.Errorf("卡伊特块 2 应保留 NOWEV!=37，实际 %+v", kite.Blocks[2].Cond)
	}
	for _, id := range []int{marjuLetter, marjuStone, marjuReward, 2689, 2690} {
		if _, ok := gamedata.ItemByID(id); !ok {
			t.Errorf("道具 %d 应在道具表里", id)
		}
	}
}
