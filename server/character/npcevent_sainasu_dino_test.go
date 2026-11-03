package character

// STAGE-315（第 1 组）：恐龙博士的任务 6、7（事件 15、16）和强盗洞窟的任务 10（事件 13）端到端
// （纯逻辑，不连数据库）：用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言命中的块、
// 道具和宠物增减、事件位变化。反向用例覆盖等级不够、宠物等级不够、丢了足迹、道具不齐、背包满、完成后再来。
// 协议层（对话窗口、推送）不在这里测。脚本的数据检查见本文件末尾的 TestSainasuDinoScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	dinoHaru      = 1356 // 恐龙博士哈鲁（sainasu/event/event15_1，11006）：任务 6、7
	banditSaton   = 1244 // 老人萨顿（sainasu/event/event13_1，1112）：任务 10
	dinoQuest6    = 15   // 任务 6 的事件号（抓鲁尼帖斯）
	dinoQuest7    = 16   // 任务 7 的事件号（抓贝鲁卡）
	banditQuest   = 13   // 任务 10 的事件号
	dinoRuni      = 74   // 鲁尼帖斯的宠物模板（脚本 PET>14-74）
	dinoBeruka    = 191  // 贝鲁卡的宠物模板（脚本 PET>29-191）
	dinoReward    = 1350 // 任务 6 的随机奖励里 kingRand 取到的第一件
	dinoPetReward = 173  // 任务 7 的奖励宠物模板（脚本 GetPet:95，怪物 95 的模板是 173）
	banditTrace   = 2431 // 盗贼们的足迹
	banditCloth   = 2428 // 萨顿家的衣服（怪物 513 掉落）
	banditAxe     = 2429 // 萨顿家的斧头（怪物 514 掉落）
	banditHelm    = 2430 // 萨顿家的头盔（怪物 515 掉落）
	banditCure    = 2450 // 萨顿家的秘药
)

// sainasuPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func sainasuPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// sainasuFullBag 把背包占满（每格一件 2417，只占格子）。
func sainasuFullBag() []itemRec {
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	return full
}

// 任务 6 正向：哈鲁接任务（Lv10）→ 交一只 15 级鲁尼帖斯 → 收走宠物、给随机奖励 1350，事件 15 完成。
func TestDinoQuest6Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 10
	blk := sainasuPick(t, s, r, dinoHaru, 2)
	if blk.Type != "REQUEST" || blk.Event != dinoQuest6 {
		t.Fatalf("接任务应命中事件 15 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(dinoQuest6) {
		t.Fatalf("接取后事件 15 应进行中，实际 %v", r.EventFlags)
	}

	r.Pets = []petRec{{Slot: 0, TemplateID: dinoRuni, Name: "鲁尼帖斯", Level: 15}}
	kingRun(t, r, sainasuPick(t, s, r, dinoHaru, 0))
	if len(r.Pets) != 0 || countItem(r.Items, dinoReward) != 1 {
		t.Fatalf("交鲁尼帖斯应收走宠物、给奖励 %d，实际 items=%v pets=%d", dinoReward, r.Items, len(r.Pets))
	}
	if !r.eventDone(dinoQuest6) || r.eventDoing(dinoQuest6) {
		t.Fatalf("事件 15 应完成，实际 %v", r.EventFlags)
	}
	sainasuPick(t, s, r, dinoHaru, -1) // 完成后没有块命中，只说闲聊
}

// 任务 7 正向：任务 6 完成后（Lv16）接任务 16 → 交一只 30 级贝鲁卡 → 收走宠物、给宠物模板 173，事件 16 完成。
func TestDinoQuest7Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{dinoQuest6}, nil)
	r.Level = 16
	blk := sainasuPick(t, s, r, dinoHaru, 3)
	if blk.Type != "REQUEST" || blk.Event != dinoQuest7 {
		t.Fatalf("接任务应命中事件 16 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(dinoQuest7) {
		t.Fatalf("接取后事件 16 应进行中，实际 %v", r.EventFlags)
	}

	r.Pets = []petRec{{Slot: 0, TemplateID: dinoBeruka, Name: "贝鲁卡", Level: 30}}
	kingRun(t, r, sainasuPick(t, s, r, dinoHaru, 1))
	found := false
	for _, p := range r.Pets {
		if p.TemplateID == dinoPetReward {
			found = true
		}
	}
	if !found || !r.eventDone(dinoQuest7) {
		t.Fatalf("交贝鲁卡应换到宠物模板 %d、事件 16 完成，实际 pets=%+v flags=%v", dinoPetReward, r.Pets, r.EventFlags)
	}
}

// 反向：不到 10 级时哈鲁只有闲聊（-1）；宠物不到 15 级时交不了任务 6（命中 REQUEST 的放弃问话）。
func TestDinoLevelGates(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 9
	sainasuPick(t, s, r, dinoHaru, -1)
	if r.eventDoing(dinoQuest6) || len(r.Items) != 0 {
		t.Fatalf("9 级不应接到任务 6，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	r = kingRecord(nil, nil, []int{dinoQuest6})
	r.Level = 10
	r.Pets = []petRec{{Slot: 0, TemplateID: dinoRuni, Name: "鲁尼帖斯", Level: 14}}
	blk := sainasuPick(t, s, r, dinoHaru, 2) // 条件 PET>14，14 级不够
	if blk.Type != "REQUEST" {
		t.Fatalf("宠物等级不够应命中 REQUEST（放弃问话），实际 %s", blk.Type)
	}
	if len(r.Items) != 0 || len(r.Pets) != 1 {
		t.Fatalf("等级不够不应给奖励或收宠物，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
}

// 反向：背包满时任务 6 的随机奖励放不下，台词是 ItemFullMsg，宠物不收走、事件不变。
func TestDinoFullBagChangesNothing(t *testing.T) {
	r := kingRecord(sainasuFullBag(), nil, []int{dinoQuest6})
	r.Level = 10
	r.Pets = []petRec{{Slot: 0, TemplateID: dinoRuni, Name: "鲁尼帖斯", Level: 15}}
	s := kingService()
	blk := sainasuPick(t, s, r, dinoHaru, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if len(r.Pets) != 1 || len(r.Items) != len(sainasuFullBag()) || !r.eventDoing(dinoQuest6) {
		t.Fatalf("失败后宠物不应被收走，实际 pets=%d items=%d flags=%v", len(r.Pets), len(r.Items), r.EventFlags)
	}
}

// 任务 10 正向：萨顿接任务拿到足迹 → 打盗贼凑齐三件传家宝 → 交四件换秘药 2450，事件 13 完成。
func TestBanditQuest10Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Level = 16
	blk := sainasuPick(t, s, r, banditSaton, 2)
	if blk.Type != "REQUEST" || blk.Event != banditQuest {
		t.Fatalf("接任务应命中事件 13 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(banditQuest) || countItem(r.Items, banditTrace) != 1 {
		t.Fatalf("接取后事件 13 应进行中、背包应有足迹，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	sainasuAdd(&r.Items, banditCloth) // 怪物 513 掉落
	sainasuAdd(&r.Items, banditAxe)   // 怪物 514 掉落
	sainasuAdd(&r.Items, banditHelm)  // 怪物 515 掉落
	kingRun(t, r, sainasuPick(t, s, r, banditSaton, 0))
	if countItem(r.Items, banditCure) != 1 || !r.eventDone(banditQuest) {
		t.Fatalf("交三件传家宝应拿到 2450、事件 13 完成，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	for _, id := range []int{banditCloth, banditAxe, banditHelm, banditTrace} {
		if countItem(r.Items, id) != 0 {
			t.Fatalf("道具 %d 应被收走，实际 %v", id, r.Items)
		}
	}
	sainasuPick(t, s, r, banditSaton, 3) // 完成后只说闲话
}

// 反向：任务进行中把足迹弄丢了，萨顿会再给一枚（块 1）；三件传家宝不齐时交不了任务。
func TestBanditLostTrace(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, []int{banditQuest})
	r.Level = 16
	kingRun(t, r, sainasuPick(t, s, r, banditSaton, 1))
	if countItem(r.Items, banditTrace) != 1 {
		t.Fatalf("丢了足迹应能补一枚，实际 %v", r.Items)
	}
	sainasuPick(t, s, r, banditSaton, 2) // 有足迹时不再给（ITEM!=2431 不成立），命中 REQUEST（放弃问话）
	if countItem(r.Items, banditTrace) != 1 {
		t.Fatalf("已有足迹不应再给，实际 %v", r.Items)
	}

	sainasuAdd(&r.Items, banditCloth)
	sainasuAdd(&r.Items, banditAxe)             // 少头盔
	blk := sainasuPick(t, s, r, banditSaton, 2) // 三件不齐、足迹还在：命中 REQUEST（放弃问话）
	if blk.Type != "REQUEST" {
		t.Fatalf("三件不齐应命中 REQUEST，实际 %s", blk.Type)
	}
	if r.eventDone(banditQuest) || countItem(r.Items, banditCure) != 0 {
		t.Fatalf("三件不齐不应完成任务，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：背包满时接任务拿不到足迹（块 2 的 ItemFullMsg），事件不进入进行中。
func TestBanditFullBag(t *testing.T) {
	s := kingService()
	r := kingRecord(sainasuFullBag(), nil, nil)
	r.Level = 16
	blk := sainasuPick(t, s, r, banditSaton, 2)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(banditQuest) || len(r.Items) != len(sainasuFullBag()) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 数据检查：两个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 6、7、10 的完成事件对得上；
// 道具和宠物模板都查得到，三件传家宝有怪物掉落来源。
func TestSainasuDinoScriptData(t *testing.T) {
	files := map[int]string{
		dinoHaru:    "sainasu/event/event15_1",
		banditSaton: "sainasu/event/event13_1",
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
			for _, enemy := range b.GetPet {
				e, ok := gamedata.EnemyByID(enemy)
				if !ok {
					t.Errorf("%s 块 %d 的怪物 %d 不在敌人表里", script.File, i, enemy)
					continue
				}
				if _, ok := gamedata.PetByID(e.Template); !ok {
					t.Errorf("%s 块 %d 的怪物 %d 的宠物模板 %d 不在宠物表里", script.File, i, enemy, e.Template)
				}
			}
		}
	}
	for no, event := range map[int]int{6: dinoQuest6, 7: dinoQuest7, 10: banditQuest} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	// 三件传家宝由柯尔克山洞窟的盗贼 100% 掉落，玩家打得到。
	drops := map[int]int{513: banditCloth, 514: banditAxe, 515: banditHelm}
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
}

// sainasuAdd 往背包的空格子里放一件道具（每件占一格），给测试准备打怪掉落。
func sainasuAdd(items *[]itemRec, id int) {
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
