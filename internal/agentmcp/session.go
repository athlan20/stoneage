package agentmcp

// 会话（STAGE-300）：连接、握手、登录、断线后的重连，以及不能自动重连的断开原因。
// 规则沿用 docs/agent-guide.md 第 3 节和 docs/protocol.md 第 9 节，这里只负责把它们落到 MCP 服务上。

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

const (
	clientVersion      = "stoneage-mcp/0.1.0" // hello 的 client 字段（x.y.z 版本号，服务端据此做最低版本检查）
	maxConnectFailures = 3                    // 连续失败这么多次后不再自动重连（需求第 2 节）
	defaultHeartbeat   = 20 * time.Second     // 服务端没给 heartbeat_seconds 时的兜底
)

// fatalReasons 是不能自动重连的断开原因（需求第 2 节，docs/agent-guide.md 第 3 节）。
var fatalReasons = map[string]bool{
	"KICKED_DUPLICATE_LOGIN": true,
	"KICKED_BY_PLAYER":       true,
	"AGENT_TOKEN_REVOKED":    true,
	"ACCOUNT_DISABLED":       true,
	"INVALID_TOKEN":          true,
}

// fatalGuides 是每种不可重连原因的应对说明，工具返回错误时带上它。
var fatalGuides = map[string]string{
	"KICKED_DUPLICATE_LOGIN": "同一账号的另一条 agent 连接登录了，本连接被挤下线。不要重连：停止操作，用 say_to_player 告诉玩家，等玩家指示。",
	"KICKED_BY_PLAYER":       "玩家让 agent 下线。不要重连：停止操作，等玩家告诉你再继续。",
	"AGENT_TOKEN_REVOKED":    "令牌已被吊销、过期，或超过在线时长上限。不要用这个令牌再登录：停止操作，等玩家重新给令牌。",
	"ACCOUNT_DISABLED":       "账号已被禁用。停止操作，不再登录。",
	"INVALID_TOKEN":          "令牌不存在、已过期或已被吊销。停止操作，等玩家重新给令牌。",
}

// Session 是 MCP 服务到游戏服务端的一个会话。第一次需要时才连接（ensure），断线后下一次需要时再连。
type Session struct {
	cfg    Config
	log    *protoLog
	events *eventQueue
	ids    atomic.Uint64

	connMu sync.Mutex // 串行化连接和重连，同一时间只拨一次号
	mu     sync.Mutex // 保护下面的状态
	w      *wire      // 当前连接，nil 表示没有
	// failures 是连续连接失败的次数（拨号、握手或登录失败），连上后清零。
	failures   int
	fatal      *toolErr // 不可自动重连的原因；有了就一直返回它，直到进程重启
	inWorld    bool
	account    string
	docsNotice string // 下一次工具结果里的 docs_updated，取出后清空
}

func newSession(cfg Config) *Session {
	return &Session{
		cfg:    cfg,
		log:    &protoLog{w: cfg.LogFile, token: cfg.Token},
		events: newEventQueue(),
	}
}

func (s *Session) newID() uint64 { return s.ids.Add(1) }

// ensure 返回一条可用的连接；没有就拨号、握手、登录。
func (s *Session) ensure(ctx context.Context) (*wire, *toolErr) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.mu.Lock()
	if s.fatal != nil {
		e := s.fatal
		s.mu.Unlock()
		return nil, e
	}
	if s.w != nil && !s.w.isDead() {
		w := s.w
		s.mu.Unlock()
		return w, nil
	}
	s.w = nil
	if s.failures >= maxConnectFailures {
		s.mu.Unlock()
		return nil, &toolErr{Code: "RECONNECT_LIMIT", Message: "连续 3 次连接失败，已停止自动重连。请检查服务端是否可用，然后重启 stoneage-mcp。"}
	}
	s.mu.Unlock()

	w, e := s.open(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	if e != nil {
		s.failures++
		return nil, e
	}
	s.failures = 0
	s.w = w
	return w, nil
}

// open 拨号并完成 hello 和 agent_login，成功后启动心跳。失败时关掉这条线。
func (s *Session) open(ctx context.Context) (*wire, *toolErr) {
	lc, err := dialLine(ctx, s.cfg.Server)
	if err != nil {
		return nil, &toolErr{Code: "CONNECT_FAILED", Message: "连接服务端失败：" + s.log.scrub(err.Error())}
	}
	w := newWire(s, lc)
	go w.readLoop()
	fail := func(e *toolErr) (*wire, *toolErr) {
		w.quiet.Store(true)
		w.shutdown("CONNECTION_LOST", e.Message)
		return nil, e
	}

	m, e := w.call(ctx, s.newID(), "hello", map[string]any{
		"protocol": 1,
		"client":   clientVersion,
		"role":     protocol.RoleAgent,
		"compress": []string{protocol.CompressGzip},
	})
	if e != nil {
		return fail(e)
	}
	if !okOf(m) {
		return fail(errOf(m))
	}
	var h struct {
		HeartbeatSeconds int    `json:"heartbeat_seconds"`
		DocsRev          string `json:"docs_rev"`
	}
	_ = json.Unmarshal(m.Data, &h)
	interval := time.Duration(h.HeartbeatSeconds) * time.Second
	if interval <= 0 {
		interval = defaultHeartbeat
	}
	if h.DocsRev != "" {
		s.checkDocs(h.DocsRev)
	}

	m, e = w.call(ctx, s.newID(), "agent_login", map[string]any{"token": s.cfg.Token})
	if e != nil {
		return fail(e)
	}
	if !okOf(m) {
		le := errOf(m)
		if le.Code == "INVALID_TOKEN" || le.Code == "ACCOUNT_DISABLED" {
			s.setFatal(le.Code, le.Message)
		}
		return fail(le)
	}
	var l struct {
		Account string `json:"account"`
	}
	_ = json.Unmarshal(m.Data, &l)
	s.mu.Lock()
	s.account = l.Account
	s.mu.Unlock()
	go w.heartbeat(interval)
	return w, nil
}

// do 发一个请求（必要时先连接），成功返回响应的 data，失败返回工具错误。
func (s *Session) do(ctx context.Context, typ string, data any) (json.RawMessage, *toolErr) {
	w, e := s.ensure(ctx)
	if e != nil {
		return nil, e
	}
	m, e := w.call(ctx, s.newID(), typ, data)
	if e != nil {
		return nil, e
	}
	if !okOf(m) {
		return nil, errOf(m)
	}
	s.track(typ)
	return m.Data, nil
}

// track 记下进入和离开世界的结果，connect 用它判断是否已经在世界里。
func (s *Session) track(typ string) {
	switch typ {
	case "enter_world":
		s.mu.Lock()
		s.inWorld = true
		s.mu.Unlock()
	case "leave_world":
		s.mu.Lock()
		s.inWorld = false
		s.mu.Unlock()
	}
}

func (s *Session) isInWorld() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inWorld
}

// setFatal 记下不可自动重连的原因。只记第一次。
func (s *Session) setFatal(reason, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fatal != nil {
		return
	}
	guide := fatalGuides[reason]
	if guide == "" {
		guide = "这条连接不能自动重连。停止操作，等玩家指示。"
	}
	text := guide + "（只有重启 stoneage-mcp 进程后才会再次尝试。）"
	if msg != "" {
		text += " 服务端说明：" + msg
	}
	s.fatal = &toolErr{Code: reason, Message: text}
}

// lost 在连接断开时清理状态，并在没有 disconnect 推送时补一条 mcp_closed 推送，让 AI 知道断了。
func (s *Session) lost(w *wire, reason, msg string) {
	s.mu.Lock()
	if s.w == w {
		s.w = nil
		s.inWorld = false
	}
	s.mu.Unlock()
	if w.quiet.Load() || w.disconnected.Load() {
		return
	}
	raw, _ := json.Marshal(map[string]any{"type": "mcp_closed", "data": map[string]any{"reason": reason, "message": msg}})
	s.events.push(event{Type: "mcp_closed", Raw: raw})
}

// close 主动断开当前连接（MCP 服务退出时用）。
func (s *Session) close() {
	s.mu.Lock()
	w := s.w
	s.mu.Unlock()
	if w != nil {
		w.quiet.Store(true)
		w.shutdown("CLOSED", "stoneage-mcp 已退出")
	}
}

func okOf(m message) bool { return m.OK != nil && *m.OK }

// errOf 把失败的响应转成工具错误。
func errOf(m message) *toolErr {
	if m.Error == nil {
		return &toolErr{Code: "BAD_RESPONSE", Message: "服务端返回失败，但没有错误信息"}
	}
	return &toolErr{Code: m.Error.Code, Message: m.Error.Message, Data: m.Error.Data}
}
