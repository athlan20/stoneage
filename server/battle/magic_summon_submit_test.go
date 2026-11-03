package battle

import "testing"

// 召唤龙阵与精灵版技能的提交校验（STAGE-202）：目标按 BattleTarget() 校验。奔龙阵必须选一个活着的敌方，
// 强恩阵、奔雷阵、GM 烟火与不防守战法不带目标，精灵版技能（破除防御）按技能的目标类型选敌方。
func TestSummonSubmitTargetRules(t *testing.T) {
	cases := []struct {
		name   string
		id     int // 精灵编号（放在栏位 1）
		target int
		want   string // 错误码，"" 表示接受
	}{
		{"奔龙阵缺目标", 394, -1, "BAD_REQUEST"},
		{"奔龙阵选自己", 394, 0, "INVALID_TARGET"},
		{"奔龙阵选敌方", 394, 15, ""},
		{"强恩阵不用选", 395, -1, ""},
		{"奔雷阵不用选", 396, -1, ""},
		{"GM 烟火不用选", 399, -1, ""},
		{"破除防御选敌方", 326, 15, ""},
		{"破除防御选己方", 326, 0, "INVALID_TARGET"},
		{"不防守战法不用选", 330, -1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBattleWithSide(t, magicSide(tc.id), "win", rollRnd(9999))
			_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: tc.target})
			expectCode(t, perr, tc.want)
		})
	}
}
