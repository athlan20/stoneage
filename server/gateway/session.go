package gateway

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

const outQueueSize = 256

// Session 是一条客户端连接。请求在读循环里按到达顺序逐个处理；
// 推送可以从任意 goroutine 发出，由单独的写 goroutine 按入队顺序写出。
type Session struct {
	id     uint64
	srv    *Server
	conn   net.Conn
	remote net.Addr // 玩家的地址，WebSocket 经过代理时不是 conn 的对端地址
	log    *slog.Logger

	out  chan outMsg
	done chan struct{}
	once sync.Once
	// compress 在 hello 协商出 gzip 后打开（STAGE-246），之后超过阈值的消息压缩发送。
	compress atomic.Bool
	// ctx 在连接关闭（对端断开、被踢、空闲超时、关服）时取消，请求处理函数都用它，
	// 这样排队等待的数据库查询和密码计算会立即放弃。
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	state     protocol.State
	role      protocol.Role
	client    string
	account   string
	accountID int64
	tokenID   string    // agent 连接登录用的令牌编号
	loginAt   time.Time // 登录成功的时间，agent 在线时长限制用（STAGE-118）
}

func newSession(srv *Server, conn net.Conn, id uint64, remote net.Addr) *Session {
	ctx, cancel := context.WithCancel(srv.baseCtx)
	return &Session{
		ctx: ctx, cancel: cancel,
		id: id, srv: srv, conn: conn, remote: remote,
		log:   srv.log.With("session", id, "remote", remote.String()),
		out:   make(chan outMsg, outQueueSize),
		done:  make(chan struct{}),
		state: protocol.StateConnected,
		role:  protocol.RolePlayer,
	}
}

func (s *Session) ID() uint64                { return s.id }
func (s *Session) Server() *Server           { return s.srv }
func (s *Session) RemoteAddr() net.Addr      { return s.remote }
func (s *Session) Done() <-chan struct{}     { return s.done }
func (s *Session) State() protocol.State     { s.mu.RLock(); defer s.mu.RUnlock(); return s.state }
func (s *Session) Role() protocol.Role       { s.mu.RLock(); defer s.mu.RUnlock(); return s.role }
func (s *Session) Client() string            { s.mu.RLock(); defer s.mu.RUnlock(); return s.client }
func (s *Session) Account() string           { s.mu.RLock(); defer s.mu.RUnlock(); return s.account }
func (s *Session) SetState(v protocol.State) { s.mu.Lock(); s.state = v; s.mu.Unlock() }
func (s *Session) AccountID() int64          { s.mu.RLock(); defer s.mu.RUnlock(); return s.accountID }
func (s *Session) SetAccountID(v int64)      { s.mu.Lock(); s.accountID = v; s.mu.Unlock() }
func (s *Session) SetAccount(v string)       { s.mu.Lock(); s.account = v; s.mu.Unlock() }
func (s *Session) TokenID() string           { s.mu.RLock(); defer s.mu.RUnlock(); return s.tokenID }
func (s *Session) SetTokenID(v string)       { s.mu.Lock(); s.tokenID = v; s.mu.Unlock() }
func (s *Session) LoginAt() time.Time        { s.mu.RLock(); defer s.mu.RUnlock(); return s.loginAt }
func (s *Session) SetLoginAt(v time.Time)    { s.mu.Lock(); s.loginAt = v; s.mu.Unlock() }

// Push 发一条服务端推送。
func (s *Session) Push(typ string, data any) {
	s.sendRaw(protocol.EncodePush(typ, data))
}

// Disconnect 推送 disconnect 后关闭连接。message 为空时用 reason 对应的默认说明。
func (s *Session) Disconnect(reason, message string) {
	if message == "" {
		message = protocol.DisconnectMessage(reason)
	}
	s.Push("disconnect", protocol.Disconnect{Reason: reason, Message: message})
	s.Close()
}

// Close 关闭连接：先把已排队的消息写完再关。可重复调用。
func (s *Session) Close() {
	s.once.Do(func() { close(s.done); s.cancel() })
}

func (s *Session) sendRaw(line []byte) {
	select {
	case <-s.done:
		return
	default:
	}
	m := s.packOut(line)
	// STAGE-258：线上超过 64 KiB 的消息不发，改由 oversize.go 处理（响应改发错误，推送丢弃），连接不断。
	if lineBytes(m.wire) > protocol.MaxLineBytes {
		s.oversizeMsg(m)
		return
	}
	s.enqueue(m)
}

// enqueue 把一条已打包的消息放进发送队列。
func (s *Session) enqueue(m outMsg) {
	select {
	case s.out <- m:
		s.srv.trace.log("→", s.id, m.plain, lineBytes(m.plain))
	default:
		// 客户端读得太慢，队列满了：断开这一条连接，不拖累其他人。
		s.log.Warn("发送队列已满，断开连接", s.msgAttrs(m)...)
		s.Close()
	}
}

func (s *Session) writeLoop() {
	defer s.conn.Close()
	// STAGE-246：写失败记原因，发出的超大消息记一条日志（诊断用，见 session_diag.go）。
	write := func(m outMsg) bool {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.srv.opts.WriteTimeout))
		if _, err := s.conn.Write(m.wire); err != nil {
			s.logWriteFailure(m, err)
			return false
		}
		if lineBytes(m.plain) > bigMessageBytes {
			s.logBigMessage(m)
		}
		return true
	}
	for {
		select {
		case m := <-s.out:
			if !write(m) {
				s.Close()
				return
			}
		case <-s.done:
			for {
				select {
				case m := <-s.out:
					if !write(m) {
						return
					}
				default:
					return
				}
			}
		}
	}
}

// run 是连接的主循环，返回时连接已关闭。
func (s *Session) run() {
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); s.writeLoop() }()
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("连接处理崩溃，只断开这一条连接", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		s.serveRequests()
	}()
	s.Close()    // 通知写循环：把排队的消息写完后关闭连接
	<-writerDone // 等写循环退出（连接此时已关闭）
	for _, f := range s.srv.onClose {
		s.runOnClose(f)
	}
	s.srv.hub.remove(s)
}

// runOnClose 调用一个关闭回调；回调崩溃只记日志，不影响其他回调和服务。
func (s *Session) runOnClose(f func(*Session)) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("连接关闭回调崩溃", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	f(s)
}

// inbound 是读 goroutine 交给主循环的一项：一行请求，或读取结束的原因。
type inbound struct {
	line []byte
	err  error
}

// inboundQueue 是读 goroutine 最多提前读几行。队列满了就停止读取，由 TCP 的流控挡住发得太快的客户端。
const inboundQueue = 16

// serveRequests 按到达顺序逐个处理请求。读取在单独的 goroutine 里进行，这样处理请求期间
// 也能发现对端已经断开，并取消 ctx，让正在排队的查询和密码计算立即放弃。
func (s *Session) serveRequests() {
	in := make(chan inbound, inboundQueue)
	go s.readLines(in)
	for item := range in {
		if s.ctx.Err() != nil {
			return
		}
		switch {
		case item.err == nil:
			if len(trimSpace(item.line)) == 0 {
				continue
			}
			// STAGE-246：压缩行先解压再处理；协议日志只记原文，不记 base64，免得密码以编码形式进日志。
			line, perr := protocol.DecodeLine(item.line)
			if perr != nil {
				s.log.Warn("收到的压缩消息有误，断开连接", "reason", perr.Message, "bytes", len(item.line))
				s.srv.trace.log("←", s.id, []byte("<无法解压的压缩行>"), len(item.line))
				s.sendRaw(protocol.EncodeError(0, protocol.UnknownType, perr))
				return
			}
			s.srv.trace.log("←", s.id, line, len(line))
			if !s.handleLine(s.ctx, line) {
				return
			}
		case errors.Is(item.err, errLineTooLong):
			s.srv.trace.log("←", s.id, []byte("<超长的行>"), -1)
			s.sendRaw(protocol.EncodeError(0, protocol.UnknownType, protocol.Errorf(protocol.CodeBadRequest, "一行消息不能超过 %d KiB", s.srv.opts.MaxLine/1024)))
			return
		case isTimeout(item.err):
			s.Disconnect(protocol.ReasonIdleTimeout, "")
			return
		}
	}
}

// readLines 不停读取请求行交给主循环。对端断开或读出错时取消 ctx；
// 行太长、空闲超时则把原因按顺序交给主循环，让前面已收到的请求先处理完。
func (s *Session) readLines(in chan<- inbound) {
	defer close(in)
	send := func(it inbound) bool {
		select {
		case in <- it:
			return true
		case <-s.ctx.Done():
			return false
		}
	}
	br := bufio.NewReaderSize(s.conn, 4096)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.srv.opts.IdleTimeout))
		line, err := readLine(br, s.srv.opts.MaxLine)
		switch {
		case err == nil:
			if !send(inbound{line: line}) {
				return
			}
		case errors.Is(err, errLineTooLong) || isTimeout(err):
			send(inbound{err: err})
			return
		default:
			s.cancel() // 对端已经断开
			return
		}
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t' || b[len(b)-1] == '\r' || b[len(b)-1] == '\n') {
		b = b[:len(b)-1]
	}
	return b
}
