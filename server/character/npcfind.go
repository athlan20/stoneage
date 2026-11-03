package character

// npc_find 请求（STAGE-84，docs/protocol.md 5.43，查看类）：按名字找 NPC，供客户端 `npc 名字` 用。
// 只列角色当前地图上的，以及能传到（有落点、有名字）的地图上的 NPC；排序时当前地图的在前，
// 当前地图内按离角色的距离，其余按地图编号，最后按 NPC 编号。

import (
	"context"
	"sort"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// npcFindLimit 是一次最多返回的条数（客户端只看前几个，同名的很多时不必全给）。
const npcFindLimit = 20

// npcFindItem 是 npc_find 响应里的一项（docs/protocol.md 5.43）。
type npcFindItem struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Floor   int    `json:"floor"`
	MapName string `json:"map_name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

func (s *Service) handleNpcFind(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Name *string `json:"name"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if name == "" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要非空字符串字段 name（NPC 名字）")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var here struct{ floor, x, y int }
	if err := l.With(func(r *record) { here.floor, here.x, here.y = r.Floor, r.X, r.Y }); err != nil {
		return nil, asProtocolError(err)
	}

	items := []npcFindItem{}
	for _, n := range s.npcs.ByName(name) {
		if n.Floor != here.floor && !gamedata.WarpableFloor(n.Floor) {
			continue // 去不了的地图上的 NPC 不列出，免得客户端选中后传不过去
		}
		mapName, _ := gamedata.MapName(n.Floor)
		items = append(items, npcFindItem{ID: n.ID, Name: n.Name, Type: n.Type, Floor: n.Floor, MapName: mapName, X: n.X, Y: n.Y})
	}
	sortNpcFind(items, here.floor, here.x, here.y)
	if len(items) > npcFindLimit {
		items = items[:npcFindLimit]
	}
	return map[string]any{"npcs": items}, nil
}

// sortNpcFind 排序：当前地图在前；当前地图内离角色近的在前；其余按地图编号；最后按 NPC 编号。
func sortNpcFind(items []npcFindItem, floor, x, y int) {
	dist := func(it npcFindItem) int {
		dx, dy := abs(it.X-x), abs(it.Y-y)
		if dx > dy {
			return dx
		}
		return dy
	}
	less := func(a, b npcFindItem) bool {
		aHere, bHere := a.Floor == floor, b.Floor == floor
		if aHere != bHere {
			return aHere
		}
		if aHere && dist(a) != dist(b) {
			return dist(a) < dist(b)
		}
		if a.Floor != b.Floor {
			return a.Floor < b.Floor
		}
		return a.ID < b.ID
	}
	sort.Slice(items, func(i, j int) bool { return less(items[i], items[j]) })
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
