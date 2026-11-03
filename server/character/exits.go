package character

import (
	"context"
	"encoding/json"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

func (s *Service) handleExits(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var result any
	if err = l.With(func(r *record) {
		now := s.opts.Now()
		exits := gamedata.ExitsOf(r.Floor)
		views := make([]gamedata.ExitView, 0, len(exits))
		for _, e := range exits {
			views = append(views, e.View(now))
		}
		result = map[string]any{"floor": r.Floor, "exits": views, "server_time_ms": now.UnixMilli(), "next_change_ms": gamedata.NextExitChange(exits, now)}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	return result, nil
}
func (s *Service) handleUseExit(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(req.Data, &fields) != nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "出口参数无效")
	}
	for key := range fields {
		if key != "source_floor" && key != "exit_id" {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "不接受出口参数 %s", key)
		}
	}
	var in struct {
		SourceFloor *int    `json:"source_floor"`
		ExitID      *string `json:"exit_id"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.SourceFloor == nil || in.ExitID == nil || *in.ExitID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 source_floor 和 exit_id")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var failure *protocol.Error
	err = l.Operate(func(r *record) {
		if failure = followerError(l.p); failure != nil { // 队员不能自己换图（STAGE-274）
			return
		}
		if failure = inBattle(l); failure != nil {
			return
		}
		if r.Floor != *in.SourceFloor {
			failure = protocol.Errorf(protocol.CodePositionChanged, "所在地图已变化，请刷新出口")
			return
		}
		for _, e := range gamedata.ExitsOf(r.Floor) {
			if e.ID != *in.ExitID {
				continue
			}
			if reason := e.UnavailableAt(s.opts.Now()); reason != "" {
				failure = protocol.Errorf(protocol.CodeExitUnavailable, "%s", reason)
				return
			}
			if len(e.Cells) == 0 {
				failure = protocol.Errorf(protocol.CodeExitUnavailable, "出口数据无效")
				return
			}
			s.teleport(l, r, e.ToFloor, e.Cells[0].ToX, e.Cells[0].ToY)
			return
		}
		failure = protocol.Errorf(protocol.CodeExitNotFound, "出口不存在或不属于当前地图")
	})
	if err != nil {
		return nil, asProtocolError(err)
	}
	if failure != nil {
		return nil, failure
	}
	return map[string]any{}, nil
}

// teleport is called only inside a successful, locked character operation.
func (s *Service) teleport(l *Live, r *record, floor, x, y int) {
	r.Floor, r.X, r.Y = floor, x, y
	l.p.players.moved(l.p) // STAGE-273：换图
	l.p.pushPosition(r)
	l.p.pushWindowClosed(WindowReasonMoved)
	if l.p.encounterOn {
		l.p.stopEncounter(encounterOffWarp)
	}
}
