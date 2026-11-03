// 宠物击杀数（STAGE-150）的落库测试：战斗胜利的结算把击杀数加到宠物记录上，写回数据库后重读仍在，
// 并且多场战斗累加。需要 STONEAGE_TEST_DATABASE_URL（测试 env 在数据库里跑）。

package character

import (
	"context"
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

func TestPetKillsSettledAndSaved(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	var charID int64
	withRec(t, e, "alice", func(r *record) {
		charID = r.ID
		r.Pets = []petRec{{Slot: 0, TemplateID: 1, Name: "乌力", Level: 1, HP: 10, State: petStateBattle,
			Skills: [gamedata.PetSkillSlots]int{-1, -1, -1, -1, -1, -1, -1}}}
		e.chars.savePets(charID, r.Pets)
	})

	// 第一场胜利：宠物击败 2 只怪，结算后记录里是 2，并写回数据库。
	settleWinKills(t, e, 0, 2)
	if got := dbPetKills(t, e, charID, 0); got != 2 {
		t.Fatalf("第一场后数据库里的击杀数 = %d，want 2", got)
	}

	// 第二场胜利：击败 1 只，累加到 3，不是覆盖成 1。
	settleWinKills(t, e, 0, 1)
	if got := dbPetKills(t, e, charID, 0); got != 3 {
		t.Fatalf("第二场后数据库里的击杀数 = %d，want 3", got)
	}
}

// settleWinKills 把一场胜利的击杀数结算到角色上（applyResult，同生产路径），再按生产路径写库。
func settleWinKills(t *testing.T, e *env, slot, kills int) {
	t.Helper()
	withRec(t, e, "alice", func(r *record) {
		end := &battle.EndView{Result: "win", Rewards: &battle.Rewards{
			PetKills: []battle.PetKill{{Slot: slot, Kills: kills}},
		}}
		e.chars.applyResult(r, &battle.Battle{}, end)
		e.chars.savePets(r.ID, r.Pets)
	})
}

// dbPetKills 直接从数据库读宠物的击杀数（不经过内存），确认写回的是库里的值。
func dbPetKills(t *testing.T, e *env, charID int64, slot int) int {
	t.Helper()
	pets, err := readPets(context.Background(), e.chars.db, charID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pets {
		if p.Slot == slot {
			return p.Kills
		}
	}
	t.Fatalf("库里没有栏位 %d 的宠物", slot)
	return 0
}
