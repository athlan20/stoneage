package character

import (
	"sync"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// agentBoard 是一个账号的 agent 面板：计划和实际活动（docs/protocol.md 9.7，STAGE-119）。
// 只在内存里：服务端重启后清空；agent 断开后保留，下一次 agent 登录时清空。
type agentBoard struct {
	mu    sync.Mutex
	plan  agentPlan
	items []activityItem // 最多 agentActivityMax 条，旧的在前
}

// agentActivityMax 是每个账号保存的活动条数上限。
const agentActivityMax = 200

// agentBoardOf 取账号的面板，没有就新建；账号 id 为 0 不会走到这里（调用方保证已登录）。
func (s *Service) agentBoardOf(acct int64) *agentBoard {
	s.boards.mu.Lock()
	defer s.boards.mu.Unlock()
	if s.boards.m == nil {
		s.boards.m = map[int64]*agentBoard{}
	}
	b := s.boards.m[acct]
	if b == nil {
		b = &agentBoard{plan: emptyPlan()}
		s.boards.m[acct] = b
	}
	return b
}

// agentBoards 保存全部账号的面板，锁只保护 map 本身。
type agentBoards struct {
	mu sync.Mutex
	m  map[int64]*agentBoard
}

func (b *agentBoard) setPlan(p agentPlan) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.plan = p
}

// reset 清空计划和活动（agent 每次登录都从空白开始），返回清空前是否有计划、是否有活动：
// 只有原来非空时才需要推送清空，原来就是空的不必再推。
func (b *agentBoard) reset() (hadPlan, hadItems bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	hadPlan, hadItems = b.plan.Goal != "", len(b.items) > 0
	b.plan = emptyPlan()
	b.items = nil
	return hadPlan, hadItems
}

// append 追加一条活动，超出上限时丢掉最旧的。
func (b *agentBoard) append(item activityItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, item)
	if over := len(b.items) - agentActivityMax; over > 0 {
		b.items = append([]activityItem(nil), b.items[over:]...)
	}
}

// snapshot 返回计划和活动的副本，给补发用；活动为空时返回空数组而不是 null。
func (b *agentBoard) snapshot() (agentPlan, []activityItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := make([]activityItem, len(b.items))
	copy(items, b.items)
	return b.plan, items
}

// pushToPlayers 把推送发给同一账号、已登录的玩家连接（世界中或不在世界中都算）。
func (s *Service) pushToPlayers(account, typ string, data any) {
	if account == "" {
		return
	}
	for _, sess := range s.hub.ByAccount(account) {
		if sess.Role() == protocol.RolePlayer && sess.State() >= protocol.StateLoggedIn {
			sess.Push(typ, data)
		}
	}
}

// agentLive 找同一账号里除 except 以外、已登录的 agent 连接；有的话返回它的令牌编号。
func (s *Service) agentLive(account string, except *gateway.Session) (string, bool) {
	if account == "" {
		return "", false
	}
	for _, sess := range s.hub.ByAccount(account) {
		if sess != except && sess.Role() == protocol.RoleAgent && sess.State() >= protocol.StateLoggedIn {
			return sess.TokenID(), true
		}
	}
	return "", false
}

// agentStatusView 是 agent_status 的 data：online 为假时 token_id 为 null。
func agentStatusView(online bool, token string) map[string]any {
	var tok any
	if online && token != "" {
		tok = token
	}
	return map[string]any{"online": online, "token_id": tok}
}

// observeDone 在 agent 登录成功、玩家进入世界成功时补发或清空面板（docs/protocol.md 9.7 的“补发和清空的时机”）。
// 推送在请求的响应之前发出，和 control_changed 一样。
func (s *Service) observeDone(sess *gateway.Session, req *protocol.Request, _ protocol.Meta, result string) {
	if result != "ok" {
		return
	}
	switch {
	case req.Type == "agent_login" && sess.Role() == protocol.RoleAgent:
		hadPlan, hadItems := s.agentBoardOf(sess.AccountID()).reset()
		s.pushToPlayers(sess.Account(), "agent_status", agentStatusView(true, sess.TokenID()))
		if hadPlan {
			s.pushToPlayers(sess.Account(), "agent_plan", emptyPlan())
		}
		if hadItems {
			s.pushToPlayers(sess.Account(), "agent_activity", map[string]any{"full": true, "items": []activityItem{}})
		}
	case req.Type == "enter_world" && sess.Role() == protocol.RolePlayer:
		s.backfillPlayer(sess)
	}
}

// agentStatusOf 是这个账号 agent 的在线状态（和 agent_status 的 data 一样），放在 enter_world 的响应里：
// 玩家进入世界时的在线状态靠响应给，不靠推送。
func (s *Service) agentStatusOf(account string) map[string]any {
	token, online := s.agentLive(account, nil)
	return agentStatusView(online, token)
}

// backfillPlayer 玩家进入世界时补发计划和全部活动，只发给这一条连接；没有的不推送，客户端的默认状态就是空。
// 在线状态已经放在 enter_world 的响应里（agentStatusOf）。
func (s *Service) backfillPlayer(sess *gateway.Session) {
	plan, items := s.agentBoardOf(sess.AccountID()).snapshot()
	if plan.Goal != "" {
		sess.Push("agent_plan", plan)
	}
	if len(items) > 0 {
		sess.Push("agent_activity", map[string]any{"full": true, "items": items})
	}
}

// observeClose 在连接结束时处理 agent 下线：同账号没有别的已登录 agent 连接时，推送 online:false。
// 同账号的新连接先登录再踢旧连接（auth/kick.go），所以旧连接断开时新连接已经算在线，不会误推下线。
func (s *Service) observeClose(sess *gateway.Session) {
	if sess.Role() != protocol.RoleAgent || sess.AccountID() == 0 {
		return
	}
	if _, online := s.agentLive(sess.Account(), sess); online {
		return
	}
	s.pushToPlayers(sess.Account(), "agent_status", agentStatusView(false, ""))
}
