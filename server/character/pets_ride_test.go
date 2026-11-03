package character

import (
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// 上骑的检查按 docs/protocol.md 5.47 的顺序：第一条不满足就回对应的错误码（STAGE-123）。
func TestPlanPetRideErrors(t *testing.T) {
	ok := rideTarget{CharLevel: 2, RideLevel: 40, LevelGap: 5, Body: 3, Color: 2, Loyalty: 100}
	tiger := petRec{Slot: 1, TemplateID: 194, Level: 1, State: petStateRest}
	cases := []struct {
		name string
		pets []petRec
		slot int
		t    rideTarget
		code string
	}{
		{"没有宠物", []petRec{tiger}, 3, ok, protocol.CodeNoPet},
		{"已经在骑别的", []petRec{tiger, {Slot: 2, TemplateID: 253, Level: 1, State: petStateRide}}, 1, ok, protocol.CodeAlreadyRiding},
		{"出战的不能骑", []petRec{{Slot: 1, TemplateID: 194, Level: 1, State: petStateBattle}}, 1, ok, protocol.CodePetNotRest},
		{"待命的不能骑", []petRec{{Slot: 1, TemplateID: 194, Level: 1, State: petStateStandby}}, 1, ok, protocol.CodePetNotRest},
		{"骑乘等级不够", []petRec{{Slot: 1, TemplateID: 194, Level: 50, State: petStateRest}}, 1, rideTarget{CharLevel: 60, RideLevel: 40, LevelGap: 5, Body: 3, Color: 2, Loyalty: 100}, protocol.CodeRideNotLearned},
		{"忠诚不到 100", tigerPets(tiger), 1, rideTarget{CharLevel: 2, RideLevel: 40, LevelGap: 5, Body: 3, Color: 2, Loyalty: 66}, protocol.CodeRideLoyaltyLow},
		{"人物等级加差距不够", []petRec{{Slot: 1, TemplateID: 194, Level: 8, State: petStateRest}}, 1, rideTarget{CharLevel: 2, RideLevel: 200, LevelGap: 5, Body: 3, Color: 2, Loyalty: 100}, protocol.CodeRideLevelGap},
		{"转生超过 2 次的不能骑", []petRec{{Slot: 1, TemplateID: 194, Level: 1, State: petStateRest, Transmigration: 3}}, 1, ok, protocol.CodeRideTransLimit},
		{"颜色不对的虎", tigerPets(petRec{Slot: 1, TemplateID: 193, Level: 1, State: petStateRest}), 1, ok, protocol.CodeRideNotMatch},
		{"普通宠物不在骑乘表里", tigerPets(petRec{Slot: 1, TemplateID: 2, Level: 1, State: petStateRest}), 1, ok, protocol.CodeRideNotMatch},
	}
	for _, c := range cases {
		_, err := planPetRide(c.pets, c.slot, c.t)
		perr, isProto := err.(*protocol.Error)
		if !isProto || perr.Code != c.code {
			t.Errorf("%s：err = %v，want %s", c.name, err, c.code)
		}
	}
}

// STAGE-196：转生次数 2 是上限，恰好 2 次的宠物能骑（docs/ride.md 第 9 条）。
func TestRideTransLimitBoundary(t *testing.T) {
	pets := []petRec{{Slot: 1, TemplateID: 194, Level: 1, State: petStateRest, Transmigration: MaxRideTransmigration}}
	ok := rideTarget{CharLevel: 2, RideLevel: 40, LevelGap: 5, Body: 3, Color: 2, Loyalty: 100}
	if _, err := planPetRide(pets, 1, ok); err != nil {
		t.Fatalf("转生 %d 次应能骑，实际 %v", MaxRideTransmigration, err)
	}
}

// STAGE-181：没学过骑乘（ride_level 为 0）时文案说“没学过”，不再写成“等级小于等于 0 级”。
func TestRideNotLearnedMessage(t *testing.T) {
	pets := []petRec{{Slot: 1, TemplateID: 194, Level: 1, State: petStateRest}}
	_, err := planPetRide(pets, 1, rideTarget{CharLevel: 2, RideLevel: 0, LevelGap: 5, Body: 3, Color: 2, Loyalty: 100})
	perr, ok := err.(*protocol.Error)
	if !ok || perr.Code != protocol.CodeRideNotLearned {
		t.Fatalf("没学过骑乘应回 RIDE_NOT_LEARNED，实际 %v", err)
	}
	if !strings.Contains(perr.Message, "没有学过骑乘") || strings.Contains(perr.Message, "小于等于") {
		t.Errorf("文案应说明没学过骑乘，实际 %q", perr.Message)
	}
}

// tigerPets 把 tiger 放进列表；测试里只有这一只宠物（栏位 1）。
func tigerPets(p petRec) []petRec { return []petRec{p} }

// 检查都通过时改成骑乘；已经在骑这只时成功但不改任何东西。
func TestPlanPetRideOK(t *testing.T) {
	pets := []petRec{{Slot: 1, TemplateID: 194, Level: 1, State: petStateRest}}
	ok := rideTarget{CharLevel: 2, RideLevel: 40, LevelGap: 5, Body: 3, Color: 2, Loyalty: 100}
	changes, err := planPetRide(pets, 1, ok)
	if err != nil || changes[1] != petStateRide || len(changes) != 1 {
		t.Fatalf("上骑 = %v, %v；want 栏位 1 变成骑乘", changes, err)
	}
	pets[0].State = petStateRide
	changes, err = planPetRide(pets, 1, ok)
	if err != nil || len(changes) != 0 {
		t.Fatalf("已经在骑这只 = %v, %v；want 成功且不改", changes, err)
	}
}

// 下骑：正在骑的回到休息；没有在骑时不改任何东西。
func TestPlanPetUnride(t *testing.T) {
	riding := []petRec{{Slot: 2, State: petStateRide}}
	if changes := planPetUnride(riding); changes[2] != petStateRest || len(changes) != 1 {
		t.Fatalf("下骑 = %v；want 栏位 2 回到休息", changes)
	}
	if changes := planPetUnride([]petRec{{Slot: 0, State: petStateBattle}}); len(changes) != 0 {
		t.Fatalf("没有在骑时下骑 = %v；want 不改", changes)
	}
}

// pet_state 只认休息、待命、出战；骑乘中的宠物改状态回 PET_RIDING（docs/protocol.md 5.22）。
func TestPetStateRejectsRideAndPool(t *testing.T) {
	for _, name := range []string{"ride", "pooled"} {
		n := name
		if _, ok := petStateByName(&n); ok {
			t.Errorf("pet_state 不应该接受 %s", name)
		}
	}
	battle := "battle"
	if code, ok := petStateByName(&battle); !ok || code != petStateBattle {
		t.Fatalf("battle = %d, %v", code, ok)
	}
	_, err := planPetState([]petRec{{Slot: 1, State: petStateRide}}, 1, petStateRest)
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.CodePetRiding {
		t.Fatalf("骑乘中改状态 err = %v，want PET_RIDING", err)
	}
}
