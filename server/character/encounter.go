// 遇敌开关（STAGE-40，docs/protocol.md 5.29）：`encounter on|off` 打开后角色按所在位置
// 的遇敌区域一场接一场地战斗，直到关掉、打输、传送、离开世界或掉线。
// 敌人按 gmsv/char/enemy.c 的 ENEMY_getEnemy 抽取（server/battle/encounter.go）；
// 这里只管开关状态、连场的节奏和自动关闭。

package character

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// defaultEncounterInterval 是两场遇敌战斗之间的间隔（需求 STAGE-40：默认已改为 0.5 秒，服务端配置可调）。
const defaultEncounterInterval = 500 * time.Millisecond

// encounterPlan 是一次“按区域遇敌”的计划（STAGE-152，docs/protocol/encounter-areas.md）。
// AreaID 为空表示由 `encounter on` 打开的“当前位置无限遇敌”。
type encounterPlan struct {
	AreaID string
	Number int
	Mode   string // "once" | "infinite"
}

// continues 报告之后还会不会再开下一场。
func (p *encounterPlan) continues() bool { return p != nil && p.Mode == "infinite" }

// 自动关掉遇敌开关的原因（推送 encounter_changed 的 reason 与提示文案）。
const (
	encounterOffLose    = "lose"      // 战斗失败
	encounterOffWarp    = "warp"      // 传送走了
	encounterOffNoEnemy = "no_enemy"  // 开下一场时这里已经没有敌人
	encounterOnceDone   = "once_done" // 单次遇敌打完了（STAGE-152）
	encounterOffControl = "control"   // 控制权转交（STAGE-311）
)

var encounterOffText = map[string]string{
	encounterOffLose:    "战斗失败，遇敌开关已自动关闭",
	encounterOffWarp:    "已传送到别处，遇敌开关自动关闭",
	encounterOffNoEnemy: "这里没有敌人，遇敌开关已自动关闭",
	encounterOffControl: "控制权已切换，遇敌开关自动关闭",
}

// handleEncounter 是遇敌开关（docs/protocol.md 5.29，操作类）：on 打开并立刻开第一场，
// off 关掉（当前这场照常打完），不带参数时查询。观察方发起被 Operate 拦截（协议 9.3）。
func (s *Service) handleEncounter(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Set *string `json:"set"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	var arg string
	if in.Set != nil {
		arg = *in.Set
	}
	switch arg {
	case "", "on", "off":
	default:
		return nil, protocol.Errorf(protocol.CodeBadRequest, "set 要是 on 或 off（不带则查询当前状态）")
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
		switch arg {
		case "":
			out = encounterChangedData(live.p, "")
		case "off":
			live.p.stopEncounter("")
			out = encounterChangedData(live.p, "")
		case "on":
			if !live.p.encounterOn {
				if perr = s.checkNoEnemyEncounter(live.p); perr != nil { // 龙王的守护生效中（STAGE-340）
					return
				}
				// 不在战斗中时先试着开第一场：开不出来（比如村里）什么都不推，
				// 开关保持关闭（battle_start 已由 startWildBattle 推出，随后才推开关变化）。
				if live.p.battle == nil && !s.startWildBattle(live.p) {
					perr = protocol.Errorf(protocol.CodeNoEnemyHere, "这里没有敌人，遇敌开关没有打开")
					return
				}
				live.p.encounterOn = true
				live.p.plan = &encounterPlan{Mode: "infinite"} // 当前位置无限遇敌（STAGE-152）
				live.p.pushAll("encounter_changed", encounterChangedData(live.p, ""))
			}
			out = encounterChangedData(live.p, "")
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return out, nil
}

// startWildBattle 按角色当前所在位置的遇敌区域立刻开一场野外战斗（遇敌开关的 on 和
// 连场的下一场共用）。调用方持有 p.mu。开不出来（不在区域、区域抽不出敌人）返回 false。
func (s *Service) startWildBattle(p *Presence) bool {
	if p.battle != nil {
		return true // 已经在战斗里：开关只负责这一场结束后接着开
	}
	if ok, handled := s.startPartyWild(p); handled { // 真人队的队长遇敌：全体在线队员同场（STAGE-275）
		return ok
	}
	area := gamedata.EncounterAreaAt(p.rec.Floor, p.rec.X, p.rec.Y)
	if area == nil {
		return false
	}
	held := map[int]bool{}
	for _, it := range p.rec.Items {
		held[it.ItemID] = true // 敌群的“持有才出现 / 持有不出现”道具条件
	}
	b, ok := battle.BuildWild(s.nextBattleID(), battle.EncounterScene(p.rec.Floor),
		sideInput(&p.rec), &battle.WildSpec{Area: area, HeldItems: held}, s.wildRnd())
	if !ok {
		return false
	}
	s.wireBattle(p, b)
	return true
}

// wildRnd 返回野外战斗的随机源：测试里按用例固定种子，生产环境用全局随机。
func (s *Service) wildRnd() func(n int) int {
	if s.opts.WildRnd != nil {
		return s.opts.WildRnd
	}
	return rand.IntN
}

// scheduleEncounter 在一场战斗结束后安排下一场：间隔从 battle_end 起算（默认 2 秒）。
// 调用方持有 p.mu。
func (s *Service) scheduleEncounter(p *Presence) {
	p.stopEncounterTimer()
	p.encounterTimer = time.AfterFunc(s.opts.encounterInterval(), func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.leaving || p.battle != nil || !p.encounterOn {
			return
		}
		if p.plan != nil && p.plan.Mode == "once" {
			return // 单次遇敌不再开第二场（STAGE-152）
		}
		// 守护生效中（STAGE-340）也按“这里没有敌人”自动关掉开关。
		if p.noEnemyActive(s.now()) || !s.startWildBattle(p) {
			p.stopEncounter(encounterOffNoEnemy)
		}
	})
}

// stopEncounter 关掉遇敌开关并推送：手动 off 不带原因；自动关掉时带上原因并给一条系统消息。
// 关掉时同时清掉按区域遇敌的计划（STAGE-152）和“下一场”的计时器。调用方持有 p.mu。
// STAGE-311 起控制权转交也走这里（reason 为 control）：开关跟着控制方走，新操控方要用就自己重开。
func (p *Presence) stopEncounter(reason string) {
	p.encounterOn = false
	p.plan = nil
	p.stopEncounterTimer()
	p.pushAll("encounter_changed", encounterChangedData(p, reason))
	if text, ok := encounterOffText[reason]; ok {
		p.pushAll("notice", map[string]string{"level": "info", "text": text})
	}
}

// encounterChangedData 是 encounter_changed 推送与 encounter / status 响应的共同状态：
// encounter 布尔字段保留兼容，STAGE-152 起并列给出区域、模式与“还会不会继续”。
// 没有计划时这几项为 null。调用方持有 p.mu。
func encounterChangedData(p *Presence, reason string) map[string]any {
	data := map[string]any{
		"encounter": p.encounterOn,
		// 没有计划时显式给 null，客户端不用猜字段缺省。
		"encounter_area_id":     nil,
		"encounter_area_number": nil,
		"encounter_mode":        nil,
		"encounter_continues":   false,
	}
	if p.plan != nil {
		if p.plan.AreaID != "" {
			data["encounter_area_id"] = p.plan.AreaID
		}
		if p.plan.Number != 0 {
			data["encounter_area_number"] = p.plan.Number
		}
		data["encounter_mode"] = p.plan.Mode
		data["encounter_continues"] = p.plan.continues()
	}
	if reason != "" {
		data["reason"] = reason
	}
	return data
}

// stopEncounterTimer 停掉“下一场”的计时器；开关关闭、角色离开世界时调用。调用方持有 p.mu。
func (p *Presence) stopEncounterTimer() {
	if p.encounterTimer != nil {
		p.encounterTimer.Stop()
		p.encounterTimer = nil
	}
}

// encounterInterval 取两场战斗之间的间隔（Options 的零值取默认 2 秒）。
func (o Options) encounterInterval() time.Duration {
	if o.EncounterInterval <= 0 {
		return defaultEncounterInterval
	}
	return o.EncounterInterval
}

// handleEncounterStart 是按区域开战（STAGE-152，docs/protocol.md 5.51，操作类）：
// 校验后把角色定位到该区域的代表落点，再立刻开第一场。位置推送先于 battle_start；
// 任何一步失败都不移动角色、不改原有计划、不关 NPC 窗口。观察方发起被 Operate 拦下。
func (s *Service) handleEncounterStart(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		SourceFloor *int   `json:"source_floor"` // 指针：缺省要报 BAD_REQUEST，不能当成 0 层去比对（STAGE-174）
		AreaID      string `json:"area_id"`
		Mode        string `json:"mode"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.SourceFloor == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 source_floor，填读取遇敌区域时所在地图的 floor")
	}
	if in.Mode != "once" && in.Mode != "infinite" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "mode 要是 once 或 infinite")
	}
	if in.AreaID == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "area_id 不能为空")
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
		// 检查顺序同协议 5.50：非战斗在当前地图之前。
		if live.p.battle != nil {
			perr = protocol.Errorf(protocol.CodeInBattle, "战斗中不能换遇敌区域，先打完这一场")
			return
		}
		if perr = s.checkNoEnemyEncounter(live.p); perr != nil { // 龙王的守护生效中（STAGE-340）
			return
		}
		if *in.SourceFloor != r.Floor {
			perr = protocol.Errorf(protocol.CodePositionChanged, "所在地图已变化，请重新读取遇敌区域")
			return
		}
		area, ok := gamedata.EncounterAreaByKey(in.AreaID)
		if !ok || area.Floor != r.Floor {
			perr = protocol.Errorf(protocol.CodeEncounterAreaNotFound, "本图没有这个遇敌区域")
			return
		}
		view, ok := buildAreaView(r.Floor, area, heldItemSet(r))
		if !ok {
			perr = protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "这个区域当前不能遇敌")
			return
		}
		if !view.Available {
			reason := "这个区域当前不能遇敌"
			if view.UnavailableReason != nil {
				reason = *view.UnavailableReason
			}
			perr = protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "%s", reason)
			return
		}
		// 先按代表落点把这一场建出来；建不出来就当请求失败，位置一动不动。
		if !s.startWildBattleAt(live.p, view.Anchor.X, view.Anchor.Y) {
			perr = protocol.Errorf(protocol.CodeEncounterAreaUnavailable, "这个区域现在抽不出敌人")
			return
		}
		live.p.encounterOn = true
		live.p.plan = &encounterPlan{AreaID: area.Key, Number: area.ID, Mode: in.Mode}
		live.p.pushAll("encounter_changed", encounterChangedData(live.p, ""))
		out = encounterChangedData(live.p, "")
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return out, nil
}

// startWildBattleAt 在指定坐标开一场野外战斗（按区域遇敌用，STAGE-152）：先把战斗建出来，
// 成功了才改坐标并推 position_changed，所以位置一定排在 battle_start 前面。调用方持有 p.mu。
func (s *Service) startWildBattleAt(p *Presence, x, y int) bool {
	return s.startWildBattleOn(p, p.rec.Floor, x, y)
}

// startWildBattleOn 同 startWildBattleAt，但地图可以指定：1 级宠物目录跨图开战时（STAGE-251）先换到
// floor 再开，成功后角色换到这张图的 (x,y)；失败时什么都不动。
func (s *Service) startWildBattleOn(p *Presence, floor, x, y int) bool {
	if p.battle != nil {
		return false
	}
	area := gamedata.EncounterAreaAt(floor, x, y)
	if area == nil {
		return false
	}
	if ok, handled := s.startPartyWildMoved(p, floor, x, y); handled { // 真人队：队长移动，队员跟过去同场（STAGE-275 D1）
		return ok
	}
	b, ok := battle.BuildWild(s.nextBattleID(), battle.EncounterScene(floor),
		sideInput(&p.rec), &battle.WildSpec{Area: area, HeldItems: heldItemSet(&p.rec)}, s.wildRnd())
	if !ok {
		return false
	}
	p.rec.Floor, p.rec.X, p.rec.Y = floor, x, y
	p.players.moved(p) // STAGE-273：换图
	p.pushPosition(&p.rec)
	p.pushWindowClosed(WindowReasonMoved)
	s.wireBattle(p, b)
	return true
}

// heldItemSet 是背包里持有的物品编号集合（敌群道具条件用）。
func heldItemSet(r *record) map[int]bool {
	held := map[int]bool{}
	for _, it := range r.Items {
		held[it.ItemID] = true
	}
	return held
}
