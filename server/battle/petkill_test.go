// 宠物击杀数（STAGE-150）的测试：出过手的宠物每击败一只怪 +1；没出手、被击败、中途离场的规则见 petkill.go。

package battle

import "testing"

// 单元测试：直接驱动记账函数，不依赖战斗流程。
func TestPetKillBookkeeping(t *testing.T) {
	b := &Battle{}
	player := &Unit{Kind: KindPlayer, HP: 10}
	acted := &Unit{Kind: KindPet, PetSlot: 1, HP: 10}
	idle := &Unit{Kind: KindPet, PetSlot: 2, HP: 10}
	b.markActed(player) // 人物不进宠物名单
	b.markActed(acted)
	b.markActed(acted) // 同一只只记一次
	if len(b.petsActed) != 1 {
		t.Fatalf("出过手的宠物 %d 只，want 1", len(b.petsActed))
	}
	b.creditKill()
	b.creditKill()
	if got := b.petKills(); len(got) != 1 || got[0].Slot != 1 || got[0].Kills != 2 {
		t.Fatalf("击杀数 = %+v，want 栏位 1 击杀 2", got)
	}
	if idle.Kills != 0 || idle.Acted {
		t.Errorf("没出过手的宠物被记了：%+v", idle)
	}
}

// 出过手、打完又被换下战场的宠物照样记：petsActed 不随离场删除。
func TestPetKillSurvivesSwap(t *testing.T) {
	b := &Battle{}
	pet := &Unit{Kind: KindPet, PetSlot: 3, No: 6, HP: 10}
	b.units[pet.No] = pet
	b.markActed(pet)
	b.creditKill()
	b.leave(pet)
	b.creditKill()
	if got := b.petKills(); len(got) != 1 || got[0].Kills != 2 {
		t.Fatalf("换下战场后击杀数 = %+v，want 2", got)
	}
}

// 战斗结束时已经倒下的宠物不结算（被击败的不加）。
func TestPetKillSkipsDownPet(t *testing.T) {
	b := &Battle{}
	pet := &Unit{Kind: KindPet, PetSlot: 0, HP: 10}
	b.markActed(pet)
	b.creditKill()
	pet.HP, pet.Down = 0, true // 同 hurt：倒下时血量为 0 并标记 Down
	if got := b.petKills(); len(got) != 0 {
		t.Fatalf("倒下的宠物也结算了：%+v", got)
	}
}

// 端到端：人物和出战宠都出手打赢。出过手并且活着的宠物，击杀数等于本场击败的怪物数；倒下的宠物不结算。
func TestPetKillsInWin(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(9999))
	end := fightToWin(t, b, submitAll)
	if len(b.petsActed) == 0 {
		t.Fatal("出战宠没有出手，测试没有覆盖到击杀数")
	}
	pet := b.petsActed[0]
	kills := end.Rewards.PetKills
	if pet.Alive() {
		if len(kills) != 1 || kills[0].Slot != pet.PetSlot || kills[0].Kills != len(b.slain) {
			t.Fatalf("宠物活着，击杀数 = %+v，want 栏位 %d 击杀 %d", kills, pet.PetSlot, len(b.slain))
		}
	} else if len(kills) != 0 {
		t.Fatalf("宠物已倒下却结算了击杀数：%+v", kills)
	}
}

// 端到端：宠物没出手（只交人物的指令），战后不结算击杀数。
func TestPetKillsNeedAction(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(9999))
	end := fightToWin(t, b, func(b *Battle, target int) {
		if b.PlayerUnit().Alive() {
			b.Submit(b.Turn(), "player", "attack", Cmd{Target: target})
		}
	})
	if len(b.slain) == 0 {
		t.Fatal("没有击败任何怪物，测试没有意义")
	}
	if len(b.petsActed) != 0 || len(end.Rewards.PetKills) != 0 {
		t.Fatalf("宠物没出手却有击杀数：acted %d，结算 %+v", len(b.petsActed), end.Rewards.PetKills)
	}
}

// fightToWin 按 submit 交指令打到战斗结束，要求结果是胜利。
func fightToWin(t *testing.T, b *Battle, submit func(b *Battle, target int)) *EndView {
	t.Helper()
	for turn := 1; turn <= 40; turn++ {
		if b.Over() {
			break
		}
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		submit(b, target)
		if _, end := b.Settle(); end != nil {
			if end.Result != "win" {
				t.Fatalf("结果 = %s, want win", end.Result)
			}
			return end
		}
	}
	t.Fatal("40 回合内没有打赢")
	return nil
}
