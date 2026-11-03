// 0 号宠物栏位在 JSON 里不能省略（STAGE-268）：capture / swap_pet 的 pet_slot、
// battle_end.rewards.exp 里宠物的 slot，以及 rewards.pets 的 slot 为 0 时都要出现。

package battle

import (
	"encoding/json"
	"strings"
	"testing"
)

// jsonOf 把值序列化成 JSON 字符串（断言字段在不在用）。
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// firstEvent 取一回合里指定 kind 的第一条事件。
func eventOfKind(t *testing.T, round Round, kind string) *Event {
	t.Helper()
	for i := range round.Events {
		if round.Events[i].Kind == kind {
			return &round.Events[i]
		}
	}
	t.Fatalf("这一回合没有 %s 事件: %+v", kind, round.Events)
	return nil
}

// TestCaptureEventPetSlotZero：抓进 0 号栏位时 pet_slot 为 0，rewards 里宠物的 slot 也是 0。
func TestCaptureEventPetSlotZero(t *testing.T) {
	b := newTestBattle(t, "capture", chanceRnd()) // 捕捉必成功
	b.PlayerUnit().Attack = 5                     // 先打残，见 TestBattleCapture
	b.Cbs.PlaceCaptured = func(*Unit, *CaptureKeep) (int, *Discarded) { return 0, nil }
	for turn := 1; turn <= 2; turn++ {
		b.Submit(turn, "player", "attack", Cmd{Target: 15})
		b.Submit(turn, "pet", "guard", Cmd{})
		b.Settle()
	}
	b.Submit(3, "player", "capture", Cmd{Target: 15})
	b.Submit(3, "pet", "guard", Cmd{})
	round, _ := b.Settle()

	cap := eventOfKind(t, round, "capture")
	if cap.PetSlot == nil || *cap.PetSlot != 0 {
		t.Fatalf("抓进 0 号栏位，pet_slot 应为 0: %+v", cap)
	}
	if s := jsonOf(t, cap); !strings.Contains(s, `"pet_slot":0`) {
		t.Errorf("capture 事件 JSON 里 0 号栏位不能省略: %s", s)
	}

	r := b.rewards()
	var petExp *RewardExp
	for i := range r.Exp {
		if r.Exp[i].Who == "pet" {
			petExp = &r.Exp[i]
		}
	}
	if petExp == nil || petExp.Slot == nil || *petExp.Slot != 0 {
		t.Fatalf("rewards.exp 里宠物的 slot 应为 0: %+v", r.Exp)
	}
	if s := jsonOf(t, petExp); !strings.Contains(s, `"slot":0`) {
		t.Errorf("rewards.exp 里 0 号栏位不能省略: %s", s)
	}
	if len(r.Pets) != 1 || r.Pets[0].Slot != 0 {
		t.Fatalf("rewards.pets 应有一只 0 号栏位的宠物: %+v", r.Pets)
	}
	if s := jsonOf(t, r.Pets); !strings.Contains(s, `"slot":0`) {
		t.Errorf("rewards.pets 里 0 号栏位不能省略: %s", s)
	}
}

// TestSwapPetEventPetSlotZero：换到 0 号栏位时 pet_slot 为 0；收回时仍是 −1。
func TestSwapPetEventPetSlotZero(t *testing.T) {
	side := testSide()
	slot0, slot1 := side.BattlePet, side.Bench[1]
	slot1.Slot = 1 // 让 1 号栏出战，0 号留在备用席，才能换到 0
	side.BattlePet = slot1
	side.Bench[0] = slot0
	side.Bench[1] = nil
	b := newBattleWithSide(t, side, "win", rollRnd(9999))

	next, perr := b.Submit(1, "player", "swap_pet", Cmd{PetSlot: 0})
	if perr != nil || next != "wait" {
		t.Fatalf("换到 0 号栏位：next=%v err=%v", next, perr)
	}
	round, _ := b.Settle()
	swap := eventOfKind(t, round, "swap_pet")
	if swap.PetSlot == nil || *swap.PetSlot != 0 {
		t.Fatalf("换到 0 号栏位，pet_slot 应为 0: %+v", swap)
	}
	if s := jsonOf(t, swap); !strings.Contains(s, `"pet_slot":0`) {
		t.Errorf("swap_pet 事件 JSON 里 0 号栏位不能省略: %s", s)
	}

	// 收回（pet_slot = −1）照旧出现。
	next, perr = b.Submit(b.Turn(), "player", "swap_pet", Cmd{PetSlot: -1})
	if perr != nil || next != "wait" {
		t.Fatalf("收回宠物：next=%v err=%v", next, perr)
	}
	round, _ = b.Settle()
	recall := eventOfKind(t, round, "swap_pet")
	if recall.PetSlot == nil || *recall.PetSlot != -1 {
		t.Fatalf("收回时 pet_slot 应为 −1: %+v", recall)
	}
	if s := jsonOf(t, recall); !strings.Contains(s, `"pet_slot":-1`) {
		t.Errorf("收回时 pet_slot 不能省略: %s", s)
	}
}

// TestPetSlotOmittedWithoutSlot：失败、资质放生的捕捉没有栏位，pet_slot 必须省略。
func TestPetSlotOmittedWithoutSlot(t *testing.T) {
	b := newTestBattle(t, "capture", chanceRnd())
	b.PlayerUnit().Attack = 5
	b.Cbs.PlaceCaptured = func(*Unit, *CaptureKeep) (int, *Discarded) { return -1, nil } // 宠位满（STAGE-267）
	for turn := 1; turn <= 2; turn++ {
		b.Submit(turn, "player", "attack", Cmd{Target: 15})
		b.Submit(turn, "pet", "guard", Cmd{})
		b.Settle()
	}
	b.Submit(3, "player", "capture", Cmd{Target: 15})
	b.Submit(3, "pet", "guard", Cmd{})
	round, _ := b.Settle()
	cap := eventOfKind(t, round, "capture")
	if cap.PetSlot != nil {
		t.Fatalf("抓取失败时不该有 pet_slot: %+v", cap)
	}
	if s := jsonOf(t, cap); strings.Contains(s, `"pet_slot"`) {
		t.Errorf("没有栏位时 pet_slot 应省略: %s", s)
	}
}
