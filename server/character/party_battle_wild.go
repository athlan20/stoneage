package character

// 真人同场的野外开战，带移动（STAGE-275 第三轮 D1）：练级传送、1 级宠物目录、区域遇敌会先把队长定位到目标点再开战。
// 队员跟随队长的移动由 followRealParty（挂在 pushPosition 上）同步过去，它自己会加队员的锁，所以这里的顺序是：
//
//	1) 持队长的锁、加全体队员的锁核对并建好战斗（只读队员的背包、精灵、宠物），然后放开队员的锁
//	2) 队长移动到目标点，队员被跟随过去（此时队员的锁不持有）
//	3) 重新加队员的锁，核对：仍是同一批人、队员都不在战斗中、没在离开、都到了队长的位置
//	   都满足 → 真人同场开战；不满足 → 不同场，队长在新位置单人开战（已移动，但没有战斗的话返回 false）
//
// 步骤 1 建不出来时还没有移动，返回 handled 为假，由调用方照常开单人战斗。

import (
	"sort"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// startPartyWildMoved 是真人队的队长移动到 (floor,x,y) 后开野外战斗（调用方持有队长的 mu）。
// 返回 (开战是否成功, 是否按真人队处理)：开关关着、不是队长、队员不足 2 人、或建不出来时 handled 为假。
func (s *Service) startPartyWildMoved(leader *Presence, floor, x, y int) (ok bool, handled bool) {
	if !s.partyPlayersOn.Load() || s.parties == nil {
		return false, false
	}
	ms := s.parties.snapshot(leader)
	if len(ms) < 2 || ms[0].p != leader {
		return false, false
	}
	area := gamedata.EncounterAreaAt(floor, x, y)
	if area == nil {
		return false, false
	}
	mates := make([]*Presence, 0, len(ms)-1)
	for _, m := range ms[1:] {
		mates = append(mates, m.p)
	}

	// 1) 建战斗：队员锁下核对，建好后放开。
	unlock := lockMembers(mates)
	if !samePartyMembers(s.parties.snapshot(leader), ms) || !matesFree(mates) {
		unlock()
		return false, false
	}
	pms := make([]battle.PartyMate, 0, len(mates))
	for i, m := range mates {
		pms = append(pms, battle.PartyMate{No: i + 1, Side: sideInput(&m.rec)})
	}
	b, built, err := battle.BuildWildParty(s.nextBattleID(), battle.EncounterScene(floor),
		sideInput(&leader.rec), pms, &battle.WildSpec{Area: area, HeldItems: heldItemSet(&leader.rec)}, s.wildRnd())
	unlock()
	if err != nil || !built {
		return false, false
	}

	// 2) 队长移动，队员跟过去（followRealParty 自己加锁）。
	leader.rec.Floor, leader.rec.X, leader.rec.Y = floor, x, y
	leader.players.moved(leader)
	leader.pushPosition(&leader.rec)
	leader.pushWindowClosed(WindowReasonMoved)

	// 3) 重新核对并开战。
	unlock = lockMembers(mates)
	defer unlock()
	if samePartyMembers(s.parties.snapshot(leader), ms) && matesFree(mates) && matesAt(mates, leader) {
		s.wireParty(b, append([]*Presence{leader}, mates...))
		return true, true
	}
	// 队员没都到位（或在战斗中）：不同场，队长在新位置单人开战。
	sb, sok := battle.BuildWild(s.nextBattleID(), battle.EncounterScene(floor),
		sideInput(&leader.rec), &battle.WildSpec{Area: area, HeldItems: heldItemSet(&leader.rec)}, s.wildRnd())
	if !sok {
		return false, true
	}
	s.wireBattle(leader, sb)
	return true, true
}

// lockMembers 按 charID 升序加队员的锁（队长之后的唯一嵌套，见 party_battle.go 的锁顺序）。返回解锁函数。
func lockMembers(mates []*Presence) func() {
	sorted := append([]*Presence(nil), mates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].charID < sorted[j].charID })
	for _, m := range sorted {
		m.mu.Lock()
	}
	return func() {
		for i := len(sorted) - 1; i >= 0; i-- {
			sorted[i].mu.Unlock()
		}
	}
}

// matesFree 表示队员都不在战斗中、也没在离开。调用时持有队员的锁。
func matesFree(mates []*Presence) bool {
	for _, m := range mates {
		if m.battle != nil || m.leaving {
			return false
		}
	}
	return true
}

// matesAt 表示队员都在队长的位置上。调用时持有队员的锁。
func matesAt(mates []*Presence, leader *Presence) bool {
	for _, m := range mates {
		if m.rec.Floor != leader.rec.Floor || m.rec.X != leader.rec.X || m.rec.Y != leader.rec.Y {
			return false
		}
	}
	return true
}
