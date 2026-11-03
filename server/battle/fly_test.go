package battle

import "testing"

// 人物被打飞（STAGE-63，battle_event.c 的 BATTLE_DamageSub）：单次伤害 ≥ 最大血量×1.2+20 直接打飞；
// 打倒时超出血量的部分（溢出）跨战斗累计，累计达到同一阈值也打飞；打飞后累计清零。
func TestFlyRules(t *testing.T) {
	cases := []struct {
		name         string
		hp, maxHP    int // 开战时的血量；阈值 = maxHP×1.2+20
		carried      int // 带进这场战斗的累计溢出值
		dmg          int
		wantFlown    bool
		wantOverkill int
	}{
		{"单次伤害到阈值", 50, 50, 0, 80, true, 0},
		{"单次伤害差一点", 50, 50, 0, 79, false, 29},
		{"没打倒不累计", 50, 50, 0, 30, false, 0},
		{"溢出累计到阈值", 20, 50, 30, 70, true, 0},
		{"溢出累计没到阈值", 20, 50, 30, 60, false, 70},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			side := testSide()
			side.HP, side.MaxHP, side.Overkill = c.hp, c.maxHP, c.carried
			b := newBattleWithSide(t, side, "win", fixedRnd())
			var ev Event
			b.hurt(b.PlayerUnit(), c.dmg, "hit", &ev)
			if got := b.PlayerFlown(); got != c.wantFlown {
				t.Errorf("打飞 = %v，want %v", got, c.wantFlown)
			}
			if got := b.PlayerUnit().Overkill; got != c.wantOverkill {
				t.Errorf("累计溢出 = %d，want %d", got, c.wantOverkill)
			}
		})
	}
}

// 怪物的伤害不影响人物的打飞判定：只有玩家人物单位记录打飞（怪物的打飞标记 STAGE-63 不做）。
func TestFlyOnlyRecordsPlayer(t *testing.T) {
	b := newTestBattle(t, "win", fixedRnd())
	var ev Event
	m := b.Unit(enemySide*sideOffset + firstPetNo)
	b.hurt(m, 10000, "hit", &ev)
	if m.Flown || b.PlayerFlown() {
		t.Errorf("怪物被打倒不应记成人物被打飞：怪物 %v，人物 %v", m.Flown, b.PlayerFlown())
	}
}
