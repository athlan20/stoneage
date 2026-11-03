package character

// 造斧任务链（STAGE-94 问题 1）：哈恩（20501ev1）的事件 19 不对应任务表，进度必须记在事件表里，否则接了委托书
// 回来时哈恩又说“接委托书”，造斧块进不去，巴尔的两个完成块也拿不到石斧（任务 22 永久卡住）。
// 链路：巴尔给委托书 2466 → 哈恩接委托书（给水壶 2463，设进行中 19）→ 科巴尔特换水（2463 换 2464）→ 哈恩造斧（2464、2466 换石斧 2465，完成 19）。

import (
	"context"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

var axeNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)

const (
	axeHahn     = 1386 // 造斧专家哈恩（20501ev1）
	axeKobaruto = 1384 // 看守用水的科巴尔特（20501ev2）
	axeEvent    = 19   // 哈恩的造斧事件（不对应任务表）
	axeLetter   = 2466 // 巴尔的委托书
	axeKettle   = 2463 // 哈恩的水壶
	axeWater    = 2464 // 装满的水
	axeStoneAxe = 2465 // 石斧
)

// axeSim 按脚本的选块顺序走一步：用真实的选块和交易计算，进度用 applyEventEdit 等同一套函数改，
// 和 commitEvent 写回内存的方式一样（任务和事件分开存）。
func axeSim(t *testing.T, s *Service, r *record, npc int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	idx, refuse := s.pickEventBlock(r, script, "")
	if refuse || idx < 0 {
		t.Fatalf("NPC %d 应该能对话并命中一块，实际 idx=%d refuse=%v", npc, idx, refuse)
	}
	return &script.Blocks[idx]
}

// axeCommit 把一块的交易和进度改动落到 r 上（同 eventAccept / eventRequest 的顺序）。
func axeCommit(t *testing.T, r *record, blk *gamedata.EventBlock, editDoing bool) {
	t.Helper()
	plan, fail, ok := planEvent(r, blk, eventSel{group: -1}, func(int) int { return 0 })
	if !ok {
		t.Fatalf("事件 %d 的交易不成立：%s", blk.Event, fail)
	}
	r.Items, r.Gold = plan.items, plan.gold
	p := progressOf(r)
	if editDoing {
		applyEventEdit(p, editSetDoing, blk.Event)
	}
	endSetFlg(p, blk.Event, blk.EndSet)
	cleanFlg(p, blk.Clean)
	r.Quests, r.EventFlags = stateRecs(p.quests), stateRecs(p.events)
}

// 造斧链整条走通：每一步选中的块都是预期的那一块。
func TestAxeChainWalksToStoneAxe(t *testing.T) {
	s := &Service{opts: Options{Now: func() time.Time { return axeNow }}}
	r := &record{Level: 20, Items: []itemRec{{Slot: firstBagSlot, ItemID: axeLetter, Count: 1}}}

	// 1. 哈恩：拿着委托书，还没有进行中 → 接委托书（REQUEST，事件 19，给水壶，设进行中）。
	blk := axeSim(t, s, r, axeHahn)
	if blk.Type != "REQUEST" || blk.Event != axeEvent {
		t.Fatalf("第一次找哈恩应是接委托书（REQUEST 事件 19），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	axeCommit(t, r, blk, true)
	if !r.eventDoing(axeEvent) {
		t.Fatalf("接了委托书，事件 19 应进行中，实际 %v", r.EventFlags)
	}

	// 2. 科巴尔特：用水壶换水。
	blk = axeSim(t, s, r, axeKobaruto)
	if blk.Type != "ACCEPT" || blk.Event != -1 {
		t.Fatalf("科巴尔特应是换水（ACCEPT，没有事件），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	axeCommit(t, r, blk, false)

	// 3. 哈恩：拿着水和委托书回来 → 造斧（ACCEPT 事件 19，给石斧，完成 19）。
	// 这一步在修复前会再次命中“接委托书”，因为 19 没有被记住。
	blk = axeSim(t, s, r, axeHahn)
	if blk.Type != "ACCEPT" || blk.Event != axeEvent {
		t.Fatalf("带着水回来应是造斧（ACCEPT 事件 19），实际 %s 事件 %d", blk.Type, blk.Event)
	}
	axeCommit(t, r, blk, false)
	if !r.eventDone(axeEvent) || r.eventDoing(axeEvent) {
		t.Fatalf("造斧后事件 19 应已完成，实际 %v", r.EventFlags)
	}
	if countItem(r.Items, axeStoneAxe) != 1 || countItem(r.Items, axeWater) != 0 || countItem(r.Items, axeLetter) != 0 {
		t.Fatalf("造斧后应有石斧、没有水和委托书，实际 %v", r.Items)
	}
}

// 石斧到手后，巴尔（事件 18，伐木任务 22）的交付块能命中：任务 22 进行中、背包里有石斧。
func TestAxeChainEndsWithBalu(t *testing.T) {
	s := &Service{opts: Options{Now: func() time.Time { return axeNow }}}
	r := &record{Level: 20,
		Items:      []itemRec{{Slot: firstBagSlot, ItemID: axeStoneAxe, Count: 1}},
		Quests:     []questRec{{No: 22, State: questDoing}}, // 伐木任务，进行中事件是 18
		EventFlags: []questRec{{No: axeEvent, State: questDone}},
	}
	script, ok := gamedata.NpcEventOf(1285) // 樵夫巴尔（20501ev3）
	if !ok {
		t.Fatalf("巴尔没有任务脚本")
	}
	idx, refuse := s.pickEventBlock(r, script, "")
	if refuse || idx < 0 {
		t.Fatalf("巴尔应能对话，实际 idx=%d refuse=%v", idx, refuse)
	}
	if blk := &script.Blocks[idx]; blk.Type != "ACCEPT" || blk.Event != 18 {
		t.Fatalf("交石斧应是 ACCEPT 事件 18，实际 %s 事件 %d", blk.Type, blk.Event)
	}
}

// 数据库里的事件进度（character_event_flags），按事件号。
func (e *env) eventRows(t *testing.T) map[int]int {
	t.Helper()
	rows, err := e.chars.db.Query(context.Background(), `SELECT ef.event_no, ef.state FROM character_event_flags ef
		JOIN characters c ON c.id = ef.character_id WHERE c.name = '勇者小明'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var no, st int
		if err := rows.Scan(&no, &st); err != nil {
			t.Fatal(err)
		}
		out[no] = st
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// eventFlagSetup 让角色已经有事件 ev 的进度（setup 用，只写数据库）。
func eventFlagSetup(t *testing.T, ev, state int) func(*env) {
	return func(e *env) {
		e.exec(t, `INSERT INTO character_event_flags (character_id, event_no, state)
			VALUES ((SELECT id FROM characters WHERE name = '勇者小明'), $1, $2)`, ev, state)
	}
}

// axeEnv 把勇者小明放在哈恩旁边（20508 图），setup 在进入世界之前执行，然后进入世界。
func axeEnv(t *testing.T, setup ...func(e *env)) (*env, *client) {
	t.Helper()
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	info, ok := gamedata.NPCByID(axeHahn)
	if !ok {
		t.Fatalf("NPC %d 不在表里", axeHahn)
	}
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", info.Floor, info.X, info.Y); err != nil {
		t.Fatal(err)
	}
	for _, f := range setup {
		f(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// 接委托书（数据库）：接了之后水壶进背包，事件 19 的进行中写进 character_event_flags（重新进入世界也还在）。
func TestAxeRequestRecordsDoingFlag(t *testing.T) {
	e, c := axeEnv(t, putItemSetup(t, 5, axeLetter, 1))
	blk := refundBlock(t, axeHahn, axeEvent) // 接任务的台词从脚本里取
	expectWindow(t, talkTo(c, axeHahn), refundHead(blk.Msg.Request), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, refundHead(blk.Msg.Thanks), 0)

	if got := e.itemCounts(t); got[axeKettle] != 1 || got[axeLetter] != 1 {
		t.Fatalf("接委托书应给水壶、保留委托书，实际 %v", got)
	}
	if got := e.eventRows(t); got[axeEvent] != questDoing {
		t.Fatalf("事件 19 应记为进行中，实际 %v", got)
	}
}

// 带着水回来造斧（数据库）：进度从 character_event_flags 读进来，造斧后石斧到手，事件 19 完成。
func TestAxeAcceptAfterRequestUsesSavedFlag(t *testing.T) {
	e, c := axeEnv(t,
		putItemSetup(t, 5, axeWater, 1),
		putItemSetup(t, 6, axeLetter, 1),
		eventFlagSetup(t, axeEvent, questDoing),
	)
	script, _ := gamedata.NpcEventOf(axeHahn)
	var accept *gamedata.EventBlock
	for i := range script.Blocks {
		if b := &script.Blocks[i]; b.Type == "ACCEPT" && b.Event == axeEvent {
			accept = b
		}
	}
	if accept == nil {
		t.Fatalf("哈恩的脚本里应有事件 19 的 ACCEPT 块")
	}
	expectWindow(t, talkTo(c, axeHahn), refundHead(accept.Msg.Accept), 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, refundHead(accept.Msg.Thanks), 0)

	if got := e.itemCounts(t); got[axeStoneAxe] != 1 || got[axeWater] != 0 || got[axeLetter] != 0 {
		t.Fatalf("造斧后应有石斧、没有水和委托书，实际 %v", got)
	}
	if got := e.eventRows(t); got[axeEvent] != questDone {
		t.Fatalf("造斧后事件 19 应已完成，实际 %v", got)
	}
}
