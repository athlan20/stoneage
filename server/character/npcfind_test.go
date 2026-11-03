package character

// npc_find 请求（STAGE-84，docs/protocol.md 5.43）：按名字找 NPC。村长家的厨师（225）在村长家（1006），
// 糊涂小孩（635）在地图 5500，道场的商人（226/437）在师傅的房间（1008/3008）。
// STAGE-161 起「能不能去」只看快捷传送目录：1008 在目录里（师傅的房间），所以它上面的 NPC 会列出来。

import (
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// findNpcs 取出 npc_find 响应里的列表，没有 npcs 字段或不是数组就失败。
func findNpcs(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["npcs"].([]any)
	if !ok {
		t.Fatalf("npc_find 的 npcs 应是数组（不能是 null）: %v", m)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(map[string]any))
	}
	return out
}

func TestNpcFindRequiresWorldAndName(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.expectError(t, "npc_find", `{"name":"村长家的厨师"}`, "WRONG_STATE") // 还没进入世界
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	c.expectError(t, "npc_find", `{}`, "BAD_REQUEST")
	c.expectError(t, "npc_find", `{"name":"  "}`, "BAD_REQUEST")
	c.expectError(t, "npc_find", `{"name":3}`, "BAD_REQUEST")

	// 没有这个名字：返回空数组，不报错。
	if list := findNpcs(t, c.ok("npc_find", `{"name":"不存在的人"}`)); len(list) != 0 {
		t.Errorf("找不到时应返回空列表，实际 %v", list)
	}
}

func TestNpcFindOnCurrentMap(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	list := findNpcs(t, c.ok("npc_find", `{"name":"村长家的厨师"}`))
	if len(list) != 1 {
		t.Fatalf("村长家的厨师应只有 1 个，实际 %v", list)
	}
	if got := list[0]; got["id"] != float64(225) || got["floor"] != float64(1006) || got["map_name"] != "萨姆吉尔的村长家" {
		t.Errorf("村长家的厨师应是 225 号、在 1006（萨姆吉尔的村长家），实际 %v", got)
	}
}

func TestNpcFindOnOtherWarpableMap(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 糊涂小孩在另一张能传到的地图上：列出来，地图名由服务端给；角色没有动。
	list := findNpcs(t, c.ok("npc_find", `{"name":"糊涂小孩"}`))
	if len(list) != 1 {
		t.Fatalf("糊涂小孩应只有 1 个，实际 %v", list)
	}
	if got := list[0]; got["id"] != float64(635) || got["floor"] != float64(5500) || got["map_name"] == "" {
		t.Errorf("糊涂小孩应是 635 号、在 5500 且有地图名，实际 %v", got)
	}
	m := c.ok("status", `{}`)
	if pos := m["position"].(map[string]any); pos["floor"] != float64(1006) {
		t.Errorf("npc_find 不应移动角色，实际 %v", pos)
	}
}

// 能不能去只看快捷传送目录（STAGE-161）：列出来的 NPC 一定在目录里，目录外的图一个都不列。
func TestNpcFindOnlyListsWarpableMaps(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 目录外的图（测试图、占位图、整图是海面的）根本不可传送。
	for _, floor := range []int{777, 501, 516, 517, 518, 519, 520, 521, 522, 999, 5561, 5581} {
		if gamedata.WarpableFloor(floor) {
			t.Errorf("地图 %d 在目录外，不该可传送", floor)
		}
	}

	// 师傅的房间（1008）在目录里，道场的商人能列出来；列出的每一条都要在目录里。
	list := findNpcs(t, c.ok("npc_find", `{"name":"道场的商人"}`))
	if len(list) == 0 {
		t.Error("1008 师傅的房间在目录里，道场的商人应当列出来")
	}
	for _, it := range list {
		if floor := int(it["floor"].(float64)); !gamedata.WarpableFloor(floor) {
			t.Errorf("地图 %d 不在目录里，不该列出它的 NPC：%v", floor, it)
		}
	}
}

func TestNpcFindCurrentMapFirstAndLimited(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 包含“村长家”：没有完全同名的，退到包含匹配；当前地图的排在最前。
	list := findNpcs(t, c.ok("npc_find", `{"name":"村长家"}`))
	if len(list) == 0 || list[0]["id"] != float64(225) {
		t.Fatalf("当前地图的村长家的厨师应排第一，实际 %v", list)
	}
	for _, it := range list {
		if !strings.Contains(it["name"].(string), "村长家") {
			t.Errorf("包含匹配的结果名字里应有“村长家”，实际 %v", it)
		}
	}

	// 很多 NPC 名字里有“的”：最多返回 20 条。
	if list := findNpcs(t, c.ok("npc_find", `{"name":"的"}`)); len(list) != npcFindLimit {
		t.Errorf("结果应截到 %d 条，实际 %d 条", npcFindLimit, len(list))
	}
}
