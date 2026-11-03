package character

import (
	"context"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
	"sort"
)

type questNPC struct {
	npcListItem
	Relation string `json:"relation"`
}

// 从实际脚本的事件引用生成关系，不根据攻略文字、NPC 名字或任意默认值猜目标。
func questBlockMentions(b gamedata.EventBlock, q gamedata.Quest) bool {
	matches := func(n int) bool { return n > 0 && (n == q.NowEvent || n == q.EndEvent) }
	if matches(b.Event) {
		return true
	}
	for _, n := range b.EndSet {
		if matches(n) {
			return true
		}
	}
	for _, n := range b.Clean {
		if matches(n) {
			return true
		}
	}
	for _, or := range b.Cond {
		for _, t := range or {
			if (t.Op == "NOWEV" || t.Op == "ENDEV") && matches(t.N) {
				return true
			}
		}
	}
	return false
}

func (s *Service) questNPCCandidates(r *record, q gamedata.Quest) []questNPC {
	out := []questNPC{}
	for _, base := range gamedata.NPCsAll() {
		script, ok := gamedata.NpcEventOf(base.ID)
		if !ok || !s.npcEventOpen(script.File) {
			continue
		}
		related := false
		for _, b := range script.Blocks {
			if b.Unsupported == "" && questBlockMentions(b, q) {
				related = true
				break
			}
		}
		if !related {
			continue
		}
		n, ok := s.npcs.ByID(base.ID)
		if !ok {
			continue
		}
		if n.Floor != r.Floor && !gamedata.WarpableFloor(n.Floor) {
			continue
		}
		relation := "reference"
		idx, refuse := s.pickEventBlock(r, script, "")
		if !refuse && idx >= 0 && questBlockMentions(script.Blocks[idx], q) {
			relation = "current"
		}
		name, _ := gamedata.MapName(n.Floor)
		out = append(out, questNPC{npcListItem: npcListItem{ID: n.ID, Name: n.Name, Type: n.Type, Floor: n.Floor, MapName: name, X: n.X, Y: n.Y, Here: n.Floor == r.Floor}, Relation: relation})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Relation != b.Relation {
			return a.Relation == "current"
		}
		if a.Here != b.Here {
			return a.Here
		}
		if a.Floor != b.Floor {
			return a.Floor < b.Floor
		}
		return a.ID < b.ID
	})
	return out
}

func (s *Service) handleQuestNPCs(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		No int `json:"no"`
	}
	if e := req.Decode(&in); e != nil {
		return nil, e
	}
	q, ok := gamedata.QuestByNo(in.No)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "没有这个任务")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = l.With(func(r *record) {
		state := ""
		for _, p := range r.Quests {
			if p.No == q.No {
				state = questStateName(p.State)
				break
			}
		}
		npcs := s.questNPCCandidates(r, q)
		reason := "参考 NPC 不保证是下一步；对话时以当前任务条件为准"
		if len(npcs) == 0 {
			reason = "当前开放脚本没有可验证的 NPC 关系，请使用 NPC 搜索"
		}
		out = map[string]any{"no": q.No, "state": state, "npcs": npcs, "reason": reason}
	})
	if err != nil {
		return nil, asProtocolError(err)
	}
	return out, nil
}
