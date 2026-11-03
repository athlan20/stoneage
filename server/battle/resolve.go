// 一回合的结算（docs/protocol.md 10.7）：所有单位按敏捷依次行动，产出事件序列，
// 回合末做状态结算并判定胜负。伤害、命中、逃跑与捕获的概率公式在 rules.go，
// 逐个需求（STAGE-44 起）换成原版 gmsv/battle 的算法。

package battle

// Settle 结算当前回合。调用前 AllSubmitted 必须为真（超时未交的按 skip 处理）。
// 返回按观看者（玩家人物所在方）生成的 battle_round 内容；战斗在这回合结束时
// 同时给出 battle_end 的内容（最后一回合不需要 battle_ack，协议 10.8）。
func (b *Battle) Settle() (Round, *EndView) {
	b.phase = PhaseReplay
	b.duelNoteCmds()
	b.beginRound()
	events := []Event{}
	order := b.actingOrder()
	for i := 0; i < len(order); i++ {
		if b.fightOver() { // 人物倒下就败了，后面的单位（含宠物）不再出手（STAGE-169）；决斗里任一方没有存活人物都算
			break
		}
		u := order[i]
		if b.units[u.No] != u || !u.Alive() { // 行动途中被击倒，或者已被换下战场
			continue
		}
		if group, target := b.comboGroup(order, i); len(group) > 1 { // 合击：一组人合成一次行动（STAGE-126）
			b.setActing(group)
			events = append(events, b.combo(group, target))
			i += len(group) - 1
			continue
		}
		b.setActing([]*Unit{u})
		events = append(events, b.act(u)...)
	}
	b.acting = nil
	events = append(events, b.statusTick()...)
	events = append(events, b.tickField()...) // 场地的剩余回合减一，减到 0 恢复成无属性（STAGE-200）
	round := Round{BattleID: b.ID, Turn: b.turn, Events: events}
	for _, u := range b.units {
		if u != nil {
			round.State = append(round.State, b.stateOf(u))
		}
	}
	if b.party { // 真人同场：每人的 battle_end 由 EndFor 给出（party_reward.go），这里只判胜负和推进回合
		if !b.finishParty() {
			b.nextTurn()
		}
		return round, nil
	}
	if end := b.finish(); end != nil {
		return round, end
	}
	b.nextTurn()
	return round, nil
}

// finish 判定战斗是否结束并生成 battle_end 的内容；没结束时返回 nil。
func (b *Battle) finish() *EndView {
	if b.over { // 行动里已经判定了结果（如逃跑成功）
		return b.endView()
	}
	playerGone := b.sideGone(0)
	enemyGone := b.sideGone(1)
	if !playerGone && !enemyGone {
		return nil
	}
	b.over = true
	if playerGone {
		b.result = "lose"
	} else if len(b.slain)+len(b.captur) > 0 {
		// 敌方全倒或被捉走：算胜利。逃走的怪物已经离场、不算存活的敌人，
		// 所以“一只跑了、其余被打死”也是胜利（STAGE-193）。
		b.result = "win"
	} else {
		b.result = "draw" // 敌人全部逃走：没分出胜负（协议 10.9）
	}
	b.settleLoyalty()
	return b.endView()
}

// endView 按 result 生成 battle_end 的内容。
func (b *Battle) endView() *EndView {
	switch b.result {
	case "win":
		v := &EndView{BattleID: b.ID, Result: "win", Turn: b.turn, Rewards: b.rewards()}
		if b.NPC != nil && b.NPC.End != "" { // NPC 发起的战斗：赢了说结束台词（STAGE-96）
			v.NPC = &NPCEnd{NPCRef: b.NPC.NPCRef, Say: b.NPC.End}
		}
		return v
	case "lose":
		return &EndView{BattleID: b.ID, Result: "lose", Turn: b.turn, Penalty: &Penalty{}, Rewards: b.nonWinRewards()}
	case "escaped":
		return &EndView{BattleID: b.ID, Result: "escaped", Turn: b.turn, Rewards: b.nonWinRewards()}
	default:
		return &EndView{BattleID: b.ID, Result: "draw", Turn: b.turn, Rewards: b.nonWinRewards()}
	}
}

// finishEscape 是玩家逃跑成功后的结束（在 escape 的行动里调用）。
func (b *Battle) finishEscape(u *Unit) {
	b.over = true
	b.result = "escaped"
	b.leave(u)
	if pet := b.commandPet(); pet != nil {
		b.leftPet = pet // STAGE-247：原版逃跑不改 CHAR_DEFAULTPET，出战的宠物逃跑后仍是出战
		b.leave(pet)
	}
	for _, x := range b.units { // 分身队跟着队长一起撤（STAGE-162）
		if x != nil && x.Auto && x.Side() == u.Side() {
			b.leave(x)
		}
	}
}

// skipReason 把“不能行动”的状态翻成 skip 事件的 reason。
func skipReason(s StatusSet) string {
	switch {
	case s.Sleep > 0:
		return "sleep"
	case s.Stone > 0:
		return "stone"
	case s.Paralysis > 0:
		return "paralysis"
	case s.Barrier > 0: // STAGE-201
		return "barrier"
	case s.Confusion > 0:
		return "confusion"
	}
	return "timeout"
}

func skipEvent(u *Unit, reason string) Event {
	no := u.No
	return Event{Kind: "skip", Actor: &no, Reason: reason}
}
