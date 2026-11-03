// 真人同场战斗的结算（STAGE-275，2.6）：经验只给击杀者、掉落只落到出手且活着的人、捕获归捕获者、
// 离场与胜负各自的 battle_end。
package battle

import "testing"

// deadMonster 造一只已经被击倒的 1 级怪（带掉落 drops）。
func deadMonster(t *testing.T, b *Battle, drops ...int) *Unit {
	t.Helper()
	sc, _ := Lookup("win")
	ms, err := sc.makeMonsters(sc.NewRand())
	if err != nil {
		t.Fatal(err)
	}
	m := ms[0]
	m.Level = 3
	m.Drops = drops
	m.Down, m.HP = true, 0
	return m
}

func expOf(v *Rewards, who string) int64 {
	var sum int64
	for _, e := range v.Exp {
		if e.Who == who {
			sum += e.Gain
		}
	}
	return sum
}

func TestPartyExpOnlyKiller(t *testing.T) {
	b := partyFixture(t, 2)
	m := deadMonster(t, b)
	b.slain = []*Unit{m}
	b.killers[m] = []int{1} // 队员一人打倒
	if got := expOf(b.partyRewards(0), "player"); got != 0 {
		t.Fatalf("没出手的队长拿到经验 %d", got)
	}
	if got := expOf(b.partyRewards(1), "player"); got <= 0 {
		t.Fatalf("击杀者的经验 = %d", got)
	}
}

func TestPartyExpComboMembersEachGetFull(t *testing.T) {
	b := partyFixture(t, 2)
	m := deadMonster(t, b)
	b.slain = []*Unit{m}
	b.killers[m] = []int{0, 1}
	single := applyExpRate(levelDiffExp(b.units[0].Level, m.Level, killExp(m)), b.expRate())
	if got := expOf(b.partyRewards(0), "player"); got != single {
		t.Fatalf("组合技成员应各得一份完整经验：got %d want %d", got, single)
	}
	if got := expOf(b.partyRewards(1), "player"); got != single {
		t.Fatalf("队员的组合经验 = %d，应为 %d", got, single)
	}
}

func TestPartyPetExpGoesToOwnerPet(t *testing.T) {
	b := partyFixture(t, 2)
	m := deadMonster(t, b)
	b.slain = []*Unit{m}
	b.killers[m] = []int{6} // 队员的宠物出手
	b.petsActed = []*Unit{b.units[6]}
	r := b.partyRewards(1)
	if expOf(r, "player") != 0 {
		t.Fatal("宠物出手不给人物经验")
	}
	if expOf(r, "pet") <= 0 {
		t.Fatal("队员的宠物应拿到经验")
	}
	if expOf(b.partyRewards(0), "pet") != 0 {
		t.Fatal("队长不应拿到队员宠物的经验")
	}
}

func TestPartyDropsOnlyToLiveKiller(t *testing.T) {
	b := partyFixture(t, 2)
	m := deadMonster(t, b, 100)
	b.slain = []*Unit{m}
	b.killers[m] = []int{1}
	b.assignDrops()
	if len(b.partyDrops[1]) != 1 || len(b.partyDrops[0]) != 0 {
		t.Fatalf("掉落应只落到出手的队员：%v", b.partyDrops)
	}
	if got := b.partyRewards(1).Items; len(got) != 1 || got[0].ItemID != 100 {
		t.Fatalf("队员的奖励物品 = %+v", got)
	}
}

func TestPartyDropsSkipDeadKiller(t *testing.T) {
	b := partyFixture(t, 2)
	m := deadMonster(t, b, 100)
	b.slain = []*Unit{m}
	b.killers[m] = []int{0}
	b.units[0].Down, b.units[0].HP = true, 0 // 出手的队长已经倒下
	b.assignDrops()
	if len(b.partyDrops) != 0 {
		t.Fatalf("死亡的出手者拿不到掉落：%v", b.partyDrops)
	}
	// 组合技里一死一活：掉落只给活着的那一个。
	b.units[0].Down = false
	b.units[0].HP = 10
	b.units[1].Down, b.units[1].HP = true, 0
	b.killers[m] = []int{0, 1}
	b.assignDrops()
	if len(b.partyDrops[0]) != 1 || len(b.partyDrops[1]) != 0 {
		t.Fatalf("掉落应落到活着的队长：%v", b.partyDrops)
	}
}

func TestPartyCaptureGoesToCaptor(t *testing.T) {
	b := partyFixture(t, 2)
	m := deadMonster(t, b)
	m.PetSlot = 2
	b.captur = []*Unit{m}
	b.captors[m] = 1
	if got := b.partyRewards(1).Pets; len(got) != 1 || got[0].Slot != 2 {
		t.Fatalf("捕获的宠物应归捕获者：%+v", got)
	}
	if got := b.partyRewards(0).Pets; len(got) != 0 {
		t.Fatal("队长不应拿到队员捕获的宠物")
	}
}

func TestPartyEndViewsPerMember(t *testing.T) {
	b := partyFixture(t, 3)
	b.depart(1, "escape")
	b.depart(2, "timeout")
	if v := b.EndFor(1); v == nil || v.Result != "escaped" {
		t.Fatalf("逃跑的队员 = %+v", v)
	}
	if v := b.EndFor(2); v == nil || v.Result != "forced_out" || v.Rewards != nil || v.Penalty != nil {
		t.Fatalf("超时踢出的队员应没有奖励和惩罚：%+v", v)
	}
	if b.EndFor(0) != nil {
		t.Fatal("战斗还没结束，队长不应有 battle_end")
	}
	b.depart(0, "offline")
	if b.EndFor(0) != nil {
		t.Fatal("掉线的人不推送 battle_end")
	}
	if !b.Over() {
		t.Fatal("场上没有真人了，战斗应结束")
	}
}

func TestPartyEndWinAndLose(t *testing.T) {
	b := partyFixture(t, 2)
	b.over, b.result = true, "win"
	b.units[1].Down, b.units[1].HP = true, 0
	if v := b.EndFor(0); v == nil || v.Result != "win" || v.Rewards == nil {
		t.Fatalf("胜利的队长 = %+v", v)
	}
	if v := b.EndFor(1); v == nil || v.Rewards == nil || len(v.Rewards.Exp) != 0 || v.Rewards.Gold != 0 {
		t.Fatalf("死亡的队员没有奖励：%+v", v)
	}
	b.result = "lose"
	if v := b.EndFor(0); v == nil || v.Penalty == nil || v.Rewards != nil {
		t.Fatalf("失败的队长应有惩罚：%+v", v)
	}
}
