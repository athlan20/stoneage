package gamedata

import (
	"math/rand/v2"
	"testing"
)

// fixed 返回一个每次都给出同一个值的 rnd（值要小于 n）。
func fixed(v int) func(int) int { return func(n int) int { return v % n } }

func TestNewPetGrowthExtremes(t *testing.T) {
	tmpl := PetTemplate{InitNum: 10, LvUpPoint: 4, Base: Growth{20, 12, 15, 25}}
	// rnd 恒为 4：资质 +2，之后的 10 次加点全部落在体力（rnd(4)=0 时加体力，这里 4%4=0）。
	base, ability := NewPetGrowth(tmpl, 1, fixed(4))
	if base != (Growth{22, 14, 17, 27}) {
		t.Errorf("资质 +2 后应为 22 14 17 27，现在 %+v", base)
	}
	perLevel := 10 // 等级 1：INITNUM + 0 × LVUPPOINT
	wantVital := perLevel * (base.Vital + 10)
	if ability.Vital != wantVital || ability.Strength != perLevel*14 || ability.Toughness != perLevel*17 || ability.Dexterity != perLevel*27 {
		t.Errorf("能力不对: %+v（体力应为 %d）", ability, wantVital)
	}
}

func TestNewPetGrowthLowEnd(t *testing.T) {
	tmpl := PetTemplate{InitNum: 10, LvUpPoint: 4, Base: Growth{20, 12, 15, 25}}
	// rnd 恒为 0：资质 -2；10 次加点都落在 rnd(4)=0 的体力上。
	base, _ := NewPetGrowth(tmpl, 1, fixed(0))
	if base != (Growth{18, 10, 13, 23}) {
		t.Errorf("资质 -2 后应为 18 10 13 23，现在 %+v", base)
	}
}

func TestNewPetGrowthLevel(t *testing.T) {
	tmpl := PetTemplate{InitNum: 10, LvUpPoint: 4, Base: Growth{20, 12, 15, 25}}
	// 等级 5：(5−1)×4 + 10 = 26。rnd 恒为 2：资质 ±2 加 0；10 次加点都落在 rnd(4)=2 的耐久力上，耐久 15+10=25。
	_, ability := NewPetGrowth(tmpl, 5, fixed(2))
	if ability.Vital != 26*20 || ability.Strength != 26*12 || ability.Toughness != 26*25 || ability.Dexterity != 26*25 {
		t.Errorf("能力不对: %+v", ability)
	}
}

func TestPetRank(t *testing.T) {
	cases := map[int]int{100: 0, 99: 1, 95: 1, 94: 2, 90: 2, 89: 3, 85: 3, 84: 4, 80: 4, 79: 5, 0: 5}
	for sum, want := range cases {
		tmpl := PetTemplate{Base: Growth{Vital: sum}}
		if got := PetRank(tmpl); got != want {
			t.Errorf("资质合计 %d 的品质 = %d，应为 %d", sum, got, want)
		}
	}
}

func TestPetDerived(t *testing.T) {
	// 能力单位是 1/100 点：体力 3、腕力 2、耐久 1、速度 5 点。
	a := Ability{Vital: 300, Strength: 200, Toughness: 100, Dexterity: 500}
	if got := PetAttack(a); got != 2 { // 2 + 0.1×1 + 0.1×3 + 0.05×5 = 2.65
		t.Errorf("攻击 = %d，应为 2", got)
	}
	if got := PetDefense(a); got != 1 { // 1 + 0.1×2 + 0.1×3 + 0.05×5 = 1.75
		t.Errorf("防御 = %d，应为 1", got)
	}
	if got := PetAgility(a); got != 5 {
		t.Errorf("敏捷 = %d，应为 5", got)
	}
	if got := PetMaxHP(a); got != 20 { // 3×4 + 2 + 1 + 5
		t.Errorf("最大 HP = %d，应为 20", got)
	}
}

// STAGE-159：坏数据把能力写成 1 点时，最大血量按 1 算，不能是 0。
func TestPetMaxHPFloor(t *testing.T) {
	if got := PetMaxHP(Ability{Vital: 1, Strength: 1, Toughness: 1, Dexterity: 1}); got != 1 {
		t.Errorf("最大 HP = %d，应兜底为 1", got)
	}
}

func TestLoyalty(t *testing.T) {
	cases := []struct {
		name                                         string
		hostLevel, charm, trans, petLevel, modAI, vr int
		want                                         int
	}{
		{"普通", 10, 60, 0, 5, 150, 0, 88},
		{"可变部分 150 按 1.5 计，取整加 1", 10, 60, 0, 5, 150, 150, 89},
		{"转生主人加 (140−等级)÷2", 20, 60, 1, 40, 100, 0, 93},
		{"MODAI 为 0 按 100", 10, 60, 0, 5, 0, 0, 100},
		{"超过 100 截到 100", 100, 60, 0, 1, 100, 0, 100},
	}
	for _, c := range cases {
		if got := Loyalty(c.hostLevel, c.charm, c.trans, c.petLevel, c.modAI, c.vr); got != c.want {
			t.Errorf("%s：忠诚 = %d，应为 %d", c.name, got, c.want)
		}
	}
}

// seq 返回按 n 取值的 rnd：分随机点时（n=4）恒给 point，取成长系数时给 rate。
func seq(point, rate int) func(int) int {
	return func(n int) int {
		if n == 4 {
			return point
		}
		return rate % n
	}
}

func TestPetLevelUpFormula(t *testing.T) {
	base := Growth{20, 12, 15, 25}
	// 10 个随机点全落在体力；品质 0 的系数取下限 4.50。
	got := PetLevelUp(base, 0, seq(0, 0))
	want := Ability{Vital: 135, Strength: 54, Toughness: 67, Dexterity: 112} // (20+10)×4.5、12×4.5、15×4.5=67.5、25×4.5=112.5
	if got != want {
		t.Errorf("增量 = %+v，应为 %+v", got, want)
	}
	// 品质 5 的系数取上限 6.00：随机点全落在速度。
	got = PetLevelUp(base, 5, seq(3, 50))
	want = Ability{Vital: 120, Strength: 72, Toughness: 90, Dexterity: 210}
	if got != want {
		t.Errorf("增量 = %+v，应为 %+v", got, want)
	}
}

// 原版用 float 运算再截断：系数 4.68 在 float 里略小于 4.68，25 × 4.68 截成 116，整数算是 117。
func TestPetLevelUpFloatTruncation(t *testing.T) {
	got := PetLevelUp(Growth{25, 0, 0, 0}, 0, seq(1, 18)) // 系数 450+18 = 4.68，随机点全给腕力
	if got.Vital != 116 {
		t.Errorf("体力增量 = %d，应为 116（同原版 float 截断）", got.Vital)
	}
}

// 真随机下：每项增量都落在 [资质 × 系数下限, (资质 + 10) × 系数上限] 里，且总是正的。
func TestPetLevelUpRange(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	base := Growth{18, 10, 13, 23}
	for rank := 0; rank <= 5; rank++ {
		lo, hi := petGrowthRate[rank][0], petGrowthRate[rank][1]
		for i := 0; i < 2000; i++ {
			d := PetLevelUp(base, rank, r.IntN)
			for k, pair := range [][2]int{{d.Vital, base.Vital}, {d.Strength, base.Strength},
				{d.Toughness, base.Toughness}, {d.Dexterity, base.Dexterity}} {
				if pair[0] < pair[1]*lo/100-1 || pair[0] > (pair[1]+10)*hi/100 {
					t.Fatalf("品质 %d 第 %d 项增量 %d 超出范围（资质 %d）", rank, k, pair[0], pair[1])
				}
			}
		}
	}
}

func TestPetMaxLevel(t *testing.T) {
	if got := PetMaxLevel(PetTemplate{}); got != PetLevelCap {
		t.Errorf("没填上限的宠物上限 = %d，应为 %d", got, PetLevelCap)
	}
	if got := PetMaxLevel(PetTemplate{LimitLevel: 10}); got != 10 {
		t.Errorf("填了 10 级上限的宠物上限 = %d", got)
	}
}

func TestAddVariableAI(t *testing.T) {
	if got := AddVariableAI(9800, PetLevelUpAI); got != MaxVariableAI {
		t.Errorf("可变忠诚 = %d，应封顶在 %d", got, MaxVariableAI)
	}
	if got := AddVariableAI(0, PetLevelUpAI); got != 500 {
		t.Errorf("可变忠诚 = %d，应为 500", got)
	}
}
