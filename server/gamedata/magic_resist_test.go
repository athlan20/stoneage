// STAGE-228 魔抗系精灵的目标类型与效果文字：魔法防御单人是己方一个、Lv2 是己方全体；火、电、冰抗的文字写明原版未生效。

package gamedata

import "testing"

func TestResistSpellTargetAndText(t *testing.T) {
	cases := []struct {
		id     int
		target TargetType
		text   string
	}{
		{392, TargetAlly, "使己方一个魔抗提升 20%，持续 3 回合"},
		{393, TargetAllyAll, "使己方全体魔抗提升 20%，持续 3 回合"},
		{419, TargetAlly, "使己方一个火抗，持续 3 回合（原版未生效，只占状态槽）"},
		{420, TargetAllyAll, "使己方全体冰抗，持续 3 回合（原版未生效，只占状态槽）"},
	}
	for _, c := range cases {
		tpl, ok := MagicByID(c.id)
		if !ok {
			t.Fatalf("精灵 %d 应在精灵表里", c.id)
		}
		if got := tpl.BattleTarget(); got != c.target {
			t.Errorf("精灵 %d 目标类型应为 %v，实际 %v", c.id, c.target, got)
		}
		if got := tpl.Effect(); got != c.text {
			t.Errorf("精灵 %d 效果文字应为 %q，实际 %q", c.id, c.text, got)
		}
	}
}
