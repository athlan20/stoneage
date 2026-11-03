package character

import (
	"context"
	"math/rand/v2"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-207：加工、料理的合成端到端（真实数据库、真实协议）。材料和配方都是 gamedata 里的真实数据：
// 1、2 是斧头，种子 1 时两件合成得到合成枪1（4200）；1 和 190 合成总是匹配不到，走 fallback；
// 1234（小块肉）是料理，和斧头混合是 illegal；1233（迷人香菇）不能作材料；3 号斧头叠放用。
const (
	mergeAxeA   = 1
	mergeAxeB   = 2
	mergeFailB  = 190
	mergeDish   = 1234
	mergeNotMat = 1233
	mergeRecipe = 4200
	mergePetTpl = 1062 // 凯比，加工在 3 号技能栏位
)

// bagEntry 是测试背包里的一格。
type bagEntry struct{ slot, id, count int }

// seededRnd 是合成的随机源：固定种子，和协议用例的做法一样，结果可复现。
func seededRnd(seed uint64) func(int) int { return rand.New(rand.NewPCG(seed, 7)).IntN }

// mergeEnv 让勇者小明站在村长家（1006，15，22），身边是凯比（5 级），背包按 bag 摆好，然后进入世界。
func mergeEnv(t *testing.T, bag []bagEntry, setup ...func(*Service)) (*env, *client) {
	t.Helper()
	e := newEnemyEnv(t, setup...)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", 1006, 15, 22); err != nil {
		t.Fatal(err)
	}
	givePetSetup(t, mergePetTpl, 5)(e)
	for _, b := range bag {
		putItemSetup(t, b.slot, b.id, b.count)(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// bagRows 读数据库里勇者小明的背包（按槽位从小到大）。
func (e *env) bagRows(t *testing.T) []itemRec {
	t.Helper()
	items, err := e.chars.loadItems(context.Background(), e.presence(t, "勇者小明").charID)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// jsonInts 把 JSON 解出来的数组转成 []int。
func jsonInts(t *testing.T, v any) []int {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("应是数组: %v", v)
	}
	out := make([]int, len(arr))
	for i, x := range arr {
		out[i] = int(x.(float64))
	}
	return out
}

// mergeReq 是 pet_skill 的合成请求（加工，3 号技能栏位，宠物在 0 号栏位）。
func mergeReq(items string) string {
	return `{"slot":0,"skill":3,"items":` + items + `}`
}

func TestMergeCreatedConsumesAndPlaces(t *testing.T) {
	e, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 1}, {7, mergeDish, 1}},
		func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	d, pushes := c.okp("pet_skill", mergeReq(`[5,6]`))
	wantPushes(t, pushes, "inventory_changed")
	if d["result"] != "created" {
		t.Fatalf("种子 1 时两件斧头应合成成功: %v", d)
	}
	item := d["item"].(map[string]any)
	if item["slot"] != float64(5) || item["item_id"] != float64(mergeRecipe) || item["name"] != "合成枪1" {
		t.Errorf("产物应放进扣料后的第一个空格 5 号，得到 %v", item)
	}
	if got := jsonInts(t, d["consumed"]); !reflect.DeepEqual(got, []int{5, 6}) {
		t.Errorf("consumed = %v", got)
	}
	want := []itemRec{{Slot: 5, ItemID: mergeRecipe, Count: 1, Merged: true}, {Slot: 7, ItemID: mergeDish, Count: 1}}
	if got := e.bagRows(t); !reflect.DeepEqual(got, want) {
		t.Errorf("背包 = %+v，应为 %+v（只有产物带合成品标记）", got, want)
	}
}

func TestMergeFallbackReturnsOneMaterial(t *testing.T) {
	e, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeFailB, 1}},
		func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	d, pushes := c.okp("pet_skill", mergeReq(`[5,6]`))
	wantPushes(t, pushes, "inventory_changed")
	if d["result"] != "fallback" {
		t.Fatalf("这一对材料配不出配方，应是 fallback: %v", d)
	}
	rows := e.bagRows(t)
	if len(rows) != 1 || rows[0].Slot != 5 || rows[0].Merged || (rows[0].ItemID != mergeAxeA && rows[0].ItemID != mergeFailB) {
		t.Errorf("应只剩一件随机还回的材料，放在 5 号格（不带标记）: %+v", rows)
	}
	if item := d["item"].(map[string]any); item["item_id"] != float64(rows[0].ItemID) {
		t.Errorf("响应的 item 和背包不一致: %v / %+v", item, rows)
	}
}

func TestMergeMixedFoodAndItemConsumesAll(t *testing.T) {
	e, c := mergeEnv(t, []bagEntry{{5, mergeDish, 1}, {6, mergeAxeA, 1}},
		func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	d, pushes := c.okp("pet_skill", mergeReq(`[5,6]`))
	wantPushes(t, pushes, "inventory_changed")
	if d["result"] != "illegal" || d["message"] != "非法的合成方法" || d["item"] != nil {
		t.Errorf("食物与道具混合应是 illegal、没有 item: %v", d)
	}
	if rows := e.bagRows(t); len(rows) != 0 {
		t.Errorf("两件都应扣光，背包 = %+v", rows)
	}
}

// 冷却：2 件 5 秒。冷却中的请求也扣料、还回一件材料，并且刷新时间戳，所以隔 4 秒再来还是冷却。
func TestMergeCooldownRefreshesTimestamp(t *testing.T) {
	var clock atomic.Int64
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local).Unix()
	clock.Store(base)
	bag := []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 1}, {7, mergeAxeA, 1}, {8, mergeAxeB, 1},
		{9, mergeAxeA, 1}, {10, mergeAxeB, 1}, {11, mergeAxeA, 1}, {12, mergeAxeB, 1}, {13, mergeAxeA, 1}, {14, mergeAxeB, 1}}
	_, c := mergeEnv(t, bag, func(s *Service) {
		s.opts.MergeRnd = seededRnd(1)
		s.opts.Now = func() time.Time { return time.Unix(clock.Load(), 0) }
	})
	steps := []struct {
		at    int64
		items string
		kind  string // 期望的 result；"" 表示不是冷却（配方或失败都行）
	}{
		{0, `[5,6]`, "created"},
		{1, `[7,8]`, "cooldown"},  // 距上次 1 秒
		{5, `[9,10]`, "cooldown"}, // 距上次（冷却中那次）4 秒，时间戳已被刷新
		{9, `[11,12]`, "cooldown"},
		{15, `[13,14]`, ""}, // 距上次 6 秒，可以合成
	}
	for _, st := range steps {
		clock.Store(base + st.at)
		m, pushes := c.callp("pet_skill", mergeReq(st.items))
		if m["ok"] != true {
			t.Fatalf("t+%d 合成 %s 失败: %v", st.at, st.items, m)
		}
		wantPushes(t, pushes, "inventory_changed")
		d := m["data"].(map[string]any)
		if st.kind != "" && d["result"] != st.kind {
			t.Errorf("t+%d 合成 %s：result = %v，应为 %s", st.at, st.items, d["result"], st.kind)
		}
		if st.kind == "" && d["result"] == "cooldown" {
			t.Errorf("t+%d 应已过冷却，得到 %v", st.at, d)
		}
		if st.kind == "cooldown" && d["message"] != "合成料理过於频繁，休息一下比较好哟。" {
			t.Errorf("冷却的提示 = %v", d["message"])
		}
	}
}

// 重新进入世界，冷却清零（时间戳只在内存里）。
func TestMergeCooldownClearedByReenter(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local).Unix())
	_, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 1}, {7, mergeAxeA, 1}, {8, mergeAxeB, 1}},
		func(s *Service) {
			s.opts.MergeRnd = seededRnd(1)
			s.opts.Now = func() time.Time { return time.Unix(clock.Load(), 0) }
		})
	c.okp("pet_skill", mergeReq(`[5,6]`))
	if m, _ := c.callp("leave_world", `{}`); m["ok"] != true {
		t.Fatalf("leave_world 失败: %v", m)
	}
	c.ok("enter_world", `{"slot":0}`)
	if d, _ := c.okp("pet_skill", mergeReq(`[7,8]`)); d["result"] == "cooldown" {
		t.Errorf("重新进入世界后不应冷却: %v", d)
	}
}

// 参数错误和规则错误都不扣任何东西，也不推送。
func TestMergeErrorsChangeNothing(t *testing.T) {
	e, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 1}, {7, mergeNotMat, 1}, {8, mergeAxeA, 3}},
		func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	before := e.bagRows(t)
	cases := []struct {
		name, data, code string
	}{
		{"缺 items", `{"slot":0,"skill":3}`, protocol.CodeBadRequest},
		{"items 不是列表", `{"slot":0,"skill":3,"items":"5,6"}`, protocol.CodeBadRequest},
		{"只有一件", mergeReq(`[5]`), protocol.CodeBadRequest},
		{"重复的格号", mergeReq(`[5,5]`), protocol.CodeBadRequest},
		{"格号越界", mergeReq(`[5,20]`), protocol.CodeBadRequest},
		{"装备格不能合成", mergeReq(`[5,3]`), protocol.CodeBadRequest},
		{"空格", mergeReq(`[5,15]`), protocol.CodeEmptySlot},
		{"非材料", mergeReq(`[5,7]`), protocol.CodeItemUnusable},
		{"战斗技能", `{"slot":0,"skill":1,"items":[5,6]}`, protocol.CodeSkillUnusable},
		{"没有宠物", `{"slot":1,"skill":3,"items":[5,6]}`, protocol.CodeNoPet},
	}
	for _, tc := range cases {
		m, pushes := c.callp("pet_skill", tc.data)
		if errCode(m) != tc.code || len(pushes) != 0 {
			t.Errorf("%s：应回复 %s 且不推送，得到 %v %v", tc.name, tc.code, m, pushes)
		}
	}
	if after := e.bagRows(t); !reflect.DeepEqual(before, after) {
		t.Errorf("出错后背包变了: %+v -> %+v", before, after)
	}
}

// 叠放的材料格只扣 1 件。
func TestMergeStackedMaterialLosesOne(t *testing.T) {
	e, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 3}},
		func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	c.okp("pet_skill", mergeReq(`[5,6]`))
	rows := e.bagRows(t)
	if len(rows) != 2 || rows[1].Slot != 6 || rows[1].Count != 2 {
		t.Errorf("6 号格应还剩 2 件，背包 = %+v", rows)
	}
}

// 5 号格空着，其余 14 格的材料全部扣光后，产物放进扣料之后的第一个空格（5 号）。
func TestMergeProductGoesToFirstFreedSlot(t *testing.T) {
	var bag []bagEntry
	items := "["
	for slot := 6; slot <= 19; slot++ {
		bag = append(bag, bagEntry{slot, mergeAxeA, 1})
		if slot > 6 {
			items += ","
		}
		items += itoa(slot)
	}
	items += "]"
	e, c := mergeEnv(t, bag, func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	d, _ := c.okp("pet_skill", mergeReq(items))
	if item, ok := d["item"].(map[string]any); !ok || item["slot"] != float64(5) {
		t.Errorf("产物应放进 5 号格（扣料之后的第一个空格）: %v", d)
	}
	rows := e.bagRows(t)
	if len(rows) != 1 || rows[0].Slot != 5 {
		t.Errorf("14 件扣光后应只剩 5 号格的一件: %+v", rows)
	}
}

// 背包一个空格都没有：BAG_FULL，什么都不动。
func TestMergeBagFullChangesNothing(t *testing.T) {
	var bag []bagEntry
	for slot := 5; slot <= 19; slot++ {
		bag = append(bag, bagEntry{slot, mergeAxeA, 1})
	}
	e, c := mergeEnv(t, bag, func(s *Service) { s.opts.MergeRnd = seededRnd(1) })
	before := e.bagRows(t)
	m, pushes := c.callp("pet_skill", mergeReq(`[5,6]`))
	if errCode(m) != protocol.CodeBagFull || len(pushes) != 0 {
		t.Errorf("背包满应回复 BAG_FULL: %v %v", m, pushes)
	}
	if after := e.bagRows(t); !reflect.DeepEqual(before, after) {
		t.Errorf("背包满时背包变了")
	}
}

// 写库失败（产物的插入被触发器拒绝）：背包不变、不推送、冷却时间不记。
func TestMergeWriteFailureChangesNothing(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local).Unix())
	e, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 1}, {7, mergeAxeA, 1}, {8, mergeAxeB, 1}},
		func(s *Service) {
			s.opts.MergeRnd = seededRnd(1)
			s.opts.Now = func() time.Time { return time.Unix(clock.Load(), 0) }
		})
	e.exec(t, `CREATE FUNCTION merge_reject() RETURNS trigger AS $$ BEGIN
		IF NEW.item_id = 4200 THEN RAISE EXCEPTION 'reject'; END IF; RETURN NEW; END $$ LANGUAGE plpgsql`)
	e.exec(t, `CREATE TRIGGER merge_reject BEFORE INSERT ON character_items FOR EACH ROW EXECUTE FUNCTION merge_reject()`)
	before := e.bagRows(t)
	m, pushes := c.callp("pet_skill", mergeReq(`[5,6]`))
	if m["ok"] == true || len(pushes) != 0 {
		t.Fatalf("写库失败应回复错误且不推送: %v %v", m, pushes)
	}
	if after := e.bagRows(t); !reflect.DeepEqual(before, after) {
		t.Errorf("写库失败后背包变了: %+v -> %+v", before, after)
	}
	// 时间没有记下：紧接着的请求不是冷却（冷却也是成功的回复，所以看 result）。
	if m, _ := c.callp("pet_skill", mergeReq(`[7,8]`)); m["ok"] != true || m["data"].(map[string]any)["result"] == "cooldown" {
		t.Errorf("写库失败的那次不应记下冷却时间: %v", m)
	}
}

// 观察方（agent 在操控）发合成请求被拒，背包不变。
func TestMergeObserverRejected(t *testing.T) {
	e, c := mergeEnv(t, []bagEntry{{5, mergeAxeA, 1}, {6, mergeAxeB, 1}})
	agent := e.agentFor(c)
	agent.send("enter_world", `{"slot":0}`)
	e.expectAgentEnter(t, c, agent)
	before := e.bagRows(t)
	if m, pushes := c.callp("pet_skill", mergeReq(`[5,6]`)); errCode(m) != protocol.CodeControlledByAgent || len(pushes) != 0 {
		t.Errorf("观察方应回复 CONTROLLED_BY_AGENT: %v %v", m, pushes)
	}
	if after := e.bagRows(t); !reflect.DeepEqual(before, after) {
		t.Errorf("观察方的请求改了背包")
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
