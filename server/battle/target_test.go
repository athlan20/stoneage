package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// targetBattle 是一场 no_escape 战斗，人物的精灵栏位和宠物的技能栏位放上每种目标类型的代表：
// 精灵 0 攻击敌方一个（岩石撞击）、1 自愈、2 治疗己方一个、3 攻击敌方全体；宠物技能 1 攻击、2 待机、3 加工（只能战斗外用）。
func targetBattle(t *testing.T) *Battle {
	t.Helper()
	b := newTestBattle(t, "no_escape", fixedRnd(1, 2, 3, 4))
	b.sides[0].Magics[1] = 0    // 治愈的精灵 Lv1：自愈
	b.sides[0].Magics[2] = 10   // 滋润的精灵 Lv1：治疗己方一个
	b.sides[0].Magics[3] = 305  // 地震：攻击敌方全体
	b.PetUnit().Skills[1] = 0   // 待机
	b.PetUnit().Skills[2] = 200 // 加工：只能在战斗外用
	return b
}

// TestMagicAndSkillTargetsFollowTheirType：精灵和宠物技能按目标类型决定要不要 target，需要时只能选合法的一方
// （docs/protocol.md 10.5）。每条用例都从头开一场战斗，互不影响。
func TestMagicAndSkillTargetsFollowTheirType(t *testing.T) {
	cases := []struct {
		name   string
		actor  string
		action string
		cmd    Cmd
		prep   func(*Battle)
		want   string // 期望的错误码，空表示提交成功
	}{
		{name: "攻击精灵选敌方", actor: "player", action: "magic", cmd: Cmd{Magic: 0, Target: 15}},
		{name: "攻击精灵缺少目标", actor: "player", action: "magic", cmd: Cmd{Magic: 0, Target: -1}, want: protocol.CodeBadRequest},
		{name: "攻击精灵选己方的宠物", actor: "player", action: "magic", cmd: Cmd{Magic: 0, Target: 5}, want: protocol.CodeInvalidTarget},
		{name: "攻击精灵选已倒下的敌方", actor: "player", action: "magic", cmd: Cmd{Magic: 0, Target: 15},
			prep: func(b *Battle) { b.Unit(15).Down = true }, want: protocol.CodeInvalidTarget},
		{name: "自愈不需要目标", actor: "player", action: "magic", cmd: Cmd{Magic: 1}},
		{name: "自愈带了敌方也只治自己", actor: "player", action: "magic", cmd: Cmd{Magic: 1, Target: 15}},
		{name: "治疗他人选己方", actor: "player", action: "magic", cmd: Cmd{Magic: 2, Target: 5}},
		{name: "治疗他人选敌方是错的", actor: "player", action: "magic", cmd: Cmd{Magic: 2, Target: 15}, want: protocol.CodeInvalidTarget},
		{name: "治疗他人缺少目标落在自己身上", actor: "player", action: "magic", cmd: Cmd{Magic: 2, Target: -1}},
		{name: "全体精灵不带目标", actor: "player", action: "magic", cmd: Cmd{Magic: 3}},
		{name: "复活选倒下的己方宠物", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: 5},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 50; b.PetUnit().Down = true }},
		{name: "复活选活着的己方是错的", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: 5},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 50 }, want: protocol.CodeInvalidTarget},
		{name: "复活选倒下的敌方是错的", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: 15},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 50; b.Unit(15).Down = true }, want: protocol.CodeInvalidTarget},
		{name: "复活缺少目标", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: -1},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 50; b.PetUnit().Down = true }, want: protocol.CodeBadRequest},
		{name: "解状态缺少目标落在自己身上", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: -1},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 60 }},
		{name: "解状态选敌方是错的", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: 15},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 60 }, want: protocol.CodeInvalidTarget},
		{name: "状态精灵选己方是错的", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: 5},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 130 }, want: protocol.CodeInvalidTarget},
		{name: "状态精灵选敌方", actor: "player", action: "magic", cmd: Cmd{Magic: 4, Target: 15},
			prep: func(b *Battle) { b.sides[0].Magics[4] = 130 }},
		{name: "宠物攻击选敌方", actor: "pet", action: "skill", cmd: Cmd{Skill: 1, Target: 15}},
		{name: "宠物攻击选己方是错的", actor: "pet", action: "skill", cmd: Cmd{Skill: 1, Target: 0}, want: protocol.CodeInvalidTarget},
		{name: "宠物攻击缺少目标", actor: "pet", action: "skill", cmd: Cmd{Skill: 1, Target: -1}, want: protocol.CodeBadRequest},
		{name: "待机不需要目标", actor: "pet", action: "skill", cmd: Cmd{Skill: 2}},
		{name: "加工只能在战斗外用", actor: "pet", action: "skill", cmd: Cmd{Skill: 3}, want: protocol.CodeSkillUnusable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := targetBattle(t)
			if tc.prep != nil {
				tc.prep(b)
			}
			if tc.actor == "pet" { // 宠物的指令排在人物之后
				if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
					t.Fatalf("人物防御提交失败: %v", perr)
				}
			}
			_, perr := b.Submit(b.Turn(), tc.actor, tc.action, tc.cmd)
			got := ""
			if perr != nil {
				got = perr.Code
			}
			if got != tc.want {
				t.Fatalf("错误码 %q，want %q（%v）", got, tc.want, perr)
			}
		})
	}
}
