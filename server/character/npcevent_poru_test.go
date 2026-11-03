package character

// STAGE-317：红暴任务 40（事件 63）、四圣石任务 42（事件 65）、44（事件 66）和古瓜任务 68（也以事件 65 完成）
// 端到端（纯逻辑，不连数据库）：用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言
// 命中的块、道具和宠物增减、事件位变化。反向用例覆盖前置没做、背包满、宠物栏满、完成后再来，
// 以及任务 42 和 68 共用事件 65 的互相影响。协议层（对话窗口、推送）不在这里测。
// 任务 41 的交付（poru/mary01）在 STAGE-339 放开，端到端见 npcevent_stage339_test.go；任务 43 的卡点见 TestPoruUnreachableTasks 和 docs/quest.md。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	poruGrandpa = 1308 // 走失孙女的老爷爷（poru/boy02，5500）：任务 40 接取
	poruDragon  = 1315 // 龙王（poru/boy04，5511）：任务 40 交任务、给龙王的守护
	poruMarina  = 1317 // 玛丽娜丝（poru/mary，5530）：任务 41 接取（四圣石收集）
	poruSquad   = 1318 // 小队长（poru/mary04，5532）：任务 44 给鲁乌
	poruTitan   = 1320 // 泰坦队长（poru/team01，5532）：任务 42、44 接取
	poruCourier = 1222 // 泰坦队队员（poru/titan，500）：任务 42 收公文换通行证
	poruSam     = 1220 // 萨姆吉尔（poru/mary02，500）：给两封公文（已在白名单）

	poruQ40 = 63 // 任务 40 的完成事件
	poruQ41 = 64 // 任务 41 的完成事件（四圣石接任务/收集）
	poruQ42 = 65 // 任务 42 和 68 共用的完成事件
	poruQ44 = 66 // 任务 44 的完成事件
	poruQ43 = 55 // 任务 43 的完成事件（练鲁乌后交鉴定书）

	poruGuard   = 18544 // 龙王的守护（任务 40 完成后龙王给两颗）
	poruLetterA = 18551 // 给米兰达队长的公文
	poruLetterB = 18552 // 给爱丽儿队长的公文
	poruPass    = 18553 // 泰坦部队的通行证
	poruStone   = 18556 // 岳之圣石（泰坦队长给，任务 43 的奖励）
	poruCert    = 2474  // 采石 定书(超特级)
	poruPet40   = 303   // 任务 40 给的宠物模板（脚本 GetPet:352，怪物 352 的模板是 303）
	poruPet44   = 720   // 任务 44 给的鲁乌模板（脚本 GetPet:1483，怪物 1483 的模板是 720）
)

// poruPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func poruPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// poruQuestState 取任务 no 的进度（-1 表示没有记录），用来区分共用事件 65 的任务 42 和 68。
func poruQuestState(r *record, no int) int {
	for _, q := range r.Quests {
		if q.No == no {
			return q.State
		}
	}
	return -1
}

// 任务 40 正向：老爷爷接任务 → 龙王交任务给宠物（模板 303），事件 63 完成 → 再找龙王拿两颗龙王的守护。
func TestPoruQuest40Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	blk := poruPick(t, s, r, poruGrandpa, 3)
	if blk.Type != "REQUEST" || blk.Event != poruQ40 {
		t.Fatalf("接任务应命中事件 63 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(poruQ40) {
		t.Fatalf("接取后事件 63 应进行中，实际 %v", r.EventFlags)
	}

	kingRun(t, r, poruPick(t, s, r, poruDragon, 1))
	found := false
	for _, p := range r.Pets {
		if p.TemplateID == poruPet40 {
			found = true
		}
	}
	if !found || !r.eventDone(poruQ40) {
		t.Fatalf("交任务应拿到宠物模板 %d、事件 63 完成，实际 pets=%+v flags=%v", poruPet40, r.Pets, r.EventFlags)
	}

	kingRun(t, r, poruPick(t, s, r, poruDragon, 0))
	if countItem(r.Items, poruGuard) != 2 {
		t.Fatalf("完成后龙王应给两颗龙王的守护，实际 %v", r.Items)
	}
}

// 反向：没接任务时龙王没有块命中；宠物栏满时交不了任务（PetFullMsg）；背包满时拿不到龙王的守护。
func TestPoruQuest40Gates(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	poruPick(t, s, r, poruDragon, -1)
	if len(r.Pets) != 0 || len(r.Items) != 0 {
		t.Fatalf("没接任务时龙王不应给东西，实际 pets=%d items=%v", len(r.Pets), r.Items)
	}

	r = kingRecord(nil, nil, []int{poruQ40})
	fullPets := make([]petRec, 0, MaxPetSlots)
	for slot := 0; slot < MaxPetSlots; slot++ {
		fullPets = append(fullPets, petRec{Slot: slot, TemplateID: poruPet40, Name: "宠物", Level: 1})
	}
	r.Pets = fullPets
	blk := poruPick(t, s, r, poruDragon, 1)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.PetFull || fail == "" {
		t.Fatalf("宠物栏满时应说 PetFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}

	r = kingRecord(nil, []int{poruQ40}, nil)
	blk = poruPick(t, s, r, poruDragon, 0)
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r.Items = full
	blk = poruPick(t, s, r, poruDragon, 0)
	_, fail, ok = planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("背包满时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
}

// 任务 42 正向：玛丽娜丝接任务 41（事件 64 进行中）→ 泰坦队队员接任务 42（事件 65 进行中，
// 任务 68 也跟着变成进行中）→ 萨姆吉尔给两封公文 → 泰坦队队员交公文换通行证（事件 65 完成，
// 任务 42 和 68 同时完成）。
func TestPoruQuest42SharesEvent65(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, nil)
	kingRun(t, r, poruPick(t, s, r, poruMarina, 5))
	if !r.eventDoing(poruQ41) {
		t.Fatalf("接任务 41 后事件 64 应进行中，实际 %v", r.EventFlags)
	}

	kingRun(t, r, poruPick(t, s, r, poruCourier, 7))
	if !r.eventDoing(poruQ42) {
		t.Fatalf("接任务 42 后事件 65 应进行中，实际 %v", r.EventFlags)
	}
	if poruQuestState(r, 42) != questDoing || poruQuestState(r, 68) != questDoing {
		t.Fatalf("事件 65 进行中时任务 42、68 都应是进行中，实际 42=%d 68=%d", poruQuestState(r, 42), poruQuestState(r, 68))
	}

	kingRun(t, r, poruPick(t, s, r, poruSam, 3))
	kingRun(t, r, poruPick(t, s, r, poruSam, 2))
	if countItem(r.Items, poruLetterA) != 1 || countItem(r.Items, poruLetterB) != 1 {
		t.Fatalf("萨姆吉尔应给两封公文，实际 %v", r.Items)
	}

	kingRun(t, r, poruPick(t, s, r, poruCourier, 3))
	if countItem(r.Items, poruPass) != 1 || !r.eventDone(poruQ42) {
		t.Fatalf("交公文应拿到通行证、事件 65 完成，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	if poruQuestState(r, 42) != questDone || poruQuestState(r, 68) != questDone {
		t.Fatalf("事件 65 完成后任务 42、68 都应完成，实际 42=%d 68=%d", poruQuestState(r, 42), poruQuestState(r, 68))
	}
}

// 反向：事件 65 被任务 68 先占掉（已完成）时，任务 42 接不了；反过来任务 68 没有独立脚本，
// 只能跟着事件 65 走。两条任务共用同一个事件位，一条完成另一条也就完成。
func TestPoruEvent65SharedBothWays(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, []int{poruQ41})
	kingRun(t, r, poruPick(t, s, r, poruCourier, 7))
	kingRun(t, r, poruPick(t, s, r, poruSam, 3))
	kingRun(t, r, poruPick(t, s, r, poruSam, 2))
	kingRun(t, r, poruPick(t, s, r, poruCourier, 3))
	if poruQuestState(r, 68) != questDone {
		t.Fatalf("任务 42 完成时任务 68 应同时完成，实际 %d", poruQuestState(r, 68))
	}

	// 反过来：任务 68 先把事件 65 标成完成（模拟别的途径），任务 42 就接不了。
	r = kingRecord(nil, []int{poruQ40, poruQ42}, []int{poruQ41})
	blk := poruPick(t, s, r, poruCourier, 1) // 没有通行证：命中“通行证丢了”的补发块，不是接任务
	if blk.Event == poruQ42 && blk.Type == "REQUEST" {
		t.Fatalf("事件 65 已完成时不应再接任务 42")
	}
	if poruQuestState(r, 42) == questDoing {
		t.Fatalf("事件 65 已完成时任务 42 不应进入进行中，实际 %d", poruQuestState(r, 42))
	}
}

// 任务 44 正向：任务 41 进行中、任务 42/68 已完成 → 泰坦队长接任务 44（事件 66 进行中）→
// 小队长给鲁乌，事件 66 完成。任务 43 的后续（交鲁乌换圣石）走不通，见 TestPoruUnreachableTasks。
func TestPoruQuest44Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40, poruQ42}, []int{poruQ41})
	blk := poruPick(t, s, r, poruTitan, 8)
	if blk.Type != "REQUEST" || blk.Event != poruQ44 {
		t.Fatalf("接任务应命中事件 66 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(poruQ44) {
		t.Fatalf("接取后事件 66 应进行中，实际 %v", r.EventFlags)
	}

	kingRun(t, r, poruPick(t, s, r, poruSquad, 4))
	found := false
	for _, p := range r.Pets {
		if p.TemplateID == poruPet44 {
			found = true
		}
	}
	if !found || !r.eventDone(poruQ44) {
		t.Fatalf("小队长应给鲁乌（模板 %d）、事件 66 完成，实际 pets=%+v flags=%v", poruPet44, r.Pets, r.EventFlags)
	}
}

// 反向：任务 42/68 没完成（事件 65 未完成）时，泰坦队长不接任务 44。
func TestPoruQuest44Needs65(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40}, []int{poruQ41})
	poruPick(t, s, r, poruTitan, -1)
	if r.eventDoing(poruQ44) {
		t.Fatalf("事件 65 未完成不应接任务 44，实际 %v", r.EventFlags)
	}
}

// 卡点检查：任务 43 没有接取块（没有块把事件 55 设成进行中），
// 它的交任务还依赖未放开的 sainasu/event/nevent03_3 给的采石鉴定书 2474。这两条任务本批走不通。
func TestPoruUnreachableTasks(t *testing.T) {
	if !gamedata.NpcEventEnabled("poru/mary01") {
		t.Fatalf("poru/mary01 应已放开（STAGE-339，任务 41 的交付）")
	}
	mary01, _ := gamedata.NpcEventOf(1316)
	found := false
	for i := range mary01.Blocks {
		if mary01.Blocks[i].Event == poruQ41 && len(mary01.Blocks[i].EndSet) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("poru/mary01 应有完成事件 64 的块（任务 41 的出口）")
	}
	// STAGE-335：采石鉴定书的来源 nevent03_3 已放开（见 npcevent_sainasu_ore_test.go），这里只确认它在白名单里。
	if !gamedata.NpcEventEnabled("sainasu/event/nevent03_3") {
		t.Fatalf("sainasu/event/nevent03_3 应在白名单里（STAGE-335）")
	}

	// 没有任何脚本用 REQUEST 块接任务 43（事件 55），所以 NOWEV=55 永远不成立。
	for _, s := range gamedata.NpcEventScripts() {
		for i := range s.Blocks {
			b := &s.Blocks[i]
			if b.Type == "REQUEST" && b.Event == poruQ43 {
				t.Fatalf("%s 块 %d 能接任务 43，测试假设已过时", s.File, i)
			}
		}
	}
	// 没有采石鉴定书时，泰坦队长的事件 55 交任务块不成立。
	s := kingService()
	r := kingRecord(nil, []int{poruQ40, poruQ42, poruQ44}, []int{poruQ41, poruQ43})
	r.Pets = []petRec{{Slot: 0, TemplateID: poruPet44, Name: "鲁乌", Level: 30}}
	if blk := poruPick(t, s, r, poruTitan, -1); blk != nil {
		t.Fatalf("没有 2474 时不应命中交鲁乌的块，实际 %s", blk.Type)
	}
}

// 数据检查：六个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；任务 40、42、44 的完成事件对得上；
// 公文和通行证有来源；宠物模板查得到；任务 42/68 共用事件 65，任务 41/43 的卡点仍成立。
func TestPoruScriptData(t *testing.T) {
	files := map[int]string{
		poruGrandpa: "poru/boy02",
		poruDragon:  "poru/boy04",
		poruMarina:  "poru/mary",
		poruSquad:   "poru/mary04",
		poruTitan:   "poru/team01",
		poruCourier: "poru/titan",
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
	for no, event := range map[int]int{40: poruQ40, 42: poruQ42, 43: poruQ43, 44: poruQ44} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	if q, ok := gamedata.QuestByNo(68); !ok || q.EndEvent != poruQ42 {
		t.Errorf("任务 68 的完成事件应与任务 42 相同（65），实际 %+v", q)
	}
	// 公文由萨姆吉尔给，通行证由泰坦队队员给，龙王的守护由龙王给。
	if sam, _ := gamedata.NpcEventOf(poruSam); sam.Blocks[3].GetItem[0].ID != poruLetterA || sam.Blocks[2].GetItem[0].ID != poruLetterB {
		t.Errorf("萨姆吉尔的块 3/2 应给两封公文，实际 %+v / %+v", sam.Blocks[3].GetItem, sam.Blocks[2].GetItem)
	}
	courier, _ := gamedata.NpcEventOf(poruCourier)
	if courier.Blocks[3].GetItem[0].ID != poruPass {
		t.Errorf("泰坦队队员的交公文块应给通行证，实际 %+v", courier.Blocks[3].GetItem)
	}
	dragon, _ := gamedata.NpcEventOf(poruDragon)
	if dragon.Blocks[0].GetItem[0].ID != poruGuard {
		t.Errorf("龙王的块 0 应给龙王的守护，实际 %+v", dragon.Blocks[0].GetItem)
	}
	if _, ok := gamedata.ItemByID(poruCert); !ok {
		t.Errorf("采石鉴定书 %d 应在道具表里", poruCert)
	}
	if _, ok := gamedata.ItemByID(poruStone); !ok {
		t.Errorf("岳之圣石 %d 应在道具表里", poruStone)
	}
}
