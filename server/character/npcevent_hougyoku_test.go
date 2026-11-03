package character

// STAGE-319：四宝玉之迷（任务 31，事件 45）端到端（纯逻辑，不连数据库）：用真实白名单、真实选块
// pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具增减、事件位变化。
// 反向用例覆盖等级不够、没接任务就凑齐宝玉、宝玉不齐、背包满、完成后再来。协议层（对话窗口、推送）不在这里测。
// 脚本的数据检查（白名单、整本支持、NPC 类型和地图、道具来源、NpcWarp）见本文件末尾的 TestHougyokuScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	hougyokuNPC     = 1333 // 村子的学者（extra/event/hougyoku，奇努伊村 6000）
	hougyokuQuest   = 45   // 任务 31 的事件号
	hougyokuQuestNo = 31
	hougyokuCrystal = 2764 // 结晶石（接任务时给，交任务时收走）
	hougyokuReward  = 2765 // 咒术的手镯（完成奖励）
)

// hougyokuOrbs 是四个宝玉（地、水、火、风），由怪物掉落，见 TestHougyokuScriptData。
var hougyokuOrbs = []int{2760, 2761, 2762, 2763}

// hougyokuPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func hougyokuPick(t *testing.T, s *Service, r *record, want int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(hougyokuNPC)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", hougyokuNPC)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Fatalf("%s 不在白名单里，NPC %d 对话会说“还不能对话”", script.File, hougyokuNPC)
	}
	return kingPick(t, s, r, hougyokuNPC, want)
}

// hougyokuHero 是 60 级的勇者小明（任务 31 要求 Lv>59），空背包。
func hougyokuHero() *record {
	r := kingRecord(nil, nil, nil)
	r.Level = 60
	return r
}

// hougyokuAdd 往背包的空格子里放道具（每件占一格），模拟打怪掉落和任务给的结晶石。
func hougyokuAdd(r *record, ids ...int) {
	used := make(map[int]bool, len(r.Items))
	for _, it := range r.Items {
		used[it.Slot] = true
	}
	slot := firstBagSlot
	for _, id := range ids {
		for used[slot] {
			slot++
		}
		r.Items = append(r.Items, itemRec{Slot: slot, ItemID: id, Count: 1})
		used[slot] = true
	}
}

// 任务 31 正向：学者接取（拿到结晶石 2764）→ 打齐四个宝玉 → 交任务，四个宝玉和 2764 被收走，
// 拿到 2765，事件 45 完成；完成后再对话只说闲话。
func TestHougyokuQuest31Walks(t *testing.T) {
	s := kingService()
	r := hougyokuHero()
	blk := hougyokuPick(t, s, r, 1)
	if blk.Type != "REQUEST" || blk.Event != hougyokuQuest {
		t.Fatalf("接任务应命中事件 45 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(hougyokuQuest) || countItem(r.Items, hougyokuCrystal) != 1 {
		t.Fatalf("接取后事件 45 应进行中、背包应有 2764，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	hougyokuAdd(r, hougyokuOrbs...)
	blk = hougyokuPick(t, s, r, 0)
	if blk.Type != "ACCEPT" {
		t.Fatalf("凑齐宝玉后应交任务（ACCEPT），实际 %s", blk.Type)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, hougyokuReward) != 1 || !r.eventDone(hougyokuQuest) || r.eventDoing(hougyokuQuest) {
		t.Fatalf("交任务后应拿到 2765、事件 45 完成，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	for _, id := range append([]int{hougyokuCrystal}, hougyokuOrbs...) {
		if countItem(r.Items, id) != 0 {
			t.Fatalf("交任务后道具 %d 应被收走，实际 %v", id, r.Items)
		}
	}

	blk = hougyokuPick(t, s, r, 2) // 完成后命中 ENDEV=45 的闲聊块
	if blk.Type != "MESSAGE" || blk.Event != -1 {
		t.Fatalf("完成后再来应只说闲话，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, hougyokuReward) != 1 {
		t.Fatalf("完成后再来不应再发奖励，实际 %v", r.Items)
	}
}

// 反向：等级不到 60（脚本条件是 LV>59）时只命中 LV>0 的闲聊块，不接任务、不给结晶石。
func TestHougyokuLevelGate(t *testing.T) {
	s := kingService()
	for _, lv := range []int{1, 59} {
		r := kingRecord(nil, nil, nil)
		r.Level = lv
		blk := hougyokuPick(t, s, r, 3)
		if blk.Type != "MESSAGE" || blk.Event != -1 {
			t.Fatalf("%d 级应命中闲聊块，实际 %s 事件 %d", lv, blk.Type, blk.Event)
		}
		if r.eventDoing(hougyokuQuest) || len(r.Items) != 0 {
			t.Fatalf("%d 级不应接到任务，实际 %v flags=%v", lv, r.Items, r.EventFlags)
		}
	}
}

// 反向：没接任务（没有 2764）时即使身上有四个宝玉也不能交任务；对话命中 REQUEST（接任务），先给结晶石。
func TestHougyokuNeedsCrystal(t *testing.T) {
	s := kingService()
	r := hougyokuHero()
	hougyokuAdd(r, hougyokuOrbs...)
	blk := hougyokuPick(t, s, r, 1)
	if blk.Type != "REQUEST" {
		t.Fatalf("没有 2764 时应先接任务，实际 %s", blk.Type)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, hougyokuCrystal) != 1 || !r.eventDoing(hougyokuQuest) {
		t.Fatalf("接任务应给 2764 并进入进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：任务进行中、少一个宝玉时对话命中 REQUEST（原版的放弃问话），不会完成、不会发奖励。
func TestHougyokuIncompleteOrbs(t *testing.T) {
	s := kingService()
	r := hougyokuHero()
	kingRun(t, r, hougyokuPick(t, s, r, 1))
	hougyokuAdd(r, hougyokuOrbs[:3]...)
	blk := hougyokuPick(t, s, r, 1)
	if blk.Type != "REQUEST" || blk.Event != hougyokuQuest {
		t.Fatalf("宝玉不齐应命中事件 45 的 REQUEST（放弃问话），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, hougyokuReward) != 0 || r.eventDone(hougyokuQuest) || !r.eventDoing(hougyokuQuest) {
		t.Fatalf("宝玉不齐不应完成任务，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：背包满时接任务给不了 2764，交易不成立、台词是 ItemFullMsg，事件不进入进行中。
func TestHougyokuFullBagChangesNothing(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, nil)
	r.Level = 60
	blk := hougyokuPick(t, s, r, 1)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, hougyokuCrystal) != 0 || r.eventDoing(hougyokuQuest) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 反向：任务 45 已完成时，即使背包里又凑齐 2764 和四个宝玉，也不会重复交任务、不会重复拿 2765。
func TestHougyokuDoneDoesNotRepeat(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{hougyokuQuest}, nil)
	r.Level = 60
	hougyokuAdd(r, hougyokuCrystal)
	hougyokuAdd(r, hougyokuOrbs...)
	blk := hougyokuPick(t, s, r, 2)
	if blk.Type != "MESSAGE" || blk.Event != -1 {
		t.Fatalf("已完成后应命中闲聊块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, hougyokuReward) != 0 || countItem(r.Items, hougyokuCrystal) != 1 {
		t.Fatalf("已完成后不应再发 2765、也不应收走 2764，实际 %v", r.Items)
	}
}

// 数据检查：hougyoku 在白名单里、整本支持；NPC 是任务类型且在正式地图（奇努伊村 6000）上，
// NPC 名字和交任务后的移动点对得上；任务 31 的完成事件是 45；用到的道具都查得到，四个宝玉有怪物掉落来源。
func TestHougyokuScriptData(t *testing.T) {
	script, ok := gamedata.NpcEventOf(hougyokuNPC)
	if !ok || script.File != "extra/event/hougyoku" {
		t.Fatalf("NPC %d 应对应 extra/event/hougyoku，实际 %+v ok=%v", hougyokuNPC, script, ok)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Errorf("%s 应已放开", script.File)
	}
	npc, ok := gamedata.NPCByID(hougyokuNPC)
	if !ok || npc.Kind != "changeevent" || npc.Floor != 6000 || !gamedata.FormalFloor(npc.Floor) {
		t.Errorf("NPC %d 应是奇努伊村（6000，正式地图）的任务 NPC，实际 %+v", hougyokuNPC, npc)
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
	q, ok := gamedata.QuestByNo(hougyokuQuestNo)
	if !ok || q.EndEvent != hougyokuQuest {
		t.Fatalf("任务 %d 的完成事件应是 %d，实际 %+v", hougyokuQuestNo, hougyokuQuest, q)
	}

	// 四个宝玉由 90 级的四种恐龙掉落（enemies.tsv 的掉落槽位），玩家打得到，不是只能靠管理命令。
	drops := map[int]int{1305: 2760, 1306: 2761, 1308: 2762, 1307: 2763}
	for enemy, item := range drops {
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
			t.Errorf("怪物 %d 应掉落宝玉 %d，实际掉落 %+v", enemy, item, e.Drops)
		}
	}

	// 交任务后 NPC 按 NpcWarp 在四个村子之间移动（最后回到奇努伊村）；顺序与原版一致。
	want := []gamedata.WarpPoint{{Floor: 4000, X: 53, Y: 101}, {Floor: 2000, X: 44, Y: 39}, {Floor: 5105, X: 14, Y: 3}, {Floor: 6000, X: 58, Y: 44}}
	if got := script.Blocks[0].Warp; len(got) != len(want) {
		t.Fatalf("交任务块应有 %d 个移动点，实际 %+v", len(want), got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("第 %d 个移动点应是 %+v，实际 %+v", i, want[i], got[i])
			}
		}
	}
	if npc.X != 58 || npc.Y != 44 {
		t.Errorf("NPC 初始位置应是 (58,44)，实际 (%d,%d)", npc.X, npc.Y)
	}
}
