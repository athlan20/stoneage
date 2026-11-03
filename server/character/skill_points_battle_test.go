package character

import (
	"context"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-151 的验收：战斗升级奖励的未分配点数要立即落库，升级后不下线就能加点，加点失败也不能清零奖励。
// 这里跑真实战斗（不直接改内存），经验先差一点到下一级，任何一场胜利都会升级。
func TestSkillPointsFromBattleSpendableWithoutLeaving(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	entered := c.ok("enter_world", `{"slot":0}`)
	dex0 := intOf(statsOf(entered["status"].(map[string]any)), "dexterity")
	withRec(t, e, "alice", func(r *record) { r.Exp = gamedata.NextExp(r.Level) })

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.playWildToEnd(t)

	if _, _, _, _, p := e.queryPoints("勇者小明"); p != 3 {
		t.Fatalf("战斗升级后数据库里的未分配点数应是 3（不下线也要落库），实际 %d", p)
	}
	resp, pushes := c.callp("addpoint", `{"stat":"dexterity","points":1}`)
	if resp["ok"] != true {
		t.Fatalf("升级后应能用掉奖励点数，实际 %v", resp)
	}
	st := statusOf(t, pushes[len(pushes)-1])
	if intOf(st, "skill_points") != 2 || intOf(statsOf(st), "dexterity") != dex0+1 {
		t.Errorf("加 1 点后应剩 2 点，实际 %v", st)
	}
}

// 加点失败（库里的点数被别处改少了）：内存要同步成库里的值，并推 status_changed，不能只报错。
func TestAddPointFailureSyncsStatusToClient(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) { r.SkillPoints = 3 })
	if _, err := e.chars.db.Exec(context.Background(),
		`UPDATE characters SET skill_points = 0 WHERE name = $1`, "勇者小明"); err != nil {
		t.Fatal(err)
	}

	resp, pushes := c.callp("addpoint", `{"stat":"vital","points":1}`)
	if e, _ := resp["error"].(map[string]any); e == nil || e["code"] != "NOT_ENOUGH_POINTS" {
		t.Fatalf("库里只有 0 点，应报 NOT_ENOUGH_POINTS：%v", resp)
	}
	wantPushes(t, pushes, "status_changed")
	if st := statusOf(t, pushes[0]); intOf(st, "skill_points") != 0 {
		t.Errorf("失败后应推送权威的 0 点，实际 %v", st["skill_points"])
	}
}

// STAGE-189：战斗中也能加点（连场遇敌时两场之间只有约 2 秒空档，原来加点全被 IN_BATTLE 挡回）。
// 加点立即扣点、加能力并推送；推送里的血量是战斗单位的实时值；打完这一场后加的能力还在，没被战斗结算覆盖。
func TestAddPointAllowedInBattle(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	e.givePoints("勇者小明", 3)
	c.recv() // 管理命令发的点数

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	live := e.chars.LiveOf(e.session("alice"))
	if err := live.Operate(func(r *record) { live.p.battle.b.PlayerUnit().HP = 10 }); err != nil {
		t.Fatal(err)
	}

	resp, pushes := c.callp("addpoint", `{"stat":"vital","points":2}`)
	if resp["ok"] != true {
		t.Fatalf("战斗中加点应成功：%v", resp)
	}
	var st map[string]any
	for _, m := range pushes {
		if m["type"] == "status_changed" {
			st = statusOf(t, m)
		}
	}
	if st == nil {
		t.Fatalf("战斗中加点应推送 status_changed，实际推送 %v", pushes)
	}
	if intOf(st, "skill_points") != 1 || intOf(statsOf(st), "vital") != 10 || intOf(st, "max_hp") != 52 {
		t.Errorf("体力 +2 后应剩 1 点、体力 10、最大血量 52，实际 %v", st)
	}
	if intOf(st, "hp") != 10 {
		t.Errorf("战斗中推送的血量应是战斗单位的实时值 10，实际 %v", st["hp"])
	}

	end := c.playWildToEnd(t)
	gained := 0 // 这一场胜利升级奖励的点数：和战斗中没花完的点数累加，不是覆盖
	if rw, _ := end["rewards"].(map[string]any); rw != nil {
		exps, _ := rw["exp"].([]any)
		for _, x := range exps {
			if m, _ := x.(map[string]any); m != nil && m["who"] == "player" {
				gained += intOf(m, "skill_points")
			}
		}
	}
	if v, _, _, _, p := e.queryPoints("勇者小明"); v != 10 || p != 1+gained {
		t.Errorf("打完这一场后体力应保留 10、点数应是 1+%d，实际体力 %d、点数 %d", gained, v, p)
	}
}
