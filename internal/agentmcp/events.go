package agentmcp

// 推送缓冲（STAGE-300）：没有 id 的推送先放进这里，下一次工具调用时随结果一起取出并清空。

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// maxEvents 是推送队列的上限，满了丢最旧的并记丢弃数（需求第 2 节）。
const maxEvents = 500

// event 是一条推送：type 用来匹配 wait_events，Raw 是服务端原样的 JSON 行。
type event struct {
	Type string
	Raw  json.RawMessage
}

type eventQueue struct {
	mu      sync.Mutex
	items   []event
	dropped int
	notify  chan struct{} // 有新推送时发一个信号，wait_events 用它醒来
	// recent 是上一次工具结果里已经交出去的推送类型。出招的结果里常常已经带着 battle_end，
	// 紧接着的 wait_events 要认得它，不能再傻等到超时。
	recent []string
	// waited 表示这次 drain 是 wait_events 的结果：它交出的推送不记进 recent，免得下一次 wait_events 立刻返回。
	waited bool
}

func newEventQueue() *eventQueue {
	return &eventQueue{notify: make(chan struct{}, 1)}
}

// push 加入一条推送；队列满了丢最旧的，并把丢弃计数加一。
func (q *eventQueue) push(e event) {
	q.mu.Lock()
	if len(q.items) >= maxEvents {
		q.items[0] = event{}
		q.items = q.items[1:]
		q.dropped++
	}
	q.items = append(q.items, e)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// drain 取出积压的全部推送和丢弃计数，并清空。
func (q *eventQueue) drain() ([]event, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	items, dropped := q.items, q.dropped
	q.items, q.dropped = nil, 0
	q.recent = nil
	if !q.waited {
		for _, e := range items {
			q.recent = append(q.recent, e.Type)
		}
	}
	q.waited = false
	return items, dropped
}

// takeRecent 返回上一次工具结果里已经交出去、且匹配 types 的推送类型，并清空记录。
// 连接断开的两种推送不算：它们会一直留在 fatal 错误里，不需要在这里重复。
func (q *eventQueue) takeRecent(types []string) []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var hit []string
	for _, t := range q.recent {
		if len(types) > 0 && matchType(t, types) && t != "disconnect" && t != "mcp_closed" {
			hit = append(hit, t)
		}
	}
	q.recent = nil
	q.waited = true
	return hit
}

// waitFor 等到有匹配 types 的推送，或超过 d、ctx 结束。返回是否匹配到。推送本身由调用方 drain 取出。
func (q *eventQueue) waitFor(ctx context.Context, types []string, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		if q.hasMatch(types) {
			return true
		}
		select {
		case <-q.notify:
		case <-timer.C:
			return q.hasMatch(types)
		case <-ctx.Done():
			return false
		}
	}
}

func (q *eventQueue) hasMatch(types []string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.items {
		if matchType(e.Type, types) {
			return true
		}
	}
	return false
}

// matchType 判断一条推送是否算匹配。types 为空时任何推送都匹配；
// 连接断开的两种推送（disconnect、mcp_closed）总是匹配，让 AI 在等待时就知道连接没了。
func matchType(typ string, types []string) bool {
	if typ == "disconnect" || typ == "mcp_closed" || len(types) == 0 {
		return true
	}
	for _, t := range types {
		if t == typ {
			return true
		}
	}
	return false
}
