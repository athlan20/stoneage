package gamedata

import (
	_ "embed"
	"sort"
	"strings"
	"sync"
)

// 宠物固定原子（STAGE-205，合成宠物专长用，设计见 docs/synthesis.md 第 6 节）。数据由 tools/gen-petbase.py
// 从原版服务端 gmsv/data/enemybase.txt 生成：每个固定原子一行，列为 模板编号、序号（1–5）、原子名、BASEADD、FIXMIN、FIXMAX、在原子表里（1/0）。

//go:embed petatom.tsv
var petatomTSV string

// PetAtom 是宠物模板的一个固定原子（原版 ENEMYTEMP 的 E_T_ATOMFIXNAME1..5 与 BASEADD/FIXMIN/FIXMAX）。
// Known 为 false 表示原子名不在原子表里：原版数据里的缺口（如“加特洛”“美鲁娜”），原版 ITEM_merge_getPetFix
// 打印 "fucking atom" 后跳过，合成时也要跳过，不算材料。
type PetAtom struct {
	Atom    string
	BaseAdd int
	FixMin  int
	FixMax  int
	Known   bool
}

var (
	petAtomOnce  sync.Once
	petAtomTable map[int][]PetAtom
)

// PetAtoms 返回宠物模板的固定原子，按序号 1–5 排列（没有的模板返回 nil）。包含 Known 为 false 的条目，调用方自行跳过。
func PetAtoms(templateID int) []PetAtom {
	petAtomOnce.Do(loadPetAtoms)
	return append([]PetAtom(nil), petAtomTable[templateID]...)
}

// petAtomTemplateIDs 返回有固定原子的模板编号，升序。
func petAtomTemplateIDs() []int {
	petAtomOnce.Do(loadPetAtoms)
	ids := make([]int, 0, len(petAtomTable))
	for id := range petAtomTable {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// loadPetAtoms 解析 petatom.tsv，按模板编号归组，组内保持文件里的序号顺序。
func loadPetAtoms() {
	petAtomTable = make(map[int][]PetAtom, 712)
	for _, line := range strings.Split(petatomTSV, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 {
			continue
		}
		id := atoi(f[0])
		petAtomTable[id] = append(petAtomTable[id], PetAtom{
			Atom:    f[2],
			BaseAdd: atoi(f[3]),
			FixMin:  atoi(f[4]),
			FixMax:  atoi(f[5]),
			Known:   atoi(f[6]) == 1,
		})
	}
}
