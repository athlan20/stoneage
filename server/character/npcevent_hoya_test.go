package character

// STAGE-221：哥亚山洞窟的任务 23、24、25（事件 6、7、8）端到端（纯逻辑，不连数据库）：用真实白名单、真实选块
// pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具和宠物增减、事件位变化。
// 反向用例覆盖条件不满足、背包满、进行中再选、完成后再来。协议层（对话窗口、推送）不在这里测。
// 脚本的数据检查（整本支持、NPC 类型和地图、道具和宠物都查得到）见本文件末尾的 TestHoyaScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	hoyaMarina     = 1298 // 玛丽娜丝的商人（event06_2，卡鲁它那村 4000）：给贝壳
	hoyaYoung      = 1370 // 年青人（event06_1，哥亚山洞窟 1 楼 20401）：任务 23 的接取和交付
	hoyaUlisukai   = 1371 // 乌力斯凯（event07_1，20401）：任务 24 的接取，任务 25 的交付
	hoyaFlowerShop = 1377 // 乌力花店（event07_3，哥亚山洞窟 4 楼 20404）：任务 24 中途拿 2424
	hoyaUlimi      = 1382 // 乌力美（event07_2，山顶 20406）：任务 24 的交付，任务 25 的接取

	hoyaShell   = 2414 // 不可思议的贝壳（任务 23 的道具，也是 24、25 的入场条件）
	hoyaMidA    = 2424 // 任务 24 的中间物（花店给，乌力美收）
	hoyaMidB    = 2425 // 任务 24 的中间物（乌力斯凯给，花店和乌力美收）
	hoyaLetter  = 2426 // 任务 25 的信（乌力美给，乌力斯凯收）
	hoyaReward  = 1206 // 任务 25 的奖励（乌力斯凯给）
	hoyaShellOf = 2    // 玛丽娜丝的商人一次给的贝壳数（NOWEV=6 且没有贝壳时给 2）
)

// hoyaPick 先确认 NPC 的脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func hoyaPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// hoyaFullBag 把背包占满（每格一件 2417，只占格子）。
func hoyaFullBag() []itemRec {
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	return full
}

// 任务 23 正向：年青人接取（只设进行中）→ 商人给两个贝壳 → 年青人交两个，领宠物 44，留一个贝壳，事件 6 完成。
func TestHoya23WalksToPet(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)

	hoyaPick(t, s, r, hoyaMarina, -1) // 任务还没接：商人只闲聊
	kingRun(t, r, hoyaPick(t, s, r, hoyaYoung, 2))
	if !r.eventDoing(6) || len(r.Items) != 0 || len(r.Pets) != 0 {
		t.Fatalf("接取后事件 6 应进行中、不给东西，实际 doing=%v items=%v pets=%d", r.eventDoing(6), r.Items, len(r.Pets))
	}

	kingRun(t, r, hoyaPick(t, s, r, hoyaMarina, 0))
	if got := countItem(r.Items, hoyaShell); got != hoyaShellOf {
		t.Fatalf("商人应给 %d 个贝壳，实际 %d", hoyaShellOf, got)
	}
	hoyaPick(t, s, r, hoyaMarina, 1) // 已经有两个：只说“已经不需要了”

	kingRun(t, r, hoyaPick(t, s, r, hoyaYoung, 0))
	if got := countItem(r.Items, hoyaShell); got != 1 {
		t.Fatalf("交付后应剩 1 个贝壳（扣 1），实际 %d", got)
	}
	if len(r.Pets) != 1 {
		t.Fatalf("交两个贝壳应领到 1 只宠物（44），实际 %d 只", len(r.Pets))
	}
	if !r.eventDone(6) || r.eventDoing(6) {
		t.Fatalf("交付后事件 6 应完成、不再进行中，实际 %v", r.EventFlags)
	}
}

// 任务 24 正向（接上任务 23 完成后的状态，手上有一个贝壳）：乌力斯凯接取（给花 2425）→ 花店给信 2424 →
// 乌力美交花和信，接任务 25（给 2426）。事件 7 完成，2424 和 2425 收走。
func TestHoya24WalksToLetter(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(hoyaShell), []int{6}, nil)

	kingRun(t, r, hoyaPick(t, s, r, hoyaUlisukai, 1)) // REQUEST 7：给花 2425，进行中
	if countItem(r.Items, hoyaMidB) != 1 || !r.eventDoing(7) {
		t.Fatalf("接取后应有 2425、事件 7 进行中，实际 %v doing=%v", r.Items, r.eventDoing(7))
	}

	kingRun(t, r, hoyaPick(t, s, r, hoyaFlowerShop, 0)) // ACCEPT 7：给信 2424，不设完成
	if countItem(r.Items, hoyaMidA) != 1 || !r.eventDoing(7) {
		t.Fatalf("花店应给 2424 且事件 7 仍进行中，实际 %v doing=%v", r.Items, r.eventDoing(7))
	}
	hoyaPick(t, s, r, hoyaFlowerShop, -1) // 已经拿到信：只闲聊

	kingRun(t, r, hoyaPick(t, s, r, hoyaUlimi, 0)) // ACCEPT 7：交 2424、2425，完成 7
	if countItem(r.Items, hoyaMidA) != 0 || countItem(r.Items, hoyaMidB) != 0 || !r.eventDone(7) {
		t.Fatalf("乌力美收下后 2424、2425 应没有，事件 7 应完成，实际 %v done=%v", r.Items, r.eventDone(7))
	}

	kingRun(t, r, hoyaPick(t, s, r, hoyaUlimi, 1)) // REQUEST 8：给信 2426，进行中
	if countItem(r.Items, hoyaLetter) != 1 || !r.eventDoing(8) {
		t.Fatalf("接任务 25 后应有 2426、事件 8 进行中，实际 %v doing=%v", r.Items, r.eventDoing(8))
	}
	if countItem(r.Items, hoyaShell) != 1 {
		t.Fatalf("任务 24、25 不应动贝壳，实际 %d", countItem(r.Items, hoyaShell))
	}
}

// 任务 25 正向：拿着 2426 回乌力斯凯那里 → 领奖 1206，收走 2426，事件 8 完成；之后乌力斯凯只闲聊。
func TestHoya25WalksToReward(t *testing.T) {
	s := kingService()
	r := kingRecord(append(kingItems(hoyaShell), itemRec{Slot: firstBagSlot + 1, ItemID: hoyaLetter, Count: 1}),
		[]int{6, 7}, []int{8})

	kingRun(t, r, hoyaPick(t, s, r, hoyaUlisukai, 0))
	if countItem(r.Items, hoyaReward) != 1 || countItem(r.Items, hoyaLetter) != 0 || !r.eventDone(8) {
		t.Fatalf("交信后应领到 1206、信收走、事件 8 完成，实际 %v done=%v", r.Items, r.eventDone(8))
	}
	if countItem(r.Items, hoyaShell) != 1 {
		t.Fatalf("任务 25 不应动贝壳，实际 %d", countItem(r.Items, hoyaShell))
	}
}

// 反向：只有一个贝壳时年青人走的是另一块（领宠物 47），商人见到一个贝壳时给一个（不是两个）。
func TestHoya23OneShellPath(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(hoyaShell), nil, []int{6})

	kingRun(t, r, hoyaPick(t, s, r, hoyaYoung, 1))
	if len(r.Pets) != 1 || countItem(r.Items, hoyaShell) != 0 || !r.eventDone(6) {
		t.Fatalf("一个贝壳交付应领宠物 47、贝壳用光、事件 6 完成，实际 pets=%d items=%v", len(r.Pets), r.Items)
	}

	r = kingRecord(nil, nil, []int{6})
	kingRun(t, r, hoyaPick(t, s, r, hoyaMarina, 0))
	if countItem(r.Items, hoyaShell) != hoyaShellOf {
		t.Fatalf("没有贝壳时商人应给 %d 个，实际 %v", hoyaShellOf, r.Items)
	}
	r = kingRecord(kingItems(hoyaShell), nil, []int{6})
	kingRun(t, r, hoyaPick(t, s, r, hoyaMarina, 2))
	if countItem(r.Items, hoyaShell) != 2 {
		t.Fatalf("手上有一个时商人应补到两个，实际 %v", r.Items)
	}
}

// 反向：任务 23 还没接（事件 6 没进行中）时，年青人和商人都不给东西；进行中但没有贝壳时，年青人不交付。
func TestHoya23ConditionsHold(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	hoyaPick(t, s, r, hoyaYoung, 2) // 未接：命中的是 REQUEST（接取），不是交付
	if r.eventDoing(6) || len(r.Items) != 0 {
		t.Fatalf("只是看到 REQUEST 不应改任何东西")
	}

	r = kingRecord(nil, nil, []int{6})
	blk := hoyaPick(t, s, r, hoyaYoung, 2) // 进行中、没有贝壳：命中的是 REQUEST（原版没有 StopMsg，只是不说话），不交付
	if blk.Type != "REQUEST" || blk.Event != 6 {
		t.Fatalf("进行中再选应命中事件 6 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if len(r.Pets) != 0 || len(r.Items) != 0 || !r.eventDoing(6) {
		t.Fatalf("没有贝壳时不应给宠物，实际 pets=%d items=%v", len(r.Pets), r.Items)
	}
}

// 反向：背包满时商人给不了贝壳，交易不成立、台词是 ItemFullMsg，什么都不改（事件 6 仍进行中）。
func TestHoya23FullBagChangesNothing(t *testing.T) {
	s := kingService()
	r := kingRecord(hoyaFullBag(), nil, []int{6})
	blk := hoyaPick(t, s, r, hoyaMarina, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, hoyaShell) != 0 || len(r.Items) != len(hoyaFullBag()) || !r.eventDoing(6) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d doing=%v", len(r.Items), r.eventDoing(6))
	}
}

// 反向：没有贝壳（任务 23 没做）时，乌力斯凯不接任务 24（REQUEST 需要 2414），花店和乌力美也没有块。
func TestHoya24NeedsShell(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{6}, nil)
	hoyaPick(t, s, r, hoyaUlisukai, -1)
	if countItem(r.Items, hoyaMidB) != 0 || r.eventDoing(7) {
		t.Fatalf("没有贝壳不应接到任务 24，实际 %v", r.Items)
	}
}

// 反向：任务 24 进行中再找乌力斯凯，命中的是 REQUEST 的放弃问话（StopMsg），不会再给一朵花 2425；
// 花店在手上已有信 2424 时不再给第二封（条件 ITEM!=2424），乌力美没收到信之前也不会交付。
func TestHoya24DoingDoesNotRegrant(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(hoyaShell, hoyaMidB), []int{6}, []int{7})
	blk := hoyaPick(t, s, r, hoyaUlisukai, 1)
	if blk.Type != "REQUEST" || blk.Event != 7 {
		t.Fatalf("进行中再找乌力斯凯应命中事件 7 的 REQUEST（只会问放弃），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, hoyaMidB) != 1 {
		t.Fatalf("只是对话不应再给花，实际 %v", r.Items)
	}

	r = kingRecord(kingItems(hoyaShell, hoyaMidB, hoyaMidA), []int{6}, []int{7})
	hoyaPick(t, s, r, hoyaFlowerShop, -1) // 手上已有 2424：ACCEPT 不再命中，事件 7 还没完成，MESSAGE 也不命中
	if countItem(r.Items, hoyaMidA) != 1 {
		t.Fatalf("已有信 2424 不应再给，实际 %v", r.Items)
	}
}

// 反向：任务 25 进行中（2426 已在手），再找乌力斯凯只闲聊或命中放弃问话，不会再领奖 1206。
func TestHoya25DoneGivesNothingAgain(t *testing.T) {
	s := kingService()
	r := kingRecord(append(kingItems(hoyaShell), itemRec{Slot: firstBagSlot + 1, ItemID: hoyaLetter, Count: 1}),
		[]int{6, 7, 8}, nil)
	hoyaPick(t, s, r, hoyaUlisukai, 2) // 已完成的 8 不再命中，只剩闲聊的 MESSAGE 块
	if countItem(r.Items, hoyaReward) != 0 {
		t.Fatalf("任务 25 完成后不应再领奖，实际 %v", r.Items)
	}
}

// 反向：全部完成后（6、7、8 都完成，手上一个贝壳）五个 NPC 都只闲聊，东西和事件位都不变。
func TestHoyaAllDoneOnlyChat(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(hoyaShell), []int{6, 7, 8}, nil)
	before := len(r.Items)
	hoyaPick(t, s, r, hoyaMarina, -1)  // 贝壳还有，不给
	hoyaPick(t, s, r, hoyaYoung, -1)   // 事件 6 完成，不再交付
	hoyaPick(t, s, r, hoyaUlisukai, 2) // 完成 7、8：闲聊的 MESSAGE 块
	hoyaPick(t, s, r, hoyaFlowerShop, 1)
	hoyaPick(t, s, r, hoyaUlimi, 2)
	if len(r.Items) != before || len(r.Pets) != 0 || countItem(r.Items, hoyaReward) != 0 {
		t.Fatalf("全部完成后闲聊不应改东西，实际 %v pets=%d", r.Items, len(r.Pets))
	}
	if !r.eventDone(6) || !r.eventDone(7) || !r.eventDone(8) {
		t.Fatalf("事件位应保持完成，实际 %v", r.EventFlags)
	}
}

// 一个贝壳路线：年青人交付时只有一个贝壳，领宠物 47 并把贝壳用光；之后商人在“事件 6 完成、没有贝壳”时补一个，
// 才能接上任务 24（REQUEST 7 要 2414）。这是原版的设计，所以补贝壳的块不能删。
func TestHoya23OneShellThen24(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(hoyaShell), nil, []int{6})
	kingRun(t, r, hoyaPick(t, s, r, hoyaYoung, 1))
	kingRun(t, r, hoyaPick(t, s, r, hoyaMarina, 3))
	if countItem(r.Items, hoyaShell) != 1 {
		t.Fatalf("事件 6 完成后应补回 1 个贝壳，实际 %v", r.Items)
	}
	kingRun(t, r, hoyaPick(t, s, r, hoyaUlisukai, 1))
	if !r.eventDoing(7) || countItem(r.Items, hoyaMidB) != 1 {
		t.Fatalf("有贝壳后应接上任务 24（给花 2425），实际 doing=%v items=%v", r.eventDoing(7), r.Items)
	}
}

// 任务 23 完成后再来：没有贝壳时商人会补一个（ACCEPT -1，ENDEV=6 且没有贝壳，原版照搬，丢了可补），
// 补过之后不会再补；年青人和任务 23 的块都不再命中，不会重复领宠物。
func TestHoya23ShellReissueAfterDone(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{6}, nil)
	kingRun(t, r, hoyaPick(t, s, r, hoyaMarina, 3))
	if countItem(r.Items, hoyaShell) != 1 {
		t.Fatalf("没有贝壳时应补 1 个，实际 %v", r.Items)
	}
	hoyaPick(t, s, r, hoyaMarina, -1) // 有了：不再补
	hoyaPick(t, s, r, hoyaYoung, -1)  // 任务 23 完成：不再交付、不再领宠物
	if len(r.Pets) != 0 || countItem(r.Items, hoyaShell) != 1 {
		t.Fatalf("完成后再来不应重复领宠物或贝壳，实际 pets=%d items=%v", len(r.Pets), r.Items)
	}
}

// 数据检查：五个脚本都在白名单里、整本支持（没有不支持的块）、NPC 是任务类型且在正式地图上（能用快捷传送到）、
// 用到的道具和宠物都查得到。
func TestHoyaScriptData(t *testing.T) {
	files := map[string]int{
		"jaruga/event/event06_1": hoyaYoung,
		"jaruga/event/event06_2": hoyaMarina,
		"jaruga/event/event07_1": hoyaUlisukai,
		"jaruga/event/event07_2": hoyaUlimi,
		"jaruga/event/event07_3": hoyaFlowerShop,
	}
	for file, npcID := range files {
		script, ok := gamedata.NpcEventOf(npcID)
		if !ok || script.File != file {
			t.Fatalf("NPC %d 应对应脚本 %s，实际 %v", npcID, file, script)
		}
		if !gamedata.NpcEventEnabled(file) {
			t.Errorf("%s 应已放开", file)
		}
		npc, ok := gamedata.NPCByID(npcID)
		if !ok || npc.Kind != "changeevent" || !gamedata.FormalFloor(npc.Floor) {
			t.Errorf("%s 的 NPC %d 应是任务类型、在正式地图上，实际 %+v", file, npcID, npc)
		}
		for i := range script.Blocks {
			b := &script.Blocks[i]
			if b.Unsupported != "" {
				t.Errorf("%s 块 %d 不支持：%s", file, i, b.Unsupported)
			}
			for _, g := range b.Cond {
				for _, term := range g {
					if term.Op == "ITEM" {
						checkKingItem(t, file, term.ID)
					}
				}
			}
			for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
				checkKingItem(t, file, it.ID)
			}
			for _, enemy := range b.GetPet {
				e, ok := gamedata.EnemyByID(enemy)
				if !ok {
					t.Errorf("%s 块 %d 的怪物 %d 不在敌人表里", file, i, enemy)
					continue
				}
				if _, ok := gamedata.PetByID(e.Template); !ok {
					t.Errorf("%s 块 %d 的怪物 %d 的宠物模板 %d 不在宠物表里", file, i, enemy, e.Template)
				}
			}
		}
	}
}
