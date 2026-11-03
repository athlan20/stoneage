package agentmcp

// 一条已连上的线（STAGE-300）：读循环、按 id 把响应交回等待者、心跳、战斗回放确认。

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// requestTimeout 是单个请求等待响应的上限（需求第 2 节）。
const requestTimeout = 15 * time.Second

// message 是服务端发来的一行：有 id 且有 ok 的是响应，其余是推送。
// server/protocol 里的响应结构是未导出的，这里照同样的字段镜像一份，只用来解码。
type message struct {
	ID    uint64          `json:"id"`
	Type  string          `json:"type"`
	OK    *bool           `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *wireError      `json:"error"`
}

type wireError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// wire 是一条连接。响应按 id 交给等待它的调用；推送进入 Session 的队列。
type wire struct {
	s            *Session
	lc           lineConn
	wmu          sync.Mutex // 串行化写入
	mu           sync.Mutex // 保护 pending
	pending      map[uint64]chan message
	dead         chan struct{}
	once         sync.Once
	quiet        atomic.Bool // 为 true 时断开不推 mcp_closed（握手失败、退出时主动断开）
	disconnected atomic.Bool // 收到过服务端的 disconnect 推送（它自己已经说明了原因）
}

func newWire(s *Session, lc lineConn) *wire {
	return &wire{s: s, lc: lc, pending: map[uint64]chan message{}, dead: make(chan struct{})}
}

func (w *wire) isDead() bool {
	select {
	case <-w.dead:
		return true
	default:
		return false
	}
}

// register 登记一个等待响应的 id；连接已经断开时返回 false。
func (w *wire) register(id uint64) (chan message, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.isDead() {
		return nil, false
	}
	ch := make(chan message, 1)
	w.pending[id] = ch
	return ch, true
}

func (w *wire) unregister(id uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.pending, id)
}

// deliver 把响应交给等待它的调用；没有等待者（如战斗确认）的响应直接丢弃。
func (w *wire) deliver(m message) {
	w.mu.Lock()
	ch, ok := w.pending[m.ID]
	delete(w.pending, m.ID)
	w.mu.Unlock()
	if ok {
		ch <- m
	}
}

// write 发出一条请求。日志记录的是脱敏后的内容（protocol.Redact 会隐去 token）。
func (w *wire) write(ctx context.Context, id uint64, typ string, data any) error {
	if data == nil {
		data = map[string]any{}
	}
	line, err := json.Marshal(map[string]any{"id": id, "type": typ, "data": data})
	if err != nil {
		return err
	}
	w.s.log.record("->", line)
	w.wmu.Lock()
	defer w.wmu.Unlock()
	return w.lc.WriteLine(ctx, line)
}

// call 发一个请求并等它的响应。连接断开、超时、ctx 结束都返回工具错误。
func (w *wire) call(ctx context.Context, id uint64, typ string, data any) (message, *toolErr) {
	ch, ok := w.register(id)
	if !ok {
		return message{}, lostError("连接已断开")
	}
	defer w.unregister(id)
	if err := w.write(ctx, id, typ, data); err != nil {
		return message{}, lostError("发送失败：" + w.s.log.scrub(err.Error()))
	}
	timer := time.NewTimer(requestTimeout)
	defer timer.Stop()
	select {
	case m := <-ch:
		return m, nil
	case <-w.dead:
		select {
		case m := <-ch:
			return m, nil
		default:
		}
		return message{}, lostError("等待响应时连接断开")
	case <-timer.C:
		return message{}, &toolErr{Code: "TIMEOUT", Message: fmt.Sprintf("%s 等待响应超时（%d 秒）", typ, int(requestTimeout.Seconds()))}
	case <-ctx.Done():
		return message{}, &toolErr{Code: "CANCELLED", Message: typ + " 已取消"}
	}
}

func lostError(msg string) *toolErr {
	return &toolErr{Code: "CONNECTION_LOST", Message: msg}
}

// readLoop 持续读取服务端的行，直到连接断开。
func (w *wire) readLoop() {
	reason, msg := "CONNECTION_LOST", "连接已断开"
	defer func() { w.shutdown(reason, msg) }()
	for {
		line, err := w.lc.ReadLine()
		if err != nil {
			msg = "连接已断开：" + w.s.log.scrub(err.Error())
			return
		}
		// 压缩行（STAGE-246）：这里还原成原文再解析，之后的代码看不出它原来是压缩的。
		raw, perr := protocol.DecodeLine(line)
		if perr != nil {
			reason, msg = "PROTOCOL_ERROR", perr.Message
			return
		}
		w.s.log.record("<-", raw)
		var m message
		if json.Unmarshal(raw, &m) != nil {
			continue // 不是 JSON 对象的行直接忽略（协议规定收到不认识的内容不算错误）
		}
		if m.ID != 0 && m.OK != nil {
			w.deliver(m)
			continue
		}
		w.onPush(m, raw)
	}
}

// onPush 处理没有 id 的推送：断开原因、战斗回放确认，然后放进队列。
func (w *wire) onPush(m message, raw []byte) {
	switch m.Type {
	case "disconnect":
		w.disconnected.Store(true)
		var d struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(m.Data, &d)
		if fatalReasons[d.Reason] {
			w.s.setFatal(d.Reason, d.Message)
		}
	case "battle_round":
		// 战斗回放播完即确认（docs/protocol.md 10.8）：由 MCP 服务自动发，AI 不需要管。
		// 最后一回合服务端紧接着推 battle_end，确认迟到也按成功处理（STAGE-282）。
		var d struct {
			Turn int `json:"turn"`
		}
		_ = json.Unmarshal(m.Data, &d)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
			defer cancel()
			_ = w.write(ctx, w.s.newID(), "battle_ack", map[string]any{"turn": d.Turn})
		}()
	}
	w.s.events.push(event{Type: m.Type, Raw: append(json.RawMessage(nil), raw...)})
}

// shutdown 关闭这条连接，只生效一次。
func (w *wire) shutdown(reason, msg string) {
	w.once.Do(func() {
		// 先记下断线的推送，再标记死亡：等待中的调用醒来时，mcp_closed 已经在队列里，随它的结果一起返回。
		w.s.lost(w, reason, msg)
		close(w.dead)
		_ = w.lc.Close()
	})
}

// heartbeat 按服务端给的间隔发 ping（docs/protocol.md 5.2），连接断开后结束。
func (w *wire) heartbeat(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-w.dead:
			return
		case <-t.C:
			_, _ = w.call(context.Background(), w.s.newID(), "ping", map[string]any{})
		}
	}
}
