// 魔法闪避接进攻击精灵（STAGE-218，原版 BATTLE_MagicDodge）：每个被打的目标各判一次。
// 闪避率本身（人物运气 × 3、宠物和怪物等级 × 0.2 封顶 30）和掷骰边界见 magic_dragon_test.go。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 攻击精灵逐目标判闪避：10 级怪物闪避率 2，掷 2 闪过，不扣血、效果是 dodge。
func TestAttackMagicDodgesTarget(t *testing.T) {
	b := fullEnemyBattle(t)
	b.Rnd = func(n int) int {
		if n == 100 {
			return 1 // 掷 2
		}
		return 0
	}
	m := b.Unit(15)
	m.Level = 10
	before := m.HP
	tpl, _ := gamedata.MagicByID(301) // 岩石撞击：单体
	you := b.PlayerUnit()
	evs := b.castMagic(you, tpl, Cmd{Target: 15})
	if len(evs) != 1 {
		t.Fatalf("应只有一个 magic 事件: %+v", evs)
	}
	eff := effectOn(evs[0], 15)
	if eff == nil || eff.Result != "dodge" || eff.Delta != 0 {
		t.Fatalf("应有一条 dodge 效果: %+v", evs[0].Effects)
	}
	if m.HP != before {
		t.Fatalf("闪过后不应扣血：%d → %d", before, m.HP)
	}
	if you.MP != 100-tpl.MP {
		t.Fatalf("MP 应照扣 %d：%d", tpl.MP, you.MP)
	}
}

// 全体精灵对每个目标各判一次：掷 2 时 10 只 10 级怪全闪过（闪避率 2），10 条 dodge、血量都不变。
func TestAttackMagicDodgesEachTarget(t *testing.T) {
	b := fullEnemyBattle(t)
	b.Rnd = func(n int) int {
		if n == 100 {
			return 1
		}
		return 0
	}
	for no := 10; no < 20; no++ {
		b.Unit(no).Level = 10
	}
	tpl, _ := gamedata.MagicByID(305) // 地震：全体
	evs := b.castMagic(b.PlayerUnit(), tpl, Cmd{})
	if len(evs) != 1 {
		t.Fatalf("应只有一个 magic 事件: %+v", evs)
	}
	if len(evs[0].Effects) != 10 {
		t.Fatalf("应有 10 条效果，实际 %d", len(evs[0].Effects))
	}
	for _, eff := range evs[0].Effects {
		if eff.Result != "dodge" || eff.Delta != 0 {
			t.Fatalf("每个目标都应闪过: %+v", eff)
		}
	}
	for no := 10; no < 20; no++ {
		if b.Unit(no).HP != 100 {
			t.Fatalf("%d 号不应扣血: %d", no, b.Unit(no).HP)
		}
	}
}
