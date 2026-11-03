package character

// STAGE-330：猜谜的座位（原版 npc_quiz.c 的 QuizPlayer 与 NPC_PlayerCheck）。每个猜谜人物最多 8 人同时答题
// （原版 MEPLAYER），一个角色同一时间最多占一个座位（原版按角色索引 QuizPlayer，同一个人不能挑战两位）。
// 座位只在内存里：离开世界、断线、换图、再次 talk、关掉窗口、战斗开始时释放（见 releaseQuiz 的调用处）。

import (
	"sync"

	"github.com/athlan20/stoneage/server/gamedata"
)

// quizMaxPlayers 是同一个猜谜人物能同时有几个人在答（原版 MEPLAYER）。
const quizMaxPlayers = 8

// quizSeat 是一个人占着的座位：谁、在哪位猜谜人物那里（地图和格子用来查人还在不在旁边），答题的状态。
// run 在付完参赛道具、开始答题之后才有；active 由 quizHub.mu 保护，座位被释放或被清掉后为 false。
type quizSeat struct {
	owner  *Presence
	npc    int
	floor  int
	x, y   int
	run    *gamedata.QuizRun
	active bool
}

// quizHub 是全服的猜谜座位表，自己有锁。持有 quizHub.mu 时不阻塞地取别人的 Presence.mu（TryLock），
// 不等待，所以不会和持有 Presence.mu 去取 quizHub.mu 的一方死锁。
type quizHub struct {
	mu    sync.Mutex
	seats map[int][]*quizSeat // NPC 编号 → 正在这位人物那里答题的座位
}

func newQuizHub() *quizHub {
	return &quizHub{seats: map[int][]*quizSeat{}}
}

// stillNear 报告座位的主人是否还在人物旁边（同一张图，切比雪夫距离不超过 quizReach）。
// 对方的锁拿不到时当作还在，不冒险清掉（原版 NPC_RealyCheack 按 3x3 格子查人）。调用时要持有 quizHub.mu。
func (seat *quizSeat) stillNear() bool {
	if !seat.owner.mu.TryLock() {
		return true
	}
	defer seat.owner.mu.Unlock()
	r := &seat.owner.rec
	return r.Floor == seat.floor && max(abs(r.X-seat.x), abs(r.Y-seat.y)) <= quizReach
}

// acquire 为 p 在人物 npc 那里占一个座位。先清掉已经走开的人，再看有没有空位，满了返回 false。
// 调用时 p 持有自己的 Presence.mu（talk 或选“是”的那一步），p 自己的旧座位已经在别处释放过。
func (h *quizHub) acquire(p *Presence, npc gamedata.NPC) (*quizSeat, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	kept := h.seats[npc.ID][:0]
	for _, seat := range h.seats[npc.ID] {
		if seat.owner != p && seat.stillNear() {
			kept = append(kept, seat)
		} else {
			seat.active = false
		}
	}
	if len(kept) >= quizMaxPlayers {
		h.setLocked(npc.ID, kept)
		return nil, false
	}
	seat := &quizSeat{owner: p, npc: npc.ID, floor: npc.Floor, x: npc.X, y: npc.Y, active: true}
	h.setLocked(npc.ID, append(kept, seat))
	return seat, true
}

// setLocked 更新一位人物的座位列表，空了就删掉这一项。调用时要持有 h.mu。
func (h *quizHub) setLocked(npc int, seats []*quizSeat) {
	if len(seats) == 0 {
		delete(h.seats, npc)
		return
	}
	h.seats[npc] = seats
}

// release 放掉座位：从座位表里去掉并标记为无效。重复调用没有影响。
func (h *quizHub) release(seat *quizSeat) {
	h.mu.Lock()
	defer h.mu.Unlock()
	seat.active = false
	list := h.seats[seat.npc]
	for i, s := range list {
		if s == seat {
			h.setLocked(seat.npc, append(list[:i:i], list[i+1:]...))
			return
		}
	}
}

// isActive 报告座位是否还有效（没有被释放，也没有因为主人走开而被清掉）。
func (h *quizHub) isActive(seat *quizSeat) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return seat.active
}

// releaseQuiz 放掉 p 占着的猜谜座位，没有就什么都不做。调用时要持有 p.mu。
func (p *Presence) releaseQuiz() {
	if p.quiz == nil {
		return
	}
	if p.quizzes != nil {
		p.quizzes.release(p.quiz)
	}
	p.quiz = nil
}
