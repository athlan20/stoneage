package character

// STAGE-306：决斗（docs/pk.md 3.3，docs/protocol.md 5.74）。本文件是发起前的校验；
// 开战在 duel_start.go，结束写回在 duel_battle.go，双真人战斗引擎在 server/battle/duel.go。
// 社交开关 duel 在 social.go，决斗点数的纯规则在 server/duel。

import (
	"context"
	"sync"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// duelSettingKey 是全服设置 duel.enabled 的 key（settings.go）。
const duelSettingKey = "duel.enabled"

// duelGate 是决斗的总开关检查（协议 5.74 校验第 1 步）。
func (s *Service) duelGate() *protocol.Error {
	if !s.duelOn.Load() {
		return protocol.Errorf(protocol.CodeWrongState, "决斗暂未开放")
	}
	return nil
}

// duelInfo 是一次决斗请求里读到的角色状态（在角色锁下读取，之后不再持锁）。
// party 是真人队的成员快照，队长在 party[0]；没有真人队时为空。
type duelInfo struct {
	p      *Presence
	floor  int
	dead   bool
	battle bool
	trade  bool
	clone  bool
	duelOn bool
	party  []*partyMember
}

func duelInfoOf(p *Presence) duelInfo {
	ms := p.parties.snapshot(p) // 先取队伍快照（不持角色锁），再读角色字段
	p.mu.Lock()
	defer p.mu.Unlock()
	return duelInfo{
		p: p, floor: p.rec.Floor, dead: p.rec.HP <= 0, battle: p.battle != nil, trade: p.trade != nil,
		clone: p.party != nil, duelOn: p.rec.SocialDuel, party: ms,
	}
}

// inParty 判断 p 是不是 ms 里的某一员。
func inParty(ms []*partyMember, p *Presence) bool {
	for _, m := range ms {
		if m.p == p {
			return true
		}
	}
	return false
}

// handleDuelChallenge 是 duel_challenge（操作类，世界中，必须拥有控制权；协议 5.74）。
// 校验顺序见 5.74：功能开关、控制权（守卫）、自己的状态、队长身份、对方的存在与位置、对方（队员看队长）的开关与状态。
func (s *Service) handleDuelChallenge(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.duelGate(); perr != nil {
		return nil, perr
	}
	var in struct {
		Name string `json:"name"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Name == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字符串字段 name（对方角色名）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := duelInfoOf(l.p)
	switch {
	case me.dead:
		return nil, protocol.Errorf(protocol.CodeCharacterDead, "角色已经倒下，不能决斗")
	case me.battle:
		return nil, protocol.Errorf(protocol.CodeInBattle, "战斗中不能决斗")
	case me.trade:
		return nil, protocol.Errorf(protocol.CodeInTrade, "交易中，先完成或取消交易")
	case me.clone:
		return nil, protocol.Errorf(protocol.CodeAlreadyInParty, "有分身队的角色不能决斗")
	case len(me.party) > 0 && me.party[0].p != l.p:
		return nil, protocol.Errorf(protocol.CodeNotPartyLeader, "队员不能发起决斗，请队长发起")
	}
	tgt := s.findOnline(in.Name)
	if tgt == nil || tgt.hidden.Load() { // 隐身者视同不存在（协议 5.69）
		return nil, protocol.Errorf(protocol.CodePlayerNotFound, "没有这个在线角色")
	}
	to := duelInfoOf(tgt)
	switch {
	case to.floor != me.floor:
		return nil, protocol.Errorf(protocol.CodePlayerNotNearby, "对方不在同一张地图")
	case tgt == l.p || inParty(me.party, tgt):
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "不能和自己或队友决斗")
	}
	// 对方是队员时看他的队长（pk.md 2.1，整队应战）。
	lead := to
	if len(to.party) > 0 {
		lead = duelInfoOf(to.party[0].p)
	}
	switch {
	case !lead.duelOn:
		return nil, protocol.Errorf(protocol.CodeTargetRefuses, "对方没有接受决斗")
	case lead.battle:
		return nil, protocol.Errorf(protocol.CodeTargetInBattle, "对方在战斗中")
	case lead.trade:
		return nil, protocol.Errorf(protocol.CodeTargetInTrade, "对方在交易中")
	case lead.clone:
		return nil, protocol.Errorf(protocol.CodeTargetInParty, "对方有分身队，不能决斗")
	case lead.dead:
		return nil, protocol.Errorf(protocol.CodeTargetDead, "对方已经倒下，不能决斗")
	}
	if perr := s.duelRate.check(s.opts.Now(), l.p.charID, tgt.charID); perr != nil { // 第 9 步：频率
		return nil, perr
	}
	// 通过全部校验：整队（或单人）开战，先推双方所有参战者的 battle_start，再回响应。
	id, perr := s.startDuel(duelSideMembers(l.p, me.party), duelSideMembers(lead.p, lead.party), me.party, lead.party)
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"battle_id": id}, nil
}

// duelRateWindow 是同一挑战者对同一目标两次发起之间的最短间隔（协议 5.74 第 9 步）。
const duelRateWindow = 10 * time.Second

// duelRates 记录通过了前 8 步校验的发起时间（因对方拒绝而失败的不记）。零值可用。
type duelRates struct {
	mu   sync.Mutex
	sent map[[2]int64]time.Time
}

// check 判断 from 对 to 的这次发起是否太频繁：太频繁回 RATE_LIMITED（带 retry_after_seconds），否则记下时间。
func (d *duelRates) check(now time.Time, from, to int64) *protocol.Error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sent == nil {
		d.sent = map[[2]int64]time.Time{}
	}
	key := [2]int64{from, to}
	if t, ok := d.sent[key]; ok && now.Sub(t) < duelRateWindow {
		secs := int((duelRateWindow - now.Sub(t) + time.Second - 1) / time.Second)
		return protocol.Errorf(protocol.CodeRateLimited, "发起决斗太频繁，过一会儿再试").
			WithData(map[string]any{"retry_after_seconds": secs})
	}
	d.sent[key] = now
	return nil
}
