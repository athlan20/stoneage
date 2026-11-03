// 偷窃、捐献、旅程伙伴的测试（STAGE-291 批 A）：对照原版 BATTLE_Steal、BATTLE_StealMoney、BATTLE_Abduct。
package battle

import "testing"

// rollPct 让 RAND(1,100) 掷出 v（roll = v），其余骰子取 0。
func rollPct(v int) func(n int) int {
	return func(n int) int {
		if n == 100 {
			return v - 1
		}
		return 0
	}
}

// 偷窃打怪：没有伤害，宠物不离场（怪物的成功率为 0）。
func TestStealNoDamageAndStaysOnField(t *testing.T) {
	b := skillTestBattle(t, 140)
	pet := b.PetUnit()
	ev, ok := petEvent(playTurn(t, b, 15))
	if !ok {
		t.Fatal("偷窃没有宠物事件")
	}
	if damageTo(ev, 15) != 0 || len(ev.Left) != 0 {
		t.Errorf("偷窃打怪不应伤害或离场：%+v", ev)
	}
	if b.units[pet.No] == nil {
		t.Error("偷窃失败后宠物不应离场")
	}
}

// 旅程伙伴的成功率：max((怪物 Lv − 宠物 Lv) × 0.6 + 30, 50)，截断取整。
func TestAbductPerFormula(t *testing.T) {
	cases := []struct{ enemy, pet, want int }{{30, 10, 50}, {50, 10, 54}, {60, 10, 60}}
	for _, c := range cases {
		if got := abductPer(c.enemy, c.pet); got != c.want {
			t.Errorf("abductPer(%d,%d) = %d，want %d", c.enemy, c.pet, got, c.want)
		}
	}
}

// 旅程伙伴成功：怪物和宠物都离场。
func TestAbductSuccessRemovesMonsterAndPet(t *testing.T) {
	b := skillTestBattle(t, 130)
	b.Rnd = rollPct(1)
	pet := b.PetUnit()
	ev, ok := petEvent(playTurn(t, b, 15))
	if !ok {
		t.Fatal("旅程伙伴没有宠物事件")
	}
	if b.units[15] != nil || b.units[pet.No] != nil {
		t.Errorf("成功后怪物和宠物都应离场，离场名单 %v", ev.Left)
	}
	if damageTo(ev, 15) != 0 {
		t.Errorf("旅程伙伴不造成伤害，实际 %d", damageTo(ev, 15))
	}
}

// 旅程伙伴失败：怪物留场，宠物仍然离场（原版不论成败宠物都 BATTLE_Exit）。
func TestAbductFailStillPetLeaves(t *testing.T) {
	b := skillTestBattle(t, 130)
	b.Rnd = rollPct(100)
	pet := b.PetUnit()
	playTurn(t, b, 15)
	if b.units[15] == nil {
		t.Error("失败后怪物应留场")
	}
	if b.units[pet.No] != nil {
		t.Error("失败后宠物也应离场")
	}
}

// 捐献成功：石币记入战斗（RAND(10,100) 取 10），宠物离场。
func TestStealMoneySuccessTakesGoldAndLeaves(t *testing.T) {
	b := skillTestBattle(t, 211)
	b.Rnd = rollPct(1)
	pet := b.PetUnit()
	playTurn(t, b, 15)
	if b.stolenGold != 10 {
		t.Errorf("捐献的石币 %d，want 10", b.stolenGold)
	}
	if b.units[pet.No] != nil {
		t.Error("捐献成功后宠物应离场")
	}
}

// 捐献失败：不伤害，宠物留场，石币为 0。
func TestStealMoneyFailKeepsPet(t *testing.T) {
	b := skillTestBattle(t, 211)
	b.Rnd = rollPct(100)
	pet := b.PetUnit()
	ev, _ := petEvent(playTurn(t, b, 15))
	if b.stolenGold != 0 || b.units[pet.No] == nil {
		t.Errorf("捐献失败时宠物应留场、石币为 0，实际石币 %d", b.stolenGold)
	}
	if damageTo(ev, 15) != 0 {
		t.Errorf("捐献不伤害，实际 %d", damageTo(ev, 15))
	}
}

// 捐献的石币不论结果都入账：非胜利结算也带上（没偷到时不带，battle_end 与原来一样）。
func TestStolenGoldKeptOnNonWin(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	if b.nonWinRewards() != nil {
		t.Fatal("没偷到石币时非胜利结算不应带奖励")
	}
	b.stolenGold = 50
	if r := b.nonWinRewards(); r == nil || r.Gold != 50 {
		t.Fatalf("非胜利结算应带偷到的 50 石币，实际 %+v", r)
	}
}
