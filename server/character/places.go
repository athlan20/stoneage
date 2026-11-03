package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// handlePlaces 返回地点列表（docs/protocol.md 5.15，查看类）。
func (s *Service) handlePlaces(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	if _, err := s.liveOrErr(sess); err != nil {
		return nil, err
	}
	return map[string]any{"places": gamedata.Places}, nil
}

// handleWarp 把角色直接传送（docs/protocol.md 5.16，操作类）：没有移动过程，朝向不变，免费。
// 目标是地点编号 place，或者任意地图的编号 floor（STAGE-82，落点见 gamedata.LandingOf），二选一。
func (s *Service) handleWarp(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Place *int `json:"place"`
		Floor *int `json:"floor"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if (in.Place == nil) == (in.Floor == nil) {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要整数字段 place（地点编号，见 places）或 floor（地图编号），两个只能给一个")
	}
	floor, x, y, err := warpTarget(in.Place, in.Floor)
	if err != nil {
		return nil, err
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var battleErr *protocol.Error
	if err := l.Operate(func(r *record) {
		if perr := followerError(l.p); perr != nil { // 队员不能自己换图（STAGE-274）
			battleErr = perr
			return
		}
		if perr := inBattle(l); perr != nil { // 战斗中不能传送（docs/protocol.md 10.1）
			battleErr = perr
			return
		}
		s.teleport(l, r, floor, x, y)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if battleErr != nil {
		return nil, battleErr
	}
	return map[string]any{}, nil
}

// warpTarget 把 place 或 floor 换成传送后的 floor、x、y；目标不存在或不在正式目录里时回复
// PLACE_NOT_FOUND，位置不变。
//
// STAGE-161 起两者都要过同一道准入：目标 floor 必须是快捷传送目录里的正式地图。
// 菜单里看不到的测试图、占位图不能经手写 warp 绕进来。
func warpTarget(place, floor *int) (int, int, int, error) {
	if place != nil {
		p, ok := gamedata.PlaceByID(*place)
		if !ok {
			return 0, 0, 0, protocol.Errorf(protocol.CodePlaceNotFound, "没有编号为 %d 的地点", *place)
		}
		if !gamedata.WarpableFloor(p.Floor) {
			return 0, 0, 0, protocol.Errorf(protocol.CodePlaceNotFound, "地点 %d（%s）所在的地图不在快捷传送目录里", *place, p.Name)
		}
		return p.Floor, p.X, p.Y, nil
	}
	if !gamedata.WarpableFloor(*floor) {
		return 0, 0, 0, protocol.Errorf(protocol.CodePlaceNotFound, "地图 %d 不在快捷传送目录里（地点编号见地图上的 X 标记）", *floor)
	}
	x, y, ok := gamedata.LandingOf(*floor)
	if !ok {
		return 0, 0, 0, protocol.Errorf(protocol.CodePlaceNotFound, "地图 %d 没有可用落点", *floor)
	}
	return *floor, x, y, nil
}
