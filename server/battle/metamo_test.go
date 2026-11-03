package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-203：变幻的精灵（metamo）只能在战斗外用。战斗的 magic 指令遇到它回复 MAGIC_UNUSABLE；其他精灵不受影响。
func TestMetamoMagicUnusableInBattle(t *testing.T) {
	side := testSide()
	side.Magics[1] = 350 // 变幻的精灵 Lv1
	b := newBattleWithSide(t, side, "win", rollRnd(9999))
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1}); perr == nil || perr.Code != protocol.CodeMagicUnusable {
		t.Fatalf("变幻的精灵在战斗中：got %v want MAGIC_UNUSABLE", perr)
	}
	// 同一场战斗里，普通精灵（岩石撞击，栏位 0）照样能交。
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 0, Target: 15}); perr != nil {
		t.Fatalf("普通精灵应可用：%v", perr)
	}
}

// STAGE-203：castMagic 兜底——分身队自动出招不走提交校验，遇到变幻的精灵也不能施放，也不扣 MP。
func TestCastMagicSkipsMetamo(t *testing.T) {
	side := testSide()
	side.Magics[1] = 350
	b := newBattleWithSide(t, side, "win", rollRnd(9999))
	u := b.PlayerUnit()
	mp := u.MP
	tmpl, _ := b.slotMagic(1)
	evs := b.castMagic(u, tmpl, Cmd{Magic: 1})
	if len(evs) != 1 || evs[0].Kind != "skip" {
		t.Fatalf("变幻的精灵应该只给一个 skip 事件，得到 %+v", evs)
	}
	if u.MP != mp {
		t.Fatalf("MP 不该被扣：%d -> %d", mp, u.MP)
	}
}
