package character

// NPC 对话窗口（STAGE-69，docs/protocol.md 5.33–5.36）：talk 打开、npc_choose 回应、
// npc_close 关闭，warp 和战斗开始时自动关。窗口是角色的状态（Presence.npcWindow），
// 推给玩家和 agent 的所有连接；选项列表本期恒空，商店等带选项的窗口在各自需求里接入。
// 文字内容见 gamedata（npc-talk.json），规则出处 npc_townpeople.c / npc_signboard.c /
// npc_timeman.c / npcgen_mugon。

import (
	"context"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// npc_window_closed 的 reason（docs/protocol.md 5.34）。
const (
	WindowReasonChosen = "CHOSEN" // 选项已被选择
	WindowReasonClosed = "CLOSED" // 某条连接主动关闭
	WindowReasonMoved  = "MOVED"  // 换了地图
	WindowReasonBattle = "BATTLE" // 战斗开始
)

// npcOption 是窗口里的一个选项。本期对话都没有选项，结构留给商店（STAGE-70 起）。
type npcOption struct {
	No      int          `json:"no"`
	Label   string       `json:"label"`
	Product *shopProduct `json:"product,omitempty"`
}

// npcWindow 是一个角色当前打开的对话窗口：正在和谁说话、有哪些选项，
// 商店对话另带阶段（shop.go 的状态机）。
type npcWindow struct {
	id       string
	npc      int
	options  []npcOption
	shop     *shopSession         // 商店对话的阶段与待定商品；普通对话为 nil
	petshop  *petshopSession      // 宠物店对话的阶段与待定宠物（STAGE-73）
	poolitem *poolItemSession     // 道具仓库对话的阶段与待定格子（STAGE-309，npc_poolitem.go）
	petskill *petskillShopSession // 宠物技能店对话的阶段与已选的技能、宠物、栏位（STAGE-143）
	event    *eventSession        // 任务 NPC 的“是/否”问答（STAGE-90）；普通对话为 nil
	fight    bool                 // 战斗 NPC 的“是/否”确认（STAGE-96，npcenemy.go）
	rider    *riderSession        // 骑乘训练师的当前页（STAGE-122，rider.go）；其他 NPC 为 nil
	pettrans *petTransSession     // 宠物转生的当前步骤（STAGE-236，npc_pettrans.go）；其他 NPC 为 nil
	charm    *charmSession        // 美容师的当前步骤（STAGE-255，charm.go）；其他 NPC 为 nil
	tensei   *tenseiSession       // 主城精灵王（人物转生）的当前步骤（STAGE-266，npc_tensei.go）；其他 NPC 为 nil
	duelRank *duelRankSession     // 强者表的菜单或当前页（STAGE-308，npc_duelranking.go）；其他 NPC 为 nil
	quiz     *quizSession         // 猜谜的当前窗口（STAGE-330，npc_quiz.go）；其他窗口为 nil
}

// talkableKind 判断这个原版类型本期有没有对话逻辑（docs/protocol.md 5.33 的四类）。
func talkableKind(kind string) bool {
	switch kind {
	case "npcgen_man", "npcgen_signboard", "npcgen_timeman", "npcgen_mugon":
		return true
	}
	return false
}

// handleTalk 和 NPC 对话（操作类，docs/protocol.md 5.33）：校验通过后记住窗口并推送
// npc_window；失败不弹窗。观察方由 Operate 拦，战斗中由 inBattle 拦。
func (s *Service) handleTalk(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Npc *int `json:"npc"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Npc == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要整数字段 npc（NPC 编号，见 npcs）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	defer s.settleNPCWarps(l) // 任务脚本的 NpcWarp 在角色锁之外结算（npcwarp.go）
	var perr *protocol.Error
	if err := l.Operate(func(r *record) {
		if e := inBattle(l); e != nil { // 战斗中不能对话（docs/protocol.md 10.1）
			perr = e
			return
		}
		l.p.npcMenuV2 = req.Type == "npc_open"
		npc, ok := s.npcs.ByID(*in.Npc)
		if !ok {
			perr = protocol.Errorf(protocol.CodeNPCNotFound, "没有编号为 %d 的 NPC", *in.Npc)
			return
		}
		if npc.Floor != r.Floor {
			perr = protocol.Errorf(protocol.CodeNPCHere, "%s 不在这张地图上（它在地图 %d）", npcDisplayName(npc), npc.Floor)
			return
		}
		// 同图超出对话距离时先走到 NPC 旁边（STAGE-263，talk_approach.go）；找不到空格就不动，
		// 由下面各 NPC 自己的距离检查回复“太远了”。
		l.p.releaseQuiz() // 再次 talk（或换一个 NPC 说话）：先放掉猜谜的座位（STAGE-330）
		s.approachNPC(l.p, r, npc, talkReach(npc))
		if shop := gamedata.NpcShopOf(npc.ID); shop != nil && (npc.Kind == "npcgen_shop" || npc.Kind == "npcgen_limitshop") {
			// 商店（STAGE-70）：不是普通的“说话”，而是买卖菜单（docs/protocol.md 5.38）。
			s.openShop(l.p, npc, shop)
			return
		}
		if healer := gamedata.NpcHealerOf(npc.ID); healer != nil && npc.Kind == "npcgen_winhealer" {
			// 护士（STAGE-71）：治疗确认窗口（docs/protocol.md 5.39）。
			s.openHealer(l.p, npc, healer)
			return
		}
		if npc.Kind == "npcgen_charm" {
			// 美容师（STAGE-255，原版 npc_charm.c）：花石币买魅力，三步窗口（charm.go）。
			openCharm(l.p, npc)
			return
		}
		if sp := gamedata.NpcSavepointOf(npc.ID); sp != nil && npc.Kind == "npcgen_savepoint" {
			// 存档点（STAGE-72）：记录窗口（docs/protocol.md 5.40）。
			s.openSavepoint(l.p, npc, sp)
			return
		}
		if ps := gamedata.NpcPetshopOf(npc.ID); ps != nil && npc.Kind == "npcgen_petshop" {
			// 宠物店（STAGE-73）：买卖寄放菜单（docs/protocol.md 5.41）。
			s.openPetshop(l.p, npc, ps)
			return
		}
		if npc.Kind == poolItemKind {
			// 道具仓库（STAGE-309）：寄存、领取菜单（docs/protocol.md 5.60）。
			s.openPoolItemShop(l.p, npc)
			return
		}
		if pss := gamedata.NpcPetskillShopOf(npc.ID); pss != nil && npc.Kind == "npcgen_petskillshop" {
			// 宠物技能店（STAGE-143）：技能列表，离店太远时回复错误（docs/protocol.md 5.51）。
			perr = s.openPetskillShop(l, r, npc, pss)
			return
		}
		if wm := gamedata.NpcWarpmanOf(npc.ID); wm != nil && npc.Kind == "npcgen_warpman" {
			// 传送员（STAGE-74）：按通行条件与费用传送（docs/protocol.md 5.42）。
			// 依赖任务/庄园/家族条件的传送员本期跳过，不放行也不弹窗（验收 4）。
			if wm.Unsupported {
				perr = protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 暂时不能送你去别处", npcDisplayName(npc))
				return
			}
			perr = s.openWarpman(l.p, npc, wm)
			return
		}
		if npc.Kind == "npcenemy" {
			// 战斗 NPC（STAGE-96，docs/battle-npc.md）：参数放开了才能打，其余的回“还不能对话”。
			ne, ok := gamedata.NpcEnemyOf(npc.ID)
			if !ok || ne.Unsupported != "" || !s.npcEnemyOpen(ne.File) {
				perr = protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 还不能对话", npcDisplayName(npc))
				return
			}
			perr = s.talkEnemy(l, r, npc, ne)
			return
		}
		if npc.Kind == "changeevent" {
			// 任务 NPC（STAGE-90，docs/quest.md）：脚本放开了才说话，其余的回“还不能对话”。
			ev, ok := gamedata.NpcEventOf(npc.ID)
			if !ok || !s.npcEventOpen(ev.File) {
				perr = protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 还不能对话", npcDisplayName(npc))
				return
			}
			perr = s.talkEvent(ctx, l, r, npc, ev)
			return
		}
		if cfg := quizConfigOf(npc); cfg != nil { // 猜谜人物（STAGE-330，npc_quiz.go）：开始窗口或组队、背包满的台词
			perr = s.talkQuiz(ctx, l, r, npc, cfg)
			return
		}
		if npc.Kind == "riderman" {
			// 骑乘训练师（STAGE-122，docs/ride.md §2）：主菜单，学骑乘课程（docs/protocol.md 5.49）。
			openRider(l.p, npc, r)
			return
		}
		if npc.Kind == duelRankNPCKind { // 强者表（STAGE-308，npc_duelranking.go）：菜单，选项后查排行
			openDuelRankMenu(l.p, npc)
			return
		}
		if npc.Kind == tenseiKind {
			// 主城精灵王（STAGE-266，人物转生）：资格检查后开窗，窗口见 npc_tensei.go。
			perr = s.talkTensei(l, r, npc)
			return
		}
		if npc.Kind == petTransKind {
			// 宠物转生（STAGE-236，精灵王）：按身上的玛蕾分流，窗口见 npc_pettrans.go。
			perr = s.talkPetTrans(l, r, npc)
			return
		}
		if !talkableKind(npc.Kind) {
			perr = protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 还不能对话", npcDisplayName(npc))
			return
		}
		l.p.npcWindow = &npcWindow{npc: npc.ID} // 已经开着窗口时直接替换（关旧开新）
		l.p.pushAll("npc_window", npcWindowView(npc, s.talkText(npc)))
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{}, nil
}

// npcDisplayName 是错误提示里对 NPC 的称呼：有名字用名字，没有的用类型名（原版
// timeman/mugon 的 name 可能为空，提示里不能是空串）。
func npcDisplayName(npc gamedata.NPC) string {
	if npc.Name != "" {
		return npc.Name
	}
	return npc.Type
}

// talkText 取这个 NPC 这次说的话：有参数的按类型取（多句随机），没有的回「……」。
func (s *Service) talkText(npc gamedata.NPC) string {
	return gamedata.NpcTalkOf(npc.ID).TalkTextAt(s.opts.Now(), s.talkRnd())
}

// talkRnd 返回对话选句的随机源：测试里按用例名固定种子，生产环境用全局随机（同 luckRnd）。
func (s *Service) talkRnd() func(n int) int {
	if s.opts.NpcTalkRnd != nil {
		return s.opts.NpcTalkRnd
	}
	return nil
}

// npcWindowView 是推送 npc_window 的 data（docs/protocol.md 5.34）。
func npcWindowView(npc gamedata.NPC, text string, options ...npcOption) map[string]any {
	if options == nil {
		options = []npcOption{} // 没有选项时是 []，不是 null
	}
	return map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": text, "options": options,
	}
}

// pushWindowClosed 清掉窗口状态并给所有连接推 npc_window_closed。没有窗口时不动。
// 调用时要持有 p.mu；reason 的取值见文件头（docs/protocol.md 5.34）。
func (p *Presence) pushWindowClosed(reason string) {
	if reason != WindowReasonChosen { // 关掉、换图、战斗：猜谜的座位也放掉（STAGE-330）；选项引发的换页不放
		p.releaseQuiz()
	}
	if p.npcWindow == nil {
		return
	}
	p.npcWindow = nil
	p.pushAll("npc_window_closed", map[string]any{"reason": reason})
}

// handleNpcChoose 回应对话窗口（操作类，docs/protocol.md 5.35）：普通对话选选项后关窗口
// （本期都没有选项，任何选择都回 NPC_BAD_CHOICE）；商店窗口按阶段推进（shop.go）。
func (s *Service) handleNpcChoose(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		No       *int   `json:"no"`
		WindowID string `json:"window_id"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.No == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要整数字段 no（选项编号）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	defer s.settleNPCWarps(l) // 任务脚本的 NpcWarp 在角色锁之外结算（npcwarp.go）
	var perr *protocol.Error
	if err := l.Operate(func(r *record) {
		w := l.p.npcWindow
		if w == nil {
			perr = protocol.Errorf(protocol.CodeWrongState, "没有打开的对话窗口")
			return
		}
		if perr = l.p.checkWindow(in.WindowID); perr != nil {
			return
		}
		if w.shop != nil {
			perr = s.shopChoose(ctx, l, r, w, *in.No)
			return
		}
		if w.fight && w.npc != 0 { // 战斗 NPC 的是/否（STAGE-96）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.fightChoose(l, r, npc, *in.No)
			}
			return
		}
		if w.event != nil && w.npc != 0 {
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.eventChoose(ctx, l, r, npc, w, *in.No)
			}
			return
		}
		if w.duelRank != nil && w.npc != 0 { // 强者表（STAGE-308）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.duelRankChoose(ctx, l, r, npc, w, *in.No)
			}
			return
		}
		if w.quiz != nil && w.npc != 0 { // 猜谜（STAGE-330，npc_quiz.go）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.quizChoose(ctx, l, r, npc, w, *in.No)
			}
			return
		}
		if w.tensei != nil && w.npc != 0 { // 主城精灵王（STAGE-266）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.tenseiChoose(ctx, l, r, npc, w.tensei, *in.No)
			}
			return
		}
		if w.pettrans != nil && w.npc != 0 { // 宠物转生（STAGE-236）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.petTransChoose(ctx, l, r, npc, w.pettrans, *in.No)
			}
			return
		}
		if w.charm != nil && w.npc != 0 { // 美容师（STAGE-255）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.charmChoose(l, r, w, npc, *in.No)
			}
			return
		}
		if w.rider != nil && w.npc != 0 { // 骑乘训练师（STAGE-122）
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.riderChoose(ctx, l, r, npc, w, *in.No)
			}
			return
		}
		if gamedata.NpcHealerOf(w.npc) != nil {
			perr = s.handleHealerChoose(ctx, l, r, w, *in.No)
			return
		}
		if sp := gamedata.NpcSavepointOf(w.npc); sp != nil && w.npc != 0 {
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.savepointChoose(ctx, l, r, npc, sp, *in.No)
			}
			return
		}
		if w.poolitem != nil && w.npc != 0 {
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.poolItemChoose(ctx, l, r, w, npc, *in.No)
			}
			return
		}
		if ps := gamedata.NpcPetshopOf(w.npc); ps != nil && w.npc != 0 {
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.petshopChoose(ctx, l, r, w, npc, ps, *in.No)
			}
			return
		}
		if pss := gamedata.NpcPetskillShopOf(w.npc); pss != nil && w.npc != 0 && w.petskill != nil {
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.petskillShopChoose(ctx, l, r, w, npc, pss, *in.No)
			}
			return
		}
		if wm := gamedata.NpcWarpmanOf(w.npc); wm != nil && w.npc != 0 {
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.warpmanChoose(ctx, l, r, npc, wm, *in.No)
			}
			return
		}
		if *in.No < 1 || *in.No > len(w.options) {
			perr = protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", *in.No)
			return
		}
		l.p.pushWindowClosed(WindowReasonChosen)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{}, nil
}

// handleNpcReply 对窗口的文字回复（操作类，docs/protocol.md 5.37）：商店买数量的输入、任务 NPC 的暗号等。
// 普通对话窗口不接受文字回复。
func (s *Service) handleNpcReply(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Text     *string `json:"text"`
		WindowID string  `json:"window_id"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Text == nil || strings.TrimSpace(*in.Text) == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 text 字段（回复的内容）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	defer s.settleNPCWarps(l) // 暗号也能触发任务脚本的 NpcWarp（npcwarp.go）
	var perr *protocol.Error
	if err := l.Operate(func(r *record) {
		w := l.p.npcWindow
		if w == nil {
			perr = protocol.Errorf(protocol.CodeWrongState, "没有打开的对话窗口")
			return
		}
		if perr = l.p.checkWindow(in.WindowID); perr != nil {
			return
		}
		if w.event != nil && w.event.ask == askSpeak {
			// 暗号原样比对，只去掉首尾的换行（不去空格，大小写照旧）。
			if npc, ok := gamedata.NPCByID(w.npc); ok {
				perr = s.eventSpeak(ctx, l, r, npc, strings.Trim(*in.Text, "\r\n"))
			}
			return
		}
		if w.quiz != nil && w.npc != 0 { // 猜谜的文字题（STAGE-330）：去掉首尾的换行，大小写和空格照原样比对
			if npc, ok := s.npcs.ByID(w.npc); ok {
				perr = s.quizReply(ctx, l, r, npc, w, strings.Trim(*in.Text, "\r\n"))
			}
			return
		}
		if w.shop == nil {
			perr = protocol.Errorf(protocol.CodeBadRequest, "这个窗口不接受文字回复")
			return
		}
		perr = s.shopReply(ctx, l, r, w, strings.TrimSpace(*in.Text))
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{}, nil
}

// handleNpcClose 关闭对话窗口（确认类，docs/protocol.md 5.36）：观察方也可以发（和
// battle_ack 一样，用 With 不用 Operate），没有窗口时也回复成功（幂等）。
func (s *Service) handleNpcClose(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
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
	if err := l.With(func(r *record) {
		if l.p.npcWindow != nil {
			if perr = l.p.checkWindow(in.WindowID); perr != nil {
				return
			}
		}
		l.p.pushWindowClosed(WindowReasonClosed)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if perr != nil {
		return nil, perr
	}
	return map[string]any{}, nil
}
