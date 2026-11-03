package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-119：活动摘要（类别名加排序后的标量参数，docs/protocol.md 9.7）。
func TestActivitySummary(t *testing.T) {
	cases := []struct {
		name, typ, data, want string
	}{
		{"有参数", "turn", `{"direction":3}`, "转向 direction=3"},
		{"多个参数按键名排序", "pet_rename", `{"slot":0,"name":"小白"}`, "给宠物改名 name=小白 slot=0"},
		{"布尔值", "encounter", `{"set":true}`, "遇敌开关 set=true"},
		{"没有参数", "leave_world", ``, "离开世界"},
		{"空对象", "leave_world", `{}`, "离开世界"},
		{"数组不算参数", "item_move", `[1,2]`, "移动物品"},
		{"嵌套对象跳过", "item_use", `{"slot":5,"x":{"a":1}}`, "使用物品 slot=5"},
		{"未登记的类型用类型名", "some_future", `{"a":1}`, "some_future a=1"},
		{"控制字符去掉", "talk", "{\"text\":\"a\\u0007b\"}", "和 NPC 对话 text=ab"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &protocol.Request{Type: c.typ}
			if c.data != "" {
				req.Data = []byte(c.data)
			}
			if got := activitySummary(req); got != c.want {
				t.Errorf("摘要 = %q，想要 %q", got, c.want)
			}
		})
	}
}

// 摘要超过 40 个字符时截断并加省略号。
func TestActivitySummaryTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 60; i++ {
		long += "字"
	}
	req := &protocol.Request{Type: "pet_rename", Data: []byte(`{"name":"` + long + `"}`)}
	got := activitySummary(req)
	if n := len([]rune(got)); n != activitySummaryMax {
		t.Fatalf("截断后应当是 %d 个字符，实际 %d：%q", activitySummaryMax, n, got)
	}
	if got[len(got)-len("…"):] != "…" {
		t.Errorf("截断后应当以省略号结尾：%q", got)
	}
}
