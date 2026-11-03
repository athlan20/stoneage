package character

import (
	"context"
	"errors"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// 控制权变化的原因（docs/protocol.md 9.3 的 control_changed.reason）。
const (
	ReasonAgentEntered     = "AGENT_ENTERED"
	ReasonTakenByPlayer    = "TAKEN_BY_PLAYER"
	ReasonReleasedByPlayer = "RELEASED_BY_PLAYER"
	ReasonPeerLeft         = "PEER_LEFT"
)

// pushAll 把一条推送发给这个角色在世界中的所有连接（玩家和 agent），保证两边看到的状态一致。调用时要持有 p.mu。
func (p *Presence) pushAll(typ string, data any) {
	if typ == "inventory_changed" {
		p.inventoryRevision++
	}
	if typ == "npc_window" {
		p.decorateWindow(data)
	}
	for _, m := range p.members {
		m.sess.Push(typ, data)
	}
}

// setController 改变操控方，并给在世界中的每条连接推送 control_changed（each 的 you 不同）。调用时要持有 p.mu。
// by 是引起这次变化的连接身份。返回要写入审计的事件，调用方在释放 p.mu 之后写。
func (p *Presence) setController(to protocol.Role, reason string, by protocol.Role) []auditEvent {
	p.controller = to
	ev := auditEvent{acctID: p.acct, charID: p.charID, role: string(by), kind: "control", request: reason, result: "ok", detail: "controller=" + string(to)}
	for role, m := range p.members {
		m.sess.Push("control_changed", map[string]any{"controller": string(to), "you": controlView(to, role)["you"], "reason": reason, "agent_online": p.members[protocol.RoleAgent] != nil})
		ev.account = m.sess.Account()
		if role == protocol.RoleAgent {
			ev.tokenID = m.sess.TokenID()
		}
	}
	if p.encounterOn { // STAGE-311：遇敌开关跟着控制方走，控制权转交就停掉连场；当前这一场照常打完
		p.stopEncounter(encounterOffControl)
	}
	if p.trade != nil && p.parties != nil { // STAGE-294：控制权被抢走，交易取消（docs/trade.md 2.9）。异步执行：这里持有角色锁，不能再取注册表锁
		go p.parties.s.trades.controlChanged(p)
	}
	return []auditEvent{ev}
}

// guard 是网络层的请求守卫：观察方发“操作”类请求一律拒绝。enter_world 和 leave_world 不受限制。
func (s *Service) guard(sess *gateway.Session, meta protocol.Meta) *protocol.Error {
	if meta.Category != protocol.CategoryOperation || meta.Type == "enter_world" || meta.Type == "leave_world" {
		return nil
	}
	l := s.LiveOf(sess)
	if l == nil {
		return nil
	}
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	if !l.left && l.p.controller != l.role {
		return controlError(l.role)
	}
	if l.p.trade != nil && !tradeAllowedDuringTrade(meta.Type) { // STAGE-294：交易中的操作拦截（docs/trade.md 2.5）
		return protocol.Errorf(protocol.CodeInTrade, "交易中，先完成或取消交易")
	}
	return nil
}

// tradeAllowedDuringTrade 是交易中仍可以发的操作类请求：交易本身的请求（trade_*）和离开世界。
func tradeAllowedDuringTrade(typ string) bool {
	switch typ {
	case "leave_world", "trade_invite", "trade_reply", "trade_cancel", "trade_offer", "trade_lock", "trade_unlock", "trade_confirm":
		return true
	}
	return false
}

// liveOrErr 取连接当前在世界中的句柄。
func (s *Service) liveOrErr(sess *gateway.Session) (*Live, error) {
	if l := s.LiveOf(sess); l != nil {
		return l, nil
	}
	return nil, protocol.Errorf(protocol.CodeWrongState, "不在世界中")
}

// asProtocolError 把“已经离开”转成协议错误，其他错误原样返回。
func asProtocolError(err error) error {
	if errors.Is(err, ErrLeft) {
		return protocol.Errorf(protocol.CodeWrongState, "不在世界中")
	}
	return err
}

func (s *Service) handleTakeControl(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	return s.changeControl(ctx, sess, protocol.RolePlayer, ReasonTakenByPlayer)
}

func (s *Service) handleReleaseControl(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	return s.changeControl(ctx, sess, protocol.RoleAgent, ReasonReleasedByPlayer)
}

// changeControl 是 take_control / release_control：agent 不在世界中回复 AGENT_OFFLINE；
// 已经是想要的状态时什么都不改，也不推送，直接回复成功。
func (s *Service) changeControl(ctx context.Context, sess *gateway.Session, to protocol.Role, reason string) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	p := l.p
	p.mu.Lock()
	if l.left {
		p.mu.Unlock()
		return nil, asProtocolError(ErrLeft)
	}
	if p.members[protocol.RoleAgent] == nil {
		p.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeAgentOffline, "agent 不在线，或者不在世界中")
	}
	var events []auditEvent
	if p.controller != to {
		events = p.setController(to, reason, l.role)
	}
	p.mu.Unlock()
	s.writeAudit(context.WithoutCancel(ctx), events)
	return map[string]any{}, nil
}

// handleKickAgent 让 agent 下线：先保存并交出控制权，再推送 KICKED_BY_PLAYER 断开它。令牌仍然有效。
func (s *Service) handleKickAgent(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	p := l.p
	p.mu.Lock()
	agent := p.members[protocol.RoleAgent]
	var ev auditEvent
	if agent != nil {
		ev = auditEvent{acctID: p.acct, account: sess.Account(), charID: p.charID, role: string(l.role), tokenID: agent.sess.TokenID(),
			kind: "control", request: protocol.ReasonKickedByPlayer, result: "ok"}
	}
	p.mu.Unlock()
	if agent == nil {
		return nil, protocol.Errorf(protocol.CodeAgentOffline, "agent 不在线，或者不在世界中")
	}
	_ = s.finish(agent, true) // 保存；agent 是操控方的话，控制权转回玩家（PEER_LEFT）
	agent.sess.Disconnect(protocol.ReasonKickedByPlayer, "")
	s.writeAudit(context.WithoutCancel(ctx), []auditEvent{ev})
	return map[string]any{}, nil
}
