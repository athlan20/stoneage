// STAGE-226：人物的魔法熟练度与抗性落库后重读还在，新角色全是 0。需要 STONEAGE_TEST_DATABASE_URL。

package character

import (
	"context"
	"testing"
	"time"
)

func TestMagicStatsSavedAndReloaded(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	want := [4]int{30, -5, 0, 100}
	var accountID int64
	withRec(t, e, "alice", func(r *record) {
		if r.MagicExp != ([4]int{}) || r.MagicResist != ([4]int{}) || r.MagicAttExp != ([4]int{}) || r.MagicDefExp != ([4]int{}) {
			t.Fatalf("新角色的魔法数值应全为 0，得到 exp=%v resist=%v att=%v def=%v", r.MagicExp, r.MagicResist, r.MagicAttExp, r.MagicDefExp)
		}
		r.MagicExp, r.MagicResist = want, [4]int{10, 20, 30, 40}
		r.MagicAttExp, r.MagicDefExp = [4]int{1, 2, 3, 4}, [4]int{5, 6, 7, 8}
		accountID = r.AccountID
		if err := e.chars.save(context.Background(), r, time.Now()); err != nil {
			t.Fatal(err)
		}
	})

	// 直接从库里读回（不经过内存里的记录）。
	got, err := e.chars.findBySlot(context.Background(), accountID, 0)
	if err != nil || got == nil {
		t.Fatalf("重读角色失败：%v", err)
	}
	if got.MagicExp != want {
		t.Errorf("MagicExp = %v, want %v", got.MagicExp, want)
	}
	if got.MagicResist != [4]int{10, 20, 30, 40} {
		t.Errorf("MagicResist = %v, want [10 20 30 40]", got.MagicResist)
	}
	if got.MagicAttExp != [4]int{1, 2, 3, 4} || got.MagicDefExp != [4]int{5, 6, 7, 8} {
		t.Errorf("MagicAttExp = %v, MagicDefExp = %v", got.MagicAttExp, got.MagicDefExp)
	}
}
