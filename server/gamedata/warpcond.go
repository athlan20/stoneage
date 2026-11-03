package gamedata

// 传送员的 FREE 条件表达式（npc_warpman.c 的 NPC_ActionPassCheck / NPC_ActionBigSmallCheck）：
// `组1,组2` 候选任一满足即可（OR），组内 `条件&条件` 全要满足（AND）；
// 单个条件是 `KEY<值` / `KEY>值` / `KEY=值` / `KEY!=值`，值后可带 `-上限`（如 `LV=30-40`）。
// 支持的 KEY：LV（等级）、TRANS（转生次数）、ITEM（持有某编号物品，= 和 != 都支持）、ENDEV（事件已完成，= 和 != 都支持）、
// NOWEV（事件进行中，只支持 =：原版 NOWEV!= 恒真，不照搬）。MANOR / FM（庄园、家族）不支持：
// 生成器把它们标记 unsupported，这里解析到也不放行。ENDEV、NOWEV 查的是任务进度（STAGE-90），
// 由调用方通过 WarpState 提供。

import (
	"strconv"
	"strings"
)

// WarpState 是传送条件要看的角色状态。回调为空时，依赖它的条件不通过。
type WarpState struct {
	Level, Transmigration int
	HasItem               func(itemID int) bool // 持有某编号物品（ITEM）
	Done                  func(event int) bool  // 事件已完成（ENDEV）
	Doing                 func(event int) bool  // 事件进行中（NOWEV）
}

// passCheck 判断 FREE 条件原文是否通过。空串和 ALLFREE 恒真。
func passCheck(free string, st WarpState) bool {
	free = strings.TrimSpace(free)
	if free == "" || strings.Contains(free, "ALLFREE") {
		return true
	}
	for _, group := range strings.Split(free, ",") {
		if passGroup(group, st) {
			return true
		}
	}
	return false
}

func passGroup(group string, st WarpState) bool {
	for _, cond := range strings.Split(group, "&") {
		if !passCond(strings.TrimSpace(cond), st) {
			return false
		}
	}
	return true
}

// passCond 判定单个条件；解析不了（不支持的标记、坏的数字）一律 false。
func passCond(cond string, st WarpState) bool {
	// `-上限`：取掉之后是普通的比较，先记下上限。
	lower, upper := cond, -1
	if i := strings.LastIndex(cond, "-"); i > 0 {
		if u, err := strconv.Atoi(cond[i+1:]); err == nil {
			lower, upper = cond[:i], u
		}
	}
	op, key, val := "", "", -1
	for _, o := range []string{"!=", ">=", "<=", ">", "<", "="} {
		if i := strings.Index(lower, o); i > 0 {
			op, key = o, strings.TrimSpace(lower[:i])
			v, err := strconv.Atoi(strings.TrimSpace(lower[i+len(o):]))
			if err != nil {
				return false
			}
			val = v
			break
		}
	}
	if op == "" {
		return false
	}
	if upper >= 0 { // 带上限的条件（如 LV=30-40）：只有 = 语义合理
		if op != "=" {
			return false
		}
		return inRange(key, val, upper, st)
	}
	switch key {
	case "LV":
		return compare(st.Level, op, val)
	case "TRANS":
		return compare(st.Transmigration, op, val)
	case "ITEM": // 原版 NPC_EventItemCheck：= 是持有，!= 是没有
		if st.HasItem == nil || (op != "=" && op != "!=") {
			return false
		}
		return st.HasItem(val) == (op == "=")
	case "ENDEV": // 原版 NPC_ENDEventNoCheck：= 是已完成，!= 是未完成
		if st.Done == nil || (op != "=" && op != "!=") {
			return false
		}
		return st.Done(val) == (op == "=")
	case "NOWEV": // 原版 NPC_NOWEventNoCheck：只有 = 有意义
		if st.Doing == nil || op != "=" {
			return false
		}
		return st.Doing(val)
	}
	return false // MANOR、FM 等：不支持即不通过
}

func inRange(key string, val, upper int, st WarpState) bool {
	switch key {
	case "LV":
		return st.Level >= val && st.Level <= upper
	case "TRANS":
		return st.Transmigration >= val && st.Transmigration <= upper
	}
	return false
}

func compare(have int, op string, want int) bool {
	switch op {
	case ">":
		return have > want
	case "<":
		return have < want
	case ">=":
		return have >= want
	case "<=":
		return have <= want
	case "!=":
		return have != want
	}
	return have == want
}

// feeOf 计算费用（MONEY 原文）：数字、LV*N（等级×N）、-1（不能付费进入）。
func feeOf(money string, level int) int {
	money = strings.TrimSpace(money)
	if money == "" {
		return 0
	}
	if strings.HasPrefix(money, "LV*") {
		n, err := strconv.Atoi(strings.TrimPrefix(money, "LV*"))
		if err != nil || n < 0 {
			return -1
		}
		return level * n
	}
	n, err := strconv.Atoi(money)
	if err != nil {
		return -1
	}
	return n
}
