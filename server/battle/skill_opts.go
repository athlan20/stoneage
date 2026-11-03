// 宠物技能参数的读取（STAGE-76）：gamedata/petskill.tsv 的“参数”列沿用原版 petskill.txt 的写法，
// 如 “攻%+90”“倍2 回避30”“回避%+30 反击%+50 会心%+20”。原版 pet_skill.c 用 strstr 找标记、
// 再用 sscanf 读标记后面的数，这里照做：找到标记就读它后面紧跟的数（正负号和小数照读），找不到返回 false。

package battle

import (
	"strconv"
	"strings"
)

// optNumber 读取参数里标记后面紧跟的数（strstr(option, 标记) + sscanf）。
func optNumber(opt, marker string) (float64, bool) {
	i := strings.Index(opt, marker)
	if i < 0 {
		return 0, false
	}
	rest := opt[i+len(marker):]
	end := 0
	for end < len(rest) && strings.IndexByte("+-.0123456789", rest[end]) >= 0 {
		end++
	}
	v, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// leadingInt 读取参数开头的整数（突击参数的蓄力回合数，原版 sscanf(option, "%d", &N)）。
func leadingInt(opt string) (int, bool) {
	fields := strings.Fields(opt)
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return v, true
}

// pctOf 是 base 的 pct% 取整（原版 strdef = (int)(strdef * fPer)，结果直接截断）。
func pctOf(base int, pct float64) int {
	return int(float64(base) * pct / 100)
}
