package character

// 美容师（STAGE-255，原版 npc_charm.c）：费用公式、封顶 100、钱不够不改数据、
// 魅力已满直接给完美窗口、「什么也不做」不改数据、形象分支台词、三步窗口的推送。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 萨姆吉尔的医院（地图 1005）的美容师：npcs.tsv 的 1789 号，坐标 (25,15)。
const charmNPC = 1789

func TestCharmNPCIsInHospital(t *testing.T) {
	for _, n := range gamedata.NPCsByFloor(1005) {
		if n.ID == charmNPC {
			if n.Type != "美容师" || n.Kind != "npcgen_charm" || n.X != 25 || n.Y != 15 {
				t.Fatalf("1789 号应是 1005 的美容师 (25,15): %+v", n)
			}
			return
		}
	}
	t.Fatal("1005 应有 1789 号美容师")
}

func TestCharmPrice(t *testing.T) {
	for _, c := range []struct {
		level, charm, trans, want int
	}{
		{20, 60, 0, 4000},  // 示例：20 级、魅力 60，未转生
		{20, 90, 0, 6000},  // 90/3 = 30
		{60, 60, 0, 12000}, // 60 级
		{20, 60, 1, 8000},  // 转生 1 次翻倍
		{20, 60, 3, 16000}, // 转生 3 次 ×4
		{20, 0, 0, 200},    // 魅力 0 按 3 算：20×10×1
		{20, 1, 0, 200},    // 魅力 1 按 3 算
		{20, 2, 0, 0},      // 原版只把 <=1 改成 3，2/3 = 0 档，价钱是 0（照搬）
		{20, 3, 0, 200},
		{20, 4, 0, 200},
		{20, 59, 0, 3800}, // 59/3 = 19
		{20, 97, 0, 6400}, // 97/3 = 32
		{20, 99, 0, 6600}, // 99/3 = 33
		{20, 100, 0, -1},  // 已满
	} {
		if got := charmPrice(c.level, c.charm, c.trans); got != c.want {
			t.Errorf("charmPrice(lv=%d, charm=%d, trans=%d) = %d，应为 %d", c.level, c.charm, c.trans, got, c.want)
		}
	}
}

func TestCharmImageComment(t *testing.T) {
	for body, want := range map[int]string{
		0: "真的变得好可爱唷！", 6: "真的变得好可爱唷！",
		1: "是不是比刚才要帅多了呢？", 3: "是不是比刚才要帅多了呢？",
		4: "变得更酷了呢！", 5: "变得更酷了呢！",
		7: "愈来愈可爱了呢！", 9: "愈来愈可爱了呢！",
		10: "可真是变得愈来愈美了呢！", 11: "可真是变得愈来愈美了呢！",
	} {
		if got := charmImageComment(body); got != want {
			t.Errorf("body %d 台词 = %q，应为 %q", body, got, want)
		}
	}
}

// charmSetup 把美容师的角色放到 1005（25,15），设好等级、魅力和金币。
func charmSetup(t *testing.T, e *env, level, charm int, gold int64) *client {
	t.Helper()
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 1005, 25, 15
		r.Level, r.Charm, r.Gold = level, charm, gold
	})
	return c
}

func TestCharmThreeStepsRaisesCharm(t *testing.T) {
	e := newEnv(t)
	c := charmSetup(t, e, 1, 60, 1_000_000)

	m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, charmNPC))
	if m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 美容师应推一个窗口: %v %v", m, pushes)
	}
	text := pushes[0]["data"].(map[string]any)["text"].(string)
	for _, want := range []string{"美容师", "欢迎光临，今天要做什麽呢？", "《　给我魅力　》", "《 什麽也不做 》"} {
		if !strings.Contains(text, want) {
			t.Errorf("第一步正文缺 %q: %q", want, text)
		}
	}

	// 第二步：选 1 给我魅力 → 先关旧窗口，再推报价（1 级 × 10 × 60/3 × 1 = 200）。
	m, pushes = c.callp("npc_choose", `{"no":1}`)
	if m["ok"] != true || len(pushes) != 2 || pushes[0]["type"] != "npc_window_closed" || pushes[1]["type"] != "npc_window" {
		t.Fatalf("选给我魅力应推 closed + 报价窗口: %v %v", m, pushes)
	}
	quote := pushes[1]["data"].(map[string]any)["text"].(string)
	if !strings.Contains(quote, "要将你的魅力上升五点的话") || !strings.Contains(quote, "需要 200 的 stone") {
		t.Errorf("报价应为 200: %q", quote)
	}

	// 第三步：选 1 是 → 扣 200、魅力 +5，推 status_changed、pets_changed，再弹结果窗口。
	before := petLoyalties(t, e)
	m, pushes = c.callp("npc_choose", `{"no":1}`)
	if m["ok"] != true {
		t.Fatalf("选是应成功: %v", m)
	}
	var types []string
	for _, p := range pushes {
		types = append(types, p["type"].(string))
	}
	if strings.Join(types, ",") != "status_changed,pets_changed,npc_window_closed,npc_window" {
		t.Fatalf("推送顺序不对: %v", types)
	}
	if res := pushes[3]["data"].(map[string]any)["text"].(string); !strings.Contains(res, "嗯,这样你的魅力就上升罗！") {
		t.Errorf("结果台词不对: %q", res)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != 1_000_000-200 || r.Charm != 65 {
			t.Errorf("应扣 200、魅力 65，实际 gold=%d charm=%d", r.Gold, r.Charm)
		}
	})

	// 随身宠物的忠诚随魅力上涨（同宠物页的现算），只升不降。
	after := petLoyalties(t, e)
	if len(before) == 0 || len(before) != len(after) {
		t.Fatalf("应有随身宠物: before=%v after=%v", before, after)
	}
	changed := false
	for i := range before {
		if after[i] < before[i] {
			t.Errorf("魅力涨了，忠诚不该降: %d → %d", before[i], after[i])
		}
		changed = changed || after[i] != before[i]
	}
	if !changed {
		t.Errorf("魅力 +5 后随身宠物的忠诚应有变化: %v → %v", before, after)
	}

	// 结果窗口是 OK：任意选择都关窗，之后再选是 WRONG_STATE。
	if _, pushes = c.callp("npc_choose", `{"no":1}`); len(pushes) != 1 || pushes[0]["type"] != "npc_window_closed" {
		t.Errorf("OK 窗口选择后应只推 closed: %v", pushes)
	}
	c.wantErr(t, "npc_choose", `{"no":1}`, "WRONG_STATE")
}

func TestCharmCapsAt100(t *testing.T) {
	e := newEnv(t)
	c := charmSetup(t, e, 1, 96, 1_000_000)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, charmNPC))
	c.callp("npc_choose", `{"no":1}`)
	c.callp("npc_choose", `{"no":1}`)
	withRec(t, e, "alice", func(r *record) {
		// 96 的价：1 × 10 × 32 × 1 = 320；只涨到 100（+4），价钱照 5 点收（原版行为）。
		if r.Charm != 100 || r.Gold != 1_000_000-320 {
			t.Errorf("96 美容后应是 100、扣 320，实际 charm=%d gold=%d", r.Charm, r.Gold)
		}
	})
}

func TestCharmNotEnoughGoldChangesNothing(t *testing.T) {
	e := newEnv(t)
	c := charmSetup(t, e, 1, 60, 199)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, charmNPC))
	c.callp("npc_choose", `{"no":1}`)
	c.wantErr(t, "npc_choose", `{"no":1}`, "NOT_ENOUGH_GOLD")
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != 199 || r.Charm != 60 {
			t.Errorf("钱不够时金币和魅力都不该变，实际 gold=%d charm=%d", r.Gold, r.Charm)
		}
	})
	// 窗口还开着，选否关窗，数据不变。
	if _, pushes := c.callp("npc_choose", `{"no":2}`); len(pushes) != 1 || pushes[0]["type"] != "npc_window_closed" {
		t.Errorf("选否应关窗: %v", pushes)
	}
}

func TestCharmFullShowsPerfectWithoutCharge(t *testing.T) {
	e := newEnv(t)
	c := charmSetup(t, e, 1, 100, 1_000_000)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, charmNPC))
	m, pushes := c.callp("npc_choose", `{"no":1}`)
	if m["ok"] != true || len(pushes) != 2 {
		t.Fatalf("魅力满选给我魅力应直接给结果窗口: %v %v", m, pushes)
	}
	if text := pushes[1]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "你的魅力真是完美") {
		t.Errorf("满值窗口文案不对: %q", text)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != 1_000_000 || r.Charm != 100 {
			t.Errorf("满值不收钱、不加魅力，实际 gold=%d charm=%d", r.Gold, r.Charm)
		}
	})
}

func TestCharmWhatNotChangesNothing(t *testing.T) {
	e := newEnv(t)
	c := charmSetup(t, e, 1, 60, 1_000_000)
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, charmNPC))
	if _, pushes := c.callp("npc_choose", `{"no":2}`); len(pushes) != 1 || pushes[0]["type"] != "npc_window_closed" {
		t.Errorf("什么也不做应只关窗: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Gold != 1_000_000 || r.Charm != 60 {
			t.Errorf("什么也不做不该改数据，实际 gold=%d charm=%d", r.Gold, r.Charm)
		}
	})
}

// petLoyalties 取随身宠物（槽位顺序）的忠诚，同宠物页 pets_changed 里的 loyalty。
func petLoyalties(t *testing.T, e *env) []int {
	t.Helper()
	var out []int
	withRec(t, e, "alice", func(r *record) {
		for _, p := range r.petsView()["pets"].([]petView) {
			out = append(out, p.Loyalty)
		}
	})
	return out
}
