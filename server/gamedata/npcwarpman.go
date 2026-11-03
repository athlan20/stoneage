package gamedata

// 传送员参数（STAGE-74，docs/protocol.md 5.42）：npcs.tsv 里每名传送员（npcgen_warpman）
// 的落点、通行条件和台词，由 tools/gen-npcs.py 从原版参数文件（genout/wpm_* 等）生成。
// 规则出处是 gmsv/npc/npc_warpman.c（NEWWARPMAN）：
//   - FREE 是通行条件：逗号分隔的候选组任一满足即可（原版 NPC_ActionPassCheck 的 OR），
//     组内 & 连接（AND）；`ALLFREE` 恒真。条件标记只实现了 LV / TRANS / ITEM
//     （LV>30、LV=30-40 这类范围用 - 后缀；ITEM=编号 持有即可）；ENDEV / NOWEV（任务
//     事件）、MANOR（庄园）、FM（家族）的数据 unsupported=true，运行时跳过、不放行。
//   - MONEY 是免费条件不满足时的费用：数字（固定）、`LV*N`（等级×N）、`-1`（不能付费进入）。
//   - DelItem 是传送成功后收走的门票编号（角色持有时收走第一件）。
//   - PartyMsg 是组队提示：一期没有组队，单人直接通过，文案保留。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-warpman.json
var npcWarpmanJSON []byte

// NpcWarpman 是一名传送员。
type NpcWarpman struct {
	Warp       []int   `json:"warp"`              // 落点 [floor, x, y]
	Free       string  `json:"free,omitempty"`    // 通行条件原文（空 = 直接可传）
	Money      string  `json:"money,omitempty"`   // 费用：数字 / LV*N / -1
	DelItem    []int   `json:"del_item,omitempty"`
	FreeMsg    string  `json:"free_msg,omitempty"`
	PayMsg     string  `json:"pay_msg,omitempty"`
	MoneyMsg   string  `json:"money_msg,omitempty"`
	PartyMsg   string  `json:"party_msg,omitempty"`
	Nomal      string  `json:"nomal,omitempty"`
	Unsupported bool   `json:"unsupported"` // 依赖任务/庄园/家族的条件，本期跳过
}

var (
	warpOnce  sync.Once
	warpByID  map[int]*NpcWarpman
)

// NpcWarpmanOf 返回一名传送员的参数；这个 NPC 不是传送员时返回 nil。
func NpcWarpmanOf(id int) *NpcWarpman {
	warpOnce.Do(loadNpcWarpman)
	return warpByID[id]
}

// BornAt 返回落点。
func (w *NpcWarpman) BornAt() (floor, x, y int, ok bool) {
	if len(w.Warp) != 3 {
		return 0, 0, 0, false
	}
	return w.Warp[0], w.Warp[1], w.Warp[2], true
}

// PassFree 按原版 NPC_ActionPassCheck 判定角色是否满足通行条件（候选组任一满足即可）。
// st 是角色的等级、转生、道具和任务进度（ITEM、ENDEV、NOWEV 用）。
func (w *NpcWarpman) PassFree(st WarpState) bool {
	return passCheck(w.Free, st)
}

// Fee 计算免费条件不满足时的费用：固定数字、LV*N（等级×N）、-1（不能付费进入）。
// 原文为空时费用为 0（没有 PayMsg 的传送员只能免费进入，进不了就不响应）。
func (w *NpcWarpman) Fee(level int) int {
	return feeOf(w.Money, level)
}

func loadNpcWarpman() {
	warpByID = map[int]*NpcWarpman{}
	var list []struct {
		ID int `json:"id"`
		NpcWarpman
	}
	if err := json.Unmarshal(npcWarpmanJSON, &list); err != nil {
		warpByID = map[int]*NpcWarpman{}
		return
	}
	for i := range list {
		r := list[i].NpcWarpman
		warpByID[list[i].ID] = &r
	}
}
