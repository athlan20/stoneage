package character

// STAGE-315（第 2 组）：梦德洞窟的任务 8（事件 9）端到端（纯逻辑，不连数据库）：柯尔克、霍特尔、卡坦三地
// 接任务 → 梦德洞窟找族长拿两块保护石 → 先还卡坦（完成事件 11）→ 再还霍特尔（完成事件 10、12）→
// 回柯尔克交任务（完成事件 9）。每步断言命中的块、道具增减、事件位变化。
// 反向用例覆盖前置没做、顺序不对、背包满、完成后再来。协议层不在这里测。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	dreamKirk   = 1246 // 柯尔克村姑（sainasu/event/event09，1200）：任务 8 的接取和交付
	dreamHotel  = 1251 // 霍特尔村姑（sainasu/event/event10，1300）：收霍特尔的保护石 2419
	dreamKatan  = 1254 // 卡坦的年轻人（sainasu/event/event11，1400）：收卡坦的保护石 2420
	dreamBoss   = 1359 // 梦德族长（sainasu/event/event12，11107）：给两块保护石
	dreamQuest  = 9    // 任务 8 的事件号
	dreamStoneH = 2419 // 霍特尔的保护石
	dreamStoneK = 2420 // 卡坦的保护石
	dreamGem    = 2421 // 勾玉（还霍特尔的奖励）
	dreamArmor  = 1052 // 芙雷雅Lv1铠6（还卡坦的奖励）
	dreamReward = 1357 // 黄色大首饰（交任务 8 的奖励）
)

// dreamPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func dreamPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// dreamHero 是 30 级、完成成人仪式的勇者小明（梦德洞窟的前置）。
func dreamHero() *record {
	r := kingRecord(nil, []int{4}, nil)
	r.Level = 30
	return r
}

// 任务 8 正向：三地接任务 → 族长给两块保护石 → 先还卡坦（完成 11）→ 还霍特尔（完成 10、12）→
// 回柯尔克交任务（完成 9），拿到勾玉、铠和黄色大首饰。
func TestDreamQuest8Walks(t *testing.T) {
	s := kingService()
	r := dreamHero()
	kingRun(t, r, dreamPick(t, s, r, dreamKirk, 1)) // 柯尔克的 REQUEST 是块 1
	// 先接卡坦（只有 NOWEV=9：块 3），再霍特尔（NOWEV=9&11：块 2）。
	kingRun(t, r, dreamPick(t, s, r, dreamKatan, 3))
	kingRun(t, r, dreamPick(t, s, r, dreamHotel, 2))
	if !r.eventDoing(dreamQuest) || !r.eventDoing(10) || !r.eventDoing(11) {
		t.Fatalf("三地接取后事件 9、10、11 都应进行中，实际 %v", r.EventFlags)
	}

	kingRun(t, r, dreamPick(t, s, r, dreamBoss, 0))
	if countItem(r.Items, dreamStoneH) != 1 || countItem(r.Items, dreamStoneK) != 1 {
		t.Fatalf("族长应给两块保护石，实际 %v", r.Items)
	}

	// 先还卡坦：块 1（NOWEV=11&ITEM=2420），只完成事件 11。
	kingRun(t, r, dreamPick(t, s, r, dreamKatan, 1))
	if countItem(r.Items, dreamArmor) != 1 || countItem(r.Items, dreamStoneK) != 0 {
		t.Fatalf("还卡坦保护石应拿到 1052、收走 2420，实际 %v", r.Items)
	}
	if !r.eventDone(11) || r.eventDoing(11) || r.eventDone(10) {
		t.Fatalf("还卡坦后事件 11 完成、10 仍未完成，实际 %v", r.EventFlags)
	}

	// 再还霍特尔：块 0（ENDEV=11&NOWEV=10&ITEM=2419），完成事件 10 和 12。
	kingRun(t, r, dreamPick(t, s, r, dreamHotel, 0))
	if countItem(r.Items, dreamGem) != 1 || countItem(r.Items, dreamStoneH) != 0 {
		t.Fatalf("还霍特尔保护石应拿到 2421、收走 2419，实际 %v", r.Items)
	}
	if !r.eventDone(10) || !r.eventDone(12) {
		t.Fatalf("还霍特尔后事件 10、12 应完成，实际 %v", r.EventFlags)
	}

	// 回柯尔克交任务：块 0（ENDEV=12&ENDEV=10&ENDEV=11）。
	kingRun(t, r, dreamPick(t, s, r, dreamKirk, 0))
	if countItem(r.Items, dreamReward) != 1 || !r.eventDone(dreamQuest) {
		t.Fatalf("交任务 8 应拿到 1357、事件 9 完成，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	dreamPick(t, s, r, dreamKirk, 2) // 完成后只说闲话
}

// 反向：没完成成人仪式或不到 30 级时，柯尔克村姑没有块命中，接不到任务。
func TestDreamQuest8Gates(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil) // 30 级但没完成成人仪式
	r.Level = 30
	dreamPick(t, s, r, dreamKirk, -1)
	if r.eventDoing(dreamQuest) {
		t.Fatalf("没完成成人仪式不应接到任务 8，实际 %v", r.EventFlags)
	}
	r = kingRecord(nil, []int{4}, nil) // 完成了成人仪式但只有 1 级
	dreamPick(t, s, r, dreamKirk, -1)
	if r.eventDoing(dreamQuest) {
		t.Fatalf("1 级不应接到任务 8，实际 %v", r.EventFlags)
	}
}

// 反向：只接了柯尔克的任务（事件 10、11 没进行中）时族长没有块命中；三地任务都在进行中、
// 已经拿过保护石时，族长也不会再给第二套；先还霍特尔（事件 11 未完成）也交不了。
func TestDreamQuest8Order(t *testing.T) {
	s := kingService()
	r := dreamHero()
	kingRun(t, r, dreamPick(t, s, r, dreamKirk, 1)) // 只接任务 9
	dreamPick(t, s, r, dreamBoss, -1)               // 缺事件 10、11 的进行中，族长不给石头

	kingRun(t, r, dreamPick(t, s, r, dreamKatan, 3))
	kingRun(t, r, dreamPick(t, s, r, dreamHotel, 2))
	kingRun(t, r, dreamPick(t, s, r, dreamBoss, 0)) // 这次拿到两块保护石
	dreamPick(t, s, r, dreamBoss, -1)               // 已有保护石，不再重复给

	blk := dreamPick(t, s, r, dreamHotel, 2) // 事件 11 未完成：命中 REQUEST（放弃问话），不还石头
	if blk.Type != "REQUEST" {
		t.Fatalf("事件 11 未完成应命中 REQUEST，实际 %s", blk.Type)
	}
	if r.eventDone(10) || countItem(r.Items, dreamGem) != 0 {
		t.Fatalf("事件 11 未完成时不应还霍特尔，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：背包满时族长给不了两块保护石，台词是 ItemFullMsg，事件不变。
func TestDreamQuest8FullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, []int{4}, []int{dreamQuest, 10, 11})
	r.Level = 30
	blk := dreamPick(t, s, r, dreamBoss, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, dreamStoneH) != 0 || len(r.Items) != len(full) {
		t.Fatalf("失败后不应给保护石，实际 %v", r.Items)
	}
}

// 数据检查：四个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 8 的完成事件是 9；
// 两块保护石由族长给、两件奖励和最终奖励都查得到；脚本引用的中间事件 10、11、12 都对得上。
func TestDreamQuest8ScriptData(t *testing.T) {
	files := map[int]string{
		dreamKirk:  "sainasu/event/event09",
		dreamHotel: "sainasu/event/event10",
		dreamKatan: "sainasu/event/event11",
		dreamBoss:  "sainasu/event/event12",
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
	if q, ok := gamedata.QuestByNo(8); !ok || q.EndEvent != dreamQuest {
		t.Fatalf("任务 8 的完成事件应是 9，实际 %+v", q)
	}
	boss, _ := gamedata.NpcEventOf(dreamBoss)
	if boss.Blocks[0].GetItem[0].ID != dreamStoneH || boss.Blocks[0].GetItem[1].ID != dreamStoneK {
		t.Errorf("族长的块 0 应给两块保护石，实际 %+v", boss.Blocks[0].GetItem)
	}
}
