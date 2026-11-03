package gamedata

// 运行中的 NPC 位置（STAGE-93，docs/quest.md、docs/protocol.md 5.32）：npcs.tsv 是初始位置，任务脚本的
// NpcWarp 把 NPC 移到脚本给的另一个位置（原版 npc_exchangeman.c 的 NPC_EventWarpNpc，CHAR_warpToSpecificPoint）。
// 全服共用一份（同一个服务端里的所有连接看到的是同一个 NPC），只在内存里：服务重启后回到 npcs.tsv 的位置，
// 轮转进度也归零（原版同样不落库）。
//
// mu 是叶子锁：这里不调用别的锁。角色锁里可以查位置，但移动要放在角色锁之外做（server/character/npcwarp.go）。

import (
	"sort"
	"sync"
)

// NPCBoard 记录被 NpcWarp 移走过的 NPC；没动过的 NPC 直接用静态表（NPCByID 等）。
// 同一块板上还记着战斗 NPC 的占用（one_battle，STAGE-96）：只在内存里，重启后全部空闲，和原版一样。
type NPCBoard struct {
	mu    sync.Mutex
	moved map[int]NPC    // 编号 → 当前位置（只有移动过的）
	turn  map[int]int    // 编号 → NpcWarp 轮转的下一个下标（从 1 起，对照原版 CHAR_WORK_EVENTWARP）
	busy  map[int]string // 战斗 NPC 编号 → 正在打它的战斗编号（one_battle）
}

// NewNPCBoard 返回一份初始的 NPC 位置：全部在 npcs.tsv 的位置，没有战斗占用。
func NewNPCBoard() *NPCBoard {
	return &NPCBoard{moved: map[int]NPC{}, turn: map[int]int{}, busy: map[int]string{}}
}

// Occupy 登记 NPC 正被战斗 battleID 占用（one_battle）。检查和登记在同一把锁里完成：NPC 已经被占用时
// 返回 false，两个人同时开战也只有一个能登记上。
func (b *NPCBoard) Occupy(id int, battleID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.busy[id]; ok {
		return false
	}
	b.busy[id] = battleID
	return true
}

// Release 释放 battleID 对 NPC 的占用；占用不是它登记的（或已经释放）就什么都不做。
func (b *NPCBoard) Release(id int, battleID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.busy[id] == battleID {
		delete(b.busy, id)
	}
}

// Busy 报告 NPC 是否正被战斗占用。对话时用它提前拒绝；真正开战以 Occupy 的结果为准。
func (b *NPCBoard) Busy(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.busy[id]
	return ok
}

// ByID 返回 NPC 的当前位置；编号不存在返回 false。
func (b *NPCBoard) ByID(id int) (NPC, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.byIDLocked(id)
}

func (b *NPCBoard) byIDLocked(id int) (NPC, bool) {
	if n, ok := b.moved[id]; ok {
		return n, true
	}
	return NPCByID(id)
}

// OnFloor 返回地图上当前的 NPC，按编号升序（同 NPCsByFloor，但算上移动）：移走的不再列在原地图，
// 移到这里的列进来。地图上没有 NPC 时返回 nil（当空列表用）。
func (b *NPCBoard) OnFloor(floor int) []NPC {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []NPC
	for _, n := range NPCsByFloor(floor) {
		if _, ok := b.moved[n.ID]; !ok {
			out = append(out, n)
		}
	}
	for _, n := range b.moved {
		if n.Floor == floor {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ByName 按名字找 NPC（规则同 NPCsByName），返回的是当前位置。名字不随移动改变，所以匹配的集合不变。
func (b *NPCBoard) ByName(name string) []NPC {
	b.mu.Lock()
	defer b.mu.Unlock()
	found := NPCsByName(name)
	for i, n := range found {
		if cur, ok := b.moved[n.ID]; ok {
			found[i] = cur
		}
	}
	return found
}

// Warp 把 NPC 移到 points 里轮到的下一个位置（原版 NPC_EventWarpNpc）：第一次用第一个，每用一次往后挪，
// 用到最后一个之后回到第一个。返回移动前后的位置；NPC 不存在或列表为空时返回 false。
func (b *NPCBoard) Warp(id int, points []WarpPoint) (from, to NPC, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cur, ok := b.byIDLocked(id)
	if !ok || len(points) == 0 {
		return NPC{}, NPC{}, false
	}
	i := b.turn[id]
	if i < 1 || i > len(points) {
		i = 1
	}
	p := points[i-1]
	b.turn[id] = i%len(points) + 1
	to = cur
	to.Floor, to.X, to.Y = p.Floor, p.X, p.Y
	b.moved[id] = to
	return cur, to, true
}
