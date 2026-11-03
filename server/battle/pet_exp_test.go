// STAGE-244：出战宠物的战斗经验按宠物自己的等级算等级差（battle.c 胜利结算对每个参战单位各算一遍），
// 不再复用人物那份。骑乘宠物的 0.6 倍见 ride_test.go。

package battle

import "testing"

// 人物 30 级、出战宠 1 级，打 10 级怪（基础经验 100）：人物等级差 20，缩到最低 1；
// 宠物等级差 −9（怪比宠高），拿全额 100。
func TestPetExpUsesOwnLevel(t *testing.T) {
	b := newFormulaBattle(func(int) int { return 0 })
	p := b.PlayerUnit()
	p.Level = 30
	pet := b.PetUnit()
	if pet == nil {
		t.Fatal("测试战斗应有出战宠物")
	}
	pet.Level = 1
	m := b.Unit(15)
	m.Level, m.EnemyExp = 10, 100
	b.slain = []*Unit{m}

	r := b.rewards()
	if len(r.Exp) != 2 || r.Exp[0].Who != "player" || r.Exp[1].Who != "pet" {
		t.Fatalf("经验条目 = %+v，应有人物和出战宠物两行", r.Exp)
	}
	if r.Exp[0].Gain != 1 {
		t.Errorf("30 级人物打 10 级怪 = %d 经验，应缩到最低 1", r.Exp[0].Gain)
	}
	if r.Exp[1].Gain != 100 {
		t.Errorf("1 级宠物打 10 级怪 = %d 经验，应全额 100", r.Exp[1].Gain)
	}
	if r.Exp[1].Gain <= r.Exp[0].Gain {
		t.Errorf("宠物经验 %d 应大于人物经验 %d", r.Exp[1].Gain, r.Exp[0].Gain)
	}
}

// 人物和出战宠物等级都在怪的 5 级以内（等级差都不超过 5）：两人都全额，和改动前一致。
func TestPetExpCloseLevelUnchanged(t *testing.T) {
	b := newFormulaBattle(func(int) int { return 0 })
	b.PlayerUnit().Level = 12
	b.PetUnit().Level = 11
	m := b.Unit(15)
	m.Level, m.EnemyExp = 10, 100
	b.slain = []*Unit{m}

	r := b.rewards()
	if len(r.Exp) != 2 || r.Exp[0].Gain != 100 || r.Exp[1].Gain != 100 {
		t.Fatalf("经验 = %+v，人物和宠物都应全额 100", r.Exp)
	}
}
