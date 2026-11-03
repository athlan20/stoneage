package auth

// agent 在线时长限制（STAGE-118）：配置 STONEAGE_AGENT_MAX_ONLINE_MINUTES 后，agent 登录满这么久就被断开，
// 走和令牌吊销相同的路径（先保存、控制权按 PEER_LEFT 转回玩家、推送 AGENT_TOKEN_REVOKED）。默认 0 不限制。

import (
	"context"
	"time"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// agentConn 是判断在线时长用到的连接信息；gateway.Session 满足它，测试可以换成假的连接。
type agentConn interface {
	Role() protocol.Role
	Account() string
	TokenID() string
	LoginAt() time.Time
}

// overdueAgents 返回已登录、身份是 agent、并且登录满 limit 的连接。未登录（没有登录时间）的不算。
func overdueAgents[S agentConn](sessions []S, now time.Time, limit time.Duration) []S {
	var out []S
	for _, sess := range sessions {
		at := sess.LoginAt()
		if sess.Role() != protocol.RoleAgent || sess.Account() == "" || at.IsZero() {
			continue
		}
		if now.Sub(at) >= limit {
			out = append(out, sess)
		}
	}
	return out
}

// kickOvertimeAgents 断开超过在线时长上限的 agent。限制为 0 时什么都不做。
// 每次检查最多晚一个检查周期（默认 60 秒）才断开，和令牌到期的检查共用同一个定时器。
func (s *Service) kickOvertimeAgents(ctx context.Context) {
	if s.opts.AgentMaxOnline <= 0 {
		return
	}
	for _, sess := range overdueAgents(s.hub.Sessions(), s.opts.Now(), s.opts.AgentMaxOnline) {
		s.kick([]*gateway.Session{sess}, protocol.ReasonAgentTokenRevoked)
		s.audit(context.WithoutCancel(ctx), "agent_online_limit", sess.Account(), "", true, sess.TokenID())
	}
}
