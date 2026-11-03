// 精灵（魔法）指令的执行（STAGE-48、STAGE-77）：MAGIC_AttMagic 的伤害（STAGE-227 起是 FIX 公式，magic_fix.go）、
// MAGIC_Recovery / MAGIC_OtherRecovery 的回复，出自 gmsv/magic/magic.c 与 gmsv/battle/battle_magic.c
// 的 MAGIC_AttMagic_Battle；状态（MAGIC_StatusChange_Battle）、解状态（MAGIC_StatusRecovery_Battle）
// 和复活（MAGIC_Ressurect_Battle）是 STAGE-77 接入的。MP（气力）按人物栏位上的精灵模板扣
// （模板的 MP 是 tools/gen-magic.py 推算的）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// slotMagic 取队长 magic 栏位上的精灵模板，MP 换成这个栏位的施放消耗；空栏位或不认识的编号返回 false。
func (b *Battle) slotMagic(slot int) (gamedata.MagicTemplate, bool) {
	return b.slotMagicFor(0, slot)
}

// unitMagic 是单位 u 的 slot 栏位上的精灵：分身（auto.go）用它自己带的栏位（u.Magics，STAGE-195），
// 人物（队长或队员）用他自己开战时的栏位（slotMagicFor）。施放消耗两边都按主人的快照（分身的快照从队长复制，装备精灵的 MP 也一样）。
func (b *Battle) unitMagic(u *Unit, slot int) (gamedata.MagicTemplate, bool) {
	if u.Auto {
		return b.magicIn(u.Magics, slot)
	}
	return b.slotMagicFor(b.actorNo(u), slot)
}

// magicIn 按栏位表 ids 取精灵模板，MP 用队长快照里这个栏位的施放消耗。
func (b *Battle) magicIn(ids [5]int, slot int) (gamedata.MagicTemplate, bool) {
	return b.magicInSide(b.sides[0], ids, slot)
}

// magicInSide 按一方快照的栏位表 ids 取精灵模板，MP 用这一方快照里这个栏位的施放消耗（STAGE-190）。
func (b *Battle) magicInSide(side *SideInput, ids [5]int, slot int) (gamedata.MagicTemplate, bool) {
	t, ok := gamedata.MagicByID(ids[slot])
	if ok && side.MagicMPSet[slot] {
		t.MP = side.MagicMP[slot]
	}
	return t, ok
}

// castMagic 执行一次精灵（cmd.Magic 是精灵栏位 0–4）。气力不够在提交时就被拒，
// 这里再扣一次（提交与结算之间没有回气）。伤害公式的属性相克与普攻同一张表。
// 场地属性（STAGE-200）施放后多一个 field 事件，所以返回的是事件序列。
func (b *Battle) castMagic(u *Unit, t gamedata.MagicTemplate, cmd Cmd) []Event {
	if t.Func == gamedata.MagicFuncMetamo { // STAGE-203：变幻的精灵战斗里不能用（提交时已拒；分身队自动出招不走提交，这里兜底，不扣 MP）
		return []Event{skipEvent(u, "timeout")}
	}
	u.MP -= t.MP
	// 表现信息一起发下去（STAGE-167）：客户端按 func/elem/all/status 挑特效与范围，
	// 不靠 name 做映射（同名精灵、改过的名字都不可靠）。
	ev := Event{Kind: "magic", Actor: &u.No, Name: t.Name, Magic: &MagicView{
		ID: t.ID, Func: t.Func, Elem: elemWire(t.Elem), All: t.All, Status: t.Status,
	}}
	switch t.Func {
	case gamedata.MagicFuncAttack:
		targets := b.magicAttackTargets(u, cmd, t)
		ok := b.magicCastOK(u, magicElemIndex(t.Elem)) // 施法成败（原版每次施放掷一次，STAGE-227）
		hits := 0
		for _, target := range targets {
			// 每个被打的目标各判一次魔法闪避（原版 BATTLE_MagicDodge 在取格循环里逐目标调用）；
			// 闪过的不扣血，效果写 dodge，同召唤龙阵。
			if b.magicDodge(target) {
				b.effect(&ev, target, "dodge", 0)
				continue
			}
			hits++
			b.magicHit(u, target, t, ok, &ev)
		}
		if !ok && u.Kind == KindPlayer { // 施法失败才涨攻击熟练度（原版 Magic_ComputeAttExp，STAGE-227）
			magicComputeAttExp(u, magicElemIndex(t.Elem), t.Lv, hits)
		}
	case gamedata.MagicFuncHeal: // 自己（或己方全体）
		if t.All {
			for _, x := range b.units {
				if x != nil && x.Alive() && x.Side() == u.Side() {
					b.healUnit(x, t.Power, &ev)
				}
			}
		} else {
			b.healUnit(u, t.Power, &ev)
		}
	case gamedata.MagicFuncHealOther: // 己方单人（或全体）
		if t.All {
			for _, x := range b.units {
				if x != nil && x.Alive() && x.Side() == u.Side() {
					b.healUnit(x, t.Power, &ev)
				}
			}
		} else if x := b.Unit(cmd.Target); x != nil && x.Alive() {
			b.healUnit(x, t.Power, &ev)
		} else {
			b.healUnit(u, t.Power, &ev) // 没给目标时落在自己身上
		}
	case gamedata.MagicFuncStatus: // 状态：打对方（单人或全体）
		for _, target := range b.statusTargets(u, cmd, t) {
			b.magicStatus(u, target, t, &ev)
		}
	case gamedata.MagicFuncCure: // 解状态：救己方（单人或全体）
		for _, target := range b.statusTargets(u, cmd, t) {
			if removed := target.Status.cureMagic(t.Status); len(removed) > 0 {
				ev.Effects = append(ev.Effects, b.stateEffect(target, nil, removed))
			}
		}
	case gamedata.MagicFuncResist: // 魔抗系（STAGE-228，magic_resist.go）：己方单人或全体，不掷骰
		for _, target := range b.statusTargets(u, cmd, t) {
			b.resistMagic(target, t, &ev)
		}
	case gamedata.MagicFuncRevive: // 复活：提交时已选定一个倒下的己方单位
		if x := b.Unit(cmd.Target); x != nil && !x.Alive() && x.Side() == u.Side() && x.Kind != KindMonster && !b.duelNoRevive(x) {
			b.reviveUnit(x, t.Power, &ev)
		}
	case gamedata.MagicFuncDragon: // 召唤龙阵（STAGE-202，magic_dragon.go）：闪避、伤害，不反击、不附带状态
		b.castDragon(u, t, cmd, &ev)
	case gamedata.MagicFuncAttSkill: // 精灵版宠物技能（STAGE-202，magic_attskill.go）：只有技能事件，不另包 magic 事件
		return []Event{b.castAttSkill(u, t, cmd)}
	case gamedata.MagicFuncField: // 场地属性（STAGE-200）：整场生效，不用选
		return append([]Event{ev}, b.setField(t)...)
	case gamedata.MagicFuncReverse: // 属性反转（STAGE-200）：所选单位；极光是它所在的一方全体
		for _, target := range b.reverseTargets(cmd, t.All) {
			ev.Effects = append(ev.Effects, b.reverseUnit(target))
		}
	case gamedata.MagicFuncDefend: // 防御：己方一个挂上吸收、反射或无效次数（magic_def.go，STAGE-199）
		b.defendMagic(u, cmd, t, &ev)
	case gamedata.MagicFuncReviveDefend: // 复活防御：复活之后再挂上防御次数（STAGE-199）
		b.reviveDefendMagic(u, cmd, t, &ev)
	}
	return []Event{ev}
}

// statusTargets 是状态类精灵的目标：全体时是场上活着的单位（状态打对方，解状态救己方），
// 单人时是提交时选的那一个（再确认一次还活着且站在对的一方）；解状态的己方一个缺省时落在自己身上
// （协议 10.5 的 ally）。目标的合法性在提交时已经用目标类型校验过（target.go）。
func (b *Battle) statusTargets(u *Unit, cmd Cmd, t gamedata.MagicTemplate) []*Unit {
	ours := t.Func == gamedata.MagicFuncCure || t.Func == gamedata.MagicFuncResist
	if !t.All {
		target := cmd.Target
		if target == -1 && ours {
			target = u.No
		}
		if x := b.Unit(target); x != nil && x.Alive() && (x.Side() == u.Side()) == ours {
			return []*Unit{x}
		}
		return nil
	}
	var out []*Unit
	for _, x := range b.units {
		if x != nil && x.Alive() && (x.Side() == u.Side()) == ours {
			out = append(out, x)
		}
	}
	return out
}

// magicStatus 是状态精灵对一个敌方单位的结算（BATTLE_MultiStatusChange）：成功则挂上状态，回合数是精灵的 turn
// （魔障、虚弱 +1，见 magic_debuff.go）；没中记一个 miss、状态不变（原版没中不出字，这里给客户端一个反馈）。
// 引擎没有的状态（剧毒）不会挂上。
func (b *Battle) magicStatus(caster, target *Unit, t gamedata.MagicTemplate, ev *Event) {
	f := target.Status.field(t.Status)
	if f == nil || !b.statusCheck(caster, target, t.Success, 30, 1.0) || !statusCanLand(t.Status, target) {
		b.effect(ev, target, "miss", 0)
		return
	}
	*f = statusTurnsOf(t)
	ev.Effects = append(ev.Effects, b.stateEffect(target, []string{t.Status}, nil))
}

// stateEffect 是只改变状态的效果（result 为 none）：挂上的状态放进 status_add，解掉的放进 status_remove。
func (b *Battle) stateEffect(target *Unit, add, remove []string) Effect {
	eff := Effect{Target: target.No, Result: "none", HPPct: target.HPPct(), Down: !target.Alive(),
		StatusAdd: add, StatusRemove: remove}
	if b.hpShown(target) {
		hp := target.HP
		eff.HP = &hp
	}
	return eff
}

// reviveUnit 是复活精灵对一个倒下的己方单位的结算（BATTLE_MultiRessurect）：回复 RAND(0.9 威力, 1.1 威力)
// 的血量，至少 1，不超过最大血量；威力为 0 时回满（原版按最大血量算）。复活后单位活着，下一回合起能行动。
func (b *Battle) reviveUnit(target *Unit, power int, ev *Event) {
	b.reviveAmount(target, power, false, ev)
}

// reviveAmount 是复活的结算的共用部分（精灵与复活道具）：percent 为真时 power 是最大血量的百分比
// （复活药参数带 %，STAGE-262），再按 0.9–1.1 倍随机，与定值的规则一样。
func (b *Battle) reviveAmount(target *Unit, power int, percent bool, ev *Event) {
	before := target.HP
	up := target.MaxHP
	if power > 0 {
		base := power
		if percent {
			base = target.MaxHP * power / 100
		}
		lo, hi := int(float64(base)*0.9), int(float64(base)*1.1)
		up = lo + b.Rnd(hi-lo+1)
	}
	target.Down = false
	target.HP = min(target.HP+max(1, up), target.MaxHP)
	b.addLoyalty(target, loyalRevived) // 被复活精灵救起（AI_FIX_PETRESSURECT，loyalty.go）
	hp := target.HP
	ev.Effects = append(ev.Effects, Effect{Target: target.No, Result: "heal", Delta: target.HP - before,
		HPPct: target.HPPct(), Down: false, HP: &hp})
}

// healUnit 是治疗系精灵的回复：效果 result 是 heal，delta 为正。
// 骑着宠物的人物（STAGE-257）：总回复量按缺口水位拆给人和骑宠（rideHeal），效果只写人物那一份。
func (b *Battle) healUnit(u *Unit, power int, ev *Event) {
	before := u.HP
	if u.Kind == KindPlayer && u.ridden() {
		u.rideHeal(power)
	} else {
		u.HP += power
		if u.HP > u.MaxHP {
			u.HP = u.MaxHP
		}
	}
	gain := u.HP - before
	b.onRecovered(u) // 宠物被回复：每场一次忠诚上升（AI_FIX_PETRECOVERY，loyalty.go）
	eff := Effect{Target: u.No, Result: "heal", Delta: gain, HPPct: u.HPPct(), Down: false}
	if b.hpShown(u) {
		hp := u.HP
		eff.HP = &hp
	}
	ev.Effects = append(ev.Effects, eff)
}

// elemOf 把属性名换成属性值。
// elemWire 把精灵表里的中文属性换成协议用的英文取值（STAGE-167）：攻击系之外返回空串。
func elemWire(name string) string {
	switch name {
	case "地":
		return "earth"
	case "水":
		return "water"
	case "火":
		return "fire"
	case "风":
		return "wind"
	}
	return ""
}

func elemOf(name string) Elements {
	switch name {
	case "地":
		return Elements{Earth: 100}
	case "水":
		return Elements{Water: 100}
	case "火":
		return Elements{Fire: 100}
	case "风":
		return Elements{Wind: 100}
	}
	return Elements{}
}
