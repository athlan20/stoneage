// 异常攻击技能（PETSKILL_StatusChange）的参数解析（STAGE-75）。参数格式见 petskill.txt
// 第 4 列，如“毒 turn 3  攻%-30”；对照 gmsv/battle/pet_skill.c 的 PETSKILL_StatusChange。

package gamedata

import (
	"fmt"
	"strings"
)

// statusChangeDefaultTurn 是参数里没写 turn 时的回合数（原版 PETSKILL_StatusChange 的 turn = 3）。
const statusChangeDefaultTurn = 3

// StatusChangeArg 是异常攻击的参数。
type StatusChangeArg struct {
	Status string  // 协议状态名：poison / stone / confusion / drunk / sleep
	Turn   int     // turn N：回合数（原版存进 gBattleStausTurn，上状态时再 +1）
	AtkPct float64 // 攻%N：攻击力的增减百分比（-30 即攻击力打七成）
}

// ParseStatusChange 解析异常攻击的参数；关键字不是异常状态（如“全”）时返回假。
func (s PetSkill) ParseStatusChange() (StatusChangeArg, bool) {
	arg := strings.TrimSpace(s.Arg)
	var out StatusChangeArg
	for key, name := range statusKeys {
		if name != "all" && strings.HasPrefix(arg, key) {
			out.Status = name
		}
	}
	if out.Status == "" {
		return StatusChangeArg{}, false
	}
	out.Turn = statusChangeDefaultTurn
	if i := strings.Index(arg, "turn"); i >= 0 {
		var n int
		if _, err := fmt.Sscanf(arg[i+len("turn"):], "%d", &n); err == nil {
			out.Turn = n
		}
	}
	if i := strings.Index(arg, "攻%"); i >= 0 {
		_, _ = fmt.Sscanf(arg[i+len("攻%"):], "%f", &out.AtkPct)
	}
	return out, true
}
