// 连续攻击的测试（STAGE-291 批 A）：每一下伤害平分（原版 damage /= gDamageDiv，最少 1），
// 打满 N 下才停（原版出手循环不看单次结果）。
package battle

import "testing"

func TestComboShareDivides(t *testing.T) {
	cases := []struct{ dmg, n, want int }{{10, 3, 3}, {9, 3, 3}, {2, 3, 1}, {7, 2, 3}}
	for _, c := range cases {
		if got := comboShare(c.dmg, c.n); got != c.want {
			t.Errorf("comboShare(%d,%d) = %d，want %d", c.dmg, c.n, got, c.want)
		}
	}
}

// bigHP 让怪物扛得住三下，免得第一下就倒下改打别人。
func bigHP(b *Battle, no int) {
	u := b.Unit(no)
	u.HP, u.MaxHP = 100000, 100000
}

// 三段攻击打同一只怪：三下都打满，每下是普攻伤害的三分之一（至少 1），三下之和同量级。
func TestComboThreeHitsShareNormalDamage(t *testing.T) {
	normal := skillTestBattle(t, 1)
	bigHP(normal, 15)
	nRound := playTurn(t, normal, 15)
	nEv, ok := petEvent(nRound)
	if !ok {
		t.Fatal("普攻没有宠物事件")
	}
	full := damageTo(nEv, 15)
	if full < 3 {
		t.Fatalf("普攻伤害 %d 太小，测不出平分", full)
	}

	combo := skillTestBattle(t, 11)
	bigHP(combo, 15)
	cEv, ok := petEvent(playTurn(t, combo, 15))
	if !ok {
		t.Fatal("三段攻击没有宠物事件")
	}
	hits := 0
	for _, e := range cEv.Effects {
		if e.Target != 15 {
			continue
		}
		hits++
		if got, want := -e.Delta, max(full/3, 1); got != want {
			t.Errorf("第 %d 下伤害 %d，want %d（普攻 %d 的三分之一）", hits, got, want, full)
		}
	}
	if hits != 3 {
		t.Errorf("三段攻击打了 %d 下，want 3", hits)
	}
}
