// 分身队（STAGE-162，docs/protocol.md 5.57）：队长加 4 个“当前状态的自己”的分身，一起走路、一起打架。
// 队伍只存在于世界角色（Presence）的内存里，分身不建账号、不写数据库：队长解散、最后一条连接离开或服务端重启，
// 分身就全部消失，重新登录不保留。本期没有真人组队，队伍后台只有创建、解散、查看和推送。
// 战斗里分身的出招和结算规则在 server/battle/auto.go。
// 战斗中也能解散、重组（STAGE-189）：正在打的这场分身照常出场、打完为止（战斗单位是开战时的拷贝），
// 战后由 battle_apply.go 按结算后的队长重建；新队伍从下一场起上场。

package character

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// 队伍推送 party_changed 的 reason（协议 5.57）。
const (
	partyCreated   = "created"
	partyDisbanded = "disbanded"
	partyMoved     = "moved"
	partyBattleEnd = "battle_end"
)

// partyState 是一支分身队，挂在 Presence 上（受 p.mu 保护）。队长是角色本人，数据一直读 rec，不在这里存；
// 这里只有 4 个分身的快照。
type partyState struct {
	clones []*partyClone
	// rebuild 表示这支队是在战斗中组的：创建时队长的经验还没结算，战后要按新等级重新做一次（STAGE-189）。
	rebuild bool
	// plan 是组队时带来的自动战斗方案（STAGE-195，party_plan.go）；nil 表示分身走默认的普通攻击。
	plan *battle.AutoPlan
}

// partyClone 是一个分身：创建那一刻队长的快照。in 是战斗用的快照（含出战宠物），detail 是详情页用的静态内容。
// 战后 in 的血量、气力恢复满值（restore），所以每场战斗都从满血开始。
type partyClone struct {
	index  int // 成员稳定编号 1–4
	in     *battle.SideInput
	detail partyDetail
}

// newCloneParty 按队长此刻的状态做 4 个分身。sideInput 在没有设置出战宠物时会顺手把第一只标成出战，
// 这里拿记录的副本去算，不改队长的宠物状态。
func newCloneParty(r *record) *partyState {
	cp := *r
	cp.Pets = append([]petRec(nil), r.Pets...)
	base := sideInput(&cp)
	detail := newPartyDetail(r)
	ps := &partyState{}
	for i := 1; i <= battle.MaxClones; i++ {
		ps.clones = append(ps.clones, &partyClone{
			index:  i,
			in:     cloneInput(base, fmt.Sprintf("%s·分身%d", r.Name, i)),
			detail: detail,
		})
	}
	return ps
}

// cloneInput 从队长的战斗快照做一个分身的快照：保留等级、能力、血量气力、装备（武器类别）、精灵和出战宠物，
// 不带背包、金币、备用宠物和骑乘（分身不领奖励；精灵只在自动战斗方案里用，STAGE-195）。
func cloneInput(base *battle.SideInput, name string) *battle.SideInput {
	c := *base
	c.Name = name
	c.Overkill = 0
	c.Bench = [battle.PetSlots]*battle.PetInput{}
	c.Rider = nil
	c.Items = [20]battle.BattleItem{}
	c.Gold, c.Transmigration = 0, 0
	if base.BattlePet != nil {
		pet := *base.BattlePet
		c.BattlePet = &pet
	}
	return &c
}

// battleInputs 是开战时要放进己方阵容的分身快照。
func (ps *partyState) battleInputs() []*battle.SideInput {
	out := make([]*battle.SideInput, 0, len(ps.clones))
	for _, c := range ps.clones {
		out = append(out, c.in)
	}
	return out
}

// restore 在战斗结束后把分身恢复满血满气力，宠物也回满（协议 5.57：战后自动恢复满血）。
func (ps *partyState) restore() {
	for _, c := range ps.clones {
		c.in.HP, c.in.MP = c.in.MaxHP, c.in.MaxMP
		if c.in.BattlePet != nil {
			c.in.BattlePet.HP = c.in.BattlePet.MaxHP
		}
	}
}

// pushPartyChanged 把最新的队伍推给角色的每条连接；没有队伍时什么都不做。调用时要持有 p.mu。
func (p *Presence) pushPartyChanged(r *record, reason string) {
	if p.party == nil {
		return
	}
	p.pushAll("party_changed", map[string]any{"reason": reason, "party": p.partyView(r)})
}

// pushPosition 推角色的新位置；有队伍时紧跟一条 moved，让分身同步到队长的位置（协议 5.57）。调用时要持有 p.mu。
func (p *Presence) pushPosition(r *record) {
	p.pushAll("position_changed", r.position())
	p.pushPartyChanged(r, partyMoved)
	p.followRealParty(r) // STAGE-274：真人队的队员跟随队长
}

// handleParty 是查看队伍（查看类）。真人队的视图要逐个读队员，所以在角色锁之外组装（STAGE-274）。
func (s *Service) handleParty(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var real bool
	if err := l.With(func(r *record) {
		real = l.p.realParty.Load() != nil
		if !real {
			out = map[string]any{"party": l.p.partyView(r)}
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if real {
		out = map[string]any{"party": s.partyRealField(l.p)}
	}
	return out, nil
}

// handlePartyClone 组成分身队（操作类）。校验顺序：控制权（Operate）→ 倒下 → 已有队伍。战斗中也能组（STAGE-189）。
// data 里可以带 auto_profile（STAGE-195，party_plan.go）：收下就挂在队伍上，响应带 auto_profile（applied / ignored）。
func (s *Service) handlePartyClone(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var in struct {
		AutoProfile json.RawMessage `json:"auto_profile"`
	}
	if len(req.Data) > 0 {
		if perr := req.Decode(&in); perr != nil {
			return nil, perr
		}
	}
	var plan *battle.AutoPlan
	applied := ""
	if in.AutoProfile != nil {
		plan, applied = parseAutoPlan(in.AutoProfile)
	}
	var opErr *protocol.Error
	var out map[string]any
	if err := l.Operate(func(r *record) {
		p := l.p
		switch {
		case r.HP <= 0:
			opErr = protocol.Errorf(protocol.CodeCharacterDead, "角色已经倒下，不能组成分身队")
		case p.party != nil || p.realParty.Load() != nil: // 真人队里不能再加分身（STAGE-274）
			opErr = protocol.Errorf(protocol.CodeAlreadyInParty, "已经有队伍了，先解散再组")
		default:
			p.party = newCloneParty(r)
			p.party.rebuild = p.battle != nil // 战斗中组的队，战后按结算后的队长重建（STAGE-189）
			p.party.plan = plan
			p.pushPartyChanged(r, partyCreated)
			out = map[string]any{"party": p.partyView(r)}
			if applied != "" {
				out["auto_profile"] = applied
			}
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return out, nil
}

// handlePartyDisband 解散队伍（操作类）：分身立即消失；战斗中解散的话，正在打的这场分身打完为止（STAGE-189）。
func (s *Service) handlePartyDisband(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	if l, err := s.liveOrErr(sess); err == nil && l.p.realParty.Load() != nil {
		return s.partyDisbandPlayers(sess) // 真人队（STAGE-274）
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var opErr *protocol.Error
	if err := l.Operate(func(r *record) {
		p := l.p
		switch {
		case p.party == nil:
			opErr = protocol.Errorf(protocol.CodeNotInParty, "没有队伍可解散")
		default:
			p.party = nil
			p.pushAll("party_changed", map[string]any{"reason": partyDisbanded, "party": (*partyView)(nil)})
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return map[string]any{}, nil
}
