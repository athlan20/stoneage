package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 出生礼包：生产入口打开后，每个新角色的背包第一格是一把马祖的棍棒；
// 默认（Options 零值）不送，背包是空的。
func TestCreateGivesBirthGifts(t *testing.T) {
	e := newEnv(t, func(s *Service) { s.opts.NewPlayerGifts = gamedata.NewPlayerGifts })
	for _, acct := range []string{"alice", "bob"} {
		c := e.loggedIn(acct)
		c.ok("create_character", createJSON(0, "勇者"+acct, 0))
		c.ok("enter_world", `{"slot":0}`)
		slots, _ := c.ok("inventory", `{}`)["slots"].([]any)
		if len(slots) != 1 {
			t.Fatalf("%s：背包应有 1 件，实际 %v", acct, slots)
		}
		it := slots[0].(map[string]any)
		if it["item_id"].(float64) != 2690 || it["slot"].(float64) != 5 || it["count"].(float64) != 1 {
			t.Errorf("%s：背包第一格 = %v，want 马祖的棍棒 ×1 在 5 号格", acct, it)
		}
		c.ok("leave_world", `{}`)
	}
}

func TestCreateNoGiftsByDefault(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者甲", 0))
	c.ok("enter_world", `{"slot":0}`)
	if slots, _ := c.ok("inventory", `{}`)["slots"].([]any); len(slots) != 0 {
		t.Errorf("默认应不送物品，实际 %v", slots)
	}
}
