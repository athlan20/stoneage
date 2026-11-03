package character

// STAGE-222：琉璃洞窟（任务 36，事件 39）的端到端（纯逻辑，用真实白名单、真实选块和交易，写法同
// npcevent_king_test.go）。链路：拉鲁（1294，3413 森林的独栋房屋，ruri02）接事件 39（REQUEST，要求 ENDEV=4 成人仪式，
// 给项链 2699）→ 伊布（1204，200 加鲁卡，ruri03）给日记 2698（NOWEV=39）→ 女神蒂亚特（1401，21015 琉璃的洞窟１５楼，
// ruri01）收日记和项链，给大地之绊 2701、结束事件 39。大地的主人（ruri04）、大小袋子（ruri05/06）不在这条链上，不放开。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 琉璃链里的 NPC 和道具编号（台词对应 ruri01、ruri02、ruri03）。
const (
	ruriTiare   = 1401 // 女神蒂亚特（21015，ruri01）
	ruriRaru    = 1294 // 拉鲁（3413，ruri02）
	ruriIbu     = 1204 // 伊布（200，ruri03）
	ruriDiary   = 2698 // 伊布的日记（伊布给，交给蒂亚特）
	ruriNeck    = 2699 // 拉鲁的项链（拉鲁给，交给蒂亚特）
	ruriEarthCh = 2701 // 大地之绊（事件 39 完成时给）
)

// TestRuriChainWalksToEarthBond 从头走完任务 36：拉鲁接事件 39 → 伊布给日记 → 蒂亚特收日记和项链，给大地之绊；
// 之后三人都只闲谈。
func TestRuriChainWalksToEarthBond(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil)

	// 1. 拉鲁：接事件 39（REQUEST，给项链 2699，设进行中）。
	blk := kingPick(t, s, r, ruriRaru, 1)
	if blk.Type != "REQUEST" || blk.Event != 39 {
		t.Fatalf("拉鲁的第一块应是接事件 39（REQUEST），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, ruriNeck) != 1 || !r.eventDoing(39) {
		t.Fatalf("接了任务应拿到项链、事件 39 进行中，实际 items=%v", r.Items)
	}

	// 2. 伊布：事件 39 进行中、身上没有日记，给日记（ACCEPT，没有事件号，不结束 39）。
	blk = kingPick(t, s, r, ruriIbu, 1)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("伊布应给日记（ACCEPT 没有事件），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, ruriDiary) != 1 || !r.eventDoing(39) || r.eventDone(39) {
		t.Fatalf("应拿到日记且事件 39 仍进行中，实际 items=%v", r.Items)
	}

	// 3. 蒂亚特：日记和项链都在，收走两件，给大地之绊，结束事件 39。
	blk = kingPick(t, s, r, ruriTiare, 0)
	if blk.Type != "ACCEPT" || blk.Event != 39 {
		t.Fatalf("蒂亚特应收日记和项链（ACCEPT 事件 39），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	kingRun(t, r, blk)
	if countItem(r.Items, ruriDiary) != 0 || countItem(r.Items, ruriNeck) != 0 || countItem(r.Items, ruriEarthCh) != 1 {
		t.Fatalf("交完应收走日记和项链、给大地之绊，实际 %v", r.Items)
	}
	if !r.eventDone(39) || r.eventDoing(39) {
		t.Fatalf("交完应完成事件 39 且不再进行中，实际 %v", r.EventFlags)
	}

	// 4. 之后三人只闲谈：蒂亚特说任务已完成（块 4），拉鲁和伊布各说一句（拉鲁块 2、伊布块 0），不再发奖。
	kingTalk(t, s, r, ruriTiare, 4)
	kingTalk(t, s, r, ruriRaru, 2)
	kingTalk(t, s, r, ruriIbu, 0)
	if len(r.Items) != 1 || countItem(r.Items, ruriEarthCh) != 1 {
		t.Fatalf("完成后闲谈不应改动背包，实际 %v", r.Items)
	}
}

// TestRuriRequestNeedsFamilyRite 没完成成人仪式（ENDEV=4）时拉鲁不接任务、不给项链，只闲谈。
func TestRuriRequestNeedsFamilyRite(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	kingTalk(t, s, r, ruriRaru, 3)
	if len(r.Items) != 0 || r.eventDoing(39) {
		t.Fatalf("没完成仪式不应接到任务，实际 items=%v", r.Items)
	}
}

// TestRuriDiaryOnlyWhileDoing 事件 39 没进行中时伊布不给日记（块 1 要求 NOWEV=39），只闲谈。
func TestRuriDiaryOnlyWhileDoing(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{4}, nil)
	kingTalk(t, s, r, ruriIbu, 3)
	if len(r.Items) != 0 {
		t.Fatalf("没接任务时伊布不应给日记，实际 %v", r.Items)
	}
}

// TestRuriTiareNeedsBothItems 只有日记没有项链时，蒂亚特不收（块 0 不命中，块 3 催去拿项链），日记留在身上，
// 事件 39 仍进行中。
func TestRuriTiareNeedsBothItems(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(ruriDiary), []int{4}, []int{39})
	blk := kingPick(t, s, r, ruriTiare, 3)
	if blk.Type != "MESSAGE" {
		t.Fatalf("缺项链时应是催促的 MESSAGE 块，实际 %s", blk.Type)
	}
	if countItem(r.Items, ruriDiary) != 1 || countItem(r.Items, ruriEarthCh) != 0 || !r.eventDoing(39) {
		t.Fatalf("缺项链时不应收日记、不应结束，实际 items=%v", r.Items)
	}
}

// TestRuriRequestBagFullChangesNothing 背包满时拉鲁收不下项链（REQUEST 的 GetItem 失败），整笔不做，事件 39 不开始。
func TestRuriRequestBagFullChangesNothing(t *testing.T) {
	s := kingService()
	var full []itemRec
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 2417, Count: 1}) // 2417 是仪玉，这里只占格子
	}
	r := kingRecord(full, []int{4}, nil)
	blk := kingPick(t, s, r, ruriRaru, 1)
	_, fail, ok := planEvent(r, blk, eventSel{group: -1}, kingRand)
	if ok || fail != blk.Msg.ItemFull || fail == "" {
		t.Fatalf("满包时应说 ItemFullMsg 且交易不成立，实际 ok=%v fail=%q", ok, fail)
	}
	if countItem(r.Items, ruriNeck) != 0 || r.eventDoing(39) || len(r.Items) != len(full) {
		t.Fatalf("满包失败后不应有任何改动，实际 items=%d 事件39进行中=%v", len(r.Items), r.eventDoing(39))
	}
}

// TestRuriNoRewardAfterDone 事件 39 完成后，项链、日记和大地之绊都不会再给：拉鲁和伊布只闲谈，
// 蒂亚特的块 0 不再命中（事件已完成）。
func TestRuriNoRewardAfterDone(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(ruriEarthCh), []int{4, 39}, nil)
	kingTalk(t, s, r, ruriRaru, 2)
	kingTalk(t, s, r, ruriIbu, 0)
	kingTalk(t, s, r, ruriTiare, 4)
	if len(r.Items) != 1 || countItem(r.Items, ruriEarthCh) != 1 {
		t.Fatalf("完成后不应再发东西，实际 %v", r.Items)
	}
}

// TestRuriWhitelisted 任务 36 用到的脚本都在白名单里、整本支持（ruri01、ruri02、ruri03，以及前置的成人仪式 event04_1、
// event04_2）。大地的主人（ruri04，只说闲话，单独放开）；大小袋子（ruri05、ruri06，有 Break 块）不在链上，这里确认它们没有被顺带放开。
func TestRuriWhitelisted(t *testing.T) {
	for _, file := range []string{"jaruga/event/ruri01", "jaruga/event/ruri02", "jaruga/event/ruri03", "jaruga/event/ruri04",
		"jaruga/event/event04_1", "jaruga/event/event04_2"} {
		if !gamedata.NpcEventEnabled(file) {
			t.Errorf("%s 应已放开", file)
		}
	}
	for _, file := range []string{"jaruga/event/ruri05", "jaruga/event/ruri06"} {
		if gamedata.NpcEventEnabled(file) {
			t.Errorf("%s 含 Break 块，不应放开", file)
		}
	}
	for _, npc := range []int{ruriTiare, ruriRaru, ruriIbu} {
		script, ok := gamedata.NpcEventOf(npc)
		if !ok {
			t.Fatalf("NPC %d 没有任务脚本", npc)
		}
		for i := range script.Blocks {
			if u := script.Blocks[i].Unsupported; u != "" {
				t.Errorf("%s 第 %d 块不支持（%s），不该放开", script.File, i, u)
			}
		}
	}
}

// TestRuriEarthOwnerOnlyTalks 大地的主人（1403，ruri04）四段话只说闲话：不发物品、不改事件状态。
// 四种状态各命中一块：没做过 → 块 3；事件 39 进行中 → 块 1（ACCEPT，讲故事）；有 2700 → 块 2；完成 39 且有 2700 → 块 0。
func TestRuriEarthOwnerOnlyTalks(t *testing.T) {
	const owner, bag = 1403, 2700
	s := kingService()
	cases := []struct {
		name        string
		items       []itemRec
		done, doing []int
		want        int
	}{
		{"没做过", nil, nil, nil, 3},
		{"事件39进行中", nil, nil, []int{39}, 1},
		{"有大袋子", kingItems(bag), nil, nil, 2},
		{"完成39且有大袋子", kingItems(bag), []int{39}, nil, 0},
	}
	for _, c := range cases {
		r := kingRecord(c.items, c.done, c.doing)
		blk := kingPick(t, s, r, owner, c.want)
		before := countItem(r.Items, bag)
		kingRun(t, r, blk)
		if len(r.Items) != len(c.items) || countItem(r.Items, bag) != before {
			t.Errorf("%s：大地的主人不该改背包，实际 items=%v", c.name, r.Items)
		}
		if len(c.done) == 0 && r.eventDone(39) {
			t.Errorf("%s：不该完成事件 39", c.name)
		}
		if len(c.doing) == 0 && r.eventDoing(39) {
			t.Errorf("%s：不该让事件 39 进行中", c.name)
		}
	}
}
