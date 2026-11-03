package gamedata

// NPC 数据（STAGE-68，docs/protocol.md 5.32）：原版 NPC 创建文件里本期会实现的几类
// （对话、告示牌、商店、护士、存档点、宠物店、传送员），由 tools/gen-npcs.py 从
// data/npc 生成。npcgen.c 的生成、对话、开店等玩法在后面的需求里接入；本期只提供
// 按地图查列表（character.Npcs），坐标是出生区域的中心，用来展示和以后做地图用。

import (
	_ "embed"
	"sort"
	"strings"
	"sync"
)

//go:embed npcs.tsv
var npcTSV string

// npcs.tsv 的列数，与 tools/gen-npcs.py 的输出一致。
const npcColumns = 7 // 编号、地图、x、y、原版类型、显示类型、名字

// NPC 是 npcs.tsv 的一行：一个 NPC 的静态资料。序列化成协议 5.32 的字段
// （id、x、y、type、name）；Floor 和 Kind 只在服务端内部用，不进协议。
type NPC struct {
	ID    int    `json:"id"`   // 全表唯一的编号，客户端列表与以后的 NPC 交互命令都用它
	Floor int    `json:"-"`    // 所在地图编号
	X     int    `json:"x"`    // 展示用的格子坐标（出生区域中心）
	Y     int    `json:"y"`    //
	Kind  string `json:"-"`    // 原版类型名，如 npcgen_man（后续玩法按它接入）
	Type  string `json:"type"` // 显示用的类型名，如「对话」
	Name  string `json:"name"` // 名字
}

var (
	npcOnce    sync.Once
	npcByFloor map[int][]NPC
	npcByID    map[int]NPC
)

// NPCsByFloor 返回一张地图上的全部 NPC，按编号升序；地图上没有 NPC 时返回 nil（当空列表用）。
func NPCsByFloor(floor int) []NPC {
	npcOnce.Do(loadNPCs)
	return npcByFloor[floor]
}

// NPCByID 按编号取一个 NPC（对话等交互命令用，docs/protocol.md 5.33）；编号不存在返回 false。
func NPCByID(id int) (NPC, bool) {
	npcOnce.Do(loadNPCs)
	n, ok := npcByID[id]
	return n, ok
}

// NPCsByName 按名字找 NPC（npc_find，docs/protocol.md 5.43，STAGE-84）：有完全同名的就只返回它们，
// 没有时返回名字里包含这个词的。空名字的 NPC 不参与匹配。返回的切片按编号升序。
func NPCsByName(name string) []NPC {
	npcOnce.Do(loadNPCs)
	var exact, contains []NPC
	for _, n := range npcByID {
		switch {
		case n.Name == "":
		case n.Name == name:
			exact = append(exact, n)
		case strings.Contains(n.Name, name):
			contains = append(contains, n)
		}
	}
	found := exact
	if len(found) == 0 {
		found = contains
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })
	return found
}

// NPCsAll 返回全表的 NPC，按编号升序（npc_list 的 NPC 目录用，docs/protocol.md 5.47，STAGE-134）。
// 调用方自己按可达性过滤：目录要区分「全部可达」和「本地图」两类。
func NPCsAll() []NPC {
	npcOnce.Do(loadNPCs)
	all := make([]NPC, 0, len(npcByID))
	for _, n := range npcByID {
		all = append(all, n)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

func loadNPCs() {
	npcByFloor = map[int][]NPC{}
	npcByID = map[int]NPC{}
	eachTSV(npcTSV, npcColumns, func(f []string) {
		n := NPC{
			ID: tsvNum(f[0]), Floor: tsvNum(f[1]),
			X: tsvNum(f[2]), Y: tsvNum(f[3]),
			Kind: f[4], Type: f[5], Name: f[6],
		}
		npcByFloor[n.Floor] = append(npcByFloor[n.Floor], n)
		npcByID[n.ID] = n
	})
}
