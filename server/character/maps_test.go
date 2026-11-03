package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// TestMapsListsWarpableNamedMaps：maps 只列快捷传送目录里的正式地图（STAGE-161），
// 按 floor 排序、不重复，名字与 position.map_name 一致；同一次响应还带目录版本号和节点树。
func TestMapsListsWarpableNamedMaps(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.expectError(t, "maps", `{}`, "WRONG_STATE") // 还没进入世界
	c.ok("enter_world", `{"slot":0}`)

	// STAGE-258：完整目录（含节点树）线上超过 64 KiB，不声明 gzip 的连接收到 RESPONSE_TOO_LARGE；
	// 这里只测平面表（parts 只取 maps，走不压缩的连接也能收到），节点树的长度直接从目录数据里核对。
	c.expectError(t, "maps", `{}`, "RESPONSE_TOO_LARGE")
	if len(gamedata.TravelCatalog()) == 0 {
		t.Fatal("快捷传送目录为空")
	}
	body := c.ok("maps", `{"parts":["maps"]}`)
	if v, _ := body["catalog_version"].(string); v == "" {
		t.Error("maps 响应缺少 catalog_version")
	}
	list := body["maps"].([]any)
	if len(list) < 500 {
		t.Fatalf("地图列表太短：%d 张", len(list))
	}
	seen := map[int]bool{}
	last := -1
	found := false
	for _, raw := range list {
		m := raw.(map[string]any)
		floor := int(m["floor"].(float64))
		if floor <= last || seen[floor] {
			t.Fatalf("地图应按 floor 升序且不重复，%d 排在 %d 后面", floor, last)
		}
		seen[floor], last = true, floor
		if m["name"] == "" {
			t.Errorf("%d 没有名字", floor)
		}
		if !gamedata.WarpableFloor(floor) {
			t.Errorf("%d 不在快捷传送目录里却列在 maps 里", floor)
		}
		if _, _, ok := gamedata.LandingOf(floor); !ok {
			t.Errorf("%d 没有落点却列在 maps 里", floor)
		}
		if floor == 1006 {
			found = m["name"] == "萨姆吉尔的村长家"
		}
	}
	if !found {
		t.Error("列表里应有 1006 萨姆吉尔的村长家")
	}
	if seen[99999] {
		t.Error("不存在的地图不应出现")
	}
	// 目录之外的图（测试图、占位图、整图是海面的）一律不出现。
	for _, floor := range []int{777, 501, 516, 522, 999, 5561, 5581} {
		if seen[floor] {
			t.Errorf("地图 %d 不在正式目录里，不该出现在 maps", floor)
		}
	}
}
