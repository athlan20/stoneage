// 回旋镖横扫的规则测试（STAGE-167）：一排最多 5 个、按视觉顺序单向扫过、双方方向相反、
// 每次命中独立、伤害乘 0.3、不碰另一排、空排改选。

package battle

import "testing"

// monsters 造 n 只容易被命中的怪物：闪避与暴击都不触发。
func monsters(n int) []*Unit {
	out := make([]*Unit, n)
	for i := range out {
		out[i] = &Unit{Kind: KindMonster, Name: "怪", Level: 1, HP: 200, MaxHP: 200, Attack: 5, Defense: 0, Agility: 0}
	}
	return out
}

// sameInts 比较两个整数序列（包内已有 equalInts 用于奖励测试，这里换个名字避免重名）。
func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// targetOrder 是事件里 effects 的目标顺序。
func targetOrder(ev Event) []int {
	out := make([]int, 0, len(ev.Effects))
	for _, eff := range ev.Effects {
		out = append(out, eff.Target)
	}
	return out
}

func boomerangBattle(t *testing.T) *Battle {
	t.Helper()
	side := testSide()
	side.WeaponType = 4 // 先随便给个弓，下面单独测回旋镖时覆盖
	b := New("b1", "field", side, monsters(5), fixedRnd(9999))
	b.units[0].WeaponMotion = motionBoomerang
	return b
}

// 我方出手打敌方宠物排（15–19）：顺序固定为 19→17→15→16→18，正好是需求给的那组回归样例。
func TestBoomerangSweepsTheTargetRowInVisualOrder(t *testing.T) {
	b := boomerangBattle(t)
	ev := b.boomerang(b.units[0], b.units[16]) // 目标在 15–19 这一排
	if ev.Kind != "attack" {
		t.Fatalf("回旋镖应当是一条 attack 事件，实际 %q", ev.Kind)
	}
	if ev.Weapon != motionBoomerang {
		t.Errorf("事件要带武器身份 boomerang，实际 %q", ev.Weapon)
	}
	if ev.Row == nil || *ev.Row != 3 {
		t.Fatalf("横排号应是 3（15–19），实际 %v", ev.Row)
	}
	want := []int{19, 17, 15, 16, 18}
	if got := targetOrder(ev); !sameInts(got, want) {
		t.Errorf("横扫顺序应是 %v，实际 %v", want, got)
	}
	for _, eff := range ev.Effects {
		if eff.Target < 15 || eff.Target > 19 {
			t.Errorf("只该打这一排（15–19），实际打了 %d", eff.Target)
		}
	}
}

// 敌方出手方向相反：同一排按编号反着扫。
func TestBoomerangReversesForTheEnemySide(t *testing.T) {
	b := boomerangBattle(t)
	enemy := b.units[16]
	enemy.WeaponMotion = motionBoomerang
	// 敌方打我方的宠物排（5–9）：反向，得到 8→6→5→7→9。
	for i := 5; i <= 9; i++ {
		if b.units[i] == nil {
			b.units[i] = &Unit{No: i, Kind: KindPet, Name: "我方宠", HP: 100, MaxHP: 100, Attack: 1}
		}
	}
	ev := b.boomerang(enemy, b.units[6])
	want := []int{8, 6, 5, 7, 9}
	if got := targetOrder(ev); !sameInts(got, want) {
		t.Errorf("敌方横扫顺序应是 %v，实际 %v", want, got)
	}
}

// 目标那一排只有 2 个活着的：只产生 2 条效果，空位不补受击。
func TestBoomerangSkipsEmptySlots(t *testing.T) {
	b := boomerangBattle(t)
	b.units[17].Down = true
	b.units[18].Down = true
	b.units[19].Down = true
	ev := b.boomerang(b.units[0], b.units[15])
	want := []int{15, 16}
	if got := targetOrder(ev); !sameInts(got, want) {
		t.Errorf("只剩 15、16 能打，实际 %v", got)
	}
}

// 回旋镖的每次命中都按原版系数打 0.3 折。
func TestBoomerangDamageIsScaled(t *testing.T) {
	if got := scaledBoomerangDamage(100); got != 30 {
		t.Errorf("100 点伤害应折成 30，实际 %d", got)
	}
	if got := scaledBoomerangDamage(2); got != 1 {
		t.Errorf("2 点伤害折算后至少 1 点，实际 %d", got)
	}
	if got := scaledBoomerangDamage(0); got != 0 {
		t.Errorf("没伤害时保持 0，实际 %d", got)
	}
}

// 打自己所在的那一排不出手（原版 attackNo/5 == defNo 时 BATTLE_NoAction）。
func TestBoomerangRefusesItsOwnRow(t *testing.T) {
	b := boomerangBattle(t)
	b.units[2] = &Unit{No: 2, Kind: KindPet, Name: "我方宠", HP: 100, MaxHP: 100}
	ev := b.boomerang(b.units[0], b.units[2]) // 0 和 2 都在 0–4 这一排
	if ev.Kind != "skip" {
		t.Fatalf("打自己那一排应当跳过，实际 %q", ev.Kind)
	}
}

// 目标排全空了：改选对方还有人的排；对方一个人都没有时跳过。
func TestBoomerangRetargetsWhenTheRowIsEmpty(t *testing.T) {
	b := boomerangBattle(t)
	for i := 15; i <= 19; i++ {
		b.units[i].Down = true
	}
	b.units[12] = &Unit{No: 12, Kind: KindMonster, Name: "后排怪", HP: 100, MaxHP: 100}
	ev := b.boomerang(b.units[0], b.units[16]) // 目标排在 15–19，但都倒下了
	if ev.Row == nil || *ev.Row != 2 {
		t.Fatalf("整排失效后应改到还有人的排（2），实际 %v", ev.Row)
	}
	if got := targetOrder(ev); !sameInts(got, []int{12}) {
		t.Errorf("改选后只该打到 12，实际 %v", got)
	}

	for i := 10; i <= 19; i++ {
		if b.units[i] != nil {
			b.units[i].Down = true
		}
	}
	if ev := b.boomerang(b.units[0], b.units[16]); ev.Kind != "skip" {
		t.Errorf("对方全灭时应跳过，实际 %q", ev.Kind)
	}
}

