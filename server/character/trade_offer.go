package character

// STAGE-294：交易中的报价、锁定、解锁、确定（docs/trade.md 2.3，docs/protocol.md 5.72）。
// 报价用背包和宠物的内存数据做第一道校验（格子存在、石币够、宠物在且不骑乘）；锁定时的预演和确定后的交换读数据库。

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// tradeOfferKeys 是 trade_offer 允许的字段（协议 5.72）；其他字段回复 BAD_REQUEST。
var tradeOfferKeys = map[string]bool{"id": true, "items": true, "gold": true, "pet": true}

// handleTradeOffer 是 trade_offer：整体替换报价（省略的字段不改），任何修改 rev +1、双方清零。
func (s *Service) handleTradeOffer(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.tradeGate(); perr != nil {
		return nil, perr
	}
	var keys map[string]json.RawMessage
	if perr := req.Decode(&keys); perr != nil {
		return nil, perr
	}
	for k := range keys {
		if !tradeOfferKeys[k] {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "trade_offer 不认识字段 %s", k)
		}
	}
	var in struct {
		ID    string          `json:"id"`
		Items *[]int          `json:"items"`
		Gold  *int64          `json:"gold"`
		Pet   json.RawMessage `json:"pet"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.ID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字段 id（交易编号）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := tradeInfoOf(l.p)
	h := s.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	tr := h.byChar[me.id]
	if tr == nil || tr.ID != in.ID {
		return nil, protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	cur := tr.side(me.id).Offer
	next := tradeOffer{Items: append([]int(nil), cur.Items...), Gold: cur.Gold, Pet: cur.Pet}
	if in.Items != nil {
		next.Items = append([]int{}, (*in.Items)...)
	}
	if in.Gold != nil {
		next.Gold = *in.Gold
	}
	if len(in.Pet) > 0 {
		if string(in.Pet) == "null" {
			next.Pet = -1
		} else {
			n, perr := strconv.Atoi(string(in.Pet))
			if perr != nil {
				return nil, protocol.Errorf(protocol.CodeBadRequest, "%s", fmt.Sprintf("pet 要是宠物栏位（0–%d）或 null", tradeMaxPetSlot))
			}
			next.Pet = n
		}
	}
	if perr := checkOfferShape(next); perr != nil { // 形状错误（BAD_REQUEST）先于背包检查
		return nil, perr
	}
	if perr := tradeHoldingCheck(l.p, next); perr != nil {
		return nil, perr
	}
	now := s.opts.Now()
	if perr := tr.setOffer(me.id, next, now); perr != nil {
		return nil, perr
	}
	peer := tr.peer(me.id)
	pushTo(peer.P, "trade_updated", map[string]any{"trade": tradeViewFor(tr, peer.CharID, now)})
	return map[string]any{"trade": tradeViewFor(tr, me.id, now)}, nil
}

// tradeHoldingCheck 是报价的第一道校验：用自己内存里的背包和宠物（docs/trade.md 2.6 的“物品存在”“石币够”“宠物不骑乘”）。
func tradeHoldingCheck(p *Presence, o tradeOffer) *protocol.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, slot := range o.Items {
		if findItem(p.rec.Items, slot) == nil {
			return protocol.Errorf(protocol.CodeEmptySlot, "%d 号格没有物品", slot)
		}
	}
	if o.Gold > p.rec.Gold {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "石币不够")
	}
	if o.Pet >= 0 {
		pet := findPet(p.rec.Pets, o.Pet)
		if pet == nil {
			return protocol.Errorf(protocol.CodeEmptySlot, "%d 号宠物栏没有宠物", o.Pet)
		}
		if pet.State == petStateRide {
			return protocol.Errorf(protocol.CodePetRiding, "骑乘中的宠物不能交易，先下骑")
		}
	}
	return nil
}

// tradeRevReq 是 trade_lock 和 trade_confirm 的请求体。
type tradeRevReq struct {
	ID  string `json:"id"`
	Rev *int   `json:"rev"`
}

// tradeOfTrade 解析 id 和 rev，取出自己正在进行的交易（调用时持有 h.mu）。
func (h *tradeHub) tradeOfTrade(charID int64, id string) (*trade, *protocol.Error) {
	tr := h.byChar[charID]
	if tr == nil || tr.ID != id {
		return nil, protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	return tr, nil
}

// handleTradeLock 是 trade_lock：带 rev 锁定，锁定前预演（不过则拒绝，说明是谁的问题）。
func (s *Service) handleTradeLock(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	return s.tradeLockOrUnlock(sess, req, true)
}

// handleTradeUnlock 是 trade_unlock：取消自己的锁定，双方回到编辑阶段。
func (s *Service) handleTradeUnlock(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	return s.tradeLockOrUnlock(sess, req, false)
}

func (s *Service) tradeLockOrUnlock(sess *gateway.Session, req *protocol.Request, lock bool) (any, error) {
	if perr := s.tradeGate(); perr != nil {
		return nil, perr
	}
	var in tradeRevReq
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.ID == "" || (lock && in.Rev == nil) {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字段 id（交易编号）；trade_lock 还需要 rev")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := tradeInfoOf(l.p)
	h := s.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	tr, perr := h.tradeOfTrade(me.id, in.ID)
	if perr != nil {
		return nil, perr
	}
	now := s.opts.Now()
	before := tr.Rev
	if lock {
		perr = tr.lock(me.id, *in.Rev, now, func() *protocol.Error {
			return s.tradePreflightErr(tr, me.id)
		})
	} else {
		perr = tr.unlock(me.id, now)
	}
	if perr != nil {
		return nil, perr
	}
	peer := tr.peer(me.id)
	if tr.Rev != before || lock {
		pushTo(peer.P, "trade_updated", map[string]any{"trade": tradeViewFor(tr, peer.CharID, now)})
	}
	return map[string]any{"trade": tradeViewFor(tr, me.id, now)}, nil
}

// tradePreflightErr 把预演的失败转成协议错误（见 tradeFailErr）。
func (s *Service) tradePreflightErr(tr *trade, me int64) *protocol.Error {
	return tradeFailErr(s.tradePreflight(tr), me)
}

// tradeFailErr 把校验失败转成协议错误：who 按锁定方的视角写成 "me" 或 "peer"（docs/trade.md 2.6）。
// STAGE-294：拆出来成纯函数，单测不用数据库也能验文案。
func tradeFailErr(fail *tradeFail, me int64) *protocol.Error {
	if fail == nil {
		return nil
	}
	who := "peer"
	if fail.who == me {
		who = "me"
	}
	if fail.code == protocol.CodeBagFull || fail.code == protocol.CodeGoldFull || fail.code == protocol.CodePetFull {
		// 对方的满格只说“对方”，不泄露具体格子数（docs/trade.md 2.6）
		msg := "对方物品栏已满"
		if who == "me" {
			msg = fail.msg
		}
		if fail.code == protocol.CodeGoldFull && who == "peer" {
			// STAGE-294：对方石币超上限不是物品栏的事，单独说“石币”（docs/trade.md 2.6）
			msg = "对方石币已达上限"
		}
		if fail.code == protocol.CodePetFull {
			msg = "宠物栏已满"
			if who == "peer" {
				msg = "对方宠物栏已满"
			}
		}
		return protocol.Errorf(fail.code, "%s", msg).WithData(map[string]any{"who": who})
	}
	return fail.err().WithData(map[string]any{"who": who})
}

// handleTradeConfirm 是 trade_confirm：只在双方都锁定后可用，带 rev。第二个确定的人触发交换。
func (s *Service) handleTradeConfirm(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	if perr := s.tradeGate(); perr != nil {
		return nil, perr
	}
	var in tradeRevReq
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.ID == "" || in.Rev == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要字段 id（交易编号）和 rev")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	me := tradeInfoOf(l.p)
	h := s.trades
	h.mu.Lock()
	defer h.mu.Unlock()
	tr, perr := h.tradeOfTrade(me.id, in.ID)
	if perr != nil {
		return nil, perr
	}
	now := s.opts.Now()
	done, perr := tr.confirm(me.id, *in.Rev, now)
	if perr != nil {
		return nil, perr
	}
	peer := tr.peer(me.id)
	if !done {
		pushTo(peer.P, "trade_updated", map[string]any{"trade": tradeViewFor(tr, peer.CharID, now)})
		return map[string]any{"trade": tradeViewFor(tr, me.id, now)}, nil
	}
	fail, err := s.tradeExecute(tr)
	if err != nil { // 数据库错误：已回滚，双方取消（INTERNAL），内存不变
		h.cancelLocked(tr, 0, "", protocol.CodeInternal)
		return nil, protocol.Errorf(protocol.CodeInternal, "交易失败，已取消，请重新发起")
	}
	if fail == nil {
		return map[string]any{"done": true}, nil
	}
	// 交换失败：交易不关闭，回到编辑阶段，失效的报价项移除，记下原因（docs/trade.md 2.3）
	owner := tr.side(fail.who)
	owner.Offer.Items = withoutSlots(owner.Offer.Items, fail.removed)
	tr.failExchange(fail.who, fail.code, fail.removed, now)
	pushTo(peer.P, "trade_updated", map[string]any{"trade": tradeViewFor(tr, peer.CharID, now)})
	return map[string]any{"trade": tradeViewFor(tr, me.id, now)}, nil
}

// withoutSlots 返回去掉 gone 里的格号后的新切片。
func withoutSlots(slots []int, gone []int) []int {
	drop := map[int]bool{}
	for _, s := range gone {
		drop[s] = true
	}
	out := []int{}
	for _, s := range slots {
		if !drop[s] {
			out = append(out, s)
		}
	}
	return out
}
