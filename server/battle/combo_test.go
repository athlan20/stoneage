// 合击（STAGE-126）的测试：分组条件、起头概率、结算规则对照原版 ComboCheck / BATTLE_Combo。

package battle

import "testing"

// comboRnd 让合击的 d100 出 roll，判定类的 d10000 出大值（不闪不暴击），其余二选一类取固定值 pick。
func comboRnd(roll, pick int) func(n int) int {
	return func(n int) int {
		switch n {
		case 100:
			return roll
		case 10000:
			return 9999
		}
		return pick % n
	}
}

// comboBattle 开一场合击打开的战斗：人物（敏捷 4）和宠物（敏捷 7）都攻击 15 号怪；
// 怪物敏捷压到 1，排在最后，不挤进人物和宠物之间。
func comboBattle(t *testing.T, rnd func(n int) int) *Battle {
	t.Helper()
	b := newTestBattle(t, "win", rnd)
	b.Combo = true
	for _, no := range []int{15, 16} {
		b.Unit(no).Agility = 1
		b.Unit(no).HP, b.Unit(no).MaxHP = 500, 500
	}
	return b
}

func submitAttacks(t *testing.T, b *Battle, player, pet int) {
	t.Helper()
	if _, perr := b.Submit(1, "player", "attack", Cmd{Target: player}); perr != nil {
		t.Fatal(perr)
	}
	if _, perr := b.Submit(1, "pet", "attack", Cmd{Target: pet}); perr != nil {
		t.Fatal(perr)
	}
}

func firstEvent(t *testing.T, b *Battle) Event {
	t.Helper()
	round, _ := b.Settle()
	if len(round.Events) == 0 {
		t.Fatal("一个事件都没有")
	}
	return round.Events[0]
}

// 人物和宠物打同一个怪，起头判定过了：合成一个 combo 事件，宠物（敏捷高）先出手，每人一条效果。
func TestComboPlayerAndPet(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	submitAttacks(t, b, 15, 15)
	ev := firstEvent(t, b)
	if ev.Kind != "combo" || ev.Actor == nil || *ev.Actor != 5 {
		t.Fatalf("第一个事件 = %+v，want combo，起头是宠物 5", ev)
	}
	if len(ev.Actors) != 2 || ev.Actors[0] != 5 || ev.Actors[1] != 0 {
		t.Fatalf("actors = %v，want [5 0]", ev.Actors)
	}
	if len(ev.Effects) != 2 {
		t.Fatalf("effects 有 %d 条，want 2（一人一条）", len(ev.Effects))
	}
	for i, eff := range ev.Effects {
		if eff.Target != 15 || (eff.Result != "hit" && eff.Result != "crit") || eff.Delta >= 0 {
			t.Errorf("effects[%d] = %+v，want 打在 15 号上的命中", i, eff)
		}
	}
	if ev.Effects[1].HPPct > ev.Effects[0].HPPct {
		t.Errorf("第二下之后血量比例 %d 反而比第一下后的 %d 高", ev.Effects[1].HPPct, ev.Effects[0].HPPct)
	}
}

// 合击不判闪避：即使闪避判定必中，合击仍然命中。
func TestComboIgnoresDodge(t *testing.T) {
	rnd := func(n int) int {
		if n == 100 {
			return 0
		}
		return 0 // d10000 出 0：闪避判定必中（普通攻击会 dodge）
	}
	b := comboBattle(t, rnd)
	b.Unit(15).Agility = 100 // 对方敏捷高，闪避率高（也先出手，先看它们的事件）
	submitAttacks(t, b, 15, 15)
	round, _ := b.Settle()
	var ev Event
	for _, e := range round.Events {
		if e.Kind == "combo" {
			ev = e
		}
	}
	if ev.Kind != "combo" {
		t.Fatalf("没有 combo 事件：%+v", round.Events)
	}
	for i, eff := range ev.Effects {
		if eff.Result == "dodge" || eff.Result == "miss" || eff.Delta >= 0 {
			t.Errorf("effects[%d] = %+v，合击不会闪开或落空，至少 1 点伤害", i, eff)
		}
	}
}

// 起头判定没过（d100 出 99 → 100 > 50）：各打各的。
func TestComboRollFails(t *testing.T) {
	b := comboBattle(t, comboRnd(99, 0))
	submitAttacks(t, b, 15, 15)
	round, _ := b.Settle()
	for _, ev := range round.Events {
		if ev.Kind == "combo" {
			t.Fatalf("起头判定没过却出现了 combo：%+v", ev)
		}
	}
}

// 概率边界：d100 出 49（roll 50）刚好过人物方的 50%，出 50（roll 51）过不了。
func TestComboPlayerPercentBoundary(t *testing.T) {
	for _, tc := range []struct {
		d    int
		want bool
	}{{49, true}, {50, false}} {
		b := comboBattle(t, comboRnd(tc.d, 0))
		submitAttacks(t, b, 15, 15)
		got := firstEvent(t, b).Kind == "combo"
		if got != tc.want {
			t.Errorf("d100=%d：合击 = %v，want %v", tc.d, got, tc.want)
		}
	}
}

// 打的不是同一个目标：不合击。
func TestComboNeedsSameTarget(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	submitAttacks(t, b, 15, 16)
	if ev := firstEvent(t, b); ev.Kind == "combo" {
		t.Fatalf("目标不同却合击了：%+v", ev)
	}
}

// 合击开关关着（剧本默认）：不合击。
func TestComboOffByDefault(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	b.Combo = false
	submitAttacks(t, b, 15, 15)
	if ev := firstEvent(t, b); ev.Kind == "combo" {
		t.Fatalf("合击没打开却出现了 combo：%+v", ev)
	}
}

// 人物和宠物之间隔着一个对面的单位：队伍在出手顺序里不挨着，不合击。
func TestComboBrokenByOtherSide(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	b.Unit(15).Agility = 5 // 宠物 7 > 怪物 5 > 人物 4
	submitAttacks(t, b, 15, 15)
	round, _ := b.Settle()
	for _, ev := range round.Events {
		if ev.Kind == "combo" && ev.Actor != nil && *ev.Actor < unitsPerSide {
			t.Fatalf("人物和宠物被怪物隔开却合击了：%+v", ev)
		}
	}
}

// 装了投掷武器（回旋标）的不参与合击。
func TestComboSkipsThrowWeapon(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	b.Unit(0).Weapon = weaponThrou
	submitAttacks(t, b, 15, 15)
	if ev := firstEvent(t, b); ev.Kind == "combo" {
		t.Fatalf("人物拿着投掷武器却合击了：%+v", ev)
	}
}

// 目标被前面的人打倒，后面的人不再出手：actors 与 effects 都只有一项。
func TestComboStopsWhenTargetDown(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	b.Unit(15).HP = 1
	submitAttacks(t, b, 15, 15)
	ev := firstEvent(t, b)
	if ev.Kind != "combo" || len(ev.Actors) != 1 || len(ev.Effects) != 1 || !ev.Effects[0].Down {
		t.Fatalf("事件 = %+v，want 只有起头的一下并打倒目标", ev)
	}
	if len(b.slain) != 1 {
		t.Errorf("击杀名单 %d 个，want 1（不能重复记）", len(b.slain))
	}
}

// 怪物也会合击：两只怪挨着、都打人物时（20%）合成一次，轮到的怪物用提前定好的行动，不重复抽。
func TestComboMonsters(t *testing.T) {
	b := comboBattle(t, comboRnd(0, 0))
	b.Unit(15).Agility, b.Unit(16).Agility = 20, 19 // 怪物先于人物和宠物出手
	for _, no := range []int{0, 5} {
		b.Unit(no).HP, b.Unit(no).MaxHP = 5000, 5000
	}
	submitAttacks(t, b, 15, 15)
	ev := firstEvent(t, b)
	if ev.Kind != "combo" || len(ev.Actors) != 2 || ev.Actors[0] != 15 || ev.Actors[1] != 16 {
		t.Fatalf("第一个事件 = %+v，want 15、16 两只怪的合击", ev)
	}
	for _, eff := range ev.Effects {
		if eff.Target != ev.Effects[0].Target {
			t.Fatalf("合击打了不同的目标：%+v", ev.Effects)
		}
	}
	if b.Unit(15).plan != nil || b.Unit(16).plan != nil {
		t.Error("合击后怪物提前定的行动应该清掉")
	}
}
