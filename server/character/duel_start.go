package character

// STAGE-306：决斗开战（docs/pk.md 3.3、4.3，协议 5.74）。duel_challenge 通过校验后，这里把双方的真人（队长带队员，
// 或单人）锁起来重新核对一遍，建双真人战斗（battle.NewDuel）并挂到每个人身上。结束与写回见 duel_battle.go。

import (
	"sort"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/protocol"
)

// duelSideMembers 把一方的真人名单整理成 Presence 列表：有真人队用队伍快照（队长在前），否则就是队长一个人。
func duelSideMembers(lead *Presence, party []*partyMember) []*Presence {
	if len(party) == 0 {
		return []*Presence{lead}
	}
	out := make([]*Presence, 0, len(party))
	for _, m := range party {
		out = append(out, m.p)
	}
	return out
}

// startDuel 开战。a 是发起方（队长在前），b 是被挑战方（队长在前）；aParty / bParty 是各自的队伍快照（没有真人队为空），
// 用来在加锁后确认两边的队伍没变。成功返回战斗编号；调用方不持有任何角色锁。
// 锁顺序：全部参战者按 charID 升序加 mu（别处加多把角色锁的路径同此顺序，见 party_battle.go）。
func (s *Service) startDuel(a, b []*Presence, aParty, bParty []*partyMember) (string, *protocol.Error) {
	all := append(append([]*Presence(nil), a...), b...)
	sort.Slice(all, func(i, j int) bool { return all[i].charID < all[j].charID })
	for _, p := range all {
		p.mu.Lock()
	}
	defer func() {
		for i := len(all) - 1; i >= 0; i-- {
			all[i].mu.Unlock()
		}
	}()
	// 加锁后再核对：队伍没变，每个人都在世界里、没倒下、不在战斗和交易中、没有分身队。
	if !samePartyMembers(s.parties.snapshot(a[0]), aParty) || !samePartyMembers(s.parties.snapshot(b[0]), bParty) {
		return "", protocol.Errorf(protocol.CodeWrongState, "队伍刚刚变了，请重新发起")
	}
	if perr := duelRecheck(a, false); perr != nil {
		return "", perr
	}
	if perr := duelRecheck(b, true); perr != nil {
		return "", perr
	}
	if !b[0].rec.SocialDuel {
		return "", protocol.Errorf(protocol.CodeTargetRefuses, "对方没有接受决斗")
	}
	toMate := func(side []*Presence, base int) []battle.PartyMate {
		out := make([]battle.PartyMate, 0, len(side))
		for i, p := range side {
			in := sideInput(&p.rec)
			in.DuelPoint = p.rec.DuelPoint
			out = append(out, battle.PartyMate{No: base + i, Side: in})
		}
		return out
	}
	bt, err := battle.NewDuel(s.nextBattleID(), battle.EncounterScene(a[0].rec.Floor), toMate(a, 0), toMate(b, 10), s.wildRnd())
	if err != nil {
		return "", protocol.Errorf(protocol.CodeInternal, "决斗开战失败：%v", err)
	}
	seats := make([]*Presence, 15)
	for i, p := range a {
		seats[i] = p
	}
	for i, p := range b {
		seats[10+i] = p
	}
	s.wireParty(bt, seats)
	s.startDuelLog(bt, seats)
	return bt.ID, nil
}

// duelRecheck 在锁下核对一方每个人的状态；target 为真时用 TARGET_ 开头的错误码（对方）。
func duelRecheck(side []*Presence, target bool) *protocol.Error {
	pick := func(self, other string, selfMsg, otherMsg string) *protocol.Error {
		if target {
			return protocol.Errorf(other, "%s", otherMsg)
		}
		return protocol.Errorf(self, "%s", selfMsg)
	}
	for _, p := range side {
		switch {
		case p.leaving:
			return protocol.Errorf(protocol.CodeWrongState, "有人刚刚离开了游戏")
		case p.rec.HP <= 0:
			return pick(protocol.CodeCharacterDead, protocol.CodeTargetDead, "队伍里有人已经倒下，不能决斗", "对方队伍里有人已经倒下，不能决斗")
		case p.battle != nil:
			return pick(protocol.CodeInBattle, protocol.CodeTargetInBattle, "队伍里有人在战斗中", "对方在战斗中")
		case p.trade != nil:
			return pick(protocol.CodeInTrade, protocol.CodeTargetInTrade, "队伍里有人在交易中", "对方在交易中")
		case p.party != nil:
			return pick(protocol.CodeAlreadyInParty, protocol.CodeTargetInParty, "有分身队的角色不能决斗", "对方有分身队，不能决斗")
		case p.rec.Floor != side[0].rec.Floor:
			return protocol.Errorf(protocol.CodeWrongState, "队伍里有人不在队长所在的地图")
		}
	}
	return nil
}
