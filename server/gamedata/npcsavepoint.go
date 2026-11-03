package gamedata

// 存档点（STAGE-72，docs/protocol.md 5.40）：npcs.tsv 里每个存档点 NPC（npcgen_savepoint）
// 的落点和记录要交的物品，由 tools/gen-npcs.py 从原版参数文件（genout/sp_*）生成。
// 规则出处是 gmsv/npc/npc_savepoint.c：
//   - Born 是战败送回的落点（floor,x,y）；
//   - GetItem（items）是记录要交的物品：外层是候选，任一组满足即可（原版 NPC_UsedCheck 的 OR）；
//     内层是一组 AND 物品（`&` 连接，全部要有，收物品时一起收，原版 NPC_AndReduce）；
//     没有 GetItem（items 为空）时不要东西，直接记录；
//   - 台词逐字保留：Request（要物品）、Ok（记录成功）、Really（收走物品）、Normal（已记录过）。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-savepoint.json
var npcSavepointJSON []byte

// NpcSavepoint 是一个存档点。
type NpcSavepoint struct {
	Born    []int             `json:"born"`             // 战败送回的落点 [floor, x, y]
	Groups  [][]SavepointItem `json:"groups,omitempty"` // 记录要交的物品：候选（任一组即可），每组内全部要有
	Request string            `json:"request,omitempty"`
	Ok      string            `json:"ok,omitempty"`
	Really  string            `json:"really,omitempty"`
	Normal  string            `json:"normal,omitempty"`
}

// SavepointItem 是一组里的一件物品。
type SavepointItem struct {
	Item  int `json:"item"`
	Count int `json:"count"`
}

var (
	spvOnce sync.Once
	spvByID map[int]*NpcSavepoint
)

// NpcSavepointOf 返回一个存档点的参数；这个 NPC 不是存档点时返回 nil。
func NpcSavepointOf(id int) *NpcSavepoint {
	spvOnce.Do(loadNpcSavepoint)
	return spvByID[id]
}

// BornAt 返回落点；数据不全时返回 false。
func (s *NpcSavepoint) BornAt() (floor, x, y int, ok bool) {
	if len(s.Born) != 3 {
		return 0, 0, 0, false
	}
	return s.Born[0], s.Born[1], s.Born[2], true
}

func loadNpcSavepoint() {
	spvByID = map[int]*NpcSavepoint{}
	var list []struct {
		ID int `json:"id"`
		NpcSavepoint
	}
	if err := json.Unmarshal(npcSavepointJSON, &list); err != nil {
		spvByID = map[int]*NpcSavepoint{}
		return
	}
	for i := range list {
		r := list[i].NpcSavepoint
		spvByID[list[i].ID] = &r
	}
}
