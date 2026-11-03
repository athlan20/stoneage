package gateway

import (
	"context"
	"errors"
	"runtime/debug"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// handleLine 处理一行请求，返回 false 表示连接应当结束。
func (s *Session) handleLine(ctx context.Context, line []byte) bool {
	req, perr, fatal := protocol.ParseRequest(line)
	if perr != nil {
		s.reply(req, nil, perr)
		return !fatal
	}
	data, err := s.dispatch(ctx, req)
	s.notifyDone(ctx, req, err)
	if err != nil {
		var pe *protocol.Error
		if ctx.Err() != nil && !errors.As(err, &pe) {
			return false // 连接已经关闭，处理中途放弃，不是服务端出错
		}
		if !errors.As(err, &pe) {
			s.log.Error("处理请求出错", "type", req.Type, "err", err)
			pe = protocol.Errorf(protocol.CodeInternal, "%s", protocol.MessageInternal)
		}
		s.reply(req, nil, pe)
		return !pe.CloseAfter
	}
	s.reply(req, data, nil)
	return true
}

// notifyDone 通知审计回调；连接已经关闭而中途放弃的请求不算。
func (s *Session) notifyDone(ctx context.Context, req *protocol.Request, err error) {
	if len(s.srv.onDone) == 0 {
		return
	}
	meta, ok := protocol.Lookup(req.Type)
	if !ok {
		return
	}
	result := "ok"
	if err != nil {
		var pe *protocol.Error
		switch {
		case errors.As(err, &pe):
			result = pe.Code
		case ctx.Err() != nil:
			return
		default:
			result = protocol.CodeInternal
		}
	}
	for _, f := range s.srv.onDone {
		f(s, req, meta, result)
	}
}

func (s *Session) reply(req *protocol.Request, data any, perr *protocol.Error) {
	if perr != nil {
		s.sendRaw(protocol.EncodeError(req.ID, req.Type, perr))
		return
	}
	s.sendRaw(protocol.EncodeOK(req.ID, req.Type, data))
}

// dispatch 依次做状态、身份检查，再调用处理函数。
func (s *Session) dispatch(ctx context.Context, req *protocol.Request) (data any, err error) {
	state := s.State()
	// 握手之前只能发 hello；发别的请求回复 WRONG_STATE 并断开。
	if state == protocol.StateConnected && req.Type != "hello" {
		return nil, protocol.Errorf(protocol.CodeWrongState, "请先发送 hello 完成握手").Closing()
	}
	meta, ok := protocol.Lookup(req.Type)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeUnknownType, "不认识的请求类型：%s", req.Type)
	}
	if !meta.AllowsRole(s.Role()) {
		return nil, protocol.Errorf(protocol.CodeWrongRole, "%s 连接不能发送 %s 请求", roleName(s.Role()), req.Type)
	}
	if !meta.AllowsState(state) {
		return nil, protocol.Errorf(protocol.CodeWrongState, "当前状态（%s）不允许 %s 请求", state, req.Type)
	}
	if s.srv.guard != nil {
		if perr := s.srv.guard(s, meta); perr != nil {
			return nil, perr
		}
	}
	h := s.srv.handlers[req.Type]
	if h == nil {
		return nil, protocol.Errorf(protocol.CodeUnknownType, "服务端暂未开放 %s 请求", req.Type)
	}
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("请求处理崩溃", "type", req.Type, "panic", r, "stack", string(debug.Stack()))
			data, err = nil, protocol.Errorf(protocol.CodeInternal, "%s", protocol.MessageInternal)
		}
	}()
	return h(ctx, s, req)
}

func roleName(r protocol.Role) string {
	if r == protocol.RoleAgent {
		return "agent"
	}
	return "玩家"
}

// hello 请求的 data。
type helloRequest struct {
	Protocol *int     `json:"protocol"`
	Client   string   `json:"client"`
	Role     string   `json:"role"`
	Compress []string `json:"compress"` // STAGE-246：客户端能接收的压缩方式，如 ["gzip"]；不声明就不压缩
}

func (s *Server) handleHello(_ context.Context, sess *Session, req *protocol.Request) (any, error) {
	var in helloRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Protocol == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "hello 缺少 protocol 字段")
	}
	role := protocol.Role(in.Role)
	switch role {
	case "":
		role = protocol.RolePlayer
	case protocol.RolePlayer, protocol.RoleAgent:
	default:
		return nil, protocol.Errorf(protocol.CodeBadRequest, "role 只能是 player 或 agent")
	}
	if *in.Protocol != protocol.Version {
		return nil, protocol.Errorf(protocol.CodeProtocolVersion, "协议版本不一致，服务端支持的版本：%d", protocol.Version).
			WithData(map[string]any{"supported": []int{protocol.Version}}).Closing()
	}
	if protocol.ClientTooOld(in.Client, s.opts.MinClientVersion) {
		cur, _ := protocol.ClientVersion(in.Client)
		return nil, protocol.Errorf(protocol.CodeClientTooOld, "当前版本 %s 过旧，需要升级到 %s 以上，运行 stoneage update", cur, s.opts.MinClientVersion).
			WithData(map[string]any{"min_client": s.opts.MinClientVersion}).Closing()
	}
	sess.mu.Lock()
	sess.role, sess.client, sess.state = role, in.Client, protocol.StateHandshaken
	sess.mu.Unlock()
	data := map[string]any{
		"protocol":             protocol.Version,
		"server":               s.opts.ServerName,
		"server_version":       s.opts.ServerVersion,
		"docs_rev":             s.opts.DocsRev,
		"heartbeat_seconds":    s.opts.HeartbeatSeconds,
		"idle_timeout_seconds": int(s.opts.IdleTimeout.Seconds()),
		"register_enabled":     s.opts.RegisterEnabled,
	}
	// STAGE-246：确认压缩协商。hello 响应本身远小于阈值，不会被压缩。
	if c := sess.negotiateCompress(in.Compress); c != "" {
		data["compress"] = c
	}
	// 版本要求只在配置了时才返回（STAGE-100）。
	if s.opts.MinClientVersion != "" {
		data["min_client"] = s.opts.MinClientVersion
	}
	if s.opts.LatestClientVersion != "" {
		data["latest_client"] = s.opts.LatestClientVersion
	}
	return data, nil
}

func handlePing(context.Context, *Session, *protocol.Request) (any, error) {
	return map[string]any{"server_time_ms": time.Now().UnixMilli()}, nil
}
