package character

// NPC 对话的通用前置（STAGE-263，缺陷单见 docs/protocol.md 5.33）：原版客户端能自己走到 NPC 旁边再对话，
// 我们的客户端没有走路，所以 talk 遇到“同一张图、但离 NPC 超出它的对话距离”时，服务端先把角色移到 NPC 相邻的
// 空格（等价于原版走过去），推送 position_changed，再由各 NPC 的处理继续。找不到空格就不动角色，
// 由各 NPC 自己的距离检查回复“离 X 太远了，走近一点再说”。不同图的 NPC 不在这里处理（仍先确认传送）。

import (
	"sort"

	"github.com/athlan20/stoneage/server/gamedata"
)

// talkReach 返回这个 NPC 对话要求的距离（格，切比雪夫）；0 表示对话本身不查距离、不需要走过去。
// 新增带距离检查的 NPC 时在这里登记（原版 npc 文件里的 NPC_Util_CharDistance 上限）。
func talkReach(npc gamedata.NPC) int {
	if gamedata.NpcPetskillShopOf(npc.ID) != nil && npc.Kind == "npcgen_petskillshop" {
		return petskillShopReach
	}
	if quizConfigOf(npc) != nil { // 猜谜（STAGE-330）：离人物一格以内才能答
		return quizReach
	}
	return 0
}

// approachNPC 在同一张图上超出 reach 时，把角色移到 NPC 相邻的空格并推送 position_changed，返回是否移动。
// 调用时要持有 p.mu（在 talk 的 Operate 里）；推送先于之后的 npc_window。
func (s *Service) approachNPC(p *Presence, r *record, npc gamedata.NPC, reach int) bool {
	if reach <= 0 || r.Floor != npc.Floor || max(abs(r.X-npc.X), abs(r.Y-npc.Y)) <= reach {
		return false
	}
	cell, ok := s.freeCellNextTo(p, r, npc)
	if !ok {
		return false
	}
	r.X, r.Y = cell[0], cell[1]
	p.pushPosition(r)
	return true
}

// freeCellNextTo 在 NPC 周围一圈（8 格）里找角色能站的格子，离角色近的优先（切比雪夫距离，平局按固定顺序）。
// 不能站的格子：NPC 所在格（含 NPC 当前位置表里的全部 NPC）、普通出口的触发格（站上去就被传走）、
// 地图边界外、同图其他在线角色占着的格。调用时要持有 p.mu。
func (s *Service) freeCellNextTo(p *Presence, r *record, npc gamedata.NPC) ([2]int, bool) {
	taken := map[[2]int]bool{{npc.X, npc.Y}: true}
	for _, other := range s.npcs.OnFloor(npc.Floor) {
		taken[[2]int{other.X, other.Y}] = true
	}
	others, ok := s.otherCellsOn(p, npc.Floor)
	if !ok {
		return [2]int{}, false // 别人的锁拿不到，没法确认格子是否空着，宁可不动
	}
	for _, c := range others {
		taken[c] = true
	}
	exits := exitTriggerCells(npc.Floor)
	w, h, sized := gamedata.MapSize(npc.Floor)
	for _, c := range ringAround(npc.X, npc.Y, r.X, r.Y) {
		if sized && (c[0] < 0 || c[1] < 0 || c[0] >= w || c[1] >= h) {
			continue
		}
		if taken[c] || exits[c] {
			continue
		}
		return c, true
	}
	return [2]int{}, false
}

// otherCellsOn 返回同图其他在线角色（不含 self）的格子。别人的 Presence.mu 只用 TryLock：持有自己的锁时
// 不能阻塞去等别人的锁（两人互等会死锁）。拿不到锁时返回 false，调用方当作没找到空格。
func (s *Service) otherCellsOn(self *Presence, floor int) ([][2]int, bool) {
	s.mu.Lock()
	others := make([]*Presence, 0, len(s.presences))
	for _, p := range s.presences {
		if p != self {
			others = append(others, p)
		}
	}
	s.mu.Unlock()
	cells := make([][2]int, 0, len(others))
	for _, p := range others {
		if !p.mu.TryLock() {
			return nil, false
		}
		if p.rec.Floor == floor {
			cells = append(cells, [2]int{p.rec.X, p.rec.Y})
		}
		p.mu.Unlock()
	}
	return cells, true
}

// ringAround 返回以 (cx, cy) 为中心的 8 个相邻格，按离 (fromX, fromY) 的切比雪夫距离从近到远排；
// 平局保持固定顺序（先 y 后 x），保证同样的输入总是同样的结果。
func ringAround(cx, cy, fromX, fromY int) [][2]int {
	ring := make([][2]int, 0, 8)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx != 0 || dy != 0 {
				ring = append(ring, [2]int{cx + dx, cy + dy})
			}
		}
	}
	dist := func(c [2]int) int { return max(abs(c[0]-fromX), abs(c[1]-fromY)) }
	sort.SliceStable(ring, func(i, j int) bool { return dist(ring[i]) < dist(ring[j]) })
	return ring
}
