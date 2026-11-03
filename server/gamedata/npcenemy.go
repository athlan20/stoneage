package gamedata

// 战斗 NPC 的参数（STAGE-96，docs/battle-npc.md 5.1）：npc-enemy.json 由 tools/gen-npcs.py（tools/npcenemy.py）
// 从原版服务端 data/npc 的 NPCEnemy 参数文件转换，对照 gmsv/npc/npc_npcenemy.c。这里只提供数据和白名单，
// 确认窗口、占用和开战在 server/character（npcenemy.go）里做。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-enemy.json
var npcEnemyJSON []byte

// NpcEnemy 是一个战斗 NPC 的参数。File 是参数文件相对 data/npc 的路径（不带 .arg），白名单按它判断。
type NpcEnemy struct {
	ID          int        `json:"id"`
	File        string     `json:"file"`
	Ask         []string   `json:"ask"`     // 确认窗口的正文（askbattlemsg1..6，按行）
	Denied      string     `json:"denied"`  // 前置道具不够时的台词
	Already     string     `json:"already"` // 同一个 NPC 正被占用（one_battle）时的台词
	Start       string     `json:"start"`   // 开战时 NPC 说的话
	End         string     `json:"end"`     // 赢了时 NPC 说的话
	Enemies     []int      `json:"enemies"` // 敌人编号（enemy.txt），按顺序入场，最多 10 个
	Items       []int      `json:"items"`   // 开战要全部持有的道具（背包里的）
	Steal       int        `json:"steal"`   // 前置道具什么时候收：-1 不收，0 开战时收，1 赢了才收
	OneBattle   bool       `json:"one_battle"`
	DieAct      int        `json:"die_act"` // 1 赢了传送到 Warp
	Warp        *WarpPoint `json:"warp"`
	ReviveSecs  int        `json:"revive_secs"`
	Unsupported string     `json:"unsupported"` // 非空的运行时不放行（道场、分路传送、消失与复活等）
}

// npcEnemyEnabled 是放开的战斗 NPC（按参数文件）。数据是全量转换的，但只有这里列出的会生效，
// 其余战斗 NPC 对话时回“还不能对话”。第一批只放里昂蛙王（精灵王的传说·前传，地图 30605）。
var npcEnemyEnabled = map[string]bool{
	"king/event69_5": true, // 里昂蛙王
}

// NpcEnemyEnabled 报告参数文件是否放开（生效）。
func NpcEnemyEnabled(file string) bool { return npcEnemyEnabled[file] }

var (
	npcEnemyOnce   sync.Once
	npcEnemyByID   map[int]*NpcEnemy
	npcEnemyByFile map[string]*NpcEnemy
)

func loadNpcEnemies() {
	var list []NpcEnemy
	if err := json.Unmarshal(npcEnemyJSON, &list); err != nil {
		panic("npc-enemy.json 解析失败: " + err.Error())
	}
	npcEnemyByID = make(map[int]*NpcEnemy, len(list))
	npcEnemyByFile = make(map[string]*NpcEnemy, len(list))
	for i := range list {
		npcEnemyByID[list[i].ID] = &list[i]
		npcEnemyByFile[list[i].File] = &list[i]
	}
}

// NpcEnemyOf 按 NPC 编号取战斗参数；不是战斗 NPC 时返回 false。
func NpcEnemyOf(npcID int) (*NpcEnemy, bool) {
	npcEnemyOnce.Do(loadNpcEnemies)
	n, ok := npcEnemyByID[npcID]
	return n, ok
}

// NpcEnemyByFile 按参数文件（如 king/event69_5）取战斗参数；测试剧本用它找到 NPC。
func NpcEnemyByFile(file string) (*NpcEnemy, bool) {
	npcEnemyOnce.Do(loadNpcEnemies)
	n, ok := npcEnemyByFile[file]
	return n, ok
}
