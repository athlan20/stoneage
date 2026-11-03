// STAGE-334：英雄的祝福（18543，ITEM_useSkup）在世界中使用：未分配能力点 +1、扣 1 个物品（协议 5.31）。

package character

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

func TestSkillUpItemGivesPoint(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	// 5 号格放 3 个英雄的祝福（18543，自己用）。
	const item, slot, count = 18543, 5, 3
	slotStr := strconv.Itoa(slot)
	var charID int64
	var start int
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, $2, $3, $4)`,
			r.ID, slot, item, count); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: slot, ItemID: item, Count: count}}
		charID, start = r.ID, r.SkillPoints
	})

	// 只能对自己用：选别的目标回 INVALID_TARGET，道具不扣。
	c.wantErr(t, "item_use", `{"slot":`+slotStr+`,"target":1}`, protocol.CodeInvalidTarget)

	// 每用一个：未分配点 +1（回包与库里都要变），物品 -1。
	for i := 1; i <= count; i++ {
		m, _ := c.callp("item_use", `{"slot":`+slotStr+`}`) // callp 跳过 status_changed、inventory_changed 等推送
		if m["ok"] != true {
			t.Fatalf("第 %d 次 item_use 应成功: %v", i, m)
		}
		data, _ := m["data"].(map[string]any)
		if got, _ := data["skill_points"].(float64); int(got) != start+i {
			t.Fatalf("第 %d 次使用后 skill_points 应为 %d，实际 %v", i, start+i, data["skill_points"])
		}
		if d, _ := data["skill_points_delta"].(float64); d != 1 {
			t.Fatalf("skill_points_delta 应为 1，实际 %v", data["skill_points_delta"])
		}
	}

	// 用完后格子消失，再用回 EMPTY_SLOT。
	c.wantErr(t, "item_use", `{"slot":`+slotStr+`}`, protocol.CodeEmptySlot)
	withRec(t, e, "alice", func(r *record) {
		if findItem(r.Items, slot) != nil {
			t.Fatal("用完的英雄的祝福格子应该消失")
		}
		if r.SkillPoints != start+count {
			t.Fatalf("内存里的未分配点应为 %d，实际 %d", start+count, r.SkillPoints)
		}
	})
	var dbPoints int
	if err := e.chars.db.QueryRow(context.Background(),
		`SELECT skill_points FROM characters WHERE id = $1`, charID).Scan(&dbPoints); err != nil {
		t.Fatal(err)
	}
	if dbPoints != start+count {
		t.Fatalf("库里的未分配点应为 %d（必须落库），实际 %d", start+count, dbPoints)
	}
}
