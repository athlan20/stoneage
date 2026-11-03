// Package gateway 是服务端的网络层：监听 TCP，按 docs/protocol.md 收发 JSON Lines，
// 维护连接状态、握手、心跳、超时和权限检查，并把具体请求分发给各功能注册的处理函数。
package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// Options 是网络层的参数。零值字段取默认值。
type Options struct {
	ServerName       string        // hello 响应里的 server 字段
	ServerVersion    string        // hello 响应里的 server_version；空时取 ServerName 里 “/” 后面的部分（STAGE-188）
	DocsRev          string        // hello 响应里的 docs_rev：协议和手册的修订号（docs.Rev()，STAGE-188）
	HeartbeatSeconds int           // 默认 20
	IdleTimeout      time.Duration // 默认 60 秒
	WriteTimeout     time.Duration // 默认 5 秒
	MaxLine          int           // 默认 64 KiB
	RegisterEnabled  bool          // hello 响应里的 register_enabled
	Trace            io.Writer     // 非空时写协议日志（密码和令牌已替换为 ***）
	Logger           *slog.Logger
	// TrustedProxies 是可信代理的地址段。只有 WebSocket 请求的直连地址在其中时，才采信 CF-Connecting-IP（STAGE-101）。
	TrustedProxies []netip.Prefix
	// WSPingInterval 是 WebSocket 连接上服务端主动发 ping 的间隔，默认 30 秒，要小于 Cloudflare 的 100 秒空闲上限。
	WSPingInterval time.Duration
	// MinClientVersion 是最低客户端版本 x.y.z（STAGE-100）；空表示不检查。
	MinClientVersion string
	// LatestClientVersion 是最新客户端版本 x.y.z（STAGE-100）；空表示 hello 响应里不返回 latest_client。
	LatestClientVersion string
	// RateLimitMode 是按连接限速的模式（STAGE-351）：off 不检查，log 超限只记日志，enforce 超限回复 RATE_LIMITED。
	// 空值或不认识的取 log（不拒绝）。
	RateLimitMode string
	// RateLimitRate 是令牌桶每秒补充的令牌数，默认 20；RateLimitBurst 是桶的容量，默认 40。
	RateLimitRate  float64
	RateLimitBurst int
}

func (o *Options) fill() {
	switch o.RateLimitMode {
	case rateModeOff, rateModeLog, rateModeEnforce:
	default:
		o.RateLimitMode = rateModeLog
	}
	if o.RateLimitRate <= 0 {
		o.RateLimitRate = 20
	}
	if o.RateLimitBurst <= 0 {
		o.RateLimitBurst = 40
	}
	if o.ServerName == "" {
		o.ServerName = "stoneage-server/0.1.0"
	}
	if o.ServerVersion == "" {
		o.ServerVersion = o.ServerName
		if i := strings.LastIndex(o.ServerName, "/"); i >= 0 {
			o.ServerVersion = o.ServerName[i+1:]
		}
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 60 * time.Second
	}
	if o.HeartbeatSeconds <= 0 {
		o.HeartbeatSeconds = 20
		// 空闲时限很短时（测试用），心跳间隔要跟着缩短，否则客户端来不及发心跳。
		if third := int(o.IdleTimeout.Seconds() / 3); third >= 1 && third < o.HeartbeatSeconds {
			o.HeartbeatSeconds = third
		}
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = 5 * time.Second
	}
	if o.MaxLine <= 0 {
		o.MaxLine = protocol.MaxLineBytes
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.WSPingInterval <= 0 {
		o.WSPingInterval = 30 * time.Second
	}
}

// Handler 处理一种请求。返回的 data 成为成功响应的 data；
// 返回 *protocol.Error 会成为失败响应，其他错误按 INTERNAL 处理并记入日志。
// 同一条连接上的请求按到达顺序逐个调用，不会并发。ctx 在连接关闭（对端断开、被踢、超时、关服）时取消，
// 处理函数里的数据库查询、等待都应该用它。
type Handler func(ctx context.Context, s *Session, req *protocol.Request) (any, error)

// Server 是网络层的入口。
type Server struct {
	opts     Options
	log      *slog.Logger
	hub      *Hub
	trace    *tracer
	handlers map[string]Handler
	onClose  []func(*Session)
	guard    Guard
	onDone   []func(*Session, *protocol.Request, protocol.Meta, string)

	// baseCtx 是所有连接 ctx 的父级，Shutdown 时取消。
	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu      sync.Mutex
	ln      net.Listener
	wsHTTP  *http.Server // WebSocket 入口，ServeWS 时设置
	routes  []httpRoute  // 和 WebSocket 共用端口的普通 HTTP 路径，HandleHTTP 登记
	closing bool
	wg      sync.WaitGroup
	nextID  atomic.Uint64
}

// New 创建服务。hello 和 ping 已内置，其余请求由各功能调用 Handle 注册。
func New(opts Options) *Server {
	opts.fill()
	s := &Server{opts: opts, log: opts.Logger, hub: newHub(), handlers: map[string]Handler{}}
	s.baseCtx, s.baseCancel = context.WithCancel(context.Background())
	if opts.Trace != nil {
		s.trace = &tracer{w: opts.Trace}
	}
	s.handlers["hello"] = s.handleHello
	s.handlers["ping"] = handlePing
	return s
}

// Handle 注册某种请求的处理函数。type 必须已登记在 protocol.Requests 里，
// 这样每种请求的类别、状态和身份限制都有统一出处。必须在 Serve 之前调用。
func (s *Server) Handle(typ string, h Handler) {
	if _, ok := protocol.Lookup(typ); !ok {
		panic(fmt.Sprintf("gateway: 请求 %q 没有登记在 protocol.Requests 里", typ))
	}
	s.handlers[typ] = h
}

// Guard 在状态和身份检查通过之后、调用处理函数之前运行，返回非 nil 就拒绝这个请求。
// 控制权规则（观察方不能发“操作”类请求）靠它在网络层统一执行，具体的处理函数不用各自判断。
type Guard func(s *Session, meta protocol.Meta) *protocol.Error

// SetGuard 设置请求守卫，必须在 Serve 之前调用。
func (s *Server) SetGuard(g Guard) { s.guard = g }

// OnDone 注册请求处理完毕的回调（审计用）：每个认识的请求处理完（无论成功、失败还是被守卫拒绝）后调用一次，
// result 是 "ok" 或错误码。回调在处理这条请求的 goroutine 里同步执行，必须在 Serve 之前注册。
func (s *Server) OnDone(f func(sess *Session, req *protocol.Request, meta protocol.Meta, result string)) {
	s.onDone = append(s.onDone, f)
}

// OnClose 注册连接结束时的回调（正常断开、被踢、超时、关服都会调用，每条连接一次），
// 用来保存数据、清理这条连接占用的资源。回调在连接自己的 goroutine 里按注册顺序执行，
// 此时连接已经不再接收请求；关服时 Shutdown 会等回调执行完。必须在 Serve 之前调用。
func (s *Server) OnClose(f func(*Session)) { s.onClose = append(s.onClose, f) }

// httpRoute 是 HandleHTTP 登记的一条路径。
type httpRoute struct {
	pattern string
	handler http.Handler
}

// HandleHTTP 登记一个普通 HTTP 路径（如 /protocol.md、/wiki/），和 WebSocket 入口共用同一个端口。
// 路径规则同 http.ServeMux（以 / 结尾的是前缀）。必须在 ServeWS 之前调用；/ws 由网络层占用，不能登记。
func (s *Server) HandleHTTP(pattern string, h http.Handler) {
	if pattern == WSPath {
		panic("gateway: " + WSPath + " 已被 WebSocket 入口占用")
	}
	s.routes = append(s.routes, httpRoute{pattern: pattern, handler: h})
}

// Hub 返回连接注册表，用来按账号找连接、向观看者推送。
func (s *Server) Hub() *Hub { return s.hub }

// Options 返回生效的参数（已填入默认值）。
func (s *Server) Options() Options { return s.opts }

// Serve 在 l 上接受连接，直到 l 被关闭。每条连接独立处理，互不影响。
func (s *Server) Serve(l net.Listener) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return net.ErrClosed
	}
	s.ln = l
	s.mu.Unlock()
	for {
		conn, err := l.Accept()
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing || errors.Is(err, net.ErrClosed) {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			s.log.Error("接受连接失败", "err", err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if !s.admit(conn, conn.RemoteAddr()) {
			return nil
		}
	}
}

// admit 登记一条新连接并开始处理。登记和 Shutdown 开始关服用同一把锁：关服开始后才到的连接直接关掉，
// 开始之前登记的连接一定能被 Shutdown 看到并通知。服务正在关闭时关掉连接并返回 false。
// remote 是玩家的地址：WebSocket 经过代理时是 CF-Connecting-IP 里的真实地址。
func (s *Server) admit(conn net.Conn, remote net.Addr) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		_ = conn.Close()
		return false
	}
	s.wg.Add(1)
	sess := newSession(s, conn, s.nextID.Add(1), remote)
	s.hub.add(sess)
	go func() {
		defer s.wg.Done()
		sess.log.Debug("连接已建立")
		sess.run()
		sess.log.Debug("连接已关闭")
	}()
	return true
}

// Shutdown 停止接受新连接，给所有连接推送 SERVER_SHUTDOWN 并断开，等它们处理完。
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	ln, wsHTTP := s.ln, s.wsHTTP
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if wsHTTP != nil {
		_ = wsHTTP.Close()
	}
	for _, sess := range s.hub.Sessions() {
		sess.Disconnect(protocol.ReasonServerShutdown, "")
	}
	s.baseCancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
