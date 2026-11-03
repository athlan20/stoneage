package gateway

import (
	"sync"

	"github.com/athlan20/stoneage/server/protocol"
)

// Hub 记录所有在线连接，并支持“一个键多个观看者”的推送：
// 同一个角色可以同时被玩家连接和 agent 连接观看，对这个角色的推送要同时发给它们（docs/protocol.md 9.3）。
type Hub struct {
	mu       sync.RWMutex
	sessions map[uint64]*Session
	watchers map[string]map[*Session]struct{}
}

func newHub() *Hub {
	return &Hub{sessions: map[uint64]*Session{}, watchers: map[string]map[*Session]struct{}{}}
}

func (h *Hub) add(s *Session) {
	h.mu.Lock()
	h.sessions[s.id] = s
	h.mu.Unlock()
}

func (h *Hub) remove(s *Session) {
	h.mu.Lock()
	delete(h.sessions, s.id)
	for key, set := range h.watchers {
		delete(set, s)
		if len(set) == 0 {
			delete(h.watchers, key)
		}
	}
	h.mu.Unlock()
}

// Count 返回当前在线连接数。
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions)
}

// Sessions 返回当前所有连接的快照。
func (h *Hub) Sessions() []*Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Session, 0, len(h.sessions))
	for _, s := range h.sessions {
		out = append(out, s)
	}
	return out
}

// ByAccount 返回已登录为 account 的所有连接（玩家连接和 agent 连接各至多一条）。
func (h *Hub) ByAccount(account string) []*Session {
	var out []*Session
	for _, s := range h.Sessions() {
		if s.Account() == account {
			out = append(out, s)
		}
	}
	return out
}

// Watch 让连接开始观看 key（例如某个角色）；连接断开时自动取消。
func (h *Hub) Watch(key string, s *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sessions[s.id]; !ok {
		return
	}
	set := h.watchers[key]
	if set == nil {
		set = map[*Session]struct{}{}
		h.watchers[key] = set
	}
	set[s] = struct{}{}
}

// Unwatch 取消观看。
func (h *Hub) Unwatch(key string, s *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.watchers[key]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.watchers, key)
		}
	}
}

// Watchers 返回正在观看 key 的连接。
func (h *Hub) Watchers(key string) []*Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Session, 0, len(h.watchers[key]))
	for s := range h.watchers[key] {
		out = append(out, s)
	}
	return out
}

// Publish 把一条推送同时发给所有观看 key 的连接，返回发送的连接数。
func (h *Hub) Publish(key, typ string, data any) int {
	line := protocol.EncodePush(typ, data)
	watchers := h.Watchers(key)
	for _, s := range watchers {
		s.sendRaw(line)
	}
	return len(watchers)
}
