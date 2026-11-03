package character

// 真人同场战斗（STAGE-275，docs/social-players.md 2.6、2.11，docs/protocol.md 10.14）：队长遇敌时，全体在线队员同场。
// 战斗对象 partyBattle 是共享的：每名真人的 battleState 都指向它，所有对战斗的操作先拿战斗锁，再拿各真人的 mu。
//
// 锁顺序（只在这里定义，别处照此用）：
//
//	开战：队长的 mu（调用方持有）→ 各队员的 mu（charID 升序）→ partyHub.mu（只读快照时短暂持有）
//	战斗中的操作（指令、确认、计时、离场）：partyBattle.mu → 全部真人的 mu（charID 升序，all）
//
// 战斗中的操作一律锁全员，所以任何一名真人的请求、计时器、掉线都与其它人的操作互斥。
// 分身队和单人战斗不经过这里（battleState.pb 为 nil）。

import (
	"sort"
	"sync"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// partyBattle 是一场真人同场战斗的共享状态。字段受 mu（以及 all 各人的 mu）保护。
type partyBattle struct {
	s     *Service
	b     *battle.Battle
	mu    sync.Mutex
	all   []*Presence // 开战时的全部真人，按 charID 升序（加锁顺序，只在开战时定）
	seats []*Presence // 号位：seats[no] 是 no 号真人（队长 0，队员 1–4；决斗里对方是 10–14，中间为 nil）；离场的号位仍占着，见 gone
	gone  []bool      // 已离场的号位（不再收推送、不再交指令）
	done  bool        // 战斗已结束（写回完成，状态已摘掉）

	timed   bool           // 本回合的交指令截止计时已经挂上
	ackWait bool           // 已推 battle_round，在等所有连接确认
	ackSeen map[*Live]bool // 本回合已确认的连接（每条连接只计一次）
	turn    int            // ackWait 对应的回合（等确认时引擎已在下一回合）
	timers  []*time.Timer  // 交指令截止与确认等待的计时器，战斗结束时全部停掉

	dpChanges []duelChangeLog // 决斗里每名真人离场时的决斗点数变化（duel_battle.go），结束时写进 duel_logs
}

// newPartyBattle 把开好的战斗和真人对应起来。seats 按号位排好（队长在 0）。
func newPartyBattle(s *Service, b *battle.Battle, seats []*Presence) *partyBattle {
	var all []*Presence
	gone := make([]bool, len(seats))
	for no, p := range seats {
		if p == nil {
			gone[no] = true // 空号位（决斗里 5–9 之类）视为已离场，各处遍历 active 时自然跳过
			continue
		}
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].charID < all[j].charID })
	return &partyBattle{s: s, b: b, all: all, seats: seats, gone: gone, ackSeen: map[*Live]bool{}}
}

// lockAll 按顺序拿战斗的锁：先 mu，再全部真人的 mu。返回解锁函数。
func (pb *partyBattle) lockAll() func() {
	pb.mu.Lock()
	for _, p := range pb.all {
		p.mu.Lock()
	}
	return func() {
		for i := len(pb.all) - 1; i >= 0; i-- {
			pb.all[i].mu.Unlock()
		}
		pb.mu.Unlock()
	}
}

// noOf 返回真人 p 的号位；不是这场战斗的人或已经离场返回 -1。调用时持有 lockAll。
func (pb *partyBattle) noOf(p *Presence) int {
	for no, s := range pb.seats {
		if s == p && !pb.gone[no] {
			return no
		}
	}
	return -1
}

// active 返回还在战斗中的号位（号位升序）。调用时持有 lockAll。
func (pb *partyBattle) active() []int {
	var nos []int
	for no := range pb.seats {
		if !pb.gone[no] {
			nos = append(nos, no)
		}
	}
	return nos
}

// pushEach 给每个还在战斗中的真人推一条消息，内容按号位生成（同一个人的推送内容始终一致，10.14）。调用时持有 lockAll。
func (pb *partyBattle) pushEach(typ string, data func(no int) any) {
	for _, no := range pb.active() {
		pb.seats[no].pushAll(typ, data(no))
	}
}

// stopTimers 停掉这场战斗的计时器。
func (pb *partyBattle) stopTimers() {
	for _, t := range pb.timers {
		t.Stop()
	}
	pb.timers = nil
}

// deadlineMS 是交指令截止的毫秒时间戳，没有截止时间（本回合还没人交指令）时为 0。调用时持有 lockAll。
func (pb *partyBattle) deadlineMS() int64 {
	if d := pb.b.Deadline(); !d.IsZero() {
		return d.UnixMilli()
	}
	return 0
}

// pushTurns 给每人推本回合的 battle_turn（deadline_ms 为 null，needs 是他自己的）。调用时持有 lockAll。
func (pb *partyBattle) pushTurns() {
	now := pb.s.nowMS()
	pb.timed = false
	pb.pushEach("battle_turn", func(no int) any { return pb.b.TurnFor(no, now) })
}

// startPartyWild 是队长遇敌时的真人同场开战（调用方持有队长的 mu）。返回 (开战是否成功, 是否按真人同场处理)：
// 不是真人同场的情形（开关关着、没有真人队、队员不足 2 人、队员忙着）返回 (false, false)，由调用方照常开单人战斗。
func (s *Service) startPartyWild(leader *Presence) (ok bool, handled bool) {
	if !s.partyPlayersOn.Load() || s.parties == nil {
		return false, false
	}
	ms := s.parties.snapshot(leader)
	if len(ms) < 2 || ms[0].p != leader {
		return false, false
	}
	mates := make([]*Presence, 0, len(ms)-1)
	for _, m := range ms[1:] {
		mates = append(mates, m.p)
	}
	sorted := append([]*Presence(nil), mates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].charID < sorted[j].charID })
	for _, m := range sorted {
		m.mu.Lock()
	}
	defer func() {
		for i := len(sorted) - 1; i >= 0; i-- {
			sorted[i].mu.Unlock()
		}
	}()
	// 锁住以后再核对：仍是同一支队，队员都不在战斗中、没在离开。
	if !samePartyMembers(s.parties.snapshot(leader), ms) {
		return false, false
	}
	for _, m := range mates {
		if m.battle != nil || m.leaving {
			return false, false
		}
	}
	area := gamedata.EncounterAreaAt(leader.rec.Floor, leader.rec.X, leader.rec.Y)
	if area == nil {
		return false, true
	}
	pms := make([]battle.PartyMate, 0, len(mates))
	for i, m := range mates {
		pms = append(pms, battle.PartyMate{No: i + 1, Side: sideInput(&m.rec)})
	}
	b, ok, err := battle.BuildWildParty(s.nextBattleID(), battle.EncounterScene(leader.rec.Floor),
		sideInput(&leader.rec), pms, &battle.WildSpec{Area: area, HeldItems: heldItemSet(&leader.rec)}, s.wildRnd())
	if err != nil || !ok {
		return false, true
	}
	s.wireParty(b, append([]*Presence{leader}, mates...))
	return true, true
}

// samePartyMembers 判断两次快照是不是同一批真人（同一支队的同一批人，顺序也一样）。
func samePartyMembers(a, b []*partyMember) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// wireParty 把开好的真人同场战斗挂到每名真人身上并推送开战（调用方持有全员的 mu）。
func (s *Service) wireParty(b *battle.Battle, seats []*Presence) {
	pb := newPartyBattle(s, b, seats)
	b.Clock = s.now // 引擎的截止时间用服务的时钟（测试里可以推进）
	b.Cbs.ExpRate = s.ExpRate
	leader := seats[0]
	b.Cbs.PlaceCaptured = func(m *battle.Unit, keep *battle.CaptureKeep) (int, *battle.Discarded) {
		return captureOutcome(&leader.rec, m, keep) // 队长捕捉
	}
	b.Cbs.PlaceCapturedFor = func(owner int, m *battle.Unit, keep *battle.CaptureKeep) (int, *battle.Discarded) {
		return captureOutcome(&pb.seats[owner].rec, m, keep) // 队员捕捉归捕获者（STAGE-275）
	}
	for _, p := range seats {
		if p == nil {
			continue
		}
		p.battle = &battleState{b: b, pb: pb}
		p.lastBattleTurn = 0                   // STAGE-282：新战斗开始，上一场的迟到确认不再算数
		p.pushWindowClosed(WindowReasonBattle) // 对话窗口给战场让路（协议 5.34）
		p.players.touch(p)
	}
	for no, p := range seats {
		if p == nil {
			continue
		}
		if b.IsDuel() { // 决斗：带 duel 名单，对方人物只给血量百分比（STAGE-306）
			p.pushAll("battle_start", b.DuelStartFor(no))
		} else {
			p.pushAll("battle_start", b.StartViewFor(no))
		}
	}
	pb.timers = append(pb.timers, time.AfterFunc(battle.PartyTimeLimit, func() { s.partyTimeUp(pb) })) // 整场 3600 秒（10.14）
	pb.pushTurns()
}
