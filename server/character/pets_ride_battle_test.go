package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// 骑着 1 号栏的宠物时，把 2 号栏的宠物设成出战或待命都能成功，骑着的那只状态不变（STAGE-149，docs/ride.md §4）。
func TestPlanPetStateWhileRidingAnotherPet(t *testing.T) {
	pets := []petRec{
		{Slot: 1, TemplateID: 194, Level: 1, HP: 40, State: petStateRide},
		{Slot: 2, TemplateID: 2, Level: 1, HP: 34, State: petStateRest},
	}
	changes, err := planPetState(pets, 2, petStateBattle)
	if err != nil || len(changes) != 1 || changes[2] != petStateBattle {
		t.Fatalf("骑乘时出战另一只：changes = %v，err = %v，want 只改 2 号栏为出战", changes, err)
	}
	changes, err = planPetState(pets, 2, petStateStandby)
	if err != nil || len(changes) != 1 || changes[2] != petStateStandby {
		t.Fatalf("骑乘时待命另一只：changes = %v，err = %v，want 只改 2 号栏为待命", changes, err)
	}
}

// 骑着的宠物自己不能出战或待命，要先下骑。
func TestPlanPetStateRidden(t *testing.T) {
	pets := []petRec{{Slot: 1, TemplateID: 194, Level: 1, HP: 40, State: petStateRide}}
	for _, state := range []int{petStateBattle, petStateStandby, petStateRest} {
		if _, err := planPetState(pets, 1, state); protoCode(err) != protocol.CodePetRiding {
			t.Fatalf("骑着的宠物改成 %d：err = %v，want PET_RIDING", state, err)
		}
	}
}
