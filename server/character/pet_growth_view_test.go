package character

// STAGE-216：宠物的成长率和五档素质，纯函数测试（不连数据库）。
// 用的是乌力（模板 1，资质 体力20 腕力12 耐久15 速度25）；能力单位 1/100 点。

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 起始能力：最大 HP 20、攻击 2、防御 1、敏捷 5（算法见 gamedata/petstats.go）。
var growthOrigin = gamedata.Ability{Vital: 300, Strength: 200, Toughness: 100, Dexterity: 500}

// growthPet 造一只乌力：origin 为 nil 表示历史宠物。
func growthPet(level int, origin *PetOrigin, stats gamedata.Ability) *petRec {
	return &petRec{TemplateID: 1, Level: level, Stats: stats, Origin: origin}
}

// near 比较两个小数（成长率是两位小数，允许浮点误差）。
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// requireRates 检查四项成长率：want 为 nil 的项必须是 nil，否则必须等于 want。
func requireRates(t *testing.T, got ratesView, want [4]*float64) {
	t.Helper()
	fields := [4]struct {
		name string
		v    *float64
	}{{"hp", got.HP}, {"attack", got.Attack}, {"defense", got.Defense}, {"agility", got.Agility}}
	for i, f := range fields {
		switch {
		case want[i] == nil && f.v != nil:
			t.Errorf("rates.%s = %v，应为 null", f.name, *f.v)
		case want[i] != nil && f.v == nil:
			t.Errorf("rates.%s 为 null，应为 %v", f.name, *want[i])
		case want[i] != nil && !near(*f.v, *want[i]):
			t.Errorf("rates.%s = %v，应为 %v", f.name, *f.v, *want[i])
		}
	}
}

func f64(v float64) *float64 { return &v }

// 1 级起养，现在 11 级：升了 10 级。最大 HP 20→96 得 7.6；攻击 2→15 得 1.3；防御 1→13 得 1.2；敏捷 5→14 得 0.9。
var growthCurStats = gamedata.Ability{Vital: 1500, Strength: 1200, Toughness: 1000, Dexterity: 1400}

func TestPetGrowthViewRaised(t *testing.T) {
	p := growthPet(11, &PetOrigin{Level: 1, Stats: growthOrigin}, growthCurStats)
	g := growthOf(p)
	if g.Basis != growthRaised || g.FromLevel == nil || *g.FromLevel != 1 {
		t.Fatalf("basis %s、from_level %v，应为 raised、1", g.Basis, g.FromLevel)
	}
	requireRates(t, g.Rates, [4]*float64{f64(7.6), f64(1.3), f64(1.2), f64(0.9)})
}

// 捕获时 5 级、现在 15 级：同样的能力增量除以 10 级；不能从 1 级算起（那样除以 14）。
func TestPetGrowthViewCaught(t *testing.T) {
	p := growthPet(15, &PetOrigin{Level: 5, Stats: growthOrigin}, growthCurStats)
	g := growthOf(p)
	if g.Basis != growthCaught || g.FromLevel == nil || *g.FromLevel != 5 {
		t.Fatalf("basis %s、from_level %v，应为 caught、5", g.Basis, g.FromLevel)
	}
	requireRates(t, g.Rates, [4]*float64{f64(7.6), f64(1.3), f64(1.2), f64(0.9)})
}

// 刚获得、还没升级：basis 仍按起始等级定，四项都是 null。
func TestPetGrowthViewNotLeveledUp(t *testing.T) {
	cases := []struct {
		name  string
		level int
		basis string
	}{
		{"1 级起养刚获得", 1, growthRaised},
		{"捕获时 5 级刚获得", 5, growthCaught},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			origin := &PetOrigin{Level: c.level, Stats: growthOrigin}
			g := growthOf(growthPet(c.level, origin, growthOrigin))
			if g.Basis != c.basis || g.FromLevel == nil || *g.FromLevel != c.level {
				t.Fatalf("basis %s、from_level %v，应为 %s、%d", g.Basis, g.FromLevel, c.basis, c.level)
			}
			requireRates(t, g.Rates, [4]*float64{})
		})
	}
}

// 历史宠物没有起始记录：unknown，from_level 为 null，四项为 null，不倒推。
func TestPetGrowthViewUnknown(t *testing.T) {
	g := growthOf(growthPet(11, nil, growthCurStats))
	if g.Basis != growthUnknown || g.FromLevel != nil {
		t.Fatalf("basis %s、from_level %v，应为 unknown、null", g.Basis, g.FromLevel)
	}
	requireRates(t, g.Rates, [4]*float64{})
}

// 两位小数：除不尽的四舍五入。gained 为 3：最大 HP 20→24 得 4/3=1.33；攻击 2→2 得 0；
// 整除的情况：最大 HP 20→32 得 4.00（写成 4），敏捷 5→17 得 4，攻击 2→3 得 1/3=0.33。
func TestPetGrowthViewTwoDecimals(t *testing.T) {
	cases := []struct {
		name  string
		stats gamedata.Ability
		want  [4]*float64
		json  string // 期望在 JSON 里出现的片段：数字，不是字符串
	}{
		{"除不尽", gamedata.Ability{Vital: 400, Strength: 200, Toughness: 100, Dexterity: 500},
			[4]*float64{f64(1.33), f64(0), f64(0), f64(0)}, `"hp":1.33`},
		{"整除与三分之一", gamedata.Ability{Vital: 300, Strength: 200, Toughness: 100, Dexterity: 1700},
			[4]*float64{f64(4), f64(0.33), f64(0.33), f64(4)}, `"hp":4,"attack":0.33,"defense":0.33,"agility":4`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := growthOf(growthPet(4, &PetOrigin{Level: 1, Stats: growthOrigin}, c.stats))
			requireRates(t, g.Rates, c.want)
			raw, err := json.Marshal(g.Rates)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), c.json) {
				t.Errorf("rates 编码为 %s，应包含 %s", raw, c.json)
			}
		})
	}
}

// 成长率的 hp 用最大 HP：当前 HP 从满改成 1，成长率不变；同一只宠物 petEntry 里 hp 变了、rates.hp 不变。
func TestPetGrowthViewHPUsesMaxHP(t *testing.T) {
	r := &record{Level: 1, Charm: gamedata.NewPlayerCharm}
	full := growthPet(11, &PetOrigin{Level: 1, Stats: growthOrigin}, growthCurStats)
	full.HP = gamedata.PetMaxHP(growthCurStats)
	hurt := *full
	hurt.HP = 1
	a, okA := r.petEntry(*full)
	b, okB := r.petEntry(hurt)
	if !okA || !okB {
		t.Fatal("乌力的模板应存在")
	}
	if a.HP == b.HP {
		t.Fatalf("测试前提不成立：两次的当前 HP 都是 %d", a.HP)
	}
	if a.Growth.Rates.HP == nil || b.Growth.Rates.HP == nil || !near(*a.Growth.Rates.HP, *b.Growth.Rates.HP) {
		t.Errorf("当前 HP 变了，rates.hp 也变了：满血 %v，1 点血 %v", a.Growth.Rates.HP, b.Growth.Rates.HP)
	}
	if !near(*a.Growth.Rates.HP, 7.6) {
		t.Errorf("rates.hp = %v，应为 7.6（最大 HP 20→96，10 级）", *a.Growth.Rates.HP)
	}
}

// 历史宠物或刚获得的宠物：unknown / null 的 JSON 形状，rates 本身是对象，四项写成 null（不省略）。
func TestPetGrowthViewJSONNulls(t *testing.T) {
	raw, err := json.Marshal(growthOf(growthPet(11, nil, growthCurStats)))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"basis":"unknown","from_level":null,"rates":{"hp":null,"attack":null,"defense":null,"agility":null}}`
	if string(raw) != want {
		t.Errorf("编码为 %s，应为 %s", raw, want)
	}
}

// 五档：tier = points −（模板资质 + 2）。乌力的模板资质是 体力20 腕力12 耐久15 速度25。
func TestPetGrowthViewQualityTiers(t *testing.T) {
	tmpl, ok := gamedata.PetByID(1)
	if !ok {
		t.Fatal("乌力的模板应存在")
	}
	cases := []struct {
		name  string
		base  gamedata.Growth
		tiers [4]int
	}{
		{"满（模板 +2）为 0", gamedata.Growth{Vital: 22, Strength: 14, Toughness: 17, Dexterity: 27}, [4]int{0, 0, 0, 0}},
		{"+1 为 −1", gamedata.Growth{Vital: 21, Strength: 13, Toughness: 16, Dexterity: 26}, [4]int{-1, -1, -1, -1}},
		{"0 为 −2", gamedata.Growth{Vital: 20, Strength: 12, Toughness: 15, Dexterity: 25}, [4]int{-2, -2, -2, -2}},
		{"−1 为 −3", gamedata.Growth{Vital: 19, Strength: 11, Toughness: 14, Dexterity: 24}, [4]int{-3, -3, -3, -3}},
		{"−2 为 −4", gamedata.Growth{Vital: 18, Strength: 10, Toughness: 13, Dexterity: 23}, [4]int{-4, -4, -4, -4}},
		// 四项各自独立，且超出范围照实发、不截断。
		{"各项独立且越界", gamedata.Growth{Vital: 25, Strength: 13, Toughness: 5, Dexterity: 24}, [4]int{3, -1, -12, -3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &petRec{TemplateID: 1, Base: c.base}
			q := qualityOf(p, tmpl)
			wantStats := [4]string{"vital", "strength", "toughness", "dexterity"}
			wantPoints := [4]int{c.base.Vital, c.base.Strength, c.base.Toughness, c.base.Dexterity}
			if len(q) != 4 {
				t.Fatalf("应固定四项，实际 %d 项", len(q))
			}
			for i := range q {
				if q[i].Stat != wantStats[i] || q[i].Points != wantPoints[i] || q[i].Tier != c.tiers[i] {
					t.Errorf("第 %d 项 %+v，应为 stat=%s points=%d tier=%d",
						i, q[i], wantStats[i], wantPoints[i], c.tiers[i])
				}
			}
		})
	}
}
