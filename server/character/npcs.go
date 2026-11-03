package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// handleNpcs 返回角色当前所在地图上的 NPC 列表（docs/protocol.md 5.32，查看类，STAGE-68）。
// 本期没有走路，NPC 列表只用来查看；编号是全表唯一的，以后的 NPC 交互命令沿用。
func (s *Service) handleNpcs(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var floor int
	if err := l.With(func(r *record) { floor = r.Floor }); err != nil {
		return nil, asProtocolError(err)
	}
	npcs := s.npcs.OnFloor(floor)
	if npcs == nil { // 没有NPC的地图返回空数组，不是 null（docs/protocol.md 2）
		npcs = []gamedata.NPC{}
	}
	return map[string]any{"floor": floor, "npcs": npcs}, nil
}
