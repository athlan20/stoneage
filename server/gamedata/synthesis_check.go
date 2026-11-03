package gamedata

import (
	"fmt"
	"sort"
)

// 合成数据的校验（STAGE-205，设计见 docs/synthesis.md 第 8 节）。只报告，不 panic：测试断言返回空。
// 检查项：物品配方的原子都在原子表里（原版 item_gen.c 的 “fucking atom”）；宠物固定原子的名字与 Known 标记一致；
// 配方的原子数在 1–5 之间；能作材料、能作配方的物品必须有原子。

// ValidateSynthesisData 返回所有问题的描述，没有问题返回 nil。
func ValidateSynthesisData() []string {
	itemOnce.Do(loadItems)
	var problems []string

	ids := make([]int, 0, len(itemTable))
	for id := range itemTable {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		it := itemTable[id]
		ings := itemIngredients[id]
		for _, g := range ings {
			if _, ok := AtomIndex(g.Atom); !ok {
				problems = append(problems, fmt.Sprintf("物品 %d %s：原子 %s 不在原子表里", id, it.Name, g.Atom))
			}
		}
		if (it.CanMergeFrom || it.CanMergeTo) && len(ings) == 0 {
			problems = append(problems, fmt.Sprintf("物品 %d %s：能作材料或配方，但没有原子", id, it.Name))
		}
		if it.CanMergeTo && (len(ings) < 1 || len(ings) > 5) {
			problems = append(problems, fmt.Sprintf("物品 %d %s：配方有 %d 个原子，应为 1–5 个", id, it.Name, len(ings)))
		}
	}

	for _, id := range petAtomTemplateIDs() {
		for _, a := range PetAtoms(id) {
			_, inTable := AtomIndex(a.Atom)
			if a.Known != inTable {
				problems = append(problems, fmt.Sprintf("宠物模板 %d：固定原子 %s 的 Known 标记（%t）与原子表（%t）不一致", id, a.Atom, a.Known, inTable))
			}
		}
	}
	return problems
}
