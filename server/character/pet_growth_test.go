// 宠物成长（STAGE-33）：战斗经验升级按原版 CHAR_PetLevelUp 逐级累加随机成长、忠诚加成、
// 等级上限；捕捉入栏原样带走怪物的能力和剩余血量。

package character

import (
	"math/rand/v2"
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// newPet 用模板生成一只指定等级的宠物（同 givepet）。
func newPet(t *testing.T, templateID, level int, r *rand.Rand) petRec {
	t.Helper()
	tmpl, ok := gamedata.PetByID(templateID)
	if !ok {
		t.Fatalf("没有模板 %d", templateID)
	}
	base, stats := gamedata.NewPetGrowth(tmpl, level, r.IntN)
	return petRec{TemplateID: templateID, Name: tmpl.Name, Level: level, Stats: stats, Base: base,
		Rank: gamedata.PetRank(tmpl), HP: gamedata.PetMaxHP(stats)}
}

func TestGainPetExpLevelsUp(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	pet := newPet(t, 1, 1, r)
	before := pet
	// 1 级升 2 级要 2 点经验（升级即扣除，STAGE-57）；增量用同一个随机序列算出来对照。
	want := before.Stats.Add(gamedata.PetLevelUp(before.Base, before.Rank, rand.New(rand.NewPCG(9, 9)).IntN))
	if got := gainPetExp(&pet, 2, rand.New(rand.NewPCG(9, 9)).IntN); got != 2 {
		t.Fatalf("升到 %d 级，应为 2", got)
	}
	if pet.Stats != want {
		t.Errorf("能力 = %+v，应在原能力上累加一次成长 %+v", pet.Stats, want)
	}
	if pet.VariableAI != gamedata.PetLevelUpAI {
		t.Errorf("忠诚可变部分 = %d，升一级应 +%d", pet.VariableAI, gamedata.PetLevelUpAI)
	}
	if pet.HP != before.HP {
		t.Errorf("升级不回血：血量 %d → %d", before.HP, pet.HP)
	}
	if pet.Exp != 0 {
		t.Errorf("经验 = %d，升级扣除本级所需后应为 0", pet.Exp)
	}
}

// expTo 算从 1 级升到 level 级要给的按级经验总和（升级即扣除，STAGE-57 后经验不跨级累计）。
func expTo(level int) int64 {
	var sum int64
	for l := 1; l < level; l++ {
		sum += gamedata.NextExp(l)
	}
	return sum
}

// 修复前升级按刷怪公式整体重算，生成时随机的 10 点丢了，约一半的宠物升级后能力反而下降。
// 现在每一级都只往上加：连升多级、多只随机宠物，任何一项都不会比升级前低。
func TestGainPetExpNeverDecreases(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for i := 0; i < 300; i++ {
		pet := newPet(t, 1+i%30, 1, r) // 前 30 个模板轮流
		prev := pet.Stats
		gainPetExp(&pet, expTo(11), r.IntN) // 一次升到 11 级
		if pet.Level != 11 {
			t.Fatalf("模板 %d 升到 %d 级，应为 11", pet.TemplateID, pet.Level)
		}
		s := pet.Stats
		if s.Vital <= prev.Vital || s.Strength <= prev.Strength || s.Toughness <= prev.Toughness || s.Dexterity <= prev.Dexterity {
			t.Fatalf("模板 %d 升级后能力没有全部变大：%+v → %+v", pet.TemplateID, prev, s)
		}
		if pet.VariableAI != 10*gamedata.PetLevelUpAI {
			t.Fatalf("升 10 级忠诚可变部分 = %d，应为 %d", pet.VariableAI, 10*gamedata.PetLevelUpAI)
		}
	}
}

// 同种、同资质的宠物练到同一等级，能力也不一样（每级的成长系数和随机点不同）。
func TestGainPetExpSameSpeciesDiffers(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	a := newPet(t, 1, 1, r)
	b := a
	gainPetExp(&a, expTo(30), r.IntN)
	gainPetExp(&b, expTo(30), r.IntN)
	if a.Level != 30 || b.Level != 30 {
		t.Fatalf("应都升到 30 级：%d、%d", a.Level, b.Level)
	}
	if a.Stats == b.Stats {
		t.Errorf("同一只宠物的两份拷贝练到 30 级能力完全相同：%+v", a.Stats)
	}
}

// 模板填了等级上限（玛蕾菲雅 401 是 10 级）：到了上限不再升，经验清零。
func TestGainPetExpLimitLevel(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 1))
	pet := newPet(t, 401, 9, r)
	if got := gainPetExp(&pet, gamedata.NextExp(20), r.IntN); got != 10 {
		t.Fatalf("升到 %d 级，应停在上限 10", got)
	}
	if pet.Exp != 0 {
		t.Errorf("到上限后经验 = %d，应清零", pet.Exp)
	}
}

// 没填上限的宠物停在 140 级（setup.cf 的 LEVEL）。
func TestGainPetExpLevelCap(t *testing.T) {
	r := rand.New(rand.NewPCG(2, 2))
	pet := newPet(t, 1, gamedata.PetLevelCap, r)
	stats := pet.Stats
	if got := gainPetExp(&pet, gamedata.NextExp(150), r.IntN); got != gamedata.PetLevelCap {
		t.Fatalf("升到 %d 级，应停在 %d", got, gamedata.PetLevelCap)
	}
	if pet.Stats != stats {
		t.Errorf("没升级，能力却变了")
	}
}

// 捕捉：能力、资质、品质原样从怪物带走，血量是被抓时剩下的。
func TestPlaceCapturedPetKeepsMonster(t *testing.T) {
	tmpl, _ := gamedata.PetByID(1)
	m := battle.NewMonster(tmpl, 3, 15, rand.New(rand.NewPCG(4, 4)).IntN, nil)
	m.HP = 3
	r := &record{Pets: []petRec{{Slot: 0, TemplateID: 1}}}
	slot := placeCapturedPet(r, m)
	if slot != 1 {
		t.Fatalf("应放进第一个空栏位 1，现在 %d", slot)
	}
	got := r.Pets[1]
	if got.Stats != m.Ability || got.Base != m.Growth || got.Rank != m.Rank || got.Level != 3 {
		t.Errorf("宠物 %+v 和怪物不一致（能力 %+v、资质 %+v）", got, m.Ability, m.Growth)
	}
	if got.HP != 3 {
		t.Errorf("血量 = %d，应为被抓时剩下的 3", got.HP)
	}
}
