package character

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func menuCall(t *testing.T, c *client, typ string, data map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(data)
	m, pushes := c.callp(typ, string(b))
	if m["ok"] != true {
		t.Fatalf("%s: %v", typ, m)
	}
	for i := len(pushes) - 1; i >= 0; i-- {
		if pushes[i]["type"] == "npc_window" {
			return pushes[i]["data"].(map[string]any)
		}
	}
	return nil
}

func v2Buy(t *testing.T, c *client, count string) map[string]any {
	w := menuCall(t, c, "npc_open", map[string]any{"npc": shopNPC})
	w = menuCall(t, c, "npc_choose", map[string]any{"window_id": w["window_id"], "no": 1})
	w = menuCall(t, c, "npc_choose", map[string]any{"window_id": w["window_id"], "no": 1})
	return menuCall(t, c, "npc_reply", map[string]any{"window_id": w["window_id"], "text": count})
}

func TestShopMenuQuoteCancelCommitAndReplay(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)
	var gold int64
	withRec(t, e, "alice", func(r *record) { gold = r.Gold })
	w := v2Buy(t, c, "2")
	q := w["trade"].(map[string]any)
	if q["count"] != float64(2) || q["total"] != 2*q["unit_price"].(float64) {
		t.Fatal(q)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != gold {
			t.Fatal("报价扣款")
		}
	})
	old := w["window_id"]
	menuCall(t, c, "npc_confirm", map[string]any{"window_id": old, "confirm": false})
	c.wantErr(t, "npc_confirm", fmt.Sprintf(`{"window_id":%q,"confirm":true}`, old), "WRONG_STATE")
	w = v2Buy(t, c, "2")
	old = w["window_id"]
	menuCall(t, c, "npc_confirm", map[string]any{"window_id": old, "confirm": true})
	_, price := shopFirstItem(t)
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != gold-int64(price*2) {
			t.Fatalf("余额 %d", r.Gold)
		}
	})
	c.wantErr(t, "npc_confirm", fmt.Sprintf(`{"window_id":%q,"confirm":true}`, old), "WRONG_STATE")
}

func TestShopMenuPartialSellAndChangedQuote(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)
	item, _ := shopFirstItem(t)
	withRec(t, e, "alice", func(r *record) {
		_, err := e.chars.db.Exec(context.Background(), `DELETE FROM character_items WHERE character_id=$1`, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.chars.db.Exec(context.Background(), `INSERT INTO character_items(character_id,slot,item_id,count) VALUES($1,5,$2,3)`, r.ID, item)
		if err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: 5, ItemID: item, Count: 3}}
	})
	w := menuCall(t, c, "npc_open", map[string]any{"npc": shopNPC})
	w = menuCall(t, c, "npc_choose", map[string]any{"window_id": w["window_id"], "no": 2})
	w = menuCall(t, c, "npc_choose", map[string]any{"window_id": w["window_id"], "no": 5})
	w = menuCall(t, c, "npc_reply", map[string]any{"window_id": w["window_id"], "text": "2"})
	menuCall(t, c, "npc_confirm", map[string]any{"window_id": w["window_id"], "confirm": true})
	withRec(t, e, "alice", func(r *record) {
		if it := findItem(r.Items, 5); it == nil || it.Count != 1 {
			t.Fatal(r.Items)
		}
	})
	w = v2Buy(t, c, "1")
	withRec(t, e, "alice", func(r *record) { e.chars.LiveOf(e.session("alice")).p.npcWindow.shop.quote.expires = time.Time{} })
	c.wantErr(t, "npc_confirm", fmt.Sprintf(`{"window_id":%q,"confirm":true}`, w["window_id"]), "WRONG_STATE")
}

func TestWindowMenuRejectsMissingOrStaleRevision(t *testing.T) {
	p := &Presence{npcMenuV2: true, npcWindow: &npcWindow{id: "current"}}
	for _, id := range []string{"", "old"} {
		if p.checkWindow(id) == nil {
			t.Fatal(id)
		}
	}
	if p.checkWindow("current") != nil {
		t.Fatal("当前版本被拒")
	}
}
