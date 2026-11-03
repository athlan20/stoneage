package character

import (
	"context"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-135：返回服务端窗口的上一阶段，不能只在客户端假造旧菜单。
func (s *Service) handleNpcBack(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		WindowID string `json:"window_id"`
	}
	if e := req.Decode(&in); e != nil {
		return nil, e
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var perr *protocol.Error
	err = l.Operate(func(r *record) {
		if perr = l.p.checkWindow(in.WindowID); perr != nil {
			return
		}
		w := l.p.npcWindow
		if w.shop == nil {
			l.p.pushWindowClosed(WindowReasonClosed)
			return
		}
		npc, _ := s.npcs.ByID(w.npc)
		shop := gamedata.NpcShopOf(w.npc)
		stage := w.shop.stage
		if stage == shopStageConfirm {
			if w.shop.quote.direction == "sell" {
				stage = shopStageSellQty
			} else {
				stage = shopStageBuyQty
			}
		}
		w.shop.quote = nil
		switch stage {
		case shopStageBuyQty:
			w.shop.stage = shopStageMenu
			perr = s.shopChoose(ctx, l, r, w, 1)
		case shopStageSellQty:
			perr = s.openSellList(l, r, w, npc, shop)
		case shopStageBuyList, shopStageSellList:
			s.openShop(l.p, npc, shop)
		default:
			l.p.pushWindowClosed(WindowReasonClosed)
		}
	})
	if err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{}, nil
}
