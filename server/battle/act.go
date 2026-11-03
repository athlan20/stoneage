// 行动的分派（STAGE-127 从 resolve.go 拆出）：人物、宠物的指令按类型执行（act / doCmd），
// 怪物按战术串行动（monsterAct，STAGE-60）。原版对应 gmsv/battle/battle.c 的 BATTLE_ProcessAttack 系列。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// act 执行一个单位本回合的行动，返回产生的事件。宠物先过听话判定（loyalty.go）：不听话时先给 disobey 事件，
// 再执行改写后的指令。
func (b *Battle) act(u *Unit) []Event {
	b.lastAtk = atkRec{} // STAGE-292：每次出手重新记攻击结果，反击循环只看这一下
	u.actedRound = true
	if u.pend != nil { // 蓄力或隐身中：不用再出招，按之前定下的继续（STAGE-76）
		return append(b.resumePending(u), b.strikeCounter(u, false)...) // 突击、地球一周的释放
	}
	if u.Kind == KindMonster {
		return b.monsterAct(u)
	}
	if u.Auto { // 分身队的单位：服务端自动出招（STAGE-162，auto.go）
		return b.autoAct(u)
	}
	cmd := b.commandOf(u)
	if cmd == nil { // 超时没交：按默认动作跳过
		return []Event{skipEvent(u, "timeout")}
	}
	var pre []Event
	if u.Kind == KindPet {
		plan := b.obey(u)
		cmd = plan.cmd
		if plan.mode != loyalNormal {
			if plan.mode == loyalRandom { // 乱来：清掉忠犬标记，这回合不再替主人挡刀（loyalty.go）
				b.clearGuardian(u)
			}
			pre = append(pre, disobeyEvent(u, plan.mode))
		}
	}
	if cmd.Action != "escape" { // 逃跑不算出手（逃跑的宠物不计击杀，STAGE-150）
		b.markActed(u)
	}
	u.Status.Guard = false // 行动开始时防御解除（防御只保护到这次行动）
	return append(pre, b.doCmd(u, cmd)...)
}

// doCmd 执行单位的一条指令（人物、宠物的出招），返回产生的事件。
func (b *Battle) doCmd(u *Unit, cmd *Cmd) []Event {
	switch cmd.Action {
	case "attack":
		target := b.Unit(cmd.Target)
		if u.Status.Confusion > 0 { // 混乱中目标随机（可能打自己人）
			target = b.confuseTarget(u)
		}
		// 回旋镖横扫目标所在的横排（STAGE-167），不走单击；投掷武器本来就不触发反击，
		// 所以这里不需要再补 counter。
		if u.isBoomerang() {
			return []Event{b.boomerang(u, target)}
		}
		// 攻击结算后进反击循环（STAGE-292，counter_chain.go）
		return append([]Event{b.attack(u, target)}, b.strikeCounter(u, true)...)
	case "guard":
		u.Status.Guard = true
		return []Event{b.selfEffect(u, "guard", "none", 0, []string{"guard"})}
	case "escape":
		return []Event{b.escape(u)}
	case "capture":
		target := b.Unit(cmd.Target)
		if target == nil { // 目标在同一回合里先离场了（逃走、被击倒），捕捉失败而不是崩溃（STAGE-275 第三轮）
			no, t, ok := u.No, cmd.Target, false
			return []Event{{Kind: "capture", Actor: &no, Target: &t, OK: &ok}}
		}
		return []Event{b.capture(u, target, cmd.Keep)}
	case "swap_pet":
		return b.swapPet(b.actorNo(u), *cmd)
	case "skill":
		if t, ok := gamedata.PetSkillByID(u.Skills[cmd.Skill-1]); ok {
			return append([]Event{b.castSkill(u, t, *cmd)}, b.skillCounter(u, t.Func)...)
		}
		return []Event{skipEvent(u, "timeout")}
	case "magic":
		if t, ok := b.unitMagic(u, cmd.Magic); ok {
			return b.castMagic(u, t, *cmd)
		}
		return []Event{skipEvent(u, "timeout")}
	case "item":
		return []Event{b.useItem(u, *cmd)}
	case "none": // 不听话的宠物这回合不出手（loyalty.go）
		return []Event{skipEvent(u, "disobey")}
	case "lost_escape": // 不听话的宠物跑掉，人物不跑（loyalty.go）
		return []Event{b.petLostEscape(u)}
	default: // 未知动作：提交时已校验，防御性按跳过处理
		return []Event{skipEvent(u, "timeout")}
	}
}

// monsterAct 决定并执行怪物的行动：按遇敌表的战术串抽模式与目标
// （gmsv/battle/battle_ai.c 的 BATTLE_ai_normal，STAGE-60）；
// 剧本指定 FleeTurn 时那一回合全体怪物逃走（BATTLE 的 LOSTESCAPE，测试用）。
func (b *Battle) monsterAct(u *Unit) []Event {
	if b.FleeTurn > 0 && b.turn >= b.FleeTurn {
		no := u.No
		ev := Event{Kind: "escape", Actor: &no, OK: &ptrTrue}
		ev.Left = append(ev.Left, u.No)
		b.leave(u)
		return []Event{ev}
	}
	if !u.Status.CanAct() {
		return []Event{skipEvent(u, skipReason(u.Status))}
	}
	act, ok, cached := b.takePlan(u)
	if !ok { // 战术串为空、抽中精灵模式或没有可选目标：不出招（BATTLE_COM_NONE）
		return []Event{skipEvent(u, "timeout")}
	}
	switch act.kind {
	case "attack":
		target := b.Unit(act.target)
		if u.Status.Confusion > 0 { // 混乱中目标随机，同人物和宠物的 attack（精灵能让怪物混乱，STAGE-77）
			target = b.confuseTarget(u)
		} else if cached && (target == nil || !target.targetable()) {
			target = b.adjustTarget(u, target) // 合击检查时提前定的目标已倒下：同原版改选（BATTLE_TargetAdjust）
		}
		if target == nil || !target.Alive() {
			return []Event{skipEvent(u, "timeout")}
		}
		// 怪物出手后同样进反击循环（STAGE-292）：反击时看它这回合的行动（canCounter），
		// 怪物的行动在它出手时才执行，没出手的按提前定好的计划判断。
		return append([]Event{b.attack(u, target)}, b.strikeCounter(u, true)...)
	case "guard":
		u.Status.Guard = true
		return []Event{b.selfEffect(u, "guard", "none", 0, []string{"guard"})}
	case "escape":
		return []Event{b.monsterEscape(u)}
	case "skill":
		if t, ok := gamedata.PetSkillByID(u.Skills[act.skill-1]); ok {
			return append([]Event{b.castSkill(u, t, Cmd{Target: act.target, Skill: act.skill})}, b.skillCounter(u, t.Func)...)
		}
	}
	return []Event{skipEvent(u, "timeout")}
}
