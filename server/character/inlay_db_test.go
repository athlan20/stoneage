package character

// STAGE-298：精工的数据库测试（需要 STONEAGE_TEST_DATABASE_URL）：事务回滚（写入失败时扣料和镶入一起回滚）、
// 叠放材料只扣 1 个、镶满时已镶部分保留并带 applied、交易过户时 inlay 随行走、交易中 item_inlay 回 IN_TRADE。

import (
	"context"
	"testing"
)

// insertInlayItem 直接往库里放一件物品（测试用，数值 0 耐久）。
func insertInlayItem(t *testing.T, e *env, charID int64, slot, itemID, count int) {
	t.Helper()
	if _, err := e.chars.db.Exec(context.Background(),
		`INSERT INTO character_items (character_id, slot, item_id, count, durability, max_durability) VALUES ($1, $2, $3, $4, 0, 0)`,
		charID, slot, itemID, count); err != nil {
		t.Fatal(err)
	}
}

// dbItemAt 读库里某格的物品（没有就返回 nil）。
func dbItemAt(t *testing.T, e *env, charID int64, slot int) *itemRec {
	t.Helper()
	for _, it := range dbItems(t, e, charID) {
		if it.Slot == slot {
			it := it
			return &it
		}
	}
	return nil
}

// inlayDBChar 建一个角色，背包放好要用的物品，读回内存。
func inlayDBChar(t *testing.T, e *env, items ...[4]int) *Presence {
	t.Helper()
	p := tradeDBChar(t, e, "alice", "勇者小明")
	for _, it := range items {
		insertInlayItem(t, e, p.charID, it[0], it[1], it[2])
	}
	reloadHolding(t, e, p)
	return p
}

// 写入失败时（这里用触发器让镶入的那一行报错）整个精工回滚：扣掉的材料回来，目标的镶嵌不变，内存不变。
func TestInlayDBRollbackOnWriteFailure(t *testing.T) {
	loadTestOverlay(t)
	e := newEnv(t)
	p := inlayDBChar(t, e, [4]int{5, 20, 1}, [4]int{6, 1600, 1}, [4]int{7, 11, 2})
	ctx := context.Background()
	if _, err := e.chars.db.Exec(ctx, `CREATE FUNCTION inlay_boom() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'boom'; END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.chars.db.Exec(ctx, `CREATE TRIGGER inlay_boom BEFORE INSERT ON character_items FOR EACH ROW WHEN (NEW.inlay IS NOT NULL) EXECUTE FUNCTION inlay_boom()`); err != nil {
		t.Fatal(err)
	}
	_, err := e.chars.applyInlay(ctx, p, &p.rec, 5, []int{6, 7})
	if err == nil {
		t.Fatal("写入失败应返回错误")
	}
	if got := dbItemAt(t, e, p.charID, 5); got == nil || got.Inlay != nil {
		t.Fatalf("回滚后目标不应镶上，得 %+v", got)
	}
	if got := dbItemAt(t, e, p.charID, 6); got == nil || got.Count != 1 {
		t.Fatalf("回滚后材料 1600 应还在，得 %+v", got)
	}
	if got := dbItemAt(t, e, p.charID, 7); got == nil || got.Count != 2 {
		t.Fatalf("回滚后叠放的 11 应还是 2 个，得 %+v", got)
	}
	if len(p.rec.Items) != 3 || p.rec.Items[0].Inlay != nil {
		t.Fatalf("回滚后内存不应变：%+v", p.rec.Items)
	}
}

// 叠放的材料只扣 1 个；目标上的镶嵌状态写进库。
func TestInlayDBStackedMaterialConsumesOne(t *testing.T) {
	loadTestOverlay(t)
	e := newEnv(t)
	p := inlayDBChar(t, e, [4]int{5, 20, 1}, [4]int{7, 11, 2})
	out, err := e.chars.applyInlay(context.Background(), p, &p.rec, 5, []int{7})
	if err != nil {
		t.Fatal(err)
	}
	if out.Applied != 1 || out.Full {
		t.Fatalf("want applied 1, got %+v", out)
	}
	if got := dbItemAt(t, e, p.charID, 7); got == nil || got.Count != 1 {
		t.Fatalf("叠放的 11 应剩 1 个，得 %+v", got)
	}
	tgt := dbItemAt(t, e, p.charID, 5)
	if tgt == nil || tgt.Inlay == nil || tgt.Inlay.Codes[0] == nil || *tgt.Inlay.Codes[0] != "EMERALD" {
		t.Fatalf("目标的镶嵌状态没写进库：%+v", tgt)
	}
}

// 镶满时不回滚：已镶的材料扣掉、镶上，第 3 个留在背包，applyInlay 返回 Full。
func TestInlayDBFullKeepsApplied(t *testing.T) {
	loadTestOverlay(t)
	e := newEnv(t)
	p := inlayDBChar(t, e, [4]int{5, 30, 1}, [4]int{6, 1600, 1}, [4]int{7, 11, 1}, [4]int{8, 1600, 1})
	out, err := e.chars.applyInlay(context.Background(), p, &p.rec, 5, []int{6, 7, 8})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Full || out.Applied != 2 || len(out.Consumed) != 2 {
		t.Fatalf("want full applied 2, got %+v", out)
	}
	if dbItemAt(t, e, p.charID, 6) != nil || dbItemAt(t, e, p.charID, 7) != nil {
		t.Fatal("已镶的两个材料应扣掉")
	}
	if got := dbItemAt(t, e, p.charID, 8); got == nil || got.Count != 1 {
		t.Fatalf("第 3 个材料应留在背包，得 %+v", got)
	}
	tgt := dbItemAt(t, e, p.charID, 5)
	if tgt == nil || tgt.Inlay == nil || freeInlaySlots(tgt.Inlay.Codes) != 0 {
		t.Fatalf("目标应镶满，得 %+v", tgt)
	}
}

// 交易过户：物品行的 id 不变（trade_exec 只改 character_id 和 slot），inlay 跟着行走。
func TestInlayDBTradeKeepsInlay(t *testing.T) {
	e := newEnv(t)
	a := tradeDBChar(t, e, "alice", "勇者小明")
	b := tradeDBChar(t, e, "bob", "路人乙")
	insertInlayItem(t, e, a.charID, 5, 20, 1)
	if _, err := e.chars.db.Exec(context.Background(),
		`UPDATE character_items SET inlay = $2 WHERE character_id = $1 AND slot = 5`, a.charID,
		`{"codes":["RUBY",null,null],"bonus":{"attack":2,"defence":0,"agility":0,"hp":0,"mp":0,"luck":0,"other_damage":0,"other_defc":0},"effect":"攻+21 防-7 敏-6 HP+0 MP+0 伤+0 吸+0"}`); err != nil {
		t.Fatal(err)
	}
	reloadHolding(t, e, a)
	reloadHolding(t, e, b)
	_, fail, err := runTrade(t, e, a, b, tradeOffer{Items: []int{5}, Pet: -1}, tradeOffer{Pet: -1})
	if err != nil || fail != nil {
		t.Fatalf("过户应成功：err=%v fail=%+v", err, fail)
	}
	got := dbItemAt(t, e, b.charID, 5)
	if got == nil || got.Inlay == nil || got.Inlay.Codes[0] == nil || *got.Inlay.Codes[0] != "RUBY" || got.Inlay.Effect == "" {
		t.Fatalf("过户后 inlay 应随物品到乙，得 %+v", got)
	}
	if dbItemAt(t, e, a.charID, 5) != nil {
		t.Fatal("甲的格子应空了")
	}
}

// 交易中 item_inlay 回 IN_TRADE：它是操作类请求，不在交易允许名单里，守卫会拦下。
func TestInlayBlockedDuringTrade(t *testing.T) {
	if tradeAllowedDuringTrade("item_inlay") {
		t.Fatal("item_inlay 交易中应回 IN_TRADE")
	}
}
