package character

// STAGE-294：交易的全服注册表、推送和超时（docs/trade.md 2.2–2.5，docs/protocol.md 5.72）。
// 锁顺序：tradeHub.mu → 角色的 Presence.mu（两个角色按 charID 从小到大）。调用时不能持有任何角色的锁。
// 注册表只在内存里，服务重启全部消失（同组队请求）。

import (
	"sort"
	"sync"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

const (
	tradeSettingKey  = "trade.enabled"
	tradeRequestTTL  = 30 * time.Second // 邀请有效期（协议 5.72）
	tradeRateWindow  = 10 * time.Second // 同一发起人对同一对象的最短间隔
	tradeMaxIncoming = 5                // 每个角色同时待处理的交易请求上限
)

// tradeRequest 是一条待答复的交易邀请；被接受后请求变成交易，id 沿用（docs/trade.md 2.2）。
type tradeRequest struct {
	id                   string
	from, to             *Presence
	fromID, toID         int64
	fromName, toName     string
	fromLevel, fromTrans int
	expires              time.Time
}

// tradeHub 是全服的交易状态。requests 和 byChar 只在 mu 下读写。
type tradeHub struct {
	s        *Service
	mu       sync.Mutex
	seq      int
	requests map[string]*tradeRequest
	sent     map[pairKey]time.Time
	byChar   map[int64]*trade // 角色编号 → 它正在进行的交易
	timer    *time.Timer
}

func newTradeHub(s *Service) *tradeHub {
	return &tradeHub{s: s, requests: map[string]*tradeRequest{}, sent: map[pairKey]time.Time{}, byChar: map[int64]*trade{}}
}

// tradeGate 是交易的总开关检查：关闭时回 WRONG_STATE（协议 5.72）。
func (s *Service) tradeGate() *protocol.Error {
	if !s.tradeOn.Load() {
		return protocol.Errorf(protocol.CodeWrongState, "交易尚未开放")
	}
	return nil
}

// tradeInfo 是一次请求里读到的角色状态（在角色锁下读取，之后不再持锁）。
type tradeInfo struct {
	p       *Presence
	id      int64
	name    string
	floor   int
	level   int
	trans   int
	dead    bool
	battle  bool
	real    bool // 有真人队
	noTrade bool // 拒绝交易邀请（social_trade 关）
	hidden  bool
	trade   *trade
	clone   bool
}

func tradeInfoOf(p *Presence) tradeInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return tradeInfo{
		p: p, id: p.charID, name: p.rec.Name, floor: p.rec.Floor, level: p.rec.Level, trans: p.rec.Transmigration,
		dead: p.rec.HP <= 0, battle: p.battle != nil, real: p.realParty.Load() != nil,
		noTrade: p.rec.SocialNoTrade, hidden: p.hidden.Load(), trade: p.trade, clone: p.party != nil,
	}
}

// pushTo 在角色锁下推送（给不持有任何锁的调用方用）。
func pushTo(p *Presence, typ string, data any) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.pushAll(typ, data)
	p.mu.Unlock()
}

// lockPresences 按 charID 从小到大锁住几个角色，返回解锁函数。调用方保证不会传入同一个角色两次。
func lockPresences(ps ...*Presence) func() {
	sort.Slice(ps, func(i, j int) bool { return ps[i].charID < ps[j].charID })
	for _, p := range ps {
		p.mu.Lock()
	}
	return func() {
		for i := len(ps) - 1; i >= 0; i-- {
			ps[i].mu.Unlock()
		}
	}
}

// attachLocked 把交易挂到两个角色上（p.trade）并登记到 byChar。调用时持有 h.mu。
func (h *tradeHub) attachLocked(tr *trade) {
	a, b := tr.Sides[0].P, tr.Sides[1].P
	unlock := lockPresences(a, b)
	a.trade, b.trade = tr, tr
	unlock()
	h.byChar[tr.Sides[0].CharID] = tr
	h.byChar[tr.Sides[1].CharID] = tr
}

// detachLocked 把交易从两个角色和注册表上摘下。调用时持有 h.mu。
func (h *tradeHub) detachLocked(tr *trade) {
	a, b := tr.Sides[0].P, tr.Sides[1].P
	unlock := lockPresences(a, b)
	a.trade, b.trade = nil, nil
	unlock()
	delete(h.byChar, tr.Sides[0].CharID)
	delete(h.byChar, tr.Sides[1].CharID)
}

// finishLocked 结束一笔交易（取消或完成）：摘下、推 trade_ended。调用时持有 h.mu。
// codes 是每一方看到的 code（nil 表示没有）；summaries 只在完成时非空，按 CharID 给出每一方自己视角的摘要。
// STAGE-294：两方各拿各的 summary（甲的 gave 是甲给出的，乙的 gave 是乙给出的），不能共用一份。
func (h *tradeHub) finishLocked(tr *trade, status, by string, codes map[int64]any, summaries map[int64]map[string]any) {
	h.detachLocked(tr)
	for _, side := range tr.Sides {
		data := map[string]any{"id": tr.ID, "status": status, "by": nil, "code": codes[side.CharID], "summary": nil}
		if by != "" {
			data["by"] = by
		}
		if summary := summaries[side.CharID]; summary != nil {
			data["summary"] = summary
		}
		pushTo(side.P, "trade_ended", data)
	}
}

// cancelLocked 取消交易。who 是发起取消的角色编号（0 表示系统取消，如超时、断线），
// 发起人看不到 code，对方看到 peerCode（例如 CANCELLED、PEER_LEFT、TIMEOUT）。
func (h *tradeHub) cancelLocked(tr *trade, who int64, byName string, peerCode string) {
	codes := map[int64]any{}
	for _, side := range tr.Sides {
		if side.CharID == who {
			codes[side.CharID] = nil
		} else {
			codes[side.CharID] = peerCode
		}
	}
	h.finishLocked(tr, "cancelled", byName, codes, nil)
}

// tradeSideView 是报价的一方（协议 5.72 的 me / peer）。mine 为 true 时带背包格号和宠物栏位。
func tradeSideView(s *tradeSide, mine bool) map[string]any {
	s.P.mu.Lock()
	defer s.P.mu.Unlock()
	items := []map[string]any{}
	for _, slot := range s.Offer.Items {
		it := findItem(s.P.rec.Items, slot)
		if it == nil {
			continue
		}
		tpl, _ := gamedata.ItemByID(it.ItemID)
		v := map[string]any{
			"item_id": it.ItemID, "name": tpl.Name, "effect": tpl.Effect, "count": it.Count,
			"durability": it.Durability, "max_durability": it.MaxDurability, "crush_level": it.CrushLevel,
		}
		if mine {
			v["slot"] = slot
		}
		items = append(items, v)
	}
	var pet any
	if s.Offer.Pet >= 0 {
		for i := range s.P.rec.Pets {
			p := &s.P.rec.Pets[i]
			if p.Slot != s.Offer.Pet {
				continue
			}
			tpl, _ := gamedata.PetByID(p.TemplateID)
			v := map[string]any{"name": p.Name, "species": tpl.Name, "level": p.Level, "transmigration": p.Transmigration}
			if mine {
				v["slot"] = p.Slot
				v["state"] = petStateNames[p.State]
			}
			pet = v
		}
	}
	return map[string]any{
		"name": s.Name, "items": items, "gold": s.Offer.Gold, "pet": pet,
		"locked": s.Locked, "confirmed": s.Confirmed,
	}
}

// tradeViewFor 是交易对象，按 me 的视角（协议 5.72）。
func tradeViewFor(tr *trade, me int64, now time.Time) map[string]any {
	var lastErr any
	if e := tr.LastErr; e != nil {
		who := "peer"
		if e.Who == me {
			who = "me"
		}
		lastErr = map[string]any{"code": e.Code, "who": who}
	}
	return map[string]any{
		"id": tr.ID, "rev": tr.Rev, "idle_expires_in": tr.idleExpiresIn(now),
		"me":         tradeSideView(tr.side(me), true),
		"peer":       tradeSideView(tr.peer(me), false),
		"last_error": lastErr,
	}
}

// armLocked 有待处理的请求或进行中的交易时挂上单个扫描定时器。调用时持有 h.mu。
func (h *tradeHub) armLocked() {
	if h.timer != nil || (len(h.requests) == 0 && len(h.byChar) == 0) {
		return
	}
	h.timer = time.AfterFunc(time.Second, h.sweep)
}

// sweep 作废到期的邀请（expired，双方推送），并取消空闲超时的交易（TIMEOUT）。测试可以直接调用。
func (h *tradeHub) sweep() {
	now := h.s.opts.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timer = nil
	for id, r := range h.requests {
		if !now.Before(r.expires) {
			delete(h.requests, id)
			data := map[string]any{"id": id, "status": "expired", "from": r.fromName, "to": r.toName}
			pushTo(r.from, "trade_request_result", data)
			pushTo(r.to, "trade_request_result", data)
		}
	}
	seen := map[*trade]bool{}
	for _, tr := range h.byChar {
		if seen[tr] || !tr.expired(now) {
			continue
		}
		seen[tr] = true
		h.cancelLocked(tr, 0, "", "TIMEOUT")
	}
	h.armLocked()
}

// expireTrades 立即执行一次超时扫描（测试用，不用真等）。
func (s *Service) expireTrades() { s.trades.sweep() }

// controlChanged 在控制权被 take、release 或 agent 进入时调用（不持有角色锁）：正在进行的交易以 CONTROL_CHANGED 取消，不转交给 agent。
func (h *tradeHub) controlChanged(p *Presence) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if tr := h.byChar[p.charID]; tr != nil {
		h.cancelLocked(tr, 0, "", "CONTROL_CHANGED")
	}
	h.armLocked()
}

// onGone 在角色的最后一条连接离开世界后调用（断线、离开）：待处理的邀请作废，进行中的交易以 PEER_LEFT 取消（docs/trade.md 2.5）。
// 调用时不能持有角色锁。
func (h *tradeHub) onGone(p *Presence) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, r := range h.requests {
		if r.from == p || r.to == p {
			delete(h.requests, id)
			other := r.to
			if r.to == p {
				other = r.from
			}
			pushTo(other, "trade_request_result", map[string]any{"id": id, "status": "cancelled", "from": r.fromName, "to": r.toName})
		}
	}
	if tr := h.byChar[p.charID]; tr != nil {
		h.cancelLocked(tr, p.charID, "", "PEER_LEFT")
	}
	h.armLocked()
}
