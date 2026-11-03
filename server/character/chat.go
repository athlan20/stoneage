package character

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/wordfilter"
)

// STAGE-278：公开聊天 chat / chat_message（docs/protocol.md 5.68，设计见 docs/chat.md）。
// 聊天类请求：不受控制权限限制，战斗中也能发；推送只给玩家连接（agent 连接收不到公开聊天）。

type chatRequest struct {
	Channel *string `json:"channel"`
	Text    *string `json:"text"`
}

// chatMsg 是通过检查、要推送的一条消息（屏蔽词已替换）。
type chatMsg struct {
	from    string
	level   int
	floor   int
	text    string
	members []*Presence // 队伍频道的收件人（真人队成员，STAGE-277）
}

func (s *Service) handleChat(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var in chatRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	channel := chatChannelMap
	if in.Channel != nil {
		channel = *in.Channel
	}
	if channel != chatChannelMap && channel != chatChannelWorld && channel != chatChannelParty {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "频道要是 map、world 或 party")
	}
	if in.Text == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 text 字段")
	}
	text, perr := chatCleanText(*in.Text)
	if perr != nil {
		return nil, perr
	}
	now := s.opts.Now()
	until, err := s.chatMutedUntil(ctx, l.p.charID)
	if err != nil {
		return nil, err
	}
	if t, muted := chatMuteActive(until, now); muted {
		return nil, protocol.Errorf(protocol.CodeChatMuted, "你被管理员禁言了，到 %s 解除", t.Format("15:04:05")).
			WithData(map[string]any{"until_ms": t.UnixMilli()})
	}
	msg, perr := s.chatAdmit(l, channel, text, now, s.chatConfig())
	if perr != nil {
		return nil, perr
	}
	if msg == nil { // 同一句 10 秒内重复：静默丢弃，响应照常成功
		return map[string]any{}, nil
	}
	data := map[string]any{"channel": channel, "from": msg.from, "level": msg.level, "text": msg.text, "time_ms": now.UnixMilli()}
	if channel == chatChannelParty {
		pushPartyChat(msg.members, data)
	} else {
		s.pushChat(l.p, channel, msg.floor, data)
	}
	// 只审计 agent 的公开发言（5.68/9.6）；玩家自己的发言不写库。
	if sess.Role() == protocol.RoleAgent {
		s.writeAudit(context.WithoutCancel(ctx), []auditEvent{{
			acctID: sess.AccountID(), account: sess.Account(), charID: l.p.charID, role: string(sess.Role()), tokenID: sess.TokenID(),
			kind: "chat", request: "chat", result: "ok",
			detail: fmt.Sprintf("channel=%s by=%s text=%s", channel, sess.Role(), msg.text),
		}})
	}
	return map[string]any{}, nil
}

// chatMutedUntil 读角色的禁言解除时间（nil 表示没有禁言）。每次发言都读库，管理员的禁言立即生效，不用通知。
func (s *Service) chatMutedUntil(ctx context.Context, charID int64) (*time.Time, error) {
	var until *time.Time
	err := s.db.QueryRow(ctx, `SELECT chat_muted_until FROM characters WHERE id = $1`, charID).Scan(&until)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return until, err
}

// chatAdmit 在角色锁里检查等级、频率和重复，并登记这次推送。返回 nil 消息表示静默丢弃（重复）。
func (s *Service) chatAdmit(l *Live, channel, text string, now time.Time, cfg chatConfig) (*chatMsg, *protocol.Error) {
	p := l.p
	p.mu.Lock()
	defer p.mu.Unlock()
	if l.left {
		return nil, protocol.Errorf(protocol.CodeWrongState, "不在世界中")
	}
	r := &p.rec
	var members []*Presence
	if channel == chatChannelParty {
		ms, perr := s.chatPartyMembers(p)
		if perr != nil {
			return nil, perr
		}
		members = ms
	}
	if channel == chatChannelWorld && r.Level < cfg.WorldMinLevel {
		return nil, protocol.Errorf(protocol.CodeChatLevelLow, "世界频道需要 %d 级", cfg.WorldMinLevel).
			WithData(map[string]any{"min_level": cfg.WorldMinLevel})
	}
	cooldown := cfg.MapInterval
	if channel == chatChannelWorld {
		cooldown = cfg.WorldCooldown
	}
	if wait := chatWait(p.chatSent, channel, now, cooldown); wait > 0 {
		return nil, protocol.Errorf(protocol.CodeRateLimited, "说得太快了，请稍后再试").
			WithData(map[string]any{"retry_after_seconds": chatRetrySeconds(wait)})
	}
	masked := wordfilter.Current().Mask(text) // STAGE-287：归一化后匹配，见 server/wordfilter
	if chatIsDuplicate(p.chatSent, channel, masked, now) {
		return nil, nil
	}
	if p.chatSent == nil {
		p.chatSent = map[string]chatSent{}
	}
	p.chatSent[channel] = chatSent{at: now, text: masked}
	return &chatMsg{from: r.Name, level: r.Level, floor: r.Floor, text: masked, members: members}, nil
}

// pushChat 把消息推给收件人的玩家连接（发送者自己的玩家连接也收到，agent 连接不收）：
// map 是同一张地图（floor）上在世界中的角色，world 是全服在世界中的角色。调用时不持有任何锁。
func (s *Service) pushChat(self *Presence, channel string, floor int, data map[string]any) {
	self.pushPlayerConn("chat_message", data)
	if channel == chatChannelMap {
		for _, e := range s.players.onFloor(floor, self.charID) {
			e.p.pushPlayerConn("chat_message", data)
		}
		return
	}
	s.mu.Lock()
	others := make([]*Presence, 0, len(s.presences))
	for _, p := range s.presences {
		if p != self {
			others = append(others, p)
		}
	}
	s.mu.Unlock()
	for _, p := range others {
		p.pushPlayerConn("chat_message", data)
	}
}

// pushPlayerConn 只把推送发给角色的玩家连接（不发给 agent 连接）。
func (p *Presence) pushPlayerConn(typ string, data any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m := p.members[protocol.RolePlayer]; m != nil {
		m.sess.Push(typ, data)
	}
}
