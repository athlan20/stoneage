package character

// STAGE-294：交易的注册表、邀请限制、超时、断线、交换前校验和过户计划（不需要数据库、不需要连接）。
// 数据库相关的测试（事务回滚、过户后的行）在 trade_db_test.go，需要 STONEAGE_TEST_DATABASE_URL。

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/protocol"
)

func newTradeTestService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	s, clk := newPartyTestService(t)
	s.tradeOn.Store(true)
	return s, clk
}

// fakeTradePresence 是一个只有数据、没有连接的角色，背包里有 1 件物品（5 号格）。
func fakeTradePresence(s *Service, id int64, name string, gold int64) *Presence {
	p := fakePartyPresence(s, id, name)
	p.rec.Gold = gold
	p.rec.Floor = 1006
	p.rec.Level = 20
	p.rec.HP = 100
	p.rec.Items = []itemRec{{Slot: 5, ItemID: 1, Count: 1}}
	return p
}

// tradeInvite 让 from 邀请 to（直接走注册表），返回请求。
func tradeInvite(t *testing.T, s *Service, from, to *Presence) *tradeRequest {
	t.Helper()
	r, perr := s.trades.newRequest(tradeInfoOf(from), tradeInfoOf(to))
	if perr != nil {
		t.Fatalf("发出邀请失败: %s", perr.Code)
	}
	return r
}

// tradeAccept 让 to 接受请求，返回交易（失败时返回错误码）。
func tradeAccept(s *Service, r *tradeRequest, to *Presence) (*trade, *protocol.Error) {
	h := s.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.acceptLocked(r, tradeInfoOf(to))
}

func TestTradeInviteLimits(t *testing.T) {
	s, clk := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	r := tradeInvite(t, s, a, b)
	if _, perr := s.trades.newRequest(tradeInfoOf(a), tradeInfoOf(b)); perr == nil || perr.Code != protocol.CodeRequestPending {
		t.Fatalf("同一对待处理的邀请应回 REQUEST_PENDING，得 %v", perr)
	}
	// 撤回（发起人）后 10 秒内再发：频率限制
	h := s.trades
	h.mu.Lock()
	delete(h.requests, r.id)
	h.mu.Unlock()
	if _, perr := s.trades.newRequest(tradeInfoOf(a), tradeInfoOf(b)); perr == nil || perr.Code != protocol.CodeRateLimited {
		t.Fatalf("10 秒内再发应回 RATE_LIMITED，得 %v", perr)
	}
	clk.Add(11 * time.Second)
	tradeInvite(t, s, a, b)
}

func TestTradeInviteBusyAfterFive(t *testing.T) {
	s, clk := newTradeTestService(t)
	b := fakeTradePresence(s, 9, "乙", 0)
	for i := 0; i < tradeMaxIncoming; i++ {
		a := fakeTradePresence(s, int64(100+i), "甲"+string(rune('a'+i)), 0)
		tradeInvite(t, s, a, b)
	}
	extra := fakeTradePresence(s, 200, "丙", 0)
	clk.Add(time.Minute)
	if _, perr := s.trades.newRequest(tradeInfoOf(extra), tradeInfoOf(b)); perr == nil || perr.Code != protocol.CodeTargetBusy {
		t.Fatalf("对方待处理已满 5 条应回 TARGET_BUSY，得 %v", perr)
	}
}

func TestTradeAcceptBuildsTradeAndCancelsOtherInvites(t *testing.T) {
	s, _ := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	c := fakeTradePresence(s, 3, "丙", 0)
	r := tradeInvite(t, s, a, b)
	other := tradeInvite(t, s, c, b) // 乙收到的另一条邀请，接受后作废
	tr, perr := tradeAccept(s, r, b)
	if perr != nil {
		t.Fatalf("接受失败: %s", perr.Code)
	}
	if s.trades.byChar[1] != tr || s.trades.byChar[2] != tr || a.trade != tr || b.trade != tr {
		t.Fatal("交易应当挂在双方的角色上")
	}
	if _, ok := s.trades.requests[other.id]; ok {
		t.Fatal("乙的其他待处理邀请应当作废")
	}
	if _, ok := s.trades.requests[r.id]; ok {
		t.Fatal("被接受的请求应当从请求表里移除（变成交易）")
	}
	if tr.ID != r.id {
		t.Fatalf("交易 id = %s，应沿用请求 id %s", tr.ID, r.id)
	}
}

func TestTradeAcceptRejectsDeadInitiator(t *testing.T) {
	s, _ := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	r := tradeInvite(t, s, a, b)
	a.rec.HP = 0
	_, perr := tradeAccept(s, r, b)
	if perr == nil || perr.Code != protocol.CodeCharacterDead {
		t.Fatalf("发起人倒下应回 CHARACTER_DEAD，得 %v", perr)
	}
	if _, ok := s.trades.requests[r.id]; ok {
		t.Fatal("校验失败的请求应当作废")
	}
	if a.trade != nil || b.trade != nil {
		t.Fatal("校验失败不应建立交易")
	}
}

func TestTradeDisconnectCancelsTrade(t *testing.T) {
	s, _ := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	tr, perr := tradeAccept(s, tradeInvite(t, s, a, b), b)
	if perr != nil {
		t.Fatalf("接受失败: %s", perr.Code)
	}
	s.trades.onGone(a) // 甲断线（最后一条连接）
	if len(s.trades.byChar) != 0 {
		t.Fatalf("断线后注册表应清空，剩 %d 项", len(s.trades.byChar))
	}
	if b.trade != nil {
		t.Fatal("对方的交易引用应当清掉")
	}
	if tr.LastErr != nil {
		t.Fatal("取消的交易不应留下 last_error")
	}
}

func TestTradeIdleTimeoutAndInviteExpiry(t *testing.T) {
	s, clk := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	r := tradeInvite(t, s, a, b)
	clk.Add(tradeRequestTTL)
	s.expireTrades()
	if _, ok := s.trades.requests[r.id]; ok {
		t.Fatal("30 秒后邀请应当过期")
	}
	c := fakeTradePresence(s, 3, "丙", 0)
	tr, perr := tradeAccept(s, tradeInvite(t, s, a, c), c)
	if perr != nil {
		t.Fatalf("接受失败: %s", perr.Code)
	}
	clk.Add(tradeIdleTimeout - time.Second)
	s.expireTrades()
	if s.trades.byChar[1] == nil {
		t.Fatal("空闲未满 5 分钟不应取消")
	}
	clk.Add(time.Second)
	s.expireTrades()
	if s.trades.byChar[1] != nil || s.trades.byChar[3] != nil || a.trade != nil || c.trade != nil {
		t.Fatalf("空闲满 5 分钟应取消交易 %s", tr.ID)
	}
}

func TestTradeCheckBagFullAndSwap(t *testing.T) {
	full := make([]tradeItemRow, 0, 15)
	for slot := tradeBagFirst; slot <= tradeBagLast; slot++ {
		full = append(full, tradeItemRow{rowID: int64(slot), slot: slot, itemID: 1, count: 1})
	}
	recv := &tradeHolding{charID: 2, name: "乙", level: 20, items: full, petRowIDs: map[int]int64{}}
	giver := &tradeHolding{charID: 1, name: "甲", level: 20, gold: 0, items: []tradeItemRow{{rowID: 100, slot: 5, itemID: 1, count: 1}}, petRowIDs: map[int]int64{}}
	// 乙背包满：甲给一件，乙不给 → BAG_FULL（who = 乙）
	fail := tradeCheck(giver, recv, tradeOffer{Items: []int{5}, Pet: -1}, tradeOffer{Pet: -1})
	if fail == nil || fail.code != protocol.CodeBagFull || fail.who != 2 {
		t.Fatalf("背包满应回 BAG_FULL（who=乙），得 %+v", fail)
	}
	// 乙同时交出一件（互换）→ 通过
	if fail := tradeCheck(giver, recv, tradeOffer{Items: []int{5}, Pet: -1}, tradeOffer{Items: []int{7}, Pet: -1}); fail != nil {
		t.Fatalf("互换时腾出的格子应可用，得 %+v", fail)
	}
}

func TestTradeCheckGoldCapIsFlat(t *testing.T) {
	if battle.MaxHaveGold != 10000000 {
		t.Fatalf("MaxHaveGold = %d，应统一为 1000 万", battle.MaxHaveGold)
	}
	recv := &tradeHolding{charID: 2, name: "乙", gold: 9999000, trans: 0, petRowIDs: map[int]int64{}}
	giver := &tradeHolding{charID: 1, name: "甲", gold: 5000, petRowIDs: map[int]int64{}}
	fail := tradeCheck(giver, recv, tradeOffer{Gold: 2000, Pet: -1}, tradeOffer{Pet: -1})
	if fail == nil || fail.code != protocol.CodeGoldFull || fail.who != 2 {
		t.Fatalf("超过 1000 万应回 GOLD_FULL（who=乙），得 %+v", fail)
	}
	recv.trans = 1 // 转生不再抬高上限
	if fail := tradeCheck(giver, recv, tradeOffer{Gold: 2000, Pet: -1}, tradeOffer{Pet: -1}); fail == nil || fail.code != protocol.CodeGoldFull {
		t.Fatalf("转生后上限仍是 1000 万，应回 GOLD_FULL，得 %+v", fail)
	}
	recv.gold = 9998000
	if fail := tradeCheck(giver, recv, tradeOffer{Gold: 2000, Pet: -1}, tradeOffer{Pet: -1}); fail != nil {
		t.Fatalf("正好到 1000 万应通过，得 %+v", fail)
	}
}

func TestTradeCheckPetRules(t *testing.T) {
	five := make([]petRec, 0, MaxPetSlots)
	for slot := 0; slot < MaxPetSlots; slot++ {
		five = append(five, petRec{Slot: slot, Level: 5, State: petStateRest})
	}
	recv := &tradeHolding{charID: 2, name: "乙", level: 20, pets: five, petRowIDs: map[int]int64{}}
	giver := &tradeHolding{charID: 1, name: "甲", level: 20, pets: []petRec{{Slot: 0, Level: 10, State: petStateRest}}, petRowIDs: map[int]int64{0: 77}}
	// 乙宠物栏满，甲给宠物 → PET_FULL
	if fail := tradeCheck(giver, recv, tradeOffer{Pet: 0}, tradeOffer{Pet: -1}); fail == nil || fail.code != protocol.CodePetFull {
		t.Fatalf("宠物栏满应回 PET_FULL，得 %+v", fail)
	}
	// 乙同时交出一只宠物（互换）→ 通过
	if fail := tradeCheck(giver, recv, tradeOffer{Pet: 0}, tradeOffer{Pet: 0}); fail != nil {
		t.Fatalf("互换宠物应通过，得 %+v", fail)
	}
	// 宠物比乙高 6 级以上：未转生的乙不能收
	empty := &tradeHolding{charID: 2, name: "乙", level: 20, petRowIDs: map[int]int64{}}
	strong := &tradeHolding{charID: 1, name: "甲", level: 20, pets: []petRec{{Slot: 0, Level: 26, State: petStateRest}}, petRowIDs: map[int]int64{}}
	if fail := tradeCheck(strong, empty, tradeOffer{Pet: 0}, tradeOffer{Pet: -1}); fail == nil || fail.code != protocol.CodePetTooStrong || fail.who != 2 {
		t.Fatalf("宠物太强应回 PET_TOO_STRONG（who=乙），得 %+v", fail)
	}
	empty.trans = 1
	if fail := tradeCheck(strong, empty, tradeOffer{Pet: 0}, tradeOffer{Pet: -1}); fail != nil {
		t.Fatalf("接收方转生过不受等级差限制，得 %+v", fail)
	}
	// 骑乘中的宠物不能交易
	riding := &tradeHolding{charID: 1, name: "甲", pets: []petRec{{Slot: 0, Level: 5, State: petStateRide}}, petRowIDs: map[int]int64{}}
	if fail := tradeCheck(riding, empty, tradeOffer{Pet: 0}, tradeOffer{Pet: -1}); fail == nil || fail.code != protocol.CodePetRiding {
		t.Fatalf("骑乘中应回 PET_RIDING，得 %+v", fail)
	}
}

func TestTradeCheckRemovesVanishedItems(t *testing.T) {
	giver := &tradeHolding{charID: 1, name: "甲", items: []tradeItemRow{{rowID: 1, slot: 5, itemID: 1, count: 1}}, petRowIDs: map[int]int64{}}
	recv := &tradeHolding{charID: 2, name: "乙", petRowIDs: map[int]int64{}}
	fail := tradeCheck(giver, recv, tradeOffer{Items: []int{5, 7}, Pet: -1}, tradeOffer{Pet: -1})
	if fail == nil || fail.code != protocol.CodeEmptySlot || len(fail.removed) != 1 || fail.removed[0] != 7 || fail.who != 1 {
		t.Fatalf("7 号格已经没有物品，应回 EMPTY_SLOT 并列出 7，得 %+v", fail)
	}
}

func TestTradePlanPlacesIntoFreeSlotsInOrder(t *testing.T) {
	// 乙的 5、6 号有东西，但 6 号这件同时交出去，所以空格是 6、7…
	recv := &tradeHolding{charID: 2, name: "乙", gold: 100, items: []tradeItemRow{
		{rowID: 50, slot: 5, itemID: 1, count: 1}, {rowID: 60, slot: 6, itemID: 1, count: 1},
	}, petRowIDs: map[int]int64{}}
	giver := &tradeHolding{charID: 1, name: "甲", gold: 900, items: []tradeItemRow{
		{rowID: 10, slot: 8, itemID: 1, count: 1}, {rowID: 11, slot: 9, itemID: 1, count: 1},
	}, petRowIDs: map[int]int64{}}
	offA := tradeOffer{Items: []int{8, 9}, Gold: 300, Pet: -1}
	offB := tradeOffer{Items: []int{6}, Gold: 50, Pet: -1}
	if fail := tradeCheck(giver, recv, offA, offB); fail != nil {
		t.Fatalf("校验失败: %+v", fail)
	}
	plan := planTrade(giver, recv, offA, offB)
	if len(plan.moves) != 3 {
		t.Fatalf("过户数 = %d，要 3", len(plan.moves))
	}
	// 甲给的两件去乙的 6、7 号（6 号被乙同时交出）
	if plan.moves[0].to != 2 || plan.moves[0].slot != 6 || plan.moves[1].slot != 7 {
		t.Fatalf("甲的物品去向 = %+v", plan.moves[:2])
	}
	// 乙给的一件去甲的 5 号（甲背包里 5 号空着）
	if plan.moves[2].to != 1 || plan.moves[2].slot != 5 || plan.moves[2].rowID != 60 {
		t.Fatalf("乙的物品去向 = %+v", plan.moves[2])
	}
	if plan.gold[1] != 900-300+50 || plan.gold[2] != 100-50+300 {
		t.Fatalf("石币 = %+v", plan.gold)
	}
}

func TestTradeLockRefusedByPreflightKeepsEditing(t *testing.T) {
	s, _ := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	tr, _ := tradeAccept(s, tradeInvite(t, s, a, b), b)
	reject := func() *protocol.Error { return protocol.Errorf(protocol.CodeBagFull, "对方背包满") }
	if perr := tr.lock(1, tr.Rev, s.opts.Now(), reject); perr == nil || tr.side(1).Locked {
		t.Fatal("预演不过应拒绝锁定，且不改变锁定状态")
	}
}

// STAGE-294：石币超上限对方看到的是“对方石币已达上限”，不是“对方物品栏已满”；自己看到的仍是校验的原话。
func TestTradeFailErrMessageByViewer(t *testing.T) {
	gold := &tradeFail{code: protocol.CodeGoldFull, who: 2, msg: "乙 的石币超过携带上限"}
	if perr := tradeFailErr(gold, 1); perr == nil || perr.Code != protocol.CodeGoldFull || perr.Message != "对方石币已达上限" {
		t.Fatalf("对方石币超上限文案不对：%+v", perr)
	} else if perr.Data.(map[string]any)["who"] != "peer" {
		t.Fatalf("who 应为 peer：%+v", perr.Data)
	}
	if perr := tradeFailErr(&tradeFail{code: protocol.CodeGoldFull, who: 1, msg: "甲 的石币超过携带上限"}, 1); perr == nil || perr.Message != "甲 的石币超过携带上限" {
		t.Fatalf("自己石币超上限应用校验原话：%+v", perr)
	}
	if perr := tradeFailErr(&tradeFail{code: protocol.CodeBagFull, who: 2, msg: "乙 的背包空间不够"}, 1); perr == nil || perr.Message != "对方物品栏已满" {
		t.Fatalf("对方背包满应仍为“对方物品栏已满”：%+v", perr)
	}
	if perr := tradeFailErr(nil, 1); perr != nil {
		t.Fatalf("没有失败应返回 nil：%+v", perr)
	}
}

// STAGE-294：成交摘要按每一方视角生成：甲出 300、乙出 200，甲 gave=300 got=200，乙 gave=200 got=300。
func TestTradeSummariesPerSide(t *testing.T) {
	a := &tradeSide{CharID: 1, Name: "甲", Offer: tradeOffer{Gold: 300, Pet: -1}}
	b := &tradeSide{CharID: 2, Name: "乙", Offer: tradeOffer{Gold: 200, Pet: -1}}
	sums := tradeSummaries(a, b)
	for _, c := range []struct {
		who       int64
		gave, got int64
	}{{1, 300, 200}, {2, 200, 300}} {
		sum := sums[c.who]
		if g := sum["gave"].(map[string]any)["gold"]; g != c.gave {
			t.Errorf("角色 %d 的 gave.gold = %v，应为 %d", c.who, g, c.gave)
		}
		if g := sum["got"].(map[string]any)["gold"]; g != c.got {
			t.Errorf("角色 %d 的 got.gold = %v，应为 %d", c.who, g, c.got)
		}
	}
}

func TestTradeAllowedDuringTradeList(t *testing.T) {
	for _, typ := range []string{"trade_offer", "trade_lock", "trade_confirm", "leave_world"} {
		if !tradeAllowedDuringTrade(typ) {
			t.Fatalf("%s 应当允许在交易中发出", typ)
		}
	}
	for _, typ := range []string{"item_use", "item_drop", "pet_drop", "warp", "encounter_start", "party_invite"} {
		if tradeAllowedDuringTrade(typ) {
			t.Fatalf("%s 交易中应回 IN_TRADE", typ)
		}
	}
}

func TestTradeControlChangedCancelsTrade(t *testing.T) {
	s, _ := newTradeTestService(t)
	a := fakeTradePresence(s, 1, "甲", 0)
	b := fakeTradePresence(s, 2, "乙", 0)
	if _, perr := tradeAccept(s, tradeInvite(t, s, a, b), b); perr != nil {
		t.Fatalf("接受失败: %s", perr.Code)
	}
	s.trades.controlChanged(a) // 控制权被 take 走
	if len(s.trades.byChar) != 0 || a.trade != nil || b.trade != nil {
		t.Fatal("控制权被抢走后交易应当取消")
	}
}

func TestPartyTargetInTradeRefused(t *testing.T) {
	me := peerState{floor: 1006}
	tp := peerState{floor: 1006, trade: true}
	if perr := partyTargetCheck(me, tp, false, "invite"); perr == nil || perr.Code != protocol.CodeTargetInTrade {
		t.Fatalf("对方在交易中应回 TARGET_IN_TRADE，得 %v", perr)
	}
}
