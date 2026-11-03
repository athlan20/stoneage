package character

// STAGE-322：暴龙之路任务 14（事件 27）、黄金羚羊之路任务 15、16、17（事件 30、29、54）端到端
// （纯逻辑，不连数据库）：用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言命中的块、
// 道具增减、事件位变化。反向用例覆盖顺序不对、道具不齐、背包满、完成后再来。
// 协议层（对话窗口、推送）不在这里测。脚本的数据检查见本文件末尾的 TestGiiruScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	giTitan  = 1475 // 帖尼姆团长（giiru/event/giruev2_1，31805）：任务 14
	giQuarry = 1477 // 采石所管理人（giiru/event/event30_1，31902）：任务 15 接取
	giStone  = 1478 // 精灵之石（giiru/event/event30_2，31909）：任务 16、17
	giWood   = 1479 // 精灵之木（giiru/event/event30_6，31911）：任务 16 完成
	giKui    = 1419 // 库伊老爷爷（king/event69_1，30600，已在白名单）：任务 15、17 的完成块

	giQ14    = 27   // 任务 14 的完成事件
	giQ15    = 30   // 任务 15 的完成事件
	giQ16    = 29   // 任务 16 的完成事件
	giQ17    = 54   // 任务 17 的完成事件
	giPermit = 2527 // 采石许可证（任务 14 的奖励）
	giRight  = 2533 // 采石权（任务 14 的中间物，怪物 1004 掉落）
	giKey    = 2549 // 采石场的钥匙
	giMirror = 2547 // 心性之镜（任务 15 的奖励）
	giEye    = 2548 // 真龟之眼（任务 16 的中间物，怪物 1060 掉落）
	giCloth  = 2551 // 守护精灵的衣服（任务 16 的奖励）
)

// giPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func giPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// giAdd 往背包的空格子里放一件道具（每件占一格），给测试准备打怪掉落。
func giAdd(items *[]itemRec, id int) {
	used := make(map[int]bool, len(*items))
	for _, it := range *items {
		used[it.Slot] = true
	}
	slot := firstBagSlot
	for used[slot] {
		slot++
	}
	*items = append(*items, itemRec{Slot: slot, ItemID: id, Count: 1})
}

// 任务 14 正向：帖尼姆团长接任务（Lv30）→ 打怪拿采石权 2533 → 交回换许可证 2527，事件 27 完成。
func TestGiiruDragon14Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 30
	blk := giPick(t, s, r, giTitan, 3)
	if blk.Type != "REQUEST" || blk.Event != giQ14 {
		t.Fatalf("接任务应命中事件 27 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(giQ14) {
		t.Fatalf("接取后事件 27 应进行中，实际 %v", r.EventFlags)
	}
	giAdd(&r.Items, giRight) // 怪物 1004 掉落
	kingRun(t, r, giPick(t, s, r, giTitan, 1))
	if countItem(r.Items, giRight) != 0 || countItem(r.Items, giPermit) != 1 || !r.eventDone(giQ14) {
		t.Fatalf("交采石权应换到许可证、完成事件 27，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：没有采石权时交不了任务 14（命中 REQUEST 的放弃问话）；不到 30 级接不了。
func TestGiiruDragon14Gates(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 29
	giPick(t, s, r, giTitan, -1)
	if r.eventDoing(giQ14) {
		t.Fatalf("29 级不应接到任务 14，实际 %v", r.EventFlags)
	}

	r = kingRecord(nil, nil, []int{giQ14})
	r.Level = 30
	blk := giPick(t, s, r, giTitan, 3)
	if blk.Type != "REQUEST" {
		t.Fatalf("没有 2533 时应命中 REQUEST（放弃问话），实际 %s", blk.Type)
	}
	if countItem(r.Items, giPermit) != 0 {
		t.Fatalf("没有采石权不应给许可证，实际 %v", r.Items)
	}
}

// 任务 15、16、17 正向：采石所管理人接任务 15 → 精灵之石接任务 17 → 库伊给心性之镜（完成 15）→
// 拿真龟之眼在精灵之石接任务 16 → 精灵之木给衣服（完成 16）→ 库伊完成 17。
func TestGiiruGoldSheep15To17Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil) // 成人仪式完成
	kingRun(t, r, giPick(t, s, r, giQuarry, 7))
	if countItem(r.Items, giKey) != 1 || !r.eventDoing(giQ15) {
		t.Fatalf("接任务 15 应拿到钥匙、事件 30 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	kingRun(t, r, giPick(t, s, r, giStone, 4))
	if !r.eventDoing(giQ17) {
		t.Fatalf("接任务 17 后事件 54 应进行中，实际 %v", r.EventFlags)
	}
	kingRun(t, r, giPick(t, s, r, giKui, 6))
	if countItem(r.Items, giMirror) != 1 || !r.eventDone(giQ15) {
		t.Fatalf("库伊应给心性之镜、完成事件 30，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	giAdd(&r.Items, giEye) // 怪物 1060 掉落
	kingRun(t, r, giPick(t, s, r, giStone, 2))
	if !r.eventDoing(giQ16) {
		t.Fatalf("拿真龟之眼找精灵之石应接任务 16，实际 %v", r.EventFlags)
	}
	kingRun(t, r, giPick(t, s, r, giWood, 1))
	if countItem(r.Items, giCloth) != 1 || !r.eventDone(giQ16) {
		t.Fatalf("精灵之木应给守护精灵的衣服、完成事件 29，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	kingRun(t, r, giPick(t, s, r, giKui, 2))
	if countItem(r.Items, giMirror) != 0 || !r.eventDone(giQ17) {
		t.Fatalf("库伊应交走心性之镜、完成事件 54，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：没接任务 15 时库伊不给心性之镜；没有真龟之眼时接不了任务 16；先交心性之镜也交不了任务 17。
func TestGiiruGoldSheepOrder(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil)
	giPick(t, s, r, giKui, 7) // 没有任务 15、17，库伊命中 ENDEV=4 的兜底闲话
	if r.eventDone(giQ15) || countItem(r.Items, giMirror) != 0 {
		t.Fatalf("没接任务时不应给心性之镜，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	r = kingRecord(nil, []int{4, giQ15}, []int{giQ17})
	giPick(t, s, r, giStone, 3) // 没有 2548：命中“还没找齐线索”的提示块
	if r.eventDoing(giQ16) {
		t.Fatalf("没有真龟之眼不应接任务 16，实际 %v", r.EventFlags)
	}

	r = kingRecord(kingItems(giMirror), []int{4, giQ15}, []int{giQ17})
	giPick(t, s, r, giKui, 4) // 事件 29 未完成，命中任务 17 的提示块
	if r.eventDone(giQ17) {
		t.Fatalf("事件 29 未完成时不应完成事件 54，实际 %v", r.EventFlags)
	}
}

// 反向：背包满时接任务 15 拿不到钥匙（ItemFullMsg），事件不进入进行中。
func TestGiiruFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, []int{4}, nil)
	blk := giPick(t, s, r, giQuarry, 7)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(giQ15) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 数据检查：四个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 14–17 的完成事件对得上；
// 采石权 2533、真龟之眼 2548 有怪物掉落来源；心性之镜 2547 和守护精灵的衣服 2551 的来源对得上。
func TestGiiruScriptData(t *testing.T) {
	files := map[int]string{
		giTitan:  "giiru/event/giruev2_1",
		giQuarry: "giiru/event/event30_1",
		giStone:  "giiru/event/event30_2",
		giWood:   "giiru/event/event30_6",
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
					if term.Op == "ITEM" && term.ID != 2391 { // 2391 原版道具表里也没有
						checkKingItem(t, script.File, term.ID)
					}
				}
			}
			for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
				if it.ID != 2391 { // 2391 原版道具表里也没有（DelItem 空操作）
					checkKingItem(t, script.File, it.ID)
				}
			}
		}
	}
	for no, event := range map[int]int{14: giQ14, 15: giQ15, 16: giQ16, 17: giQ17} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	// 2533 由怪物 1004、2548 由怪物 1060 100% 掉落，玩家打得到。
	for enemy, item := range map[int]int{1004: giRight, 1060: giEye} {
		e, ok := gamedata.EnemyByID(enemy)
		if !ok {
			t.Errorf("怪物 %d 不在敌人表里", enemy)
			continue
		}
		found := false
		for _, d := range e.Drops {
			if d.Item == item {
				found = true
			}
		}
		if !found {
			t.Errorf("怪物 %d 应掉落道具 %d，实际掉落 %+v", enemy, item, e.Drops)
		}
	}
	kui, _ := gamedata.NpcEventOf(giKui)
	if kui.Blocks[6].GetItem[0].ID != giMirror || len(kui.Blocks[6].EndSet) != 1 || kui.Blocks[6].EndSet[0] != giQ15 {
		t.Errorf("库伊的块 6 应给心性之镜、完成事件 30，实际 %+v", kui.Blocks[6])
	}
}
