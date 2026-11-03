package gamedata

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

// 原子表（STAGE-205，合成用）。数据由 tools/gen-itematom.py 从原版服务端 gmsv/data/itematom.txt 生成。
// 原子的下标是行序，即原版 ITEM_getAtomIndexByName 的返回值。

//go:embed itematom.tsv
var itematomTSV string

// atomsOnce 保证原子表只解析一次；atomIndex 是名字到下标的表，atomNames 按下标排列。
var (
	atomsOnce sync.Once
	atomIndex map[string]int
	atomNames []string
)

// AtomIndex 按原子名取下标；不认识的名字返回 false。
func AtomIndex(name string) (int, bool) {
	atomsOnce.Do(loadAtoms)
	i, ok := atomIndex[name]
	return i, ok
}

// AtomNames 返回全部原子名，下标与 AtomIndex 一致（即原版 ITEM_getAtomIndexByName 的顺序）。
// 返回的是副本，调用方改动不影响表。
func AtomNames() []string {
	atomsOnce.Do(loadAtoms)
	return append([]string(nil), atomNames...)
}

// AtomCount 是原子表的条数（原版 83 个）。
func AtomCount() int {
	atomsOnce.Do(loadAtoms)
	return len(atomNames)
}

// loadAtoms 解析 itematom.tsv：原子下标、名字、魔法标志、编号（后两列原版只存不用）。
func loadAtoms() {
	atomIndex = make(map[string]int, 83)
	atomNames = make([]string, 0, 83)
	sc := bufio.NewScanner(strings.NewReader(itematomTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			continue
		}
		i := atoi(f[0])
		atomIndex[f[1]] = i
		// 下标就是行序，正常情况下 len(atomNames) == i；这里按下标写，行序错了也不会错位。
		for len(atomNames) <= i {
			atomNames = append(atomNames, "")
		}
		atomNames[i] = f[1]
	}
}
