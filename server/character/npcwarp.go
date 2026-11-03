package character

// NPC 移位（STAGE-93，docs/quest.md，对照 npc_exchangeman.c 的 NPC_EventWarpNpc）：任务脚本的一步成功后，
// NPC 自己移到 NpcWarp 里轮到的下一个位置。移动对全服生效（gamedata.NPCBoard），之后：
// 原地图和新地图上的连接收到 npcs_changed（新的 NPC 列表，docs/protocol.md 5.46）；原地图上别的连接
// 正开着这个 NPC 的窗口时关掉（原因 MOVED）。
//
// 移位分两步做：交易成功时只在角色锁里登记（queueNPCWarp，记在 Live.warps），handleTalk / handleNpcChoose /
// handleNpcReply 的 Operate 返回、锁放开之后由 settleNPCWarps 执行。推送要拿别的角色的锁，在角色锁里做会和它们互相等。

import (
	"github.com/athlan20/stoneage/server/gamedata"
)

// npcWarp 是一次待结算的移位：哪个 NPC，以及它的 NpcWarp 位置列表。
type npcWarp struct {
	npc    int
	points []gamedata.WarpPoint
}

// queueNPCWarp 登记这一步的移位（块里有 NpcWarp 时）。调用时持有角色锁，交易已经成功才能调用。
func (l *Live) queueNPCWarp(npc gamedata.NPC, blk *gamedata.EventBlock) {
	if len(blk.Warp) > 0 {
		l.warps = append(l.warps, npcWarp{npc: npc.ID, points: blk.Warp})
	}
}

// settleNPCWarps 执行本次请求登记的移位，并推送给受影响的连接。调用时不能持有任何角色锁。
func (s *Service) settleNPCWarps(l *Live) {
	warps := l.warps
	l.warps = nil
	for _, w := range warps {
		from, to, ok := s.npcs.Warp(w.npc, w.points)
		if !ok {
			continue
		}
		s.pushNPCMoved(l.p, from.Floor, to.Floor, w.npc)
	}
}

// pushNPCMoved 给世界中所在地图是 from 或 to 的角色推送新的 NPC 列表。trigger 是触发移位的角色：
// 它刚开的 NPC 窗口不关（移位是这一步的结果，窗口上的台词还要让它看完）。
func (s *Service) pushNPCMoved(trigger *Presence, from, to, npc int) {
	s.mu.Lock()
	presences := make([]*Presence, 0, len(s.presences))
	for _, p := range s.presences {
		presences = append(presences, p)
	}
	s.mu.Unlock()
	for _, p := range presences {
		p.mu.Lock()
		if f := p.rec.Floor; f == from || f == to {
			if p != trigger && f == from && p.npcWindow != nil && p.npcWindow.npc == npc {
				p.pushWindowClosed(WindowReasonMoved)
			}
			list := s.npcs.OnFloor(f)
			if list == nil { // 没有 NPC 的地图推送空数组，不是 null（docs/protocol.md 2）
				list = []gamedata.NPC{}
			}
			p.pushAll("npcs_changed", map[string]any{"floor": f, "npcs": list})
		}
		p.mu.Unlock()
	}
}
