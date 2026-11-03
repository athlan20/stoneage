package character

import (
	"errors"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// protoCode 取出协议错误码；不是协议错误或为 nil 时返回空串。
func protoCode(err error) string {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return perr.Code
	}
	return ""
}

func pet(slot, state, hp int) petRec {
	return petRec{Slot: slot, TemplateID: 1, Name: "乌力", Level: 5, State: state, HP: hp}
}

func TestPlanPetStateBattleDemotesOldBattlePet(t *testing.T) {
	pets := []petRec{pet(0, petStateBattle, 10), pet(1, petStateRest, 10), pet(2, petStateStandby, 10)}
	changes, err := planPetState(pets, 1, petStateBattle)
	if err != nil {
		t.Fatalf("出战应当成功：%v", err)
	}
	if len(changes) != 2 || changes[1] != petStateBattle || changes[0] != petStateRest {
		t.Errorf("应把 1 号设为出战、0 号变成休息，实际 %v", changes)
	}
	if _, ok := changes[2]; ok {
		t.Errorf("待命的 2 号不应被改动")
	}
}

func TestPlanPetStateSameStateIsNoop(t *testing.T) {
	pets := []petRec{pet(0, petStateBattle, 10)}
	changes, err := planPetState(pets, 0, petStateBattle)
	if err != nil || len(changes) != 0 {
		t.Errorf("已经出战再设出战应当什么都不改，实际 %v, %v", changes, err)
	}
}

func TestPlanPetStateFaintedCannotBattle(t *testing.T) {
	pets := []petRec{pet(0, petStateRest, 0)}
	if _, err := planPetState(pets, 0, petStateBattle); protoCode(err) != protocol.CodePetFainted {
		t.Errorf("倒下的宠物出战应回复 PET_FAINTED，实际 %v", err)
	}
	// 倒下的宠物可以待命或休息。
	if _, err := planPetState(pets, 0, petStateStandby); err != nil {
		t.Errorf("倒下的宠物应当能设为待命：%v", err)
	}
}

func TestPlanPetStateStandbyLimit(t *testing.T) {
	pets := []petRec{
		pet(0, petStateStandby, 10), pet(1, petStateStandby, 10), pet(2, petStateStandby, 10),
		pet(3, petStateRest, 10),
	}
	if _, err := planPetState(pets, 3, petStateStandby); protoCode(err) != protocol.CodeStandbyFull {
		t.Errorf("待命满 %d 只时再设待命应回复 STANDBY_FULL，实际 %v", MaxStandbyPets, err)
	}
	// 已经待命的那只重复设待命，不算超限。
	if changes, err := planPetState(pets, 0, petStateStandby); err != nil || len(changes) != 0 {
		t.Errorf("待命满了时，对已经待命的宠物再设待命应当不改，实际 %v, %v", changes, err)
	}
	// 待命满了也能设休息，并能设出战（出战不占待命名额）。
	if _, err := planPetState(pets, 0, petStateRest); err != nil {
		t.Errorf("设休息不受待命上限影响：%v", err)
	}
	if _, err := planPetState(pets, 3, petStateBattle); err != nil {
		t.Errorf("设出战不受待命上限影响：%v", err)
	}
}

func TestPlanPetStateNoPet(t *testing.T) {
	pets := []petRec{pet(0, petStateRest, 10)}
	if _, err := planPetState(pets, 4, petStateBattle); protoCode(err) != protocol.CodeNoPet {
		t.Errorf("空栏位应回复 NO_PET，实际 %v", err)
	}
}

func TestNormalizePetName(t *testing.T) {
	cases := []struct {
		raw, want, code string
	}{
		{"  小白  ", "小白", ""},
		{"", "", ""},
		{"   ", "", ""},
		{strings.Repeat("字", 16), strings.Repeat("字", 16), ""},
		{strings.Repeat("字", 17), "", protocol.CodeInvalidName},
		{"小 白", "", protocol.CodeInvalidName},
		{"小\t白", "", protocol.CodeInvalidName},
		{"小\x01白", "", protocol.CodeInvalidName},
		{"傻逼", "", protocol.CodeInvalidName}, // STAGE-287：屏蔽词表
		{"傻.逼", "", protocol.CodeInvalidName},
	}
	for _, c := range cases {
		got, err := normalizePetName(c.raw)
		if protoCode(err) != c.code {
			t.Errorf("normalizePetName(%q) 错误码 = %q，应为 %q", c.raw, protoCode(err), c.code)
			continue
		}
		if got != c.want {
			t.Errorf("normalizePetName(%q) = %q，应为 %q", c.raw, got, c.want)
		}
	}
}

func TestPetNameRestoresSpecies(t *testing.T) {
	if got := petName("", "乌力"); got != "乌力" {
		t.Errorf("空名字应恢复种族名，实际 %q", got)
	}
	if got := petName("小白", "乌力"); got != "小白" {
		t.Errorf("给了名字就用名字，实际 %q", got)
	}
}

func TestPetSlotArg(t *testing.T) {
	for _, bad := range []int{-1, MaxPetSlots} {
		if _, err := petSlotArg(&bad); protoCode(err) != protocol.CodeBadRequest {
			t.Errorf("栏位 %d 应回复 BAD_REQUEST，实际 %v", bad, err)
		}
	}
	if _, err := petSlotArg(nil); protoCode(err) != protocol.CodeBadRequest {
		t.Errorf("缺少 slot 应回复 BAD_REQUEST，实际 %v", err)
	}
	ok := MaxPetSlots - 1
	if slot, err := petSlotArg(&ok); err != nil || slot != ok {
		t.Errorf("最大栏位 %d 应当合法，实际 %d, %v", ok, slot, err)
	}
}

func TestPetStateByName(t *testing.T) {
	for name, want := range map[string]int{"rest": petStateRest, "standby": petStateStandby, "battle": petStateBattle} {
		n := name
		if got, ok := petStateByName(&n); !ok || got != want {
			t.Errorf("petStateByName(%q) = %d, %v，应为 %d", name, got, ok, want)
		}
	}
	bad := "fight"
	if _, ok := petStateByName(&bad); ok {
		t.Errorf("不认识的状态应返回 false")
	}
	if _, ok := petStateByName(nil); ok {
		t.Errorf("缺少状态应返回 false")
	}
}
