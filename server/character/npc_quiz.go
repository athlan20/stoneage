package character

// STAGE-330 / STAGE-332：猜谜人物（原版 gmsv/npc/npc_quiz.c）的对话流程。talk 后：组队不让参加；任务 2 没完成说
// 一句话（本单补的，原版没有）；agent 连接直接认定（npc_quiz_agent.go）；人类连接开始窗口（是/否）。选“是”：没有
// 参赛道具说 NoEntryMsg；同图人数满说等待；都通过就交出参赛道具、占座位，逐题出题（选择题用 npc_choose，文字题用
// npc_reply）。答满后先发道具（GetItem，背包满就留在座位上，原版同样），再发台词（Border/FailureMsg，原版是聊天行，
// 这里是消息栏的 notice），释放座位，最后按 Warp 传送。
// 抽题、判题、过关的规则在 gamedata/npcquiz_logic.go；座位在 npc_quiz_seats.go；参赛道具和发奖在 npc_quiz_entry.go。

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// quizKind 是猜谜人物的 NPC 类型（gen-npcs.py 的 kind，原版 npcgen_quiz）。
const quizKind = "npcgen_quiz"

// quizReach 是能和猜谜人物说话的距离（格，切比雪夫距离；原版 NPC_QuizTalked 的 CharDistance > 1 直接不理）。
const quizReach = 1

// quizQuestEvent 是任务 2「猜谜3兄弟」的事件号（quiz/equiz_01 接、equiz_02 完成）：猜谜大师要求它已经完成。
// 原版猜谜大师本身不查任务，入门证 2753 只有完成任务 2 的路上才拿得到；这里多查一步（任务书的要求）。
const quizQuestEvent = 44

// quizNoQuestMsg 是任务 2 没完成时的台词。原版没有这一句（原版靠入门证挡人），这里是本单补的。
const quizNoQuestMsg = "还没有完成猜谜三兄弟的课题，先去找他们办完吧。"

// quizWaitMsg 是同一位人物已经有 8 人在答题时的台词（原版源文件里这句是乱码，这里按意思重写）。
const quizWaitMsg = "请稍等一下\n现在有其他人正在挑战中，请稍候再来。"

// 猜谜窗口的阶段（quizSession.stage）。
type quizStage int

const (
	qsStart  quizStage = iota // 开始窗口：是 / 否
	qsChoice                  // 选择题：选项；关窗口就是放弃（npc_close）
	qsText                    // 文字题：文字回复（npc_reply）
)

// quizSession 是猜谜窗口的状态（挂在 npcWindow.quiz 上）；seat 是这一场占着的座位。
type quizSession struct {
	stage quizStage
	seat  *quizSeat
}

// quizConfigOf 返回猜谜人物的参数。参数文件名由地图和格子定：quz_<地图>_<x>_<y>（quizman.create 的 genout 文件名）。
// 不是猜谜人物或没有参数时返回 nil。
func quizConfigOf(npc gamedata.NPC) *gamedata.NpcQuiz {
	if npc.Kind != quizKind {
		return nil
	}
	return gamedata.NpcQuizOf(fmt.Sprintf("quz_%d_%d_%d", npc.Floor, npc.X, npc.Y))
}

// quizText 把参数里的台词换成正文：参数文件里的换行是字面的 \n（原版 C 字符串里的换行）。
func quizText(s string) string {
	return strings.ReplaceAll(s, `\n`, "\n")
}

// quizRnd 是猜谜的随机源：测试和用例里跟对话选句用同一个固定源，正式运行用全局随机。
func (s *Service) quizRnd() func(n int) int {
	if f := s.talkRnd(); f != nil {
		return f
	}
	return rand.IntN
}

// quizDistance 检查角色离猜谜人物够不够近（同一张图，切比雪夫距离不超过 quizReach）。
func quizDistance(r *record, npc gamedata.NPC) *protocol.Error {
	if r.Floor != npc.Floor || max(abs(r.X-npc.X), abs(r.Y-npc.Y)) > quizReach {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "离%s太远了，走近一点再说", npcDisplayName(npc))
	}
	return nil
}

// quizOpen 打开（或替换）猜谜窗口。sess 为 nil 时只是一句话（没有座位和题目）。typed 为 true 时窗口接受文字回复。
// 调用时要持有 p.mu。
func quizOpen(p *Presence, npc gamedata.NPC, sess *quizSession, text string, options []npcOption, typed bool) {
	p.npcWindow = &npcWindow{npc: npc.ID, options: options, quiz: sess}
	view := npcWindowView(npc, text, options...)
	if typed {
		view["accepts_text"] = true
	}
	p.pushAll("npc_window", view)
}

// quizSay 说一句台词（只有正文、没有选项的窗口）；台词为空时什么都不说（参数里没有这一句）。
func quizSay(p *Presence, npc gamedata.NPC, msg string) {
	if msg == "" {
		return
	}
	quizOpen(p, npc, nil, quizText(msg), nil, false)
}

// talkQuiz 是 talk 猜谜人物（原版 NPC_QuizTalked）。调用时要持有 p.mu。
// 原版在 talk 时不查背包：道具栏满只在答满、发奖时说“道具栏已满”（quizFinish）。
func (s *Service) talkQuiz(ctx context.Context, l *Live, r *record, npc gamedata.NPC, cfg *gamedata.NpcQuiz) *protocol.Error {
	p := l.p
	if perr := quizDistance(r, npc); perr != nil {
		return perr
	}
	if inQuizTeam(p) { // 组队不让参加（原版 NPC_QUIZPARTY_CHAECK）
		quizSay(p, npc, cfg.PartyMsg)
		return nil
	}
	if !r.eventDone(quizQuestEvent) { // 本单补的：任务 2 没完成不让挑战（台词见 quizNoQuestMsg）
		quizSay(p, npc, quizNoQuestMsg)
		return nil
	}
	if l.role == protocol.RoleAgent { // agent 直接认定，不出题（STAGE-332 方案 A，npc_quiz_agent.go）
		return s.quizAgentPass(ctx, l, r, npc, cfg)
	}
	options := []npcOption{{No: 1, Label: "是"}, {No: 2, Label: "否"}}
	quizOpen(p, npc, &quizSession{stage: qsStart}, quizText(cfg.StartMsg), options, false)
	return nil
}

// quizStale 报告这一场的座位是不是已经失效（被清掉了）。失效时关窗口，什么都不做（原版 NPC_QuizGetPlayer 返回 NULL）。
func (s *Service) quizStale(l *Live, q *quizSession) bool {
	if q.seat == nil || s.quizzes.isActive(q.seat) {
		return false
	}
	l.p.pushWindowClosed(WindowReasonClosed)
	return true
}

// quizChoose 回应猜谜窗口的选项（npc_choose）。调用时要持有 p.mu。
func (s *Service) quizChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, w *npcWindow, no int) *protocol.Error {
	q := w.quiz
	if perr := quizDistance(r, npc); perr != nil {
		return perr
	}
	switch q.stage {
	case qsStart:
		if no != 1 && no != 2 {
			return quizBadChoice(no)
		}
		if no == 2 { // 否：直接关窗
			l.p.pushWindowClosed(WindowReasonChosen)
			return nil
		}
		cfg := quizConfigOf(npc)
		if cfg == nil {
			return protocol.Errorf(protocol.CodeNPCNotTalkable, "%s 还不能对话", npcDisplayName(npc))
		}
		return s.quizStartYes(ctx, l, r, npc, cfg)
	case qsChoice:
		if no < 1 || no > len(w.options) {
			return quizBadChoice(no)
		}
		if s.quizStale(l, q) {
			return nil
		}
		q.seat.run.AnswerChoice(no)
		return s.quizAnswered(ctx, l, r, npc, q.seat)
	}
	return quizBadChoice(no) // 文字题没有选项
}

// quizReply 回应文字题（npc_reply）：输入里包含正确答案就算对（原版 strstr），答了就进下一题。调用时要持有 p.mu。
func (s *Service) quizReply(ctx context.Context, l *Live, r *record, npc gamedata.NPC, w *npcWindow, text string) *protocol.Error {
	q := w.quiz
	if q.stage != qsText {
		return protocol.Errorf(protocol.CodeBadRequest, "这个窗口不接受文字回复")
	}
	if perr := quizDistance(r, npc); perr != nil {
		return perr
	}
	if s.quizStale(l, q) {
		return nil
	}
	q.seat.run.AnswerText(text)
	return s.quizAnswered(ctx, l, r, npc, q.seat)
}

func quizBadChoice(no int) *protocol.Error {
	return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
}

// quizStartYes 是开始窗口选“是”（原版 NPC_Quiz_selectWindow 的 1）：参赛道具不够说 NoEntryMsg；同图人数满说等待；
// 都通过就交出参赛道具、占座位、出第一题。调用时要持有 p.mu。
func (s *Service) quizStartYes(ctx context.Context, l *Live, r *record, npc gamedata.NPC, cfg *gamedata.NpcQuiz) *protocol.Error {
	p := l.p
	p.pushWindowClosed(WindowReasonChosen)
	if !quizEntryOK(r, cfg.EntryItem) {
		quizSay(p, npc, cfg.NoEntryMsg)
		return nil
	}
	seat, ok := s.quizzes.acquire(p, npc)
	if !ok {
		quizSay(p, npc, quizWaitMsg)
		return nil
	}
	run, err := gamedata.NewQuizRun(cfg, gamedata.QuizBank())
	if err != nil {
		s.quizzes.release(seat)
		return protocol.Errorf(protocol.CodeInternal, "猜谜参数不对：%v", err)
	}
	seat.run = run
	if perr := s.commitEvent(ctx, l, r, quizEntryResult(r, cfg.EntryItem)); perr != nil {
		s.quizzes.release(seat)
		return perr
	}
	p.quiz = seat
	return s.quizAsk(l, r, npc, seat)
}

// quizAsk 出下一题（原版 NPC_Quiz_selectWindow 的 2）：选择题是选项窗口，文字题是文字输入窗口。
func (s *Service) quizAsk(l *Live, r *record, npc gamedata.NPC, seat *quizSeat) *protocol.Error {
	ask, ok := seat.run.Next(s.quizRnd())
	if !ok {
		return protocol.Errorf(protocol.CodeInternal, "猜谜出题失败")
	}
	head := fmt.Sprintf("quiz 第 %d 题\n%s", ask.No, quizText(ask.Text))
	if ask.Typed {
		text := head + "\n↓写在这里\n?(写完请按ＯＫ)"
		quizOpen(l.p, npc, &quizSession{stage: qsText, seat: seat}, text, nil, true)
		return nil
	}
	options := make([]npcOption, len(ask.Options))
	for i, o := range ask.Options {
		options[i] = npcOption{No: i + 1, Label: o}
	}
	quizOpen(l.p, npc, &quizSession{stage: qsChoice, seat: seat}, head, options, false)
	return nil
}

// quizAnswered 答完一题之后：关掉答题窗口，答满了就结算，否则出下一题。
func (s *Service) quizAnswered(ctx context.Context, l *Live, r *record, npc gamedata.NPC, seat *quizSeat) *protocol.Error {
	l.p.pushWindowClosed(WindowReasonChosen)
	if seat.run.Finished() {
		return s.quizFinish(ctx, l, r, npc, seat)
	}
	return s.quizAsk(l, r, npc, seat)
}

// quizFinish 答满了（原版 NPC_Quiz_selectWindow 的 2，quizno 等于 Quiznum 那一支）：先发道具，背包满时只说
// “道具栏已满”，座位保留（原版同样不释放）；道具发成后说 Border 台词，释放座位，最后按 Warp 传送。
func (s *Service) quizFinish(ctx context.Context, l *Live, r *record, npc gamedata.NPC, seat *quizSeat) *protocol.Error {
	p := l.p
	res := seat.run.Result(s.quizRnd())
	if res.Item != 0 {
		next, ok := quizGrant(r, r.Items, res.Item)
		if !ok {
			p.pushAll("notice", map[string]string{"level": "info", "text": "道具栏已满"})
			return nil
		}
		if perr := s.commitEvent(ctx, l, r, next); perr != nil {
			return perr
		}
	}
	if res.Message != "" {
		p.pushAll("notice", map[string]string{"level": "info", "text": npcDisplayName(npc) + "：" + quizText(res.Message)})
	}
	p.releaseQuiz()
	if res.Warp != nil {
		s.teleport(l, r, res.Warp.Floor, res.Warp.X, res.Warp.Y)
	}
	return nil
}
