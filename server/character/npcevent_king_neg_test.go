package character

// STAGE-220：精灵王的传说的反向用例和风险核对（纯逻辑，见 npcevent_king_test.go）。
// 条件不满足时选中的是催促、拒绝或闲谈的块，不会重复发奖；栏位满、背包满时整笔不做；
// 原版本来就能再领的（丢了可以补）照搬，这里记成测试，方便以后改动时看见。

import (
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 没有前置（事件 4 未完成）：接不了 69，四个人都只闲谈（没有块命中，不是拒绝）。
func TestKingNeedsCeremonyFirst(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	kingTalk(t, s, r, kingGrandpa, -1)
	kingTalk(t, s, r, kingShin, -1)
	kingTalk(t, s, r, kingFrog, -1)
	kingTalk(t, s, r, kingGrandma, -1)
}

// 背包满：新藏给金珠时收不下（ItemFullMsg），整笔不做，69 仍进行中；反过来，拿着金珠去换黑玉时先收后给，
// 满包也能换（planEvent 先删道具再算空位，同原版 NPC_AcceptDel）。
func TestKingFullBagChangesNothing(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1}) // 2417 是仪玉，这里只占格子
	}
	r := kingRecord(full, []int{4}, []int{69})

	blk := kingPick(t, s, r, kingShin, 4)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, kingFrogGold) != 0 || !r.eventDoing(69) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d 69doing=%v", len(r.Items), r.eventDoing(69))
	}

	// 背包满但手上有金珠：换黑玉成立，背包仍是满的（金珠的格子让给黑玉）。
	r.Items = append([]itemRec{{Slot: firstBagSlot, ItemID: kingFrogGold, Count: 1}}, full[1:]...)
	blk = kingPick(t, s, r, kingFrog, 1)
	kingRun(t, r, blk)
	if countItem(r.Items, kingBlackJade) != 1 || countItem(r.Items, kingFrogGold) != 0 || len(r.Items) != len(full) {
		t.Fatalf("满包时换黑玉应成立且背包仍满，实际 %d 件，黑玉 %d", len(r.Items), countItem(r.Items, kingBlackJade))
	}
}

// 任务做完之后不再发奖：69、70 都完成，手上有项链，祖母、祖父、新藏、蛙王都只闲谈（或催促的台词）。
func TestKingDoneGivesNothingAgain(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(kingNecklace), []int{4, 69, 70}, nil)
	kingTalk(t, s, r, kingGrandma, 11) // 项链还在：闲谈
	kingTalk(t, s, r, kingGrandpa, 6)  // 闲谈
	kingTalk(t, s, r, kingShin, -1)    // 69、70 都完成：没有块命中，闲聊一句
	kingTalk(t, s, r, kingFrog, -1)
	if countItem(r.Items, kingNecklace) != 1 || countItem(r.Items, kingFrogGold) != 0 || len(r.Pets) != 0 {
		t.Fatalf("完成后闲谈不应改动背包和宠物，实际 %v pets=%d", r.Items, len(r.Pets))
	}
}

// 原版行为（照搬，记录在这里）：事件 70 完成后，项链丢了（卖了、丢了）可以再找愿藏的祖母补一条，但同时只能有一条。
func TestKingNecklaceReissueAfterLoss(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4, 69, 70}, nil)
	blk := kingPick(t, s, r, kingGrandma, 2)
	kingRun(t, r, blk)
	if countItem(r.Items, kingNecklace) != 1 {
		t.Fatalf("丢了项链后应补一条，实际 %v", r.Items)
	}
	kingTalk(t, s, r, kingGrandma, 11) // 补过一条之后再找：只闲谈

	// 事件 180 之后（现在没有脚本设置 180，后续内容才有）项链又丢了：走的是 MESSAGE 块 0（MESSAGE 也结算 GetItem，
	// 和 ACCEPT 的块 2 是两条路径）。这里直接把 180 记成已完成，覆盖这条发奖路径。
	r = kingRecord(nil, []int{4, 69, 70, 71, 180}, nil)
	blk = kingPick(t, s, r, kingGrandma, 0)
	if blk.Type != "MESSAGE" {
		t.Fatalf("180 之后补项链应是 MESSAGE 块，实际 %s", blk.Type)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, kingNecklace) != 1 {
		t.Fatalf("事件 180 之后丢了项链也应补一条，实际 %v", r.Items)
	}
	kingTalk(t, s, r, kingGrandma, 11)
}

// 原版行为（照搬）：守护者的魔法戒指（19641–19644）在 72 完成后丢了，再找对应的守护者可以补一枚（ENDEV=72 那一组）。
// 72 进行中、没有戒指的时候是正常的发放（ENDEV=71&NOWEV=72 那一组）。
func TestKingRingReissueAfterLoss(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4, 69, 70, 71, 72}, nil)
	blk := kingPick(t, s, r, kingYarus, 1)
	kingRun(t, r, blk)
	if countItem(r.Items, kingFirstRing) != 1 {
		t.Fatalf("72 完成后丢了戒指，应能补一枚，实际 %v", r.Items)
	}
	kingTalk(t, s, r, kingYarus, 0)
}

// 事件 71 完成、72 进行中、身上有项链：祖母的第 1 块（71 之后才有的“确认玛蕾菲雅被收回”）不能被选中。
// 那一块里有 NOW=72 的组合：原版不认 NOW（恒不成立），转换时丢掉了。如果误当成 NOWEV=72 解释，
// 其中 ENDEV=71&NOWEV=72&ENDEV!=180 会成立，祖母就会抢在第 11 块前面说话。
func TestKingEvent71BlockNoNowGroup(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(kingNecklace), []int{4, 69, 70, 71}, []int{72})
	kingTalk(t, s, r, kingGrandma, 11)

	script, _ := gamedata.NpcEventOf(kingGrandma)
	blk := &script.Blocks[1]
	if len(blk.Cond) != 3 {
		t.Fatalf("第 1 块应剩 3 个条件组（原文 4 组，NOW=72 的那组已丢掉），实际 %d", len(blk.Cond))
	}
	for _, g := range blk.Cond {
		if hasTerm(g, "ENDEV", 71) && hasTerm(g, "NOWEV", 72) {
			t.Fatalf("第 1 块不应有“ENDEV=71 且 NOWEV=72”的组：那是 NOW=72 被误读成 NOWEV=72 的样子")
		}
	}
}

// hasTerm 报告条件组里有没有 op 为 n 的条件。
func hasTerm(g []gamedata.EventTerm, op string, n int) bool {
	for _, term := range g {
		if term.Op == op && term.N == n {
			return true
		}
	}
	return false
}

// 祖母：70 进行中但 69 未完成时，不给项链和宠物（第 12 块要求 69 已完成），只催促去找新藏。
func TestKingGrandmaWaitsForSixtyNine(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, []int{69, 70})
	kingTalk(t, s, r, kingGrandma, 13)
	if len(r.Items) != 0 || len(r.Pets) != 0 || !r.eventDoing(70) {
		t.Fatalf("69 未完成时祖母不应给东西，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
}

// 宠物栏满（GetPet）：祖母的 1479 在栏满时整笔不做（不给项链，70 不完成），说 PetFullMsg；
// 空出一格后成立，进第一个空的随身栏位（这里是 2 号）。
func TestKingGetPetSlotFull(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4, 69}, []int{70})
	blk := kingPick(t, s, r, kingGrandma, 12)

	r.Pets = petsOf(MaxPetSlots)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.PetFull || fail == "" {
		t.Fatalf("栏满时应说 PetFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}

	r.Pets = []petRec{petFor(0, 560, 10, false), petFor(1, 560, 10, false), petFor(3, 560, 10, false), petFor(4, 560, 10, false)}
	plan, _, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if !ok {
		t.Fatalf("空出 2 号栏位后应成立")
	}
	var got *petRec
	for i := range plan.pets {
		if plan.pets[i].TemplateID == kingTmplMaleifa {
			got = &plan.pets[i]
		}
	}
	if got == nil || got.Slot != 2 {
		t.Fatalf("玛蕾菲雅应进 2 号栏位，实际 %+v", plan.pets)
	}
}

// 宠物栏满（GetPet 355，心性之镜的线，事件 54）：栏满时 2547 不收、54 不完成，说 PetFullMsg；
// 有空位时收走 2547、送 355 并完成 54（DelItem 收走背包里的全部 2547）。
func TestKingGetPet355SlotFull(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(2547), []int{4, 29, 30}, []int{54})
	blk := kingPick(t, s, r, 1419, 2)

	r.Pets = petsOf(MaxPetSlots)
	if _, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand); ok || fail != blk.Msg.PetFull {
		t.Fatalf("栏满时应说 PetFullMsg，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, 2547) != 1 {
		t.Fatalf("栏满时 2547 不应被收走")
	}

	r.Pets = petsOf(MaxPetSlots - 1)
	kingRun(t, r, blk)
	if countItem(r.Items, 2547) != 0 || !r.eventDone(54) || len(r.Pets) != MaxPetSlots {
		t.Fatalf("有空位时应收走 2547、完成 54 并送宠物，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
}

// 数据核对：king 目录的 13 个脚本都在白名单里，对应的 NPC 都在 NPC 表里（且 NPC 编号与脚本一致），
// 所在地图是正式地图（快捷传送目录里有、有落点）；脚本引用的道具、怪物、宠物模板都查得到；
// 每个条件组都有条件（没有恒真的空组）。
func TestKingScriptData(t *testing.T) {
	var kingScripts []*gamedata.EventScript
	for _, s := range gamedata.NpcEventScripts() {
		if strings.HasPrefix(s.File, "king/") {
			kingScripts = append(kingScripts, s)
		}
	}
	if len(kingScripts) != 13 {
		t.Fatalf("king 目录应有 13 个任务脚本，实际 %d", len(kingScripts))
	}
	for _, s := range kingScripts {
		if !gamedata.NpcEventEnabled(s.File) {
			t.Errorf("%s 应已放开", s.File)
		}
		npc, ok := gamedata.NPCByID(s.ID)
		if !ok {
			t.Errorf("%s 的 NPC %d 不在 NPC 表里", s.File, s.ID)
			continue
		}
		if npc.Kind != "changeevent" {
			t.Errorf("%s 的 NPC %d 类型应是 changeevent，实际 %s", s.File, s.ID, npc.Kind)
		}
		if !gamedata.FormalFloor(npc.Floor) {
			t.Errorf("%s 的 NPC %d 在地图 %d，不是正式地图（快捷传送到不了）", s.File, s.ID, npc.Floor)
		}
		for i := range s.Blocks {
			b := &s.Blocks[i]
			if b.Unsupported != "" {
				t.Errorf("%s 块 %d 不支持：%s", s.File, i, b.Unsupported)
			}
			for _, g := range b.Cond {
				if len(g) == 0 {
					t.Errorf("%s 块 %d 有空的条件组（恒真）", s.File, i)
				}
				for _, term := range g {
					if term.Op == "ITEM" {
						checkKingItem(t, s.File, term.ID)
					}
				}
			}
			for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
				checkKingItem(t, s.File, it.ID)
			}
			for _, id := range b.GetRand {
				checkKingItem(t, s.File, id)
			}
			for _, enemy := range b.GetPet {
				e, ok := gamedata.EnemyByID(enemy)
				if !ok {
					t.Errorf("%s 块 %d 的怪物 %d 不在敌人表里", s.File, i, enemy)
					continue
				}
				if _, ok := gamedata.PetByID(e.Template); !ok {
					t.Errorf("%s 块 %d 的怪物 %d 的宠物模板 %d 不在宠物表里", s.File, i, enemy, e.Template)
				}
			}
		}
	}
}

// checkKingItem 查道具表（DelItem 的 N 为 0 只表示“全部”，编号照常查）。
func checkKingItem(t *testing.T, file string, id int) {
	t.Helper()
	if _, ok := gamedata.ItemByID(id); !ok {
		t.Errorf("%s 引用的道具 %d 不在道具表里", file, id)
	}
}
