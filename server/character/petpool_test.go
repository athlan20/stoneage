package character

// 寄放在宠物店的宠物（STAGE-73，state=3、slot=50+池位）不是随身宠：开战不能用它（以前 sideInput 会
// 用 slot=50 当 Bench 下标直接 panic），捕捉不占随身栏位（以前 taken[50] 越界），战后也不改它的状态。

import (
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// TestPooledPetDoesNotBreakBattle：随身一只、寄放一只，开战不 panic，寄放的那只仍在池里。
func TestPooledPetDoesNotBreakBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Pets = append(r.Pets, petshopPet(0, petStateRest), petshopPet(gamedata.PooledSlotBase, petStatePooled))
	})

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.drainToEnd()
	withRec(t, e, "alice", func(r *record) {
		if len(r.Pets) != 2 || r.Pets[1].Slot != gamedata.PooledSlotBase || r.Pets[1].State != petStatePooled {
			t.Errorf("战后寄放的宠物应还在池里（slot=50、state=3），实际 %+v", r.Pets)
		}
	})
}

// TestPooledPetOnlyIsNotSentIntoBattle：只有寄放的宠物时不能拿它出战（sideInput 不把它当出战宠）。
func TestPooledPetOnlyIsNotSentIntoBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		r.Pets = append(r.Pets, petshopPet(gamedata.PooledSlotBase, petStatePooled))
		side := sideInput(r)
		if side.BattlePet != nil {
			t.Errorf("只有寄放的宠物时不该有出战宠: %+v", side.BattlePet)
		}
		for slot, in := range side.Bench {
			if in != nil {
				t.Errorf("寄放的宠物不该进替补席（栏位 %d）", slot)
			}
		}
	})
}

// TestPlaceCapturedPetSkipsPooled：寄放的宠物不占随身栏位，捕捉只看 0–19；随身满了返回 -1。
func TestPlaceCapturedPetSkipsPooled(t *testing.T) {
	r := &record{Pets: []petRec{petshopPet(gamedata.PooledSlotBase, petStatePooled)}}
	monster := &battle.Unit{Name: "怪物", TemplateID: 2, Level: 1, HP: 10}
	if got := placeCapturedPet(r, monster); got != 0 {
		t.Fatalf("寄放的宠物不占栏位，捕捉应落在 0 号，实际 %d", got)
	}
	for slot := 1; slot < MaxPetSlots; slot++ {
		r.Pets = append(r.Pets, petshopPet(slot, petStateRest))
	}
	if got := placeCapturedPet(r, monster); got != -1 {
		t.Errorf("随身 %d 只满了应返回 -1，实际 %d", MaxPetSlots, got)
	}
}
