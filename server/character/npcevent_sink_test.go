package character

// STAGE-316：深红洞窟（任务 37，事件 40）端到端（纯逻辑，不连数据库）：用真实白名单、真实选块
// pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具增减、事件位变化。
// 反向用例覆盖前置没做、没有羽毛、背包满、完成后再来不重复发奖。协议层（对话窗口、推送）不在这里测。
// 脚本的数据检查（白名单、整本支持、NPC 类型和地图、NpcWarp、道具）见本文件末尾的 TestSinkScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	sinkRed     = 1307 // 真红（giiru/event/sink/sink07，达那村长老房间 5106）：接任务 37
	sinkBird    = 1369 // 不死鸟（giiru/event/sink/sink01，拉多拉山山顶 20309）：给不死鸟的羽毛
	sinkIra     = 1417 // 伊拉（giiru/event/sink/sink02，深红洞窟地下9楼 30016）：交羽毛换火之羽，完成事件 40
	sinkQuest   = 40   // 任务 37 的事件号
	sinkQuestNo = 37
	sinkAdult   = 4    // 成人仪式的完成事件（任务 37 的前置）
	sinkFeather = 2706 // 不死鸟的羽毛（接任务后找不死鸟拿）
	sinkWing    = 2707 // 火的羽毛饰品（交任务奖励）

	sinkSerpentMark = 2391 // 蛇王的信物：原版脚本引用、但原版 itemset.txt 里也没有，支线块用不到
)

// sinkPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func sinkPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// sinkFullBag 把背包占满（每格一件 2417，只占格子）。
func sinkFullBag() []itemRec {
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	return full
}

// 任务 37 正向：成人仪式完成后真红接任务 → 不死鸟给羽毛 → 伊拉收羽毛给火之羽，事件 40 完成。
func TestSink37Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{sinkAdult}, nil)
	blk := sinkPick(t, s, r, sinkRed, 2)
	if blk.Type != "REQUEST" || blk.Event != sinkQuest {
		t.Fatalf("接任务应命中事件 40 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(sinkQuest) {
		t.Fatalf("接取后事件 40 应进行中，实际 %v", r.EventFlags)
	}

	blk = sinkPick(t, s, r, sinkBird, 1)
	if blk.Type != "ACCEPT" || len(blk.GetItem) != 1 || blk.GetItem[0].ID != sinkFeather {
		t.Fatalf("不死鸟应给 2706，实际 %s %+v", blk.Type, blk.GetItem)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, sinkFeather) != 1 {
		t.Fatalf("应拿到不死鸟的羽毛，实际 %v", r.Items)
	}

	blk = sinkPick(t, s, r, sinkIra, 0)
	if blk.Type != "ACCEPT" || blk.Event != sinkQuest {
		t.Fatalf("交任务应命中事件 40 的 ACCEPT 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if blk.Charm != 1 {
		t.Errorf("交任务块应加 1 点魅力（原版 Charm:1），实际 %d", blk.Charm)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, sinkWing) != 1 || countItem(r.Items, sinkFeather) != 0 {
		t.Fatalf("交任务后应拿到 2707、收走 2706，实际 %v", r.Items)
	}
	if !r.eventDone(sinkQuest) || r.eventDoing(sinkQuest) {
		t.Fatalf("事件 40 应完成，实际 %v", r.EventFlags)
	}
}

// 反向：没完成成人仪式（ENDEV=4）时，真红只说闲话，接不到任务。
func TestSink37NeedsAdult(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	sinkPick(t, s, r, sinkRed, -1)
	if r.eventDoing(sinkQuest) || len(r.Items) != 0 {
		t.Fatalf("没完成成人仪式不应接到任务 37，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：任务进行中、没有羽毛时，伊拉只提示要带羽毛；不死鸟第一次给羽毛，第二次只说话，不给第二根。
func TestSink37FeatherOnce(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{sinkAdult}, []int{sinkQuest})
	blk := sinkPick(t, s, r, sinkIra, 9)
	if blk.Type != "MESSAGE" || blk.Event != -1 {
		t.Fatalf("没有羽毛时伊拉应提示（MESSAGE），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, sinkPick(t, s, r, sinkBird, 1))
	if countItem(r.Items, sinkFeather) != 1 {
		t.Fatalf("第一次应拿到羽毛，实际 %v", r.Items)
	}
	sinkPick(t, s, r, sinkBird, 2) // ITEM=2706：只说羽毛已经在手上
	if countItem(r.Items, sinkFeather) != 1 {
		t.Fatalf("第二次不应再给羽毛，实际 %v", r.Items)
	}
}

// 反向：背包满时不死鸟给不了羽毛，交易不成立、台词是 ItemFullMsg，事件和背包都不变。
func TestSink37FullBagChangesNothing(t *testing.T) {
	s := kingService()
	r := kingRecord(sinkFullBag(), []int{sinkAdult}, []int{sinkQuest})
	blk := sinkPick(t, s, r, sinkBird, 1)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, sinkFeather) != 0 || len(r.Items) != len(sinkFullBag()) || !r.eventDoing(sinkQuest) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 反向：事件 40 完成后，即使身上又有一根羽毛，伊拉也只说完成后的闲话，不会再换一次火之羽。
func TestSink37DoneDoesNotRepeat(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(sinkFeather), []int{sinkAdult, sinkQuest}, nil)
	blk := sinkPick(t, s, r, sinkIra, 8)
	if blk.Type != "MESSAGE" || blk.Event != -1 {
		t.Fatalf("完成后应命中 ENDEV=40 的闲话块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, sinkWing) != 0 || countItem(r.Items, sinkFeather) != 1 {
		t.Fatalf("完成后不应再发 2707、也不应收走 2706，实际 %v", r.Items)
	}
	// 真红和不死鸟在完成后也都只说闲话。
	sinkPick(t, s, r, sinkRed, 0)
	sinkPick(t, s, r, sinkBird, 0)
}

// 数据检查：两个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 37 的完成事件是 40；
// 用到的道具都查得到；不死鸟给羽毛后会按 NpcWarp 在拉多拉山山顶的四个点之间移动。
func TestSinkScriptData(t *testing.T) {
	files := map[int]string{
		sinkRed:  "giiru/event/sink/sink07",
		sinkBird: "giiru/event/sink/sink01",
		sinkIra:  "giiru/event/sink/sink02",
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
					if term.Op != "ITEM" {
						continue
					}
					if term.ID == sinkSerpentMark {
						// 原版 itemset.txt 里也没有 2391（蛇王的信物），这个支线块永远拿不到条件道具；
						// 任务 37 不经过它，照原版保留。
						if _, ok := gamedata.ItemByID(term.ID); ok {
							t.Errorf("2391 不在原版道具表里，测试假设已过时")
						}
						continue
					}
					checkKingItem(t, script.File, term.ID)
				}
			}
			for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
				checkKingItem(t, script.File, it.ID)
			}
		}
	}
	q, ok := gamedata.QuestByNo(sinkQuestNo)
	if !ok || q.EndEvent != sinkQuest {
		t.Fatalf("任务 %d 的完成事件应是 %d，实际 %+v", sinkQuestNo, sinkQuest, q)
	}
	if _, ok := gamedata.QuestByNo(1); !ok { // 前置任务 1（成人仪式）的完成事件是 4
		t.Fatal("任务 1 应存在")
	}

	// 不死鸟给羽毛后会在 20309 的四个点之间移动（原版 NpcWarp），第一个点是 NPC 的初始位置。
	script, _ := gamedata.NpcEventOf(sinkBird)
	want := []gamedata.WarpPoint{{Floor: 20309, X: 18, Y: 13}, {Floor: 20309, X: 55, Y: 49}, {Floor: 20309, X: 32, Y: 28}, {Floor: 20309, X: 46, Y: 45}}
	got := script.Blocks[1].Warp
	if len(got) != len(want) {
		t.Fatalf("不死鸟给羽毛的块应有 %d 个移动点，实际 %+v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个移动点应是 %+v，实际 %+v", i, want[i], got[i])
		}
	}
	if npc, _ := gamedata.NPCByID(sinkBird); npc.X != 46 || npc.Y != 45 {
		t.Errorf("不死鸟的初始位置应是 (46,45)，实际 (%d,%d)", npc.X, npc.Y)
	}
}
