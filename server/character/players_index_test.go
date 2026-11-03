package character

import "testing"

// STAGE-273：附近玩家索引的合并规则与地图归属（不需要数据库，也不推送）。

func newTestPresence(id int64) *Presence {
	return &Presence{charID: id}
}

func TestPlayersIndexFollowsFloor(t *testing.T) {
	x := newPlayerIndex()
	a, b := newTestPresence(1), newTestPresence(2)
	x.add(a, 1000, "甲")
	x.add(b, 1000, "乙")
	if got := x.onFloor(1000, 1); len(got) != 1 || got[0].charID != 2 {
		t.Fatalf("同图除自己以外应只有乙，得到 %v", got)
	}
	// 乙换图到 2000：1000 图上没有人了，2000 图上有乙。
	b.rec.Floor = 2000
	x.moved(b)
	if got := x.onFloor(1000, 1); len(got) != 0 {
		t.Fatalf("乙离开后 1000 图应为空，得到 %d 人", len(got))
	}
	if got := x.onFloor(2000, 1); len(got) != 1 {
		t.Fatalf("乙应在 2000 图上，得到 %d 人", len(got))
	}
	x.remove(b)
	if got := x.onFloor(2000, 1); len(got) != 0 {
		t.Fatalf("乙离开世界后 2000 图应为空")
	}
	if len(x.entries) != 1 {
		t.Fatalf("索引里应只剩甲，得到 %d 条", len(x.entries))
	}
}

func TestPlayersIndexNilSafe(t *testing.T) {
	var x *playerIndex
	p := newTestPresence(1)
	x.add(p, 1000, "甲")
	x.moved(p)
	x.touch(p)
	x.remove(p)
	if got := x.onFloor(1000, 0); got != nil {
		t.Fatalf("nil 索引应返回 nil")
	}
}

// 合并规则：同一玩家在窗口内的多次变化只留一条；leave 覆盖之前的；离开后又进入按 enter。
func TestPlayersPendingMerge(t *testing.T) {
	x := newPlayerIndex()
	defer func() {
		if x.timer != nil {
			x.timer.Stop()
		}
	}()
	e := &playerEntry{charID: 7, name: "丙", floor: 1000}
	x.mu.Lock()
	defer x.mu.Unlock()

	x.putPending(1000, &playerChange{charID: 7, name: "丙", entry: e, enter: true})
	x.putPending(1000, &playerChange{charID: 7, name: "丙", entry: e})
	if c := x.pending[1000][7]; c == nil || !c.enter || c.leave {
		t.Fatalf("enter 之后的 update 应仍是 enter，得到 %+v", c)
	}

	x.putPending(1000, &playerChange{charID: 7, name: "丙", leave: true})
	if c := x.pending[1000][7]; c == nil || !c.leave {
		t.Fatalf("leave 应覆盖之前的 enter，得到 %+v", c)
	}

	x.putPending(1000, &playerChange{charID: 7, name: "丙", entry: e})
	if c := x.pending[1000][7]; c == nil || c.leave || !c.enter {
		t.Fatalf("离开后又整条替换应视为 enter，得到 %+v", c)
	}

	x.putPending(1000, &playerChange{charID: 7, name: "丙", entry: e})
	x.putPending(1000, &playerChange{charID: 7, name: "丙", entry: e})
	if n := len(x.pending[1000]); n != 1 {
		t.Fatalf("同一玩家应只留一条，得到 %d 条", n)
	}
}

// 合并计时器只有一个：有待发变化时才起，flush 之后清空，不会每次新建。
func TestPlayersTimerNotLeaked(t *testing.T) {
	x := newPlayerIndex()
	p := newTestPresence(1)
	x.add(p, 1000, "甲")
	first := x.timer
	x.mu.Lock()
	x.putPending(1000, &playerChange{charID: 1, name: "甲", leave: true})
	x.mu.Unlock()
	if x.timer != first {
		t.Fatalf("已有计时器时不应重新创建")
	}
	x.flush()
	if x.timer != nil || len(x.pending) != 0 {
		t.Fatalf("flush 之后计时器和待发表都应清空")
	}
}
