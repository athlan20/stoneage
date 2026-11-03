// 野外战斗里单位倒下扣宠物忠诚（STAGE-146）的测试，对照原版 BATTLE_NormalDeadExtra 与 AI_FIX_PLAYERDEAD / AI_FIX_PETDEAD。

package battle

import "testing"

// 人物在野外战斗倒下：出战的宠物减 1 点（AI_FIX_PLAYERDEAD），主人 10 级以下减半；宠物倒下：它自己减 5 点（AI_FIX_PETDEAD）。
func TestDownLowersLoyalty(t *testing.T) {
	cases := []struct {
		name      string
		ownerLv   int
		downPet   bool
		wantLoyal int
	}{
		{"人物 11 级倒下", 11, false, loyalPlayerDead},
		{"人物 10 级倒下减半", 10, false, loyalPlayerDead / 2},
		{"宠物 11 级倒下", 11, true, loyalPetDead},
		{"宠物 10 级倒下减半", 10, true, loyalPetDead / 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newTestBattle(t, "win", rollRnd(0))
			b.PlayerUnit().Level = c.ownerLv
			var ev Event
			if c.downPet {
				pet := b.PetUnit()
				b.hurt(pet, pet.HP, "hit", &ev)
				if pet.loyVar != c.wantLoyal {
					t.Errorf("宠物倒下后可变部分 = %d，want %d", pet.loyVar, c.wantLoyal)
				}
			} else {
				b.hurt(b.PlayerUnit(), b.PlayerUnit().HP, "hit", &ev)
				if pet := b.PetUnit(); pet.loyVar != c.wantLoyal {
					t.Errorf("人物倒下后出战宠可变部分 = %d，want %d", pet.loyVar, c.wantLoyal)
				}
			}
			if !b.PetsChanged() {
				t.Errorf("忠诚变了，要推 pets_changed")
			}
		})
	}
}

// 已经倒下的单位再被打不算“新倒下”，不重复扣；被复活后再倒下，再扣一次。
func TestDownCountsEachNewFall(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	pet := b.PetUnit()
	var ev Event
	b.hurt(pet, pet.HP, "hit", &ev)
	b.hurt(pet, 10, "hit", &ev) // 已经倒下，再挨打不扣
	if want := loyalPetDead / 2; pet.loyVar != want {
		t.Fatalf("倒下一次后可变部分 = %d，want %d", pet.loyVar, want)
	}
	b.reviveUnit(pet, 50, &ev) // 复活精灵救起：+3 点
	b.hurt(pet, pet.HP, "hit", &ev)
	if want := loyalPetDead/2*2 + loyalRevived; pet.loyVar != want {
		t.Errorf("复活后再倒下，可变部分 = %d，want %d", pet.loyVar, want)
	}
}

// 人物被打飞倒下：不在倒下时扣（走 settleLoyalty 的打飞结算，避免重复扣）。
func TestFlownPlayerDownNotDoubleCounted(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(0))
	player := b.PlayerUnit()
	player.Level = 11
	var ev Event
	b.hurt(player, player.MaxHP*2, "hit", &ev) // 一下打飞
	if !player.Flown || player.Alive() {
		t.Fatalf("应当被打飞并倒下：Flown=%v Alive=%v", player.Flown, player.Alive())
	}
	if pet := b.PetUnit(); pet.loyVar != 0 {
		t.Errorf("被打飞倒下时倒下扣减应跳过，可变部分 = %d", pet.loyVar)
	}
}

// 战斗开始时血量为 0 的宠物是“已经倒下”的，进场不扣忠诚（不是这场里新倒下的）。
func TestPetDownAtStartNotCounted(t *testing.T) {
	side := testSide()
	side.BattlePet.HP = 0
	b := newBattleWithSide(t, side, "win", rollRnd(0))
	if pet := b.PetUnit(); pet != nil && pet.loyVar != 0 {
		t.Errorf("进场就倒下的宠物可变部分 = %d，不应变化", pet.loyVar)
	}
}
