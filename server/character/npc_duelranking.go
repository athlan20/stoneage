package character

// STAGE-308：强者表 NPC（原版 npc_duelranking.c，docs/protocol.md 5.75）。talk 弹「强者们 / 自己」菜单；
// 选强者们看第 1 页，选自己看自己所在的那一页；排行窗口带 ranking 字段，选项翻页。排行数据和 duel_ranking 请求共用。

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// duelRankNPCKind 是强者表 NPC 的类型（gen-npcs.py 的 kind）。
const duelRankNPCKind = "duelranking"

const (
	duelRankMenuText = "要看哪一个？"
	duelRankTitle    = "世界的强者们"
)

// duelRankSession 是强者表窗口的状态（挂在 npcWindow.duelRank 上）：page 为 0 是主菜单，否则是排行的当前页。
type duelRankSession struct {
	page int
}

// duelRankOptions 是排行页的选项：有下一页时为 1 下一页；不在第 1 页时加上一页，编号接在后面。
func duelRankOptions(page, pages int) []npcOption {
	opts := []npcOption{}
	if page < pages {
		opts = append(opts, npcOption{No: len(opts) + 1, Label: "下一页"})
	}
	if page > 1 {
		opts = append(opts, npcOption{No: len(opts) + 1, Label: "上一页"})
	}
	return opts
}

// openDuelRankMenu 打开强者表的主菜单。调用时要持有 p.mu。
func openDuelRankMenu(p *Presence, npc gamedata.NPC) {
	p.npcWindow = &npcWindow{npc: npc.ID, duelRank: &duelRankSession{}}
	p.pushAll("npc_window", npcWindowView(npc, duelRankMenuText, npcOption{No: 1, Label: "强者们"}, npcOption{No: 2, Label: "自己"}))
}

// openDuelRankPage 打开排行窗口（一页）。调用时要持有 p.mu。
func openDuelRankPage(p *Presence, npc gamedata.NPC, rk *duelRanking) {
	p.npcWindow = &npcWindow{npc: npc.ID, duelRank: &duelRankSession{page: rk.Page}}
	v := npcWindowView(npc, duelRankTitle, duelRankOptions(rk.Page, rk.Pages)...)
	v["ranking"] = rk
	p.pushAll("npc_window", v)
}

// duelRankChoose 处理强者表窗口的选项（npc_choose）。菜单：1 强者们（第 1 页）、2 自己（自己所在的页）；
// 排行页：按当前数据重算选项后翻页。选项不对回 NPC_BAD_CHOICE，窗口保持不关。调用时要持有 p.mu。
func (s *Service) duelRankChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, w *npcWindow, no int) *protocol.Error {
	p := l.p
	page := 0
	if w.duelRank.page == 0 {
		switch no {
		case 1:
			page = 1
		case 2:
			me, err := s.duelRankLoad(ctx, r.ID, 1)
			if err != nil {
				return protocol.Errorf(protocol.CodeInternal, "读取强者表失败：%v", err)
			}
			page = duelRankPageOf(me.Me.Rank)
		default:
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
		}
	} else {
		cur, err := s.duelRankLoad(ctx, r.ID, w.duelRank.page)
		if err != nil {
			return protocol.Errorf(protocol.CodeInternal, "读取强者表失败：%v", err)
		}
		for _, o := range duelRankOptions(cur.Page, cur.Pages) {
			if o.No != no {
				continue
			}
			if o.Label == "下一页" {
				page = cur.Page + 1
			} else {
				page = cur.Page - 1
			}
		}
		if page == 0 {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
		}
	}
	rk, err := s.duelRankLoad(ctx, r.ID, page)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, "读取强者表失败：%v", err)
	}
	p.pushWindowClosed(WindowReasonChosen)
	openDuelRankPage(p, npc, rk)
	return nil
}
