package battle

import "testing"

// STAGE-292：反击率按原版 BATTLE_CounterCalc 的修正。怪物反击宠物除以 10、不开根号，所以远低于宠物反击怪物。
func TestCounterPerMonsterOnPetIsSmaller(t *testing.T) {
	b := &Battle{}
	mon := &Unit{Kind: KindMonster, Agility: 40}
	pet := &Unit{Kind: KindPet, Agility: 40}
	weak := &Unit{Kind: KindPet, Agility: 4}
	monWeak := &Unit{Kind: KindMonster, Agility: 4}
	// 怪物 40 反击敏捷 4 的宠物：(40-4)/10 = 3，不开根号
	if got := b.counterPer(mon, weak); got != 3 {
		t.Fatalf("怪物反击宠物的反击率 = %v，应为 3", got)
	}
	// 宠物 40 反击敏捷 4 的怪物：被打方敏捷 ×0.8 → 3，(40-3)/0.08 开根号 ≈ 21.49
	if got := b.counterPer(pet, monWeak); got < 21 || got > 22 {
		t.Fatalf("宠物反击怪物的反击率 = %v，应约为 21.49", got)
	}
}

// STAGE-292：普通攻击结算后连环是否开始，依原版 BATTLE_Attack 的 iRet：打空（MISS）不改 iRet，继续；
// 会心、被防御挡下结束。
func TestMainContFollowsIRet(t *testing.T) {
	cases := map[string]bool{"hit": true, "dodge": true, "miss": true, "crit": false, "block": false}
	for result, want := range cases {
		if got := mainCont(result); got != want {
			t.Errorf("mainCont(%q) = %v，应为 %v", result, got, want)
		}
	}
}

// STAGE-292：已经出过手的单位，只有出手的是攻击（或攻击类技能、不防守）才能反击；
// 突击、地球一周的释放出手后不能（原版 COM1=NONE）。
func TestCanCounterAfterAct(t *testing.T) {
	b := &Battle{}
	u := &Unit{Kind: KindPet, actedRound: true}
	if b.canCounter(u) {
		t.Fatal("出过手但不是攻击的单位不应能反击")
	}
	u.comAtk = true
	if !b.canCounter(u) {
		t.Fatal("出手是攻击的单位应能反击")
	}
}

// STAGE-292：怪物反击宠物、同敏捷时反击率为 0：原版最低 1/10000（BATTLE_CounterCheckPet）；人物为 0 时不反击。
func TestCounterMinChanceForPetAndMonster(t *testing.T) {
	mon := &Unit{Kind: KindMonster, Agility: 40}
	pet := &Unit{Kind: KindPet, Agility: 40}
	b := &Battle{Rnd: func(int) int { return 0 }} // 骰 1
	if !b.counterCheck(mon, pet) {
		t.Fatal("怪物反击率为 0 时，骰 1 应能反击（最低 1/10000）")
	}
	b.Rnd = func(int) int { return 1 } // 骰 2
	if b.counterCheck(mon, pet) {
		t.Fatal("怪物反击率为 0 时，骰 2 不应反击")
	}
	player := &Unit{Kind: KindPlayer, Agility: 40}
	target := &Unit{Kind: KindPlayer, Agility: 40}
	b.Rnd = func(int) int { return 0 }
	if b.counterCheck(player, target) {
		t.Fatal("人物反击率为 0 时不应反击")
	}
}
