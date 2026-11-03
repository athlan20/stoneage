package character

// STAGE-294：交易的数据库测试（需要 STONEAGE_TEST_DATABASE_URL）：过户后行和耐久、合成标记保留，
// 宠物过户后休息且起始快照不变，事务失败回滚，石币上限在交换时重新校验，同格不能放两次，角色锁的加锁顺序。

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// tradeDBChar 用协议创建角色并进入世界，返回它的 Presence。
func tradeDBChar(t *testing.T, e *env, account, name string) *Presence {
	t.Helper()
	c := e.loggedIn(account)
	c.ok("create_character", createJSON(0, name, 0))
	c.ok("enter_world", `{"slot":0}`)
	p := e.chars.findOnline(name)
	if p == nil {
		t.Fatalf("%s 应该在线", name)
	}
	// 新角色自带初始物品和宠物，测试里清空，各自再放自己需要的东西
	ctx := context.Background()
	for _, q := range []string{`DELETE FROM character_items WHERE character_id = $1`, `DELETE FROM character_pets WHERE character_id = $1`, `UPDATE characters SET gold = 0 WHERE id = $1`} {
		if _, err := e.chars.db.Exec(ctx, q, p.charID); err != nil {
			t.Fatal(err)
		}
	}
	reloadHolding(t, e, p)
	return p
}

// reloadHolding 把数据库里的背包、宠物和石币读回内存（测试直接改库后用）。
func reloadHolding(t *testing.T, e *env, p *Presence) {
	t.Helper()
	ctx := context.Background()
	items, err := readItems(ctx, e.chars.db, p.charID, false)
	if err != nil {
		t.Fatal(err)
	}
	pets, err := readPets(ctx, e.chars.db, p.charID, false)
	if err != nil {
		t.Fatal(err)
	}
	var gold int64
	if err := e.chars.db.QueryRow(ctx, `SELECT gold FROM characters WHERE id = $1`, p.charID).Scan(&gold); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.rec.Items, p.rec.Pets, p.rec.Gold = items, pets, gold
	p.mu.Unlock()
}

// runTrade 建立交易并执行一次交换（与协议里第二个确定的人触发的路径一致），返回失败和数据库错误。
func runTrade(t *testing.T, e *env, a, b *Presence, offA, offB tradeOffer) (*trade, *tradeFail, error) {
	t.Helper()
	h := e.chars.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	tr := newTrade("t-db", &tradeSide{CharID: a.charID, Name: a.rec.Name, P: a}, &tradeSide{CharID: b.charID, Name: b.rec.Name, P: b}, now)
	h.attachLocked(tr)
	if perr := tr.setOffer(a.charID, offA, now); perr != nil {
		t.Fatalf("甲的报价不合法: %s", perr.Code)
	}
	if perr := tr.setOffer(b.charID, offB, now); perr != nil {
		t.Fatalf("乙的报价不合法: %s", perr.Code)
	}
	fail, err := e.chars.tradeExecute(tr)
	return tr, fail, err
}

// dbItems 返回角色在库里的物品行（按格号）。
func dbItems(t *testing.T, e *env, charID int64) []itemRec {
	t.Helper()
	items, err := readItems(context.Background(), e.chars.db, charID, false)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func dbGold(t *testing.T, e *env, charID int64) int64 {
	t.Helper()
	var gold int64
	if err := e.chars.db.QueryRow(context.Background(), `SELECT gold FROM characters WHERE id = $1`, charID).Scan(&gold); err != nil {
		t.Fatal(err)
	}
	return gold
}

func TestTradeDBSwapKeepsRowsDurabilityAndPetState(t *testing.T) {
	e := newEnv(t)
	a := tradeDBChar(t, e, "alice", "勇者小明")
	b := tradeDBChar(t, e, "bob", "路人乙")
	ctx := context.Background()
	// 甲：5 号一件受损的装备，合成标记为真；一只宠物
	if _, err := e.chars.db.Exec(ctx, `INSERT INTO character_items (character_id, slot, item_id, count, merged, durability, max_durability, crush_level) VALUES ($1, 5, 0, 1, true, 150000, 200000, 1)`, a.charID); err != nil {
		t.Fatal(err)
	}
	if _, err := GivePetWithLoyalty(ctx, e.chars.db, "trade-test", "勇者小明", 1, 5, 100, func(n int) int { return 0 }); err != nil {
		t.Fatal(err)
	}
	if _, err := e.chars.db.Exec(ctx, `UPDATE characters SET gold = 1000 WHERE id = $1`, a.charID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.chars.db.Exec(ctx, `UPDATE characters SET gold = 500 WHERE id = $1`, b.charID); err != nil {
		t.Fatal(err)
	}
	reloadHolding(t, e, a)
	reloadHolding(t, e, b)
	petSlot := a.rec.Pets[0].Slot
	originBefore := a.rec.Pets[0].Origin
	if _, err := e.chars.db.Exec(ctx, `UPDATE character_pets SET state = 1 WHERE character_id = $1`, a.charID); err != nil { // 待命
		t.Fatal(err)
	}
	reloadHolding(t, e, a)

	_, fail, err := runTrade(t, e, a, b, tradeOffer{Items: []int{5}, Gold: 100, Pet: petSlot}, tradeOffer{Gold: 50, Pet: -1})
	if err != nil || fail != nil {
		t.Fatalf("交换应当成功：err=%v fail=%+v", err, fail)
	}
	// 物品整行过户：耐久、受损、合成标记原样
	if got := dbItems(t, e, a.charID); len(got) != 0 {
		t.Fatalf("甲的背包应为空，得 %+v", got)
	}
	got := dbItems(t, e, b.charID)
	if len(got) != 1 || got[0].Slot != 5 || got[0].Durability != 150000 || got[0].MaxDurability != 200000 || got[0].CrushLevel != 1 || !got[0].Merged {
		t.Fatalf("乙的背包 = %+v，要 5 号、耐久 150000/200000、受损 1、合成标记为真", got)
	}
	// 宠物过户：乙的 0 号栏，休息，起始快照不变
	bp, err := readPets(ctx, e.chars.db, b.charID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(bp) != 1 || bp[0].Slot != 0 || bp[0].State != petStateRest {
		t.Fatalf("乙的宠物 = %+v，要 0 号栏、休息", bp)
	}
	if !reflect.DeepEqual(bp[0].Origin, originBefore) {
		t.Fatalf("起始快照变了：before=%+v after=%+v", originBefore, bp[0].Origin)
	}
	ap, _ := readPets(ctx, e.chars.db, a.charID, false)
	if len(ap) != 0 {
		t.Fatalf("甲的宠物应已过户，剩 %+v", ap)
	}
	// 石币：甲 1000-100+50，乙 500-50+100
	if g := dbGold(t, e, a.charID); g != 950 {
		t.Fatalf("甲的石币 = %d，要 950", g)
	}
	if g := dbGold(t, e, b.charID); g != 550 {
		t.Fatalf("乙的石币 = %d，要 550", g)
	}
	// 内存跟库里一致，交易已摘下
	if a.rec.Gold != 950 || b.rec.Gold != 550 || len(a.rec.Items) != 0 || len(b.rec.Items) != 1 {
		t.Fatalf("内存没有同步：甲 gold=%d items=%d 乙 gold=%d items=%d", a.rec.Gold, len(a.rec.Items), b.rec.Gold, len(b.rec.Items))
	}
	if a.trade != nil || b.trade != nil || len(e.chars.trades.byChar) != 0 {
		t.Fatal("交换成功后交易应当结束")
	}
	var logs int
	if err := e.chars.db.QueryRow(ctx, `SELECT count(*) FROM trade_logs WHERE a_char_id = $1 AND b_char_id = $2 AND a_gold = 100 AND b_gold = 50`, a.charID, b.charID).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("trade_logs 应有 1 行，得 %d", logs)
	}
}

func TestTradeDBFailureRollsBack(t *testing.T) {
	e := newEnv(t)
	a := tradeDBChar(t, e, "alice", "勇者小明")
	b := tradeDBChar(t, e, "bob", "路人乙")
	ctx := context.Background()
	if _, err := e.chars.db.Exec(ctx, `INSERT INTO character_items (character_id, slot, item_id, count, durability, max_durability, crush_level) VALUES ($1, 5, 0, 1, 90, 100, 0)`, a.charID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.chars.db.Exec(ctx, `UPDATE characters SET gold = 1000 WHERE id = $1`, a.charID); err != nil {
		t.Fatal(err)
	}
	reloadHolding(t, e, a)
	reloadHolding(t, e, b)
	// 让日志表暂时不可写：过户写到一半时失败，整个事务必须回滚
	if _, err := e.chars.db.Exec(ctx, `ALTER TABLE trade_logs RENAME TO trade_logs_off`); err != nil {
		t.Fatal(err)
	}
	_, fail, err := runTrade(t, e, a, b, tradeOffer{Items: []int{5}, Gold: 300, Pet: -1}, tradeOffer{Pet: -1})
	if _, rerr := e.chars.db.Exec(ctx, `ALTER TABLE trade_logs_off RENAME TO trade_logs`); rerr != nil {
		t.Fatal(rerr)
	}
	if err == nil || fail != nil {
		t.Fatalf("日志写不进去应返回数据库错误，得 err=%v fail=%+v", err, fail)
	}
	if got := dbItems(t, e, a.charID); len(got) != 1 || got[0].Slot != 5 {
		t.Fatalf("回滚后甲的物品应还在，得 %+v", got)
	}
	if got := dbItems(t, e, b.charID); len(got) != 0 {
		t.Fatalf("回滚后乙不应有物品，得 %+v", got)
	}
	if g := dbGold(t, e, a.charID); g != 1000 {
		t.Fatalf("回滚后甲的石币 = %d，要 1000", g)
	}
	if len(a.rec.Items) != 1 || a.rec.Gold != 1000 {
		t.Fatalf("回滚后内存不应变：items=%d gold=%d", len(a.rec.Items), a.rec.Gold)
	}
}

func TestTradeDBGoldCapRecheckedAtSwap(t *testing.T) {
	e := newEnv(t)
	a := tradeDBChar(t, e, "alice", "勇者小明")
	b := tradeDBChar(t, e, "bob", "路人乙")
	ctx := context.Background()
	if _, err := e.chars.db.Exec(ctx, `UPDATE characters SET gold = 9999000 WHERE id = $1`, b.charID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.chars.db.Exec(ctx, `UPDATE characters SET gold = 5000 WHERE id = $1`, a.charID); err != nil {
		t.Fatal(err)
	}
	reloadHolding(t, e, a)
	reloadHolding(t, e, b)
	// 乙的石币 999 万 9000，上限统一 1000 万：收 2000 会超过
	_, fail, err := runTrade(t, e, a, b, tradeOffer{Gold: 2000, Pet: -1}, tradeOffer{Pet: -1})
	if err != nil || fail == nil || fail.code != protocol.CodeGoldFull || fail.who != b.charID {
		t.Fatalf("应回 GOLD_FULL（who=乙）：err=%v fail=%+v", err, fail)
	}
	if g := dbGold(t, e, a.charID); g != 5000 {
		t.Fatalf("失败时甲的石币不应变，得 %d", g)
	}
	if g := dbGold(t, e, b.charID); g != 9999000 {
		t.Fatalf("失败时乙的石币不应变，得 %d", g)
	}
}

func TestTradeDBDuplicateSlotRejected(t *testing.T) {
	e := newEnv(t)
	a := tradeDBChar(t, e, "alice", "勇者小明")
	h := e.chars.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	tr := newTrade("t-dup", &tradeSide{CharID: a.charID, Name: a.rec.Name, P: a}, &tradeSide{CharID: 99, Name: "x"}, time.Now())
	if perr := tr.setOffer(a.charID, tradeOffer{Items: []int{5, 5}, Pet: -1}, time.Now()); perr == nil || perr.Code != protocol.CodeBadRequest {
		t.Fatalf("同格两次应回 BAD_REQUEST，得 %v", perr)
	}
}

func TestTradeLockPresencesBothOrdersNoDeadlock(t *testing.T) {
	p1 := &Presence{charID: 1}
	p2 := &Presence{charID: 2}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20000; i++ {
			lockPresences(p2, p1)()
		}
		done <- struct{}{}
	}()
	for i := 0; i < 20000; i++ {
		lockPresences(p1, p2)()
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("两个方向加锁死锁了")
	}
}
