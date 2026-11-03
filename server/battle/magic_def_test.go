// 防御次数的引擎测试（STAGE-199，对照 gmsv/battle/battle_event.c 的 BATTLE_GetDamageReact、
// BATTLE_DamageSub、BATTLE_DuckCheck）：吸收、反射、无效的结算与次数，优先级，投掷武器不反射，
// 带次数的目标不闪避。

package battle

import "testing"

// defenseFixture 是 testSide 对怪物的一场战斗：人物是被打的一方，15 号怪是攻击者。
func defenseFixture(t *testing.T) (*Battle, *Unit, *Unit) {
	t.Helper()
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	enemy := b.Unit(15)
	enemy.HP, enemy.MaxHP = 500, 500 // 反射、吸收的数值测试不让怪物在这一下倒下
	return b, b.PlayerUnit(), enemy
}

func TestDefReactVanishZeroesDamage(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Vanish = 2
	before := you.HP
	var ev Event
	if !b.defReact(enemy, you, 50, &ev) {
		t.Fatalf("无效次数大于 0 应处理这一下")
	}
	if you.HP != before || you.Def.Vanish != 1 {
		t.Fatalf("无效：血量不变、次数 2 → 1，实际 hp=%d vanish=%d", you.HP, you.Def.Vanish)
	}
	eff := effectOn(ev, you.No)
	if eff == nil || eff.Result != "vanish" || eff.Delta != 0 || len(eff.StatusRemove) != 0 {
		t.Fatalf("应有 vanish 效果且次数没用完不去标记: %+v", ev.Effects)
	}
}

func TestDefReactVanishRemovesMarkWhenUsedUp(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Vanish = 1
	var ev Event
	b.defReact(enemy, you, 50, &ev)
	eff := effectOn(ev, you.No)
	if eff == nil || len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "vanish" {
		t.Fatalf("次数用完应去掉 vanish 标记: %+v", ev.Effects)
	}
}

func TestDefReactAbsorbHealsDamageCappedAtMax(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Absorb = 1
	you.HP = you.MaxHP - 10
	var ev Event
	if !b.defReact(enemy, you, 50, &ev) {
		t.Fatalf("吸收次数大于 0 应处理这一下")
	}
	if you.HP != you.MaxHP || you.Def.Absorb != 0 {
		t.Fatalf("吸收：血量封顶为最大血量、次数用完，实际 hp=%d max=%d absorb=%d", you.HP, you.MaxHP, you.Def.Absorb)
	}
	eff := effectOn(ev, you.No)
	if eff == nil || eff.Result != "absorb" || eff.Delta != 10 || eff.HP == nil || *eff.HP != you.MaxHP {
		t.Fatalf("吸收的回复量应是 10（封顶）: %+v", ev.Effects)
	}
	if len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "absorb" {
		t.Fatalf("次数用完应去掉 absorb 标记: %+v", eff)
	}
}

func TestDefReactAbsorbHealsFullDamageWhenRoom(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Absorb = 2
	you.HP = 100
	var ev Event
	b.defReact(enemy, you, 30, &ev)
	if you.HP != 130 || you.Def.Absorb != 1 {
		t.Fatalf("吸收 30 点伤害应回 30 血、次数 2 → 1: hp=%d absorb=%d", you.HP, you.Def.Absorb)
	}
	if eff := effectOn(ev, you.No); eff == nil || len(eff.StatusRemove) != 0 {
		t.Fatalf("次数没用完不去标记: %+v", eff)
	}
}

func TestDefReactReflectHitsAttacker(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Reflect = 1
	enemyBefore, youBefore := enemy.HP, you.HP
	var ev Event
	if !b.defReact(enemy, you, 40, &ev) {
		t.Fatalf("反射次数大于 0 应处理这一下")
	}
	if enemy.HP != enemyBefore-40 || you.HP != youBefore || you.Def.Reflect != 0 {
		t.Fatalf("反射：攻击者扣 40、被打者不扣、次数 1 → 0，实际 enemy=%d you=%d reflect=%d", enemy.HP, you.HP, you.Def.Reflect)
	}
	eff := effectOn(ev, enemy.No)
	if eff == nil || eff.Result != "reflect" || eff.Delta != -40 || eff.Intent == nil || *eff.Intent != you.No {
		t.Fatalf("反射效果应落在攻击者身上、delta 为负、intent 指回被打的一方: %+v", ev.Effects)
	}
	// 次数用完：被打的单位另起一条效果去掉 reflect 标记
	if mark := effectOn(ev, you.No); mark == nil || len(mark.StatusRemove) != 1 || mark.StatusRemove[0] != "reflect" {
		t.Fatalf("反射次数用完应在被打的单位身上去掉 reflect: %+v", ev.Effects)
	}
}

func TestDefReactReflectSkippedForThrowingAttacker(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Reflect = 1
	enemy.Weapon = weaponThrou
	enemy.WeaponMotion = motionBoomerang
	enemyBefore := enemy.HP
	var ev Event
	if b.defReact(enemy, you, 40, &ev) {
		t.Fatalf("攻击者投掷时不反射，应照常扣血")
	}
	if enemy.HP != enemyBefore || you.Def.Reflect != 1 || len(ev.Effects) != 0 {
		t.Fatalf("投掷武器：攻击者不扣血、计数器不减、没有效果: enemy=%d reflect=%d effects=%v", enemy.HP, you.Def.Reflect, ev.Effects)
	}
}

// 优先级：无效 > 吸收 > 反射，同时挂着时每次只消耗一种，三个计数器互相独立。
func TestDefReactPriorityVanishThenAbsorbThenReflect(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def = DefenseCounters{Absorb: 1, Reflect: 1, Vanish: 1}
	youBefore, enemyBefore := you.HP, enemy.HP
	var ev Event
	b.defReact(enemy, you, 20, &ev) // 第一下：无效
	if you.Def != (DefenseCounters{Absorb: 1, Reflect: 1, Vanish: 0}) || you.HP != youBefore {
		t.Fatalf("第一下应是无效: %+v hp=%d", you.Def, you.HP)
	}
	b.defReact(enemy, you, 20, &ev) // 第二下：吸收
	if you.Def != (DefenseCounters{Absorb: 0, Reflect: 1, Vanish: 0}) {
		t.Fatalf("第二下应是吸收: %+v", you.Def)
	}
	b.defReact(enemy, you, 20, &ev) // 第三下：反射
	if you.Def.Reflect != 0 || enemy.HP != enemyBefore-20 {
		t.Fatalf("第三下应是反射: %+v enemy=%d", you.Def, enemy.HP)
	}
}

func TestDefReactIgnoresZeroDamageAndNoCounters(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Vanish = 1
	var ev Event
	if b.defReact(enemy, you, 0, &ev) || you.Def.Vanish != 1 {
		t.Fatalf("伤害为 0 不结算，计数器不减")
	}
	you.Def = DefenseCounters{}
	if b.defReact(enemy, you, 50, &ev) {
		t.Fatalf("没有防御次数不处理")
	}
}

// 带防御次数的目标不闪避（BATTLE_DuckCheck 一开始就返回 FALSE）：rnd 总出 0，闪避必中，
// 没有防御次数时确实闪避，挂上后不再闪避。
func TestDefenseCountersStopDodge(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(0))
	you, enemy := b.PlayerUnit(), b.Unit(15)
	if !b.duckCheck(enemy, you) {
		t.Fatalf("对照：没有防御次数、骰子必中时应闪避")
	}
	you.Def.Absorb = 1
	if b.duckCheck(enemy, you) {
		t.Fatalf("带吸收次数的目标不应闪避")
	}
}

func TestUnitStatusListsDefenseMarks(t *testing.T) {
	_, you, _ := defenseFixture(t)
	you.Def = DefenseCounters{Absorb: 1, Vanish: 2}
	got := unitStatus(you)
	if len(got) != 2 || got[0] != "absorb" || got[1] != "vanish" {
		t.Fatalf("有次数的标记应挂在状态列表末尾: %v", got)
	}
}

// 光的精灵（250）：己方一个，缺省落在自己身上，挂上吸收 1 次，气力 8。
func TestDefendSpellShieldsSelfByDefault(t *testing.T) {
	b := newBattleWithSide(t, magicSide(250), "win", rollRnd(9999))
	you := b.PlayerUnit()
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: -1})
	expectCode(t, perr, "")
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "magic" {
		t.Fatalf("应产生一个 magic 事件: %+v", evs)
	}
	eff := effectOn(evs[0], you.No)
	if eff == nil || eff.Result != "none" || len(eff.StatusAdd) != 1 || eff.StatusAdd[0] != "absorb" {
		t.Fatalf("应有挂上 absorb 的效果: %+v", evs[0].Effects)
	}
	if you.Def.Absorb != 1 || you.MP != 100-8 {
		t.Fatalf("吸收应为 1、气力扣 8: absorb=%d mp=%d", you.Def.Absorb, you.MP)
	}
}

// 守护的精灵（270）：可以选己方的宠物，挂上无效 2 次，气力 12。
func TestDefendSpellShieldsPet(t *testing.T) {
	b := newBattleWithSide(t, magicSide(270), "win", rollRnd(9999))
	you, pet := b.PlayerUnit(), b.PetUnit()
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5})
	expectCode(t, perr, "")
	evs := b.act(you)
	if eff := effectOn(evs[0], 5); eff == nil || len(eff.StatusAdd) != 1 || eff.StatusAdd[0] != "vanish" {
		t.Fatalf("宠物应挂上 vanish: %+v", evs[0].Effects)
	}
	if pet.Def.Vanish != 2 || you.Def.Vanish != 0 || you.MP != 100-12 {
		t.Fatalf("无效次数应给宠物 2、人物不变、气力扣 12: pet=%+v you=%+v mp=%d", pet.Def, you.Def, you.MP)
	}
}

// 防御精灵只能选己方；选敌方回复 INVALID_TARGET（同 10.5 的 ally）。
func TestDefendSpellRejectsEnemy(t *testing.T) {
	b := newBattleWithSide(t, magicSide(250), "win", rollRnd(9999))
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15})
	expectCode(t, perr, "INVALID_TARGET")
}

// 同一个次数再挂一次是覆盖，不叠加：吸收 1 次挂两遍还是 1 次，状态只通知一次。
func TestDefendUnitOverwritesNotStacks(t *testing.T) {
	b := newBattleWithSide(t, magicSide(250), "win", rollRnd(9999))
	you := b.PlayerUnit()
	var ev Event
	b.defendUnit(you, "absorb", 1, &ev)
	you.Def.Absorb = 0 // 消耗掉
	b.defendUnit(you, "absorb", 1, &ev)
	b.defendUnit(you, "absorb", 1, &ev)
	if you.Def.Absorb != 1 {
		t.Fatalf("覆盖不叠加：吸收应为 1，实际 %d", you.Def.Absorb)
	}
	if len(ev.Effects[1].StatusAdd) != 1 || len(ev.Effects[2].StatusAdd) != 0 {
		t.Fatalf("已经挂着的不重复通知 status_add: %+v", ev.Effects)
	}
}

// 复活光精灵 Lv1（280）：救起倒下的宠物，回 100 血（rollRnd(10)），并挂上吸收 1 次，气力 12。
func TestReviveDefendRevivesAndShields(t *testing.T) {
	b := newBattleWithSide(t, magicSide(280), "win", rollRnd(10))
	you, pet := b.PlayerUnit(), b.PetUnit()
	pet.HP, pet.Down = 0, true
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5})
	expectCode(t, perr, "")
	evs := b.act(you)
	if pet.Down || pet.HP != 100 || pet.Def.Absorb != 1 {
		t.Fatalf("宠物应活着、回 100 血、吸收 1 次: down=%v hp=%d absorb=%d", pet.Down, pet.HP, pet.Def.Absorb)
	}
	if eff := effectOn(evs[0], 5); eff == nil || eff.Result != "heal" || eff.Delta != 100 {
		t.Fatalf("应先有复活的 heal 效果: %+v", evs[0].Effects)
	}
	if you.MP != 100-12 {
		t.Fatalf("复活光 Lv1 的气力应扣 12（复活 8 + 吸收 4）: %d", you.MP)
	}
}

// 复活防御同样不能救活着的单位或怪物（INVALID_TARGET）。
func TestReviveDefendRejectsLivingAndMonsters(t *testing.T) {
	b := newBattleWithSide(t, magicSide(300), "win", rollRnd(9999))
	sturdy(b, 15).Down = true
	for _, target := range []int{5, 0, 15} {
		_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: target})
		expectCode(t, perr, "INVALID_TARGET")
	}
}

// 下面是物理直接攻击走真实的攻击流程（b.attack，即 strike）时的表现：光、镜子、守护各自的规则。
// rollRnd(9999) 让闪避、暴击都不触发；怪物（15 号）是攻击者。

// 光的精灵：被普攻一次，不扣血、反而回血（封顶为最大血量），吸收次数 1 → 0；第二次被打正常扣血。
func TestAbsorbSpellHealsOnPhysicalHit(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	enemy.Attack = 400 // 打得动，吸收的回血才大于 封顶所需的 5 点
	you.Def.Absorb = 1
	you.HP = you.MaxHP - 5
	evs := []Event{b.attack(enemy, you)}
	eff := effectOn(evs[0], you.No)
	if eff == nil || eff.Result != "absorb" || eff.Delta != 5 || you.HP != you.MaxHP {
		t.Fatalf("第一下应吸收并封顶回到最大血量（回 5 血）: eff=%+v hp=%d", eff, you.HP)
	}
	if you.Def.Absorb != 0 {
		t.Fatalf("吸收次数应用完: %d", you.Def.Absorb)
	}
	you.HP = 200
	ev2 := b.attack(enemy, you)
	if eff := effectOn(ev2, you.No); eff == nil || eff.Result != "hit" || eff.Delta >= 0 || you.HP >= 200 {
		t.Fatalf("第二下应正常扣血: eff=%+v hp=%d", eff, you.HP)
	}
}

// 镜子的精灵：被打一次，攻击者扣血、目标不扣血，反射次数 1 → 0。
func TestReflectSpellBouncesPhysicalHit(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Reflect = 1
	youBefore, enemyBefore := you.HP, enemy.HP
	ev := b.attack(enemy, you)
	if you.HP != youBefore || enemy.HP >= enemyBefore {
		t.Fatalf("反射：被打者不扣、攻击者扣: you=%d enemy=%d", you.HP, enemy.HP)
	}
	if eff := effectOn(ev, enemy.No); eff == nil || eff.Result != "reflect" || eff.Delta >= 0 {
		t.Fatalf("攻击者身上应有 reflect 效果: %+v", ev.Effects)
	}
	if you.Def.Reflect != 0 {
		t.Fatalf("反射次数应用完: %d", you.Def.Reflect)
	}
}

// 攻击者装备回旋标时不反射：按普通伤害结算，被打者照常扣血，计数器不减。
func TestReflectSpellSkippedForBoomerang(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Reflect = 1
	enemy.Weapon, enemy.WeaponMotion = weaponThrou, motionBoomerang
	youBefore := you.HP
	ev := b.attack(enemy, you)
	if you.HP >= youBefore || you.Def.Reflect != 1 {
		t.Fatalf("回旋标：被打者照常扣血、反射次数不减: hp=%d reflect=%d", you.HP, you.Def.Reflect)
	}
	if eff := effectOn(ev, you.No); eff == nil || eff.Result != "hit" {
		t.Fatalf("应是普通命中: %+v", ev.Effects)
	}
}

// 守护的精灵：连续被打两次都是无效（伤害 0），第三次正常扣血。
func TestVanishSpellBlocksTwoHits(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def.Vanish = 2
	for i := 1; i <= 2; i++ {
		before := you.HP
		ev := b.attack(enemy, you)
		if you.HP != before || effectOn(ev, you.No).Result != "vanish" {
			t.Fatalf("第 %d 下应无效: hp=%d result=%s", i, you.HP, effectOn(ev, you.No).Result)
		}
	}
	before := you.HP
	ev := b.attack(enemy, you)
	if you.HP >= before || effectOn(ev, you.No).Result != "hit" {
		t.Fatalf("第三下应正常扣血: hp=%d result=%s", you.HP, effectOn(ev, you.No).Result)
	}
}

// 三种同时挂上（无效 1、吸收 1、反射 1）：第一下无效，第二下吸收，第三下反射（攻击者扣血）。
func TestAllThreeCountersPriorityInAttack(t *testing.T) {
	b, you, enemy := defenseFixture(t)
	you.Def = DefenseCounters{Absorb: 1, Reflect: 1, Vanish: 1}
	you.HP = you.MaxHP - 1
	want := []string{"vanish", "absorb", "reflect"}
	for i, res := range want {
		ev := b.attack(enemy, you)
		target := you.No
		if res == "reflect" {
			target = enemy.No
		}
		if eff := effectOn(ev, target); eff == nil || eff.Result != res {
			t.Fatalf("第 %d 下应是 %s: %+v", i+1, res, ev.Effects)
		}
	}
}

// 带防御次数的目标不闪避：骰子总出 0（闪避必中），没有次数时闪开，挂上后就不闪避。
func TestDefenseCountersBlockDodgeInAttack(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(0))
	you, enemy := b.PlayerUnit(), b.Unit(15)
	enemy.HP, enemy.MaxHP = 500, 500
	if ev := b.attack(enemy, you); effectOn(ev, you.No).Result != "dodge" {
		t.Fatalf("对照：没有次数时骰子必中应闪避: %+v", ev.Effects)
	}
	you.Def.Absorb = 1
	if ev := b.attack(enemy, you); effectOn(ev, you.No).Result == "dodge" {
		t.Fatalf("带吸收次数不应闪避: %+v", ev.Effects)
	}
}

// 精灵伤害不触发防御：人物施放岩石撞击打带无效次数的怪物，怪物照常扣血，次数不减。
func TestSpellDamageIgnoresDefense(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	enemy := b.Unit(15)
	enemy.HP, enemy.MaxHP = 500, 500
	enemy.Def.Vanish = 2
	enemyBefore := enemy.HP
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 0, Target: 15}); perr != nil {
		t.Fatalf("岩石撞击应可以提交: %v", perr)
	}
	ev := b.act(b.PlayerUnit())
	if enemy.HP >= enemyBefore || enemy.Def.Vanish != 2 {
		t.Fatalf("精灵伤害应照常扣血、次数不减: hp=%d vanish=%d ev=%+v", enemy.HP, enemy.Def.Vanish, ev)
	}
}

// 中毒扣血不触发防御（statusTick 直接 hurt）。
func TestPoisonIgnoresDefense(t *testing.T) {
	b, you, _ := defenseFixture(t)
	you.Status.Poison = 3
	you.Def.Vanish = 1
	before := you.HP
	b.statusTick()
	if you.HP >= before || you.Def.Vanish != 1 {
		t.Fatalf("中毒应照常扣血、无效次数不减: hp=%d vanish=%d", you.HP, you.Def.Vanish)
	}
}

// 复活镜精灵 Lv1（290）：救起倒下的宠物，回 100 血，挂上反射 1 次，气力 12。
func TestReviveDefendMirrorReflects(t *testing.T) {
	b := newBattleWithSide(t, magicSide(290), "win", rollRnd(10))
	you, pet := b.PlayerUnit(), b.PetUnit()
	pet.HP, pet.Down = 0, true
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5}); perr != nil {
		t.Fatalf("复活镜应可以救宠物: %v", perr)
	}
	b.act(you)
	if pet.Down || pet.HP != 100 || pet.Def.Reflect != 1 || you.MP != 100-12 {
		t.Fatalf("复活镜：宠物活着、回 100、反射 1、气力扣 12: down=%v hp=%d reflect=%d mp=%d", pet.Down, pet.HP, pet.Def.Reflect, you.MP)
	}
}

// 复活守护精灵 Lv1（300）：救起倒下的宠物，挂上无效 2 次，气力 16。
func TestReviveDefendGuardianVanishesTwice(t *testing.T) {
	b := newBattleWithSide(t, magicSide(300), "win", rollRnd(10))
	you, pet := b.PlayerUnit(), b.PetUnit()
	pet.HP, pet.Down = 0, true
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 5}); perr != nil {
		t.Fatalf("复活守护应可以救宠物: %v", perr)
	}
	b.act(you)
	if pet.Down || pet.Def.Vanish != 2 || you.MP != 100-16 {
		t.Fatalf("复活守护：宠物活着、无效 2、气力扣 16: down=%v vanish=%d mp=%d", pet.Down, pet.Def.Vanish, you.MP)
	}
}
