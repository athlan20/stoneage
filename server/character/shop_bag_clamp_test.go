package character

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fillBagSlots 把背包 5 号起的 n 格占满（STAGE-192 用例：制造只剩几格空位的背包）。
func fillBagSlots(t *testing.T, e *env, item, n int) {
	t.Helper()
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(), `DELETE FROM character_items WHERE character_id=$1`, r.ID); err != nil {
			t.Fatal(err)
		}
		r.Items = nil
		for i := 0; i < n; i++ {
			slot := firstBagSlot + i
			if _, err := e.chars.db.Exec(context.Background(), `INSERT INTO character_items(character_id,slot,item_id,count) VALUES($1,$2,$3,1)`, r.ID, slot, item); err != nil {
				t.Fatal(err)
			}
			r.Items = append(r.Items, itemRec{Slot: slot, ItemID: item, Count: 1})
		}
	})
}

// 背包只剩 1 格时买 2 件：报价就截断成 1 件（count、total 都按 1 件算），确认后只买到 1 件、只扣 1 件的钱。
func TestShopBuyClampsToFreeSlots(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)
	item, price := shopFirstItem(t)
	fillBagSlots(t, e, item, 14)
	var gold int64
	withRec(t, e, "alice", func(r *record) { gold = r.Gold })

	w := v2Buy(t, c, "2")
	q := w["trade"].(map[string]any)
	if q["count"] != float64(1) || q["total"] != float64(price) {
		t.Fatalf("报价应按 1 格截断成 1 件: %v", q)
	}
	if text := w["text"].(string); !strings.Contains(text, "按空位买 1 件") {
		t.Fatalf("确认页应写明截断: %q", text)
	}
	menuCall(t, c, "npc_confirm", map[string]any{"window_id": w["window_id"], "confirm": true})
	withRec(t, e, "alice", func(r *record) {
		if len(r.Items) != 15 || r.Gold != gold-int64(price) {
			t.Fatalf("买到 %d 格、余额 %d，应为 15 格、%d", len(r.Items), r.Gold, gold-int64(price))
		}
	})
}

// 背包 15 格全满：选数量那一步就回复 BAG_FULL（说明是 itemfull_msg 原话），不进入确认。
func TestShopBuyRejectsWhenBagFull(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)
	item, _ := shopFirstItem(t)
	fillBagSlots(t, e, item, 15)

	w := menuCall(t, c, "npc_open", map[string]any{"npc": shopNPC})
	w = menuCall(t, c, "npc_choose", map[string]any{"window_id": w["window_id"], "no": 1})
	w = menuCall(t, c, "npc_choose", map[string]any{"window_id": w["window_id"], "no": 1})
	c.wantErr(t, "npc_reply", fmt.Sprintf(`{"window_id":%q,"text":"2"}`, w["window_id"]), "BAG_FULL")
}
