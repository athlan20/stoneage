// 骑乘的回复分账（STAGE-257，对照 gmsv/battle/battle_magic.c 的 BATTLE_MultiRecovery，_MAGIC_REHPAI 分支）：
// 战斗内回复 HP 时，骑着宠物的人物把总回复量按缺口水位拆给人和骑宠，两边各自封顶。

package battle

import "testing"

// setHeal 把人物和骑宠的最大血量设成 200 / 100，血量设成给定值（hp 为 0 时骑宠倒下）。
func setHeal(u, pet *Unit, hp, petHP int) {
	u.MaxHP, u.HP = 200, hp
	pet.MaxHP, pet.HP = 100, petHP
	pet.Down = petHP == 0
}

// rideMagicSide 是骑着一只、另一只出战的一方，1 号栏放精灵 id。
func rideMagicSide(id int) *SideInput {
	side := riderAndBattleSide()
	side.Magics[1] = id
	return side
}

// TestRideHealSplitsByGap 按缺口水位拆分：总量 80，人物缺口 100、骑宠缺口 50 时人得 52（80×66/100，66 是
// 人物缺口占比取整）、宠得 28；人满血全给骑宠（封顶后不回灌）；宠满血全给人；宠倒下时救回。
func TestRideHealSplitsByGap(t *testing.T) {
	cases := []struct {
		name               string
		hp, petHP          int
		wantHuman, wantPet int
	}{
		{"两边都有缺口", 100, 50, 52, 28},
		{"人满血，回复全进骑宠并封顶", 200, 50, 0, 50},
		{"骑宠满血，回复全给人", 100, 100, 80, 0},
		{"骑宠倒下，救回并分到血", 100, 0, 40, 40},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
			u, pet := b.PlayerUnit(), b.RiderUnit()
			setHeal(u, pet, c.hp, c.petHP)

			var ev Event
			b.healUnit(u, 80, &ev)
			if got := u.HP - c.hp; got != c.wantHuman {
				t.Fatalf("人物得到 %d，want %d", got, c.wantHuman)
			}
			if got := pet.HP - c.petHP; got != c.wantPet {
				t.Fatalf("骑宠得到 %d，want %d", got, c.wantPet)
			}
			if pet.Down != (pet.HP == 0) {
				t.Fatalf("骑宠 HP=%d 但 Down=%v", pet.HP, pet.Down)
			}
			if len(ev.Effects) != 1 || ev.Effects[0].Target != u.No || ev.Effects[0].Delta != c.wantHuman {
				t.Fatalf("战报里应只有人物的一条 heal（骑宠不单发效果）: %+v", ev.Effects)
			}
		})
	}
}

// TestRideHealRevivedMountTakesSplitAgain 骑宠被回复救回后继续骑着，下一击又开始分摊伤害（不是一直由人硬扛）。
func TestRideHealRevivedMountTakesSplitAgain(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u, pet := setRideStats(b)
	setHeal(u, pet, 100, 0)

	var ev Event
	b.healUnit(u, 80, &ev)
	if pet.Down || pet.HP != 40 {
		t.Fatalf("骑宠救回后血量 = %d（Down=%v），want 40 且未倒下", pet.HP, pet.Down)
	}

	hpU, hpP := u.HP, pet.HP
	b.hurt(u, 30, "hit", &ev)
	// 与 TestRideSplitDamage 同式：人 30×30/(50+30)+1 = 12，宠 30−12+1 = 19
	if got := hpU - u.HP; got != 12 {
		t.Fatalf("救回后人物扣血 = %d，want 12", got)
	}
	if got := hpP - pet.HP; got != 19 {
		t.Fatalf("救回后骑宠扣血 = %d，want 19", got)
	}
}

// TestRideHealSpellSelf 治愈的精灵 Lv1（威力 80）对骑乘的自己施放：人与骑宠按缺口拆分（同上表第一行）。
func TestRideHealSpellSelf(t *testing.T) {
	b := newBattleWithSide(t, rideMagicSide(0), "win", rollRnd(9999))
	u, pet := b.PlayerUnit(), b.RiderUnit()
	setHeal(u, pet, 100, 50)
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 0}); perr != nil {
		t.Fatalf("治愈自己应被接受: %v", perr)
	}
	b.act(u)
	if got := u.HP - 100; got != 52 {
		t.Fatalf("人物得到 %d，want 52", got)
	}
	if got := pet.HP - 50; got != 28 {
		t.Fatalf("骑宠得到 %d，want 28", got)
	}
}

// TestRideHealAllCountsMountOnce 恩惠（全体回复）对每个人物只算一次分账：骑宠不在单位表里，不会被遍历第二遍；
// 出战的宠物单独回自己的份。
func TestRideHealAllCountsMountOnce(t *testing.T) {
	b := newBattleWithSide(t, rideMagicSide(20), "win", rollRnd(9999)) // 恩惠的精灵 Lv1：威力 50，全体
	u, pet := b.PlayerUnit(), b.RiderUnit()
	setHeal(u, pet, 100, 50)
	battlePet := b.PetUnit()
	battlePet.MaxHP, battlePet.HP = 150, 50

	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 0}); perr != nil {
		t.Fatalf("恩惠应被接受: %v", perr)
	}
	b.act(u)
	// 人物 50×66/100 = 33，骑宠 50−33 = 17（只算一次）；出战宠物 50 满额。
	if got := u.HP - 100; got != 33 {
		t.Fatalf("人物得到 %d，want 33", got)
	}
	if got := pet.HP - 50; got != 17 {
		t.Fatalf("骑宠得到 %d，want 17（算两遍会是 34）", got)
	}
	if got := battlePet.HP - 50; got != 50 {
		t.Fatalf("出战宠物得到 %d，want 50", got)
	}
}

// TestRideHealItemSplits 战斗内吃肉（普通回复道具）同样按分账拆给人和骑宠。
func TestRideHealItemSplits(t *testing.T) {
	side := riderSide()
	side.Items[5] = BattleItem{ID: 1235, Name: "普通的肉"}
	b := newBattleWithSide(t, side, "win", rollRnd(9999))
	u, pet := b.PlayerUnit(), b.RiderUnit()
	setHeal(u, pet, 100, 50)
	if _, perr := b.Submit(1, "player", "item", Cmd{Slot: 5}); perr != nil {
		t.Fatalf("吃肉被拒: %v", perr)
	}
	b.act(u)
	if u.HP <= 100 {
		t.Fatalf("人物应被回复，hp=%d", u.HP)
	}
	if pet.HP <= 50 {
		t.Fatalf("骑宠应分到回复，hp=%d", pet.HP)
	}
}

// TestHealNotRidingUnchanged 没骑乘时回复整体加给目标，骑宠（出战的宠物）只回自己的那一份，不受影响。
func TestHealNotRidingUnchanged(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	u := b.PlayerUnit()
	u.MaxHP, u.HP = 300, 100
	var ev Event
	b.healUnit(u, 80, &ev)
	if u.HP != 180 {
		t.Fatalf("没骑乘时人物应整体加 80，hp=%d", u.HP)
	}
}
