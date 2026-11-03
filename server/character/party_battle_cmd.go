package character

// 真人同场战斗的出招、确认、回合推进与超时（STAGE-275，docs/protocol.md 10.5、10.8、10.14）。
// 每个入口都先拿战斗锁（partyBattle.lockAll），见 party_battle.go 的锁顺序。

import (
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/protocol"
)

// partyOfLive 返回这条连接的角色当前所在的真人同场战斗；不在这种战斗中（单人、分身队、没战斗）返回 nil。
func partyOfLive(live *Live) *partyBattle {
	live.p.mu.Lock()
	defer live.p.mu.Unlock()
	if bs := live.p.battle; bs != nil {
		return bs.pb
	}
	return nil
}

// partyCommand 是真人同场时的出招：交指令的是这条连接的角色，号位由它决定（10.14）。调用方不持有任何角色锁。
func (s *Service) partyCommand(live *Live, pb *partyBattle, turn int, actor, action string, cmd battle.Cmd) (any, error) {
	unlock := pb.lockAll()
	defer unlock()
	if live.left {
		return nil, ErrLeft
	}
	if live.p.controller != live.role {
		return nil, controlError(live.role)
	}
	no := pb.noOf(live.p)
	if no < 0 || live.p.battle == nil || live.p.battle.pb != pb {
		return nil, protocol.Errorf(protocol.CodeNotInBattle, "当前不在战斗中")
	}
	next, perr := pb.b.SubmitFor(no, turn, actor, action, cmd)
	if perr != nil {
		return nil, perr
	}
	pb.armTimer()
	if pb.b.AllSubmitted() {
		pb.settle()
	} else {
		pb.pushProgress()
	}
	return map[string]any{"next": next}, nil
}

// armTimer 在本回合第一条被接受的指令起挂上 120 秒的交指令截止计时（10.14）。调用时持有 lockAll。
func (pb *partyBattle) armTimer() {
	if pb.timed {
		return
	}
	pb.timed = true
	turn := pb.b.Turn()
	pb.timers = append(pb.timers, time.AfterFunc(battle.PartyCommandWait, func() {
		pb.s.partyExpire(pb, turn)
	}))
}

// pushProgress 给每人推 battle_progress（done/total，截止时间已定时带 deadline_ms）。调用时持有 lockAll。
func (pb *partyBattle) pushProgress() {
	now := pb.s.nowMS()
	pb.pushEach("battle_progress", func(int) any { return pb.b.ProgressPush(now) })
}

// partyExpire 是 120 秒截止计时的回调：本回合仍欠指令的真人被踢出（引擎的 ExpireDue 决定），然后按剩下的人继续。
func (s *Service) partyExpire(pb *partyBattle, turn int) {
	unlock := pb.lockAll()
	defer unlock()
	if pb.done || pb.ackWait || pb.b.Turn() != turn || pb.b.Phase() != battle.PhaseCommand {
		return
	}
	pb.applyDepartures(pb.b.ExpireDue(s.now()))
	if pb.done {
		return
	}
	if pb.b.Over() {
		pb.finish()
		return
	}
	if pb.b.AllSubmitted() {
		pb.settle()
	}
}

// settle 结算一回合：先推 battle_progress（都交齐了），再推 battle_round，然后等每人确认（10.8）。调用时持有 lockAll。
func (pb *partyBattle) settle() {
	pb.stopTimers()
	pb.pushProgress()
	round, _ := pb.b.Settle()     // 真人同场时 Settle 的 end 恒为 nil，结束看 Over（每人的 battle_end 由 finish 给出）
	for no, p := range pb.seats { // 道具因出招被消耗：各人消耗自己背包里的（battle_round 之前推送，协议 10.7）
		if p == nil {
			continue
		}
		if evs := itemEventsOf(round.Events, no); len(evs) > 0 {
			pb.s.consumeBattleItems(p, evs)
		}
	}
	pb.pushEach("battle_round", func(no int) any {
		if pb.b.IsDuel() {
			return round.ForSide(no / 10) // 决斗：对方的精确血量裁掉（STAGE-306）
		}
		return round
	})
	pb.applyDepartures(pb.b.TakeDepartures()) // 逃跑在本回合的结算里产生（battle.depart），先推完 battle_round 再离场（STAGE-275 第三轮）
	if pb.b.Over() {
		pb.finish()
		return
	}
	pb.ackWait, pb.ackSeen, pb.turn = true, map[*Live]bool{}, round.Turn
	turn := round.Turn
	pb.timers = append(pb.timers, time.AfterFunc(battleAckTimeout, func() {
		pb.s.partyAckTimeout(pb, turn)
	}))
}

// partyAck 是真人同场时的回放确认：这条连接确认后，全部在场真人的每条连接都确认了才进入下一回合。
func (s *Service) partyAck(live *Live, pb *partyBattle, turn int) error {
	unlock := pb.lockAll()
	defer unlock()
	if live.left {
		return ErrLeft
	}
	if live.p.battle == nil || live.p.battle.pb != pb {
		return protocol.Errorf(protocol.CodeNotInBattle, "当前不在战斗中")
	}
	if !pb.ackWait || turn != pb.turn {
		return nil // 重复或过期的确认按成功回复（协议 10.8）
	}
	pb.ackSeen[live] = true
	if pb.allAcked() {
		pb.advance()
	}
	return nil
}

// allAcked 表示还在战斗中的真人，每条连接都确认过了。调用时持有 lockAll。
func (pb *partyBattle) allAcked() bool {
	for _, no := range pb.active() {
		for _, l := range pb.seats[no].members {
			if !pb.ackSeen[l] {
				return false
			}
		}
	}
	return true
}

// advance 确认齐了（或超时）后进入下一回合。调用时持有 lockAll。
func (pb *partyBattle) advance() {
	pb.ackWait = false
	pb.pushTurns()
}

// partyAckTimeout 是等确认的超时：视为都已确认（10.8）。
func (s *Service) partyAckTimeout(pb *partyBattle, turn int) {
	unlock := pb.lockAll()
	defer unlock()
	if !pb.done && pb.ackWait && pb.turn == turn {
		pb.advance()
	}
}

// partyQuery 是真人同场时的查看当前战斗（10.11）：返回这个真人的视角，交指令的截止时间是全队的。调用方不持有任何角色锁。
func (s *Service) partyQuery(live *Live, pb *partyBattle) (any, error) {
	unlock := pb.lockAll()
	defer unlock()
	if live.left {
		return nil, ErrLeft
	}
	no := pb.noOf(live.p)
	if no < 0 || live.p.battle == nil || live.p.battle.pb != pb {
		return map[string]any{"in_battle": false}, nil
	}
	return pb.b.QueryFor(no, pb.deadlineMS(), s.nowMS()), nil
}

// itemEventsOf 取出号位 no 这一回合用掉道具的事件（道具只会是人物用，事件的 actor 为人物单位号）。
func itemEventsOf(events []battle.Event, no int) []battle.Event {
	var out []battle.Event
	for _, ev := range events {
		if ev.Kind == "item" && ev.Actor != nil && *ev.Actor == no {
			out = append(out, ev)
		}
	}
	return out
}

// partyTimeUp 是整场时限（3600 秒）的回调：仍在战斗中的真人以 draw 结束（10.14）。
func (s *Service) partyTimeUp(pb *partyBattle) {
	unlock := pb.lockAll()
	defer unlock()
	if !pb.done && pb.b.ExpireBattle(s.now()) {
		pb.finish()
	}
}
