package character

import "testing"

// TestWarpToAnyMapByFloor：warp 的 floor 参数能去任何有落点的地图（STAGE-82），位置是落点，地图名由 floor 决定。
func TestWarpToAnyMapByFloor(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 1006 是村长家的落点（mapwarp 里的传送目标），不在地点表里。
	_, push := c.roundtrip(t, "warp", `{"floor":1006}`, "position_changed")
	if push == nil {
		t.Fatal("warp 应推送 position_changed")
	}
	if f, x, y := int(push["floor"].(float64)), int(push["x"].(float64)), int(push["y"].(float64)); f != 1006 || x != 10 || y != 20 {
		t.Fatalf("应落在 1006 的落点 (10,20)，实际 %d(%d,%d)", f, x, y)
	}
	if push["map_name"] != "萨姆吉尔的村长家" {
		t.Errorf("地图名应由 floor 决定，实际 %v", push["map_name"])
	}
	if m := c.ok("status", `{}`); m["position"] == nil {
		t.Fatalf("status 应带位置: %v", m)
	} else if f, _, _ := position(m); f != 1006 {
		t.Errorf("status 里的位置应是 1006，实际 %d", f)
	}
}

// TestWarpFloorAndPlaceArgs：place 和 floor 只能给一个；地图编号不存在时回复 PLACE_NOT_FOUND，位置不变。
func TestWarpFloorAndPlaceArgs(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.warpTo(t, 3) // 加加村

	for _, data := range []string{`{}`, `{"place":1,"floor":1006}`, `{"place":null,"floor":null}`, `{"floor":"1006"}`, `{"floor":1006.5}`} {
		c.expectError(t, "warp", data, "BAD_REQUEST")
	}
	c.expectError(t, "warp", `{"floor":99999}`, "PLACE_NOT_FOUND")
	c.expectError(t, "warp", `{"place":99}`, "PLACE_NOT_FOUND")
	if f, x, y := position(c.ok("status", `{}`)); f != 3000 || x != 74 || y != 54 {
		t.Errorf("失败的 warp 不应改变位置，实际 %d(%d,%d)", f, x, y)
	}
	// place 为 null、floor 有值时按 floor 处理。
	c.roundtrip(t, "warp", `{"place":null,"floor":1006}`, "position_changed")
}
