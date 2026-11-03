// 合击（STAGE-126）：几个同一方的单位在出手顺序里挨着、都普通攻击同一个目标时，有概率合成一次行动，
// 依次打同一个敌人。原版是 gmsv/battle/battle.c 的 ComboCheck / ComboCheck2（分组）和
// battle_event.c 的 BATTLE_Combo（结算），指令名 BATTLE_COM_COMBO。
//
//   - 起头：本回合普通攻击、能行动、没装投掷武器的单位，人物方 50%、怪物方 20% 概率成为合击起头
//     （ComboCheck 的 per；起头的单位被判成功就不再重复摇，后面的人只看条件）。
//   - 并入：出手顺序里紧跟着的单位，同一方、也是普通攻击、打同一个目标、没装投掷武器、能行动，
//     就并入同一组；任何一条不满足这一组就结束，断开的那个单位自己还能再摇起头。
//   - 结算：不判闪避（BATTLE_AttackSeq 的 opt 是 COMBO 时跳过 DuckCheck），每个人各自算暴击和伤害，
//     最低 1 点；一组的伤害累加成一个事件（协议 10.7 的 combo）。目标中途倒下，剩下的人不再出手。
//   - 合击不触发反击（BATTLE_Counter 只在普通攻击之后检查）。
//
// 宠物忠诚度太低、这回合不听话（AIBAD，loyalty.go）的不参与合击。

package battle

const (
	comboPlayerPct = 50 // 人物方起头的概率（%）
	comboEnemyPct  = 20 // 怪物方起头的概率（%）
)

// monsterPlan 是怪物这一回合提前定好的行动：合击要先看跟在后面的怪物打谁，所以决定提前到检查的时候，
// 轮到它行动时 takePlan 直接取这个结果，不再重新抽。
type monsterPlan struct {
	act aiAction
	ok  bool
}

// takePlan 取怪物本回合的行动：提前定过的直接用（cached 为真），没有就现抽。
func (b *Battle) takePlan(u *Unit) (act aiAction, ok bool, cached bool) {
	if u.plan != nil {
		p := u.plan
		u.plan = nil
		return p.act, p.ok, true
	}
	act, ok = u.AI.decide(b, u)
	return act, ok, false
}

// peekPlan 看怪物本回合的行动，没定的先定下来。
func (b *Battle) peekPlan(u *Unit) *monsterPlan {
	if u.plan == nil {
		act, ok := u.AI.decide(b, u)
		u.plan = &monsterPlan{act: act, ok: ok}
	}
	return u.plan
}

// comboTarget 是单位这回合打算普通攻击的目标；不是普通攻击、不能行动、混乱、蓄力中或装了投掷武器时 ok 为假。
func (b *Battle) comboTarget(u *Unit) (target int, ok bool) {
	if !u.Alive() || u.pend != nil || !u.Status.CanAct() || u.Status.Confusion > 0 || u.Weapon == weaponThrou {
		return 0, false
	}
	if u.Kind == KindMonster {
		if b.FleeTurn > 0 && b.turn >= b.FleeTurn {
			return 0, false
		}
		p := b.peekPlan(u)
		if !p.ok || p.act.kind != "attack" {
			return 0, false
		}
		return p.act.target, true
	}
	if u.Auto { // 分身队的单位按自动定的指令算，不过听话判定（STAGE-162）
		cmd := b.autoCmds[u.No]
		if cmd == nil || cmd.Action != "attack" {
			return 0, false
		}
		return cmd.Target, true
	}
	if u.Kind == KindPet { // 忠诚不足（不听话）的宠物不参与合击；听话时看实际执行的指令
		p := b.obey(u)
		if p.mode != loyalNormal || p.cmd == nil || p.cmd.Action != "attack" {
			return 0, false
		}
		return p.cmd.Target, true
	}
	cmd := b.cmds["player"]
	if cmd == nil || cmd.Action != "attack" {
		return 0, false
	}
	return cmd.Target, true
}

// comboGroup 判断出手顺序里第 i 个单位这回合有没有合击：成功时返回这一组（含自己，按出手顺序）
// 和他们要打的目标单位号。没有合击返回 nil。
func (b *Battle) comboGroup(order []*Unit, i int) ([]*Unit, int) {
	if !b.Combo || b.party { // 真人同场不做合击（STAGE-275：原版成员合击较复杂，本期不做）
		return nil, 0
	}
	lead := order[i]
	target, ok := b.comboTarget(lead)
	if !ok {
		return nil, 0
	}
	per := comboPlayerPct
	if lead.Kind == KindMonster {
		per = comboEnemyPct
	}
	if b.roll(100) > per {
		return nil, 0
	}
	group := []*Unit{lead}
	for _, u := range order[i+1:] {
		if b.units[u.No] != u || u.Side() != lead.Side() {
			break
		}
		if t, ok := b.comboTarget(u); !ok || t != target {
			break
		}
		group = append(group, u)
	}
	if len(group) < 2 {
		return nil, 0
	}
	return group, target
}

// combo 结算一次合击（BATTLE_Combo）：出手者依次打同一个目标，每人一条效果，actors 与 effects 一一对应。
func (b *Battle) combo(group []*Unit, planned int) Event {
	first := group[0]
	for _, u := range group {
		u.plan = nil // 提前定的行动作废：这一回合他们的行动就是合击
	}
	target := b.adjustTarget(first, b.Unit(planned)) // 目标已倒下时改选（BATTLE_TargetAdjust）
	if target == nil {
		return skipEvent(first, "timeout")
	}
	no := first.No
	// 合击的武器身份取领队（同一队通常同装备）；参与者各自的武器不在这里区分。
	ev := Event{Kind: "combo", Actor: &no, Weapon: first.motionOf()}
	for _, u := range group {
		if !target.Alive() || !u.Alive() {
			break
		}
		if u.Kind != KindMonster {
			u.Status.Guard = false // 行动开始时防御解除，同 act
		}
		b.markActed(u)
		dmg, result, hit := b.landSeq(u, target, false)
		if dmg < 1 {
			dmg = 1 // 合击没有落空：最低 1 点（BATTLE_Combo 的 damage <= 0 改 1）
		}
		if result != "crit" {
			result = "hit"
		}
		ev.Actors = append(ev.Actors, u.No)
		b.strike(&ev, u, hit, target, dmg, result)
	}
	return ev
}
