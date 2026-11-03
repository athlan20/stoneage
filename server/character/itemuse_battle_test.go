package character

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-179：战斗中 item_use 回复 IN_BATTLE，背包里的道具不被消耗（协议 5.31，同 warp 等操作类）。
func TestItemUseBlockedInBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	// 背包 5 号格放一件世界中能用的回复道具（乌宝宝的肉，物品 1684），记下数量。起始背包是空的。
	const item = 1684
	const slot, before = 5, 3
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, $2, $3, $4)`,
			r.ID, slot, item, before); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: slot, ItemID: item, Count: before}}
	})

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`}`, "IN_BATTLE")

	after := 0
	withRec(t, e, "alice", func(r *record) {
		if it := findItem(r.Items, slot); it != nil {
			after = it.Count
		}
	})
	if after != before {
		t.Fatalf("战斗中被拒的 item_use 不应消耗道具：数量 %d → %d", before, after)
	}
}

// 战斗外的 item_use 照常成功（前面的拦截不能把所有使用都挡掉，也不能让正常请求 panic）。
func TestItemUseOutsideBattleStillWorks(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, 5, 1684, 2)`, r.ID); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: 5, ItemID: 1684, Count: 2}}
		r.HP = 1 // 掉血，回复才有效果
	})
	if m, _ := c.callp("item_use", `{"slot":5}`); m["ok"] != true {
		t.Fatalf("战斗外使用回复道具应成功: %v", m)
	}
}

// 血量、气力都满时用回复道具：回 ITEM_UNUSABLE，道具不扣（协议 5.31，STAGE-179 验收）。
func TestItemUseFullHPRefusesWithoutConsuming(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, 5, 1684, 2)`, r.ID); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: 5, ItemID: 1684, Count: 2}}
		r.HP, r.MP = r.maxHP(), gamedata.NewPlayerMaxMP
	})
	c.wantErr(t, "item_use", `{"slot":5}`, "ITEM_UNUSABLE")
	withRec(t, e, "alice", func(r *record) {
		if it := findItem(r.Items, 5); it == nil || it.Count != 2 {
			t.Fatalf("满血时被拒的 item_use 不应扣道具：%+v", r.Items)
		}
	})
}
