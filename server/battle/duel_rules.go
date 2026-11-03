// 决斗的规则差异与决斗点数记账（STAGE-306，docs/pk.md 2.2、2.3）。

package battle

import dpkg "github.com/athlan20/stoneage/server/duel"

// humanSlots 是可能有真人的人物单位号：0–4，决斗里再加对方的 10–14。
func (b *Battle) humanSlots() []int {
	if b.duel != nil {
		return []int{0, 1, 2, 3, 4, 10, 11, 12, 13, 14}
	}
	return []int{0, 1, 2, 3, 4}
}

// validHuman 判断 no 是不是这场战斗里可能的真人号。
func (b *Battle) validHuman(no int) bool {
	for _, x := range b.humanSlots() {
		if x == no {
			return true
		}
	}
	return false
}

// hpShown 判断推送里要不要写这个单位的精确血量：野外战斗只写玩家方（观看者）；
// 决斗里先全部写上，推给每个人之前由 Round.ForSide 把对方的裁掉（duel_view.go）。
func (b *Battle) hpShown(u *Unit) bool {
	return b.duel != nil || u.Side() == b.playerSide()
}

// joinedView 是换宠上场的单位视图。
func (b *Battle) joinedView(u *Unit, owner int) UnitView {
	if b.duel != nil {
		return u.view(u.Side(), -1)
	}
	return u.view(b.playerSide(), owner)
}

// duelNoRevive 表示复活对这个单位无效：决斗里的人物（精灵和道具都一样，道具照样消耗；宠物可以复活，pk.md 2.3）。
func (b *Battle) duelNoRevive(x *Unit) bool { return b.duel != nil && x.Kind == KindPlayer }

// fightOver 表示结算中不用再让后面的单位出手：野外是玩家方人物倒下，决斗是任一方没有存活的人物。
func (b *Battle) fightOver() bool {
	if b.duel != nil {
		return b.sideGone(0) || b.sideGone(1)
	}
	return b.sideGone(0)
}

// checkDuelEnd 判定决斗是否结束：任一方没有存活的人物（倒下或离场）即结束，另一方胜；两方同时没有则平局。
// 不是决斗时什么都不做，返回 false。
func (b *Battle) checkDuelEnd() bool {
	if b.duel == nil {
		return false
	}
	if b.over {
		return true
	}
	gone0, gone1 := b.sideGone(0), b.sideGone(1)
	if !gone0 && !gone1 {
		return false
	}
	b.over = true
	switch {
	case gone0 && gone1:
		b.result = "draw"
	case gone0:
		b.result = "lose" // 以发起方（0 号方）的角度；EndFor 给对方时反过来
	default:
		b.result = "win"
	}
	return true
}

// aliveHumans 返回某一方当前还站在场上的人物（没倒下、没离场）的单位号。
func (b *Battle) aliveHumans(side int) []int {
	var out []int
	for _, no := range b.humanSlots() {
		if u := b.units[no]; u != nil && u.Side() == side && u.Alive() {
			out = append(out, no)
		}
	}
	return out
}

// duelOnDown 是单位新倒下时的决斗点数记账：人物倒下，本场累计扣开战 DP 的一成，这份按人平分给本次行动的
// 对方出手者（宠物算主人，同一主人一份）；自己人打倒的（误伤、反弹、混乱）平分给对方当前活着的人物。宠物倒下不记。
func (b *Battle) duelOnDown(u *Unit) {
	if u.Kind != KindPlayer {
		return
	}
	b.duel.knocked = true
	var gainers []int
	for _, no := range b.acting {
		if no/unitsPerSide != u.Side() {
			gainers = append(gainers, ownerOf(no))
		}
	}
	if len(gainers) == 0 {
		gainers = b.aliveHumans(1 - u.Side())
	}
	b.duel.ledger.Knockout(u.No, gainers)
}

// duelDepart 在真人 no 离场（逃跑、超时、掉线）之前记账：人物还活着时，除非免罚，本场累计改为 -2 倍的
// 一成（覆盖此前累计），那一成平分给对方活着的人物。
func (b *Battle) duelDepart(no int) {
	if b.duel == nil {
		return
	}
	if h := b.units[no]; h == nil || !h.Alive() {
		return
	}
	if dpkg.EscapeImmune(b.turn == 1, b.duel.attacked, b.duel.knocked) {
		return
	}
	b.duel.ledger.Escape(no, b.aliveHumans(1-no/unitsPerSide))
}

// duelNoteCmds 记下本回合已交的指令里有没有攻击类（攻击、精灵、宠物技能、道具；防御、逃跑、换宠不算）。
// 结算开始和真人离场（离场会删掉他的指令）之前各调用一次。
func (b *Battle) duelNoteCmds() {
	d := b.duel
	if d == nil || d.attacked {
		return
	}
	isAtk := func(c *Cmd) bool {
		return c != nil && (c.Action == "attack" || c.Action == "magic" || c.Action == "item" || c.Action == "skill")
	}
	if isAtk(b.cmds["player"]) || isAtk(b.cmds["pet"]) {
		d.attacked = true
		return
	}
	for _, c := range b.mateCmds {
		if isAtk(c) {
			d.attacked = true
			return
		}
	}
}
