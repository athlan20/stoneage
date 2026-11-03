// 骑乘的引擎测试（STAGE-124）：数值融合、伤害分摊、经验、宠物不上格子、换宠被拒。
// 骑乘和出战是两只宠物（STAGE-149）：骑着的放在 SideInput.Rider，出战的放在 BattlePet。

package battle

import (
	"math"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// riderSide 是骑着宠物的一方：人物骑着 testSide 的宠物（0 号栏），没有出战的宠物。
func riderSide() *SideInput {
	side := testSide()
	side.Rider, side.BattlePet = side.BattlePet, nil
	return side
}

// riderAndBattleSide 是骑着一只、另一只出战的一方（STAGE-149）：0 号栏骑着，1 号栏的布比出战上格子。
func riderAndBattleSide() *SideInput {
	side := riderSide()
	side.BattlePet, side.Bench[1] = side.Bench[1], nil
	return side
}

// setRideStats 把人物和宠物的能力设成便于手算的值：人物 攻 100 防 50 速 40，宠物 攻 40 防 30 速 60。
func setRideStats(b *Battle) (u, pet *Unit) {
	u, pet = b.PlayerUnit(), b.RiderUnit()
	u.Attack, u.Defense, u.Agility = 100, 50, 40
	pet.Attack, pet.Defense, pet.Agility = 40, 30, 60
	return u, pet
}

func near(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

// TestRideNumbersFuse 骑乘时人与宠的数值按原版 BATTLE_adjustRidePet3A 合并（_BATTLE_NEWPOWER 分支）。
func TestRideNumbersFuse(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u, _ := setRideStats(b)
	if !u.ridden() {
		t.Fatal("骑着宠物时 ridden() 应为真")
	}
	if b.commandPet() != nil {
		t.Fatal("骑着的宠物不应该站在格子里")
	}

	if got := u.attackNow(); !near(got, 100*0.8+40*0.8) {
		t.Fatalf("近战攻击 = %v，want 人×0.8+宠×0.8 = %v", got, 100*0.8+40*0.8)
	}
	u.Weapon = weaponThrou
	if got := u.attackNow(); !near(got, 100+40*0.4) {
		t.Fatalf("投掷攻击 = %v，want 人+宠×0.4 = %v", got, 100+40*0.4)
	}
	u.Weapon = weaponClaw

	if got := u.defenseNow(); !near(got, (50+30)*0.7) {
		t.Fatalf("防御 = %v，want (人+宠)×0.7 = %v", got, (50+30)*0.7)
	}

	if got := u.agilityAct(); !near(got, 40*0.2+60*0.8) {
		t.Fatalf("出手敏捷 = %v，want 人×0.2+宠×0.8 = %v", got, 40*0.2+60*0.8)
	}
	if got := u.agilityHit(); !near(got, 40*0.1+60*0.9) {
		t.Fatalf("被打敏捷 = %v，want 人×0.1+宠×0.9 = %v", got, 40*0.1+60*0.9)
	}
	u.Weapon = weaponThrou
	if got := u.agilityAct(); !near(got, 40*0.8+60*0.2) {
		t.Fatalf("投掷出手敏捷 = %v，want 人×0.8+宠×0.2 = %v", got, 40*0.8+60*0.2)
	}
}

// TestRideNotInGrid 骑着的宠物不占格子：不下指令、不出手、不被选为目标，但仍是 RiderUnit（结算要用）。
func TestRideNotInGrid(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	if b.Unit(firstPetNo) != nil {
		t.Fatal("骑乘的宠物不应该在 5 号格")
	}
	if b.RiderUnit() == nil {
		t.Fatal("RiderUnit 应返回骑着的宠物，好让结算写回血量和经验")
	}
	for _, a := range b.Needs() {
		if a == "pet" {
			t.Fatal("骑乘时 needs 不应该有 pet")
		}
	}
	for _, u := range b.actingOrder() {
		if u.Kind == KindPet {
			t.Fatal("骑乘的宠物不应该出手")
		}
	}
}

// TestRideSplitDamage 骑乘挨打时血量按防御比例分摊：人分 dmg×宠防/(人防+宠防)+1，宠分 dmg−人那份+1。
func TestRideSplitDamage(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u, pet := setRideStats(b)
	hpU, hpP := u.HP, pet.HP

	var ev Event
	b.hurt(u, 30, "hit", &ev)

	// 人 30×30/(50+30)+1 = 12，宠 30−12+1 = 19
	if got := hpU - u.HP; got != 12 {
		t.Fatalf("人物扣血 = %d，want 12", got)
	}
	if got := hpP - pet.HP; got != 19 {
		t.Fatalf("宠物扣血 = %d，want 19", got)
	}
	if len(ev.Effects) != 1 || ev.Effects[0].Delta != -12 || ev.Effects[0].Target != u.No {
		t.Fatalf("战报里人物的效果 = %+v，want 扣 12", ev.Effects)
	}
}

// TestRideMountDownTakesNoSplit 宠物血量归零后仍骑着（docs/ride.md §5），之后的伤害全由人承担。
func TestRideMountDownTakesNoSplit(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u, pet := setRideStats(b)
	pet.HP = 5

	var ev Event
	b.hurt(u, 30, "hit", &ev)
	if !pet.Down || pet.HP != 0 {
		t.Fatalf("宠物血量 = %d（Down=%v），want 倒下", pet.HP, pet.Down)
	}
	if !u.ridden() {
		t.Fatal("宠物倒下后仍是骑着，ride 应为 true")
	}
	before := u.HP
	b.hurt(u, 30, "hit", &ev)
	if before-u.HP != 30 {
		t.Fatalf("宠物倒下后人物扣血 = %d，want 全部 30", before-u.HP)
	}
	if pet.HP != 0 {
		t.Fatalf("宠物倒下后不应再扣血，血量 = %d", pet.HP)
	}
}

// TestRideExpIsSixTenths 骑宠经验：按宠物自己的等级算，乘全服倍数，再乘 0.6。
func TestRideExpIsSixTenths(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	_, pet := setRideStats(b)
	pet.Level = 10
	b.slain = []*Unit{{Level: 10, EnemyExp: 100}}
	if got := b.rideExp(pet); got != 60 {
		t.Fatalf("骑宠经验 = %d，want 100×0.6 = 60", got)
	}
	b.Cbs.ExpRate = func() float64 { return 2 }
	if got := b.rideExp(pet); got != 120 {
		t.Fatalf("全服 2 倍时骑宠经验 = %d，want 120", got)
	}
}

// TestRideNoSwap 骑乘中不能换宠（收回、换成别的都不行）。
func TestRideNoSwap(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	for _, slot := range []int{-1, 1} {
		_, perr := b.Submit(b.Turn(), "player", "swap_pet", Cmd{PetSlot: slot})
		if perr == nil || perr.Code != protocol.CodeActionNotAllowed {
			t.Fatalf("骑乘中 swap_pet %d 应被拒绝，得到 %v", slot, perr)
		}
	}
}

// TestRideView 人物的协议内容带 ride；骑着的宠物不作为单位出现。
func TestRideView(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	u := b.PlayerUnit()
	if v := u.view(u.Side(), u.No); !v.Ride {
		t.Fatal("骑乘时人物的 ride 应为 true")
	}
	plain := newTestBattle(t, "win", rollRnd(9999))
	if v := plain.PlayerUnit().view(0, 0); v.Ride {
		t.Fatal("不骑乘时 ride 应为 false")
	}
}

// TestRideFullBattle 骑着宠物打一场：宠物不下指令，人和宠都被扣血，结算时宠物有经验。
func TestRideFullBattle(t *testing.T) {
	b := newBattleWithSide(t, riderSide(), "win", rollRnd(9999))
	pet := b.RiderUnit()
	maxPet := pet.MaxHP
	var end *EndView
	for turn := 1; turn <= 40 && end == nil; turn++ {
		for _, a := range b.Needs() {
			if a == "pet" {
				t.Fatalf("turn %d 骑乘时出现了宠物指令", turn)
			}
		}
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		if _, perr := b.Submit(b.Turn(), "player", "attack", Cmd{Target: target}); perr != nil {
			t.Fatalf("人物攻击提交失败: %v", perr)
		}
		round, e := b.Settle()
		for _, ev := range round.Events {
			for _, eff := range ev.Effects {
				if eff.Target == firstPetNo {
					t.Fatalf("宠物不应该作为目标出现在战报里: %+v", eff)
				}
			}
		}
		end = e
	}
	if end == nil || end.Result != "win" {
		t.Fatalf("没有打赢: %+v", end)
	}
	if pet.HP >= maxPet {
		t.Fatalf("宠物没有挨打：血量 %d / %d", pet.HP, maxPet)
	}
	if b.PlayerUnit().HP >= b.PlayerUnit().MaxHP {
		t.Fatal("人物没有挨打")
	}
	var petExp *RewardExp
	for i := range end.Rewards.Exp {
		if end.Rewards.Exp[i].Who == "pet" {
			petExp = &end.Rewards.Exp[i]
		}
	}
	if petExp == nil || petExp.Gain <= 0 {
		t.Fatalf("结算后宠物应有经验: %+v", end.Rewards.Exp)
	}
}
