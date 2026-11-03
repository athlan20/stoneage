// 偷窃、捐献、旅程伙伴（STAGE-291 批 A）：照原版 battle_event.c 的 BATTLE_Steal（4990 起）、
// BATTLE_StealMoney（4882 起，_BATTLESTEAL_FIX）、BATTLE_Abduct（4767 起）移植。三者都不造成伤害、不进反击循环。
// 原版偷窃只对真人玩家有效；新引擎没有 PvP，人物防守方的分支不可达，这里只实现怪物的分支。

package battle

// steal 是偷窃（PETSKILL_Steal → BATTLE_Steal）。怪物的成功率为 0：原版仍抽一次随机数，什么也不发生
// （不伤害、宠物不离场），事件里只显示没偷到。
func (b *Battle) steal(pet *Unit, target int, name string) Event {
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: name}
	t := b.adjustTarget(pet, b.Unit(target))
	if t == nil {
		return skipEvent(pet, "timeout")
	}
	b.roll(100) // RAND(1,100) < per，per = 0：抽了也不会成功
	b.effect(&ev, t, "miss", 0)
	return ev
}

// stealMoney 是捐献（PETSKILL_StealMoney → BATTLE_StealMoney）：对怪物 RAND(1,100) < 5 才成功，
// 拿 RAND(10,100) 石币给主人（主人持有已满时成功率为 0，石币不超过持有上限）；成功后宠物离场，
// 失败不伤害、宠物留场。石币记在 stolenGold，战斗结算时计入奖励（rewards.go 的 settleStolenGold）。
func (b *Battle) stealMoney(pet *Unit, target int, name string) Event {
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: name}
	t := b.adjustTarget(pet, b.Unit(target))
	if t == nil {
		return skipEvent(pet, "timeout")
	}
	per := 0
	if t.Kind == KindMonster {
		per = 5
	}
	room := b.goldRoom()
	if room <= 0 { // 主人已满：成功率为 0
		per = 0
	}
	if b.roll(100) >= per {
		b.effect(&ev, t, "miss", 0)
		return ev
	}
	gold := int64(b.randIn(10, 100))
	if gold > room {
		gold = room
	}
	b.stolenGold += gold
	b.leave(pet)
	ev.Left = []int{pet.No}
	return ev
}

// abduct 是旅程伙伴（PETSKILL_Abduct → BATTLE_Abduct）：对怪物的成功率见 abductPer。成功时怪物离场。
// 不论成败，宠物自己都离场（原版最后的 BATTLE_Exit），不造成伤害。
func (b *Battle) abduct(pet *Unit, target int, name string) Event {
	no := pet.No
	ev := Event{Kind: "skill", Actor: &no, Name: name}
	t := b.adjustTarget(pet, b.Unit(target))
	if t == nil {
		return skipEvent(pet, "timeout")
	}
	if t.Kind == KindPlayer { // 人物不能被带走：原版直接返回，宠物也不离场
		b.effect(&ev, t, "miss", 0)
		return ev
	}
	per := abductPer(t.Level, pet.Level)
	if b.NPC != nil { // 原版 BattleArray.WinFunc：NPC 战斗里成功率为 0
		per = 0
	}
	if b.roll(100) < per {
		b.leave(t)
		ev.Left = append(ev.Left, t.No)
	} else {
		b.effect(&ev, t, "miss", 0)
	}
	b.leave(pet)
	ev.Left = append(ev.Left, pet.No)
	return ev
}

// abductPer 是旅程伙伴的成功率（BATTLE_Abduct）：per = (怪物 Lv − 宠物 Lv) × 0.6 + 30，截断取整，最低 50。
func abductPer(enemyLv, petLv int) int {
	per := int(float64(enemyLv-petLv)*0.6) + 30
	if per < 50 {
		per = 50
	}
	return per
}

// randIn 是原版的 RAND(lo, hi)：[lo, hi] 内均匀取一个数。
func (b *Battle) randIn(lo, hi int) int { return lo + b.Rnd(hi-lo+1) }

// goldRoom 是主人还能再收的石币：持有上限减去开战时的石币和本场已偷到的（CHAR_getMaxHaveGold）。
func (b *Battle) goldRoom() int64 {
	var gold int64
	if s := b.sides[0]; s != nil {
		gold = s.Gold
	}
	return MaxHaveGold - gold - b.stolenGold
}

// settleStolenGold 把本场捐献偷到的石币计入胜利奖励的金币（原版是即时入账，这里在结算时并入），
// 封顶同 battleGoldGain：不超过持有上限。
func (b *Battle) settleStolenGold(gain int64) int64 {
	if b.stolenGold == 0 {
		return gain
	}
	gain += b.stolenGold
	if room := b.goldRoom() + b.stolenGold; gain > room {
		gain = room
	}
	return gain
}

// nonWinRewards 是非胜利结算（失败、逃跑、平手）时只带的捐献石币：原版是成功那一刻就加到人物身上，
// 不论战斗结果。没偷到时返回 nil，battle_end 与原来一样。
func (b *Battle) nonWinRewards() *Rewards {
	if b.stolenGold == 0 {
		return nil
	}
	return &Rewards{Gold: b.settleStolenGold(0)}
}
