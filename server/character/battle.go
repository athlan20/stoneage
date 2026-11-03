// 战斗的接线（STAGE-43，docs/protocol.md 第 10 章）：开战入口（协议用例的 admin 行
// start_battle）、battle_command / battle_ack / battle 三个请求，以及战斗中
// turn / warp / leave_world 的 IN_BATTLE 拦截（addpoint、party_clone、party_disband 从 STAGE-189 起战斗中也放行）。
// 回合的结算推进与结束写回在 battle_apply.go。

package character

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// 战斗的节奏（docs/protocol.md 第 7 节）。
const (
	battleTurnTimeout = 30 * time.Second // 每回合出招时限
	battleAckTimeout  = 20 * time.Second // 回放确认等待上限
)

var (
	errNotInWorld      = errors.New("角色不在游戏中，先进入世界再开战")
	errAlreadyInBattle = errors.New("角色已经在战斗中")
)

// battleState 是挂在世界角色上的战斗。所有字段受 p.mu 保护；计时器的回调重新拿锁，
// 用回合号确认战斗还停在那个阶段，防止推进已经翻页的回合。
type battleState struct {
	b        *battle.Battle
	deadline time.Time              // 本回合出招的截止时间
	ackWait  bool                   // 已推 battle_round，在等所有连接确认
	ackSeen  map[protocol.Role]bool // 本回合已确认的连接（STAGE-54：每条连接只计一次，重复确认不推进）
	turn     int                    // ackWait 对应的回合（等确认时引擎已在下一回合）
	timers   []*time.Timer
	npc      *npcBattle   // NPC 发起的战斗（STAGE-96）；野外战斗为 nil
	pb       *partyBattle // 真人同场战斗的共享状态（STAGE-275，party_battle.go）；单人和分身队为 nil
}

func (bs *battleState) stopTimers() {
	for _, t := range bs.timers {
		t.Stop()
	}
	bs.timers = nil
}

// StartBattle 让正在世界中的角色立刻开一场战斗（协议用例的 admin 行 start_battle）。
// 剧本名见 server/battle/scenario.go；玩家自己遇敌走 encounter 开关（STAGE-40）。
func (s *Service) StartBattle(character, scenario string) error {
	s.mu.Lock()
	var p *Presence
	for _, cand := range s.presences {
		if cand.rec.Name == character {
			p = cand
			break
		}
	}
	s.mu.Unlock()
	if p == nil {
		return errNotInWorld
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.leaving {
		return errNotInWorld
	}
	if p.battle != nil {
		return errAlreadyInBattle
	}
	if sc, err := battle.Lookup(scenario); err == nil && sc.NPCFile != "" {
		return s.startScenarioNPCBattle(p, sc) // 剧本是某个战斗 NPC 的一场（npc_win，STAGE-96）
	}
	b, err := battle.Build(s.nextBattleID(), sideInput(&p.rec), scenario)
	if err != nil {
		return err
	}
	s.wireBattle(p, b)
	return nil
}

// wireBattle 把开好的战斗挂到角色上并推第一条 battle_turn。调用方持有 p.mu。
func (s *Service) wireBattle(p *Presence, b *battle.Battle) {
	b.Cbs.PlaceCaptured = func(m *battle.Unit, keep *battle.CaptureKeep) (int, *battle.Discarded) {
		return captureOutcome(&p.rec, m, keep)
	}
	b.Cbs.ExpRate = s.ExpRate              // 结算那一刻读内存里的全服经验倍数（STAGE-59），改倍数立即生效
	p.pushWindowClosed(WindowReasonBattle) // 对话窗口给战场让路（STAGE-69，协议 5.34）
	if p.party != nil {                    // 分身队一起上场，自动出招（STAGE-162，battle/auto.go）
		b.AddClones(p.party.battleInputs())
		b.SetAutoPlan(p.party.plan) // 组队时带来的方案（STAGE-195，party_plan.go）
	}
	p.battle = &battleState{b: b}
	// STAGE-282：新战斗开始，上一场的迟到确认不再算数
	p.lastBattleTurn = 0
	p.players.touch(p) // STAGE-273：战斗开始，附近玩家列表里 in_battle 变了
	p.pushAll("battle_start", b.StartView())
	s.pushTurn(p)
}

// inBattle 是操作类请求在战斗中的拦截（docs/protocol.md 10.1）。
// 调用时要持有 p.mu，且这条连接还活着。
func inBattle(l *Live) *protocol.Error {
	if l.p.battle != nil {
		return protocol.Errorf(protocol.CodeInBattle, "战斗中不能这样做")
	}
	return nil
}

// handleBattleCommand 是出招（操作类，docs/protocol.md 10.5）；观察方由 Operate 拦截。
func (s *Service) handleBattleCommand(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in battleCommandRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	live, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	if in.Turn == nil || in.Actor == nil || in.Action == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 turn、actor、action 字段")
	}
	cmd := battle.Cmd{Target: -1}
	if in.Target != nil {
		cmd.Target = *in.Target
	}
	if in.Magic != nil {
		cmd.Magic = *in.Magic
	}
	if in.Slot != nil {
		cmd.Slot = *in.Slot
	}
	if in.Skill != nil {
		cmd.Skill = *in.Skill
	}
	if in.PetSlot != nil {
		cmd.PetSlot = *in.PetSlot
	}
	keep, kerr := battle.ParseCaptureKeep(in.Keep)
	if kerr != nil {
		return nil, kerr
	}
	cmd.Keep = keep
	if pb := partyOfLive(live); pb != nil { // 真人同场：按这条连接的角色的号位交指令（STAGE-275）
		return s.partyCommand(live, pb, *in.Turn, *in.Actor, *in.Action, cmd)
	}
	var next string
	var perr *protocol.Error
	if err := live.Operate(func(r *record) {
		bs := live.p.battle
		if bs == nil {
			perr = protocol.Errorf(protocol.CodeNotInBattle, "当前不在战斗中")
			return
		}
		next, perr = bs.b.Submit(*in.Turn, *in.Actor, *in.Action, cmd)
		if perr != nil {
			return
		}
		if bs.b.AllSubmitted() {
			s.settle(live.p) // settle 里先推 battle_progress 再推 battle_round（协议 10.6）
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"next": next}, nil
}

type battleCommandRequest struct {
	Turn    *int            `json:"turn"`
	Actor   *string         `json:"actor"`
	Action  *string         `json:"action"`
	Target  *int            `json:"target"`
	Magic   *int            `json:"magic"`
	Slot    *int            `json:"slot"`
	Skill   *int            `json:"skill"`
	PetSlot *int            `json:"pet_slot"`
	Keep    json.RawMessage `json:"keep"` // capture 的资质筛选（STAGE-264，10.5）
}

// handleBattleAck 是回放确认（docs/protocol.md 10.8）。观察方也可以发，所以用查看类的锁。
func (s *Service) handleBattleAck(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Turn *int `json:"turn"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Turn == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 turn 字段")
	}
	live, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	if pb := partyOfLive(live); pb != nil { // 真人同场：每条连接都确认后才进入下一回合（STAGE-275）
		if err := s.partyAck(live, pb, *in.Turn); err != nil {
			return nil, asProtocolError(err)
		}
		return map[string]any{}, nil
	}
	var ackErr *protocol.Error
	if err := live.With(func(r *record) {
		bs := live.p.battle
		if bs == nil {
			// STAGE-282：战斗刚结束，客户端回放时发出的最后一回合确认才到：按过期确认成功回复（协议 10.8），
			// 从没进过战斗或回合对不上的才是真错误。
			if t := live.p.lastBattleTurn; t == 0 || *in.Turn != t {
				ackErr = protocol.Errorf(protocol.CodeNotInBattle, "当前不在战斗中")
			}
			return
		}
		if !bs.ackWait || *in.Turn != bs.turn {
			return // 重复或过期的确认按成功回复（协议 10.8）
		}
		bs.ackSeen[live.role] = true // 按连接去重：同一连接再确认不重复计数（STAGE-54）
		if len(bs.ackSeen) >= len(live.p.members) {
			s.advanceAfterAck(live.p)
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if ackErr != nil {
		return nil, ackErr
	}
	return map[string]any{}, nil
}

// handleBattle 是查看当前战斗（docs/protocol.md 10.11）。
func (s *Service) handleBattle(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	live, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	if pb := partyOfLive(live); pb != nil {
		return s.partyQuery(live, pb)
	}
	var out any = map[string]any{"in_battle": false}
	if err := live.With(func(r *record) {
		if bs := live.p.battle; bs != nil {
			out = bs.b.QueryView(deadlineMS(bs), s.nowMS())
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	return out, nil
}
