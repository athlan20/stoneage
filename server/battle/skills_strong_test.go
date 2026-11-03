// 强化攻击类宠物技能的测试（STAGE-76）：一击必杀、突击、背水之战、忠犬、地球一周、不防守战法。
// 怪物开场麻痹（不出招，只看宠物的出手）；骰子默认不触发闪避、暴击，需要判定触发时用 rollAt 指定掷数。

package battle

import "testing"

// skillTestBattle 开一场 win 剧本的战斗：宠物的第 1 个技能栏位换成 skill，两只怪物麻痹。
func skillTestBattle(t *testing.T, skill int) *Battle {
	t.Helper()
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	for _, no := range []int{15, 16} {
		b.Unit(no).Status.Paralysis = 99
	}
	b.PetUnit().Skills[0] = skill
	return b
}

// playTurn 交一回合：人物防御（不影响宠物的出手），宠物需要指令时交第 1 栏位的技能，然后结算。
func playTurn(t *testing.T, b *Battle, target int) Round {
	t.Helper()
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物防御提交失败: %v", perr)
	}
	if needsPet(b) {
		if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: target}); perr != nil {
			t.Fatalf("宠物技能提交失败: %v", perr)
		}
	}
	round, _ := b.Settle()
	return round
}

// petEvent 取出本回合宠物的技能事件。
func petEvent(round Round) (Event, bool) {
	for _, ev := range round.Events {
		if ev.Kind == "skill" && ev.Actor != nil && *ev.Actor == 5 {
			return ev, true
		}
	}
	return Event{}, false
}

// damageTo 是事件里对某个单位造成的伤害（正数）。
func damageTo(ev Event, target int) int {
	for _, e := range ev.Effects {
		if e.Target == target {
			return -e.Delta
		}
	}
	return 0
}

// rollAt 让 d10000 的判定掷出 v（roll = v），其余骰子取 0。
func rollAt(v int) func(n int) int {
	return func(n int) int {
		if n == 10000 {
			return v - 1
		}
		return 0
	}
}

// triggerCount 数一数掷数 1–10000 里有多少个让判定触发。判定是“掷数 ≤ 阈值”或“掷数 < 阈值”，
// 所以这个数就是阈值本身，比较两次的差就能看出修正量（百分点 × 100）。
func triggerCount(b *Battle, check func() bool) int {
	n := 0
	saved := b.Rnd
	defer func() { b.Rnd = saved }()
	for v := 1; v <= 10000; v++ {
		b.Rnd = rollAt(v)
		if check() {
			n++
		}
	}
	return n
}

// TestSkillOptionParsing 对照 petskill.txt 的参数写法读取数值（原版 strstr + sscanf）。
func TestSkillOptionParsing(t *testing.T) {
	cases := []struct {
		opt, marker string
		want        float64
		ok          bool
	}{
		{"攻%+90", "攻%", 90, true},
		{"攻%-20  COM:攻击", "攻%", -20, true},
		{"攻%+25 防%-35", "防%", -35, true},
		{"倍2 回避30", "倍", 2, true},
		{"倍3 回避15", "回避", 15, true},
		{"回避%+30 反击%+50 会心%+20", "反击%", 50, true},
		{"回避%+30 反击%+50 会心%+20", "会心%", 20, true},
		{"攻%+25 防%-35", "回避", 0, false},
	}
	for _, c := range cases {
		got, ok := optNumber(c.opt, c.marker)
		if ok != c.ok || got != c.want {
			t.Errorf("optNumber(%q, %q) = %v, %v；want %v, %v", c.opt, c.marker, got, ok, c.want, c.ok)
		}
	}
	if n, ok := leadingInt("2 攻%+110"); !ok || n != 2 {
		t.Errorf("leadingInt = %d, %v；want 2, true", n, ok)
	}
}

// TestMightyDoublesDamage：一击必杀（倍2）这一下的伤害是普通攻击的 2 倍。
func TestMightyDoublesDamage(t *testing.T) {
	normal := playTurn(t, skillTestBattle(t, 1), 15)
	mighty := playTurn(t, skillTestBattle(t, 40), 15)
	ne, _ := petEvent(normal)
	me, _ := petEvent(mighty)
	if got, want := damageTo(me, 15), 2*damageTo(ne, 15); got != want || want == 0 {
		t.Fatalf("一击必杀伤害 = %d，want 普通攻击的 2 倍 = %d", got, want)
	}
}

// TestMightyRaisesTargetDodge：一击必杀的回避值 +30% 加在对方的闪避率上（gBattleDuckModyfy）。
func TestMightyRaisesTargetDodge(t *testing.T) {
	b := skillTestBattle(t, 40)
	pet, boar := b.PetUnit(), b.Unit(15)
	base := triggerCount(b, func() bool { return b.duckCheck(pet, boar) })
	pet.act = skillMods{Duck: 30}
	defer func() { pet.act = skillMods{} }()
	more := triggerCount(b, func() bool { return b.duckCheck(pet, boar) })
	if more-base != 3000 {
		t.Fatalf("闪避阈值增加 %d，want 3000（30 个百分点）", more-base)
	}
}

// TestChargeAttackWaitsThenStrikes：突击（蓄力 1 回合）第一回合不出手、不要指令，下一回合按攻%+90 出手。
func TestChargeAttackWaitsThenStrikes(t *testing.T) {
	b := skillTestBattle(t, 30)
	r1 := playTurn(t, b, 15)
	ev1, ok := petEvent(r1)
	if !ok || len(ev1.Effects) != 0 {
		t.Fatalf("蓄力回合应只有技能名、没有效果：ok=%v effects=%d", ok, len(ev1.Effects))
	}
	if needsPet(b) {
		t.Fatalf("蓄力中的宠物下一回合不应再要指令，needs=%v", b.Needs())
	}
	r2 := playTurn(t, b, 15)
	ev2, ok := petEvent(r2)
	if !ok {
		t.Fatalf("蓄力结束的回合应有宠物的攻击事件")
	}
	// 对照：攻击力直接加上 攻%（基础攻击力 25 的 90% 取整 22）后的普通攻击
	ref := skillTestBattle(t, 1)
	ref.PetUnit().Attack += pctOf(ref.PetUnit().Attack, 90)
	re, _ := petEvent(playTurn(t, ref, 15))
	if got, want := damageTo(ev2, 15), damageTo(re, 15); got != want || want == 0 {
		t.Fatalf("突击伤害 = %d，want 攻击力 +90%% 的普通攻击 %d", got, want)
	}
}

// TestDoubleChargeWaitsTwoTurns：双重突击（蓄力 2 回合）前两个回合都不出手，第三回合才攻击。
func TestDoubleChargeWaitsTwoTurns(t *testing.T) {
	b := skillTestBattle(t, 31)
	for turn := 1; turn <= 2; turn++ {
		ev, ok := petEvent(playTurn(t, b, 15))
		if !ok || len(ev.Effects) != 0 {
			t.Fatalf("第 %d 回合应还在蓄力，没有效果", turn)
		}
	}
	ev, ok := petEvent(playTurn(t, b, 15))
	if !ok || damageTo(ev, 15) == 0 {
		t.Fatalf("第 3 回合应攻击造成伤害")
	}
}

// TestEarthRoundHidesThenStrikes：地球一周第一回合隐身（不能被选为目标），下一回合伤害 +90% 出手。
func TestEarthRoundHidesThenStrikes(t *testing.T) {
	b := skillTestBattle(t, 120)
	pet := b.PetUnit()
	ev1, ok := petEvent(playTurn(t, b, 15))
	if !ok || len(ev1.Effects) != 0 {
		t.Fatalf("隐身回合应只有技能名、没有效果")
	}
	if !pet.hidden() || pet.targetable() {
		t.Fatalf("隐身中的宠物应不能被选为目标")
	}
	for _, u := range b.aiTargets(b.Unit(15), aiTargetAll) {
		if u == pet {
			t.Fatalf("怪物的目标里不应有隐身中的宠物")
		}
	}
	ev2, ok := petEvent(playTurn(t, b, 15))
	if !ok || pet.hidden() {
		t.Fatalf("下一回合宠物应出手，隐身解除")
	}
	ref := skillTestBattle(t, 1)
	re, _ := petEvent(playTurn(t, ref, 15))
	if got, want := damageTo(ev2, 15), int(float64(damageTo(re, 15))*1.9); got != want || want == 0 {
		t.Fatalf("地球一周伤害 = %d，want 普通攻击的 1.9 倍 = %d", got, want)
	}
}

// TestPowerBalanceAttackAndDefenseOnlyThisTurn：背水之战攻击力 +25%，防御力 -35% 只在这一回合生效。
func TestPowerBalanceAttackAndDefenseOnlyThisTurn(t *testing.T) {
	b := skillTestBattle(t, 50)
	pet := b.PetUnit()
	base := pet.Defense
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物防御提交失败: %v", perr)
	}
	if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: 15}); perr != nil {
		t.Fatalf("宠物技能提交失败: %v", perr)
	}
	b.beginRound()
	if got, want := pet.defenseNow(), float64(base+pctOf(base, -35)); got != want {
		t.Fatalf("背水之战中防御力 = %v，want %v", got, want)
	}
	round, _ := b.Settle()
	ev, _ := petEvent(round)

	ref := skillTestBattle(t, 1)
	ref.PetUnit().Attack += pctOf(ref.PetUnit().Attack, 25)
	re, _ := petEvent(playTurn(t, ref, 15))
	if got, want := damageTo(ev, 15), damageTo(re, 15); got != want || want == 0 {
		t.Fatalf("背水之战伤害 = %d，want 攻击力 +25%% 的普通攻击 %d", got, want)
	}

	pet.Skills[0] = 1
	playTurn(t, b, 16) // 下一回合开始时防御恢复（15 号这回合可能已经倒下，目标用还活着的 16 号）
	if got := pet.defenseNow(); got != float64(base) {
		t.Fatalf("下一回合防御力 = %v，want 恢复为 %d", got, base)
	}
}

// TestGuardianTakesHitForOwner：忠犬登记后，攻击主人的一下改由忠犬挨；忠犬睡着或攻击者是它时不替挡。
func TestGuardianTakesHitForOwner(t *testing.T) {
	b := skillTestBattle(t, 20)
	pet, boar, owner := b.PetUnit(), b.Unit(15), b.PlayerUnit()
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物防御提交失败: %v", perr)
	}
	if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: 15}); perr != nil {
		t.Fatalf("忠犬提交失败: %v", perr)
	}
	b.beginRound()
	if g := b.guardianOf(boar, owner); g != pet {
		t.Fatalf("主人的忠犬 = %v，want 宠物", g)
	}
	ownerHP := owner.HP
	ev := b.attack(boar, owner)
	if len(ev.Effects) != 1 || ev.Effects[0].Target != pet.No || owner.HP != ownerHP {
		t.Fatalf("攻击主人应由忠犬挨：effects=%+v 主人血量 %d→%d", ev.Effects, ownerHP, owner.HP)
	}
	pet.Status.Sleep = 2
	if g := b.guardianOf(boar, owner); g != nil {
		t.Fatalf("忠犬睡着时不应替挡")
	}
	pet.Status.Sleep = 0
	if g := b.guardianOf(pet, owner); g != nil {
		t.Fatalf("攻击者就是忠犬时不应替挡")
	}
}

// TestGuardianWithoutCommandDoesNotGuard：没有忠犬指令的回合，主人不会被替挡。
func TestGuardianWithoutCommandDoesNotGuard(t *testing.T) {
	b := skillTestBattle(t, 1)
	playTurn(t, b, 15)
	if g := b.guardianOf(b.Unit(15), b.PlayerUnit()); g != nil {
		t.Fatalf("普通攻击的回合不应有忠犬替挡")
	}
}

// TestGuardianAttackIsWeaker：忠犬出手时攻击力 -20%。
func TestGuardianAttackIsWeaker(t *testing.T) {
	ev, _ := petEvent(playTurn(t, skillTestBattle(t, 20), 15))
	ref := skillTestBattle(t, 1)
	ref.PetUnit().Attack += pctOf(ref.PetUnit().Attack, -20)
	re, _ := petEvent(playTurn(t, ref, 15))
	if got, want := damageTo(ev, 15), damageTo(re, 15); got != want || want == 0 {
		t.Fatalf("忠犬伤害 = %d，want 攻击力 -20%% 的普通攻击 %d", got, want)
	}
}

// TestNoGuardStance：不防守战法本回合不出手，回避率 +30%、可以反击（反击率 +50%）；会心 +20% 原版没有读取，这里也不生效。
func TestNoGuardStance(t *testing.T) {
	b := skillTestBattle(t, 150)
	pet, boar := b.PetUnit(), b.Unit(15)
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物防御提交失败: %v", perr)
	}
	if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: 15}); perr != nil {
		t.Fatalf("不防守提交失败: %v", perr)
	}
	b.beginRound()
	if !pet.stance.NoGuard || pet.stance.NoGuardDuck != 30 || pet.stance.NoGuardCounter != 50 {
		t.Fatalf("不防守的登记 = %+v，want 回避 30 反击 50", pet.stance)
	}
	if pet.Status.Guard {
		t.Fatalf("不防守不应设置防御状态")
	}

	// 回避：同样的判定，有不防守时阈值多 3000（30 个百分点）
	pet.stance = stanceMods{}
	duckBase := triggerCount(b, func() bool { return b.duckCheck(boar, pet) })
	pet.stance = stanceMods{NoGuard: true, NoGuardDuck: 30}
	if got := triggerCount(b, func() bool { return b.duckCheck(boar, pet) }) - duckBase; got != 3000 {
		t.Fatalf("不防守的回避增加 %d，want 3000", got)
	}

	// 反击：反击判定的阈值多 5000（50 个百分点）
	pet.stance = stanceMods{}
	counterBase := triggerCount(b, func() bool { return b.counterCheck(pet, boar) })
	pet.stance = stanceMods{NoGuard: true, NoGuardCounter: 50}
	if got := triggerCount(b, func() bool { return b.counterCheck(pet, boar) }) - counterBase; got != 5000 {
		t.Fatalf("不防守的反击增加 %d，want 5000", got)
	}
}

// TestNoGuardCanCounter：不防守的宠物在这一回合可以反击（不要求它在攻击）。
func TestNoGuardCanCounter(t *testing.T) {
	b := skillTestBattle(t, 150)
	pet, boar := b.PetUnit(), b.Unit(15)
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物防御提交失败: %v", perr)
	}
	if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: 15}); perr != nil {
		t.Fatalf("不防守提交失败: %v", perr)
	}
	b.beginRound()
	b.Rnd = rollAt(1) // 判定全部触发：反击、闪避都会发生
	if _, fired, _ := b.counter(pet, boar); !fired {
		t.Fatalf("不防守的宠物应能反击")
	}
}
