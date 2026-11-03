package synthesis

import (
	"sort"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-206 第 4 节：配方索引。原版 icache（item_gen.c 的 ITEM_initItemIngCache）按物品编号遍历全部配方，
// 这里改成按原子反查：只看和材料有相同原子的配方，省掉每次扫 9381 个配方。

// recipeAtom 是配方的一个原子：下标与配方数值。
type recipeAtom struct {
	index int
	value int
}

// recipe 是一个能作配方产物的物品及其原子。原子在 ItemIngredients 里的顺序保留，且跳过原子表里没有的名字
// （原版 ADD_ICACHE_INGRED 也是跳过并不计数）。
type recipe struct {
	product int
	atoms   []recipeAtom
}

// recipeIndex 保存配方（按产物编号升序）和原子到配方位置的反查表。
type recipeIndex struct {
	recipes []recipe
	byAtom  map[int][]int
}

// buildRecipeIndex 从加载的数据建索引，没有原子的配方不收。
func buildRecipeIndex() *recipeIndex {
	var rs []recipe
	for _, id := range gamedata.MergeRecipeIDs() {
		r := recipe{product: id}
		for _, g := range gamedata.ItemIngredients(id) {
			ai, ok := gamedata.AtomIndex(g.Atom)
			if !ok {
				continue
			}
			r.atoms = append(r.atoms, recipeAtom{index: ai, value: g.Value})
		}
		if len(r.atoms) > 0 {
			rs = append(rs, r)
		}
	}
	return indexRecipes(rs)
}

// indexRecipes 按给定顺序建索引；调用方要保证 rs 按产物编号升序（候选的顺序靠它）。
func indexRecipes(rs []recipe) *recipeIndex {
	idx := &recipeIndex{recipes: rs, byAtom: make(map[int][]int)}
	for pos, r := range rs {
		for _, a := range r.atoms {
			idx.byAtom[a.index] = append(idx.byAtom[a.index], pos)
		}
	}
	return idx
}

// candidates 返回能用这些材料合成的配方产物，按配方原子数 k（1–5）分组，组内按产物编号升序。
// mats 是随机化后的材料原子（同一原子只有一项）；dish 决定用道具还是料理的数值规则；excluded 是参与材料的物品编号，
// 原版 ITEM_merge_checkitem：产物不能是任何一件参与材料。
func (idx *recipeIndex) candidates(mats []atomValue, dish bool, excluded map[int]bool) [6][]int {
	val := make(map[int]int, len(mats))
	for _, m := range mats {
		val[m.index] = m.value
	}
	seen := make(map[int]bool)
	var pos []int
	for _, m := range mats {
		for _, p := range idx.byAtom[m.index] {
			if !seen[p] {
				seen[p] = true
				pos = append(pos, p)
			}
		}
	}
	sort.Ints(pos)

	var out [6][]int
	for _, p := range pos {
		r := idx.recipes[p]
		if excluded[r.product] || !recipeMatches(r, val, dish) {
			continue
		}
		if k := len(r.atoms); k >= 1 && k <= maxIngredients {
			out[k] = append(out[k], r.product)
		}
	}
	return out
}

// recipeMatches 判断配方的每个原子都能在材料里找到同名原子，且数值在范围内（原版 hitnum == inguse）。
func recipeMatches(r recipe, val map[int]int, dish bool) bool {
	for _, a := range r.atoms {
		v, ok := val[a.index]
		if !ok || !matchValue(v, a.value, dish) {
			return false
		}
	}
	return true
}
