package character

import "testing"

// 骑着的宠物是 Rider，不是出战的宠物：不进替补、不占 BattlePet；另一只出战的宠物照常上格子（STAGE-149，docs/ride.md §5）。
func TestSideInputRiderAndBattlePet(t *testing.T) {
	r := &record{Pets: []petRec{
		{Slot: 0, TemplateID: 2, Name: "乌力乌力", Level: 1, HP: 34, State: petStateBattle},
		{Slot: 1, TemplateID: 194, Name: "金格萨贝鲁", Level: 1, HP: 40, State: petStateRide},
	}}
	side := sideInput(r)
	if side.Rider == nil || side.Rider.Slot != 1 {
		t.Fatalf("Rider = %+v，want 骑着的 1 号栏", side.Rider)
	}
	if side.BattlePet == nil || side.BattlePet.Slot != 0 {
		t.Fatalf("BattlePet = %+v，want 出战的 0 号栏", side.BattlePet)
	}
	if side.Bench[0] != nil || side.Bench[1] != nil {
		t.Fatal("出战的和骑着的宠物都不应该进替补：出战的在格子里，骑着的不能换进场")
	}
}

// 骑着宠物但没有出战的宠物：不自动顶上其他宠物，BattlePet 为空（出战要玩家自己设）。
func TestSideInputRiderNoAutoBattlePet(t *testing.T) {
	r := &record{Pets: []petRec{
		{Slot: 0, TemplateID: 2, Name: "乌力乌力", Level: 1, HP: 34, State: petStateRest},
		{Slot: 1, TemplateID: 194, Name: "金格萨贝鲁", Level: 1, HP: 40, State: petStateRide},
	}}
	side := sideInput(r)
	if side.Rider == nil || side.Rider.Slot != 1 {
		t.Fatalf("Rider = %+v，want 骑着的 1 号栏", side.Rider)
	}
	if side.BattlePet != nil {
		t.Fatalf("BattlePet = %+v，want 空（骑乘时不替玩家决定出战）", side.BattlePet)
	}
	if side.Bench[0] == nil || side.Bench[0].Slot != 0 {
		t.Fatal("休息的宠物应进替补，好让玩家换宠")
	}
	if r.Pets[0].State != petStateRest {
		t.Fatalf("休息的宠物状态被改成了 %d", r.Pets[0].State)
	}
}

// 没有骑乘时，出战的宠物照常是 BattlePet，没有 Rider。
func TestSideInputWithoutRide(t *testing.T) {
	r := &record{Pets: []petRec{{Slot: 2, TemplateID: 252, Name: "布林帖斯", Level: 1, HP: 50, State: petStateBattle}}}
	side := sideInput(r)
	if side.BattlePet == nil || side.BattlePet.Slot != 2 || side.Rider != nil {
		t.Fatalf("BattlePet = %+v，Rider = %+v，want 2 号栏出战且没有骑乘", side.BattlePet, side.Rider)
	}
}
