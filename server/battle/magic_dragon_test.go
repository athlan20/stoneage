package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 召唤龙阵（STAGE-202）的测试：魔法闪避的闪避率与边界，龙阵伤害（与 damageCalc 手算的结果比较），
// 单体与全体的目标，被闪避、倒下的目标，以及龙阵不受光、镜子、守护影响。
// rollRnd(k) 让 RAND(1,100) 出 k%100+1，所以 rollRnd(8) 的点数是 9。

// dragonCast 直接调用召唤龙阵的结算（不经过提交和 castMagic），返回它写进事件的效果。
func dragonCast(t *testing.T, b *Battle, id, target int) Event {
	t.Helper()
	tpl, ok := gamedata.MagicByID(id)
	if !ok {
		t.Fatalf("龙阵 %d 不在表里", id)
	}
	u := b.PlayerUnit()
	ev := Event{Kind: "magic", Actor: &u.No, Name: tpl.Name}
	b.castDragon(u, tpl, Cmd{Target: target}, &ev)
	return ev
}

// enemiesOf 返回对方场上活着的单位（单位号从小到大）。
func enemiesOf(b *Battle) []*Unit {
	var out []*Unit
	for _, x := range b.units {
		if x != nil && x.Alive() && x.Side() != b.PlayerUnit().Side() {
			out = append(out, x)
		}
	}
	return out
}

// tough 把对方的血量抬高，免得 300 威力的龙阵一下就打倒（剧本里的怪物只有十几点血）。
func tough(us ...*Unit) {
	for _, u := range us {
		u.HP, u.MaxHP = 5000, 5000
	}
}

// 魔法闪避率（百分点）：人物是运气 × 3；宠物和怪物是等级 × 0.2，最多 30。
func TestMagicDodgeRate(t *testing.T) {
	cases := []struct {
		name string
		u    *Unit
		want int
	}{
		{"人物运气 3", &Unit{Kind: KindPlayer, Luck: 3, Level: 50}, 9},
		{"人物运气 5", &Unit{Kind: KindPlayer, Luck: 5}, 15},
		{"宠物等级 50", &Unit{Kind: KindPet, Level: 50}, 10},
		{"怪物等级 7（向下取整）", &Unit{Kind: KindMonster, Level: 7}, 1},
		{"宠物等级 200 封顶", &Unit{Kind: KindPet, Level: 200}, 30},
		{"怪物等级 150 刚好 30", &Unit{Kind: KindMonster, Level: 150}, 30},
		{"怪物等级 151 仍封顶", &Unit{Kind: KindMonster, Level: 151}, 30},
	}
	for _, tc := range cases {
		if got := magicDodgeRate(tc.u); got != tc.want {
			t.Errorf("%s：闪避率 = %d，want %d", tc.name, got, tc.want)
		}
	}
}

// 边界（原版 RAND(1,100) 与 (int)fLuck 的比较）：点数小于等于闪避率就闪过，大于才没闪过。
func TestMagicDodgeBoundary(t *testing.T) {
	// 人物运气 3，闪避率 9：点数 9 闪过，点数 10 没闪过。
	player := &Unit{Kind: KindPlayer, Luck: 3}
	cases := []struct {
		name  string
		roll  int // rollRnd 的参数，点数是 roll+1
		dodge bool
	}{
		{"人物点数 8 闪过", 7, true},
		{"人物点数 9（等于闪避率）闪过", 8, true},
		{"人物点数 10 没闪过", 9, false},
	}
	for _, tc := range cases {
		b := newBattleWithSide(t, magicSide(394), "win", rollRnd(tc.roll))
		if got := b.magicDodge(player); got != tc.dodge {
			t.Errorf("%s：magicDodge = %v，want %v", tc.name, got, tc.dodge)
		}
	}
	// 宠物等级 50，闪避率 10：点数 10 闪过，11 没闪过。
	pet := &Unit{Kind: KindPet, Level: 50}
	for _, tc := range []struct {
		roll  int
		dodge bool
	}{{9, true}, {10, false}} {
		b := newBattleWithSide(t, magicSide(394), "win", rollRnd(tc.roll))
		if got := b.magicDodge(pet); got != tc.dodge {
			t.Errorf("宠物 roll %d：magicDodge = %v，want %v", tc.roll, got, tc.dodge)
		}
	}
}

// 奔龙阵（394）打一个：伤害与虚拟攻击方的 damageCalc 手算结果一致（攻击力 = 威力 300），扣在目标的血上。
func TestDragonSingleDamageMatchesDamageCalc(t *testing.T) {
	b := newBattleWithSide(t, magicSide(394), "win", rollRnd(9999))
	enemy := enemiesOf(b)[0]
	tough(enemy)
	virt := &Unit{Kind: KindMonster, Attack: 300, Luck: 1}
	want := b.damageCalc(virt, enemy)
	if want <= 0 {
		t.Fatalf("测试前提不成立：期望伤害 %d", want)
	}
	before := enemy.HP
	ev := dragonCast(t, b, 394, enemy.No)
	eff := effectOn(ev, enemy.No)
	if eff == nil || eff.Result != "hit" || eff.Delta != -want {
		t.Fatalf("奔龙阵的效果 = %+v，应为 hit、delta %d", eff, -want)
	}
	if enemy.HP != before-want {
		t.Fatalf("目标血量 = %d，应为 %d", enemy.HP, before-want)
	}
	if len(ev.Effects) != 1 {
		t.Fatalf("奔龙阵打一个，效果应只有一条，实际 %d", len(ev.Effects))
	}
}

// 奔雷阵（396）打全体：每个活着的敌方单位各一条效果，伤害都是同一个虚拟攻击方的结果；己方不受伤。
func TestDragonAllHitsEveryLivingEnemy(t *testing.T) {
	b := newBattleWithSide(t, magicSide(396), "win", rollRnd(9999))
	enemies := enemiesOf(b)
	if len(enemies) < 2 {
		t.Fatalf("测试前提不成立：对方至少要两个单位，实际 %d", len(enemies))
	}
	tough(enemies...)
	// 倒下的目标不出现在效果里（先放倒最后一个）。
	down := enemies[len(enemies)-1]
	down.Down = true
	alive := enemies[:len(enemies)-1]
	hp := map[int]int{}
	for _, x := range alive {
		hp[x.No] = x.HP
	}
	ev := dragonCast(t, b, 396, -1)
	if len(ev.Effects) != len(alive) {
		t.Fatalf("全体应对 %d 个活着的敌方各一条，实际 %d 条", len(alive), len(ev.Effects))
	}
	if effectOn(ev, down.No) != nil {
		t.Fatalf("倒下的单位 %d 不应出现在效果里", down.No)
	}
	virt := &Unit{Kind: KindMonster, Attack: 400, Luck: 1}
	for _, x := range alive {
		eff := effectOn(ev, x.No)
		want := b.damageCalc(virt, x)
		if eff == nil || eff.Result != "hit" || eff.Delta != -want {
			t.Errorf("单位 %d 的效果 = %+v，应为 hit、delta %d", x.No, eff, -want)
		}
	}
	for _, x := range b.units {
		if x != nil && x.Side() == b.PlayerUnit().Side() && effectOn(ev, x.No) != nil {
			t.Errorf("龙阵不应打到己方单位 %d", x.No)
		}
	}
}

// 强恩阵（395）的目标是敌方全体（原版实际效果）：和奔雷阵的全体打到同一批单位。
func TestDragonQiangEnHitsWholeSide(t *testing.T) {
	b := newBattleWithSide(t, magicSide(395), "win", rollRnd(9999))
	enemies := enemiesOf(b)
	ev := dragonCast(t, b, 395, 0) // 选哪一排都一样，这里随便给一个己方号也不影响
	if len(ev.Effects) != len(enemies) {
		t.Fatalf("强恩阵应打到敌方全体 %d 个，实际 %d 条", len(enemies), len(ev.Effects))
	}
}

// 奔龙阵打的是选定的一个：目标不是敌方、或已经倒下，就没有效果（提交时已校验，这里再确认）。
func TestDragonSingleNeedsLivingEnemy(t *testing.T) {
	b := newBattleWithSide(t, magicSide(394), "win", rollRnd(9999))
	if ev := dragonCast(t, b, 394, 0); len(ev.Effects) != 0 {
		t.Fatalf("选了己方不应有效果: %+v", ev.Effects)
	}
	enemy := enemiesOf(b)[0]
	enemy.Down = true
	if ev := dragonCast(t, b, 394, enemy.No); len(ev.Effects) != 0 {
		t.Fatalf("选了倒下的单位不应有效果: %+v", ev.Effects)
	}
}

// 被闪避的目标：伤害为 0，效果 result 为 dodge，血量不变，不挂任何东西。
func TestDragonDodgedTargetTakesNothing(t *testing.T) {
	// 点数 1 小于任何非零的闪避率：怪物等级 50 时闪避率是 10，点数 1 闪过。
	b := newBattleWithSide(t, magicSide(394), "win", rollRnd(0))
	enemy := enemiesOf(b)[0]
	enemy.Level = 50
	before := enemy.HP
	ev := dragonCast(t, b, 394, enemy.No)
	eff := effectOn(ev, enemy.No)
	if eff == nil || eff.Result != "dodge" || eff.Delta != 0 {
		t.Fatalf("闪过的效果 = %+v，应为 dodge、delta 0", eff)
	}
	if enemy.HP != before {
		t.Fatalf("闪过的目标血量不应变化：%d → %d", before, enemy.HP)
	}
}

// 龙阵不触发光（吸收）、镜子（反射）、守护（无效）：目标挂着防御次数也照常受伤，次数不减（STAGE-199）。
func TestDragonIgnoresDefenseCounters(t *testing.T) {
	b := newBattleWithSide(t, magicSide(394), "win", rollRnd(9999))
	enemy := enemiesOf(b)[0]
	tough(enemy)
	var ev Event
	b.defendUnit(enemy, "absorb", 1, &ev)
	want := b.damageCalc(&Unit{Kind: KindMonster, Attack: 300, Luck: 1}, enemy)
	before := enemy.HP
	dragon := dragonCast(t, b, 394, enemy.No)
	eff := effectOn(dragon, enemy.No)
	if eff == nil || eff.Result != "hit" || eff.Delta != -want {
		t.Fatalf("挂着吸收的目标仍应受到龙阵伤害，效果 = %+v", eff)
	}
	if enemy.HP != before-want {
		t.Fatalf("血量 = %d，应为 %d", enemy.HP, before-want)
	}
	if enemy.Def.Absorb != 1 {
		t.Fatalf("龙阵不应消耗吸收次数，剩 %d", enemy.Def.Absorb)
	}
}

// 经过提交和出招（castMagic）的召唤龙阵：扣气力（奔龙阵 40），事件是 magic、func 为 dragon，没有反击。
func TestDragonViaMagicCostsMPAndReturnsMagicEvent(t *testing.T) {
	b := newBattleWithSide(t, magicSide(394), "win", rollRnd(9999))
	you := b.PlayerUnit()
	enemy := enemiesOf(b)[0]
	tough(enemy)
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: enemy.No}); perr != nil {
		t.Fatalf("提交奔龙阵失败: %v", perr)
	}
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "magic" || evs[0].Magic == nil || evs[0].Magic.Func != gamedata.MagicFuncDragon {
		t.Fatalf("应只有一个 magic（func dragon）事件: %+v", evs)
	}
	if you.MP != 100-40 {
		t.Fatalf("气力应扣 40，实际 %d", you.MP)
	}
	if effectOn(evs[0], enemy.No) == nil {
		t.Fatalf("目标应有效果: %+v", evs[0].Effects)
	}
}
