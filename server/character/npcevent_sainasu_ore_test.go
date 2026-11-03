package character

// STAGE-335：柯尔克坑道采石鉴定（nevent03_1~6）与岳之圣石（任务 43，事件 55）。
// 端到端（纯逻辑，不连数据库）：真实白名单、真实选块 pickEventBlock 和真实交易 planEvent，每步断言命中的块、
// 道具和宠物增减、事件位。正向：矿工给矿石 → 三个鉴定家换鉴定书 → 办事处只说明 → 泰坦队长用 2474 和鲁乌换岳之圣石。
// 反向：矿石不够、矿石和鉴定家对不上、背包满、鲁乌不到 30 级、没有 2474、任务 43 没进行中、已完成再来。
// 任务 43 的入口在原版数据里缺失（TestNevent03EntryGap），所以岳之圣石那一段的前置由测试直接设成进行中；
// 其余步骤都是真实脚本的结算。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	nevMiner  = 1343 // 矿工（nevent03_4，10603 层 45,23）：没有矿石时给 1 个 2471
	nevLv1    = 1344 // 一级矿石鉴定家（nevent03_1，10605 层 42,32）：2469 换 2472
	nevLv2    = 1345 // 特级矿石鉴定家（nevent03_2，10605 层 42,34）：2470 换 2473
	nevLv3    = 1346 // 最高级矿石鉴定家（nevent03_3，10605 层 42,36）：2471 换 2474
	nevOffice = 1248 // 办事处负责人（nevent03_5，1212 层 17,15）：只说明
	nevClerk  = 1247 // 职员（nevent03_6，1212 层 11,4）：只说明

	nevOre1   = 2469 // 矿石（给一级鉴定家）
	nevOre2   = 2470 // 矿石（给特级鉴定家）
	nevOre3   = 2471 // 最高级矿石（给最高级鉴定家；矿工也给）
	nevCert1  = 2472 // 一级鉴定书
	nevCert2  = 2473 // 特级鉴定书
	nevCert3  = 2474 // 超特级鉴定书（= poruCert，岳之圣石的材料）
	nevFiller = 2417 // 占满背包用的无关道具
)

// nevOres 造 n 件同编号的矿石，一件一格（bagCount 按格数累计）。
func nevOres(id, n int) []itemRec {
	ids := make([]int, n)
	for i := range ids {
		ids[i] = id
	}
	return kingItems(ids...)
}

// nevFullBag 把 keep 放在背包最前面，其余格子全部占满（用无关道具），背包满时交易的空位检查才会失败。
func nevFullBag(keep []itemRec) []itemRec {
	out := append([]itemRec{}, keep...)
	for slot := firstBagSlot + len(keep); slot <= MaxItemSlot; slot++ {
		out = append(out, itemRec{Slot: slot, ItemID: nevFiller, Count: 1})
	}
	return out
}

// nevTalk 对 npc 说话：命中第 want 块（-1 表示闲聊，由 poruPick 断言），命中就按原版结算。
func nevTalk(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
	t.Helper()
	blk := poruPick(t, s, r, npc, want)
	if blk != nil {
		kingRun(t, r, blk)
	}
	return blk
}

// nevChainRecord 造任务 41 进行中（事件 64）、任务 43 进行中（事件 55，测试直接设定，见文件头）、身上带一只 30 级鲁乌的记录。
func nevChainRecord(items []itemRec) *record {
	r := kingRecord(items, nil, []int{poruQ41, poruQ43})
	r.Pets = []petRec{{Slot: 0, TemplateID: poruPet44, Name: "鲁乌", Level: 30}}
	return r
}

// 矿工：没有矿石时给 1 个（原版 LV>0&ITEM!=2471）；已经有了就只说话；丢掉或用掉后再说话还能再领一个
// （原版如此，docs/quest.md 记了）。每次最多一个，所以不会攒出多个。
func TestNevent03MinerOreOnce(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	nevTalk(t, s, r, nevMiner, 0)
	if n := countItem(r.Items, nevOre3); n != 1 {
		t.Fatalf("第一次对话应给 1 个 2471，实际 %d", n)
	}
	nevTalk(t, s, r, nevMiner, 1)
	if n := countItem(r.Items, nevOre3); n != 1 {
		t.Fatalf("已有矿石时不应再给，实际 %d", n)
	}
	r.Items = nil
	nevTalk(t, s, r, nevMiner, 0)
	if n := countItem(r.Items, nevOre3); n != 1 {
		t.Fatalf("丢掉后应能再领 1 个（原版行为），实际 %d", n)
	}
}

// 反向：背包满时矿工拿不到矿石（ItemFullMsg），交易不成立，什么都不改。
func TestNevent03MinerBagFull(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Items = nevFullBag(nil)
	blk := poruPick(t, s, r, nevMiner, 0)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("背包满时应说 ItemFullMsg 且不给矿石，实际 ok=%v fail=%q", ok, fail)
	}
	if n := countItem(r.Items, nevOre3); n != 0 {
		t.Fatalf("背包满时不应有矿石，实际 %d", n)
	}
}

// 鉴定按矿石数量分档（原版 ITEM=n 是“至少 n 个”，按块顺序先命中的生效）：
// 14 个换 4 张，全部 14 个收走；11～13 个换 3 张，只收 11 个（原版 DelItem 的数量，剩下的留着）；
// 8～10 个换 2 张、5～7 个换 1 张：一、特级是把这种矿石全部收走（原版 DelItem 不带数量），
// 最高级（nevent03_3）原版写的是 DelItem:2471*8、*5，只收 8 个或 5 个，剩下的留着；1～4 个只说话。
func TestNevent03AssayTiers(t *testing.T) {
	s := kingService()
	cases := []struct {
		name            string
		npc, ore, cert  int
		have, wantBlock int
		wantCert, left  int
	}{
		{"一级 14 个", nevLv1, nevOre1, nevCert1, 14, 0, 4, 0},
		{"一级 12 个", nevLv1, nevOre1, nevCert1, 12, 1, 3, 1},
		{"一级 10 个", nevLv1, nevOre1, nevCert1, 10, 2, 2, 0},
		{"一级 6 个", nevLv1, nevOre1, nevCert1, 6, 3, 1, 0},
		{"一级 3 个", nevLv1, nevOre1, nevCert1, 3, 4, 0, 3},
		{"特级 14 个", nevLv2, nevOre2, nevCert2, 14, 0, 4, 0},
		{"特级 11 个", nevLv2, nevOre2, nevCert2, 11, 1, 3, 0},
		{"特级 8 个", nevLv2, nevOre2, nevCert2, 8, 2, 2, 0},
		{"特级 5 个", nevLv2, nevOre2, nevCert2, 5, 3, 1, 0},
		{"特级 2 个", nevLv2, nevOre2, nevCert2, 2, 4, 0, 2},
		{"最高级 14 个", nevLv3, nevOre3, nevCert3, 14, 0, 4, 0},
		{"最高级 13 个", nevLv3, nevOre3, nevCert3, 13, 1, 3, 2},
		{"最高级 9 个", nevLv3, nevOre3, nevCert3, 9, 2, 2, 1},
		{"最高级 7 个", nevLv3, nevOre3, nevCert3, 7, 3, 1, 2},
		{"最高级 1 个", nevLv3, nevOre3, nevCert3, 1, 4, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := kingRecord(nevOres(c.ore, c.have), nil, nil)
			nevTalk(t, s, r, c.npc, c.wantBlock)
			if n := countItem(r.Items, c.cert); n != c.wantCert {
				t.Fatalf("应换到 %d 张鉴定书 %d，实际 %d", c.wantCert, c.cert, n)
			}
			if n := countItem(r.Items, c.ore); n != c.left {
				t.Fatalf("矿石应剩 %d 个，实际 %d", c.left, n)
			}
		})
	}
}

// 反向：矿石和鉴定家对不上（14 个最高级矿石去找一级鉴定家）没有块命中，矿石和鉴定书都不动；
// 拿去对的鉴定家才换。
func TestNevent03WrongOreNoExchange(t *testing.T) {
	s := kingService()
	r := kingRecord(nevOres(nevOre3, 14), nil, nil)
	nevTalk(t, s, r, nevLv1, -1)
	if countItem(r.Items, nevOre3) != 14 || countItem(r.Items, nevCert1) != 0 {
		t.Fatalf("矿石和鉴定家对不上时不应交易，实际 %v", r.Items)
	}
	nevTalk(t, s, r, nevLv3, 0)
	if countItem(r.Items, nevCert3) != 4 || countItem(r.Items, nevOre3) != 0 {
		t.Fatalf("拿去最高级鉴定家应换 4 张 2474，实际 %v", r.Items)
	}
}

// 满包时鉴定家照样能换：交易是先收走矿石、再算空位，14 个矿石收走后腾出 14 格，够放 4 张鉴定书。
func TestNevent03AssayBagFullStillSwaps(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	r.Items = nevFullBag(nevOres(nevOre3, 14))
	nevTalk(t, s, r, nevLv3, 0)
	if countItem(r.Items, nevCert3) != 4 {
		t.Fatalf("矿石收走后腾出 14 个空位，满包也应换到 4 张，实际 %v", r.Items)
	}
}

// 办事处和职员只说明：拿着鉴定书或矿石去办事处，只说一句，东西一样不少；没有东西时也只说一句。
// 现金兑换不在本需求里（那段脚本没有放开）。
func TestNevent03OfficeAndClerkTalkOnly(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(nevCert1, nevCert2, nevCert3), nil, nil)
	nevTalk(t, s, r, nevOffice, 0)
	if len(r.Items) != 3 || countItem(r.Items, nevCert3) != 1 {
		t.Fatalf("拿着鉴定书去办事处不应改动背包，实际 %v", r.Items)
	}
	r = kingRecord(nevOres(nevOre1, 2), nil, nil)
	nevTalk(t, s, r, nevOffice, 1)
	if countItem(r.Items, nevOre1) != 2 {
		t.Fatalf("拿着矿石去办事处不应收走，实际 %v", r.Items)
	}
	r = kingRecord(nil, nil, nil)
	nevTalk(t, s, r, nevOffice, 2)
	nevTalk(t, s, r, nevClerk, 0)
	if len(r.Items) != 0 || len(r.Pets) != 0 {
		t.Fatalf("办事处和职员不应给东西，实际 %v", r.Items)
	}
}

// 正向整链：矿石 → 最高级鉴定家换 2474 → 泰坦队长用 2474 和 30 级鲁乌换岳之圣石 18556（另给两个队的通行证），
// 事件 55 完成，鲁乌和 2474 被收走。
func TestNevent03ChainToStone(t *testing.T) {
	s := kingService()
	r := nevChainRecord(nevOres(nevOre3, 14))
	nevTalk(t, s, r, nevLv3, 0)
	if countItem(r.Items, nevCert3) != 4 || countItem(r.Items, nevOre3) != 0 {
		t.Fatalf("鉴定后应有 4 张 2474、没有矿石，实际 %v", r.Items)
	}
	nevTalk(t, s, r, poruTitan, 5)
	for _, id := range []int{poruStone, 18554, 18555} {
		if countItem(r.Items, id) != 1 {
			t.Fatalf("交任务后应拿到 %d 一个，实际 %v", id, r.Items)
		}
	}
	if countItem(r.Items, nevCert3) != 0 {
		t.Fatalf("2474 应被收走，实际 %v", r.Items)
	}
	if len(r.Pets) != 0 {
		t.Fatalf("鲁乌应被收走，实际 %+v", r.Pets)
	}
	if !r.eventDone(poruQ43) || r.eventDoing(poruQ43) {
		t.Fatalf("事件 55 应完成且不再进行中，实际 %v", r.EventFlags)
	}
}

// 反向：鲁乌不到 30 级：泰坦队长只说“要好好接受训练”，2474 和鲁乌都还在，没有岳之圣石。
func TestNevent03StoneNeedsLv30(t *testing.T) {
	s := kingService()
	r := nevChainRecord(kingItems(nevCert3))
	r.Pets[0].Level = 29
	nevTalk(t, s, r, poruTitan, 6)
	if countItem(r.Items, poruStone) != 0 || countItem(r.Items, nevCert3) != 1 || len(r.Pets) != 1 {
		t.Fatalf("鲁乌不到 30 级不应换岳之圣石，实际 items=%v pets=%+v", r.Items, r.Pets)
	}
	if !r.eventDoing(poruQ43) {
		t.Fatalf("事件 55 应仍然进行中")
	}
}

// 反向：背包满（岳之圣石要 3 个空位，收走 2474 只腾出 1 个）：交易不成立，2474 和鲁乌都还在。
func TestNevent03StoneBagFull(t *testing.T) {
	s := kingService()
	r := nevChainRecord(nevFullBag(kingItems(nevCert3)))
	blk := poruPick(t, s, r, poruTitan, 5)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("背包满时应说 ItemFullMsg 且不交易，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, nevCert3) != 1 || len(r.Pets) != 1 || !r.eventDoing(poruQ43) {
		t.Fatalf("背包满时 2474、鲁乌和事件 55 都应不变，实际 items=%v pets=%+v", r.Items, r.Pets)
	}
}

// 反向：没有 2474（任务 43 进行中、鲁乌也够 30 级）：泰坦队长没有块命中，只闲聊。
func TestNevent03StoneNeedsCert(t *testing.T) {
	s := kingService()
	r := nevChainRecord(nil)
	nevTalk(t, s, r, poruTitan, -1)
	if countItem(r.Items, poruStone) != 0 || len(r.Pets) != 1 {
		t.Fatalf("没有 2474 时不应给岳之圣石，实际 items=%v pets=%+v", r.Items, r.Pets)
	}
}

// 反向：任务 43 没进行中（只有 41 在进行中）：交 2474 的块要求 NOWEV=55，一块都不命中。
func TestNevent03StoneNeedsTask43Doing(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(nevCert3), nil, []int{poruQ41})
	r.Pets = []petRec{{Slot: 0, TemplateID: poruPet44, Name: "鲁乌", Level: 30}}
	nevTalk(t, s, r, poruTitan, -1)
	if countItem(r.Items, poruStone) != 0 || countItem(r.Items, nevCert3) != 1 {
		t.Fatalf("任务 43 没接时不应换岳之圣石，实际 %v", r.Items)
	}
}

// 已完成再来：岳之圣石只给一次。事件 55 完成后，再交一张 2474（新的鉴定书）也只说话，不再给岳之圣石。
func TestNevent03StoneOnlyOnce(t *testing.T) {
	s := kingService()
	r := nevChainRecord(nevOres(nevOre3, 14))
	nevTalk(t, s, r, nevLv3, 0)
	nevTalk(t, s, r, poruTitan, 5)
	nevTalk(t, s, r, nevLv3, -1) // 没有矿石了，只闲聊
	r.Items = append(r.Items, itemRec{Slot: firstBagSlot + len(r.Items), ItemID: nevCert3, Count: 1})
	nevTalk(t, s, r, poruTitan, 4)
	if countItem(r.Items, poruStone) != 1 || countItem(r.Items, nevCert3) != 1 {
		t.Fatalf("完成后再来不应再给岳之圣石，实际 %v", r.Items)
	}
}

// 任务 43 的入口（原版数据）：没有任何 REQUEST 块的事件号是 55；能把 55 设成进行中的只有 MESSAGE 块，
// 而且它们的每个条件都要求 NOWEV=55（已经在进行中），所以只会是空操作，不是入口（mary14 的事件 55 块就是这种）。
// ACCEPT 不设进行中（eventAccept）。这是原版数据的缺口，不是转换错误；这里固定住这个结论，
// 以后有人补入口时这个测试会提醒更新 docs/quest.md。
func TestNevent03EntryGap(t *testing.T) {
	for _, s := range gamedata.NpcEventScripts() {
		for i := range s.Blocks {
			b := &s.Blocks[i]
			if b.Event != poruQ43 {
				continue
			}
			if b.Type == "REQUEST" {
				t.Fatalf("%s 块 %d 是 REQUEST 55，入口结论过时，请更新 docs/quest.md", s.File, i)
			}
			if b.Type == "MESSAGE" && !onlyFromDoing(b.Cond, poruQ43) {
				t.Fatalf("%s 块 %d 能把 55 设成进行中（条件不要求 NOWEV=55），入口结论过时", s.File, i)
			}
		}
	}
}

// onlyFromDoing 报告每个条件组都要求 NOWEV=ev（进行中），即这个块只在 ev 已经进行中时才会命中。
func onlyFromDoing(cond [][]gamedata.EventTerm, ev int) bool {
	if len(cond) == 0 {
		return false
	}
	for _, group := range cond {
		found := false
		for _, t := range group {
			if t.Op == "NOWEV" && t.Cmp == "=" && t.N == ev {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// 数据检查：六个脚本在白名单里、整本支持（没有 Unsupported、没有缺道具），NPC 编号和脚本文件对得上。
func TestNevent03ScriptData(t *testing.T) {
	want := map[int]string{
		nevMiner:  "sainasu/event/nevent03_4",
		nevLv1:    "sainasu/event/nevent03_1",
		nevLv2:    "sainasu/event/nevent03_2",
		nevLv3:    "sainasu/event/nevent03_3",
		nevOffice: "sainasu/event/nevent03_5",
		nevClerk:  "sainasu/event/nevent03_6",
	}
	for npc, file := range want {
		script, ok := gamedata.NpcEventOf(npc)
		if !ok || script.File != file {
			t.Fatalf("NPC %d 应是 %s，实际 ok=%v", npc, file, ok)
		}
		if !gamedata.NpcEventEnabled(file) {
			t.Fatalf("%s 应在白名单里", file)
		}
		for i := range script.Blocks {
			b := &script.Blocks[i]
			if b.Unsupported != "" || b.MissingItem {
				t.Fatalf("%s 块 %d 不支持（%q, 缺道具=%v）", file, i, b.Unsupported, b.MissingItem)
			}
		}
	}
}
