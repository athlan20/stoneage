package synthesis

import "github.com/athlan20/stoneage/server/gamedata"

// STAGE-206：合成的输入整理（对应 ITEM_mergeItem_merge 里挑出有效材料、ITEM_mergeItem 里汇总原子的部分）。

// material 是一件参与合成的材料。
type material struct {
	id  int
	tpl gamedata.ItemTemplate
}

// mergeableMaterials 按顺序挑出能作材料的物品（CanMergeFrom 为真）。未知编号视为不能作材料。
func mergeableMaterials(ids []int) []material {
	var out []material
	for _, id := range ids {
		t, ok := gamedata.ItemByID(id)
		if !ok || !t.CanMergeFrom {
			continue
		}
		out = append(out, material{id: id, tpl: t})
	}
	return out
}

// gatherAtoms 按原子名把所有参与材料的 (原子, 数值) 归集到一起，原子的顺序是第一次出现的顺序（原版 ingindtable）。
// 原子表里没有的名字只跳过这一个原子。注意原版 ADD_INGRED 用的是 continue，会连带跳过同一物品后面的槽位；
// docs/synthesis.md 第 3.2 节写的是只跳过这一个，当前数据没有物品触发，这里按文档实现。
func gatherAtoms(parts []material) []atomSource {
	var srcs []atomSource
	pos := make(map[int]int)
	for _, p := range parts {
		for _, g := range gamedata.ItemIngredients(p.id) {
			ai, ok := gamedata.AtomIndex(g.Atom)
			if !ok {
				continue
			}
			i, seen := pos[ai]
			if !seen {
				i = len(srcs)
				pos[ai] = i
				srcs = append(srcs, atomSource{index: ai})
			}
			srcs[i].values = append(srcs[i].values, float64(g.Value))
		}
	}
	return srcs
}
