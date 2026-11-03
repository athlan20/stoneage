// 攻击精灵的 FIX 伤害公式（STAGE-227）：E1–E4 是设计稿第 3.2 节的手算例子（rand 取 0），E5 是熟练度 0 的情形。
// 手算过程见 docs/magic-resist.md 第 3.2 节。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// fixFixture 造一场人物对一只怪的仗：人物没有属性、火熟练度 exp，怪物没有属性、火抗 resist，rnd 固定返回 r。
func fixFixture(t *testing.T, exp, resist int, r int) (*Battle, *Unit, *Unit) {
	t.Helper()
	mon := &Unit{Kind: KindMonster, Name: "乌力", Level: 1, HP: 100, MaxHP: 100, Attack: 10}
	b := New("b_fix", "test", testSide(), []*Unit{mon}, func(int) int { return r })
	you := b.PlayerUnit()
	you.Elem = Elements{}
	you.MagicExp[2] = exp
	mon.Elem = Elements{}
	mon.MagicResist[2] = resist
	return b, you, mon
}

// fireSpell 是火系攻击精灵，威力 power、等级 lv。
func fireSpell(power, lv int) gamedata.MagicTemplate {
	return gamedata.MagicTemplate{Func: gamedata.MagicFuncAttack, Elem: "火", Power: power, Lv: lv}
}

// fixDamage 是 magicPower 与 magicAdjust 串起来的伤害（FIX 分支，不含失败 ×0.7）。
func fixDamage(b *Battle, you, mon *Unit, t gamedata.MagicTemplate) (apower, dmg int) {
	apower = b.magicPower(you, mon, t, 2)
	return apower, b.magicAdjust(you, mon, 2, t.Lv, apower)
}

func TestFixDamageE1(t *testing.T) {
	b, you, mon := fixFixture(t, 50, 10, 0)
	if ap, dmg := fixDamage(b, you, mon, fireSpell(100, 1)); ap != 158 || dmg != 181 {
		t.Fatalf("E1 火球：APower %d、伤害 %d，应为 158、181", ap, dmg)
	}
}

func TestFixDamageE2(t *testing.T) {
	b, you, mon := fixFixture(t, 80, 30, 0)
	if ap, dmg := fixDamage(b, you, mon, fireSpell(150, 2)); ap != 189 || dmg != 245 {
		t.Fatalf("E2 火焰连弹：APower %d、伤害 %d，应为 189、245", ap, dmg)
	}
}

func TestFixDamageE3ElementFromCaster(t *testing.T) {
	b, you, mon := fixFixture(t, 60, 0, 0)
	you.Elem = Elements{Fire: 40} // 施法者火 40、无属性 60：精灵属性换成 等级×10×(1+40/50)，无属性 60 保留
	mon.Elem = Elements{Water: 30}
	if ap, dmg := fixDamage(b, you, mon, fireSpell(200, 3)); ap != 509 || dmg != 456 {
		t.Fatalf("E3 熔岩爆发：APower %d、伤害 %d，应为 509、456", ap, dmg)
	}
}

// E4：E2 的守方火抗从 30 变成 36（魔抗状态未做，这里只验数值）。
func TestFixDamageE4ResistUp(t *testing.T) {
	b, you, mon := fixFixture(t, 80, 36, 0)
	if _, dmg := fixDamage(b, you, mon, fireSpell(150, 2)); dmg != 210 {
		t.Fatalf("E4：火抗 36 时伤害 %d，应为 210", dmg)
	}
}

// E5：熟练度 0、随机项取最大（rand%20 = 19，施法失败）：A 只剩随机项，伤害 ×0.7。
func TestFixDamageE5ZeroExpFailure(t *testing.T) {
	b, you, mon := fixFixture(t, 0, 0, 19)
	ap, dmg := fixDamage(b, you, mon, fireSpell(100, 1))
	if ap != 20 || dmg != 23 {
		t.Fatalf("E5：APower %d、伤害 %d，应为 20、23", ap, dmg)
	}
	if int(float64(dmg)*0.7) != 16 {
		t.Fatalf("E5：失败 ×0.7 后应为 16")
	}
}

func TestMagicComputeAttExpTruncatesAdjacent(t *testing.T) {
	var u Unit
	u.MagicExp = [4]int{0, 0, 50, 5}
	// 火（2）攻击命中 1 次，等级 1：自己加 3；相克的风（3）减 int(0 - 1.5) = -1 → 经验为负，降一级。
	magicComputeAttExp(&u, 2, 1, 1)
	if u.MagicAttExp[2] != 3 || u.MagicExp[2] != 50 {
		t.Fatalf("攻击元素：经验 %d、等级 %d，应为 3、50", u.MagicAttExp[2], u.MagicExp[2])
	}
	if u.MagicExp[3] != 4 || u.MagicAttExp[3] != 0 {
		t.Fatalf("相克元素：等级 %d、经验 %d，应为 4、0", u.MagicExp[3], u.MagicAttExp[3])
	}
}

func TestMagicComputeAttExpLevelsUp(t *testing.T) {
	var u Unit
	u.MagicExp[2] = 50
	u.MagicAttExp[2] = 98
	magicComputeAttExp(&u, 2, 1, 1) // 98 + 3 = 101 > 100：升一级，经验清零
	if u.MagicExp[2] != 51 || u.MagicAttExp[2] != 0 {
		t.Fatalf("升级：等级 %d、经验 %d，应为 51、0", u.MagicExp[2], u.MagicAttExp[2])
	}
}

func TestMagicComputeDefExpAndEquipment(t *testing.T) {
	var u Unit
	u.MagicResist = [4]int{0, 0, 30, 5} // 火抗 30 里 10 是装备
	u.magicEq = [4]int{0, 0, 10, 0}
	u.MagicDefExp[2] = 90
	// 伤害 250：加 (250/20)×(1×2) = 24，90+24 > 100，基础火抗 20 → 21，装备仍在，合计 31。
	magicComputeDefExp(&u, 2, 1, 250)
	if u.MagicResist[2] != 31 || u.MagicDefExp[2] != 0 {
		t.Fatalf("抗性升级：抗性 %d、经验 %d，应为 31、0", u.MagicResist[2], u.MagicDefExp[2])
	}
	// 相克的风（3）：经验 1 - 2 < 0，降一级并把经验置 90。
	if u.MagicResist[3] != 4 || u.MagicDefExp[3] != 90 {
		t.Fatalf("相克：抗性 %d、经验 %d，应为 4、90", u.MagicResist[3], u.MagicDefExp[3])
	}
}

func TestMagicComputeDefExpSkipsSmallHit(t *testing.T) {
	var u Unit
	u.MagicDefExp[2] = 99
	magicComputeDefExp(&u, 2, 5, 199)
	if u.MagicDefExp[2] != 99 || u.MagicResist[2] != 0 {
		t.Fatalf("伤害不到 200 不该成长：经验 %d、抗性 %d", u.MagicDefExp[2], u.MagicResist[2])
	}
}

// 施法失败的攻击：攻击熟练度涨（只涨一次，按命中数）。成功的施放不涨。
func TestAttackSpellGrowsAttExpOnFailure(t *testing.T) {
	spell, ok := gamedata.MagicByID(313) // 火球：敌方一个，命中 1 个格（magic_range_test.go）
	if !ok {
		t.Fatal("火球不在精灵表里")
	}
	// 掷出 99，熟练度 0：施法失败，命中 1 个目标，攻击熟练度经验加 3×等级。
	b, you := fullEnemyFixture(t, 0, 99)
	b.castMagic(you, spell, Cmd{Target: 17})
	if you.MagicAttExp[2] != 3*spell.Lv {
		t.Fatalf("失败施放后攻击熟练度经验 %d，应为 %d", you.MagicAttExp[2], 3*spell.Lv)
	}
	// 掷 0，熟练度 100：施法成功，攻击熟练度不涨。
	b2, you2 := fullEnemyFixture(t, 100, 0)
	b2.castMagic(you2, spell, Cmd{Target: 17})
	if you2.MagicAttExp[2] != 0 {
		t.Fatalf("成功施放不该涨攻击熟练度，实际 %d", you2.MagicAttExp[2])
	}
}

// fullEnemyFixture 是满员敌方（同 magic_range_test.go）的仗，人物火熟练 exp，rnd 固定返回 r。
func fullEnemyFixture(t *testing.T, exp, r int) (*Battle, *Unit) {
	t.Helper()
	var monsters []*Unit
	for i := 0; i < 10; i++ {
		monsters = append(monsters, &Unit{Kind: KindMonster, Name: "乌力", Level: 1, HP: 100, MaxHP: 100, Attack: 10})
	}
	b := New("b_fix_grow", "test", testSide(), monsters, func(int) int { return r })
	you := b.PlayerUnit()
	you.Elem = Elements{}
	you.MagicExp[2] = exp
	return b, you
}
