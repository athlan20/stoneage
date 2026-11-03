package character

// STAGE-220：精灵王的传说（king 目录，13 个脚本，docs/quest.md「白名单与分批」）的端到端。用真实白名单，仿造斧链
// （npcevent_chain_test.go）的写法：每一步用真实的选块（pickEventBlock）和交易（planEvent），断言命中的块序号、
// 道具、宠物和进度。纯逻辑，不需要数据库；协议层的对话和落库在 npcevent_king_flow_test.go。
// 前置：任务 1 成人仪式（事件 4）已完成（每块的条件都有 ENDEV=4，本测试不走仪式）；king 脚本里没有 LV 条件，
// 所以从 1 级开始也能走完。

import (
	"strconv"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

var kingNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)

// 精灵王的传说里的 NPC 编号（server/gamedata/npcs.tsv；地图和坐标见 docs/quest.md 的 king 表）。
const (
	kingGrandpa = 1258 // 愿藏的祖父（2000，event69_3）
	kingGrandma = 1257 // 愿藏的祖母（2000，event69_7）
	kingShin    = 1420 // 新藏（30602，event69_4）
	kingFrog    = 1421 // 里昂蛙王（30607，event69_6）
	kingMarina  = 1422 // 海精灵玛丽娜丝（30619，event69_11）
	kingSamgil  = 1423 // 英雄萨姆吉尔（30619，event69_12）
	kingKohemut = 1424 // 黑暗精灵柯黑穆肯（30670，event69_24）
	kingMaleifa = 1425 // 地精灵玛蕾菲雅（30691，event69_14）
	kingYarus   = 1426 // 地守护雅尔丝（30696，event69_16）
	kingWatiel  = 1427 // 水守护瓦琦儿（30697，event69_18）
	kingFeary   = 1428 // 火守护费尔叶（30698，event69_20）
	kingWindeth = 1429 // 风守护威恩德（30699，event69_22）
)

// 道具编号（server/gamedata/items.go 查得到的名字见注释）。
const (
	kingAmulet      = 19621 // 库伊的闪亮护身符
	kingFrogGold    = 19622 // 黑蛙王的金珠
	kingBlackJade   = 19623 // 黑玉
	kingNecklace    = 19624 // 项链
	kingEarthStone  = 19625 // 地灵石
	kingWaterStone  = 19626 // 水灵石
	kingEarthNeck   = 19629 // 地灵项链（灵石项链，随机）
	kingArmor       = 1292  // 真．灵力铠
	kingFeather     = 13061 // 精灵的羽毛（13061–13064 随机给一件）
	kingFourColor   = 19645 // 四色原石
	kingFirstRing   = 19641 // 地魔法戒指（四个守护各一枚，19641–19644）
	kingRingFromWat = 19642 // 水魔法戒指
)

// 怪物编号（GetPet 用的是怪物编号，宠物模板在 npc 数据之外）：1479 玛蕾菲雅（模板 718）、484 邦达霍恩（模板 315）、
// 355 摩娜西普（模板 312，心性之镜那条线，不在精灵王的传说主链上）。
const (
	kingEnemyMaleifa = 1479
	kingTmplMaleifa  = 718
	kingEnemyBonda   = 484
	kingTmplBonda    = 315
)

// kingRand 是固定的随机源：取第 0 个（GetRandItem 的第一件、GetPet 的第一只）。
func kingRand(int) int { return 0 }

// kingService 是只用选块、交易需要的服务（不连数据库）。
func kingService() *Service {
	return &Service{opts: Options{Now: func() time.Time { return kingNow }}}
}

// kingItems 造背包：每件占一格，从 firstBagSlot 起按顺序放。
func kingItems(ids ...int) []itemRec {
	out := make([]itemRec, 0, len(ids))
	for i, id := range ids {
		out = append(out, itemRec{Slot: firstBagSlot + i, ItemID: id, Count: 1})
	}
	return out
}

// kingRecord 造勇者小明的记录：背包、已完成和进行中的事件号（按 applyEventEdit 写进进度，和脚本的 EndSetFlg
// 一样对应任务表：事件 69 就是任务 49）。
func kingRecord(items []itemRec, done, doing []int) *record {
	p := newProgress()
	for _, ev := range done {
		applyEventEdit(p, editSetDone, ev)
	}
	for _, ev := range doing {
		applyEventEdit(p, editSetDoing, ev)
	}
	return &record{Level: 1, Items: items, Quests: stateRecs(p.quests), EventFlags: stateRecs(p.events)}
}

// kingPick 按真实的选块取 NPC 命中的块，断言命中的块序号（-1 表示没有块、只说闲聊）且没有拒绝。
// 返回命中的块（-1 时为 nil）。
func kingPick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	idx, refuse := s.pickEventBlock(r, script, "")
	if refuse {
		t.Fatalf("NPC %d 应该能对话，实际拒绝（脚本里有不支持的块）", npc)
	}
	if idx != want {
		got := "没有块（闲聊）"
		if idx >= 0 {
			b := &script.Blocks[idx]
			got = "第 " + strconv.Itoa(idx) + " 块（" + b.Type + "，事件 " + strconv.Itoa(b.Event) + "）"
		}
		t.Fatalf("NPC %d（%s）应命中第 %d 块，实际 %s", npc, script.File, want, got)
	}
	if idx < 0 {
		return nil
	}
	return &script.Blocks[idx]
}

// kingRun 按 eventAccept、eventRequestYes、eventMessage 的顺序结算一块：先 planEvent（交易），成立后改背包、
// 金币、宠物，再设进行中（REQUEST、MESSAGE）、EndSetFlg、CleanFlg。交易不成立时测试失败（要测失败的直接调 planEvent）。
func kingRun(t *testing.T, r *record, blk *gamedata.EventBlock) eventPlan {
	t.Helper()
	switch blk.Type {
	case "ACCEPT", "REQUEST":
		if len(blk.Msg.Pages("thanks")) == 0 {
			t.Fatalf("事件 %d 的 %s 块没有感谢话，服务端不会结算，测试假设不成立", blk.Event, blk.Type)
		}
	case "MESSAGE":
		if len(blk.Msg.Pages("nomal")) == 0 && len(blk.Msg.Pages("nomal_window")) == 0 {
			t.Fatalf("事件 %d 的 MESSAGE 块没有台词，服务端不会结算，测试假设不成立", blk.Event)
		}
	}
	plan, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if !ok {
		t.Fatalf("事件 %d（%s 块）的交易不成立：%s", blk.Event, blk.Type, fail)
	}
	prog := progressOf(r)
	if blk.Type == "REQUEST" || blk.Type == "MESSAGE" {
		applyEventEdit(prog, editSetDoing, blk.Event)
	}
	endSetFlg(prog, blk.Event, blk.EndSet)
	if blk.Type == "ACCEPT" {
		keepDoing(prog, blk) // 与 eventAccept 一致（STAGE-341）
	}
	cleanFlg(prog, blk.Clean)
	r.Items, r.Gold, r.Pets = plan.items, plan.gold, plan.pets
	r.Quests, r.EventFlags = stateRecs(prog.quests), stateRecs(prog.events)
	return plan
}

// kingTalk 只是选块并确认命中的块序号（用于闲聊、催促这类不改东西的话）。
func kingTalk(t *testing.T, s *Service, r *record, npc, want int) {
	t.Helper()
	kingPick(t, s, r, npc, want)
}

// 任务 49→50 全链（事件 69、70）：愿藏的祖父接 69 → 新藏给金珠 → 里昂蛙王换黑玉 → 新藏接 70 → 新藏结束 69 →
// 愿藏的祖母给项链和玛蕾菲雅（宠物 1479），事件 70 完成。之后各人只剩闲谈，不再发奖。
func TestKingChain49To50(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil)

	// 1. 愿藏的祖父：还没接，接任务 69（REQUEST，不交易，设进行中）。
	blk := kingPick(t, s, r, kingGrandpa, 10)
	if blk.Type != "REQUEST" || blk.Event != 69 {
		t.Fatalf("祖父的第一块应是接任务 69（REQUEST），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(69) {
		t.Fatalf("接了任务，事件 69 应进行中，实际 %v", r.EventFlags)
	}

	// 2. 新藏：没有金珠，给金珠（ACCEPT，没有事件号，不结束 69）。
	blk = kingPick(t, s, r, kingShin, 4)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("新藏应给金珠（ACCEPT 没有事件），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, kingFrogGold) != 1 || r.eventDone(69) {
		t.Fatalf("应拿到金珠且 69 仍进行中，实际 %v", r.Items)
	}

	// 3. 里昂蛙王：拿金珠换黑玉（金珠收走，黑玉到手）。
	blk = kingPick(t, s, r, kingFrog, 1)
	kingRun(t, r, blk)
	if countItem(r.Items, kingFrogGold) != 0 || countItem(r.Items, kingBlackJade) != 1 {
		t.Fatalf("换完应没有金珠、有 1 个黑玉，实际 %v", r.Items)
	}

	// 3b. 里昂蛙王：黑玉已经有了，只催促去找新藏（不再换）。
	kingTalk(t, s, r, kingFrog, 0)

	// 4. 新藏：拿黑玉换情报，接任务 70（REQUEST，收走黑玉，设进行中）。
	blk = kingPick(t, s, r, kingShin, 2)
	if blk.Type != "REQUEST" || blk.Event != 70 {
		t.Fatalf("新藏应接任务 70（REQUEST），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, kingBlackJade) != 0 || !r.eventDoing(70) {
		t.Fatalf("接 70 后应收走黑玉、70 进行中，实际 items=%v 70doing=%v", r.Items, r.eventDoing(70))
	}

	// 5. 新藏：70 和 69 都进行中，结束 69（ACCEPT，设完成）。
	blk = kingPick(t, s, r, kingShin, 1)
	kingRun(t, r, blk)
	if !r.eventDone(69) || r.eventDoing(69) {
		t.Fatalf("新藏结束后事件 69 应已完成，实际 %v", r.EventFlags)
	}

	// 6. 愿藏的祖母：70 进行中、69 已完成，给项链（19624）和玛蕾菲雅（宠物 1479），事件 70 完成。
	blk = kingPick(t, s, r, kingGrandma, 12)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("祖母应给项链和宠物（ACCEPT），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	plan := kingRun(t, r, blk)
	if countItem(r.Items, kingNecklace) != 1 {
		t.Fatalf("应拿到项链 19624，实际 %v", r.Items)
	}
	if len(r.Pets) != 1 || r.Pets[0].TemplateID != kingTmplMaleifa || r.Pets[0].Slot != 0 {
		t.Fatalf("应拿到 0 号栏位的玛蕾菲雅（模板 %d），实际 %+v", kingTmplMaleifa, r.Pets)
	}
	if r.Pets[0].EventGiven {
		t.Fatalf("ACCEPT 送的宠物不打任务送的标记（原版 NPC_AcceptDel 不设 CHAR_ENDEVENT）")
	}
	if len(plan.notes) != 1 || plan.notes[0] != "拿到"+r.Pets[0].Name+"。" {
		t.Fatalf("应有一条“拿到XX。”提示，实际 %v", plan.notes)
	}
	if !r.eventDone(70) || r.eventDoing(70) {
		t.Fatalf("给完之后事件 70 应已完成，实际 %v", r.EventFlags)
	}

	// 7. 之后全部只闲谈：祖母（项链还在）、祖父（接玛蕾菲雅的闲谈）、新藏（没有块）、蛙王（没有块）。
	kingTalk(t, s, r, kingGrandma, 11)
	kingTalk(t, s, r, kingGrandpa, 6)
	kingTalk(t, s, r, kingShin, -1)
	kingTalk(t, s, r, kingFrog, -1)
	if countItem(r.Items, kingNecklace) != 1 || len(r.Pets) != 1 {
		t.Fatalf("闲谈不应改动背包和宠物，实际 items=%v pets=%d", r.Items, len(r.Pets))
	}
}

// 任务 51 之后（事件 122、123、71、72）：事件 71 由宠物转生 NPC 设进行中（STAGE-198 未做，这里直接设上），
// 海精灵收原石换四色原石、收宝石换灵力铠（DelItem 末尾逗号把列出的道具全收走），然后玛蕾菲雅接 72、
// 四个守护给魔戒、柯黑穆肯给宠物 484 并结束 72。完成 71 发生在玛蕾菲雅接 72 之后再对话（块 3）。
func TestKingChain51To54(t *testing.T) {
	s := kingService()
	// 49、50 已完成（事件 69、70）；事件 71 进行中（相当于把任务 51 设为进行中）。
	r := kingRecord(nil, []int{4, 69, 70}, []int{71})

	// 1. 海精灵：什么都没有，只说故事（ACCEPT，没有交易）。
	kingTalk(t, s, r, kingMarina, 5)

	// 1b. 只有一颗原石：催促（MESSAGE）。
	r.Items = kingItems(19633)
	kingTalk(t, s, r, kingMarina, 4)

	// 1c. 四种原石都有：换四色原石（收走 19633–19636，给 19645）。
	r.Items = kingItems(19633, 19634, 19635, 19636)
	blk := kingPick(t, s, r, kingMarina, 3)
	kingRun(t, r, blk)
	if countItem(r.Items, kingFourColor) != 1 || countItem(r.Items, 19633) != 0 {
		t.Fatalf("应收走四原石、给四色原石，实际 %v", r.Items)
	}

	// 1d. 只有四色原石：催促收集四宝石。
	kingTalk(t, s, r, kingMarina, 2)

	// 2. 四宝石和四色原石都有：给灵力铠 1292，收走 19637–19640 和 19645（DelItem 末尾逗号），事件 122、123 完成。
	r.Items = append(r.Items, kingItems(19637, 19638, 19639, 19640)...)
	for i := range r.Items { // 重新编号，保证槽位不重复
		r.Items[i].Slot = firstBagSlot + i
	}
	blk = kingPick(t, s, r, kingMarina, 1)
	if len(blk.DelItem) != 9 {
		t.Fatalf("DelItem 应列出 9 个道具（19633–19636、19637–19640、19645，末尾逗号不算），实际 %d", len(blk.DelItem))
	}
	kingRun(t, r, blk)
	if countItem(r.Items, kingArmor) != 1 || len(r.Items) != 1 {
		t.Fatalf("应只剩灵力铠，实际 %v", r.Items)
	}
	if !r.eventDone(122) || !r.eventDone(123) {
		t.Fatalf("做完灵力铠，事件 122、123 应已完成，实际 %v", r.EventFlags)
	}

	// 2b. 灵力铠到手后，海精灵和萨姆吉尔只闲谈。
	kingTalk(t, s, r, kingMarina, 0)
	kingTalk(t, s, r, kingSamgil, 0)

	// 3. 玛蕾菲雅：71 进行中、72 没接，接任务 72（REQUEST，不交易，设进行中）。
	blk = kingPick(t, s, r, kingMaleifa, 4)
	if blk.Type != "REQUEST" || blk.Event != 72 {
		t.Fatalf("玛蕾菲雅应接任务 72（REQUEST），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if !r.eventDoing(72) || !r.eventDoing(71) {
		t.Fatalf("接 72 后 71、72 都应进行中，实际 %v", r.EventFlags)
	}

	// 3b. 玛蕾菲雅：71、72 都进行中，给羽毛（ACCEPT，随机一件，设完成 71）。
	blk = kingPick(t, s, r, kingMaleifa, 3)
	kingRun(t, r, blk)
	if !r.eventDone(71) || r.eventDoing(71) || countItem(r.Items, kingFeather) != 1 {
		t.Fatalf("应完成 71 并拿到精灵的羽毛，实际 items=%v events=%v", r.Items, r.EventFlags)
	}
	if !r.eventDoing(72) {
		t.Fatalf("完成 71 不应结束 72")
	}
	kingTalk(t, s, r, kingMaleifa, 2)

	// 4. 四个守护：各给一枚魔法戒指（19641–19644），之后只说话。
	for _, g := range []struct{ npc, ring int }{
		{kingYarus, kingFirstRing},
		{kingWatiel, kingRingFromWat},
		{kingFeary, kingFirstRing + 2},
		{kingWindeth, kingFirstRing + 3},
	} {
		blk = kingPick(t, s, r, g.npc, 1)
		kingRun(t, r, blk)
		if countItem(r.Items, g.ring) != 1 {
			t.Fatalf("守护 %d 应给戒指 %d，实际 %v", g.npc, g.ring, r.Items)
		}
		kingTalk(t, s, r, g.npc, 0)
	}

	// 5. 黑暗精灵柯黑穆肯：72 进行中、71 已完成，给宠物 484（邦达霍恩）并结束 72。
	blk = kingPick(t, s, r, kingKohemut, 1)
	kingRun(t, r, blk)
	if !r.eventDone(72) || r.eventDoing(72) {
		t.Fatalf("给完宠物后 72 应已完成，实际 %v", r.EventFlags)
	}
	if len(r.Pets) != 1 || r.Pets[0].TemplateID != kingTmplBonda {
		t.Fatalf("应拿到邦达霍恩（模板 %d），实际 %+v", kingTmplBonda, r.Pets)
	}

	// 6. 全部完成后只闲谈。
	kingTalk(t, s, r, kingKohemut, 0)
	kingTalk(t, s, r, kingMaleifa, 0)
	kingTalk(t, s, r, kingYarus, 0)
}

// 灵石项链（事件 70 之后，愿藏的祖母的 7–10 块）：一颗灵石换一条随机的灵石项链，灵石收走、项链留下；
// 身上已经有任意一条灵石项链，再换（哪种灵石都一样）就被拒（块 3–6，说“你已经有一条灵石项链了”），什么都不收。
func TestKingGemNecklaceExchange(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(kingNecklace, kingEarthStone, kingWaterStone), []int{4, 69, 70}, nil)

	// 地灵石：收走地灵石，随机给一条地灵项链（rnd 取 0 得 19629）。
	blk := kingPick(t, s, r, kingGrandma, 7)
	kingRun(t, r, blk)
	if countItem(r.Items, kingEarthStone) != 0 || countItem(r.Items, kingEarthNeck) != 1 {
		t.Fatalf("应收走地灵石、给地灵项链，实际 %v", r.Items)
	}
	if countItem(r.Items, kingNecklace) != 1 || countItem(r.Items, kingWaterStone) != 1 {
		t.Fatalf("项链和水灵石应留下，实际 %v", r.Items)
	}

	// 身上有地灵项链：拿水灵石来换，被拒（块 3 先命中，它的台词是“你已经有一条灵石项链了”），什么都不收。
	blk = kingPick(t, s, r, kingGrandma, 3)
	if blk.Type != "MESSAGE" {
		t.Fatalf("有灵石项链时应是拒绝的 MESSAGE 块，实际 %s", blk.Type)
	}
	if countItem(r.Items, kingWaterStone) != 1 || countItem(r.Items, kingEarthNeck) != 1 {
		t.Fatalf("被拒后不应收走水灵石，实际 %v", r.Items)
	}
}
