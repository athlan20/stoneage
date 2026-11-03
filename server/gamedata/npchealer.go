package gamedata

// 护士治疗参数（STAGE-71，docs/protocol.md 5.39）：npcs.tsv 里每位护士（npcgen_winhealer）
// 的治疗参数，由 tools/gen-npcs.py 从原版创建块的 enemy 参数（level|hp_rate|mp_rate|range）
// 生成。规则出处是 gmsv/npc/npc_windowhealer.c：
//   - 费用 = 角色等级 × 倍率（HP、MP 各算各的，向下取整，至少 1；只有不满的那项收费）；
//   - 角色等级低于 level（免费线）时免费（原版「现在的等级可以免费帮你回复唷！」）；
//   - 治疗把人物 HP、MP 回满，并且回满背包里全部宠物的 HP（原版 NPC_WindowHealerAllHeal）。
// 原版参数的 range 是走近自动弹窗的距离，新服务端不自动弹窗，不收录。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-healer.json
var npcHealerJSON []byte

// NpcHealer 是一位护士的治疗参数。
type NpcHealer struct {
	Level  int     `json:"level"`   // 免费线：角色等级低于它时不收钱
	HPRate float64 `json:"hp_rate"` // 耐久力费用 = 等级 × hp_rate
	MPRate float64 `json:"mp_rate"` // 气力费用 = 等级 × mp_rate
}

var (
	healerOnce sync.Once
	healerByID map[int]*NpcHealer
)

// NpcHealerOf 返回一位护士的参数；这个 NPC 不是护士时返回 nil。
func NpcHealerOf(id int) *NpcHealer {
	healerOnce.Do(loadNpcHealer)
	return healerByID[id]
}

// HPCost 是把耐久力补满的费用（free 表示角色在免费线以下）。
func (h *NpcHealer) HPCost(lv int) (cost int, free bool) {
	return healerCost(lv, h.HPRate), lv < h.Level
}

// MPCost 是把气力补满的费用。
func (h *NpcHealer) MPCost(lv int) (cost int, free bool) {
	return healerCost(lv, h.MPRate), lv < h.Level
}

// healerCost 是单次费用：等级 × 倍率向下取整，至少 1（原版 NPC_WindowCostCheck）。
func healerCost(lv int, rate float64) int {
	cost := int(float64(lv) * rate)
	if cost < 1 {
		return 1
	}
	return cost
}

func loadNpcHealer() {
	healerByID = map[int]*NpcHealer{}
	var list []struct {
		ID int `json:"id"`
		NpcHealer
	}
	if err := json.Unmarshal(npcHealerJSON, &list); err != nil {
		healerByID = map[int]*NpcHealer{}
		return
	}
	for i := range list {
		r := list[i].NpcHealer
		healerByID[list[i].ID] = &r
	}
}
