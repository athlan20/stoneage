package character

// 任务事件的进度（STAGE-90，对照 gmsv/npc/npcutil.c 的 NPC_*Flg）。原版用事件号的位图记完成（ENDEV）和
// 进行中（NOWEV）。这里分两处存，状态规则一样（一个号只有一种状态）：
//   - 事件号对应任务表（gamedata.Quests）里的任务：进度按任务编号存在 character_quests（STAGE-86）。
//     一个事件号可能对应几个任务，原版也是一起完成的，这里同样一起改。任务表里 EndEvent 和 NowEvent 总是相同
//     （tools/gen-quests.py），所以“完成”和“进行中”是同一个任务的两种状态。
//   - 不对应任务表的事件号（脚本里只有事件号的中间步骤，如造斧的 19 号）：按事件号存在 character_event_flags（STAGE-94）。
// 脚本只看事件号，两处的差别由 progress 屏蔽。

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
)

// eventEdit 是对一个事件号的一步改动。
type eventEdit int

const (
	editSetDoing eventEdit = iota // NPC_NowEventSetFlg：设进行中（已完成的不再改成进行中）
	editClrDoing                  // NPC_NowEventSetFlgCls：清进行中
	editSetDone                   // NPC_EventSetFlg：设完成
	editClrAll                    // NPC_NowEndEventSetFlgCls（CleanFlg）：清完成和进行中
)

// progress 是一趟脚本看到的进度（副本，可以直接改）：quests 按任务编号，events 按不对应任务的事件号。
type progress struct {
	quests map[int]int
	events map[int]int
}

// progressOf 把角色的进度转成副本（交易在副本上算，检查通过才写回，见 commitEvent）。
func progressOf(r *record) progress {
	return progress{quests: stateMap(r.Quests), events: stateMap(r.EventFlags)}
}

// stateMap 把状态表（character_quests 或 character_event_flags 的行）转成 号 → 状态（副本，可以直接改）。
func stateMap(list []questRec) map[int]int {
	out := make(map[int]int, len(list))
	for _, q := range list {
		out[q.No] = q.State
	}
	return out
}

// stateRecs 把状态表转回按编号升序的列表（和 loadStates 的顺序一致）。
func stateRecs(states map[int]int) []questRec {
	out := make([]questRec, 0, len(states))
	for no, st := range states {
		out = append(out, questRec{No: no, State: st})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].No < out[j].No })
	return out
}

var (
	questEventOnce sync.Once
	questEvents    map[int]bool
)

// hasQuest 报告事件号 ev 是否对应任务表里的任务（完成或进行中）。对应的进度在 character_quests，否则在
// character_event_flags。
func hasQuest(ev int) bool {
	questEventOnce.Do(func() {
		questEvents = make(map[int]bool)
		for _, q := range gamedata.Quests() {
			questEvents[q.EndEvent] = true
			questEvents[q.NowEvent] = true
		}
	})
	return questEvents[ev]
}

// editState 对一个状态表里的一个号做一步改动。任务和不对应任务的事件共用这一套规则。
func editState(states map[int]int, kind eventEdit, n int) {
	switch kind {
	case editSetDoing:
		if states[n] != questDone {
			states[n] = questDoing
		}
	case editClrDoing:
		if states[n] == questDoing {
			delete(states, n)
		}
	case editSetDone:
		states[n] = questDone
	case editClrAll:
		delete(states, n)
	}
}

// applyEventEdit 在进度上执行一步改动。事件号 -1 表示没有事件，什么都不做（原版 shiftbit == -1）。
func applyEventEdit(p progress, kind eventEdit, ev int) {
	if ev == -1 {
		return
	}
	if !hasQuest(ev) {
		editState(p.events, kind, ev)
		return
	}
	for _, q := range gamedata.Quests() {
		if q.EndEvent == ev || q.NowEvent == ev {
			editState(p.quests, kind, q.No)
		}
	}
}

// endSetFlg 是脚本的 EndSetFlg：事件 ev 清进行中（ev 为 -1 时跳过），列表里的号设完成。
// 只有块里写了 EndSetFlg 才执行（原版把“清进行中”也放在这个分支里），所以没有字段时什么都不做。
func endSetFlg(p progress, ev int, list []int) {
	if len(list) == 0 {
		return
	}
	if ev != -1 {
		applyEventEdit(p, editClrDoing, ev)
	}
	for _, n := range list {
		applyEventEdit(p, editSetDone, n)
	}
}

// keepDoing 是 STAGE-341：ACCEPT 的事件号不在 EndSetFlg 列表里时（全库只有 poru/mary04、mary05、mary06 三块，
// 如 EventNo:55 配 EndSetFlg:66），脚本意图是“交付这一步完成别的事件，同时让事件号进入进行中”，后面的
// 交付块和闲聊块都要求 NOWEV=事件号。原版 NPC_NowEventSetFlg/Cls 的函数体不在源码树里，无法证实，
// 按脚本意图补设进行中；事件号已完成的不会被改回进行中（editSetDoing 的约定）。
func keepDoing(p progress, blk *gamedata.EventBlock) {
	if blk.Event < 0 || len(blk.EndSet) == 0 || slices.Contains(blk.EndSet, blk.Event) {
		return
	}
	applyEventEdit(p, editSetDoing, blk.Event)
}

// cleanFlg 是脚本的 CleanFlg：列表里的号清完成和进行中（可以重做）。
func cleanFlg(p progress, list []int) {
	for _, n := range list {
		applyEventEdit(p, editClrAll, n)
	}
}

// eventIs 报告事件号 ev 是否处于给定状态。事件号 -1 没有对应的进度，返回 false（原版 NPC_EventCheckFlg /
// NPC_NowEventCheckFlg 的 -1 分支）。
func (r *record) eventIs(ev, want int) bool {
	if ev == -1 {
		return false
	}
	if !hasQuest(ev) {
		for _, f := range r.EventFlags {
			if f.No == ev && f.State == want {
				return true
			}
		}
		return false
	}
	for _, q := range r.Quests {
		if q.State != want {
			continue
		}
		t, ok := gamedata.QuestByNo(q.No)
		if !ok {
			continue
		}
		if want == questDone && t.EndEvent == ev || want == questDoing && t.NowEvent == ev {
			return true
		}
	}
	return false
}

// eventDone 是原版 NPC_EventCheckFlg：事件 ev 已完成（完成的块会被跳过）。
func (r *record) eventDone(ev int) bool { return r.eventIs(ev, questDone) }

// eventDoing 是原版 NPC_NowEventCheckFlg：事件 ev 进行中。
func (r *record) eventDoing(ev int) bool { return r.eventIs(ev, questDoing) }

// stateTable 是记进度的两张表：任务进度（character_quests，键 quest_no）和事件进度（character_event_flags，键 event_no）。
type stateTable struct {
	name string // 表名
	key  string // 键列名
}

var (
	questTable = stateTable{name: "character_quests", key: "quest_no"}
	eventTable = stateTable{name: "character_event_flags", key: "event_no"}
)

// loadStates 读角色在某张表里的进度，按编号从小到大。
func (s *Service) loadStates(ctx context.Context, t stateTable, charID int64) ([]questRec, error) {
	rows, err := s.db.Query(ctx, fmt.Sprintf(`SELECT %s, state FROM %s WHERE character_id = $1 ORDER BY %s`, t.key, t.name, t.key), charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []questRec
	for rows.Next() {
		var q questRec
		if err := rows.Scan(&q.No, &q.State); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// writeStateDiff 把进度的变化写进一张表（和物品在同一个事务里提交）：状态为 0 的删行，其余插入或更新。
func writeStateDiff(ctx context.Context, tx pgx.Tx, t stateTable, charID int64, before, after map[int]int) error {
	seen := make(map[int]bool, len(before)+len(after))
	for no := range before {
		seen[no] = true
	}
	for no := range after {
		seen[no] = true
	}
	del := fmt.Sprintf(`DELETE FROM %s WHERE character_id = $1 AND %s = $2`, t.name, t.key)
	upsert := fmt.Sprintf(`INSERT INTO %s (character_id, %s, state) VALUES ($1, $2, $3)
		ON CONFLICT (character_id, %s) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`, t.name, t.key, t.key)
	for no := range seen {
		old, now := before[no], after[no]
		if old == now {
			continue
		}
		var err error
		if now == 0 {
			_, err = tx.Exec(ctx, del, charID, no)
		} else {
			_, err = tx.Exec(ctx, upsert, charID, no, now)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
