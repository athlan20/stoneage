package gamedata

import "testing"

// 出处：gmsv/char/char.c 的 CHAR_initcharWorkInt；验收口径见 docs/ui.md 角色页 A。
func TestDerivedStats(t *testing.T) {
	// 能力点 5/5/5/5（体力、腕力、耐久力、速度）：攻击 5+0.5+0.5+0.25=6.25→6，防御同理，敏捷 5，最大 HP 35。
	if got := Attack(5, 5, 5, 5); got != 6 {
		t.Errorf("Attack(5,5,5,5) = %d，want 6", got)
	}
	if got := Defense(5, 5, 5, 5); got != 6 {
		t.Errorf("Defense(5,5,5,5) = %d，want 6", got)
	}
	if got := Agility(5); got != 5 {
		t.Errorf("Agility(5) = %d，want 5", got)
	}
	if got := MaxHP(5, 5, 5, 5); got != 35 {
		t.Errorf("MaxHP(5,5,5,5) = %d，want 35", got)
	}
	// 体力 8、腕力 4、耐久力 4、速度 4：攻击 4+0.4+0.8+0.2=5.4→5，防御 4+0.4+0.8+0.2=5.4→5。
	if got := Attack(8, 4, 4, 4); got != 5 {
		t.Errorf("Attack(8,4,4,4) = %d，want 5", got)
	}
	if got := Defense(8, 4, 4, 4); got != 5 {
		t.Errorf("Defense(8,4,4,4) = %d，want 5", got)
	}
	// 全 0 时都是 0；满点 20（体力 20、腕力 0、耐久 0、速度 0）时攻击只有体力的 0.1 倍 = 2。
	if Attack(0, 0, 0, 0) != 0 || Defense(0, 0, 0, 0) != 0 {
		t.Error("全 0 的衍生数值应为 0")
	}
	if got := Attack(20, 0, 0, 0); got != 2 {
		t.Errorf("Attack(20,0,0,0) = %d，want 2", got)
	}
}
