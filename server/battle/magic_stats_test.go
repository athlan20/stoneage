// STAGE-226：战斗单位能读到魔法熟练度与抗性。怪物现算（LV×0.9、LV×0.5，随等级变），人物从快照带入，宠物为 0。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

func TestMonsterMagicFollowsLevel(t *testing.T) {
	tmpl, ok := gamedata.PetByID(1)
	if !ok {
		t.Fatal("模板 1（乌力）不在表里")
	}
	rnd := func(n int) int { return 0 }
	cases := []struct {
		level, exp, resist int
	}{
		{1, 0, 0},    // 1×0.9 截成 0，1×0.5 截成 0
		{2, 1, 1},    // 1.8 → 1，1.0 → 1
		{10, 9, 5},   // 9.0 → 9，5.0 → 5
		{15, 13, 7},  // 13.5 → 13，7.5 → 7
		{99, 89, 49}, // 89.1 → 89，49.5 → 49
	}
	for _, c := range cases {
		u := NewMonster(tmpl, c.level, 6, rnd, nil)
		for i := range u.MagicExp {
			if u.MagicExp[i] != c.exp {
				t.Errorf("LV%d 怪物 MagicExp[%d] = %d, want %d", c.level, i, u.MagicExp[i], c.exp)
			}
			if u.MagicResist[i] != c.resist {
				t.Errorf("LV%d 怪物 MagicResist[%d] = %d, want %d", c.level, i, u.MagicResist[i], c.resist)
			}
		}
		if u.MagicAttExp != ([4]int{}) || u.MagicDefExp != ([4]int{}) {
			t.Errorf("LV%d 怪物的熟练度经验应为 0，得到 %v %v", c.level, u.MagicAttExp, u.MagicDefExp)
		}
	}
}

func TestHumanUnitCarriesMagicStats(t *testing.T) {
	side := &SideInput{
		Name: "勇者", Level: 10,
		MagicExp:    [4]int{12, 0, 3, 0},
		MagicResist: [4]int{20, 10, 0, 5},
		MagicAttExp: [4]int{1, 2, 3, 4},
		MagicDefExp: [4]int{5, 6, 7, 8},
	}
	u := humanUnit(side, 0)
	if u.MagicExp != side.MagicExp || u.MagicResist != side.MagicResist ||
		u.MagicAttExp != side.MagicAttExp || u.MagicDefExp != side.MagicDefExp {
		t.Fatalf("人物单位没带上魔法数值：exp=%v resist=%v att=%v def=%v", u.MagicExp, u.MagicResist, u.MagicAttExp, u.MagicDefExp)
	}
}

func TestPetUnitMagicIsZero(t *testing.T) {
	u := petUnit(&PetInput{Name: "乌力", Level: 20, HP: 10, MaxHP: 10}, 5)
	if u.MagicExp != ([4]int{}) || u.MagicResist != ([4]int{}) {
		t.Fatalf("宠物的魔法熟练度与抗性应为 0（原版如此），得到 exp=%v resist=%v", u.MagicExp, u.MagicResist)
	}
}
