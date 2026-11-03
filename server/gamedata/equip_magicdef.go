// STAGE-226：装备的抗魔加成。原版 ITEM_MagicEquitWear / ITEM_MagicEquitReWear（gmsv/item/item_event.c）
// 穿戴时读道具参数 EA、WA、FI、WI、QU，依次计入 CHAR_EQUITDEFMAGIC_E/WA/F/WI 与 CHAR_EQUITQUIMAGIC。
// 参数格式是 “EA:20|WA:20|FI:20|WI:20|QU:20”（itemset.txt 的使用参数列）。

package gamedata

import (
	"slices"
	"strconv"
	"strings"
)

// MagicDefKeys 是装备抗魔参数的键，顺序同地、水、火、风，最后的 QU 是闪避加成（原版 CHAR_EQUITQUIMAGIC）。
var MagicDefKeys = [5]string{"EA", "WA", "FI", "WI", "QU"}

// MagicDefLimit 是单件装备每一项抗魔加成的绝对值上限。原版是判断“超出就整项不计”，不是截到上限。
const MagicDefLimit = 100

// EquipMagicDef 返回这件装备的抗魔加成，下标同 MagicDefKeys。没有参数、参数不认识或数值超限的项为 0。
func (t ItemTemplate) EquipMagicDef() (out [5]int) {
	if t.Argument == "" {
		return out
	}
	for _, kv := range strings.Split(t.Argument, "|") {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			continue
		}
		i := slices.Index(MagicDefKeys[:], strings.TrimSpace(k))
		if i < 0 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n > MagicDefLimit || n < -MagicDefLimit {
			continue
		}
		out[i] = n
	}
	return out
}
