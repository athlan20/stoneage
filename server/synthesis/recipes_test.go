package synthesis

import (
	"sort"
	"testing"
)

// STAGE-206 第 4 节：配方候选（按原子反查 + hitnum 规则）。合成数据用手写的小配方，原子下标随便取。

func TestCandidatesByAtomCount(t *testing.T) {
	idx := indexRecipes([]recipe{
		{product: 10, atoms: []recipeAtom{{1, 100}}},
		{product: 20, atoms: []recipeAtom{{1, 100}, {2, 100}}},
		{product: 30, atoms: []recipeAtom{{1, 100}, {2, 100}, {3, 100}}},
	})
	mats := []atomValue{{index: 1, value: 100}, {index: 2, value: 100}}
	got := idx.candidates(mats, false, nil)
	if len(got[1]) != 1 || got[1][0] != 10 {
		t.Errorf("k=1 应只有 10，实际 %v", got[1])
	}
	if len(got[2]) != 1 || got[2][0] != 20 {
		t.Errorf("k=2 应只有 20，实际 %v", got[2])
	}
	if len(got[3]) != 0 {
		t.Errorf("材料里没有原子 3，k=3 的 30 不应命中，实际 %v", got[3])
	}
}

func TestCandidatesRequireEveryRecipeAtom(t *testing.T) {
	idx := indexRecipes([]recipe{{product: 10, atoms: []recipeAtom{{1, 100}, {2, 100}}}})
	got := idx.candidates([]atomValue{{index: 1, value: 100}}, false, nil)
	if len(got[1])+len(got[2]) != 0 {
		t.Errorf("配方要两个原子都对上，只有一个时不应命中，实际 %v", got)
	}
}

func TestCandidatesExcludeMaterialProduct(t *testing.T) {
	idx := indexRecipes([]recipe{{product: 10, atoms: []recipeAtom{{1, 100}}}})
	got := idx.candidates([]atomValue{{index: 1, value: 100}}, false, map[int]bool{10: true})
	if len(got[1]) != 0 {
		t.Errorf("产物是参与材料的物品时应排除，实际 %v", got[1])
	}
}

func TestCandidatesRejectOutOfRangeValue(t *testing.T) {
	// 材料 v=100 的上界是 164，配方 170 不合格。
	idx := indexRecipes([]recipe{{product: 10, atoms: []recipeAtom{{1, 170}}}})
	got := idx.candidates([]atomValue{{index: 1, value: 100}}, false, nil)
	if len(got[1]) != 0 {
		t.Errorf("配方数值超出范围时不应命中，实际 %v", got[1])
	}
}

// 同一组里按产物编号升序（原版按 icache 下标遍历，随机选择的顺序依赖它）。
func TestCandidatesOrderedByProduct(t *testing.T) {
	idx := indexRecipes([]recipe{
		{product: 200, atoms: []recipeAtom{{1, 100}}},
		{product: 300, atoms: []recipeAtom{{1, 90}}},
	})
	got := idx.candidates([]atomValue{{index: 1, value: 100}}, false, nil)
	if len(got[1]) != 2 || got[1][0] != 200 || got[1][1] != 300 {
		t.Errorf("候选应按产物编号升序 [200 300]，实际 %v", got[1])
	}
}

// 全部配方的索引：9381 个能作配方的物品都有原子，列表按产物编号升序。
func TestBuildRecipeIndexFromData(t *testing.T) {
	idx := buildRecipeIndex()
	if len(idx.recipes) != 9381 {
		t.Fatalf("配方索引应有 9381 个，实际 %d", len(idx.recipes))
	}
	if !sort.SliceIsSorted(idx.recipes, func(i, j int) bool { return idx.recipes[i].product < idx.recipes[j].product }) {
		t.Errorf("配方应按产物编号升序")
	}
	for _, r := range idx.recipes {
		if len(r.atoms) < 1 || len(r.atoms) > 5 {
			t.Fatalf("产物 %d 的原子数 %d 不在 1–5 之间", r.product, len(r.atoms))
		}
	}
}
