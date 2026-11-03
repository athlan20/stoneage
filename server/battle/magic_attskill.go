// 精灵版宠物技能（STAGE-202，原版 gmsv/magic/magic.c 的 MAGIC_AttSkill）：精灵的参数是“技能函数;技能编号”，
// 施放者是人物，效果就是 petskill.txt 那个编号的宠物技能（破除防御、突击、一击必杀、背水之战、不防守战法）。
// MP 由 castMagic 开头扣掉；这里交给 castSkill 结算，事件就是技能事件（kind skill），不另包 magic 事件。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// castAttSkill 执行精灵版宠物技能：目标用提交时选的那个（不防守战法没有目标）。
// 突击（蓄力）的待出手存在人物的 pend 里，之后回合由 act → resumePending 继续；背水之战、不防守战法的整回合效果
// 在 beginRound 里由 registerPlayerStance 登记。
func (b *Battle) castAttSkill(u *Unit, t gamedata.MagicTemplate, cmd Cmd) Event {
	skill, ok := t.AttSkill()
	if !ok {
		return skipEvent(u, "timeout")
	}
	return b.castSkill(u, skill, Cmd{Target: cmd.Target, Skill: 0})
}

// registerPlayerStance 登记真人 no 这回合交的精灵版宠物技能定下的整回合效果（背水之战的防%、不防守战法的回避和反击），
// 与宠物的同类效果一样与行动顺序无关（stanceFor）。人物没交精灵指令、或交的不是精灵版技能时什么也不做。
func (b *Battle) registerPlayerStance(no int) {
	u := b.humanOf(no)
	cmd := b.cmdOfHuman(no, "player")
	if u == nil || cmd == nil || cmd.Action != "magic" || !u.Alive() {
		return
	}
	t, ok := b.unitMagic(u, cmd.Magic)
	if !ok {
		return
	}
	if skill, ok := t.AttSkill(); ok {
		stanceFor(u, skill)
	}
}
