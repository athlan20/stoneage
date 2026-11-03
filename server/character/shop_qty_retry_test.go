package character

import "testing"

// STAGE-178 复现：买东西数量输错（负数、0、字母）之后，窗口应停在“要买多少”，再输合法数量照常成交。
func TestShopBuyRetryAfterBadQuantity(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	openBuyList(t, c)
	c.callp("npc_choose", `{"no":1}`)
	for _, bad := range []string{"-5", "0", "abc", ""} {
		if m := c.call("npc_reply", `{"text":"`+bad+`"}`); m["ok"] != false {
			t.Fatalf("数量 %q 应被拒: %v", bad, m)
		}
	}
	if m, pushes := c.callp("npc_reply", `{"text":"2"}`); m["ok"] != true || len(pushes) != 3 {
		t.Fatalf("非法数量之后输入合法数量应成交，实际 %v %v", m, pushes)
	}
}
