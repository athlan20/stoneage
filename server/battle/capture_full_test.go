// 宠位已满时的捕捉（STAGE-267）：按原版 BATTLE_Capture，PET_createPetFromCharaIndex 返回 −1
// 时 flg = 0，这次捕捉失败、怪物留在场上。

package battle

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCapturePetSlotsFull：判定成功但没有空栏位时不抓走怪物，下一回合有空位才抓成功。
func TestCapturePetSlotsFull(t *testing.T) {
	b := newTestBattle(t, "capture", chanceRnd()) // 捕捉必成功
	b.PlayerUnit().Attack = 5                     // 先打残，见 TestBattleCapture
	full := true
	b.Cbs.PlaceCaptured = func(*Unit, *CaptureKeep) (int, *Discarded) {
		if full {
			return -1, nil // 宠位满：不是被资质筛掉（discarded 为 nil）
		}
		return 1, nil
	}
	for turn := 1; turn <= 2; turn++ {
		b.Submit(turn, "player", "attack", Cmd{Target: 15})
		b.Submit(turn, "pet", "guard", Cmd{})
		b.Settle()
	}

	b.Submit(3, "player", "capture", Cmd{Target: 15})
	b.Submit(3, "pet", "guard", Cmd{})
	round, end := b.Settle()
	if end != nil || b.Over() {
		t.Fatalf("捕捉失败后战斗应继续: %+v", end)
	}
	var cap *Event
	for i, e := range round.Events {
		if e.Kind == "capture" {
			cap = &round.Events[i]
		}
	}
	if cap == nil || cap.OK == nil || *cap.OK {
		t.Fatalf("宠位满时应判捕捉失败（ok 为 false）: %+v", cap)
	}
	if len(cap.Left) != 0 || cap.PetSlot != nil {
		t.Errorf("抓取失败时怪物不该离场、也不该有栏位: %+v", cap)
	}
	if cap.Discarded != nil {
		t.Errorf("宠位满不是资质丢弃，不该带 discarded: %+v", cap.Discarded)
	}
	if u := b.Unit(15); u == nil || !u.Alive() {
		t.Errorf("抓取失败后怪物应还在场上: %+v", u)
	}
	if len(b.Captured()) != 0 {
		t.Errorf("抓取失败的不进捕获名单，实际 %d 只", len(b.Captured()))
	}
	raw, _ := json.Marshal(cap)
	if s := string(raw); strings.Contains(s, `"left"`) || strings.Contains(s, `"pet_slot"`) ||
		strings.Contains(s, `"discarded"`) || !strings.Contains(s, `"ok":false`) {
		t.Errorf("失败事件应只有 target、ok 为 false: %s", s)
	}

	// 腾出栏位后，同一只怪物下一回合能正常抓到。
	full = false
	b.Submit(b.Turn(), "player", "capture", Cmd{Target: 15})
	b.Submit(b.Turn(), "pet", "guard", Cmd{})
	round, _ = b.Settle()
	cap = nil
	for i, e := range round.Events {
		if e.Kind == "capture" {
			cap = &round.Events[i]
		}
	}
	if cap == nil || cap.OK == nil || !*cap.OK {
		t.Fatalf("腾出栏位后应抓成功: %+v", cap)
	}
	if len(cap.Left) != 1 || cap.Left[0] != 15 || cap.PetSlot == nil || *cap.PetSlot != 1 {
		t.Errorf("抓成功时怪物应离场、进 1 号栏位: %+v", cap)
	}
	if len(b.Captured()) != 1 || b.Captured()[0].No != 15 {
		t.Errorf("捕获名单应有 15 号: %+v", b.Captured())
	}
}
