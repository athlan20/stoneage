package gamedata

// 宠物技能店（STAGE-143）：参数表的读取、上架规则（跳过非法和查不到的技能、保持原版顺序）、
// 学习价格（PETSKILL_COST × skill_rate，向下取整）；petskill.tsv 的价格、说明和非法列。

import "testing"

func TestPetskillTableColumns(t *testing.T) {
	attack, ok := PetSkillByID(1)
	if !ok || attack.Name != "攻击" || attack.Cost != 1000 || attack.Comment != "通常攻击" || attack.Illegal {
		t.Errorf("攻击的价格、说明应取自 petskill.txt: %+v", attack)
	}
	// 行首 T 的技能是原版的非法技能，不上架。
	fourHit, ok := PetSkillByID(12)
	if !ok || !fourHit.Illegal || fourHit.Cost != 15000 {
		t.Errorf("T四段攻击应非法、价格 15000: %+v", fourHit)
	}
}

func TestNpcPetskillShopListing(t *testing.T) {
	shop := NpcPetskillShopOf(1763)
	if shop == nil {
		t.Fatal("1763 号应是宠物技能店")
	}
	if shop.SkillRate != 1.0 || len(shop.Skills) != 9 {
		t.Errorf("1763 号：倍率 1.0、9 个技能（含 0 号待机），实际 %v %v", shop.SkillRate, shop.Skills)
	}
	// 0 号（待机）是合法技能，原版照常上架：列表第一项就是它，价格 500。
	real := shop.ListedSkills()
	if len(real) != 9 || real[0].ID != 0 || real[0].Name != "待机" || shop.LearnCost(real[0]) != 500 {
		t.Errorf("1763 号第一个上架的应是待机（500 金币）: %+v", real)
	}
	if NpcPetskillShopOf(1) != nil {
		t.Error("1 号 NPC 不是宠物技能店")
	}

	// 非法（12、13）和查不到的（9999）跳过，其余按原版顺序上架。
	fake := &NpcPetskillShop{SkillRate: 1.5, Skills: []int{12, 9999, 3, 1, 13}}
	listed := fake.ListedSkills()
	if len(listed) != 2 || listed[0].ID != 3 || listed[1].ID != 1 {
		t.Errorf("上架应是 3、1（跳过非法和查不到的）: %v", listed)
	}
}

func TestNpcPetskillShopLearnCost(t *testing.T) {
	sk1 := PetSkill{ID: 1, Cost: 1000}
	cases := []struct {
		rate float64
		cost int
		want int
	}{
		{1.0, 1000, 1000},
		{1.2, 1000, 1200},
		{1.5, 2500, 3750},
		{1.2, 1001, 1201}, // 1001×1.2=1201.2，向下取整
		{0.3, 1000, 300},  // 小数倍率
		{1.0, 0, 0},       // 价格为 0 的技能（如偷窃）
	}
	for _, c := range cases {
		shop := &NpcPetskillShop{SkillRate: c.rate}
		sk := sk1
		sk.Cost = c.cost
		if got := shop.LearnCost(sk); got != c.want {
			t.Errorf("倍率 %v 价格 %d：应得 %d，实际 %d", c.rate, c.cost, c.want, got)
		}
	}
}
