package character

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// 世界中的基础请求（docs/protocol.md 5.12、5.13）。移动、战斗等玩法还没有做，先用最小的一对
// “查看”和“操作”请求让控制权规则能被完整验证：status 是查看类，turn（转向）是操作类。

func (s *Service) handleStatus(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := l.With(func(r *record) { out = worldView(r, l.p, l.role, s.now()) }); err != nil {
		return nil, asProtocolError(err)
	}
	if l.p.realParty.Load() != nil { // 真人队（STAGE-274）
		out["party"] = s.partyRealField(l.p)
	}
	return out, nil
}

func (s *Service) handleTurn(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Direction *int `json:"direction"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Direction == nil || *in.Direction < 0 || *in.Direction > 7 {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "direction 要在 0–7 之间（0 为北，顺时针每次加 45°）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var battleErr *protocol.Error
	if err := l.Operate(func(r *record) {
		if perr := inBattle(l); perr != nil { // 战斗中不能转向（docs/protocol.md 10.1）
			battleErr = perr
			return
		}
		r.Direction = *in.Direction
		// 角色的变化同时推给玩家连接和 agent 连接，两边看到的一致。
		l.p.pushAll("position_changed", r.position())
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if battleErr != nil {
		return nil, battleErr
	}
	return map[string]any{}, nil
}

const agentSayMax = 200

// handleAgentSay 是玩家和 agent 之间的私聊（docs/protocol.md 9.5）：只在同一账号的玩家连接和 agent 连接之间传递，
// 双方只要登录了就能私聊，不要求在世界中。
func (s *Service) handleAgentSay(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Text *string `json:"text"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Text == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 text 字段")
	}
	text := strings.TrimSpace(*in.Text)
	if text == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "消息不能为空")
	}
	if n := utf8.RuneCountInString(text); n > agentSayMax {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "消息最长 %d 个字符，现在有 %d 个", agentSayMax, n)
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "消息不能包含控制字符")
		}
	}
	delivered := 0
	for _, peer := range s.hub.ByAccount(sess.Account()) {
		if peer.Role() == sess.Role() || peer.State() < protocol.StateLoggedIn {
			continue
		}
		peer.Push("agent_message", map[string]any{"from": string(sess.Role()), "text": text, "time_ms": time.Now().UnixMilli()})
		delivered++
	}
	if delivered == 0 {
		return nil, protocol.Errorf(protocol.CodePeerOffline, "对方不在线")
	}
	return map[string]any{}, nil
}
