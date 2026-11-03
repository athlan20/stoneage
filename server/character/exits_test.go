package character

import (
	"fmt"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func exitRequest(floor int, id string) string {
	return fmt.Sprintf(`{"source_floor":%d,"exit_id":%q}`, floor, id)
}
func TestUseExitVillageDoorsAndSharedEffects(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	live := e.chars.LiveOf(e.session("alice"))
	for _, door := range gamedata.ExitsOf(2000)[:2] {
		c.okp("warp", `{"floor":2000}`)
		if err := live.With(func(r *record) { r.Direction = 6; live.p.encounterOn = true }); err != nil {
			t.Fatal(err)
		}
		m, pushes := c.callp("use_exit", exitRequest(2000, door.ID))
		if m["ok"] != true || len(pushes) != 3 || pushes[0]["type"] != "position_changed" || pushes[1]["type"] != "encounter_changed" || pushes[2]["type"] != "notice" {
			t.Fatalf("wrong ordered pushes %v %v", m, pushes)
		}
		p := pushes[0]["data"].(map[string]any)
		cell := door.Cells[0]
		if p["floor"] != float64(100) || p["x"] != float64(cell.ToX) || p["y"] != float64(cell.ToY) || p["direction"] != float64(6) {
			t.Fatal(p)
		}
		if f, x, y := position(c.ok("status", `{}`)); f != 100 || x != cell.ToX || y != cell.ToY {
			t.Fatalf("actual state %d %d %d", f, x, y)
		}
	}
	c.okp("warp", `{"floor":1006}`)
	talkChef(t, c)
	exit := gamedata.ExitsOf(1006)[0]
	m, pushes := c.callp("use_exit", exitRequest(1006, exit.ID))
	if m["ok"] != true || len(pushes) != 2 || pushes[0]["type"] != "position_changed" || pushes[1]["type"] != "npc_window_closed" {
		t.Fatalf("window not closed after position %v %v", m, pushes)
	}
}
func TestUseExitFailureIsAtomicAndRechecksClock(t *testing.T) {
	var seconds atomic.Int64
	atHour := func(h int) { seconds.Store(912766409 + 5400 + int64((h*5400+1023)/1024)) }
	atHour(701)
	e := newEnv(t, func(s *Service) { s.opts.Now = func() time.Time { return time.Unix(seconds.Load(), 0) } })
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	live := e.chars.LiveOf(e.session("alice"))
	talkChef(t, c)
	// Keep a window open while changing only test state, so every failure can prove it is untouched.
	move(t, live, 2000, 72, 33, 44)
	live.With(func(r *record) { live.p.encounterOn = true })
	for _, raw := range []string{`{}`, `{"source_floor":2000}`, `{"source_floor":"2000","exit_id":"x"}`, `{"source_floor":2000,"exit_id":null}`, `{"source_floor":2000,"exit_id":3}`, `{"source_floor":2.5,"exit_id":"x"}`} {
		c.wantErr(t, "use_exit", raw, "BAD_REQUEST")
	}
	door := gamedata.ExitsOf(2000)[0]
	for _, tt := range []struct {
		floor    int
		id, code string
	}{{100, door.ID, "POSITION_CHANGED"}, {2000, "absent", "EXIT_NOT_FOUND"}, {2000, gamedata.ExitsOf(1006)[0].ID, "EXIT_NOT_FOUND"}} {
		c.wantErr(t, "use_exit", exitRequest(tt.floor, tt.id), tt.code)
	}
	list := c.ok("exits", `{}`)
	if list["floor"] != float64(2000) || list["next_change_ms"] == nil {
		t.Fatal(list)
	}
	var morning gamedata.MapExit
	for _, d := range gamedata.ExitsOf(2000) {
		if d.ToFloor == 2011 {
			morning = d
		}
	}
	found := false
	for _, raw := range list["exits"].([]any) {
		v := raw.(map[string]any)
		if v["id"] == morning.ID {
			found = v["available"] == true
		}
	}
	if !found {
		t.Fatal("morning door missing/open state wrong")
	}
	atHour(931)
	c.wantErr(t, "use_exit", exitRequest(2000, morning.ID), "EXIT_UNAVAILABLE")
	if f, x, y := position(c.ok("status", `{}`)); f != 2000 || x != 72 || y != 33 {
		t.Fatalf("failure moved %d %d %d", f, x, y)
	}
	live.With(func(r *record) {
		if live.p.npcWindow == nil || !live.p.encounterOn {
			t.Error("failure changed window/encounter")
		}
	})
	c.noMessage()
	atHour(701)
	m, pushes := c.callp("use_exit", exitRequest(2000, morning.ID))
	if m["ok"] != true || len(pushes) != 4 || pushes[0]["type"] != "position_changed" || pushes[1]["type"] != "npc_window_closed" || pushes[2]["type"] != "encounter_changed" {
		t.Fatalf("timed success side effects %v %v", m, pushes)
	}
	cell := morning.Cells[0]
	if f, x, y := position(c.ok("status", `{}`)); f != 2011 || x != cell.ToX || y != cell.ToY {
		t.Fatalf("timed target %d %d %d", f, x, y)
	}
	// A whole conflicted component remains visible but cannot move the player.
	move(t, live, 300, 10, 10, 44)
	for _, d := range gamedata.ExitsOf(300) {
		if d.Conflict {
			c.wantErr(t, "use_exit", exitRequest(300, d.ID), "EXIT_UNAVAILABLE")
			break
		}
	}
	if f, x, y := position(c.ok("status", `{}`)); f != 300 || x != 10 || y != 10 {
		t.Fatal("conflict moved player")
	}
	move(t, live, 5555, 1, 2, 44)
	empty := c.ok("exits", `{}`)
	if len(empty["exits"].([]any)) != 0 || empty["floor"] != float64(5555) || empty["next_change_ms"] != nil {
		t.Fatal(empty)
	}
}
func TestUseExitAgentControlAndSharedPosition(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"floor":2000}`)
	agent := e.agentFor(c)
	agent.roundtrip(t, "enter_world", `{"slot":0}`, "control_changed")
	c.recv()
	door := gamedata.ExitsOf(2000)[0]
	request := exitRequest(2000, door.ID)
	c.ok("exits", `{}`)
	agent.ok("exits", `{}`)
	c.wantErr(t, "use_exit", request, "CONTROLLED_BY_AGENT")
	_, push := agent.roundtrip(t, "use_exit", request, "position_changed")
	player := c.push("position_changed")
	if !reflect.DeepEqual(player, push) || push["x"] != float64(79) || push["y"] != float64(614) {
		t.Fatalf("shared exact position %v %v", player, push)
	}
	c.roundtrip(t, "take_control", `{}`, "control_changed")
	agent.push("control_changed")
	agent.wantErr(t, "use_exit", exitRequest(100, door.ID), "CONTROLLED_BY_PLAYER")
}
func TestUseExitBattleBlocked(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	door := gamedata.ExitsOf(1006)[0]
	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "use_exit", exitRequest(1006, door.ID), "IN_BATTLE")
	if f, x, y := position(c.ok("status", `{}`)); f != 1006 || x != 15 || y != 22 {
		t.Fatalf("battle failure moved %d %d %d", f, x, y)
	}
	c.drainToEnd()
}

func TestWorldViewIncludesHomeWithoutRecordedSavepoint(t *testing.T) {
	r := record{Hometown: 0}
	p := &Presence{members: map[protocol.Role]*Live{}}
	v := worldView(&r, p, protocol.RolePlayer, time.Now())
	home, ok := v["savepoint"].(map[string]any)
	h := gamedata.Hometowns[0]
	if !ok || home["floor"] != h.Floor || home["x"] != h.X || home["y"] != h.Y {
		t.Fatalf("missing authoritative fallback %v", v["savepoint"])
	}
}
