package synthesis

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-206 第 6 节：宠物固定原子的整理（buildFixedAtoms 的规则）。

func atomIdx(t *testing.T, name string) int {
	t.Helper()
	i, ok := gamedata.AtomIndex(name)
	if !ok {
		t.Fatalf("原子 %s 应在原子表里", name)
	}
	return i
}

// 原子表里没有的名字，用来模拟原版数据的缺口（docs/synthesis.md 第 10 节的“加特洛”“美鲁娜”）。
const missingAtom = "加特洛"

func TestMissingAtomIsNotInTable(t *testing.T) {
	if _, ok := gamedata.AtomIndex(missingAtom); ok {
		t.Fatalf("%s 应不在原子表里，测试的前提不成立", missingAtom)
	}
}

// 乌力（模板 1）：固定原子 石、木、皮、骨、线（docs/synthesis.md 第 2 节），BASEADD 0，FIXMIN 和 FIXMAX 都是 700。
func TestPetFixedAtomsFromData(t *testing.T) {
	got := petFixedAtoms(1)
	names := []string{"石", "木", "皮", "骨", "线"}
	want := make([]fixedAtom, len(names))
	for i, n := range names {
		want[i] = fixedAtom{index: atomIdx(t, n), baseAdd: 0, fixMin: 700, fixMax: 700}
	}
	if len(got) != len(want) {
		t.Fatalf("乌力的固定原子应有 %d 个，实际 %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("固定原子 %d 应为 %+v，实际 %+v", i, want[i], got[i])
		}
	}
}

func TestBuildFixedAtomsSwapsMinMax(t *testing.T) {
	got := buildFixedAtoms([]gamedata.PetAtom{{Atom: "石", FixMin: 800, FixMax: 600, Known: true}})
	if len(got) != 1 || got[0].fixMin != 600 || got[0].fixMax != 800 {
		t.Errorf("FIXMIN 大于 FIXMAX 时应交换，实际 %+v", got)
	}
}

// 负数按非守护兽换成 1000；BASEADD 原样保留（可以是负的）。
func TestBuildFixedAtomsNegativeBecomes1000(t *testing.T) {
	got := buildFixedAtoms([]gamedata.PetAtom{{Atom: "木", BaseAdd: -500, FixMin: -5, FixMax: 300, Known: true}})
	if len(got) != 1 {
		t.Fatalf("应有 1 个固定原子，实际 %d", len(got))
	}
	if got[0].fixMin != 1000 || got[0].fixMax != 300 || got[0].baseAdd != -500 {
		t.Errorf("FIXMIN 为负应变 1000、BASEADD 保留，实际 %+v", got[0])
	}
}

// 原版的 continue 会跳过后面的序号：石、缺口、木 → 只剩石。
func TestBuildFixedAtomsStopsAtFirstGap(t *testing.T) {
	got := buildFixedAtoms([]gamedata.PetAtom{
		{Atom: "石", FixMin: 700, FixMax: 700, Known: true},
		{Atom: missingAtom, FixMin: 700, FixMax: 700, Known: false},
		{Atom: "木", FixMin: 700, FixMax: 700, Known: true},
	})
	if len(got) != 1 || got[0].index != atomIdx(t, "石") {
		t.Errorf("第一个缺口之后的固定原子应全部丢失，实际 %+v", got)
	}
}

func TestBuildFixedAtomsGapFirstDropsAll(t *testing.T) {
	got := buildFixedAtoms([]gamedata.PetAtom{
		{Atom: missingAtom, FixMin: 700, FixMax: 700, Known: false},
		{Atom: "石", FixMin: 700, FixMax: 700, Known: true},
	})
	if len(got) != 0 {
		t.Errorf("序号 1 就是缺口时应没有固定原子，实际 %+v", got)
	}
}

func TestFindFixedFirstOccurrenceWins(t *testing.T) {
	fixed := []fixedAtom{{index: 3, baseAdd: 1}, {index: 3, baseAdd: 2}}
	if got := findFixed(fixed, 3); got == nil || got.baseAdd != 1 {
		t.Errorf("同一原子出现两次时应取序号靠前的，实际 %+v", got)
	}
	if got := findFixed(fixed, 4); got != nil {
		t.Errorf("没有的原子应返回 nil，实际 %+v", got)
	}
}
