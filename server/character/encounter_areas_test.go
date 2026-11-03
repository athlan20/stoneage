// 按区域遇敌的验收（STAGE-152）：区域列表、代表落点、单次与无限模式、拒绝条件。

package character

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	area21Key = "100-21-582-399-783-560" // 萨姆吉尔门外（区域 21）
)

// numOf 取 JSON 解出来的数字字段（统一是 float64）。
func numOf(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return -1
}

func areaByNumber(t *testing.T, data map[string]any, number int) map[string]any {
	t.Helper()
	list, _ := data["areas"].([]any)
	for _, raw := range list {
		a, _ := raw.(map[string]any)
		if numOf(a["number"]) == number {
			return a
		}
	}
	t.Fatalf("区域列表里没有 #%d：%v", number, data["areas"])
	return nil
}

// 村里（室内地图）没有任何可选区域，查询不报错、列表为空。
func TestEncounterAreasEmptyInVillage(t *testing.T) {
	_, c := encounterEnv(t, time.Second, 21)
	got := c.ok("encounter_areas", `{}`)
	if numOf(got["floor"]) != 1006 { // 建角 hometown 0 → 萨姆吉尔村长家
		t.Fatalf("新角色应站在村长家（地图 1006）：%v", got["floor"])
	}
	if list, _ := got["areas"].([]any); len(list) != 0 {
		t.Fatalf("村里不该有可选区域：%v", list)
	}
}

// 野外：区域 21 在列表里，敌人预览、人数上限与代表落点都要对得上开战口径。
func TestEncounterAreasAtOutskirts(t *testing.T) {
	_, c := encounterEnv(t, time.Second, 22)
	c.warpTo(t, 5) // 萨姆吉尔近郊
	got := c.ok("encounter_areas", `{}`)
	if numOf(got["floor"]) != 100 {
		t.Fatalf("地图应为 100：%v", got["floor"])
	}
	a := areaByNumber(t, got, 21)
	if a["id"] != area21Key {
		t.Errorf("区域 21 的稳定 ID = %v", a["id"])
	}
	if a["available"] != true {
		t.Errorf("区域 21 当前应当可用：%v", a["unavailable_reason"])
	}
	if numOf(a["max_enemies"]) != 2 {
		t.Errorf("区域 21 的有效人数上限 = %v，期望 2", a["max_enemies"])
	}
	// 代表落点必须在地图内、矩形内，而且真的由这块区域生效。
	anchor, _ := a["anchor"].(map[string]any)
	ax, ay := numOf(anchor["x"]), numOf(anchor["y"])
	rect, _ := a["rect"].(map[string]any)
	if ax < numOf(rect["x1"]) || ax > numOf(rect["x2"]) || ay < numOf(rect["y1"]) || ay > numOf(rect["y2"]) {
		t.Errorf("代表落点 (%d,%d) 不在矩形 %v 里", ax, ay, rect)
	}
	if at := gamedata.EncounterAreaAt(100, ax, ay); at == nil || at.Key != area21Key {
		t.Errorf("代表落点 (%d,%d) 实际命中 %v，不是区域 21", ax, ay, at)
	}
	// 敌人预览：昆伊 Lv1 / Lv2-3、凯比特 Lv1 / Lv2-3；缺失的敌群 1230 不作为候选出现。
	var names []string
	for _, raw := range a["enemies"].([]any) {
		e, _ := raw.(map[string]any)
		if numOf(e["group_id"]) == 1230 {
			t.Errorf("缺失敌群 1230 不该出现在预览里：%v", e)
		}
		names = append(names, e["name"].(string))
	}
	want := map[string]bool{"昆伊": false, "凯比特": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, seen := range want {
		if !seen {
			t.Errorf("预览里缺少 %s：%v", n, names)
		}
	}
	if numOf(a["level_min"]) != 1 || numOf(a["level_max"]) != 3 {
		t.Errorf("等级覆盖 = %v..%v，期望 1..3", a["level_min"], a["level_max"])
	}
}

// 单次遇敌：定位 → 开一场 → 打完后不再开第二场，并且状态里明确“不会继续”。
func TestEncounterStartOnce(t *testing.T) {
	_, c := encounterEnv(t, 40*time.Millisecond, 23)
	c.warpTo(t, 5)
	c.send("encounter_start", `{"source_floor":100,"area_id":"`+area21Key+`","mode":"once"}`)
	c.drainTo(t, "position_changed") // 位置必须先于 battle_start
	start := c.drainTo(t, "battle_start")
	if start["kind"] != "wild" {
		t.Fatalf("应当开一场野外战斗：%v", start)
	}
	c.drainTo(t, "battle_turn")
	ch := c.drainTo(t, "encounter_changed")
	if ch["encounter"] != true || ch["encounter_mode"] != "once" || ch["encounter_continues"] != false {
		t.Fatalf("单次遇敌的开关推送不对：%v", ch)
	}
	if numOf(ch["encounter_area_number"]) != 21 {
		t.Fatalf("推送里应带区域编号 21：%v", ch)
	}
	ok := drainOK(c)
	if ok["encounter_mode"] != "once" || ok["encounter_continues"] != false {
		t.Fatalf("响应状态不对：%v", ok)
	}

	end := c.playWildToEnd(t)
	c.drainTo(t, "status_changed")
	off := c.drainEncounterOff(t)
	want := "once_done"
	if end["result"] == "lose" && !loseInPlace(end) {
		want = "lose" // 被打飞送走优先按失败关掉（STAGE-194：原地复活按单次打完关）
	}
	if off["reason"] != want {
		t.Fatalf("单次打完的自动关闭原因应为 %s：%v", want, off)
	}
	if off["encounter_mode"] != nil || off["encounter_continues"] != false {
		t.Fatalf("关掉后不该留计划：%v", off)
	}
	c.expectQuiet(t, 300*time.Millisecond) // 间隔 40ms，不会再开第二场
}

// 无限遇敌：连开两场，两场都来自同一区域；关掉后不再开第三场。
func TestEncounterStartInfinite(t *testing.T) {
	_, c := encounterEnv(t, 40*time.Millisecond, 24)
	c.warpTo(t, 5)
	c.send("encounter_start", `{"source_floor":100,"area_id":"`+area21Key+`","mode":"infinite"}`)
	c.drainTo(t, "position_changed")
	first := c.drainTo(t, "battle_start")
	c.drainTo(t, "battle_turn")
	ch := c.drainTo(t, "encounter_changed")
	if ch["encounter_mode"] != "infinite" || ch["encounter_continues"] != true {
		t.Fatalf("无限遇敌的开关推送不对：%v", ch)
	}
	drainOK(c)

	end := c.playWildToEnd(t)
	if end["result"] == "lose" && !loseInPlace(end) {
		c.drainEncounterOff(t)
		t.Skip("这一场被打飞送走了，连场路径由 TestEncounterStartInfiniteChain 覆盖")
	}
	if end["result"] == "lose" { // 被打死原地复活：不关，直接看下一场开没开
		c.drainUntilNextBattle(t)
		return
	}
	c.drainTo(t, "status_changed")
	second := c.drainTo(t, "battle_start")
	if second["battle_id"] == first["battle_id"] {
		t.Fatalf("第二场应当是新的战斗：%v", second)
	}
	if resp, _ := c.roundtrip(t, "encounter", `{"set":"off"}`, "encounter_changed"); resp["encounter"] != false {
		t.Fatalf("encounter off 应成功：%v", resp)
	}
	c.playWildToEnd(t)
	c.drainTo(t, "status_changed")
	c.expectQuiet(t, 300*time.Millisecond)
}

// 拒绝条件：参数、地图、区域存在性、战斗中都给出明确错误码，且不移动角色。
func TestEncounterStartRejections(t *testing.T) {
	_, c := encounterEnv(t, time.Second, 25)
	c.warpTo(t, 5)
	before, _ := c.roundtrip(t, "status", `{}`, "")
	pos := before["position"].(map[string]any)

	c.expectError(t, "encounter_start", `{"source_floor":100,"area_id":"`+area21Key+`","mode":"always"}`, "BAD_REQUEST")
	c.expectError(t, "encounter_start", `{"source_floor":100,"area_id":"","mode":"once"}`, "BAD_REQUEST")
	c.expectError(t, "encounter_start", `{"area_id":"`+area21Key+`","mode":"once"}`, "BAD_REQUEST") // 漏写 source_floor 不能报成地图变了（STAGE-174）
	c.expectError(t, "encounter_start", `{"source_floor":200,"area_id":"`+area21Key+`","mode":"once"}`, "POSITION_CHANGED")
	c.expectError(t, "encounter_start", `{"source_floor":100,"area_id":"100-99999-0-0-0-0","mode":"once"}`, "ENCOUNTER_AREA_NOT_FOUND")

	after, _ := c.roundtrip(t, "status", `{}`, "")
	pos2 := after["position"].(map[string]any)
	if pos["x"] != pos2["x"] || pos["y"] != pos2["y"] {
		t.Fatalf("失败的请求不该移动角色：%v → %v", pos, pos2)
	}

	// 战斗中的切换被拒。
	c.send("encounter_start", `{"source_floor":100,"area_id":"`+area21Key+`","mode":"once"}`)
	c.drainTo(t, "position_changed")
	c.drainTo(t, "battle_start")
	c.drainTo(t, "battle_turn")
	c.drainTo(t, "encounter_changed")
	drainOK(c)
	c.expectError(t, "encounter_start", `{"source_floor":100,"area_id":"`+area21Key+`","mode":"infinite"}`, "IN_BATTLE")
	// 战斗中即使地图对不上，也先回 IN_BATTLE（协议 5.50：非战斗先于当前地图）。
	c.expectError(t, "encounter_start", `{"source_floor":200,"area_id":"`+area21Key+`","mode":"infinite"}`, "IN_BATTLE")
}
