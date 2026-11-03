// 真人同场战斗里的离场（STAGE-275，docs/protocol.md 10.14）：逃跑、超时踢出、掉线。
// 引擎只做战斗内的事（人物和宠物退场、血量保存、胜负判定）并把离场记成 Departure；
// 离队、解散、推送 battle_leave / party_changed 由上层（队伍对象）依据 Departure 处理。

package battle

import "time"

// Departure 是一次真人离场的结果，由 TakeDepartures 取走。
type Departure struct {
	No       int          // 离场真人的人物单位号（队长 0，队员 1–4）
	Name     string       // 名字（推送 battle_leave 用）
	Cause    string       // escape（逃跑成功）、timeout（超时踢出）、offline（掉线）
	Left     []int        // 离场的单位号：人物和它的宠物（分身队不进真人队，不会有分身）
	Disband  bool         // 队长离场：上层应解散队伍（其余队员留在战斗里）；队员离场时为 false
	Progress ProgressView // 离场之后的交指令进度（推送 battle_leave 的 progress 用）
}

// depart 让一名真人离场：人物和出战宠物从场上移走，血量保存在 finalHuman 里，原因记入 left。
// 离场后如果场上已经没有真人，战斗结束（result 为 draw；每人的结局看 EndFor）。
func (b *Battle) depart(no int, cause string) Departure {
	b.duelNoteCmds() // 决斗：离场前记下本回合是否有人交过攻击类指令（免罚判断，duel_rules.go）
	b.duelDepart(no)
	d := Departure{No: no, Cause: cause, Disband: no == 0 || no == duelLeaderB}
	if h := b.units[no]; h != nil {
		d.Name = h.Name
		d.Left = append(d.Left, no)
		b.finalHuman[no] = h
	} else if m := b.mates[no]; m != nil {
		d.Name = m.Side.Name
	}
	if p := b.units[no+5]; p != nil {
		d.Left = append(d.Left, no+5)
		if no == 0 {
			b.leftPet = p // 同单人逃跑：结算时出战宠物仍算出战（STAGE-247）
		} else {
			b.finalPets[no] = p
		}
	}
	for _, x := range d.Left {
		b.leave(b.units[x])
	}
	b.left[no] = cause
	if no != 0 {
		b.mates[no].needs = nil
	} else {
		b.needs = b.needs[:0]
	}
	delete(b.mateCmds, no)
	delete(b.mateCmds, no+5)
	b.checkDuelEnd()
	b.checkEmpty()
	d.Progress = b.progress()
	b.departs = append(b.departs, d)
	return d
}

// checkEmpty 在场上已经没有真人时结束战斗。逃跑、超时、掉线各自的结局由 EndFor 按离场原因给出。
func (b *Battle) checkEmpty() {
	if b.over || len(b.inBattleHumans()) > 0 {
		return
	}
	b.over = true
	b.result = "draw"
}

// inBattleHumans 返回还在场上（没有离场）的真人人物单位号，队长在前。
func (b *Battle) inBattleHumans() []int {
	var out []int
	for _, no := range b.humanSlots() {
		if b.units[no] != nil && (no == 0 || b.mates[no] != nil) {
			out = append(out, no)
		}
	}
	return out
}

// TakeDepartures 取走本回合之后产生的离场（逃跑、超时、掉线），上层据此离队、解散并推送。
func (b *Battle) TakeDepartures() []Departure {
	out := b.departs
	b.departs = nil
	return out
}

// Departed 返回真人离场的原因（escape / timeout / offline）；没有离场时返回空串。
func (b *Battle) Departed(no int) string { return b.left[no] }

// FinalHuman 返回真人的人物单位（在场或离场后）：结算、写回血量和魔法用。没有这个真人时返回 nil。
func (b *Battle) FinalHuman(no int) *Unit {
	if u := b.units[no]; u != nil {
		return u
	}
	return b.finalHuman[no]
}

// ExpireDue 检查交指令的 120 秒截止：到点仍没交齐的真人被踢出战斗（超时踢出，原版 battle.c:2839-2853），
// 没有默认指令。截止时间从本回合第一条被接受的指令起算（Submit，party_submit.go）。
func (b *Battle) ExpireDue(now time.Time) []Departure {
	if !b.party || b.over || b.phase != PhaseCommand || b.deadline.IsZero() || now.Before(b.deadline) {
		return nil
	}
	var out []Departure
	for _, no := range b.inBattleHumans() {
		if b.humanOwes(no) {
			out = append(out, b.depart(no, "timeout"))
		}
	}
	return out
}

// humanOwes 表示这名真人本回合还欠指令（需要交的身份里有没交的）。
func (b *Battle) humanOwes(no int) bool {
	for _, actor := range b.needsOf(no) {
		if b.cmdOfHuman(no, actor) == nil {
			return true
		}
	}
	return false
}

// needsOf 返回一名真人本回合需要交的指令（player / pet）。
func (b *Battle) needsOf(no int) []string {
	if no == 0 {
		return b.needs
	}
	if m := b.mates[no]; m != nil {
		return m.needs
	}
	return nil
}

// cmdOfHuman 取一名真人某个身份的指令：队长取 cmds，队员取 mateCmds。
func (b *Battle) cmdOfHuman(no int, actor string) *Cmd {
	if no == 0 {
		return b.cmds[actor]
	}
	return b.mateCmd(no, actor)
}

// progress 是本回合的交指令进度（真人同场）：total 为需要交指令的真人数，done 为其中已交齐的人数。
func (b *Battle) progress() ProgressView {
	var pv ProgressView
	if !b.party {
		return ProgressView{Done: 0, Total: 1}
	}
	for _, no := range b.inBattleHumans() {
		if len(b.needsOf(no)) == 0 {
			continue
		}
		pv.Total++
		if !b.humanOwes(no) {
			pv.Done++
		}
	}
	return pv
}

// ExpireBattle 检查整场时限（PartyTimeLimit）：到点时仍在场的真人都以 draw 结束（EndFor 给出），战斗结束。
// 返回是否已经超时。
func (b *Battle) ExpireBattle(now time.Time) bool {
	if !b.party || b.over || now.Before(b.startedAt.Add(PartyTimeLimit)) {
		return false
	}
	b.over = true
	b.result = "draw"
	return true
}

// finishParty 是真人同场时的胜负判定（同 finish，但不生成 battle_end：每人的结局由 EndFor 给出）。
// 返回战斗是否已经结束。
func (b *Battle) finishParty() bool {
	if b.over {
		return true
	}
	if b.duel != nil {
		return b.checkDuelEnd()
	}
	playerGone := b.sideGone(0)
	enemyGone := b.sideGone(1)
	if !playerGone && !enemyGone {
		return false
	}
	b.over = true
	switch {
	case playerGone: // 己方没有存活的非宠物真人（STAGE-169，同单人）
		b.result = "lose"
	case len(b.slain)+len(b.captur) > 0:
		b.result = "win"
		b.assignDrops()
	default:
		b.result = "draw"
	}
	b.settleLoyalty()
	return true
}

// setActing 记下正在出手的单位（击杀归属用，party_reward.go）：单人战斗也记，不影响结算。
func (b *Battle) setActing(us []*Unit) {
	b.acting = b.acting[:0]
	for _, u := range us {
		b.acting = append(b.acting, u.No)
	}
}

// Offline 让掉线的真人离场（上层在连接断开时调用）：不结算经验、战利品、金钱，宠物随之退场，血量照存（10.14）。
// 返回 false 表示这不是真人同场战斗，或者这个真人已经不在场上。
func (b *Battle) Offline(no int) (Departure, bool) {
	if !b.party || !b.validHuman(no) || b.units[no] == nil || (no != 0 && b.mates[no] == nil) {
		return Departure{}, false
	}
	return b.depart(no, "offline"), true
}
