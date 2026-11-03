// 分身队的战斗部分（STAGE-162，docs/protocol.md 5.57–5.59）：队长带着最多 4 个分身进战斗，分身和它们的宠物
// 占己方 1–4 号与 6–9 号格子（主人和宠物同一列，宠物号 = 主人号 + 5），由服务端自动出招，
// 不需要玩家逐个下指令，也不进 needs，所以队长交完指令就能结算。
//
// 分身只是帮手：不领奖励（奖励按队长单人身份结算，rewards.go 只看队长和队长的宠物），不记宠物击杀数，
// 不涨不跌宠物忠诚（loyalty.go 的 addLoyalty 对分身的宠物无效，分身倒下也不扣队长宠物的忠诚），
// 倒下不影响战斗结束判定（sideGone 不数分身：队长倒下就算己方落败，STAGE-169 起宠物不再续命）。

package battle

// MaxClones 是一支分身队最多的分身数。
const MaxClones = 4

// AddClones 把分身放进己方阵容：第 i 个分身占 1+i 号，它出战的宠物占 6+i 号。
// 只在战斗刚建好、还没推 battle_start 的时候调用；clones 里的快照不改变队长的数据，每场战斗都从快照重新生成，
// 所以战后分身自然是开战前的状态（character 服务再把快照恢复满血）。
func (b *Battle) AddClones(clones []*SideInput) {
	for i, in := range clones {
		if i >= MaxClones {
			break
		}
		no := 1 + i
		u := humanUnit(in, no)
		u.Auto = true
		u.Magics = in.Magics // STAGE-195：分身带着队长的精灵，方案里的精灵动作才用得上
		b.units[no] = u
		if p := in.BattlePet; p != nil {
			pu := petUnit(p, firstPetNo+no)
			pu.Auto = true
			b.units[pu.No] = pu
		}
	}
	b.planOptional() // 第 1 回合的可选指令（New 里的 nextTurn 那时分身还没上场）
}

// planAuto 在结算开始时给每个能行动的分身和分身的宠物定下这一回合的指令：玩家指定了目标的（STAGE-186，
// clone_cmd.go）按指定的打；队长交了方案的（STAGE-195，auto_plan.go）按方案出招；其余是普通攻击合击顺序里的
// 第一个敌人（STAGE-312，默认的“人物攻击加宠物攻击”，不再各自随机）。按单位号从小到大依次定，顺序固定；
// 睡着、石化、麻痹的这回合不出手。
func (b *Battle) planAuto() {
	b.autoCmds = map[int]*Cmd{}
	for _, u := range b.units {
		if u == nil || !u.Auto || !u.Alive() || !u.Status.CanAct() {
			continue
		}
		if c := b.cloneCmds[u.No]; c != nil { // 玩家指定了目标（STAGE-186）：指定优先
			b.autoCmds[u.No] = &Cmd{Action: "attack", Target: c.Target}
			continue
		}
		if b.plan != nil { // 有方案就以方案为准（STAGE-195）：planFromProfile 总会给出指令，下面的默认普攻只给没有方案的
			if c := b.planFromProfile(u); c != nil {
				b.autoCmds[u.No] = c
				continue
			}
		}
		if t := b.defaultEnemy(u); t != nil {
			b.autoCmds[u.No] = &Cmd{Action: "attack", Target: t.No}
		}
	}
}

// autoAct 执行分身（或分身宠物）的行动。定好的目标这时已经倒下就改选一个（同原版 BATTLE_TargetAdjust，
// STAGE-312 起按合击顺序顺延），对方一个都没有了就跳过。
func (b *Battle) autoAct(u *Unit) []Event {
	cmd := b.autoCmds[u.No]
	if cmd == nil {
		return []Event{skipEvent(u, skipReason(u.Status))}
	}
	next := *cmd
	if b.autoRetarget(u, cmd) {
		t := b.adjustTarget(u, b.Unit(cmd.Target))
		if t == nil {
			return []Event{skipEvent(u, "timeout")}
		}
		next.Target = t.No
	}
	u.Status.Guard = false // 行动开始时防御解除，同 act
	return b.doCmd(u, &next)
}

// autoRetarget 判断分身的指令在行动时要不要重新选目标（STAGE-195）：普通攻击一律要（和 STAGE-162 一样，
// 目标缺失也改选）；其余指令只有指向对方单位时才要，治疗、防御和不要目标的不改。
func (b *Battle) autoRetarget(u *Unit, cmd *Cmd) bool {
	if cmd.Action == "attack" {
		return true
	}
	if cmd.Target < 0 {
		return false
	}
	t := b.Unit(cmd.Target)
	return t == nil || t.Side() != u.Side()
}

// cmdOf 是单位这回合的指令：队长和队长的宠物取玩家交的，分身和分身的宠物取自动定的；怪物没有指令，返回 nil。
func (b *Battle) cmdOf(u *Unit) *Cmd {
	switch {
	case u.Auto:
		return b.autoCmds[u.No]
	case b.isMateUnit(u): // 真人同场的队员（人物和宠物）取他自己交的指令
		return b.mateCmds[u.No]
	case u.Kind == KindPet:
		return b.cmds["pet"]
	case u.Kind == KindPlayer:
		return b.cmds["player"]
	}
	return nil
}
