// 防御次数（STAGE-199）：光、镜子、守护的精灵，以及复活光、复活镜、复活守护，对照原版
// gmsv/battle/battle_event.c 的 BATTLE_GetDamageReact、BATTLE_DamageSub 与 BATTLE_DuckCheck。
// 每个单位有三个独立的次数：吸收、反射、无效。挂上时直接设成精灵的次数（覆盖，不叠加），
// 不随回合减少，只在被物理直接攻击时减 1；战斗结束随单位一起没有（单位每场重建）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// 防御标记的协议取值（协议 10.2 的 status、10.7 的 result）。
const (
	defAbsorb  = "absorb"
	defReflect = "reflect"
	defVanish  = "vanish"
)

// DefenseCounters 是单位的三个防御次数（原版 CHAR_WORKDAMAGEABSROB、CHAR_WORKDAMAGEREFLEC、CHAR_WORKDAMAGEVANISH）。
type DefenseCounters struct {
	Absorb, Reflect, Vanish int
}

// any 报告有没有任何一个防御次数大于 0；有的话这个单位不闪避（BATTLE_DuckCheck）。
func (d DefenseCounters) any() bool { return d.Absorb > 0 || d.Reflect > 0 || d.Vanish > 0 }

// counter 取某个标记（协议 10.2 的 absorb、reflect、vanish）对应的次数；未知的标记返回 nil。
func (d *DefenseCounters) counter(kind string) *int {
	switch kind {
	case defAbsorb:
		return &d.Absorb
	case defReflect:
		return &d.Reflect
	case defVanish:
		return &d.Vanish
	}
	return nil
}

// unitStatus 是单位的状态标记列表（协议 10.2）：异常与防御动作在 StatusSet.List 里，
// 防御次数标记接在后面，有剩余次数时才有。
func unitStatus(u *Unit) []string {
	out := u.Status.List()
	for _, kind := range []struct {
		name string
		n    int
	}{{defAbsorb, u.Def.Absorb}, {defReflect, u.Def.Reflect}, {defVanish, u.Def.Vanish}} {
		if kind.n > 0 {
			out = append(out, kind.name)
		}
	}
	return out
}

// defendUnit 是防御精灵对一个单位的结算：把对应的次数设成 count（覆盖，不叠加）。
// 原来没有这个标记时才放进 status_add，已经挂着只是刷新次数，不重复通知。
func (b *Battle) defendUnit(target *Unit, kind string, count int, ev *Event) {
	c := target.Def.counter(kind)
	if c == nil {
		return
	}
	var add []string
	if *c <= 0 && count > 0 {
		add = []string{kind}
	}
	*c = count
	ev.Effects = append(ev.Effects, b.stateEffect(target, add, nil))
}

// defReact 是物理直接攻击的防御结算（BATTLE_DamageSub 的 react 分支）：dmg 是这一下已经过防御减伤的伤害，
// 只在大于 0 时生效（原版 damage <= 0 直接返回）。优先级：无效 > 吸收 > 反射，取第一个次数大于 0 的，消耗一次。
// 返回 true 表示这一下已被防御处理（无效、吸收或反射），调用方不要再 hurt；false 表示照常扣血。
// 攻击者是投掷武器（回旋标、投掷斧、投掷石）时不反射，按普通伤害结算，计数器不减（BATTLE_IsThrowWepon）。
func (b *Battle) defReact(attacker, target *Unit, dmg int, ev *Event) bool {
	if dmg <= 0 || !target.Def.any() {
		return false
	}
	switch {
	case target.Def.Vanish > 0:
		target.Def.Vanish--
		b.defEffect(ev, target, "vanish", 0, target.Def.Vanish == 0, defVanish)
		return true
	case target.Def.Absorb > 0:
		target.Def.Absorb--
		before := target.HP
		target.HP = min(target.HP+dmg, target.MaxHP) // 吸收：这一下的伤害值变成回血，不超过最大血量
		b.defEffect(ev, target, "absorb", target.HP-before, target.Def.Absorb == 0, defAbsorb)
		return true
	case target.Def.Reflect > 0 && !attacker.throwing():
		target.Def.Reflect--
		b.hurt(attacker, dmg, "reflect", ev) // 伤害落在攻击者身上（可能打倒它，hurt 照常记账）
		no := target.No
		ev.Effects[len(ev.Effects)-1].Intent = &no // 反射指回弹开它的那一方
		if target.Def.Reflect == 0 && ev.Kind != "combo" {
			// 反射用完：挂在被打的单位身上另起一条效果（合击的 effects 与 actors 一一对应，不另起，见协议 10.7）
			ev.Effects = append(ev.Effects, b.stateEffect(target, nil, []string{defReflect}))
		}
		return true
	}
	return false
}

// defEffect 记下防御结算对被打单位的效果；次数用完（done）时把对应标记放进 status_remove。
func (b *Battle) defEffect(ev *Event, target *Unit, result string, delta int, done bool, kind string) {
	eff := Effect{Target: target.No, Result: result, Delta: delta, HPPct: target.HPPct(), Down: !target.Alive()}
	if done {
		eff.StatusRemove = []string{kind}
	}
	if b.hpShown(target) {
		hp := target.HP
		eff.HP = &hp
	}
	ev.Effects = append(ev.Effects, eff)
}

// defendMagic 是防御精灵（光、镜子、守护）的结算：对己方一个挂上防御次数，缺省落在施法者自己身上。
// 施法时目标已经不在己方活着的一边（倒下、离场）就没有效果，同状态精灵的 statusTargets 判法。
func (b *Battle) defendMagic(u *Unit, cmd Cmd, t gamedata.MagicTemplate, ev *Event) {
	target := u
	if cmd.Target != -1 {
		target = b.Unit(cmd.Target)
	}
	if target != nil && target.Alive() && target.Side() == u.Side() {
		b.defendUnit(target, t.Status, t.Turns, ev)
	}
}

// reviveDefendMagic 是复活防御精灵（复活光、复活镜、复活守护）的结算：先复活提交时选定的倒下的己方单位
// （规则同 reviveUnit），再挂上防御次数。
func (b *Battle) reviveDefendMagic(u *Unit, cmd Cmd, t gamedata.MagicTemplate, ev *Event) {
	x := b.Unit(cmd.Target)
	if x == nil || x.Alive() || x.Side() != u.Side() || x.Kind == KindMonster || b.duelNoRevive(x) {
		return
	}
	b.reviveUnit(x, t.Power, ev)
	b.defendUnit(x, t.Status, t.Turns, ev)
}

// physHurt 是物理直接攻击的扣血（STAGE-199）：先过防御结算（defReact），没有被无效、吸收或反射处理才真的 hurt。
// 返回 true 表示这一下落在了 hit 身上（扣了血，或回了血），调用方据此决定要不要触发命中后的附带效果。
// attacker 必须是真正出手的单位（不是复制出来的临时单位），反射会扣它的血。
func (b *Battle) physHurt(attacker, hit *Unit, dmg int, result string, ev *Event) bool {
	if b.defReact(attacker, hit, dmg, ev) {
		return false
	}
	b.hurt(hit, dmg, result, ev)
	if dmg > 0 { // STAGE-209：物理伤害落地后装备耐久结算（原版 BATTLE_ItemCrushSeq）
		b.itemCrushSeq(attacker, hit, dmg, ev)
	}
	return true
}
