// 行动的结果（STAGE-76 起从 rules.go 拆出）：反击、逃跑、捕捉、换宠，以及把结果写进事件的 effect / hurt。
// 反击的判定依据原版 gmsv/battle/battle_event.c 的 BATTLE_Counter 系列，逃跑与捕捉见 battle.c。

package battle

// counter 执行一次反击（BATTLE_Counter）：反击方此刻能反击（canCounter）且判定过了，才打一下，伤害打 75 折。
// 返回的事件由调用方接在攻击事件后面。fired 为假表示没有反击（循环到此结束）；
// cont 为真表示这一下让连环继续（普通命中或被闪开），会心、挡下、打空都结束连环。
func (b *Battle) counter(counter, target *Unit) (ev Event, fired, cont bool) {
	if !counter.Alive() || !target.Alive() || !b.canCounter(counter) || !b.counterCheck(counter, target) {
		return Event{}, false, false
	}
	no := counter.No
	ev = Event{Kind: "counter", Actor: &no}
	dmg, result, hit := b.attackSeq(counter, target, true)
	switch result {
	case "dodge":
		b.effect(&ev, hit, "dodge", 0)
		return ev, true, true
	case "miss", "block":
		b.effect(&ev, hit, "miss", 0)
		return ev, true, false
	}
	if dmg < 1 {
		dmg = 1
	}
	b.physHurt(counter, hit, dmg, result, &ev) // 反击也是物理直接攻击，过防御结算（STAGE-199）
	return ev, true, result == "hit"
}

// escape 执行逃跑（BATTLE_EscapeCheck：概率按运气档位和等级差算，每失败一次
// 再跑的机会更大）。逃跑成功时人物和宠物都离场，战斗结束为 escaped。
func (b *Battle) escape(u *Unit) Event {
	no := u.No
	ok := b.duel != nil || b.escapeCheck(u) // 决斗里逃跑必定成功（pk.md 2.1）
	ev := Event{Kind: "escape", Actor: &no, OK: &ok}
	if ok && b.party { // 真人同场：逃跑是个人行为，只离场自己和宠物，战斗继续（party_leave.go，10.14）
		ev.Left = b.depart(u.No, "escape").Left
		return ev
	}
	if ok {
		ev.Left = append(ev.Left, u.No)
		if pet := b.commandPet(); pet != nil {
			ev.Left = append(ev.Left, pet.No)
		}
		for _, x := range b.units { // 队长带队逃跑：分身和它们的宠物一起离场（STAGE-162）
			if x != nil && x.Auto && x.Side() == u.Side() {
				ev.Left = append(ev.Left, x.No)
			}
		}
		b.finishEscape(u)
	}
	return ev
}

// monsterEscape 是怪物自己选择逃跑（AI 的 escape）：只有这一只怪物离场，人物、宠物和其他单位都不动，
// 战斗不因此结束；敌人全部逃走时由 finish 判 draw（协议 10.9）。与 escape（玩家逃跑）分开，免得把我方宠物也带走（STAGE-176）。
func (b *Battle) monsterEscape(u *Unit) Event {
	no := u.No
	ok := b.escapeCheck(u)
	ev := Event{Kind: "escape", Actor: &no, OK: &ok}
	if ok {
		ev.Left = []int{u.No}
		b.leave(u)
	}
	return ev
}

// escapeCheck 是逃跑的成败判定（BATTLE_EscapeCheck）。运气（人物的有效运气 1–5 档，STAGE-61）
// 决定基础概率 30%–95%，敌方平均等级每比我高一级减 2%，低一级加 2%；
// 同一场战斗里每多试一次基础值翻一倍地累加。
func (b *Battle) escapeCheck(u *Unit) bool {
	tries := b.escapeTries[u.No] + 1
	b.escapeTries[u.No] = tries
	luck := u.Luck
	if luck < 1 {
		luck = 1
	}
	if luck > 5 {
		luck = 5
	}
	base := []int{0, 30, 40, 50, 60, 95}[luck]
	esc := 100 // 敌方没有人时必成（BATTLE_EscapeCheck 的 enemycnt <= 0 分支）
	sum, cnt := 0, 0
	for _, t := range b.units {
		if t != nil && t.Side() != u.Side() {
			sum += t.Level
			cnt++
		}
	}
	if cnt > 0 {
		esc = base*tries - 2*(sum/cnt-u.Level)
	}
	if esc < 1 {
		esc = 1
	}
	return b.roll(100) < esc
}

// capture 执行捕捉：概率按 BATTLE_CaptureCheck——血量越少越好抓、等级差和敏捷差
// 修正、怪物自身的捕捉难度（enemybase 的 GET）、人物的魅力和运气；睡着的怪加 15%。
// 敌人等级比我高 5 级以上时根本不能抓（提交时就被拒）。
func (b *Battle) capture(u *Unit, target *Unit, keep *CaptureKeep) Event {
	no, t := u.No, target.No
	ok := b.captureCheck(u, target)
	ev := Event{Kind: "capture", Actor: &no, Target: &t, OK: &ok}
	if !ok || target == nil || !target.Alive() {
		return ev
	}
	if place := b.placeCaptured(u); place != nil {
		slot, discarded := place(target, keep)
		if slot < 0 && discarded == nil {
			// 宠位已满：原版 BATTLE_Capture 里 PET_createPetFromCharaIndex 返回 −1 时 flg = 0，
			// 判抓取失败，怪物留在场上（STAGE-267）。
			ok = false
			return ev
		}
		ev.Left = append(ev.Left, target.No)
		if discarded != nil { // 资质不合格：算抓成功（目标离场），但不入库、不进 rewards.pets
			ev.Discarded = discarded
			b.leave(target)
			return ev
		}
		ev.PetSlot = intPtr(slot)
		target.PetSlot = slot // rewards.pets 要报它进了哪个栏位
	} else {
		ev.Left = append(ev.Left, target.No)
	}
	b.captur = append(b.captur, target)
	if b.party {
		b.captors[target] = u.No
	}
	b.leave(target)
	return ev
}

// captureCheck 是捕捉的成败判定（BATTLE_CaptureCheck）。
func (b *Battle) captureCheck(u, target *Unit) bool {
	hpPer := 10 - target.HP*target.HP/max(target.MaxHP, 1)
	level := u.Level/2 - target.Level/2
	dex := int(u.agilityAct())/15 - target.Agility/15
	per := (float64(hpPer) + float64(level) + float64(dex) +
		float64(target.CaptureRate+u.Luck)) * float64(u.Charm) / 50
	if target.Status.Sleep > 0 {
		per += 15
	}
	if per > 99 {
		per = 99
	}
	return float64(b.roll(100)) < per
}

// swapPet 执行换宠：旧宠物离场，新宠物上场（原版规则在 STAGE-47 展开）。
// owner 是换宠的真人（人物单位号，真人同场时是队员号），用他自己的宠物栏位和快照。
func (b *Battle) swapPet(owner int, cmd Cmd) []Event {
	no := owner
	ev := Event{Kind: "swap_pet", Actor: &no, PetSlot: intPtr(cmd.PetSlot)}
	if old := b.petOf(owner); old != nil {
		ev.Left = append(ev.Left, old.No)
		b.leave(old)
	}
	if cmd.PetSlot >= 0 {
		p := b.sideOfNo(owner).Bench[cmd.PetSlot]
		u := b.swapInFor(owner, cmd.PetSlot, p, owner+firstPetNo)
		b.units[u.No] = u
		ev.Joined = append(ev.Joined, b.joinedView(u, owner))
	}
	return []Event{ev}
}

// effect 记录一次对目标的效果并结算伤害。
func (b *Battle) effect(ev *Event, target *Unit, result string, dmg int) {
	if result == "crit" || result == "hit" {
		b.hurt(target, dmg, result, ev)
		return
	}
	hpPct := target.HPPct()
	eff := Effect{Target: target.No, Result: result, Delta: 0, HPPct: hpPct, Down: !target.Alive()}
	if b.hpShown(target) {
		hp := target.HP
		eff.HP = &hp
	}
	ev.Effects = append(ev.Effects, eff)
}

// hurt 扣目标的血并把效果记进事件。target 倒下时，怪物进入击杀名单（结算奖励用）；
// 人物被这一下打飞时记下打飞（STAGE-63）。中毒扣的血最多到 1，不会打飞。
func (b *Battle) hurt(target *Unit, dmg int, result string, ev *Event) {
	dmg = target.rideSplit(dmg) // 骑乘时先分一部分给宠物（ride.go），人物只扣自己那份
	before := target.HP
	wasAlive := target.Alive()
	target.HP -= dmg
	down := false
	if target.HP <= 0 {
		target.HP = 0
		target.Down = true
		down = true
		if target.Kind == KindMonster {
			b.slain = append(b.slain, target)
			b.creditKill()
			if b.party {
				b.recordKillers(target)
			}
		}
	}
	if target.Kind == KindPlayer && b.duel == nil { // 决斗里不打飞（pk.md 2.1）
		b.flyCheck(target, dmg, dmg-before)
	}
	if wasAlive && !target.Alive() {
		b.onDown(target) // 新倒下：扣宠物忠诚（STAGE-146）；被打飞的人物不在这里扣，走 settleLoyalty
	}
	eff := Effect{Target: target.No, Result: result, Delta: -dmg, HPPct: target.HPPct(), Down: down,
		Flown: target.Flown}
	if b.hpShown(target) {
		hp := target.HP
		eff.HP = &hp
	}
	ev.Effects = append(ev.Effects, eff)
}

// selfEffect 生成一个只改变自己状态的事件（如防御）。
func (b *Battle) selfEffect(u *Unit, kind, result string, delta int, add []string) Event {
	no := u.No
	eff := Effect{Target: u.No, Result: result, Delta: delta, HPPct: u.HPPct(), Down: !u.Alive(), StatusAdd: add}
	if b.hpShown(u) {
		hp := u.HP
		eff.HP = &hp
	}
	return Event{Kind: kind, Actor: &no, Effects: []Effect{eff}}
}
