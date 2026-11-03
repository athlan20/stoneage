// 真人同场战斗的结算按人进行（STAGE-275 第二轮，缺口 2）：宠物忠诚和装备耐久各自按真人结算；
// 掉线与超时踢出的不结算，逃跑离场的结算。
package battle

import "testing"

// loyaltyPair 开一场队长加一名队员的战斗，两人的出战宠物忠诚都设成 1000（避开下限，便于看增减）。
func loyaltyPair(t *testing.T) (*Battle, *Unit, *Unit) {
	t.Helper()
	b := partyWith(t, testSide(), rollRnd(9999), PartyMate{No: 1, Side: testSide()})
	lead, mate := b.units[5], b.units[6]
	lead.loyVar, mate.loyVar = 1000, 1000
	return b, lead, mate
}

// TestPartyLoyaltyWinSettlesEachHuman 胜利时队长和队员的宠物各自按全场击杀记账，两份一样多。
func TestPartyLoyaltyWinSettlesEachHuman(t *testing.T) {
	b, lead, mate := loyaltyPair(t)
	b.result = "win"
	b.slain = []*Unit{b.Unit(monsterNo(t, b))}
	b.settleLoyalty()
	if lead.loyVar <= 1000 || mate.loyVar <= 1000 {
		t.Fatalf("胜利应当给两只宠物加忠诚：队长 %d，队员 %d", lead.loyVar, mate.loyVar)
	}
	if lead.loyVar != mate.loyVar {
		t.Errorf("两只宠物的增量应一样：队长 %d，队员 %d", lead.loyVar, mate.loyVar)
	}
}

// TestPartyLoyaltyOfflineNotSettled 掉线的队员不结算：他的宠物忠诚不变，在场的队长照常结算。
func TestPartyLoyaltyOfflineNotSettled(t *testing.T) {
	b, lead, mate := loyaltyPair(t)
	if _, ok := b.Offline(1); !ok {
		t.Fatal("掉线应当成功")
	}
	b.result = "win"
	b.slain = []*Unit{b.Unit(monsterNo(t, b))}
	b.settleLoyalty()
	if mate.loyVar != 1000 {
		t.Errorf("掉线的队员宠物不应结算，忠诚变成了 %d", mate.loyVar)
	}
	if lead.loyVar <= 1000 {
		t.Errorf("在场的队长宠物应当结算，忠诚 %d", lead.loyVar)
	}
}

// TestPartyLoyaltyTimeoutNotSettled 超时被踢出的队员不结算（forced_out 无奖励无惩罚）。
func TestPartyLoyaltyTimeoutNotSettled(t *testing.T) {
	b, _, mate := loyaltyPair(t)
	b.depart(1, "timeout")
	b.result = "win"
	b.slain = []*Unit{b.Unit(monsterNo(t, b))}
	b.settleLoyalty()
	if mate.loyVar != 1000 {
		t.Errorf("超时踢出的队员宠物不应结算，忠诚变成了 %d", mate.loyVar)
	}
}

// TestPartyLoyaltyEscapeSettled 逃跑离场的队员按正常参战结算，他的宠物也一起结算（取离场时的宠物）。
func TestPartyLoyaltyEscapeSettled(t *testing.T) {
	b, _, mate := loyaltyPair(t)
	b.depart(1, "escape")
	b.result = "win"
	b.slain = []*Unit{b.Unit(monsterNo(t, b))}
	b.settleLoyalty()
	if mate.loyVar <= 1000 {
		t.Errorf("逃跑离场的队员宠物应当结算，忠诚 %d", mate.loyVar)
	}
}

// TestPartyLoyaltyFlownOnlyThatHuman 失败时只有被打飞的队员，他的宠物减忠诚，队长的宠物不动。
func TestPartyLoyaltyFlownOnlyThatHuman(t *testing.T) {
	b, lead, mate := loyaltyPair(t)
	b.Unit(1).Flown = true
	b.result = "lose"
	b.settleLoyalty()
	if mate.loyVar >= 1000 {
		t.Errorf("被打飞的队员的宠物应减忠诚，实际 %d", mate.loyVar)
	}
	if lead.loyVar != 1000 {
		t.Errorf("没被打飞的队长宠物不应变，实际 %d", lead.loyVar)
	}
}

// TestPartyMateDownLosesOwnPetLoyalty 队员倒下时扣的是他自己宠物的忠诚，不是队长的。
func TestPartyMateDownLosesOwnPetLoyalty(t *testing.T) {
	b, lead, mate := loyaltyPair(t)
	b.onDown(b.Unit(1))
	if mate.loyVar >= 1000 {
		t.Errorf("队员倒下，他的宠物应减忠诚，实际 %d", mate.loyVar)
	}
	if lead.loyVar != 1000 {
		t.Errorf("队长的宠物不应因队员倒下而变，实际 %d", lead.loyVar)
	}
}

// TestPartyMateArmorCrushes 队员被打时，他自己的防具耐久下降，队长的装备不动；EquipStateFor 按人给出。
func TestPartyMateArmorCrushes(t *testing.T) {
	mate := testSide()
	mate.Equips[crushSlotBody] = &EquipInput{Slot: crushSlotBody, Name: "铁甲", Durability: 200, MaxDurability: 200}
	leader := testSide()
	leader.Equips[crushSlotBody] = &EquipInput{Slot: crushSlotBody, Name: "铁甲", Durability: 200, MaxDurability: 200}
	b := partyWith(t, leader, rollRnd(9999), PartyMate{No: 1, Side: mate})
	var ev Event
	b.physHurt(b.Unit(monsterNo(t, b)), b.Unit(1), 40, "hit", &ev)
	if d := 200 - b.EquipStateFor(1)[crushSlotBody].Durability; d < 8 || d > 12 {
		t.Errorf("队员的防具扣了 %d，要在 [8,12]", d)
	}
	if d := b.EquipStateFor(0)[crushSlotBody].Durability; d != 200 {
		t.Errorf("队长的防具不应扣，实际 %d", d)
	}
}
