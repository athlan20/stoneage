package character

// STAGE-318：猜谜3兄弟的任务 2、3（事件 44、47）端到端（纯逻辑，不连数据库）：用真实白名单、真实选块
// pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具和宠物增减、事件位变化。
// 反向用例覆盖没接任务、进行中再选、答错（宠物等级不够）、背包满、宠物栏满、完成后再来。
// 协议层（对话窗口、推送）不在这里测。脚本的数据检查见本文件末尾的 TestQuizScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	quizKui    = 1226 // 魁司（quiz/equiz_01，猜谜三兄弟的家１２楼 810）：接任务 2
	quizIna    = 1225 // 伊那多（quiz/equiz_02，810）：收入门证给金布伊；练到 77 级后再给入门证
	quizDeshi  = 1227 // 大师的弟子（quiz/equiz_04，猜谜大师认定场 812）：交谜之箱给左迪洛斯
	quizQuest2 = 44   // 任务 2 的事件号
	quizQuest3 = 47   // 任务 3 的事件号
	quizPass   = 2753 // 猜谜专家的会员证（入门证）
	quizBox    = 2759 // 谜之箱子（任务 3 的中间物）
	quizGold   = 12   // 任务 2 给的宠物模板（金布伊；脚本里 GetPet:14，怪物 14 的模板是 12）
	quizBlue   = 302  // 任务 3 给的宠物模板（左迪洛斯；脚本里 GetPet:351，怪物 351 的模板是 302）
)

// quizPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func quizPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// quizGolden 造一只任务 2 领到的金布伊（模板 12），按指定等级。
func quizGolden(level int) []petRec {
	return []petRec{{Slot: 0, TemplateID: quizGold, Name: "金布伊", Level: level}}
}

// quizFullPets 造满 5 个宠位的宠物（每格一只 1 级金布伊）。
func quizFullPets() []petRec {
	out := make([]petRec, 0, MaxPetSlots)
	for slot := 0; slot < MaxPetSlots; slot++ {
		out = append(out, petRec{Slot: slot, TemplateID: quizGold, Name: "金布伊", Level: 1})
	}
	return out
}

// 任务 2 正向：魁司接任务拿到入门证 → 伊那多收走入门证，给一只金布伊（模板 12），事件 44 完成；
// 完成后再找两人只剩闲话，不会重复给道具或宠物。
func TestQuizQuest2WalksToGolden(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	blk := quizPick(t, s, r, quizKui, 3)
	if blk.Type != "REQUEST" || blk.Event != quizQuest2 {
		t.Fatalf("接任务应命中事件 44 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(quizQuest2) || countItem(r.Items, quizPass) != 1 {
		t.Fatalf("接取后事件 44 应进行中、背包应有 2753，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	blk = quizPick(t, s, r, quizIna, 4)
	if blk.Type != "ACCEPT" || blk.Event != quizQuest2 {
		t.Fatalf("交入门证应命中事件 44 的 ACCEPT 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, quizPass) != 0 || len(r.Pets) != 1 {
		t.Fatalf("交任务后应收走 2753、给一只宠物，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
	if r.Pets[0].TemplateID != quizGold {
		t.Fatalf("任务 2 应给模板 %d（金布伊），实际 %d", quizGold, r.Pets[0].TemplateID)
	}
	if !r.eventDone(quizQuest2) || r.eventDoing(quizQuest2) {
		t.Fatalf("事件 44 应完成，实际 %v", r.EventFlags)
	}

	quizPick(t, s, r, quizKui, 2) // 完成后魁司只说闲话
	quizPick(t, s, r, quizIna, 3) // 完成后伊那多只说闲话（宠物不到 77 级也不会再给入门证）
	if countItem(r.Items, quizPass) != 0 || len(r.Pets) != 1 {
		t.Fatalf("完成后再来不应重复给道具或宠物，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
}

// 任务 3 正向：任务 2 完成后把金布伊练到 77 级 → 伊那多再给入门证（事件 47 进行中）→
// 拿谜之箱找大师的弟子，收走箱子给左迪洛斯（模板 302），事件 47 完成。
func TestQuizQuest3WalksToBlue(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{quizQuest2}, nil)
	r.Pets = quizGolden(77)
	blk := quizPick(t, s, r, quizIna, 2)
	if blk.Type != "MESSAGE" || blk.Event != quizQuest3 {
		t.Fatalf("练到 77 级后应命中事件 47 的 MESSAGE 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, quizPass) != 1 || !r.eventDoing(quizQuest3) {
		t.Fatalf("应再拿到 2753、事件 47 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	kingAddItem(&r.Items, quizBox) // 谜之箱由猜谜小游戏给（见 TestQuizScriptData）
	blk = quizPick(t, s, r, quizDeshi, 1)
	if blk.Type != "ACCEPT" || blk.Event != quizQuest3 {
		t.Fatalf("交谜之箱应命中事件 47 的 ACCEPT 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, quizBox) != 0 {
		t.Fatalf("谜之箱应被收走，实际 %v", r.Items)
	}
	if !r.eventDone(quizQuest3) || r.eventDoing(quizQuest3) {
		t.Fatalf("事件 47 应完成，实际 %v", r.EventFlags)
	}
	found := false
	for _, p := range r.Pets {
		if p.TemplateID == quizBlue {
			found = true
		}
	}
	if !found {
		t.Fatalf("任务 3 应给模板 %d（左迪洛斯），实际 %+v", quizBlue, r.Pets)
	}

	// 完成后再来：三人都是完成后的闲话，不会再给箱子或宠物。
	quizPick(t, s, r, quizKui, 0)
	quizPick(t, s, r, quizIna, 0)
	quizPick(t, s, r, quizDeshi, 0)
	if len(r.Pets) != 2 {
		t.Fatalf("完成后再来不应再给宠物，实际 %d 只", len(r.Pets))
	}
}

// 反向：任务 2 没接时找伊那多只说闲话；进行中再找魁司是放弃问话，不会重复发入门证。
func TestQuizQuest2NotStartedAndDoing(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	quizPick(t, s, r, quizIna, -1)
	if r.eventDoing(quizQuest2) || len(r.Items) != 0 {
		t.Fatalf("没接任务时伊那多不应给任何东西，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	r = kingRecord(kingItems(quizPass), nil, []int{quizQuest2})
	blk := quizPick(t, s, r, quizKui, 3)
	if blk.Type != "REQUEST" || blk.Event != quizQuest2 {
		t.Fatalf("进行中再找魁司应命中 REQUEST（放弃问话），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, quizPass) != 1 {
		t.Fatalf("进行中不应重复发入门证，实际 %v", r.Items)
	}
}

// 反向：宠物不到 77 级时伊那多不给第二张入门证（任务 3 的 PET=77-12 条件不成立）。
func TestQuizQuest3NeedsLevel77(t *testing.T) {
	s := kingService()
	for _, lv := range []int{1, 76} {
		r := kingRecord(nil, []int{quizQuest2}, nil)
		r.Pets = quizGolden(lv)
		quizPick(t, s, r, quizIna, 3) // ENDEV=44 的提示块
		if countItem(r.Items, quizPass) != 0 || r.eventDoing(quizQuest3) {
			t.Fatalf("%d 级时不应接到任务 3，实际 %v flags=%v", lv, r.Items, r.EventFlags)
		}
	}
}

// 反向：任务 3 进行中、没有谜之箱时，大师的弟子只说闲话，不会给宠物。
func TestQuizQuest3NeedsBox(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(quizPass), []int{quizQuest2}, []int{quizQuest3})
	quizPick(t, s, r, quizDeshi, -1)
	for _, p := range r.Pets {
		if p.TemplateID == quizBlue {
			t.Fatalf("没有谜之箱不应给左迪洛斯")
		}
	}
}

// 反向：背包满时接不了任务 2（入门证放不下），台词是 ItemFullMsg，事件不变。
func TestQuizQuest2FullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, nil)
	blk := quizPick(t, s, r, quizKui, 3)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(quizQuest2) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 反向：宠物栏满时伊那多交不了金布伊，台词是 PetFullMsg，入门证也不收。
func TestQuizQuest2FullPets(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(quizPass), nil, []int{quizQuest2})
	r.Pets = quizFullPets()
	blk := quizPick(t, s, r, quizIna, 4)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.PetFull || fail == "" {
		t.Fatalf("宠物栏满时应说 PetFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, quizPass) != 1 || len(r.Pets) != MaxPetSlots || !r.eventDoing(quizQuest2) {
		t.Fatalf("失败后入门证不应被收走，实际 items=%v pets=%d flags=%v", r.Items, len(r.Pets), r.EventFlags)
	}
}

// 数据检查：三个脚本对应的五个 NPC 都在白名单里、整本支持、是任务类型且在正式地图上；
// 任务 2、3 的完成事件是 44、47；道具和宠物模板都查得到；谜之箱 2759 不由任何任务脚本给出。
func TestQuizScriptData(t *testing.T) {
	npcs := map[int]string{
		1226: "quiz/equiz_01", 1230: "quiz/equiz_01",
		1225: "quiz/equiz_02", 1229: "quiz/equiz_02",
		1227: "quiz/equiz_04",
	}
	for npcID, file := range npcs {
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
	if q, ok := gamedata.QuestByNo(2); !ok || q.EndEvent != quizQuest2 {
		t.Fatalf("任务 2 的完成事件应是 44，实际 %+v", q)
	}
	if q, ok := gamedata.QuestByNo(3); !ok || q.EndEvent != quizQuest3 {
		t.Fatalf("任务 3 的完成事件应是 47，实际 %+v", q)
	}

	// 谜之箱 2759 不由任何任务脚本给出：它来自猜谜小游戏 NPC（原版 genout/quz_812_28_14，答对 100 题）。
	// 新服务端还没有实现这类 NPC，任务 3 的最后一步因此拿不到箱子（docs/quest.md 已如实记录）。
	for _, s := range gamedata.NpcEventScripts() {
		for i := range s.Blocks {
			for _, it := range s.Blocks[i].GetItem {
				if it.ID == quizBox {
					t.Errorf("%s 块 %d 给出了谜之箱 2759，测试假设已过时", s.File, i)
				}
			}
		}
	}
}

// kingAddItem 往背包的空格子里放一件道具（每件占一格），给测试准备谜之箱。
func kingAddItem(items *[]itemRec, id int) {
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

// quizApply 把一步交易的结果写回角色（测试里代替 commitEvent，不连数据库）。
func quizApply(r *record, next eventResult) {
	r.Items, r.Gold, r.Charm, r.Pets = next.items, next.gold, next.charm, next.pets
	r.Quests, r.EventFlags = stateRecs(next.prog.quests), stateRecs(next.prog.events)
}

// STAGE-332：812 猜谜大师的真实链路（纯逻辑）：任务 2 完成 → 金布伊练到 77 → 伊那多给 2753（事件 47 进行中）→
// 猜谜大师 agent 直接认定（扣 2753、发 2759）→ 大师的弟子收箱子给左迪洛斯，任务 3 完成 → 再找大师只说没有入门证。
func TestQuizMasterChainAgent(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	kingRun(t, r, quizPick(t, s, r, quizKui, 3)) // 魁司接任务 2，拿 2753
	kingRun(t, r, quizPick(t, s, r, quizIna, 4)) // 伊那多收 2753，给金布伊，事件 44 完成
	if !r.eventDone(quizQuest2) {
		t.Fatalf("任务 2 应完成，实际 %v", r.EventFlags)
	}
	r.Pets = quizGolden(77)
	kingRun(t, r, quizPick(t, s, r, quizIna, 2)) // 练到 77 级，伊那多再给 2753（事件 47 进行中）
	if countItem(r.Items, quizPass) != 1 || !r.eventDoing(quizQuest3) {
		t.Fatalf("应有 2753、事件 47 进行中，实际 %v flags=%v", r.Items, r.EventFlags)
	}

	out := quizAgentPlan(r, quizMasterCfg(t))
	if out.kind != qaGrant {
		t.Fatalf("大师应认定通过，实际 kind=%d msg=%q", out.kind, out.msg)
	}
	quizApply(r, out.next)
	if countItem(r.Items, quizPass) != 0 || countItem(r.Items, quizBoxItem) != 1 {
		t.Fatalf("认定后应扣 2753、有 2759，实际 %v", r.Items)
	}
	if !r.eventDoing(quizQuest3) {
		t.Fatal("认定本身不应完成任务 3，任务 3 由弟子完成")
	}

	blk := quizPick(t, s, r, quizDeshi, 1) // 大师的弟子收谜之箱，给左迪洛斯
	if blk.Type != "ACCEPT" || blk.Event != quizQuest3 {
		t.Fatalf("交箱子应命中事件 47 的 ACCEPT 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, quizBoxItem) != 0 || !r.eventDone(quizQuest3) {
		t.Fatalf("箱子应被收走、事件 47 完成，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	blue := false
	for _, p := range r.Pets {
		blue = blue || p.TemplateID == quizBlue
	}
	if !blue {
		t.Fatalf("应得到左迪洛斯，实际 %+v", r.Pets)
	}

	// 完成后再找大师：没有入门证，原版 NoEntryMsg，不再发箱子。
	if out := quizAgentPlan(r, quizMasterCfg(t)); out.kind != qaNoEntry {
		t.Fatalf("完成后再找大师应没有入门证，实际 kind=%d", out.kind)
	}
}
