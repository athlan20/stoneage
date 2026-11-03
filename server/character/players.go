package character

import (
	"context"
	"sort"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-273：附近玩家（docs/social-players.md 第一节，docs/protocol.md 5.66）。
// “附近”就是同一张地图；只给 name、level、transmigration、title、in_battle、party 六项，别的一律不给。

// playersLimit 是 players 一次最多返回的人数，超出时 truncated 为 true。
const playersLimit = 50

// playerItem 是列表里一个玩家的数据，也是 players_changed 里 enter / update 的 player。
type playerItem struct {
	Name           string `json:"name"`
	Level          int    `json:"level"`
	Transmigration int    `json:"transmigration"`
	Title          string `json:"title"`     // 本期没有称号，固定 ""（同 status 的 title）
	InBattle       bool   `json:"in_battle"` // 是否战斗中
	Duel           bool   `json:"duel"`      // 是否接受决斗（社交开关 duel，STAGE-306，协议 5.66）
	Party          any    `json:"party"`     // 真人队尚未开放，固定 null；分身队不对外显示
}

// playerViewOf 取角色的列表项。调用时持有 p.mu。
func playerViewOf(p *Presence) playerItem {
	return playerItem{
		Name:           p.rec.Name,
		Level:          p.rec.Level,
		Transmigration: p.rec.Transmigration,
		Title:          "",
		InBattle:       p.battle != nil,
		Duel:           p.rec.SocialDuel,
		Party:          partySummary(p.parties.snapshot(p)),
	}
}

// handlePlayers 返回同一张地图上除自己以外的角色（docs/protocol.md 5.66，查看类，观察方也能发）。
// 排序：等级从高到低，等级相同按名字（UTF-8 字节序）。队伍排序留给真人队。
func (s *Service) handlePlayers(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var floor int
	var self int64
	if err := l.With(func(r *record) { floor, self = r.Floor, r.ID }); err != nil {
		return nil, asProtocolError(err)
	}
	entries := s.players.onFloor(floor, self)
	items := make([]playerItem, 0, len(entries))
	for _, e := range entries {
		if e.p.hidden.Load() { // 隐身者不出现在别人的列表里（STAGE-276，协议 5.69）
			continue
		}
		e.p.mu.Lock()
		items = append(items, playerViewOf(e.p))
		e.p.mu.Unlock()
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Level != items[j].Level {
			return items[i].Level > items[j].Level
		}
		return items[i].Name < items[j].Name
	})
	truncated := len(items) > playersLimit
	if truncated {
		items = items[:playersLimit]
	}
	if items == nil { // 没有其他玩家时返回空数组，不是 null（docs/protocol.md 2）
		items = []playerItem{}
	}
	return map[string]any{"floor": floor, "truncated": truncated, "players": items}, nil
}
