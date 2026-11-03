// 复活精灵（STAGE-77，对照 gmsv/battle/battle_magic.c 的 MAGIC_Ressurect_Battle 与 BATTLE_MultiRessurect）
// 的引擎测试：倒下的己方单位可以选为目标，回复量 RAND(0.9 威力, 1.1 威力) 封顶为最大血量，
// 上一场倒下的宠物进场时也是倒下的。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// 复活精灵 Lv1（50）：威力 100，回复 RAND(90,110)；rollRnd(10) 让 Rnd(21) 出 10，回复 100。
// 倒下的己方宠物可以选为目标，复活后活着、血量正确。
func TestReviveBringsBackPet(t *testing.T) {
	b := newBattleWithSide(t, magicSide(50), "win", rollRnd(10))
	you, pet := b.PlayerUnit(), b.PetUnit()
	pet.HP, pet.Down = 0, true
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5}); perr != nil {
		t.Fatalf("复活倒下的宠物应被接受: %v", perr)
	}
	evs := b.act(you)
	eff := effectOn(evs[0], 5)
	if eff == nil || eff.Result != "heal" || eff.Delta != 100 || eff.Down || eff.HP == nil || *eff.HP != 100 {
		t.Fatalf("应有复活回 100 血的效果: %+v", evs[0].Effects)
	}
	if pet.Down || !pet.Alive() || pet.HP != 100 {
		t.Fatalf("宠物应活着、血量 100: %+v", pet)
	}
	if you.MP != 100-8 {
		t.Fatalf("复活 Lv1 的气力应扣 8: %d", you.MP)
	}
}

func TestReviveRejectsLivingAndMonsters(t *testing.T) {
	b := newBattleWithSide(t, magicSide(50), "win", rollRnd(9999))
	sturdy(b, 15).Down = true
	cases := []struct {
		name   string
		target int
	}{
		{"活着的宠物", 5},
		{"活着的人物", 0},
		{"倒下的怪物", 15},
	}
	for _, tc := range cases {
		_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: tc.target})
		expectCode(t, perr, protocol.CodeInvalidTarget)
		if perr == nil {
			t.Fatalf("%s 不能复活", tc.name)
		}
	}
}

// 威力为 0 时回满（原版 MAGIC_Ressurect_Battle 的 pow <= 0 分支按最大血量算）。
func TestReviveWithZeroPowerRestoresFullHP(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	pet := b.PetUnit()
	pet.HP, pet.Down = 0, true
	var ev Event
	b.reviveUnit(pet, 0, &ev)
	if pet.HP != pet.MaxHP || pet.Down {
		t.Fatalf("威力 0 应回满: hp=%d max=%d down=%v", pet.HP, pet.MaxHP, pet.Down)
	}
}

// 上一场倒下、血量为 0 的宠物进场时是倒下的：本回合不要它的指令，但复活精灵可以把它救起来。
func TestFaintedPetEntersBattleDown(t *testing.T) {
	side := magicSide(50)
	side.BattlePet.HP = 0
	b := newBattleWithSide(t, side, "win", rollRnd(9999))
	if pet := b.PetUnit(); pet == nil || !pet.Down {
		t.Fatalf("血量为 0 的宠物应以倒下进场: %+v", pet)
	}
	for _, need := range b.Needs() {
		if need == "pet" {
			t.Fatalf("倒下的宠物本回合不该要指令: %v", b.Needs())
		}
	}
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5}); perr != nil {
		t.Fatalf("复活进场就倒下的宠物应被接受: %v", perr)
	}
}
