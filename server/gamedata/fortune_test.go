package gamedata

import "testing"

// 出处：gmsv/char/char.c 的 lucktbl（40/30/20/7/3）、CHAR_AddCharm（0–100 夹取）。
func TestRollLuckBoundaries(t *testing.T) {
	cases := map[int]int{0: 5, 2: 5, 3: 4, 9: 4, 10: 3, 29: 3, 30: 2, 59: 2, 60: 1, 99: 1}
	for r, want := range cases {
		if got := RollLuck(r); got != want {
			t.Errorf("RollLuck(%d) = %d，want %d", r, got, want)
		}
	}
}

// 0–99 各掷一次，档位分布应当是 40%、30%、20%、7%、3%。
func TestRollLuckDistribution(t *testing.T) {
	count := map[int]int{}
	for r := 0; r < 100; r++ {
		count[RollLuck(r)]++
	}
	want := map[int]int{1: 40, 2: 30, 3: 20, 4: 7, 5: 3}
	for luck, n := range want {
		if count[luck] != n {
			t.Errorf("运气 %d 的次数 = %d，want %d", luck, count[luck], n)
		}
	}
}

func TestCharmClamp(t *testing.T) {
	if got := ClampCharm(-3); got != 0 {
		t.Errorf("ClampCharm(-3) = %d，want 0", got)
	}
	if got := ClampCharm(104); got != 100 {
		t.Errorf("ClampCharm(104) = %d，want 100", got)
	}
	if got := EffectiveCharm(98, 5); got != 100 {
		t.Errorf("EffectiveCharm(98, 5) = %d，want 100（装备加成夹在 100 以内）", got)
	}
	if got := EffectiveCharm(3, -10); got != 0 {
		t.Errorf("EffectiveCharm(3, -10) = %d，want 0", got)
	}
	if got := EffectiveCharm(60, 1); got != 61 {
		t.Errorf("EffectiveCharm(60, 1) = %d，want 61", got)
	}
}

func TestLuckEffectiveRange(t *testing.T) {
	if got := EffectiveLuck(0, 0); got != LuckMin {
		t.Errorf("还没掷过的运气 = %d，want %d", got, LuckMin)
	}
	if got := EffectiveLuck(5, 2); got != LuckMax {
		t.Errorf("EffectiveLuck(5, 2) = %d，want %d", got, LuckMax)
	}
	if got := EffectiveLuck(3, -1); got != 2 {
		t.Errorf("EffectiveLuck(3, -1) = %d，want 2", got)
	}
}

// 物品表里的魅力、运气修正列（tools/gen-itemset.py 的第 47、49 列）。
func TestItemCharmAndLuckColumns(t *testing.T) {
	gun, ok := ItemByID(200) // 小的枪：攻 +5 敏 -1 魅 +1
	if !ok {
		t.Fatal("找不到物品 200")
	}
	if gun.Charm != 1 || gun.Luck != 0 || gun.Attack != 5 || gun.Agility != -1 {
		t.Errorf("物品 200 = 魅 %d 运 %d 攻 %d 敏 %d，want 魅 1 运 0 攻 5 敏 -1", gun.Charm, gun.Luck, gun.Attack, gun.Agility)
	}
	axe, ok := ItemByID(0) // 小斧头：攻 +9 防 -3 敏 -3，没有魅力、运气修正
	if !ok {
		t.Fatal("找不到物品 0")
	}
	if axe.Charm != 0 || axe.Luck != 0 {
		t.Errorf("物品 0 的魅力、运气修正 = %d、%d，want 0、0", axe.Charm, axe.Luck)
	}
}
