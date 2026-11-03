// 复活类道具（STAGE-262 A 段）在战斗外的拒绝：物品表 AbleUse=1（只能战斗中用），世界里 item_use 回 ITEM_UNUSABLE，
// 道具不被消耗。战斗内的复活由引擎测试覆盖（server/battle/item_revive_test.go）。

package character

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

func TestReviveItemUnusableInWorld(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	// 5 号格放一件复活药(100)（物品 1530，AbleUse=1），数量 2。
	const item = 1530
	const slot, before = 5, 2
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, $2, $3, $4)`,
			r.ID, slot, item, before); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: slot, ItemID: item, Count: before}}
	})
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`,"target":0}`, protocol.CodeItemUnusable)
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`,"target":1}`, protocol.CodeItemUnusable)

	after := 0
	withRec(t, e, "alice", func(r *record) {
		if it := findItem(r.Items, slot); it != nil {
			after = it.Count
		}
	})
	if after != before {
		t.Fatalf("世界里被拒的复活药不应消耗：数量 %d → %d", before, after)
	}
}
