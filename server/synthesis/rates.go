package synthesis

// STAGE-206：材料值分档（docs/synthesis.md 第 3.1 节）。原版 item_gen.c 的 ItemRandTableForItem，
// 由 ITEM_initRandTable（item_gen.c:160）的算法生成，不手抄数值。

// rateCount 是分档的档数（原版 arraysizeof(ItemRandTableForItem) 即 20）。
const rateCount = 20

// itemGenRate 对应原版 ITEM_GEN_RATE（0.7）：每档的上限取到下一档 num 的 70%。
const itemGenRate = 0.7

// rateNums 是每档的 num（原版 ItemRandTableForItem 的初值，item_gen.c）。
var rateNums = [rateCount]int{
	10, 30, 65, 125, 205, 305, 425, 565, 725, 905,
	1125, 1354, 1594, 1825, 2105, 2405, 2725, 3065, 3425, 3805,
}

// rateEntry 是一档：num、maxnum（取值上限）、minnum（下限，原版只在守护兽分支用）和 rate = maxnum / num。
type rateEntry struct {
	num    int
	minnum int
	maxnum int
	rate   float64
}

var rateTable = buildRateTable()

// buildRateTable 复现 ITEM_initRandTable：maxnum = num + (下一档 num − num) × 0.7，赋给 int 时截断；
// 最后一档 maxnum 固定 4000；minnum 为上一档 maxnum + 1，第 0 档为 0。
func buildRateTable() [rateCount]rateEntry {
	var t [rateCount]rateEntry
	for i := range t {
		t[i].num = rateNums[i]
		if i == 0 {
			t[i].minnum = 0
		} else {
			t[i].minnum = t[i-1].maxnum + 1
		}
		if i+1 < rateCount {
			// 乘积先转成 float64 再相加，避免编译器融合成一条 FMA，结果和 C 逐步计算一致。
			t[i].maxnum = int(float64(t[i].num) + float64(float64(rateNums[i+1]-t[i].num)*itemGenRate))
		} else {
			t[i].maxnum = 4000
		}
		t[i].rate = float64(t[i].maxnum) / float64(t[i].num)
	}
	return t
}

// tableNum 返回值所在的档（原版 ITEM_getTableNum，item_gen.c:1241）：第一个满足 v ≤ maxnum 的档，
// 超过最后一档时取最后一档。
func tableNum(v int) int {
	for i := range rateTable {
		if v <= rateTable[i].maxnum {
			return i
		}
	}
	return rateCount - 1
}

// rateOf 是值 v 所在档的 rate（原版 ItemRandTableForItem[ITEM_getTableNum(v)].rate）。
func rateOf(v int) float64 {
	return rateTable[tableNum(v)].rate
}
