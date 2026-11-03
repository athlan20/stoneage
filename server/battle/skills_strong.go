// 强化攻击类宠物技能（STAGE-76）：一击必杀、突击、背水之战、忠犬、地球一周、不防守战法。
// 对照原版 gmsv/battle/pet_skill.c 的 PETSKILL_* 与 battle.c、battle_event.c 的结算。
// 两类效果要分开看：
//   - 出手时才生效：倍率、攻击力加成、对方闪避加成，只作用于这一下攻击（skillMods）；
//   - 指令提交时就定下：背水之战的防御、不防守战法的回避和反击、忠犬的替挡关系，
//     整回合所有结算都生效，与行动顺序无关（stanceMods，见 skill_turn.go 的 beginRound）。
// 突击（蓄力）和地球一周（隐身）跨回合：这里保存在 Unit.pend 里，之后的回合不再要指令。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// skillMods 是一次攻击的临时修正，只在它出手的那一下有效（原版 gBattleDamageModyfy、
// gBattleDuckModyfy 与 ATTACKPOWER 的变化）。
type skillMods struct {
	Attack int     // 攻击力的增减（点数，已取整）
	Damage float64 // 伤害倍率；0 表示不变
	Duck   int     // 对方闪避率的增减（百分点）
}

// stanceMods 是整回合有效的修正，由指令提交时的技能定下（见 beginRound）。
type stanceMods struct {
	DefensePct     int  // 防御力的增减（百分比）：背水之战的防%
	NoGuard        bool // 不防守战法：本回合可以反击（不做防御）
	NoGuardDuck    int  // 不防守战法：自己的回避率增减（百分点）
	NoGuardCounter int  // 不防守战法：自己的反击率增减（百分点）
}

// pendKind 是跨回合的待出手状态。
type pendKind uint8

const (
	pendCharge pendKind = iota + 1 // 突击：蓄力中，回合开始不要指令
	pendEarth                      // 地球一周：隐身中，下一回合攻击
)

// pending 是跨回合保存的出手（原版 BATTLE_COM_S_CHARGE / BATTLE_COM_S_EARTHROUND0 的工作变量）。
type pending struct {
	kind   pendKind
	name   string
	target int // 目标单位号（出手时若已失效，按 BATTLE_TargetAdjust 改选）
	left   int // 突击：还要蓄几回合
	pct    int // 突击：攻击力 +%；地球一周：伤害 +%
}

// mighty 是一击必杀（PETSKILL_Mighty）：这一下伤害 ×倍率，对方的闪避率加上回避值。
// 参数 “倍2 回避30”：倍率按原版取整成 1/100（iBai = (int)(fBai*100)）。
func (b *Battle) mighty(pet *Unit, target int, skill gamedata.PetSkill) Event {
	bai, _ := optNumber(skill.Arg, "倍")
	duck, _ := optNumber(skill.Arg, "回避")
	return b.skillAttack(pet, target, skill.Name, skillMods{
		Damage: float64(int(bai*100)) / 100,
		Duck:   int(duck),
	})
}

// charge 是突击（PETSKILL_ChargeAttack）：记下目标、蓄力回合数和攻击力加成，然后马上算这一回合的蓄力。
// 参数 “1 攻%+90”：开头的数是蓄力回合数（1–10，超出按 1），“攻%” 后面是攻击力加成。
func (b *Battle) charge(pet *Unit, cmd Cmd, skill gamedata.PetSkill) Event {
	n, ok := leadingInt(skill.Arg)
	if !ok || n < 1 || n > 10 {
		n = 1
	}
	pct, _ := optNumber(skill.Arg, "攻%")
	pet.pend = &pending{kind: pendCharge, name: skill.Name, target: cmd.Target, left: n, pct: int(pct)}
	return b.chargeStep(pet)
}

// chargeStep 是突击的一回合（BATTLE_Charge）：还没蓄满就减一回合、什么也不做；
// 蓄满了就按攻击力加成出手（攻击力 = 基础 + 基础 × 攻% 取整）。
func (b *Battle) chargeStep(u *Unit) Event {
	p := u.pend
	if p.left > 0 {
		p.left--
		return u.skillNote(p.name)
	}
	u.pend = nil
	return b.skillAttack(u, p.target, p.name, skillMods{Attack: pctOf(u.Attack, float64(p.pct))})
}

// earthRound 是地球一周（PETSKILL_EarthRound）：这一回合绕到对方背后（隐身，不能被选为目标），
// 下一回合出手，伤害按参数 “攻%+90” 加成（gBattleDamageModyfy = 1 + 0.01 × 90）。
func (b *Battle) earthRound(pet *Unit, cmd Cmd, skill gamedata.PetSkill) Event {
	pct, _ := optNumber(skill.Arg, "攻%")
	pet.pend = &pending{kind: pendEarth, name: skill.Name, target: cmd.Target, pct: int(pct)}
	return pet.skillNote(skill.Name)
}

// earthStrike 是地球一周隐身结束后的出手（原版 COM 为 BATTLE_COM_S_EARTHROUND0 的那一回合）。
func (b *Battle) earthStrike(u *Unit) Event {
	p := u.pend
	u.pend = nil
	return b.skillAttack(u, p.target, p.name, skillMods{Damage: 1 + 0.01*float64(p.pct)})
}

// powerBalance 是背水之战（PETSKILL_PowerBalance）：这一下攻击力 +攻%。
// 防御力的 防% 不在这里改，由 beginRound 在整回合里生效。
func (b *Battle) powerBalance(pet *Unit, target int, skill gamedata.PetSkill) Event {
	atk, _ := optNumber(skill.Arg, "攻%")
	return b.skillAttack(pet, target, skill.Name, skillMods{Attack: pctOf(pet.Attack, atk)})
}

// guardianAttack 是忠犬（PETSKILL_Guardian）的出手：攻击力 攻%（-20）。
// 替挡的关系由 beginRound 登记，攻击主人时由 attackSeq 改判（guardianOf）。
func (b *Battle) guardianAttack(pet *Unit, target int, skill gamedata.PetSkill) Event {
	atk, _ := optNumber(skill.Arg, "攻%")
	return b.skillAttack(pet, target, skill.Name, skillMods{Attack: pctOf(pet.Attack, atk)})
}

// noGuard 是不防守战法（PETSKILL_NoGuard）：不出手，也不做防御；回避率和反击率在 beginRound 里整回合生效。
func (b *Battle) noGuard(pet *Unit, skill gamedata.PetSkill) Event {
	return pet.skillNote(skill.Name)
}

// skillAttack 是带修正的一次攻击：修正只作用于这一下，结束后清掉。
// 目标已经倒下时改选对方一个能选的单位（原版 BATTLE_TargetAdjust）；对方没人了就不出手。
func (b *Battle) skillAttack(u *Unit, target int, name string, mods skillMods) Event {
	u.act = mods
	defer func() { u.act = skillMods{} }()
	t := b.adjustTarget(u, b.Unit(target))
	if t == nil {
		return skipEvent(u, "timeout")
	}
	return b.attackEvent(u, t.No, name)
}

// skillNote 是只有名字、没有效果的技能事件（蓄力、隐身、不防守这些回合）。
func (u *Unit) skillNote(name string) Event {
	no := u.No
	return Event{Kind: "skill", Actor: &no, Name: name}
}

// resumePending 处理跨回合的待出手：回合开始时不需要指令，直接按之前定下的内容继续。
func (b *Battle) resumePending(u *Unit) []Event {
	if !u.Status.CanAct() {
		return []Event{skipEvent(u, skipReason(u.Status))}
	}
	if u.pend.kind == pendCharge {
		return []Event{b.chargeStep(u)}
	}
	return []Event{b.earthStrike(u)}
}
