package gamedata

// 宠物店参数（STAGE-73，docs/protocol.md 5.41）：npcs.tsv 里每家宠物店（npcgen_petshop）
// 的倍率与台词，由 tools/gen-npcs.py 从原版参数文件（genout/ps_*）生成。
// 规则出处是 gmsv/npc/npc_petshop.c：
//   - 卖宠价 = ((等级²×10 − 捕获时等级²×10) + 等级×10) × 稀有系数 × 倍率 × 魅力系数，逐步向下取整
//     （PETCOST=10；稀有 CHAR_RARE 0→1、1→5、2→8，其余原值；special_pet 列表里的模板按
//     special_rate，其余 nomal_rate；魅力系数见 SellCharm）；
//   - 寄放费 = 50 + 等级×4（NPC_GETPOOLCOST 宏；参数 pool_cost 在原版里被注释掉了没有使用）；
//   - 寄放池 15 格（CHAR_MAXPOOLPETHAVE 的扩展值），领取免费；
//   - 台词逐字保留（main、realy、over、thanks、cost、poolthanks、poolfull、getfull）。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-petshop.json
var npcPetshopJSON []byte

// NpcPetshop 是一家宠物店的参数。
type NpcPetshop struct {
	NomalRate   float64 `json:"nomal_rate"`
	SpecialRate float64 `json:"special_rate,omitempty"`
	SpecialPet  []int   `json:"special_pet,omitempty"` // 高价回收的模板编号
	Pool        bool    `json:"pool"`                  // 支持寄放/领取（原版 pool_flg）
	Main        string  `json:"main,omitempty"`
	Really      string  `json:"realy,omitempty"`
	Over        string  `json:"over,omitempty"`
	Thanks      string  `json:"thanks,omitempty"`
	Cost        string  `json:"cost,omitempty"`
	PoolThanks  string  `json:"poolthanks,omitempty"`
	PoolFull    string  `json:"poolfull,omitempty"`
	GetFull     string  `json:"getfull,omitempty"`
}

var (
	petshopOnce sync.Once
	petshopByID map[int]*NpcPetshop
)

// NpcPetshopOf 返回一家宠物店的参数；这个 NPC 不是宠物店时返回 nil。
func NpcPetshopOf(id int) *NpcPetshop {
	petshopOnce.Do(loadNpcPetshop)
	return petshopByID[id]
}

// SellCost 按原版 NPC_GetCost 估算一只宠物的卖店价。getLevel 是捕获时的等级（0 当 1），
// rare 是 CHAR_RARE（不是品质），charm 是 SellCharm 算出的魅力系数（0–50，100 表示不打折）。
func (s *NpcPetshop) SellCost(level, getLevel, rare, templateID, charm int) int {
	if getLevel < 1 {
		getLevel = 1
	}
	rareN := rare
	switch rare {
	case 0:
		rareN = 1
	case 1:
		rareN = 5
	case 2:
		rareN = 8
	}
	cost := ((level*level*10 - getLevel*getLevel*10) + level*10) * rareN
	rate := s.NomalRate
	if s.SpecialRate > 0 && specialContains(s.SpecialPet, templateID) {
		rate = s.SpecialRate
	}
	cost = int(float64(cost) * rate)
	return int(float64(cost) * float64(charm) / 100)
}

// SellCharm 是原版 NPC_GetCost 末尾的魅力系数：（人物魅力 + 宠物忠诚）下限 20，再整除 2，
// 结果当百分比乘进卖价（人物魅力是含装备的有效值，宠物忠诚见 Loyalty）。
func SellCharm(hostCharm, petAI int) int {
	return max(hostCharm+petAI, 20) / 2
}

// PoolCost 是寄放费：50 + 等级×4（原版 NPC_GETPOOLCOST）。
func PoolCost(level int) int { return 50 + level*4 }

// MaxPoolPets 是寄放池的格数（原版 CHAR_MAXPOOLPETHAVE 的扩展值）。
const MaxPoolPets = 15

// PooledSlotBase 是寄放宠物的槽位基数（数据库里 slot = 基数 + 池位，避开随身 0–4）。
const PooledSlotBase = 50

func loadNpcPetshop() {
	petshopByID = map[int]*NpcPetshop{}
	var list []struct {
		ID int `json:"id"`
		NpcPetshop
	}
	if err := json.Unmarshal(npcPetshopJSON, &list); err != nil {
		petshopByID = map[int]*NpcPetshop{}
		return
	}
	for i := range list {
		r := list[i].NpcPetshop
		petshopByID[list[i].ID] = &r
	}
}
