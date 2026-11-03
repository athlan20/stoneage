// 怪物与人物防御指令的整回合登记（STAGE-291 批 B）。原版怪物在下指令时就调用 PETSKILL_* 写工作变量
// （BATTLE_ai 选中技能时），忠犬、背水之战、不防守战法整回合生效，与行动顺序无关；防御指令也在下指令时定下（COM1 == GUARD）。
// 新引擎在回合开始时看怪物本回合的计划（peekPlan）登记。计划只在第一次看时定下、之后 takePlan 取用，
// 所以这里提前定下不会改变怪物的行动。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// registerMonsterStance 登记怪物本回合计划里的技能效果与防御指令。逃跑轮（FleeTurn）之后的怪物只会逃走，不登记。
func (b *Battle) registerMonsterStance() {
	for _, u := range b.units {
		if u == nil || u.Kind != KindMonster || !u.Alive() {
			continue
		}
		if b.FleeTurn > 0 && b.turn >= b.FleeTurn {
			continue
		}
		p := b.peekPlan(u)
		if !p.ok {
			continue
		}
		switch p.act.kind {
		case "guard":
			u.guardRound = true
		case "skill":
			t, ok := gamedata.PetSkillByID(u.Skills[p.act.skill-1])
			if !ok {
				continue
			}
			stanceFor(u, t)
			if t.Func == "PETSKILL_NormalGuard" {
				u.guardRound = true
			}
			if t.Func == "PETSKILL_Guardian" { // 忠犬对怪物：替“格号 −5”的同伴挡（原版 pos − 5），只限同一方
				if ally := b.Unit(u.No - firstPetNo); ally != nil && ally.Side() == u.Side() {
					b.guards[u.No-firstPetNo] = u.No
				}
			}
		}
	}
}

// registerHumanGuard 登记真人 no 这回合交的防御指令（COM1 == GUARD）。
func (b *Battle) registerHumanGuard(no int) {
	u := b.humanOf(no)
	cmd := b.cmdOfHuman(no, "player")
	if u != nil && cmd != nil && cmd.Action == "guard" {
		u.guardRound = true
	}
}
