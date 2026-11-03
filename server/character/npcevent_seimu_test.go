package character

// STAGE-320：梦幻洞窟的任务 32、33、34（事件 32、33、34）和卡坦的任务 4（事件 35）端到端（纯逻辑，
// 不连数据库）：用真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每一步断言命中的块、
// 道具和宠物增减、事件位变化。反向用例覆盖前置没做、顺序不对、宠物等级不够、背包满、完成后再来，
// 以及原版的两处可重复领取（先见之光换随机道具、卡坦的后续收集）。协议层不在这里测。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	seimuElder   = 1334 // 村中的老人（seimu/event/event01_1，奇努伊村 6000）：派先见之光
	seimuSage    = 1182 // 梦见的仙人（seimu/event/event01_2，梦幻的洞窟 41）：交先见之光换奖励（1183 在 42，同一份脚本）
	seimuSageAlt = 1183
	seimuKatan   = 1187 // 卡坦（seimu/event/event02_1，地图 100）：收 4 只 25 级恐龙

	seimuAdult    = 4    // 成人仪式的完成事件（梦幻洞窟任务的前置）
	seimuDream32  = 32   // 任务 32 拿竖琴
	seimuDream33  = 33   // 任务 33 拿帽子
	seimuDream34  = 34   // 任务 34 拿衣服
	seimuShark    = 35   // 任务 4 卡坦找 4 只鲨鱼
	seimuLight    = 2603 // 先见之光
	seimuCloth    = 2604 // 空想的衣服（任务 34 的奖励）
	seimuHat      = 2605 // 空想的帽子（任务 33 的奖励）
	seimuHarp     = 2606 // 空想的竖琴（任务 32 的奖励）
	seimuRing     = 2607 // 火难的戒指（任务 4 的中间证明）
	seimuGold     = 171  // 任务 4 的奖励宠物模板（修宝；脚本里 GetPet:93，怪物 93 的模板是 171）
	seimuSidePet  = 274  // 任务 4 后续收集的奖励宠物模板（朵拉比斯；脚本里 GetPet:341，怪物 341 的模板是 274）
	seimuRandom   = 450  // 三任务完成后先见之光换到的随机道具（kingRand 固定取第一件）
	seimuGag      = 223  // 加格（25 级）
	seimuBon      = 224  // 邦恩吉（25 级）
	seimuKoban    = 221  // 克邦凯斯（25 级）
	seimuKakura   = 222  // 加克拉（25 级）
	seimuFirebird = 291  // 火鸡（25 级，后续收集）
	seimuKiki     = 294  // 奇宝（25 级，后续收集）
	seimuHoluk    = 293  // 霍尔克（25 级，后续收集）
	seimuKuro     = 558  // 克克洛斯（25 级，后续收集）
)

// seimuPick 先确认脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func seimuPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
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

// seimuPets 造一组 25 级宠物（每只占一个宠位，模板按参数顺序）。
func seimuPets(templates ...int) []petRec {
	out := make([]petRec, 0, len(templates))
	for i, tmpl := range templates {
		out = append(out, petRec{Slot: i, TemplateID: tmpl, Name: "恐龙", Level: 25})
	}
	return out
}

// seimuDreamHero 是完成成人仪式的勇者小明（梦幻洞窟三个任务的前置）。
func seimuDreamHero() *record {
	return kingRecord(nil, []int{seimuAdult}, nil)
}

// 梦幻洞窟正向：老人派任务 32 → 仙人换竖琴 → 老人派 33 → 仙人换帽子 → 老人派 34 → 仙人换衣服。
func TestDream32To34Walks(t *testing.T) {
	s := kingService()
	r := seimuDreamHero()
	steps := []struct {
		quest int
		item  int
	}{
		{seimuDream32, seimuHarp},
		{seimuDream33, seimuHat},
		{seimuDream34, seimuCloth},
	}
	for i, st := range steps {
		// 三个 REQUEST 块按任务 32、33、34 的顺序倒排（块 2、1、0）。
		kingRun(t, r, seimuPick(t, s, r, seimuElder, 2-i))
		if !r.eventDoing(st.quest) || countItem(r.Items, seimuLight) != 1 {
			t.Fatalf("任务 %d 接取后应进行中、拿到先见之光，实际 %v flags=%v", st.quest, r.Items, r.EventFlags)
		}
		// 三个交任务块按 34、33、32 排（块 1、2、3）。
		kingRun(t, r, seimuPick(t, s, r, seimuSage, 3-i))
		if !r.eventDone(st.quest) || countItem(r.Items, st.item) != 1 || countItem(r.Items, seimuLight) != 0 {
			t.Fatalf("任务 %d 交完应拿到道具 %d、收走先见之光，实际 %v flags=%v", st.quest, st.item, r.Items, r.EventFlags)
		}
	}
	if len(r.Items) != 3 {
		t.Fatalf("三个任务完成后应有竖琴、帽子、衣服三件，实际 %v", r.Items)
	}
	// 完成后再找仙人：没有先见之光，只说闲话。
	seimuPick(t, s, r, seimuSage, 5)
}

// 反向：没完成成人仪式时，老人只说“没有勇者”的闲话，接不到任务。
func TestDreamNeedsAdult(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	seimuPick(t, s, r, seimuElder, 5)
	if r.eventDoing(seimuDream32) || len(r.Items) != 0 {
		t.Fatalf("没完成成人仪式不应接到任务，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：顺序不对时接不到下一个任务——只完成 32 时老人派 33，不会直接派 34。
func TestDreamQuestOrder(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{seimuAdult, seimuDream32}, nil)
	seimuPick(t, s, r, seimuElder, 1) // 块 1：任务 33
	r = kingRecord(nil, []int{seimuAdult, seimuDream32, seimuDream33}, nil)
	seimuPick(t, s, r, seimuElder, 0) // 块 0：任务 34
}

// 反向：没有先见之光时仙人只说闲话；任务完成后身上还留着一枚先见之光时，老人会收走它（原版如此）。
func TestDreamLightRecovery(t *testing.T) {
	s := kingService()
	r := seimuDreamHero()
	seimuPick(t, s, r, seimuSage, 5)
	if countItem(r.Items, seimuLight) != 0 {
		t.Fatalf("没有先见之光时仙人不应给东西，实际 %v", r.Items)
	}

	r = kingRecord(kingItems(seimuLight), []int{seimuAdult, seimuDream32, seimuDream33, seimuDream34}, nil)
	kingRun(t, r, seimuPick(t, s, r, seimuElder, 3)) // 三个都完成后，有先见之光先走回收块
	if countItem(r.Items, seimuLight) != 0 {
		t.Fatalf("三任务完成后老人应收走多余的先见之光，实际 %v", r.Items)
	}
}

// 原版行为：三个任务完成后，空手找老人拿先见之光，再找仙人换随机道具；可以反复做（LV>34 或三任务完成即可）。
func TestDreamRepeatRandomGift(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{seimuAdult, seimuDream32, seimuDream33, seimuDream34}, nil)
	for i := 0; i < 2; i++ {
		kingRun(t, r, seimuPick(t, s, r, seimuElder, 4))
		if countItem(r.Items, seimuLight) != 1 {
			t.Fatalf("第 %d 次：老人应给先见之光，实际 %v", i+1, r.Items)
		}
		kingRun(t, r, seimuPick(t, s, r, seimuSage, 4))
		if countItem(r.Items, seimuRandom) != i+1 || countItem(r.Items, seimuLight) != 0 {
			t.Fatalf("第 %d 次：仙人应收走先见之光给随机道具，实际 %v", i+1, r.Items)
		}
	}
}

// 反向：背包满时老人给不了先见之光，台词是 ItemFullMsg，事件不变。
func TestDreamFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, []int{seimuAdult}, nil)
	blk := seimuPick(t, s, r, seimuElder, 2)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if r.eventDoing(seimuDream32) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d flags=%v", len(r.Items), r.EventFlags)
	}
}

// 任务 4 正向：卡坦接任务 → 交 25 级加格、邦恩吉拿到火难的戒指 → 交 25 级克邦凯斯、加克拉换宠物 93。
func TestSharkQuest4Walks(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	blk := seimuPick(t, s, r, seimuKatan, 3)
	if blk.Type != "REQUEST" || blk.Event != seimuShark {
		t.Fatalf("接任务应命中事件 35 的 REQUEST 块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(seimuShark) {
		t.Fatalf("接取后事件 35 应进行中，实际 %v", r.EventFlags)
	}

	r.Pets = seimuPets(seimuGag, seimuBon)
	kingRun(t, r, seimuPick(t, s, r, seimuKatan, 2))
	if countItem(r.Items, seimuRing) != 1 || len(r.Pets) != 0 {
		t.Fatalf("交加格和邦恩吉应拿到 2607、收走两只宠物，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}

	r.Pets = seimuPets(seimuKoban, seimuKakura)
	kingRun(t, r, seimuPick(t, s, r, seimuKatan, 0))
	found := false
	for _, p := range r.Pets {
		if p.TemplateID == seimuGold {
			found = true
		}
	}
	if !found || countItem(r.Items, seimuRing) != 0 || !r.eventDone(seimuShark) {
		t.Fatalf("交克邦凯斯和加克拉应拿到宠物 %d、收走 2607，实际 items=%v pets=%+v flags=%v", seimuGold, r.Items, r.Pets, r.EventFlags)
	}
}

// 反向：宠物等级不是 25 时交不了（命中 REQUEST 的放弃问话），不会拿到火难的戒指。
func TestSharkNeedsLevel25(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, []int{seimuShark})
	r.Pets = []petRec{{Slot: 0, TemplateID: seimuGag, Name: "加格", Level: 24}, {Slot: 1, TemplateID: seimuBon, Name: "邦恩吉", Level: 24}}
	blk := seimuPick(t, s, r, seimuKatan, 3)
	if blk.Type != "REQUEST" || blk.Event != seimuShark {
		t.Fatalf("等级不对应命中 REQUEST（放弃问话），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	if countItem(r.Items, seimuRing) != 0 || len(r.Pets) != 2 {
		t.Fatalf("等级不对不应交走宠物或给戒指，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
}

// 反向：背包满时交加格、邦恩吉拿不到火难的戒指，台词是 ItemFullMsg，宠物不收走。
func TestSharkFullBag(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1})
	}
	r := kingRecord(full, nil, []int{seimuShark})
	r.Pets = seimuPets(seimuGag, seimuBon)
	blk := seimuPick(t, s, r, seimuKatan, 2)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, seimuRing) != 0 || len(r.Pets) != 2 || !r.eventDoing(seimuShark) {
		t.Fatalf("失败后宠物不应被收走，实际 items=%v pets=%d flags=%v", r.Items, len(r.Pets), r.EventFlags)
	}
}

// 原版行为：任务 4 完成后，卡坦还有后续收集——交 25 级火鸡、奇宝换火难的戒指，再交 25 级克克洛斯、
// 霍尔克换宠物 341；这一组可以反复做。照搬，测试固定这一行为。
func TestSharkSideQuestRepeatablePerOriginal(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{seimuShark}, nil)
	r.Pets = seimuPets(seimuFirebird, seimuKiki)
	kingRun(t, r, seimuPick(t, s, r, seimuKatan, 6))
	if countItem(r.Items, seimuRing) != 1 {
		t.Fatalf("交火鸡和奇宝应拿到 2607，实际 %v", r.Items)
	}
	r.Pets = seimuPets(seimuKuro, seimuHoluk)
	kingRun(t, r, seimuPick(t, s, r, seimuKatan, 4))
	found := false
	for _, p := range r.Pets {
		if p.TemplateID == seimuSidePet {
			found = true
		}
	}
	if !found || countItem(r.Items, seimuRing) != 0 {
		t.Fatalf("交克克洛斯和霍尔克应拿到宠物 %d，实际 items=%v pets=%+v", seimuSidePet, r.Items, r.Pets)
	}
}

// 数据检查：三个脚本在白名单里、整本支持；NPC 是任务类型且在正式地图上；四条任务的完成事件对得上；
// 道具和宠物模板都查得到；先见之光和三个奖励来自老人/仙人，火难的戒指来自卡坦，宠物 93 是任务 4 的奖励。
func TestSeimuScriptData(t *testing.T) {
	files := map[int]string{
		seimuElder:   "seimu/event/event01_1",
		seimuSage:    "seimu/event/event01_2",
		seimuSageAlt: "seimu/event/event01_2",
		seimuKatan:   "seimu/event/event02_1",
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
	for no, event := range map[int]int{32: seimuDream32, 33: seimuDream33, 34: seimuDream34, 4: seimuShark} {
		if q, ok := gamedata.QuestByNo(no); !ok || q.EndEvent != event {
			t.Errorf("任务 %d 的完成事件应是 %d，实际 %+v", no, event, q)
		}
	}
	elder, _ := gamedata.NpcEventOf(seimuElder)
	if elder.Blocks[2].GetItem[0].ID != seimuLight {
		t.Errorf("老人派任务 32 的块应给先见之光，实际 %+v", elder.Blocks[2].GetItem)
	}
	sage, _ := gamedata.NpcEventOf(seimuSage)
	if sage.Blocks[3].GetItem[0].ID != seimuHarp || sage.Blocks[2].GetItem[0].ID != seimuHat || sage.Blocks[1].GetItem[0].ID != seimuCloth {
		t.Errorf("仙人的三个交任务块应分别给竖琴、帽子、衣服，实际 %+v", sage.Blocks)
	}
	katan, _ := gamedata.NpcEventOf(seimuKatan)
	if e, _ := gamedata.EnemyByID(katan.Blocks[0].GetPet[0]); e.Template != seimuGold {
		t.Errorf("任务 4 的奖励宠物模板应是 %d，实际 %+v", seimuGold, e)
	}
}
