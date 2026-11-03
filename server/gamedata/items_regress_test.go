package gamedata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// itemsDumpSHA 是改动前（STAGE-297 之前）全部物品模板的旧字段输出的 sha256：只要 itemset.tsv 原有列的数据、
// 或者解析旧列的代码变了，这里就会红。新增的精工、伤吸、HP、MP 不在这个输出里（它们有单独的测试）。
// 数据确实要改时，按 TestItemTemplatesUnchanged 的失败信息重新登记这个值。
const itemsDumpSHA = "4a24472e1734717714d256c8dff803d3a5251507f5fcd3158baacf3af0b56330"

// TestItemTemplatesUnchanged 是 STAGE-297 的回归：加列、加字段以后，现有物品的模板输出逐字节不变。
func TestItemTemplatesUnchanged(t *testing.T) {
	ItemByID(0)
	ids := make([]int, 0, len(itemTable))
	for id := range itemTable {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var sb strings.Builder
	for _, id := range ids {
		it := itemTable[id]
		fmt.Fprintf(&sb, "%d|%q|%q|%d|%d|%d|%d|%d|%d|%d|%d|%d|%q|%q|%d|%d|%d|%d|%t|%t|%d|%d|%+v\n", id, it.Name, it.Effect, it.Type, it.Cost, it.Level, it.Attack, it.Defence, it.Agility, it.Luck, it.Charm, it.Critical, it.UseFunc, it.Argument, it.AbleUse, it.Target, it.MagicID, it.MagicMP, it.CanMergeFrom, it.CanMergeTo, it.Durability, it.MaxDurability, itemIngredients[id])
	}
	sum := sha256.Sum256([]byte(sb.String()))
	if got := hex.EncodeToString(sum[:]); got != itemsDumpSHA {
		t.Errorf("物品模板的旧字段输出变了：sha256 = %s，登记的是 %s", got, itemsDumpSHA)
	}
}
