// STAGE-279：按练级点开战或只传送（docs/protocol.md 5.71）。
// 和 pet_encounter_start 的区别：这里只带区域 ID，落点来自练级目录；校验与开战尾部与 5.63 相同。

package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// handleLevelingStart 是 leveling_start（操作类，协议 5.71）。
func (s *Service) handleLevelingStart(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		AreaID string `json:"area_id"`
		Mode   string `json:"mode"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.AreaID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "area_id 不能为空（练级目录里的区域 ID）")
	}
	if in.Mode != "once" && in.Mode != "infinite" && in.Mode != "travel" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "mode 要是 once、infinite 或 travel（只传送不遇敌）")
	}
	live, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var perr *protocol.Error
	if err := live.Operate(func(r *record) {
		if perr = followerError(live.p); perr != nil { // 队员不能自己遇敌（STAGE-274）
			return
		}
		out, perr = s.levelingStartLocked(live, r, in.AreaID, in.Mode)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return out, nil
}

// levelingStartLocked 在角色操作锁内校验并执行；失败都发生在改动位置之前。
func (s *Service) levelingStartLocked(live *Live, r *record, areaID, mode string) (map[string]any, *protocol.Error) {
	p := live.p
	if p.battle != nil {
		return nil, protocol.Errorf(protocol.CodeInBattle, "战斗中不能换遇敌区域，先打完这一场")
	}
	spot, ok := gamedata.LevelingSpotFor(areaID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeLevelingSpotNotFound, "这个区域不在练级传送目录里，请重新读取练级目录")
	}
	area, ok := gamedata.EncounterAreaByKey(areaID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeLevelingSpotNotFound, "没有这个遇敌区域，请重新读取练级目录")
	}
	view, ok := buildAreaView(spot.Floor, area, heldItemSet(r))
	if !ok || !view.Available {
		reason := "这个区域当前不能遇敌"
		if ok && view.UnavailableReason != nil {
			reason = *view.UnavailableReason
		}
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "%s", reason)
	}
	if view.Anchor != (pointView{X: spot.Anchor.X, Y: spot.Anchor.Y}) {
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "目录和服务端的区域数据对不上，请重新读取练级目录")
	}
	if mode == "travel" {
		s.teleport(live, r, spot.Floor, view.Anchor.X, view.Anchor.Y)
		return encounterChangedData(p, ""), nil
	}
	if !s.startWildBattleOn(p, spot.Floor, view.Anchor.X, view.Anchor.Y) {
		return nil, protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "这个区域现在抽不出敌人")
	}
	p.encounterOn = true
	p.plan = &encounterPlan{AreaID: area.Key, Number: area.ID, Mode: mode}
	p.pushAll("encounter_changed", encounterChangedData(p, ""))
	return encounterChangedData(p, ""), nil
}
