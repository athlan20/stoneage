package character

// STAGE-294：交易的内存状态机（docs/trade.md 2.3，docs/protocol.md 5.72）。
// 这里只管报价形状、锁定、确定和 rev 的规则，不碰连接、数据库和背包：
// 报价内容的物品、石币、宠物校验与预演在 trade_exec.go，交换也在那里做。
// 调用方持有交易所在的注册表锁；本文件的函数不加锁。

import (
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

const (
	tradeIdleTimeout = 5 * time.Minute // 交易窗口内双方都没有交易操作，自动取消（协议 5.72）
	tradeMaxItems    = 2               // 每方最多 2 件物品（整格）
	tradeMinSlot     = 5               // 背包可交易格 5–19；0–4 是装备格，不能放进报价
	tradeMaxSlot     = 19
	tradeMaxPetSlot  = MaxPetSlots - 1 // 宠物栏 0–19（STAGE-346）
)

// tradeOffer 是一方的报价。Pet 为 -1 表示不出宠物（协议里是 null）。
type tradeOffer struct {
	Items []int // 背包格号，按报价顺序
	Gold  int64
	Pet   int
}

func emptyOffer() tradeOffer { return tradeOffer{Pet: -1} }

// tradeSide 是交易的一方。
type tradeSide struct {
	CharID    int64
	Name      string
	P         *Presence // 角色的连接状态（推送、读取背包用），不参与状态机
	Offer     tradeOffer
	Locked    bool
	Confirmed bool
}

// tradeFailure 是交换失败的原因（协议 5.72 的 last_error）。Who 是“谁的问题”的角色编号。
type tradeFailure struct {
	Code  string
	Who   int64
	Items []int // 被自动移除的报价格号（只用于提示）
}

// trade 是一笔进行中的交易。Rev 从 1 开始，每次报价或锁定状态被清零时 +1。
type trade struct {
	ID      string
	Rev     int
	Sides   [2]*tradeSide
	LastOp  time.Time
	LastErr *tradeFailure
}

// newTrade 建立交易：双方都是空报价、未锁定。
func newTrade(id string, a, b *tradeSide, now time.Time) *trade {
	a.Offer = emptyOffer()
	b.Offer = emptyOffer()
	return &trade{ID: id, Rev: 1, Sides: [2]*tradeSide{a, b}, LastOp: now}
}

// side 返回角色自己的一方（不是这笔交易的参与者时为 nil）。
func (t *trade) side(charID int64) *tradeSide {
	for _, s := range t.Sides {
		if s.CharID == charID {
			return s
		}
	}
	return nil
}

// peer 返回对方的一方。
func (t *trade) peer(charID int64) *tradeSide {
	for _, s := range t.Sides {
		if s.CharID != charID {
			return s
		}
	}
	return nil
}

// bump 是任何修改：rev +1，双方的锁定和确定全部清零，上次的失败原因清掉（协议 5.72 的规则）。
func (t *trade) bump(now time.Time) {
	t.Rev++
	for _, s := range t.Sides {
		s.Locked = false
		s.Confirmed = false
	}
	t.LastOp = now
	t.LastErr = nil
}

// idleExpiresIn 是距离自动取消还剩的秒数（协议 5.72 的 idle_expires_in），不小于 0。
func (t *trade) idleExpiresIn(now time.Time) int {
	left := t.LastOp.Add(tradeIdleTimeout).Sub(now)
	if left < 0 {
		return 0
	}
	return int(left / time.Second)
}

// expired 报告交易是否已经空闲到期，应当由定时扫描取消。
func (t *trade) expired(now time.Time) bool {
	return !now.Before(t.LastOp.Add(tradeIdleTimeout))
}

// checkOfferShape 检查报价的形状，不查背包：物品最多 2 个、格号 5–19 且不重复，石币不为负，宠物为 -1 或 0–MaxPetSlots-1。
// 同一个格子出现两次回复 BAD_REQUEST（防复制，docs/trade.md 2.6）。
func checkOfferShape(o tradeOffer) *protocol.Error {
	if len(o.Items) > tradeMaxItems {
		return protocol.Errorf(protocol.CodeBadRequest, "每方最多放 2 件物品")
	}
	seen := make(map[int]bool, len(o.Items))
	for _, slot := range o.Items {
		if slot < tradeMinSlot || slot > tradeMaxSlot {
			return protocol.Errorf(protocol.CodeBadRequest, "物品格号要在 5–19 之间，装备格不能交易")
		}
		if seen[slot] {
			return protocol.Errorf(protocol.CodeBadRequest, "同一个格子不能放两次")
		}
		seen[slot] = true
	}
	if o.Gold < 0 {
		return protocol.Errorf(protocol.CodeBadRequest, "石币不能为负数")
	}
	if o.Pet < -1 || o.Pet > tradeMaxPetSlot {
		return protocol.Errorf(protocol.CodeBadRequest, "宠物栏位要在 0–%d 之间，不出宠物用 null", tradeMaxPetSlot)
	}
	return nil
}

// checkRev 比较客户端看到的 rev 与当前 rev（协议 5.72：锁定和确定都必须带 rev）。
func (t *trade) checkRev(rev int) *protocol.Error {
	if rev != t.Rev {
		return protocol.Errorf(protocol.CodeTradeStale, "对方刚改了报价，请重新看")
	}
	return nil
}

// setOffer 替换自己的报价（调用方已经把请求里的字段合并成完整报价）：通过形状检查后 rev +1，双方清零。
func (t *trade) setOffer(charID int64, o tradeOffer, now time.Time) *protocol.Error {
	if t.side(charID) == nil {
		return protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	if perr := checkOfferShape(o); perr != nil {
		return perr
	}
	t.side(charID).Offer = o
	t.bump(now)
	return nil
}

// lock 锁定自己的报价。rev 必须与当前一致；preflight 是锁定前的预演（同一套校验，docs/trade.md 2.3），不过则拒绝。
// 锁定本身不改报价，所以不 bump rev。已经锁定时重复锁定是空操作。
func (t *trade) lock(charID int64, rev int, now time.Time, preflight func() *protocol.Error) *protocol.Error {
	me := t.side(charID)
	if me == nil {
		return protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	if perr := t.checkRev(rev); perr != nil {
		return perr
	}
	if me.Locked {
		return nil
	}
	if preflight != nil {
		if perr := preflight(); perr != nil {
			return perr
		}
	}
	me.Locked = true
	t.LastOp = now
	return nil
}

// unlock 取消自己的锁定：整体回到编辑阶段（清双方的锁定和确定，rev +1）。自己没锁定时是空操作。
func (t *trade) unlock(charID int64, now time.Time) *protocol.Error {
	me := t.side(charID)
	if me == nil {
		return protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	if !me.Locked {
		return nil
	}
	t.bump(now)
	return nil
}

// confirm 确定自己的报价。只在双方都已锁定后可用（否则 WRONG_STATE），rev 必须一致。
// 返回 true 表示双方都已确定，调用方应当执行交换。
func (t *trade) confirm(charID int64, rev int, now time.Time) (bool, *protocol.Error) {
	me := t.side(charID)
	if me == nil {
		return false, protocol.Errorf(protocol.CodeTradeNotFound, "没有这笔交易")
	}
	if perr := t.checkRev(rev); perr != nil {
		return false, perr
	}
	if !me.Locked || !t.peer(charID).Locked {
		return false, protocol.Errorf(protocol.CodeWrongState, "双方都锁定后才能确定")
	}
	me.Confirmed = true
	t.LastOp = now
	return t.Sides[0].Confirmed && t.Sides[1].Confirmed, nil
}

// failExchange 交换失败：交易不关闭，回到编辑阶段（rev +1，双方清零），记下原因。
// 失效的报价项由调用方先从报价里移除，再调用本函数。
func (t *trade) failExchange(who int64, code string, removed []int, now time.Time) {
	t.bump(now)
	t.LastErr = &tradeFailure{Code: code, Who: who, Items: removed}
}
