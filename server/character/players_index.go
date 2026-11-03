package character

import (
	"sort"
	"sync"
	"time"
)

// STAGE-273：附近玩家的索引与 players_changed 推送（docs/social-players.md 第一节，docs/protocol.md 5.66）。
//
// 索引只在内存里，按地图（floor）记录世界中的每个角色（一个账号一个 Presence，见 world.go）。
// 它的锁是叶子锁：持有 mu 的时候不再去拿别的锁（不碰 Presence.mu、Service.mu），
// 所以调用方可以在持有 Presence.mu 时调用它的方法。
//
// 变化先记在 pending 里，同一地图 1 秒合并发一次（playersMergeWindow）：同一玩家在窗口内多次变化只留最后状态。
// 发送时才去读各角色的数据（先释放索引的锁，再逐个拿 Presence.mu），所以不会和角色锁死锁。
//
// 所有方法都可以在 idx 为 nil 时调用（没有索引的测试构造的 Presence），什么也不做。

const playersMergeWindow = time.Second

// playerEntry 是索引里的一个在世界中的角色。name 在进入世界时记下（角色名不会变），floor 随换图更新。
type playerEntry struct {
	p      *Presence
	charID int64
	name   string
	floor  int
	// sent 是别人（同图的其他角色）在上一次合并发送之后是否认为他在图上（STAGE-276 隐身用，窗口开始时的状态）；
	// dirty 是隐身期间他的数据变过、别人还没收到（恢复可见时补一条 update）。
	sent, dirty bool
}

// playerChange 是一个地图上待发的一项变化：leave 表示离开（按名字删），否则是整条替换（enter 或 update）。
type playerChange struct {
	charID int64
	name   string
	entry  *playerEntry // 发送时读它的当前数据（leave 时为 nil）
	leave  bool
	enter  bool // 窗口内新进入（或离开后又进入）：op 为 enter，否则为 update
}

type playerIndex struct {
	mu      sync.Mutex
	entries map[int64]*playerEntry         // charID → 条目
	byFloor map[int]map[int64]*playerEntry // floor → charID → 条目
	pending map[int]map[int64]*playerChange
	timer   *time.Timer // 合并窗口的计时器，有待发的变化时才存在
}

func newPlayerIndex() *playerIndex {
	return &playerIndex{
		entries: map[int64]*playerEntry{},
		byFloor: map[int]map[int64]*playerEntry{},
		pending: map[int]map[int64]*playerChange{},
	}
}

// add 把刚进入世界的角色加进索引，并给同图的其他角色推 enter。调用时持有 Service.mu 也可以（不碰角色锁）。
func (x *playerIndex) add(p *Presence, floor int, name string) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	e := &playerEntry{p: p, charID: p.charID, name: name, floor: floor}
	x.entries[p.charID] = e
	x.putFloor(e)
	if !p.hidden.Load() { // 隐身者进图不推 enter（协议 5.69）
		x.putPending(floor, &playerChange{charID: p.charID, name: name, entry: e, enter: true})
	}
}

// remove 把离开世界的角色移出索引，并给原图的其他角色推 leave。调用时持有 p.mu。
func (x *playerIndex) remove(p *Presence) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	e := x.entries[p.charID]
	if e == nil {
		return
	}
	delete(x.entries, p.charID)
	x.delFloor(e)
	if p.hidden.Load() { // 隐身者离图不推 leave：别人的列表里本来就没有他（已经在窗口里排队的 leave 照常发）
		return
	}
	x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, leave: true})
}

// moved 在角色的地图变了之后调用（调用时持有 p.mu，p.rec.Floor 已经是新地图）：旧图推 leave，新图推 enter。
func (x *playerIndex) moved(p *Presence) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	e := x.entries[p.charID]
	if e == nil || e.floor == p.rec.Floor {
		return
	}
	hidden := p.hidden.Load()
	if !hidden { // 隐身时旧图的人本来就没有他
		x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, leave: true})
	}
	x.delFloor(e)
	e.floor = p.rec.Floor
	x.putFloor(e)
	e.sent = false // 新图的人还没有他
	if !hidden {
		x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, entry: e, enter: true})
	}
}

// touch 在角色的列表项可能变了（等级、转生、战斗状态）之后调用：同图的其他角色收到 update。调用时持有 p.mu。
func (x *playerIndex) touch(p *Presence) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	e := x.entries[p.charID]
	if e == nil {
		return
	}
	if p.hidden.Load() { // 隐身时不推 update，记下来，恢复可见时补发
		e.dirty = true
		return
	}
	x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, entry: e})
}

// byName 找在世界中的角色（按名字，STAGE-274 组队用）。名字全服唯一；找不到返回 nil。
func (x *playerIndex) byName(name string) *Presence {
	if x == nil {
		return nil
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, e := range x.entries {
		if e.name == name {
			return e.p
		}
	}
	return nil
}

// onFloor 返回地图上除 self 以外的角色条目（副本，调用方可以放开索引的锁再读各角色的数据）。
func (x *playerIndex) onFloor(floor int, self int64) []*playerEntry {
	if x == nil {
		return nil
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	out := make([]*playerEntry, 0, len(x.byFloor[floor]))
	for id, e := range x.byFloor[floor] {
		if id != self {
			out = append(out, e)
		}
	}
	return out
}

func (x *playerIndex) putFloor(e *playerEntry) {
	m := x.byFloor[e.floor]
	if m == nil {
		m = map[int64]*playerEntry{}
		x.byFloor[e.floor] = m
	}
	m[e.charID] = e
}

func (x *playerIndex) delFloor(e *playerEntry) {
	if m := x.byFloor[e.floor]; m != nil {
		delete(m, e.charID)
		if len(m) == 0 {
			delete(x.byFloor, e.floor)
		}
	}
}

// putPending 记下一项变化并确保合并计时器在跑。同一玩家的合并规则：
// leave 覆盖之前的一切；离开之后又整条替换视为 enter；enter 之后的 update 仍是 enter（客户端还没有这一项）。调用时持有 x.mu。
func (x *playerIndex) putPending(floor int, c *playerChange) {
	m := x.pending[floor]
	if m == nil {
		m = map[int64]*playerChange{}
		x.pending[floor] = m
	}
	if cur := m[c.charID]; cur != nil && !c.leave {
		switch {
		case cur.leave:
			c.enter = true
		case cur.enter:
			c.enter = true
		}
	}
	m[c.charID] = c
	if x.timer == nil {
		x.timer = time.AfterFunc(playersMergeWindow, x.flush)
	}
}

// flush 合并窗口到期：取出所有待发的变化，逐张地图发给当时在图上的角色（不含变化的角色自己）。
func (x *playerIndex) flush() {
	x.mu.Lock()
	pending := x.pending
	x.pending = map[int]map[int64]*playerChange{}
	x.timer = nil
	type job struct {
		floor   int
		changes map[int64]*playerChange
		to      []*playerEntry
	}
	jobs := make([]job, 0, len(pending))
	for floor, changes := range pending {
		x.settleChanges(floor, changes)
		to := make([]*playerEntry, 0, len(x.byFloor[floor]))
		for _, e := range x.byFloor[floor] {
			to = append(to, e)
		}
		jobs = append(jobs, job{floor: floor, changes: changes, to: to})
	}
	x.mu.Unlock()

	for _, j := range jobs {
		x.deliver(j.floor, j.changes, j.to)
	}
}

// deliver 把一张地图上的变化推给 to 里的每个角色（变化的是它自己的那一项不发给它）。调用时不持有任何锁。
func (x *playerIndex) deliver(floor int, changes map[int64]*playerChange, to []*playerEntry) {
	if len(to) == 0 || len(changes) == 0 {
		return
	}
	ids := make([]int64, 0, len(changes))
	for id := range changes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	items := make([]playerPushItem, 0, len(ids))
	for _, id := range ids {
		c := changes[id]
		if c.leave {
			items = append(items, playerPushItem{charID: id, data: map[string]any{"op": "leave", "name": c.name}})
			continue
		}
		c.entry.p.mu.Lock()
		view := playerViewOf(c.entry.p)
		c.entry.p.mu.Unlock()
		op := "update"
		if c.enter {
			op = "enter"
		}
		items = append(items, playerPushItem{charID: id, data: map[string]any{"op": op, "player": view}})
	}
	for _, r := range to {
		var mine []map[string]any
		for _, it := range items {
			if it.charID != r.charID {
				mine = append(mine, it.data)
			}
		}
		if len(mine) == 0 {
			continue
		}
		r.p.mu.Lock()
		r.p.pushAll("players_changed", map[string]any{"floor": floor, "changes": mine})
		r.p.mu.Unlock()
	}
}

// playerPushItem 是一项待推的变化和它对应的角色，推送时用 charID 跳过角色自己。
type playerPushItem struct {
	charID int64
	data   map[string]any
}
