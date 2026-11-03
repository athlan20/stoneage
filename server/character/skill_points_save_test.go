package character

import "testing"

// STAGE-67 升级奖励的未分配点数要随离开世界落库：战斗升级只改内存里的 r.SkillPoints（battle_apply.go），
// 离开世界时 save() 必须写回，否则下线重进丢失、战后加点时数据库里是 0。
func TestSkillPointsFromRewardSurviveLeave(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	// 同战斗结算的升级奖励：只改内存，不经过任何写库路径。
	if err := e.chars.LiveOf(e.session("alice")).With(func(r *record) { r.SkillPoints += 3 }); err != nil {
		t.Fatal(err)
	}
	if m := c.call("leave_world", `{}`); m["ok"] != true {
		t.Fatalf("离开世界应成功: %v", m)
	}
	if _, _, _, _, p := e.queryPoints("勇者小明"); p != 3 {
		t.Fatalf("离开世界后数据库里的未分配点数应是 3，实际 %d", p)
	}
	st := c.ok("enter_world", `{"slot":0}`)["status"].(map[string]any)
	if intOf(st, "skill_points") != 3 {
		t.Errorf("重新进入世界后应看到未分配点数 3，实际 %v", st["skill_points"])
	}
}
