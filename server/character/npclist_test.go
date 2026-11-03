package character

// npc_list 请求（STAGE-134，docs/protocol.md 5.47）：NPC 目录——分类（reachable/here）、
// 名字搜索、分页与总数。和 npc_find（只按名字查、有条数上限）分工不同，客户端不能用后者拼目录。

import (
	"fmt"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// npcListBody 取出 npc_list 响应，npcs 必须是数组（不能是 null）。
func npcListBody(t *testing.T, m map[string]any) ([]map[string]any, map[string]any) {
	t.Helper()
	raw, ok := m["npcs"].([]any)
	if !ok {
		t.Fatalf("npc_list 的 npcs 应是数组（不能是 null）: %v", m)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(map[string]any))
	}
	return out, m
}

// npcListClient 建号并进入世界。
func npcListClient(t *testing.T) *client {
	t.Helper()
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	return c
}

func TestNpcListRequiresWorldAndChecksArguments(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.expectError(t, "npc_list", `{}`, "WRONG_STATE") // 还没进入世界
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	c.expectError(t, "npc_list", `{"scope":"附近"}`, "BAD_REQUEST")
	c.expectError(t, "npc_list", `{"offset":-1}`, "BAD_REQUEST")
	c.expectError(t, "npc_list", `{"limit":0}`, "BAD_REQUEST")
}

func TestNpcListHereOnlyGivesTheCurrentMap(t *testing.T) {
	c := npcListClient(t)
	list, body := npcListBody(t, c.ok("npc_list", `{"scope":"here"}`))
	if len(list) == 0 {
		t.Fatalf("本地图应当有 NPC，实际 %v", body)
	}
	for _, it := range list {
		if it["here"] != true {
			t.Errorf("scope=here 只应收当前地图的 NPC，实际 %v", it)
		}
	}
	if body["scope"] != "here" {
		t.Errorf("响应要回显 scope，实际 %v", body["scope"])
	}
	if body["total"] != float64(len(list)) {
		t.Errorf("here 一次就取全，total 应等于条数：total=%v 条数=%d", body["total"], len(list))
	}
}

// 可达目录里只出现快捷传送目录里的地图（STAGE-161）。
func TestNpcListReachableOnlyListsWarpableMaps(t *testing.T) {
	c := npcListClient(t)
	list, body := npcListBody(t, c.ok("npc_list", `{"scope":"reachable","limit":200}`))
	seen := map[int]bool{}
	for _, it := range list {
		floor := int(it["floor"].(float64))
		seen[floor] = true
	}
	if !seen[1006] {
		t.Errorf("村长家（1006）在目录里，应当出现在可达目录里：%v", body)
	}
	for floor := range seen {
		if !gamedata.WarpableFloor(floor) {
			t.Errorf("地图 %d 不在快捷传送目录里，不该出现在可达目录里：%v", floor, body)
		}
	}
}

func TestNpcListSearchesByName(t *testing.T) {
	c := npcListClient(t)
	list, body := npcListBody(t, c.ok("npc_list", `{"name":"村长家的厨师"}`))
	if len(list) != 1 {
		t.Fatalf("按名字搜索应当只剩 1 条，实际 %v", body)
	}
	if list[0]["id"] != float64(225) {
		t.Errorf("应当是 225 号，实际 %v", list[0])
	}
	// 搜索关键词是子串匹配：完整名字里也含「厨师」。
	sub, _ := npcListBody(t, c.ok("npc_list", `{"name":"厨师"}`))
	if len(sub) == 0 {
		t.Errorf("子串搜索结果不该为空")
	}
	// 搜不到时是空数组，不报错。
	empty, _ := npcListBody(t, c.ok("npc_list", `{"name":"不存在的人"}`))
	if len(empty) != 0 {
		t.Errorf("搜不到应返回空列表，实际 %v", empty)
	}
}

func TestNpcListPaginatesWithTotal(t *testing.T) {
	c := npcListClient(t)
	all, body := npcListBody(t, c.ok("npc_list", `{"limit":200}`))
	total := int(body["total"].(float64))
	// STAGE-140 总验收：真实目录大于200条，按协议分页取完。
	for offset := len(all); offset < total; {
		page, _ := npcListBody(t, c.ok("npc_list", fmt.Sprintf(`{"limit":200,"offset":%d}`, offset)))
		if len(page) == 0 {
			t.Fatalf("目录在 %d/%d 提前结束", offset, total)
		}
		all = append(all, page...)
		offset += len(page)
	}
	if total <= 5 {
		t.Skipf("目录只有 %d 条，分页测不出来", total)
	}

	first, firstBody := npcListBody(t, c.ok("npc_list", `{"limit":5,"offset":0}`))
	if len(first) != 5 {
		t.Fatalf("第一页应当 5 条，实际 %d", len(first))
	}
	if firstBody["total"] != float64(total) {
		t.Errorf("每页的 total 都应当是全部条数：%v", firstBody["total"])
	}
	second, _ := npcListBody(t, c.ok("npc_list", `{"limit":5,"offset":5}`))
	if len(second) == 0 {
		t.Fatalf("第二页不该为空")
	}
	if first[0]["id"] == second[0]["id"] {
		t.Errorf("第二页应当接着第一页：%v / %v", first[0], second[0])
	}
	// 翻过头给空数组，不报错。
	beyond, _ := npcListBody(t, c.ok("npc_list", `{"limit":5,"offset":100000}`))
	if len(beyond) != 0 {
		t.Errorf("超过总数应返回空列表，实际 %v", beyond)
	}
	if len(all) != total {
		t.Errorf("一次取全应当拿到 total 条：%d / %d", len(all), total)
	}
}

func TestNpcListLimitIsCapped(t *testing.T) {
	c := npcListClient(t)
	_, body := npcListBody(t, c.ok("npc_list", `{"limit":100000}`))
	if body["limit"] != float64(npcListMaxLimit) {
		t.Errorf("limit 要被压到上限 %d，实际 %v", npcListMaxLimit, body["limit"])
	}
}
