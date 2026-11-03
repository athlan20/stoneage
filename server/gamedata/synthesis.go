package gamedata

import "sort"

// 合成的物品数据（STAGE-205，设计见 docs/synthesis.md 第 2 节）：材料与配方的标记在 ItemTemplate 上
// （CanMergeFrom、CanMergeTo），原子在这里按物品编号取。数据来自 tools/gen-itemset.py 的第 82–93 列。

// ItemTypeDish 是料理（原版 ITEM_DISH）。合成时第一件材料是料理，走食物的配方规则（item_gen.c 的 ITEM_mergeItem）。
const ItemTypeDish = 20

// Ingredient 是物品的一个原子（原版 ITEM_INGNAME / ITEM_INGVALUE）：原子名与数值。
type Ingredient struct {
	Atom  string
	Value int
}

// itemIngredients 按物品编号存合成原子，在 loadItems 里填。没有原子的物品不在表里。
var itemIngredients map[int][]Ingredient

// ItemIngredients 返回物品的合成原子（最多 5 个，按列序）；没有的返回 nil。
func ItemIngredients(id int) []Ingredient {
	itemOnce.Do(loadItems)
	return itemIngredients[id]
}

// parseIngredients 解析 5 组“原子名、数值”，名字为空的组跳过（原版 ITEM_INGNAME 为空即没有这个原子）。
func parseIngredients(f []string) []Ingredient {
	var out []Ingredient
	for i := 0; i+1 < len(f); i += 2 {
		if f[i] == "" {
			continue
		}
		out = append(out, Ingredient{Atom: f[i], Value: atoi(f[i+1])})
	}
	return out
}

// MergeRecipeIDs 返回能作配方产物（CanMergeTo）的物品编号，升序。合成引擎建配方索引用（STAGE-206）。
func MergeRecipeIDs() []int {
	itemOnce.Do(loadItems)
	var ids []int
	for id, it := range itemTable {
		if it.CanMergeTo {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}
