package battle

// 野外遇敌生成的测试（STAGE-40）：用真实敌群编号造区域验证抽签的确切行为，
// 再用真实数据（萨姆吉尔门外的区域 21）验证组成与可复现性。

import (
	"math/rand/v2"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

func wildSide() *SideInput {
	return &SideInput{Name: "勇者小明", Level: 1, HP: 44, MaxHP: 44, MP: 100, MaxMP: 100}
}

// scriptedRnd 按顺序返回给定的数，用完之后一直返回 0；测试里让每一步的抽取都可预知。
func scriptedRnd(seq ...int) func(n int) int {
	i := 0
	return func(n int) int {
		if n <= 0 {
			return 0
		}
		v := 0
		if i < len(seq) {
			v = seq[i]
			i++
		}
		if v >= n {
			v = n - 1
		}
		return v
	}
}

// synthArea 造一个引用真实敌群的区域：88 是 1 级昆依（敌人 119），89 是 2–3 级昆依（敌人 120），
// 靠等级区分抽中了哪个敌群。
func synthArea(ids ...gamedata.EncounterGroupRef) *gamedata.EncounterArea {
	var groups [10]gamedata.EncounterGroupRef
	copy(groups[:], ids)
	return &gamedata.EncounterArea{
		ID: 1, Floor: 100, X1: 1, Y1: 1, X2: 9, Y2: 9, ZOrder: 1, MaxEnemies: 3,
		Groups: groups,
	}
}

func TestBuildWildPicksGroupByWeight(t *testing.T) {
	area := synthArea(
		gamedata.EncounterGroupRef{ID: 88, Weight: 1},
		gamedata.EncounterGroupRef{ID: 89, Weight: 3},
	)
	// rnd=0 落进第一个敌群（权重累计 1）：1 级昆依。
	b, ok := BuildWild("b_1", "野外", wildSide(), &WildSpec{Area: area}, scriptedRnd(0))
	if !ok {
		t.Fatal("抽不出敌群")
	}
	mons := enemyUnits(b)
	if len(mons) != 1 || mons[0].Level != 1 || mons[0].TemplateID != 113 {
		t.Fatalf("敌群 88 应出一只 1 级昆依（模板 113），实际 %+v", mons)
	}
	// rnd=1 越过第一个敌群，落在第二个（权重累计 1+3）：2–3 级昆依，等级抽到下限 2。
	b, ok = BuildWild("b_2", "野外", wildSide(), &WildSpec{Area: area}, scriptedRnd(1))
	if !ok {
		t.Fatal("抽不出敌群")
	}
	mons = enemyUnits(b)
	if len(mons) != 1 || mons[0].Level != 2 {
		t.Fatalf("敌群 89 应出一只 2 级昆依，实际 %+v", mons)
	}
}

func TestBuildWildCountAndLevels(t *testing.T) {
	// 敌群 89（敌人 120：2–3 级）：数量 = 1+rnd(3)，等级 = 2+rnd(2)。
	// 序列：敌群 0、数量 2（共 3 只）、等级 1（都 3 级），资质随机固定给满。
	area := synthArea(gamedata.EncounterGroupRef{ID: 89, Weight: 1})
	area.MaxEnemies = 4
	b, ok := BuildWild("b_1", "野外", wildSide(), &WildSpec{Area: area},
		scriptedRnd(0, 2, 1, 4, 4, 4, 4, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3))
	if !ok {
		t.Fatal("抽不出敌人")
	}
	mons := enemyUnits(b)
	if len(mons) != 3 {
		t.Fatalf("应有 3 只敌人，实际 %d", len(mons))
	}
	for _, m := range mons {
		if m.TemplateID != 113 {
			t.Errorf("敌人应为昆依（模板 113），实际 %d", m.TemplateID)
		}
		if m.Level < 2 || m.Level > 3 {
			t.Errorf("敌人等级应在 2–3，实际 %d", m.Level)
		}
		if m.HP <= 0 || m.Attack <= 0 {
			t.Errorf("敌人属性没有生成：%+v", m)
		}
	}
	// 第一只的等级抽到 rnd=1（3 级）：每只敌人先耗一次敌群内抽取（权重 1，序列值被钳到 0），
	// 序列 0,2,1,4,4,4,4,3,… 里第 6 个值 4 钳到 1，正好是第一只的等级抽取。
	if mons[0].Level != 3 {
		t.Errorf("第一只敌人应是 3 级（等级抽到上限），实际 %d", mons[0].Level)
	}
}

func TestBuildWildSameEnemyCappedByCreateMax(t *testing.T) {
	// 敌人 120 的同场上限 10，区域最多 3 只：数量抽到 3 时三只都是它。
	area := synthArea(gamedata.EncounterGroupRef{ID: 89, Weight: 1})
	area.MaxEnemies = 3
	b, ok := BuildWild("b_1", "野外", wildSide(), &WildSpec{Area: area}, scriptedRnd(0, 2, 0, 0, 0, 0, 0, 0))
	if !ok {
		t.Fatal("抽不出敌人")
	}
	if n := len(enemyUnits(b)); n != 3 {
		t.Fatalf("应有 3 只敌人，实际 %d", n)
	}
}

func TestBuildWildEmptyAreaHasNoBattle(t *testing.T) {
	// 村子里的空敌群区域（encount.txt 地图 1000 的区域 59 就是这种行）。
	area := &gamedata.EncounterArea{ID: 59, Floor: 1000, X1: 1, Y1: 1, X2: 159, Y2: 159, ZOrder: 1, MaxEnemies: 5}
	if _, ok := BuildWild("b_1", "野外", wildSide(), &WildSpec{Area: area}, scriptedRnd(0)); ok {
		t.Fatal("空敌群的区域不应开战")
	}
}

func TestBuildWildSaimGateArea(t *testing.T) {
	// 真实数据：萨姆吉尔村门外的区域 21。敌群 88/91/89/92（权重 50/50/10/10，缺失的 1230 已剔除），
	// 敌人是模板 113（昆依）和 114（凯比特），等级 1–5，最多 2 只。
	area := gamedata.EncounterAreaAt(100, 638, 491)
	if area == nil || area.ID != 21 {
		t.Fatalf("萨姆吉尔门外应命中区域 21，实际 %+v", area)
	}
	rndOf := func(seed uint64) func(n int) int { return rand.New(rand.NewPCG(seed, seed*2+1)).IntN }
	b1, ok1 := BuildWild("b_1", "野外", wildSide(), &WildSpec{Area: area}, rndOf(7))
	b2, ok2 := BuildWild("b_2", "野外", wildSide(), &WildSpec{Area: area}, rndOf(7))
	if !ok1 || !ok2 {
		t.Fatal("区域 21 开不出战斗")
	}
	m1, m2 := enemyUnits(b1), enemyUnits(b2)
	if len(m1) < 1 || len(m1) > 2 {
		t.Fatalf("区域 21 一场应有 1–2 只敌人，实际 %d", len(m1))
	}
	for i := range m1 {
		if m1[i].TemplateID != m2[i].TemplateID || m1[i].Level != m2[i].Level {
			t.Errorf("同一种子的两场组成不同：%+v vs %+v", m1[i], m2[i])
		}
		if m1[i].TemplateID != 113 && m1[i].TemplateID != 114 {
			t.Errorf("区域 21 的敌人应是昆依/凯比特（模板 113/114），实际 %d", m1[i].TemplateID)
		}
	}
}

// Units 是测试辅助：Battle 需要一个只读的方式拿场上单位（New 之后怪物占对方宠物排）。
func enemyUnits(b *Battle) []*Unit {
	var out []*Unit
	for no := 0; no < 20; no++ {
		if u := b.Unit(no); u != nil && u.Kind == KindMonster {
			out = append(out, u)
		}
	}
	return out
}
