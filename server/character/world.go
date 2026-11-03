package character

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// Presence 是一个账号在世界中的角色。玩家连接和 agent 连接可以同时在线并操控**同一个**角色（docs/protocol.md 第 9 节），
// 所以角色的数据（rec）和“谁在操控”（controller）放在这里，两条连接共用；每条连接各自持有一个 Live。
//
// 数据库里的行只在第一条连接进入时读一次，之后的变化都在内存里，最后一条连接离开、被踢或断开时保存。
//
// 并发规则：保存和控制权变化可能发生在别的 goroutine 里（重复登录挤下线、账号被禁用、令牌被吊销、连接断开、关服），
// 而连接自己可能正在处理请求。所以所有读写角色数据的地方都要通过 Live.With / Live.Operate，它们会加锁并检查这条连接是否已经离开：
// 保存和请求处理因此排队执行，离开之后的请求一律丢弃（STAGE-4 需求的技术细节）。
type Presence struct {
	acct   int64
	charID int64

	mu                sync.Mutex // 保护下面所有字段和各个 Live 的 left
	rec               record
	members           map[protocol.Role]*Live
	controller        protocol.Role
	leaving           bool         // 最后一个成员已经离开：保存完成后才会从注册表移除，期间新的进入要等
	battle            *battleState // 正在进行的战斗（nil 表示不在战斗中，STAGE-43）
	lastBattleTurn    int          // STAGE-282：刚结束的那场战斗的最后一个回合（0 表示没有），迟到的 battle_ack 凭它按成功回复
	party             *partyState  // 分身队（nil 表示没有队伍，STAGE-162）：只在内存里，最后一条连接离开时随 Presence 一起消失
	npcWindow         *npcWindow   // 当前打开的 NPC 对话窗口（nil 表示没开，STAGE-69）
	npcMenuV2         bool         // STAGE-135：由 npc_open 显式启用，角色的所有连接共享
	quiz              *quizSeat    // 猜谜的座位（STAGE-330，npc_quiz_seats.go）；没有座位为 nil
	quizzes           *quizHub     // 全服的猜谜座位表（和 Service 共用）
	inventoryRevision uint64
	lastMerge         time.Time                 // 上次进入合成（加工、料理）的时间，冷却用（STAGE-207，只在内存里，重新进入世界清零）
	encounterOn       bool                      // 遇敌开关（STAGE-40）：开着时战斗结束后自动开下一场
	encounterTimer    *time.Timer               // 下一场遇敌战斗的计时器（battle_end 之后隔 interval 再开）
	plan              *encounterPlan            // 按区域遇敌的计划（STAGE-152）：nil 表示只是普通的“当前位置无限遇敌”
	noEnemyUntil      time.Time                 // 龙王的守护（STAGE-340）：这个时刻之前不主动遇敌，只在内存里，角色离开世界即清零
	done              chan struct{}             // 保存并移除之后关闭
	players           *playerIndex              // 附近玩家索引（STAGE-273）：和 Service 共用，进入、离开、换图、战斗、升级时维护
	chatSent          map[string]chatSent       // 公开聊天：每个频道最近一次成功推送（STAGE-278，只在内存里）
	realParty         atomic.Pointer[realParty] // 真人队（STAGE-274，party_players.go）：nil 表示没有真人队
	parties           *partyHub                 // 全服的真人组队状态（STAGE-274）
	trade             *trade                    // 正在进行的交易（nil 表示没有，STAGE-294，trade.go）；由 p.mu 与交易锁共同保护
	hidden            atomic.Bool               // 隐身（STAGE-276，协议 5.69）：和 rec.SocialHidden 同步，供不持锁的读取（队伍摘要、附近玩家索引）
}

// Live 是一条连接在世界中的句柄。
type Live struct {
	sess *gateway.Session
	p    *Presence
	role protocol.Role
	left bool // 这条连接已经离开（受 p.mu 保护）

	// warps 是本次请求里已经提交、还没结算的 NPC 移位（STAGE-93）：在角色锁里登记，锁放开之后由 settleNPCWarps 执行。
	// 只有处理这条连接请求的协程会读写它。
	warps []npcWarp
}

// ErrLeft 表示这条连接已经离开世界（被踢、断开或主动离开），请求应当丢弃。
var ErrLeft = errors.New("角色已离开世界")

// controlError 是观察方发“操作”类请求时的错误：玩家被拒回复 CONTROLLED_BY_AGENT，agent 被拒回复 CONTROLLED_BY_PLAYER。
func controlError(role protocol.Role) *protocol.Error {
	if role == protocol.RoleAgent {
		return protocol.Errorf(protocol.CodeControlledByPlayer, "玩家正在操控，agent 目前只能观察")
	}
	return protocol.Errorf(protocol.CodeControlledByAgent, "agent 正在操控，输入 take 收回控制权")
}

// With 在持有角色锁的情况下执行 f（查看类）。这条连接已经离开时不执行并返回 ErrLeft。
func (l *Live) With(f func(r *record)) error {
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	if l.left {
		return ErrLeft
	}
	f(&l.p.rec)
	return nil
}

// Operate 同 With，但用于“操作”类请求：这条连接不是操控方时不执行，返回 CONTROLLED_BY_AGENT / CONTROLLED_BY_PLAYER。
// 网络层的守卫已经拦过一次，这里在持有锁时再确认，因为控制权可能在两次检查之间变化。
func (l *Live) Operate(f func(r *record)) error {
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	if l.left {
		return ErrLeft
	}
	if l.p.controller != l.role {
		return controlError(l.role)
	}
	f(&l.p.rec)
	return nil
}

// LiveOf 返回连接当前在世界中的句柄；不在世界中返回 nil。
func (s *Service) LiveOf(sess *gateway.Session) *Live {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lives[sess.ID()]
}

// controlView 是 enter_world / status 响应里的 control 字段：谁在操控，这条连接是操控方还是观察方。
func controlView(controller, me protocol.Role) map[string]string {
	you := "observer"
	if controller == me {
		you = "controller"
	}
	return map[string]string{"controller": string(controller), "you": you}
}

type enterRequest struct {
	Slot *int `json:"slot"`
}

func (s *Service) handleEnter(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in enterRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Slot == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 slot 字段")
	}
	if *in.Slot < 0 || *in.Slot >= MaxSlots {
		return nil, protocol.Errorf(protocol.CodeInvalidSlot, "栏位要在 0–%d 之间", MaxSlots-1)
	}
	acct := sess.AccountID()
	for {
		// 同账号的角色刚离开（比如刚断线又重连）、还在保存数据时，等它保存完再读，免得读到旧位置。
		if err := s.waitLeaving(ctx, acct); err != nil {
			return nil, err
		}
		rec, err := s.findBySlot(ctx, acct, *in.Slot)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			return nil, protocol.Errorf(protocol.CodeCharacterNotFound, "这个栏位没有角色")
		}
		// 背包随角色一起读一次（STAGE-28）；之后的变化走数据库通知重读（管理命令发物品）。
		if rec.Items, err = s.loadItems(ctx, rec.ID); err != nil {
			return nil, err
		}
		// 宠物随角色一起读一次（STAGE-31）；之后的变化同样靠数据库通知重读（管理命令给宠物）。
		if rec.Pets, err = s.loadPets(ctx, rec.ID); err != nil {
			return nil, err
		}
		// 任务进度随角色一起读一次（STAGE-86）；之后的变化靠数据库通知重读（管理命令 quest）。
		if rec.Quests, err = s.loadQuests(ctx, rec.ID); err != nil {
			return nil, err
		}
		// 不对应任务表的事件进度（STAGE-94）随角色一起读一次，之后只由任务脚本的交易改。
		if rec.EventFlags, err = s.loadStates(ctx, eventTable, rec.ID); err != nil {
			return nil, err
		}
		// 运气按游戏日掷（STAGE-61，原版 CHAR_setLuck）：同一天重登不变，跨日后进入时重掷，进入成功后写库。
		rollDailyLuck(rec, s.opts.Now(), s.luckRnd())
		if h := s.hooks.afterFind; h != nil {
			h(rec)
		}
		live, events, perr, retry := s.register(sess, rec)
		if retry {
			continue
		}
		if perr != nil {
			return nil, perr
		}
		s.writeAudit(context.WithoutCancel(ctx), events)
		if err := s.markEntered(context.WithoutCancel(ctx), rec, s.opts.Now()); errors.Is(err, errCharacterGone) {
			// 读到角色之后、登记进世界之前，角色被删除了（删除已经完成，所以没被“正在删除”标记拦住）：撤销这次进入。
			_ = s.leave(live, true, false)
			return nil, protocol.Errorf(protocol.CodeCharacterNotFound, "这个栏位没有角色")
		} else if err != nil {
			s.opts.Logger.Warn("记录进入时间失败", "character", rec.ID, "err", err)
		}
		sess.SetState(protocol.StateInWorld)
		var out map[string]any
		_ = live.With(func(r *record) { out = worldView(r, live.p, live.role, s.now()) })
		if live.p.realParty.Load() != nil { // 真人队要逐个读队员，不能在角色锁里做（STAGE-274）
			out["party"] = s.partyRealField(live.p)
		}
		out["agent"] = s.agentStatusOf(sess.Account()) // agent 是否在线（STAGE-119，docs/protocol.md 9.7）
		return out, nil
	}
}

// worldView 是 enter_world 和 status 的响应内容；在战斗中时多一个 battle 字段（协议 10.11）。
func worldView(r *record, p *Presence, me protocol.Role, now time.Time) map[string]any {
	out := map[string]any{
		"character": r.view(), "position": r.position(), "status": r.status(),
		"control":   controlView(p.controller, me),
		"encounter": p.encounterOn, "agent_online": p.members[protocol.RoleAgent] != nil, // 遇敌开关（STAGE-40，协议 5.29）
		"social":     socialView(r), // 社交开关（STAGE-276，协议 5.69）
		"duel_point": r.DuelPoint,   // 决斗点数（STAGE-306，协议 3.5）
	}
	// 按区域遇敌的权威状态（STAGE-152）：与 encounter_changed 的字段一致，没有计划时是 null。
	for k, v := range encounterChangedData(p, "") {
		if k != "encounter" {
			out[k] = v
		}
	}
	if p.battle != nil {
		out["battle"] = p.battle.b.QueryView(deadlineMS(p.battle), now.UnixMilli())
	}
	out["party"] = p.partyView(r) // 当前队伍，没有时为 null（STAGE-162，协议 5.57）
	// 存档点落点（STAGE-134）：客户端的「回城」入口用它，不用本地猜一个位置。
	// 没记录过存档点时 SavepointRespawn 会退回出生地，一样能回去。
	if floor, x, y, _ := SavepointRespawn(r); floor > 0 {
		name, _ := gamedata.MapName(floor)
		out["savepoint"] = map[string]any{"floor": floor, "x": x, "y": y, "map_name": name}
	}
	return out
}

// waitLeaving 等同账号“已开始离开”的角色保存完毕。
// 角色正在保存时 p.mu 被占着，这里只让同账号的请求去等，不能占着 s.mu 等（STAGE-12），否则全服世界内请求都跟着排队。
func (s *Service) waitLeaving(ctx context.Context, acct int64) error {
	for {
		s.mu.Lock()
		p := s.presences[acct]
		s.mu.Unlock()
		if p == nil {
			return nil
		}
		p.mu.Lock()
		var wait chan struct{}
		if p.leaving {
			wait = p.done
		}
		p.mu.Unlock()
		if wait == nil {
			return nil
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// register 把这条连接登记到账号的世界角色里（锁的顺序：先 p.mu，再 s.mu；等 p.mu 时不持有 s.mu，见 Service.mu）。
//   - 账号还没有角色在世界中：以 rec 新建，这条连接是操控方；
//   - 已经有，且是同一个角色（玩家和 agent 同时在线）：加入同一个 Presence；agent 进入时从玩家手里接管控制权；
//     玩家进入时如果 agent 在操控，玩家以观察方进入；
//   - 已经有，但是另一个角色：CHARACTER_IN_USE；
//   - 角色正在被删除：CHARACTER_NOT_FOUND；
//   - 连接已被踢或关闭：拒绝；
//   - 角色正在离开（最后一个成员已经走了、还在保存）：返回 retry，调用方等它保存完再来。
//
// 返回要写入审计的控制权变化。
func (s *Service) register(sess *gateway.Session, rec *record) (live *Live, events []auditEvent, perr *protocol.Error, retry bool) {
	role := sess.Role()
	s.mu.Lock()
	if perr := s.entryBlocked(sess, rec); perr != nil {
		s.mu.Unlock()
		return nil, nil, perr, false
	}
	p := s.presences[rec.AccountID]
	if p == nil {
		p = &Presence{acct: rec.AccountID, charID: rec.ID, rec: *rec, members: map[protocol.Role]*Live{}, controller: role, done: make(chan struct{}), players: s.players, parties: s.parties, quizzes: s.quizzes}
		s.presences[rec.AccountID] = p
		p.hidden.Store(rec.SocialHidden)
		s.players.add(p, rec.Floor, rec.Name) // STAGE-273：进入世界，同图其他角色收到 enter（隐身者不推，STAGE-276）
		live = &Live{sess: sess, p: p, role: role}
		p.members[role] = live
		s.lives[sess.ID()] = live
		s.mu.Unlock()
		return live, nil, nil, false
	}
	s.mu.Unlock()

	p.mu.Lock() // 这个账号的角色正在保存时会等在这里，但不影响别的账号
	defer p.mu.Unlock()
	if p.leaving {
		return nil, nil, nil, true
	}
	if p.charID != rec.ID {
		return nil, nil, protocol.Errorf(protocol.CodeCharacterInUse, "同一账号已经有另一个角色在世界中"), false
	}
	if p.members[role] != nil {
		return nil, nil, protocol.Errorf(protocol.CodeWrongState, "同一账号的这个身份已经在世界中"), false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if perr := s.entryBlocked(sess, rec); perr != nil { // 等 p.mu 期间连接可能被踢了
		return nil, nil, perr, false
	}
	live = &Live{sess: sess, p: p, role: role}
	p.members[role] = live
	s.lives[sess.ID()] = live
	if role == protocol.RoleAgent && p.controller == protocol.RolePlayer {
		events = p.setController(protocol.RoleAgent, ReasonAgentEntered, role)
	}
	return live, events, nil, false
}

// entryBlocked 检查连接现在能不能进入这个角色（连接已被踢、已经在世界中、角色正在删除）。调用时要持有 s.mu。
func (s *Service) entryBlocked(sess *gateway.Session, rec *record) *protocol.Error {
	switch {
	case s.gone[sess.ID()]:
		return protocol.Errorf(protocol.CodeWrongState, "连接即将断开")
	case s.lives[sess.ID()] != nil:
		return protocol.Errorf(protocol.CodeWrongState, "已经在世界中")
	case s.deleting[rec.ID]:
		return protocol.Errorf(protocol.CodeCharacterNotFound, "这个栏位没有角色")
	}
	return nil
}

func (s *Service) handleLeave(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	live := s.LiveOf(sess)
	if live == nil {
		// 状态检查已保证连接在世界中；走到这里说明刚被踢，连接马上会断开。
		return nil, protocol.Errorf(protocol.CodeWrongState, "不在世界中")
	}
	var battleErr *protocol.Error
	if err := live.With(func(r *record) {
		if live.p.battle != nil { // 战斗中不能离开世界（docs/protocol.md 10.1）
			battleErr = protocol.Errorf(protocol.CodeInBattle, "战斗中不能离开世界")
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if battleErr != nil {
		return nil, battleErr
	}
	if err := s.finish(live, false); err != nil {
		return nil, err
	}
	sess.SetState(protocol.StateLoggedIn)
	return map[string]any{}, nil
}

// finish 保存角色并让这条连接离开世界。force 为 true 时（被踢、断开）保存失败也照样离开，只记日志；
// 为 false 时（主动离开）保存失败就保留在世界中并返回错误，让玩家重试而不是悄悄丢数据。
// 同账号的另一条连接还在世界中时，角色留在世界里；离开的是操控方的话，控制权自动转给另一方（PEER_LEFT）。
func (s *Service) finish(l *Live, force bool) error { return s.leave(l, force, true) }

// leave 是 finish 的实现；doSave 为 false 时不保存（角色已经从数据库里删除了，撤销进入用）。
func (s *Service) leave(l *Live, force, doSave bool) error {
	p := l.p
	p.mu.Lock()
	if l.left {
		p.mu.Unlock()
		return nil
	}
	// 这是最后一条连接且还在战斗中：按原版先强制退出战斗（血量、宠物写回）再保存（STAGE-53）。
	// 主动 leave_world 在战斗中被拒（handleLeave），走到这里的都是断开、被踢或关服。
	if p.battle != nil && p.battle.pb != nil && len(p.members) == 1 {
		// 真人同场：掉线立即退场并离队（STAGE-275）。战斗锁在前，所以先放开本人的锁。
		pb := p.battle.pb
		p.mu.Unlock()
		s.partyOffline(pb, p)
		p.mu.Lock()
	}
	if p.battle != nil && p.battle.pb == nil && len(p.members) == 1 { // 单人和分身队的战斗（真人同场已在上面处理）
		s.exitBattleOffline(p, doSave)
	}
	if doSave {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.save(ctx, &p.rec, s.opts.Now()); err != nil {
			s.opts.Logger.Error("保存角色失败", "character", p.charID, "err", err)
			// 角色已被删除时保存永远不会成功，留在世界里没有意义，照样离开。
			if !force && !errors.Is(err, errCharacterGone) {
				p.mu.Unlock()
				return err
			}
		}
	}
	l.left = true
	delete(p.members, l.role)
	var events []auditEvent
	if len(p.members) == 0 {
		p.party = nil         // 队长的所有连接都走了：分身随之消失，重新登录不保留（STAGE-162）
		p.encounterOn = false // 最后一条连接离开：遇敌开关随角色一起关掉（STAGE-40）
		p.stopEncounterTimer()
		stopMetamo(&p.rec) // 变幻的精灵的变身不保留，重新进入世界回到自己的外观（STAGE-203）
		p.leaving = true
		p.players.remove(p) // STAGE-273：最后一条连接离开，同图其他角色收到 leave
		p.releaseQuiz()     // STAGE-330：最后一条连接离开，猜谜的座位让给别人
	} else if p.controller == l.role {
		for role := range p.members {
			events = p.setController(role, ReasonPeerLeft, l.role)
		}
	}
	if l.role == protocol.RoleAgent && len(p.members) > 0 && len(events) == 0 {
		p.pushAll("agent_presence", map[string]any{"online": false})
	}
	last := p.leaving
	p.mu.Unlock()

	s.mu.Lock()
	delete(s.lives, l.sess.ID())
	if last && s.presences[p.acct] == p {
		delete(s.presences, p.acct)
	}
	s.mu.Unlock()
	if last {
		s.partyOnGone(p)   // STAGE-274：断线 = 离队，待处理的请求 cancelled
		s.trades.onGone(p) // STAGE-294：断线 = 交易取消（PEER_LEFT），待处理的邀请作废
		close(p.done)
	}
	s.writeAudit(context.Background(), events)
	return nil
}

// BeforeKick 在连接被挤下线、账号被禁用、令牌被吊销或被玩家踢下线而断开之前调用（auth.Options.BeforeKick）：
// 先保存数据并让这条连接离开世界，之后它上面已读入但还没处理的请求都不能再改动角色。
func (s *Service) BeforeKick(sess *gateway.Session) {
	s.mu.Lock()
	s.gone[sess.ID()] = true
	live := s.lives[sess.ID()]
	s.mu.Unlock()
	if live != nil {
		_ = s.finish(live, true)
	}
}

// OnClose 在连接结束时调用（gateway.Server.OnClose）：直接断开也要保存。
func (s *Service) OnClose(sess *gateway.Session) {
	s.BeforeKick(sess)
	s.mu.Lock()
	delete(s.gone, sess.ID())
	s.mu.Unlock()
}
