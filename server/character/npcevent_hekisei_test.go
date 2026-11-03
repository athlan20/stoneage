package character

// STAGE-314：碧青洞窟的任务 35（事件 46）端到端（纯逻辑，不连数据库）：用真实白名单、真实选块
// pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具增减、事件位变化。
// 反向用例覆盖等级不够、前置没做、没有水晶、背包满、完成后再来，以及原版的重复合成行为。
// 协议层（对话窗口、推送）不在这里测。脚本的数据检查见本文件末尾的 TestHekiseiScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	hekiMessenger = 1289 // 水的使者（jaruga/event/hekisei，卡鲁它那村 3400）：接任务 35
	hekiGoddess   = 1408 // 女神艾妮基（jaruga/event/hekisei01，碧青洞窟地下15楼 21215）：讲详情、交水晶换戒指
	hekiSage      = 1409 // 贤者（jaruga/event/hekisei03，水苍的洞窟地下8楼 21308）：石头 + 水合成碧青水晶
	hekiFei       = 1438 // 菲伊（jaruga/event/hekisei04，吉鲁的海底通道 31302）：给群青之水

	hekiQuest   = 46 // 任务 35 的事件号
	hekiQuestNo = 35
	hekiAdult   = 4    // 成人仪式的完成事件
	hekiRuri    = 39   // 任务 36（琉璃洞窟）的完成事件，和水晶任务的前置之一
	hekiShell   = 2766 // 水苍之贝（怪物 1334/1335 掉落）
	hekiStone   = 2767 // 深蓝色石头（怪物 1324 掉落）
	hekiWater   = 2768 // 群青的水（菲伊给）
	hekiCrystal = 2769 // 碧青水晶（贤者合成）
	hekiRing    = 2770 // 水的戒指（艾妮基给的奖励）
)

// hekiPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func hekiPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// hekiHero 是 65 级的勇者小明：成人仪式和琉璃洞窟（36）都已完成，满足任务 35 的前置。
func hekiHero() *record {
	r := kingRecord(nil, []int{hekiAdult, hekiRuri}, nil)
	r.Level = 65
	return r
}

// 任务 35 正向：水的使者接任务 → 艾妮基听详情 → 菲伊给群青之水 → 打怪凑深蓝色石头 → 贤者合成水晶 →
// 艾妮基交水晶拿水的戒指（魅力 +1），事件 46 完成。
func TestHekiseiQuest35Walks(t *testing.T) {
	s := kingService()
	r := hekiHero()
	blk := hekiPick(t, s, r, hekiMessenger, 0)
	if blk.Type != "REQUEST" || blk.Event != hekiQuest {
		t.Fatalf("接任务应命中事件 46 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(hekiQuest) {
		t.Fatalf("接取后事件 46 应进行中，实际 %v", r.EventFlags)
	}

	blk = hekiPick(t, s, r, hekiGoddess, 2)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("第一次找艾妮基应听详情（ACCEPT，事件 -1），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if len(r.Items) != 0 {
		t.Fatalf("听详情不应改背包，实际 %v", r.Items)
	}

	hekiAdd(&r.Items, hekiShell) // 水苍之贝：怪物 1334/1335 掉落
	hekiAdd(&r.Items, hekiStone) // 深蓝色石头：怪物 1324 掉落

	kingRun(t, r, hekiPick(t, s, r, hekiFei, 1))
	if countItem(r.Items, hekiWater) != 1 {
		t.Fatalf("菲伊应给群青之水，实际 %v", r.Items)
	}
	kingRun(t, r, hekiPick(t, s, r, hekiSage, 1))
	if countItem(r.Items, hekiCrystal) != 1 || countItem(r.Items, hekiStone) != 0 || countItem(r.Items, hekiWater) != 0 {
		t.Fatalf("贤者应收走石头和水、给碧青水晶，实际 %v", r.Items)
	}

	blk = hekiPick(t, s, r, hekiGoddess, 0)
	if blk.Type != "ACCEPT" || blk.Event != hekiQuest {
		t.Fatalf("交水晶应命中事件 46 的 ACCEPT 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if blk.Charm != 1 {
		t.Errorf("交水晶块应加 1 点魅力（原版 Charm:1），实际 %d", blk.Charm)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, hekiRing) != 1 || countItem(r.Items, hekiCrystal) != 0 {
		t.Fatalf("交水晶后应拿到 2770、收走 2769，实际 %v", r.Items)
	}
	if !r.eventDone(hekiQuest) || r.eventDoing(hekiQuest) {
		t.Fatalf("事件 46 应完成，实际 %v", r.EventFlags)
	}

	// 完成后再来：艾妮基只说完成后的闲话，不会再收水晶或发戒指。
	blk = hekiPick(t, s, r, hekiGoddess, 1)
	if blk.Type != "MESSAGE" || blk.Event != -1 {
		t.Fatalf("完成后再来应命中“闯过洞窟”的闲话块（身上有水苍之贝），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, hekiRing) != 1 {
		t.Fatalf("完成后再来不应重复发戒指，实际 %v", r.Items)
	}
}

// 反向：等级不到 65（脚本条件是 LV>64）时，水的使者只说闲话，接不到任务。
func TestHekiseiLevelGate(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{hekiAdult, hekiRuri}, nil)
	r.Level = 64
	hekiPick(t, s, r, hekiMessenger, 4)
	if r.eventDoing(hekiQuest) {
		t.Fatalf("64 级不应接到任务 35，实际 %v", r.EventFlags)
	}
	r.Level = 65
	hekiPick(t, s, r, hekiMessenger, 0)
}

// 反向：只完成成人仪式、没完成琉璃/深红/玄黄之一时，水的使者没有块命中（三个 REQUEST 都要对应的 ENDEV）。
func TestHekiseiNeedsOneOfThree(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{hekiAdult}, nil)
	r.Level = 65
	hekiPick(t, s, r, hekiMessenger, 4)
	if r.eventDoing(hekiQuest) {
		t.Fatalf("没完成三洞之一不应接到任务 35，实际 %v", r.EventFlags)
	}
	// 深红（事件 40）和玄黄（事件 42）也各是一条入口。
	// 三个 REQUEST 块按琉璃（39）、深红（40）、玄黄（42）的顺序排列。
	for done, want := range map[int]int{39: 0, 40: 1, 42: 2} {
		r := kingRecord(nil, []int{hekiAdult, done}, nil)
		r.Level = 65
		hekiPick(t, s, r, hekiMessenger, want)
	}
}

// 反向：任务进行中、没有水晶也没有水苍之贝时，艾妮基讲详情（块 2 的兜底候选只要 NOWEV=46）；
// 有水晶时才交任务，有贝壳但没有水晶时只说闯过洞窟的话。
func TestHekiseiGoddessBranches(t *testing.T) {
	s := kingService()
	r := hekiHero()
	kingRun(t, r, hekiPick(t, s, r, hekiMessenger, 0))
	blk := hekiPick(t, s, r, hekiGoddess, 2)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("没有水晶时艾妮基应讲详情，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	hekiAdd(&r.Items, hekiShell)
	blk = hekiPick(t, s, r, hekiGoddess, 1)
	if blk.Type != "MESSAGE" {
		t.Fatalf("只有水苍之贝时应只说闯过洞窟，实际 %s", blk.Type)
	}
	if countItem(r.Items, hekiRing) != 0 {
		t.Fatalf("没有水晶不应发戒指，实际 %v", r.Items)
	}
}

// 反向：背包满时菲伊给不了群青之水，台词是 ItemFullMsg，事件不变。
// 交水晶和合成水晶都会先收走道具腾出格子，满包也能做成，所以不拿它们测失败。
func TestHekiseiFullBagChangesNothing(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, []int{hekiAdult, hekiRuri}, []int{hekiQuest})
	r.Level = 65
	blk := hekiPick(t, s, r, hekiFei, 1)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, hekiWater) != 0 || len(r.Items) != len(full) || !r.eventDoing(hekiQuest) {
		t.Fatalf("失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 原版行为：任务进行中，只要再凑齐深蓝色石头和群青之水，贤者就会再做一颗碧青水晶（已有一颗也照做）。
// 第二候选 LV>64&NOWEV=46&ITEM!=2769 还允许空手补一颗（丢了水晶的兜底）。照搬，测试固定这一行为。
func TestHekiseiSageRepeatablePerOriginal(t *testing.T) {
	s := kingService()
	r := hekiHero()
	kingRun(t, r, hekiPick(t, s, r, hekiMessenger, 0))
	hekiAdd(&r.Items, hekiStone)
	hekiAdd(&r.Items, hekiWater)
	kingRun(t, r, hekiPick(t, s, r, hekiSage, 1))
	if countItem(r.Items, hekiCrystal) != 1 {
		t.Fatalf("第一次合成应给一颗水晶，实际 %v", r.Items)
	}
	hekiAdd(&r.Items, hekiStone)
	hekiAdd(&r.Items, hekiWater)
	kingRun(t, r, hekiPick(t, s, r, hekiSage, 1))
	if countItem(r.Items, hekiCrystal) != 2 {
		t.Fatalf("原版允许再合成一颗，实际 %v", r.Items)
	}
}

// 数据检查：hekisei01 在白名单里、整本支持；接任务的使者和交任务的艾妮基都是任务类型、在正式地图上；
// 任务 35 的完成事件是 46；道具都有来源（贝壳和石头是怪物掉落，水是菲伊给，水晶是贤者给），水的戒指是奖励。
func TestHekiseiScriptData(t *testing.T) {
	script, ok := gamedata.NpcEventOf(hekiGoddess)
	if !ok || script.File != "jaruga/event/hekisei01" {
		t.Fatalf("NPC %d 应对应 jaruga/event/hekisei01，实际 %+v ok=%v", hekiGoddess, script, ok)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Errorf("%s 应已放开", script.File)
	}
	for _, npcID := range []int{hekiMessenger, hekiGoddess, hekiSage, hekiFei} {
		npc, ok := gamedata.NPCByID(npcID)
		if !ok || npc.Kind != "changeevent" || !gamedata.FormalFloor(npc.Floor) {
			t.Errorf("NPC %d 应是任务类型、在正式地图上，实际 %+v", npcID, npc)
		}
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
	q, ok := gamedata.QuestByNo(hekiQuestNo)
	if !ok || q.EndEvent != hekiQuest {
		t.Fatalf("任务 %d 的完成事件应是 %d，实际 %+v", hekiQuestNo, hekiQuest, q)
	}

	// 前置道具的来源：2766 和 2767 是怪物掉落，2768 由菲伊给，2769 由贤者合成，2770 是艾妮基的奖励。
	drops := map[int]int{1334: hekiShell, 1335: hekiShell, 1324: hekiStone}
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
			t.Errorf("怪物 %d 应掉落道具 %d，实际掉落 %+v", enemy, item, e.Drops)
		}
	}
	if fei, _ := gamedata.NpcEventOf(hekiFei); fei.Blocks[1].GetItem[0].ID != hekiWater {
		t.Errorf("菲伊的块 1 应给群青之水，实际 %+v", fei.Blocks[1].GetItem)
	}
	if sage, _ := gamedata.NpcEventOf(hekiSage); sage.Blocks[1].GetItem[0].ID != hekiCrystal {
		t.Errorf("贤者的块 1 应给碧青水晶，实际 %+v", sage.Blocks[1].GetItem)
	}
	if script.Blocks[0].GetItem[0].ID != hekiRing {
		t.Errorf("艾妮基的交任务块应给水的戒指，实际 %+v", script.Blocks[0].GetItem)
	}
}

// hekiAdd 往背包的空格子里放一件道具（每件占一格），给测试准备打怪掉落和任务道具。
func hekiAdd(items *[]itemRec, id int) {
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
