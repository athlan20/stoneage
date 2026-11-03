// 任务进度（STAGE-86，docs/protocol.md 5.44、5.45，docs/ui.md 任务页）：quests 查询、quests_changed 推送，
// 以及管理命令 quest 写库后的通知重读。进度按任务编号存在 character_quests，进入世界时读一次。

package character

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// DefaultQuestNotifyChannel 是“角色任务进度变了”的 PostgreSQL 通知频道：stoneage-admin quest
// 在同一个事务里 NOTIFY，运行中的服务端收到后重读该角色的任务并推送 quests_changed。
const DefaultQuestNotifyChannel = "character_quests_changed"

// 任务进度的状态（character_quests.state，见迁移 0013）。没有行表示没接过。
const (
	questDoing = 1 // 进行中
	questDone  = 2 // 已完成
)

// questRec 是 character_quests 的一行。
type questRec struct {
	No    int
	State int
}

// questView 是 quests 响应和 quests_changed 推送里的一项（docs/protocol.md 5.44）。
type questView struct {
	No    int    `json:"no"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// questsView 列出有进度且在任务表里的任务：进行中在前，同一状态内按编号从小到大。
// 没有任何任务时返回空切片（不是 nil），JSON 里是 []。
func (r *record) questsView() []questView {
	out := make([]questView, 0, len(r.Quests))
	for _, q := range r.Quests {
		t, ok := gamedata.QuestByNo(q.No)
		if !ok {
			continue
		}
		out = append(out, questView{No: q.No, Name: t.Name, State: questStateName(q.State)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].State != out[j].State {
			return out[i].State == questStateName(questDoing)
		}
		return out[i].No < out[j].No
	})
	return out
}

func questStateName(state int) string {
	if state == questDone {
		return "done"
	}
	return "doing"
}

// loadQuests 读角色现有的任务进度，按任务编号从小到大。
func (s *Service) loadQuests(ctx context.Context, charID int64) ([]questRec, error) {
	return s.loadStates(ctx, questTable, charID)
}

// changedQuests 比较两次读到的进度，返回状态不同（含新增、清除）的任务编号，按编号升序。
func changedQuests(before, after []questRec) []int {
	old := make(map[int]int, len(before))
	for _, q := range before {
		old[q.No] = q.State
	}
	now := make(map[int]int, len(after))
	for _, q := range after {
		now[q.No] = q.State
	}
	var out []int
	for no, st := range old {
		if now[no] != st {
			out = append(out, no)
		}
	}
	for no := range now {
		if _, ok := old[no]; !ok {
			out = append(out, no)
		}
	}
	sort.Ints(out)
	return out
}

// handleQuests 是查看类请求（docs/protocol.md 5.44）：从内存里的角色数据出任务进度。
func (s *Service) handleQuests(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := l.With(func(r *record) { out = map[string]any{"quests": r.questsView()} }); err != nil {
		return nil, asProtocolError(err)
	}
	return out, nil
}

// SetQuest 是管理命令（stoneage-admin quest 与协议用例的 give_quest 行）：把角色的任务编号设成
// doing（进行中）或 done（已完成），clear 则清除进度。任务编号必须在任务表里。
// 同一个事务里 NOTIFY，运行中的服务端收到后重读并推送；角色不在线时只写库，等下次进入世界时可见。
func SetQuest(ctx context.Context, db *pgxpool.Pool, channel, character string, no int, state string) error {
	t, ok := gamedata.QuestByNo(no)
	if !ok {
		return fmt.Errorf("任务编号 %d 不在任务表里", no)
	}
	var st int
	switch state {
	case "doing":
		st = questDoing
	case "done":
		st = questDone
	case "clear":
		st = 0
	default:
		return fmt.Errorf("状态要是 doing、done 或 clear，不是 %q", state)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, character).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return err
	}
	if st == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM character_quests WHERE character_id = $1 AND quest_no = $2`, charID, no)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO character_quests (character_id, quest_no, state) VALUES ($1, $2, $3)
			ON CONFLICT (character_id, quest_no) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`, charID, no, st)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Default().Info("管理命令改了任务", "character", character, "quest", t.Name, "state", state)
	return nil
}

// reloadQuests 重读角色的任务进度；有变化时推送 quests_changed（全量 quests 加上变化的编号 changed）。
// 没有实际变化（比如重复执行同一条命令）时不推送。角色不在世界中时没有可通知的连接，跳过。
func (s *Service) reloadQuests(ctx context.Context, payload string) {
	charID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.opts.Logger.Warn("任务通知的 payload 不是角色编号", "payload", payload)
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	quests, err := s.loadQuests(ctx, charID)
	if err != nil {
		s.opts.Logger.Warn("重读角色任务失败", "character", charID, "err", err)
		return
	}
	p.mu.Lock()
	if !p.leaving {
		changed := changedQuests(p.rec.Quests, quests)
		p.rec.Quests = quests
		if len(changed) > 0 {
			p.pushAll("quests_changed", map[string]any{"quests": p.rec.questsView(), "changed": changed})
		}
	}
	p.mu.Unlock()
}
