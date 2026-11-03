// 宠位已满时的捕捉接线（STAGE-267）：与 battle_capture 用例同样的回合流程，第 8 回合本来会抓到，
// 但随身栏位已有 20 只（STAGE-346），按原版判抓取失败，怪物留在场上，宠物列表不变。

package character

import (
	"context"
	"strconv"
	"testing"
)

func TestCaptureBattlePetSlotsFull(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	for slot := 0; slot < MaxPetSlots; slot++ {
		if _, err := GivePetWithLoyalty(context.Background(), e.chars.db, e.chars.opts.PetNotifyChannel,
			"勇者小明", 1, 5, 100, func(n int) int { return n / 2 }); err != nil {
			t.Fatal(err)
		}
	}
	c.ok("enter_world", `{"slot":0}`)

	if err := e.chars.StartBattle("勇者小明", "capture"); err != nil {
		t.Fatal(err)
	}
	var lastRound map[string]any
	failed := 0
	for turn := 1; turn <= 8; turn++ {
		c.drainTo(t, "battle_turn")
		q, _ := c.roundtrip(t, "battle", `{}`, "")
		needs := q["needs"].([]any)
		var round map[string]any
		for i, actor := range needs { // 顺序 player、pet
			body := `"actor":"` + actor.(string) + `","action":"guard"`
			if actor == "player" {
				action := "capture"
				if turn == 1 {
					action = "attack" // 先打残 15 号，后面每次捕捉才有正的成败概率
				}
				body = `"actor":"player","action":"` + action + `","target":15`
			}
			arg := `{"turn":` + strconv.Itoa(turn) + `,` + body + `}`
			if i == len(needs)-1 {
				_, round = c.roundtrip(t, "battle_command", arg, "battle_round")
			} else {
				c.roundtrip(t, "battle_command", arg, "")
			}
		}
		lastRound = round
		for _, ev := range round["events"].([]any) {
			ev := ev.(map[string]any)
			if ev["kind"] != "capture" {
				continue
			}
			if ev["ok"] == true {
				t.Fatalf("第 %d 回合宠位已满，捕捉不该成功: %v", turn, ev)
			}
			if _, has := ev["left"]; has {
				t.Errorf("第 %d 回合抓取失败，怪物不该离场: %v", turn, ev)
			}
			if _, has := ev["pet_slot"]; has {
				t.Errorf("第 %d 回合抓取失败，不该有 pet_slot: %v", turn, ev)
			}
			if turn == 8 {
				failed++
			}
		}
		c.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(turn)+`}`, "")
	}
	if failed == 0 {
		t.Fatal("第 8 回合应有一次捕捉判定（这场种子里本来会抓成功），实际没有 capture 事件")
	}
	var alive bool
	for _, u := range lastRound["state"].([]any) {
		u := u.(map[string]any)
		if int(u["no"].(float64)) == 15 && u["down"] == false {
			alive = true
		}
	}
	if !alive {
		t.Errorf("抓取失败后 15 号怪物应还在场上: %v", lastRound["state"])
	}

	// 清场结束战斗，确认没有多出宠物。
	for turn := 9; turn < 40; turn++ {
		c.drainTo(t, "battle_turn")
		q, _ := c.roundtrip(t, "battle", `{}`, "")
		monsters := aliveMonsters(q)
		if len(monsters) == 0 {
			t.Fatal("场上没有怪物，战斗却没有结束")
		}
		needs := q["needs"].([]any)
		var round map[string]any
		for i, actor := range needs {
			body := `"actor":"` + actor.(string) + `","action":"attack","target":` + strconv.Itoa(monsters[0])
			arg := `{"turn":` + strconv.Itoa(turn) + `,` + body + `}`
			if i == len(needs)-1 {
				_, round = c.roundtrip(t, "battle_command", arg, "battle_round")
			} else {
				c.roundtrip(t, "battle_command", arg, "")
			}
		}
		if round == nil {
			c.drainTo(t, "battle_round")
		}
		if end := c.drainToEnd(); end != nil {
			if end["result"] != "win" {
				t.Fatalf("战斗结果应为 win: %v", end)
			}
			if rewards, _ := end["rewards"].(map[string]any); rewards != nil {
				if pets, _ := rewards["pets"].([]any); len(pets) != 0 {
					t.Errorf("抓取失败不该有 rewards.pets: %v", pets)
				}
			}
			break
		}
		c.roundtrip(t, "battle_ack", `{"turn":`+strconv.Itoa(turn)+`}`, "")
	}

	c.roundtrip(t, "turn", `{"direction":0}`, "")
	pets, _ := c.roundtrip(t, "pets", `{}`, "")
	if list := pets["pets"].([]any); len(list) != MaxPetSlots {
		t.Errorf("宠位已满时抓取失败，宠物列表应仍是 %d 只，实际 %d: %v", MaxPetSlots, len(list), list)
	}
}
