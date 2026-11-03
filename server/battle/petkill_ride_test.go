// 骑着的宠物击杀数（STAGE-150）：人物出手时，骑着的宠物算出过手，击败的怪物照记。

package battle

import "testing"

// 骑着宠物的人物出手：骑着的宠物记为出过手，人物本身不进名单。
func TestRidePetCountsWhenRiderActs(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u := b.PlayerUnit()
	if !u.ridden() {
		t.Fatal("骑着宠物时 ridden() 应为真")
	}
	b.markActed(u)
	if len(b.petsActed) != 1 || b.petsActed[0] != u.mount {
		t.Fatalf("出过手的单位 = %+v，want 骑着的宠物", b.petsActed)
	}
	b.creditKill()
	if got := b.petKills(); len(got) != 1 || got[0].Slot != u.mount.PetSlot || got[0].Kills != 1 {
		t.Fatalf("击杀数 = %+v，want 骑着的宠物击杀 1", got)
	}
}

// 端到端：骑着宠物打赢（只交人物的指令），骑着的宠物按击败数记入击杀数。
func TestRidePetKillsInWin(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	end := fightToWin(t, b, func(b *Battle, target int) {
		if b.PlayerUnit().Alive() {
			b.Submit(b.Turn(), "player", "attack", Cmd{Target: target})
		}
	})
	if len(b.slain) == 0 {
		t.Fatal("没有击败任何怪物，测试没有意义")
	}
	mount := b.RiderUnit()
	kills := end.Rewards.PetKills
	if mount.Alive() {
		if len(kills) != 1 || kills[0].Slot != mount.PetSlot || kills[0].Kills != len(b.slain) {
			t.Fatalf("骑宠活着，击杀数 = %+v，want 栏位 %d 击杀 %d", kills, mount.PetSlot, len(b.slain))
		}
	} else if len(kills) != 0 {
		t.Fatalf("骑宠已倒下却结算了击杀数：%+v", kills)
	}
}
