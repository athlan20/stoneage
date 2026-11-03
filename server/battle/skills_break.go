// 破除防御与落马术（STAGE-291 批 A）：照原版 battle_event.c 的 BATTLE_S_GBreak（3818 起）与
// BATTLE_S_FallGround（5125 起）移植。破防不做防御减伤：目标防御中打全额，没防御伤害为 0、算没打中；
// 落马术是攻击力 −30% 的一次攻击，打中后原版掷 RAND(0,100) > 50 才落马（新引擎的怪物没有骑乘，见 fallGround）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// guardBreak 是破除防御（PETSKILL_GuardBreak → BATTLE_S_GBreak）。
// 目标防御中（且没混乱）：无视防御，伤害全额（会心照算）；目标没防御：伤害 0，显示没打中。
// 反击循环：目标没防御时继续（原版 iRet 为真），防御中结束。
func (b *Battle) guardBreak(pet *Unit, target int, name string) Event {
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: name}
	t := b.adjustTarget(pet, b.Unit(target))
	if t == nil {
		return skipEvent(pet, "timeout")
	}
	dmg, result := b.attackSeqRaw(pet, t) // 不经过 attackSeq 的防御减伤（原版 GBREAK 不调 BATTLE_GuardAdjust）
	guarded := guarding(t)
	b.lastAtk = atkRec{ok: true, result: result, target: t, cont: !guarded}
	if !guarded {
		b.effect(&ev, t, "miss", 0)
		return ev
	}
	switch result {
	case "dodge", "miss":
		b.effect(&ev, t, result, 0)
	default:
		if b.physHurt(pet, t, dmg, result, &ev) && dmg > 0 {
			b.wakeUp(t, &ev)
		}
	}
	return ev
}

// guarding 表示单位此刻在防御中（原版 CHAR_WORKBATTLECOM1 == BATTLE_COM_GUARD 且没混乱）。
// 防御指令在回合开始时登记（registerHumanGuard / registerPetStance / registerMonsterStance）。
func guarding(u *Unit) bool {
	return u.guardRound && u.Status.Confusion <= 0 // STAGE-291 批 B：按本回合交的指令算，不看行动后才置位的 Status.Guard
}

// attackSeqRaw 是不经过防御调整的一次攻击（破防用）：闪避、会心、伤害照算，不做忠犬替挡。
func (b *Battle) attackSeqRaw(attacker, target *Unit) (int, string) {
	if b.duckCheck(attacker, target) {
		return 0, "dodge"
	}
	var dmg int
	result := "hit"
	if b.roll(10000) < criticalCheck(attacker, target) {
		dmg = criDamageCalc(attacker, target, b.damageCalc(attacker, target))
		result = "crit"
	} else {
		dmg = b.damageCalc(attacker, target)
	}
	if dmg < 1 {
		dmg = b.Rnd(2)
	}
	if dmg == 0 {
		return 0, "miss"
	}
	return dmg, result
}

// fallGround 是落马术（PETSKILL_FallGround → BATTLE_S_FallGround）。攻击力按参数“攻%-30”减（原版把
// CHAR_WORKATTACKPOWER 设成 STR + STR×(−30%)），其余同一次普通攻击（闪避、会心、防御减伤都照算）。
// 打中（伤害 > 0 且没被反射吸收）后原版掷 RAND(0,100) > 50 才落马。落马只对骑着宠物的单位有效，
// 新引擎的怪物没有骑乘（ride.go 只给人物），人物骑乘的落马也还没做（docs/ride.md §7），所以这里只消耗这次随机。
// 反击循环：会心、被防御挡下、防御中的目标都不进（原版 iRet 为假）。
func (b *Battle) fallGround(pet *Unit, target int, skill gamedata.PetSkill) Event {
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: skill.Name}
	t := b.adjustTarget(pet, b.Unit(target))
	if t == nil {
		return skipEvent(pet, "timeout")
	}
	atk := *pet // 只改这一次攻击的攻击力，宠物本身的数值不变
	if pct, ok := optNumber(skill.Arg, "攻%"); ok {
		atk.Attack = pet.Attack + pctOf(pet.Attack, pct)
	}
	dmg, result, hit := b.attackSeq(&atk, t, false)
	b.lastAtk.cont = b.lastAtk.cont && !guarding(t)
	switch result {
	case "dodge", "miss", "block":
		b.effect(&ev, hit, result, 0)
		return ev
	}
	if b.physHurt(pet, hit, dmg, result, &ev) && dmg > 0 {
		b.wakeUp(hit, &ev)
		_ = b.Rnd(101) // RAND(0,100) > 50 才落马：没有可落的骑乘，只消耗这次随机，保持与原版的随机序列一致
	}
	return ev
}
