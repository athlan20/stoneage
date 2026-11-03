// 公式的单元测试（STAGE-44）：对照原版 gmsv/battle/battle_event.c 手算的期望值。
// 每个用例固定随机源，公式和常量来自 BATTLE_DamageCalc、BATTLE_GuardAdjust、
// BATTLE_AttrCalc、BATTLE_DuckCheck、BATTLE_CriticalCheckPlayer。

package battle

import (
	"math"
	"testing"
)

// d10000 是让所有万分率骰子都出 v 的随机源。
func d10000(v int) func(n int) int {
	return func(n int) int {
		if n == 10000 {
			return v
		}
		return 0
	}
}

func TestDamageCalcHighAttack(t *testing.T) {
	// 攻 30 防 10（有效 7）：进入第三段 (attack-defense)×2 + K0。
	// rnd(10000)=5000 → K0 = 0.5×30/8 − 30/16 = 1.875 − 1.875 = 0；
	// 双方都设成地 100（相克系数 1.0）→ 伤害 = (30−7)×2 + 0 = 46。
	b := newFormulaBattle(d10000(5000))
	at := b.Unit(0)
	at.Attack, at.Elem = 30, Elements{Earth: 100}
	df := b.Unit(15)
	df.Defense, df.Elem = 10, Elements{Earth: 100}
	if got := b.damageCalc(at, df); got != 46 {
		t.Fatalf("伤害 = %d，应为 46", got)
	}
}

func TestDamageCalcLowAttack(t *testing.T) {
	// 攻 2 防 10（有效 7 > 2）：第二段 RAND(0,1)，rnd(2)=1；双方地 100（相克 1.0）→ 伤害 1。
	b := newFormulaBattle(func(n int) int {
		if n == 2 {
			return 1
		}
		return 0
	})
	at, df := b.Unit(0), b.Unit(15)
	at.Attack, at.Elem = 2, Elements{Earth: 100}
	df.Defense, df.Elem = 10, Elements{Earth: 100}
	if got := b.damageCalc(at, df); got != 1 {
		t.Fatalf("伤害 = %d，应为 1", got)
	}
}

func TestDamageCalcElementClash(t *testing.T) {
	// 攻 100 防 0：伤害 (100−0)×2 = 200（K0 取 rnd=9999：9999/10000×100/8−100/16 ≈ 12.49−6.25=6.24）。
	// 攻击方地 100 对防御方水 100：地克水 1.5 → 200×100×100×1.5/10000 = 300。
	b := newFormulaBattle(d10000(9999))
	at, df := b.Unit(0), b.Unit(15)
	at.Attack, at.Elem = 100, Elements{Earth: 100}
	df.Defense, df.Elem = 0, Elements{Water: 100}
	k0 := float64(9999)/10000*100.0/8.0 - 100.0/16.0
	base := (100-0)*2 + int(k0)
	if got := b.damageCalc(at, df); got != int(float64(base)*1.5) {
		t.Fatalf("伤害 = %d，应为 %d（地克水 1.5 倍）", got, int(float64(base)*1.5))
	}
	// 换成被克：攻击方地对防御方风 → 0.6 倍。
	df.Elem = Elements{Wind: 100}
	if got := b.damageCalc(at, df); got != int(float64(base)*0.6) {
		t.Fatalf("伤害 = %d，应为 %d（地被风克 0.6 倍）", got, int(float64(base)*0.6))
	}
}

func TestGuardAdjust(t *testing.T) {
	cases := []struct{ roll, want int }{
		{25, 0}, {26, 10}, {50, 10}, {51, 20}, {70, 20}, {71, 30}, {85, 30}, {86, 40}, {95, 40}, {96, 50},
	}
	for _, c := range cases {
		if got := guardAdjust(100, c.roll); got != c.want {
			t.Fatalf("roll %d: 减伤 = %d，应为 %d", c.roll, got, c.want)
		}
	}
}

func TestDuckCheckRate(t *testing.T) {
	// 怪物（敏 16）打玩家（敏 4，非玩家攻玩家 At×0.6=9.6）：big=9.6 small=4、攻方敏高 wari=1；
	// work=(9.6−4)/0.02=280、sqrt=16.73、per=1673。骰子 1673（roll+1 后等于）→ 闪避；1674 → 不闪避。
	b := newFormulaBattle(d10000(1672))
	at, df := b.Unit(15), b.Unit(0)
	at.Agility, df.Agility = 16, 4
	if !b.duckCheck(at, df) {
		t.Fatal("骰子 1673 恰好等于闪避率（≤ 判定），应闪避")
	}
	b2 := newFormulaBattle(d10000(1673))
	if b2.duckCheck(at, df) {
		t.Fatal("骰子 1674 大于闪避率，不应闪避")
	}
	// 防御中的目标不闪避（原版直接返回 FALSE）。
	df.Status.Guard = true
	if b.duckCheck(at, df) {
		t.Fatal("防御中不应闪避")
	}
}

func TestCriticalCheckRate(t *testing.T) {
	// 怪物（敏 16）打玩家（敏 4）：非玩家打玩家 → divpara=10、不开根号、攻方敏高 wari=1。
	// work=(16−4)/10=1.2；per=1.2×100=120。
	at, df := b_formulaUnits()
	at.Kind, at.Agility = KindMonster, 16
	df.Kind, df.Agility = KindPlayer, 4
	if got := criticalCheck(at, df); got != 120 {
		t.Fatalf("暴击率 = %d，应为 120", got)
	}
	// 玩家打怪物：Df×0.6=9.6，divpara=0.09、开根号：work=(16−9.6)/0.09≈71.1、sqrt≈8.43、wari=1 → 843。
	a2, d2 := b_formulaUnits()
	a2.Kind, a2.Agility = KindPlayer, 16
	d2.Kind, d2.Agility = KindMonster, 16
	want := int(math.Sqrt((16.0-9.6)/gCriticalPara) * 100)
	if got := criticalCheck(a2, d2); got != want {
		t.Fatalf("暴击率 = %d，应为 %d", got, want)
	}
}

func TestCriDamage(t *testing.T) {
	// 暴击伤害 = 基础伤害 + 防御 × 攻击等级 / 防御等级 × 0.5：100 + 20×3/6×0.5 = 105。
	at, df := b_formulaUnits()
	at.Level, df.Level = 3, 6
	df.Defense = 20
	if got := criDamageCalc(at, df, 100); got != 105 {
		t.Fatalf("暴击伤害 = %d，应为 105", got)
	}
}

func TestCounterRequiresAttackCommand(t *testing.T) {
	// 反击的条件：本回合的指令是攻击。防御中的单位不会反击。
	b := newFormulaBattle(d10000(0))
	at, df := b.Unit(0), b.Unit(15)
	at.Agility, df.Agility = 40, 4 // 敏捷差大 → 反击率拉满
	if _, fired, _ := b.counter(at, df); fired {
		t.Fatal("没有交攻击指令不应反击")
	}
	b.Submit(1, "player", "attack", Cmd{Target: 15})
	if _, fired, _ := b.counter(at, df); !fired {
		t.Fatal("交了攻击指令应能反击（骰子 0 必中小概率）")
	}
}

// newFormulaBattle 起一场 win 剧本的战斗（单位数值随后改），随机源固定。
func newFormulaBattle(rnd func(n int) int) *Battle {
	b, err := Build("b", testSide(), "win")
	if err != nil {
		panic(err)
	}
	b.Rnd = rnd
	return b
}

// b_formulaUnits 给出两个互不隶属测试战斗的单位。
func b_formulaUnits() (*Unit, *Unit) {
	b := newFormulaBattle(d10000(0))
	return b.Unit(0), b.Unit(15)
}

func TestEscapeCheckRate(t *testing.T) {
	// 1 级玩家（运气 0 → 第 1 档 30%）对两只 3 级乌力（敌均级 3）：
	// 第一次 30×1 − 2×(3−1) = 26；失败后第二次 30×2 − 4 = 56。
	b := newFormulaBattle(func(n int) int {
		if n == 100 {
			return 25 // roll 后 26：第一次 26 < 26 不成立（失败），第二次 26 < 56 成立
		}
		return 0
	})
	p := b.PlayerUnit()
	p.Level = 1
	for _, m := range [2]*Unit{b.Unit(15), b.Unit(16)} {
		m.Level = 3
	}
	if b.escapeCheck(p) {
		t.Fatal("骰子 26 不小于第一次的 26，应失败")
	}
	if !b.escapeCheck(p) {
		t.Fatal("骰子 26 小于第二次的 56，应成功")
	}
	// 敌方等级比我高很多时，概率降到下限 1，骰子再小也跑不掉。
	for _, m := range [2]*Unit{b.Unit(15), b.Unit(16)} {
		m.Level = 60
	}
	b.escapeTries[p.No] = 0
	if b.escapeCheck(p) {
		t.Fatal("等级差悬殊时概率 1，骰子 26 不小于 1，应失败")
	}
}

func TestPoisonAttackAndTick(t *testing.T) {
	// 附毒：体力占比 0.25 → 抗性 10，概率 30−10=20；骰子 19（roll 后）小于 20，成功。
	b := newFormulaBattle(func(n int) int {
		if n == 100 {
			return 18
		}
		return 0
	})
	at, df := b.Unit(0), b.Unit(15)
	at.Level, df.Level = 1, 1
	at.Vital, at.Strength, at.Toughness, at.Dexterity = 10, 10, 10, 10
	df.Vital, df.Strength, df.Toughness, df.Dexterity = 10, 10, 10, 10
	df.HP, df.MaxHP = 50, 50
	if !b.poisonAttack(at, df) {
		t.Fatal("骰子 19 小于概率 20，应附毒成功")
	}
	// 已有异常状态时不能再上。
	if b.poisonAttack(at, df) {
		t.Fatal("已有异常状态，不应再附毒")
	}
	// 回合末：中毒扣血（点数和 40：(40−20)/4=5）并解除（普攻毒只持续 1 回合）。
	events := b.statusTick()
	if len(events) != 1 || events[0].Kind != "status_tick" {
		t.Fatalf("应有一个 status_tick 事件：%+v", events)
	}
	if df.HP != 45 {
		t.Fatalf("毒扣血后 HP = %d，应为 45", df.HP)
	}
	if df.Status.Poison != 0 {
		t.Fatal("毒应在回合末解除")
	}
	// 血量见底时毒不死，至少留 1。
	df.HP, df.Status.Poison = 2, 1
	b.statusTick()
	if df.HP != 1 {
		t.Fatalf("毒不致死，HP = %d，应为 1", df.HP)
	}
}

func TestWakeUp(t *testing.T) {
	// 睡眠中的目标被击中会醒，效果里带 status_remove。
	b := newFormulaBattle(d10000(9999))
	at, df := b.Unit(0), b.Unit(15)
	df.Status.Sleep = 3
	at.Attack, df.Defense, df.HP, df.MaxHP = 10, 0, 50, 50
	at.Elem, df.Elem = Elements{Earth: 100}, Elements{Earth: 100}
	ev := b.attack(at, df)
	if df.Status.Sleep != 0 {
		t.Fatal("被打后应醒来")
	}
	last := ev.Effects[len(ev.Effects)-1]
	found := false
	for _, s := range last.StatusRemove {
		if s == "sleep" {
			found = true
		}
	}
	if !found {
		t.Fatalf("效果里应带 status_remove sleep：%+v", last)
	}
}

func TestParalysisSkip(t *testing.T) {
	// 麻痹的单位本回合不能行动：needs 里没有玩家，结算时怪物的行动是 skip。
	b := newFormulaBattle(d10000(9999))
	b.PlayerUnit().Status.Paralysis = 1
	b.nextTurn()
	for _, a := range b.Needs() {
		if a == "player" {
			t.Fatal("麻痹的玩家不需要交指令")
		}
	}
	m := b.Unit(15)
	m.Status.Paralysis = 1
	evs := b.monsterAct(m)
	if len(evs) != 1 || evs[0].Kind != "skip" || evs[0].Reason != "paralysis" {
		t.Fatalf("麻痹的怪物应 skip：%+v", evs)
	}
}

func TestConfusionPicksAnyTarget(t *testing.T) {
	// 混乱中的单位从所有活着的单位里随机选目标（这里骰子固定选第 0 个）。
	b := newFormulaBattle(func(n int) int { return 0 })
	u := b.PlayerUnit()
	u.Status.Confusion = 1
	if got := b.confuseTarget(u); got == nil || got.No != 0 {
		t.Fatalf("混乱目标 = %+v，应为场上第一个单位", got)
	}
}

// 无属性 = 100 − 四属性和（BATTLE_GetAttr）：无属性的攻击打地 60 水 40 的目标，
// 无属性对地、水都是“被克”（0.6），伤害 100 → 100·100·60·0.6 + 100·100·40·0.6 = 600000，×1e-4 = 60。
func TestAttrAdjustNoneElement(t *testing.T) {
	atk := &Unit{Kind: KindMonster}
	tgt := &Unit{Kind: KindPlayer, Elem: Elements{Earth: 60, Water: 40}}
	if got := (&Battle{}).attrAdjust(atk, tgt, 100); got != 60 {
		t.Fatalf("无属性攻击地水目标 = %d，期望 60", got)
	}
	// 属性和超过 100 时无属性截到 0，不出现负系数。
	over := &Unit{Kind: KindPlayer, Elem: Elements{Earth: 80, Water: 80}}
	if got := elemVector(over.Elem)[4]; got != 0 {
		t.Fatalf("属性和超过 100 时无属性应为 0，实际 %v", got)
	}
}

// 普攻附毒（BATTLE_StatusAttackCheck）：体力 10/四项和 40 → 抗性 0.25/0.25×10 = 10；
// 同级：per = 30 + 0 − 10 = 20，RAND(1,100) < 20 才中。
func TestPoisonAttackRoll(t *testing.T) {
	atk := &Unit{Kind: KindMonster, Level: 5}
	tgt := func() *Unit {
		return &Unit{Kind: KindPlayer, Level: 5, Vital: 10, Strength: 10, Toughness: 10, Dexterity: 10}
	}
	for roll, want := range map[int]bool{18: true, 19: false} { // Rnd 出 18 → roll 19 < 20 中；出 19 → roll 20 不中
		b := &Battle{Rnd: func(int) int { return roll }}
		u := tgt()
		if got := b.poisonAttack(atk, u); got != want {
			t.Errorf("Rnd=%d: poisonAttack = %v，期望 %v", roll, got, want)
		}
	}
}
