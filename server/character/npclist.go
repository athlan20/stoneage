package character

// npc_list 请求（STAGE-134，docs/protocol.md 5.47，查看类）：完整的 NPC 目录。
// 与精确匹配的 npc_find（5.43）分工不同——npc_find 只认名字，用来「找到那个 NPC 并过去」；
// npc_list 给的是能翻的目录：按「全部可达 / 本地图」分类，可按名字搜索，带分页和总数。
// 客户端不能用 npc_find 拼目录（它按名字查、有条数上限，也拿不到总量）。

import (
	"context"
	"sort"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

const (
	// npcListDefaultLimit 是不带 limit 时每页的条数。
	npcListDefaultLimit = 50
	// npcListMaxLimit 是每页的上限，防止一次把整份目录拖下来。
	npcListMaxLimit = 200
)

// npcListItem 是 npc_list 响应里的一项（docs/protocol.md 5.47）。
type npcListItem struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Floor   int    `json:"floor"`
	MapName string `json:"map_name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	// Here 表示这个 NPC 就在角色当前地图上（客户端列表里标「本图」）。
	Here bool `json:"here"`
}

// handleNpcList 处理 npc_list：分类（scope）、名字搜索（name）、分页（offset/limit）。
func (s *Service) handleNpcList(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Scope  *string `json:"scope"`
		Name   *string `json:"name"`
		Offset *int    `json:"offset"`
		Limit  *int    `json:"limit"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	scope := "reachable"
	if in.Scope != nil {
		scope = strings.TrimSpace(*in.Scope)
	}
	if scope != "reachable" && scope != "here" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "scope 只能是 reachable（全部可达）或 here（本地图）")
	}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	offset := 0
	if in.Offset != nil {
		if *in.Offset < 0 {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "offset 不能是负数")
		}
		offset = *in.Offset
	}
	limit := npcListDefaultLimit
	if in.Limit != nil {
		if *in.Limit < 1 {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "limit 要是正整数")
		}
		limit = *in.Limit
		if limit > npcListMaxLimit {
			limit = npcListMaxLimit
		}
	}

	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var here struct{ floor, x, y int }
	if err := l.With(func(r *record) { here.floor, here.x, here.y = r.Floor, r.X, r.Y }); err != nil {
		return nil, asProtocolError(err)
	}

	items := make([]npcListItem, 0, len(gamedata.NPCsAll()))
	for _, n := range gamedata.NPCsAll() {
		if n.Name == "" {
			continue
		}
		// 分类：here 只看当前地图；reachable 还要排出「去不了的地图」上的 NPC。
		if scope == "here" && n.Floor != here.floor {
			continue
		}
		if scope == "reachable" && n.Floor != here.floor && !gamedata.WarpableFloor(n.Floor) {
			continue
		}
		if name != "" && !strings.Contains(n.Name, name) {
			continue
		}
		mapName, _ := gamedata.MapName(n.Floor)
		items = append(items, npcListItem{
			ID: n.ID, Name: n.Name, Type: n.Type,
			Floor: n.Floor, MapName: mapName, X: n.X, Y: n.Y,
			Here: n.Floor == here.floor,
		})
	}
	sortNpcList(items, here.floor, here.x, here.y)

	total := len(items)
	if offset >= total {
		items = []npcListItem{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		items = items[offset:end]
	}
	return map[string]any{
		"npcs":   items,
		"total":  total,
		"offset": offset,
		"limit":  limit,
		"scope":  scope,
	}, nil
}

// sortNpcList 排序：当前地图的在前；当前地图内离角色近的在前；其余按地图编号，最后按 NPC 编号。
// 和 npc_find 的排序一致，这样两处看到的相对顺序相同。
func sortNpcList(items []npcListItem, floor, x, y int) {
	dist := func(it npcListItem) int {
		dx, dy := abs(it.X-x), abs(it.Y-y)
		if dx > dy {
			return dx
		}
		return dy
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Here != b.Here {
			return a.Here
		}
		if a.Here {
			if da, db := dist(a), dist(b); da != db {
				return da < db
			}
		} else if a.Floor != b.Floor {
			return a.Floor < b.Floor
		}
		return a.ID < b.ID
	})
}
