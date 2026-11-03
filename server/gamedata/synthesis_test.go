package gamedata

import "testing"

// STAGE-205：合成数据的计数。期望值来自原版服务端 gmsv/data 的原始数据（docs/synthesis.md 第 2、10 节），
// 对不上说明加载或列号有 bug，不要直接改这里的数字。

func TestItemMergeCounts(t *testing.T) {
	itemOnce.Do(loadItems)
	if len(itemTable) != 12620 {
		t.Errorf("物品表应有 12620 行，实际 %d", len(itemTable))
	}
	var from, to, dish, other int
	for _, it := range itemTable {
		if it.CanMergeFrom {
			from++
		}
		if it.CanMergeTo {
			to++
			if it.Type == ItemTypeDish {
				dish++
			} else {
				other++
			}
		}
	}
	if from != 11145 {
		t.Errorf("能作材料的应为 11145，实际 %d", from)
	}
	if to != 9381 {
		t.Errorf("能作配方的应为 9381，实际 %d", to)
	}
	if dish != 409 || other != 8972 {
		t.Errorf("配方里料理 409、其他 8972，实际 料理 %d、其他 %d", dish, other)
	}
}

func TestRecipeAtomCounts(t *testing.T) {
	itemOnce.Do(loadItems)
	dist := map[int]int{}
	for id, it := range itemTable {
		if !it.CanMergeTo {
			continue
		}
		dist[len(ItemIngredients(id))]++
	}
	want := map[int]int{1: 11, 2: 342, 3: 2104, 4: 4870, 5: 2054}
	for n, w := range want {
		if dist[n] != w {
			t.Errorf("配方原子数为 %d 的应有 %d 个，实际 %d", n, w, dist[n])
		}
	}
	if len(dist) != len(want) {
		t.Errorf("配方原子数分布应只有 1–5 种，实际 %v", dist)
	}
}

// 原子种类：所有物品的原子名去重后是 74 种（文档第 2 节“配方用到的原子种类 74 个”）。
// 只算配方（能作配方的物品）是 73 种，第 74 个是只出现在材料、没有配方标记的物品上。
func TestAtomKinds(t *testing.T) {
	itemOnce.Do(loadItems)
	all, recipe := map[string]bool{}, map[string]bool{}
	for id, it := range itemTable {
		for _, g := range ItemIngredients(id) {
			all[g.Atom] = true
			if it.CanMergeTo {
				recipe[g.Atom] = true
			}
		}
	}
	if len(all) != 74 {
		t.Errorf("物品用到的原子种类应为 74，实际 %d", len(all))
	}
	if len(recipe) != 73 {
		t.Errorf("配方用到的原子种类应为 73，实际 %d", len(recipe))
	}
}

func TestAtomTable(t *testing.T) {
	if AtomCount() != 83 {
		t.Errorf("原子表应有 83 个，实际 %d", AtomCount())
	}
	names := AtomNames()
	if len(names) != 83 || names[0] != "饮用水" {
		t.Errorf("AtomNames 应有 83 个且第 0 个是饮用水，实际 %d 个、首个 %q", len(names), names[0])
	}
	for i, n := range names {
		if got, ok := AtomIndex(n); !ok || got != i {
			t.Errorf("原子 %s 的下标应为 %d，实际 %d（%t）", n, i, got, ok)
		}
	}
	if _, ok := AtomIndex("加特洛"); ok {
		t.Error("加特洛不在原子表里（原版数据缺口）")
	}
}

func TestSmallAxeIngredients(t *testing.T) {
	// 第一行“小斧头”：可作材料、不能作配方，原子 石 10、木 10。
	it, ok := ItemByID(0)
	if !ok || !it.CanMergeFrom || it.CanMergeTo {
		t.Fatalf("小斧头应可作材料、不可作配方，实际 %+v", it)
	}
	want := []Ingredient{{Atom: "石", Value: 10}, {Atom: "木", Value: 10}}
	got := ItemIngredients(0)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("小斧头的原子应为 石10、木10，实际 %+v", got)
	}
}

func TestRecipeBigValues(t *testing.T) {
	itemOnce.Do(loadItems)
	n := 0
	for id, it := range itemTable {
		if !it.CanMergeTo || it.Type == ItemTypeDish {
			continue
		}
		for _, g := range ItemIngredients(id) {
			if g.Value > 1000 {
				n++
				break
			}
		}
	}
	if n != 4111 {
		t.Errorf("非食物配方里含大于 1000 原子数值的应有 4111 个，实际 %d", n)
	}
}

func TestPetFixedAtoms(t *testing.T) {
	// enemybase.txt 第一行是乌力：固定原子 石、木、皮、骨、线，BASEADD 0，FIXMIN、FIXMAX 都是 700。
	want := []PetAtom{
		{Atom: "石", BaseAdd: 0, FixMin: 700, FixMax: 700, Known: true},
		{Atom: "木", BaseAdd: 0, FixMin: 700, FixMax: 700, Known: true},
		{Atom: "皮", BaseAdd: 0, FixMin: 700, FixMax: 700, Known: true},
		{Atom: "骨", BaseAdd: 0, FixMin: 700, FixMax: 700, Known: true},
		{Atom: "线", BaseAdd: 0, FixMin: 700, FixMax: 700, Known: true},
	}
	got := PetAtoms(1)
	if len(got) != len(want) {
		t.Fatalf("乌力应有 5 个固定原子，实际 %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("乌力第 %d 个固定原子应为 %+v，实际 %+v", i+1, want[i], got[i])
		}
	}
	if PetAtoms(1<<30) != nil {
		t.Error("不存在的模板应返回 nil")
	}
}

func TestPetFixedAtomTotals(t *testing.T) {
	petAtomOnce.Do(loadPetAtoms)
	total, unknown := 0, map[string]bool{}
	ids := petAtomTemplateIDs()
	for _, id := range ids {
		for _, a := range PetAtoms(id) {
			total++
			if !a.Known {
				unknown[a.Atom] = true
			}
		}
	}
	if total != 3316 {
		t.Errorf("宠物固定原子应共 3316 条，实际 %d", total)
	}
	// 有固定原子的模板：按 enemybase.txt 数，每个模板第 1–5 列至少一个原子名，共 712 个（952 个模板中）。
	// 文档第 2、10 节写的是 495，这个数按任何口径都复现不了（3316 条与 712 个模板的列布局一致），需要核对文档。
	if len(ids) != 712 {
		t.Errorf("有固定原子的模板应有 712 个，实际 %d", len(ids))
	}
	// 原版数据缺口：加特洛、美鲁娜不在原子表里，标 Known 为 false（30 条）。
	if !unknown["加特洛"] || !unknown["美鲁娜"] || len(unknown) != 2 {
		t.Errorf("原版缺口应只有 加特洛、美鲁娜 两个原子名，实际 %v", unknown)
	}
}

func TestValidateSynthesisDataClean(t *testing.T) {
	if p := ValidateSynthesisData(); len(p) != 0 {
		t.Errorf("合成数据校验应没有问题，实际 %d 个：%v", len(p), p)
	}
}
