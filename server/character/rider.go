package character

// 骑乘训练师（STAGE-122，docs/ride.md §2，docs/protocol.md 5.49）：talk 弹主菜单，菜单的文字和跳转
// 来自 gamedata 的 npc-riderman.json（原版 riderman.conf）；查询页、学习的确认页和学会后的消息由这里生成。
// 骑乘等级是 characters.ride_level（0/40/80/120/200），按初→中→高→特的顺序学，学费免费。
// 学会后写库，推 status_changed 和系统消息，窗口关掉。管理命令 stoneage-admin ridelevel 直接设等级。

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// DefaultRideNotifyChannel 是“角色骑乘等级变了”的 PostgreSQL 通知频道：stoneage-admin ridelevel 在同一个事务里
// NOTIFY，运行中的服务端收到后重读骑乘等级并推送 status_changed。
const DefaultRideNotifyChannel = "character_ride_changed"

// 骑乘训练师的窗口号（原版 winno）。主菜单、查询页是 JSON 里的窗口；学习的确认页是 110–140，
// 查询页是 5（原版 SHOWRIDEPET，这里改成文字页）。
const (
	riderMainWin  = 1
	riderQueryWin = 5
	riderMsgWin   = -1 // 结果消息页：只有关闭，没有选项
	riderLearnTo  = -1 // 确认页的“是”
	riderCloseTo  = -2 // 确认页的“否”
)

// riderSession 是骑乘训练师窗口的状态：当前在哪一页（窗口号）。
type riderSession struct {
	win int
}

// riderPage 是一页要显示的内容：正文、选项，以及每个选项选了之后去的地方（窗口号或上面的动作）。
type riderPage struct {
	text    string
	options []npcOption
	targets []int
}

// courseWinOf 返回课程确认页的窗口号（110 + 课程下标×10）；不是确认页时 ok 为 false。
func courseWinOf(win int) (int, bool) {
	if win < 110 || win > 110+10*(len(gamedata.RideCourses)-1) || (win-110)%10 != 0 {
		return 0, false
	}
	return (win - 110) / 10, true
}

// renderRiderPage 生成一个窗口号对应的页面；没有这一页时 ok 为 false。
func renderRiderPage(r *record, win int) (riderPage, bool) {
	if win == riderQueryWin {
		return riderPage{
			text:    riderQueryText(r),
			options: []npcOption{{No: 1, Label: "上一页"}},
			targets: []int{riderMainWin},
		}, true
	}
	if idx, ok := courseWinOf(win); ok {
		return riderPage{
			text:    gamedata.RideConfirmText(gamedata.RideCourses[idx]),
			options: []npcOption{{No: 1, Label: "是"}, {No: 2, Label: "否"}},
			targets: []int{riderLearnTo, riderCloseTo},
		}, true
	}
	w, ok := gamedata.RidermanWindowOf(win)
	if !ok {
		return riderPage{}, false
	}
	page := riderPage{text: w.Text}
	for _, o := range w.Options {
		page.options = append(page.options, npcOption{No: len(page.options) + 1, Label: o.Label})
		page.targets = append(page.targets, o.GoTo)
	}
	if w.Prev != 0 {
		page.options = append(page.options, npcOption{No: len(page.options) + 1, Label: "上一页"})
		page.targets = append(page.targets, w.Prev)
	}
	if w.Next != 0 {
		page.options = append(page.options, npcOption{No: len(page.options) + 1, Label: "下一页"})
		page.targets = append(page.targets, w.Next)
	}
	return page, true
}

// riderQueryText 是查询页的正文：当前外形能骑的虎和雷龙（docs/ride.md §3 第 10 条）。
func riderQueryText(r *record) string {
	tiger, dragon, ok := gamedata.RidePair(r.Appearance.Body, r.Appearance.Color)
	if !ok {
		return "你的外形没有能骑的宠物。"
	}
	return "你的外形能骑的宠物：\n虎：" + rideSpeciesName(tiger) + "\n雷龙：" + rideSpeciesName(dragon)
}

func rideSpeciesName(templateID int) string {
	if t, ok := gamedata.PetByID(templateID); ok {
		return t.Name
	}
	return "（未知）"
}

// openRider 是 talk 骑乘训练师：弹主菜单。调用时要持有 p.mu（在 talk 的 Operate 里）。
func openRider(p *Presence, npc gamedata.NPC, r *record) {
	page, _ := renderRiderPage(r, riderMainWin)
	p.npcWindow = &npcWindow{npc: npc.ID, rider: &riderSession{win: riderMainWin}}
	p.pushAll("npc_window", npcWindowView(npc, page.text, page.options...))
}

// riderShow 把窗口切到 win 这一页（先关旧窗口，同翻页的推送顺序）。调用时要持有 p.mu。
func riderShow(p *Presence, npc gamedata.NPC, win int, page riderPage) {
	p.pushWindowClosed(WindowReasonChosen)
	p.npcWindow = &npcWindow{npc: npc.ID, rider: &riderSession{win: win}}
	p.pushAll("npc_window", npcWindowView(npc, page.text, page.options...))
}

// riderMessage 弹一个只有正文的消息页（学过了、没学前一级），玩家用 npc_close 关掉。
func riderMessage(p *Presence, npc gamedata.NPC, text string) {
	p.pushWindowClosed(WindowReasonChosen)
	p.npcWindow = &npcWindow{npc: npc.ID, rider: &riderSession{win: riderMsgWin}}
	p.pushAll("npc_window", npcWindowView(npc, text))
}

// riderCourseCheck 检查能不能学第 idx 级课程：已经学过（或更高）回“不需要再学了”，没学前一级回
// “必须先学过”；都没有时返回空串。规则同原版 npc_riderman.c（学费免费，不检查金币）。
func riderCourseCheck(rideLevel, idx int) string {
	c := gamedata.RideCourses[idx]
	if rideLevel >= c.Level {
		return gamedata.RideLearnedText(c)
	}
	if idx > 0 && rideLevel < gamedata.RideCourses[idx-1].Level {
		return gamedata.RidePrereqText(gamedata.RideCourses[idx-1], c)
	}
	return ""
}

// riderChoose 处理骑乘训练师窗口的选择（npc_choose）。调用时要持有 p.mu。
func (s *Service) riderChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, w *npcWindow, no int) *protocol.Error {
	cur := w.rider.win
	page, ok := renderRiderPage(r, cur)
	if !ok || no < 1 || no > len(page.options) {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
	}
	target := page.targets[no-1]
	switch target {
	case riderCloseTo:
		l.p.pushWindowClosed(WindowReasonChosen)
		return nil
	case riderLearnTo:
		return s.riderLearn(ctx, l, r, npc, cur)
	}
	if idx, isCourse := courseWinOf(target); isCourse {
		// 选了课程：先看能不能学，不能学就弹消息页，能学才打开确认页（docs/ride.md §2）。
		if msg := riderCourseCheck(r.RideLevel, idx); msg != "" {
			riderMessage(l.p, npc, msg)
			return nil
		}
	}
	next, ok := renderRiderPage(r, target)
	if !ok {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "骑乘训练师没有这一页")
	}
	riderShow(l.p, npc, target, next)
	return nil
}

// riderLearn 在确认页选“是”：再检查一次（窗口开着的时候可能已经学过），通过就写库、推送、关窗。
func (s *Service) riderLearn(ctx context.Context, l *Live, r *record, npc gamedata.NPC, win int) *protocol.Error {
	idx, ok := courseWinOf(win)
	if !ok {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 1 号选项")
	}
	if msg := riderCourseCheck(r.RideLevel, idx); msg != "" {
		riderMessage(l.p, npc, msg)
		return nil
	}
	c := gamedata.RideCourses[idx]
	if _, err := s.db.Exec(ctx, `UPDATE characters SET ride_level = $2 WHERE id = $1`, r.ID, c.Level); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.RideLevel = c.Level
	l.p.pushWindowClosed(WindowReasonChosen)
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushAll("notice", map[string]string{"level": "info", "text": gamedata.RideLearnedNotice(c)})
	return nil
}

// SetRideLevel 是管理命令（stoneage-admin ridelevel）：直接设角色的骑乘等级，在同一个事务里 NOTIFY，
// 运行中的服务端收到后重读并推送 status_changed。level 只能是 0/40/80/120/200。
func SetRideLevel(ctx context.Context, db *pgxpool.Pool, channel, character string, level int) error {
	if _, ok := gamedata.RideCourseIndex(level); !ok && level != gamedata.RideLevelNone {
		return fmt.Errorf("骑乘等级只能是 0、40、80、120 或 200，不能是 %d", level)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx,
		`UPDATE characters SET ride_level = $2 WHERE name = $1 RETURNING id`,
		character, level,
	).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// reloadRide 收到通知后重读角色的骑乘等级，推送 status_changed。角色不在世界中时没有可通知的连接，跳过。
func (s *Service) reloadRide(ctx context.Context, payload string) {
	charID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.opts.Logger.Warn("骑乘等级通知的 payload 不是角色编号", "payload", payload)
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.leaving {
		return
	}
	var level int
	if err := s.db.QueryRow(ctx, `SELECT ride_level FROM characters WHERE id = $1`, charID).Scan(&level); err != nil {
		s.opts.Logger.Warn("重读角色的骑乘等级失败", "character", charID, "err", err)
		return
	}
	p.rec.RideLevel = level
	p.pushAll("status_changed", map[string]any{"status": p.rec.status()})
}
