package character

// STAGE-266：人物转生的事件清空、赠宠表、空栏位（纯函数），以及单事务落库（能力与转生次数真的写进库、赠宠、栏满整次拒绝）。
// 落库测试需要数据库（同窗口流程测试）。

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTenseiPresentTable(t *testing.T) {
	for _, c := range []struct {
		floor int
		want  [5]int
	}{
		{1000, [5]int{6, 272, 244, 232, 304}},
		{2000, [5]int{272, 244, 232, 6, 304}},
		{3000, [5]int{244, 232, 6, 272, 304}},
		{4000, [5]int{232, 6, 272, 244, 304}},
	} {
		for n := 1; n <= 5; n++ {
			if got, ok := tenseiPresentOf(c.floor, n); !ok || got != c.want[n-1] {
				t.Errorf("地图 %d 第 %d 转 = (%d,%v)，应为 %d", c.floor, n, got, ok, c.want[n-1])
			}
		}
	}
	if _, ok := tenseiPresentOf(1100, 1); ok {
		t.Error("不是主城不应有赠宠")
	}
	if _, ok := tenseiPresentOf(1000, 0); ok {
		t.Error("0 转不应有赠宠")
	}
	if _, ok := tenseiPresentOf(1000, 6); ok {
		t.Error("6 转不应有赠宠")
	}
}

func TestTenseiFreeSlot(t *testing.T) {
	if got := tenseiFreeSlot(nil); got != 0 {
		t.Errorf("没有宠物应取 0 号栏位，得到 %d", got)
	}
	if got := tenseiFreeSlot([]petRec{{Slot: 0}, {Slot: 2}}); got != 1 {
		t.Errorf("0、2 被占应取 1，得到 %d", got)
	}
	full := make([]petRec, MaxPetSlots)
	for i := range full {
		full[i].Slot = i
	}
	if got := tenseiFreeSlot(full); got != -1 {
		t.Errorf("全满应返回 -1，得到 %d", got)
	}
}

// 事件清空：白名单里原本完成和进行中的保留，其余清掉，再按新转生次数补 48–52。
func TestPlanTenseiEvents(t *testing.T) {
	before := progress{quests: map[int]int{}, events: map[int]int{}}
	for _, ev := range []int{69, 109, 20, 33} { // 69、109 在白名单，20、33 不在
		applyEventEdit(before, editSetDone, ev)
	}
	applyEventEdit(before, editSetDoing, 70)
	applyEventEdit(before, editSetDoing, 41) // 不在白名单的进行中
	r := &record{Quests: stateRecs(before.quests), EventFlags: stateRecs(before.events)}
	for _, ev := range []int{69, 109, 20, 33} {
		if !r.eventDone(ev) {
			t.Fatalf("造数据失败：事件 %d 应已完成", ev)
		}
	}

	after := planTenseiEvents(r, 2)
	got := &record{Quests: stateRecs(after.quests), EventFlags: stateRecs(after.events)}
	for _, ev := range []int{69, 109, 48, 49} {
		if !got.eventDone(ev) {
			t.Errorf("事件 %d 应已完成", ev)
		}
	}
	if !got.eventDoing(70) {
		t.Error("事件 70 原本进行中，应保留")
	}
	for _, ev := range []int{20, 33, 50, 51, 52} {
		if got.eventDone(ev) {
			t.Errorf("事件 %d 不应已完成", ev)
		}
	}
	if got.eventDoing(41) {
		t.Error("不在白名单的进行中事件 41 应被清掉")
	}
	// 5 转补满 48–52；原记录不被修改。
	full := &record{}
	f := planTenseiEvents(full, 5)
	fr := &record{Quests: stateRecs(f.quests), EventFlags: stateRecs(f.events)}
	for _, ev := range tenseiRebirthEvents {
		if !fr.eventDone(ev) {
			t.Errorf("5 转应补事件 %d", ev)
		}
	}
	if !r.eventDone(20) {
		t.Error("planTenseiEvents 不应改原记录")
	}
}

// tenseiDBRow 直接从库里读角色转生相关的列（withRec 看的是内存，这里要验证真的落库）。
func tenseiDBRow(t *testing.T, e *env) (trans, level, exp, sp, qTotal, lTotal, vital, str, tou, dex int) {
	t.Helper()
	err := e.chars.db.QueryRow(context.Background(), `SELECT transmigration, level, exp, skill_points,
			trans_quest_total, trans_level_total, vital, strength, toughness, dexterity
		FROM characters WHERE name = $1`, "勇者小明").Scan(&trans, &level, &exp, &sp, &qTotal, &lTotal, &vital, &str, &tou, &dex)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func tenseiPetCount(t *testing.T, e *env) (n int) {
	t.Helper()
	err := e.chars.db.QueryRow(context.Background(), `SELECT count(*) FROM character_pets
		WHERE character_id = (SELECT id FROM characters WHERE name = $1)`, "勇者小明").Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return
}

// 确定后：能力、转生次数、等级、能力点、累计真的写进库；赠一只 1 级乌力斯坦（萨姆吉尔 1 转）；推 pets_changed 与 notice。
func TestTenseiCommitPersistsAndGivesPet(t *testing.T) {
	e := newEnv(t)
	c := tenseiSetup(t, e, 80, 0)
	var before record
	withRec(t, e, "alice", func(r *record) { before = *r })
	want, err := CalcTensei(TenseiInput{
		Vital: before.Stats.Vital, Strength: before.Stats.Strength, Toughness: before.Stats.Toughness, Dexterity: before.Stats.Dexterity,
		Level: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	pets0 := tenseiPetCount(t, e)

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, tenseiNPC))
	c.callp("npc_choose", `{"no":1}`)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	var order []string
	notice := ""
	for _, p := range pushes {
		typ := fmt.Sprint(p["type"])
		order = append(order, typ)
		if typ == "notice" {
			notice = fmt.Sprint(p["data"].(map[string]any)["text"])
		}
	}
	idx := func(typ string) int {
		for i, o := range order {
			if o == typ {
				return i
			}
		}
		return -1
	}
	if idx("status_changed") < 0 || idx("pets_changed") < idx("status_changed") || idx("notice") < idx("pets_changed") {
		t.Errorf("推送顺序应是 status_changed → pets_changed → … → notice: %v", order)
	}
	if !strings.HasPrefix(notice, "拿到") {
		t.Errorf("赠宠提示应以“拿到”开头: %q", notice)
	}

	trans, level, exp, sp, qTotal, lTotal, vital, str, tou, dex := tenseiDBRow(t, e)
	if trans != 1 || level != 1 || exp != 0 || sp != 10 || qTotal != 20 || lTotal != 80 {
		t.Errorf("库里 trans=%d lv=%d exp=%d sp=%d 累计=%d/%d", trans, level, exp, sp, qTotal, lTotal)
	}
	if (Stats{Vital: vital, Strength: str, Toughness: tou, Dexterity: dex}) != (Stats{Vital: want.Vital, Strength: want.Strength, Toughness: want.Toughness, Dexterity: want.Dexterity}) {
		t.Errorf("库里四项 %d/%d/%d/%d，应为 %+v", vital, str, tou, dex, want)
	}
	if got := tenseiPetCount(t, e); got != pets0+1 {
		t.Errorf("宠物数 %d → %d，应多一只", pets0, got)
	}
	var tmpl, plv int
	if err := e.chars.db.QueryRow(context.Background(), `SELECT template_id, level FROM character_pets
		WHERE character_id = (SELECT id FROM characters WHERE name = $1) AND template_id = 6
		ORDER BY slot DESC LIMIT 1`, "勇者小明").Scan(&tmpl, &plv); err != nil || plv != 1 {
		t.Errorf("萨姆吉尔 1 转应送 1 级乌力斯坦(6): %v lv=%d", err, plv)
	}
}

// 宠物栏 20 格全满（STAGE-346）：整次转生拒绝，弹栏满提示，库里的角色和宠物都不变。
func TestTenseiPetsFullRejectsWholeRebirth(t *testing.T) {
	e := newEnv(t)
	c := tenseiSetup(t, e, 80, 0)
	for tenseiPetCount(t, e) < MaxPetSlots {
		givePetSetup(t, 6, 1)(e)
	}
	t0, l0, _, sp0, _, _, v0, s0, to0, d0 := tenseiDBRow(t, e)

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, tenseiNPC))
	c.callp("npc_choose", `{"no":1}`)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	if len(pushes) == 0 {
		t.Fatal("栏满时应弹提示窗口")
	}
	text, opts := tenseiWindowText(t, pushes[len(pushes)-1])
	if text != tenseiPetsFullMsg || len(opts) != 0 {
		t.Errorf("栏满提示不对: %q %v", text, opts)
	}
	for _, p := range pushes {
		if p["type"] == "status_changed" || p["type"] == "pets_changed" {
			t.Errorf("栏满时不应推 %v", p["type"])
		}
	}
	t1, l1, _, sp1, q1, lt1, v1, s1, to1, d1 := tenseiDBRow(t, e)
	if t1 != t0 || l1 != l0 || sp1 != sp0 || v1 != v0 || s1 != s0 || to1 != to0 || d1 != d0 || q1 != 0 || lt1 != 0 {
		t.Errorf("栏满后库里数据不应变")
	}
	if got := tenseiPetCount(t, e); got != MaxPetSlots {
		t.Errorf("栏满后宠物数应仍是 %d，得到 %d", MaxPetSlots, got)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Transmigration != 0 || r.Level != 80 {
			t.Errorf("栏满后内存里的角色也不应变: trans=%d lv=%d", r.Transmigration, r.Level)
		}
	})
}
