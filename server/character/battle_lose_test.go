package character

// STAGE-194：战斗失败分两种。被打死（没被打飞）原地以 1 血复活，位置不动，penalty.in_place 为真；
// 被打飞才送回记录点或出生地。遇敌开关：被打飞传走才关（原因 lose），原地复活保持开着。

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// loseBattle 造一场能用来结算失败的战斗；flown 为真时人物单位标成被打飞。
func loseBattle(t *testing.T, flown bool) *battle.Battle {
	t.Helper()
	b, err := battle.Build("b_lose", &battle.SideInput{Name: "勇者小明", Level: 1, HP: 30, MaxHP: 30}, "win")
	if err != nil {
		t.Fatal(err)
	}
	if u := b.PlayerUnit(); u != nil {
		u.Flown = flown
	}
	return b
}

// TestDefeatNotFlownStaysInPlace 被打死：位置、朝向不变，血量剩 1，penalty 指向当前位置并标 in_place。
func TestDefeatNotFlownStaysInPlace(t *testing.T) {
	e := newEnv(t)
	r := &record{Name: "勇者小明", Level: 1, HP: 30, Charm: 60, Floor: 3000, X: 10, Y: 12, Direction: 2, Hometown: 0, Savepoint: 419}
	end := &battle.EndView{Result: "lose", Penalty: &battle.Penalty{}}
	before := r.Charm
	e.chars.applyResult(r, loseBattle(t, false), end)

	if r.Floor != 3000 || r.X != 10 || r.Y != 12 || r.Direction != 2 {
		t.Fatalf("被打死不该传送，位置应不变：%d (%d,%d) 朝向 %d", r.Floor, r.X, r.Y, r.Direction)
	}
	if r.HP != 1 {
		t.Fatalf("被打死应原地以 1 血复活，实际 %d", r.HP)
	}
	if !end.Penalty.InPlace {
		t.Fatal("被打死的 penalty 应标 in_place")
	}
	if p, ok := end.Penalty.Respawn.(positionView); !ok || p.Floor != 3000 || p.X != 10 || p.Y != 12 || p.Direction != 2 {
		t.Fatalf("被打死的 respawn 应是当前位置：%+v", end.Penalty.Respawn)
	}
	if want := gamedata.ClampCharm(before + charmOnDeath(r.Level, false)); r.Charm != want {
		t.Errorf("被打死的魅力应照旧扣减，want %d got %d", want, r.Charm)
	}
}

// TestDefeatFlownRespawnsAtSavepointOrHometown 被打飞：没记录回出生地，记录过回存档点；血量剩 1，不标 in_place。
func TestDefeatFlownRespawnsAtSavepointOrHometown(t *testing.T) {
	const jiajiaSave = 419
	sp := gamedata.NpcSavepointOf(jiajiaSave)
	if sp == nil {
		t.Fatal("419 号应是加加村的储存点")
	}
	h := gamedata.Hometowns[0]
	cases := []struct {
		name      string
		savepoint int
		floor     int
		x, y      int
	}{
		{"没记录回出生地", 0, h.Floor, h.X, h.Y},
		{"记录过回存档点", jiajiaSave, sp.Born[0], sp.Born[1], sp.Born[2]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			r := &record{Name: "勇者小明", Level: 1, HP: 30, Charm: 60, Floor: 3000, X: 10, Y: 12, Direction: 2, Hometown: 0, Savepoint: c.savepoint}
			end := &battle.EndView{Result: "lose", Penalty: &battle.Penalty{}}
			before := r.Charm
			e.chars.applyResult(r, loseBattle(t, true), end)
			if want := gamedata.ClampCharm(before + charmOnDeath(r.Level, true)); r.Charm != want {
				t.Errorf("被打飞的魅力应多扣（charmOnDeath），want %d got %d", want, r.Charm)
			}
			if r.Floor != c.floor || r.X != c.x || r.Y != c.y || r.Direction != gamedata.DefaultDirection {
				t.Fatalf("被打飞应送回 %d (%d,%d)，实际 %d (%d,%d) 朝向 %d", c.floor, c.x, c.y, r.Floor, r.X, r.Y, r.Direction)
			}
			if r.HP != 1 {
				t.Fatalf("被打飞血量应剩 1，实际 %d", r.HP)
			}
			if end.Penalty.InPlace {
				t.Fatal("被打飞不应标 in_place")
			}
		})
	}
}

// TestPenaltyInPlaceJSON 线上格式：in_place 只在原地复活时出现，被打飞时省略（协议 10.9）。
func TestPenaltyInPlaceJSON(t *testing.T) {
	in, _ := json.Marshal(battle.Penalty{Respawn: 1, InPlace: true})
	if !strings.Contains(string(in), `"in_place":true`) {
		t.Errorf("原地复活应带 in_place:true：%s", in)
	}
	out, _ := json.Marshal(battle.Penalty{Respawn: 1})
	if strings.Contains(string(out), "in_place") {
		t.Errorf("被打飞应省略 in_place：%s", out)
	}
}

// armEncounter 把角色的遇敌开关直接打开（不开第一场），用来测“战斗结束后开关怎么走”。
func armEncounter(t *testing.T, e *env, name string, plan *encounterPlan) {
	t.Helper()
	p := e.presence(t, name)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.encounterOn = true
	p.plan = plan
}

// drainUntilNextBattle 读到下一场的 battle_start；途中出现遇敌开关的推送就失败（原地复活不该关开关）。
func (c *client) drainUntilNextBattle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m := c.recv()
		switch m["type"] {
		case "battle_start":
			return
		case "encounter_changed":
			t.Fatalf("原地复活不应关掉遇敌开关：%v", m)
		}
	}
	t.Fatal("5 秒内没有开下一场")
}

// loseInPlace 判断 battle_end 是不是被打死原地复活（penalty.in_place，STAGE-194）。
func loseInPlace(end map[string]any) bool {
	pen, _ := end["penalty"].(map[string]any)
	return pen["in_place"] == true
}

// TestDefeatInPlaceKeepsEncounterOn 被打死原地复活：遇敌开着，隔一会儿开下一场，不推关闭。
func TestDefeatInPlaceKeepsEncounterOn(t *testing.T) {
	e, c := encounterEnv(t, 300*time.Millisecond, 9)
	c.warpTo(t, 6) // 玛丽娜丝近郊，野外有敌人
	armEncounter(t, e, "勇者小明", nil)
	if err := e.chars.StartBattle("勇者小明", "lose"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_start")
	end := c.playWildToEnd(t)
	if end["result"] != "lose" || !loseInPlace(end) {
		t.Fatalf("剧本 lose 应是被打死原地复活：%v", end)
	}
	c.drainTo(t, "status_changed")
	c.drainUntilNextBattle(t)
}

// TestDefeatFlownTurnsEncounterOff 被打飞送走：遇敌关掉，原因 lose。
func TestDefeatFlownTurnsEncounterOff(t *testing.T) {
	e, c := encounterEnv(t, 300*time.Millisecond, 9)
	c.warpTo(t, 6)
	armEncounter(t, e, "勇者小明", nil)
	if err := e.chars.StartBattle("勇者小明", "fly_defeat"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_start")
	end := c.playWildToEnd(t)
	if end["result"] != "lose" || loseInPlace(end) {
		t.Fatalf("剧本 fly 应是被打飞的失败：%v", end)
	}
	c.drainTo(t, "status_changed")
	off := c.drainEncounterOff(t)
	if off["reason"] != "lose" {
		t.Fatalf("被打飞的自动关闭原因应为 lose：%v", off)
	}
}

// TestDefeatInPlaceOnceTurnsOff 单次遇敌：被打死原地复活也照常关掉（打完就停）。
func TestDefeatInPlaceOnceTurnsOff(t *testing.T) {
	e, c := encounterEnv(t, 300*time.Millisecond, 9)
	c.warpTo(t, 6)
	armEncounter(t, e, "勇者小明", &encounterPlan{Mode: "once"})
	if err := e.chars.StartBattle("勇者小明", "lose"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_start")
	end := c.playWildToEnd(t)
	if !loseInPlace(end) {
		t.Fatalf("剧本 lose 应是被打死原地复活：%v", end)
	}
	c.drainTo(t, "status_changed")
	off := c.drainEncounterOff(t)
	if off["reason"] != "once_done" {
		t.Fatalf("单次遇敌打输原地复活，关闭原因应为 once_done：%v", off)
	}
}

// TestDefeatFlownOnceTurnsOff 单次遇敌被打飞：关掉，原因是 lose（打飞优先按失败关）。
func TestDefeatFlownOnceTurnsOff(t *testing.T) {
	e, c := encounterEnv(t, 300*time.Millisecond, 9)
	c.warpTo(t, 6)
	armEncounter(t, e, "勇者小明", &encounterPlan{Mode: "once"})
	if err := e.chars.StartBattle("勇者小明", "fly_defeat"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_start")
	end := c.playWildToEnd(t)
	if end["result"] != "lose" || loseInPlace(end) {
		t.Fatalf("剧本 fly 应是被打飞的失败：%v", end)
	}
	c.drainTo(t, "status_changed")
	off := c.drainEncounterOff(t)
	if off["reason"] != "lose" {
		t.Fatalf("单次遇敌被打飞，关闭原因应为 lose：%v", off)
	}
}
