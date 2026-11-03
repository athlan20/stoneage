// STAGE-250：骑宠血量随单位快照下发（docs/protocol.md 10.2、10.7）。骑乘时人物单位和每回合 state 带 mount，
// 没骑乘时整个字段省略；对方看不到骑宠的精确血量。

package battle

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMountViewOnlyWhileRiding 没骑乘的人物单位 JSON 里没有 mount 字段，骑乘时带骑宠的名字和血量。
func TestMountViewOnlyWhileRiding(t *testing.T) {
	plain := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	if raw := marshalJSON(t, plain.PlayerUnit().view(0, plain.You)); strings.Contains(raw, `"mount"`) {
		t.Fatalf("没骑乘的人物单位不应带 mount：%s", raw)
	}

	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u, pet := setRideStats(b)
	v := u.view(0, b.You)
	if v.Mount == nil || v.Mount.Name != pet.Name || v.Mount.HP == nil || *v.Mount.HP != pet.HP {
		t.Fatalf("骑乘时 mount = %+v，want 骑宠 %s 血量 %d", v.Mount, pet.Name, pet.HP)
	}
	if *v.Mount.MaxHP != pet.MaxHP || v.Mount.HPPct != pet.HPPct() {
		t.Fatalf("mount 最大血量/百分比 = %d/%d，want %d/%d", *v.Mount.MaxHP, v.Mount.HPPct, pet.MaxHP, pet.HPPct())
	}
	if raw := marshalJSON(t, v); !strings.Contains(raw, `"mount":{"name":`) {
		t.Fatalf("骑乘时 JSON 应带 mount：%s", raw)
	}
}

// TestMountViewRefreshesEachRound 每回合 state 里的 mount 跟着骑宠扣血刷新；对方只看到百分比。
func TestMountViewRefreshesEachRound(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u, pet := setRideStats(b)
	before := *u.stateEntry(0, b.You).Mount.HP

	var ev Event
	b.hurt(u, 30, "hit", &ev)
	e := u.stateEntry(0, b.You)
	if e.Mount == nil || e.Mount.HP == nil || *e.Mount.HP != pet.HP {
		t.Fatalf("回合快照里 mount.hp = %+v，want %d", e.Mount, pet.HP)
	}
	if *e.Mount.HP >= before {
		t.Fatalf("骑宠应分摊到伤害：扣血前 %d，扣血后 %d", before, *e.Mount.HP)
	}

	enemy := u.stateEntry(1, b.You)
	if enemy.Mount == nil || enemy.Mount.HP != nil || enemy.Mount.MaxHP != nil || enemy.Mount.HPPct != pet.HPPct() {
		t.Fatalf("对方视角的 mount = %+v，want 只有 hp_pct", enemy.Mount)
	}
}

func marshalJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
