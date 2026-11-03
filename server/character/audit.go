package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// auditEvent 是要写入 agent_audit 的一行（docs/protocol.md 9.6）：账号、角色、连接身份、令牌编号、请求类型、结果、时间。
type auditEvent struct {
	acctID  int64
	account string
	charID  int64
	role    string
	tokenID string
	kind    string // request | control
	request string
	result  string
	detail  string
}

func (s *Service) writeAudit(ctx context.Context, events []auditEvent) {
	for _, e := range events {
		var charID any
		if e.charID != 0 {
			charID = e.charID
		}
		_, err := s.db.Exec(ctx, `INSERT INTO agent_audit (at, account_id, account, character_id, role, token_id, kind, request, result, detail)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			s.opts.Now(), e.acctID, e.account, charID, e.role, e.tokenID, e.kind, e.request, e.result, e.detail)
		if err != nil {
			s.opts.Logger.Error("写 agent 审计失败", "err", err)
		}
	}
}

// auditRequest 是网络层的请求完成回调：agent 连接发起的每个“操作”类请求（不论成功还是被拒绝）都写一条审计。
func (s *Service) auditRequest(sess *gateway.Session, req *protocol.Request, meta protocol.Meta, result string) {
	if sess.Role() != protocol.RoleAgent || meta.Category != protocol.CategoryOperation || sess.AccountID() == 0 {
		return
	}
	var charID int64
	if l := s.LiveOf(sess); l != nil {
		charID = l.p.charID
	}
	s.writeAudit(context.Background(), []auditEvent{{
		acctID: sess.AccountID(), account: sess.Account(), charID: charID, role: string(sess.Role()), tokenID: sess.TokenID(),
		kind: "request", request: req.Type, result: result,
	}})
	s.recordActivity(sess, req, result) // 实际活动（STAGE-119）和审计在同一个位置记
}
