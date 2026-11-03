package gamedata

import (
	"testing"
	"time"
)

func exitAtHour(h int) time.Time { return time.Unix(912766409+5400+int64((h*5400+1023)/1024), 0) }
func TestExitTimeBoundaries(t *testing.T) {
	for _, tt := range []struct {
		h       int
		m, a, n bool
	}{{0, false, true, false}, {200, false, true, false}, {201, false, false, false}, {300, false, false, false}, {301, false, false, true}, {700, false, false, true}, {701, true, false, false}, {930, true, false, false}, {931, false, true, false}, {1023, false, true, false}} {
		for gate, want := range map[string]bool{"morning": tt.m, "noon": tt.a, "night": tt.n, "all": true} {
			if got := exitTimeActive(gate, lsHour(exitAtHour(tt.h))); got != want {
				t.Errorf("hour %d gate %s = %v", tt.h, gate, got)
			}
		}
	}
	for _, gate := range []string{"morning", "noon", "night"} {
		e := MapExit{TimeGate: gate}
		for _, h := range []int{0, 200, 201, 300, 301, 700, 701, 930, 931, 1023} {
			now := exitAtHour(h).Add(500 * time.Millisecond)
			next := NextExitChange([]MapExit{e}, now)
			if next == nil || *next <= now.UnixMilli() {
				t.Fatalf("missing change %s %d", gate, h)
			}
			before := time.UnixMilli(*next - 1)
			after := time.UnixMilli(*next)
			if e.UnavailableAt(before) == e.UnavailableAt(after) {
				t.Fatalf("not a state boundary %s %d", gate, h)
			}
		}
	}
	if NextExitChange([]MapExit{{TimeGate: "all"}, {TimeGate: "morning", Conflict: true}}, time.Now()) != nil {
		t.Fatal("no usable timed exit")
	}
}
func TestExitVillageTargetsAndCopy(t *testing.T) {
	exits := ExitsOf(2000)
	if len(exits) != 13 {
		t.Fatal(len(exits))
	}
	a, b := exits[0], exits[1]
	if a.ToFloor != 100 || b.ToFloor != 100 || a.ID == b.ID || a.Cells[0] != (ExitCell{74, 33, 79, 614}) || b.Cells[0] != (ExitCell{108, 79, 114, 660}) {
		t.Fatalf("wrong doors %v %v", a, b)
	}
	exits[0].Cells[0].ToX = 999
	if ExitsOf(2000)[0].Cells[0].ToX != 79 {
		t.Fatal("mutable global data")
	}
	if len(ExitsOf(300)) != 31 {
		t.Fatal("jiru count")
	}
	if len(ExitsOf(-1)) != 0 {
		t.Fatal("empty map")
	}
}
