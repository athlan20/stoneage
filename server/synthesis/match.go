package synthesis

// STAGE-206 第 4 节：配方匹配的数值规则与“先选几种原子再抽配方”的重试（原版 ITEM_merge_with_retry，item_gen.c:575）。

// maxIngredients 是一次合成最多的原子种类（原版 ideal 上限 5，配方也最多 5 个原子）。
const maxIngredients = 5

// maxAttempts 是一次合成最多的匹配尝试次数（原版 ITEM_mergeItem 的 for(i<5)）。
const maxAttempts = 5

// foodSearchMin、foodSearchMax 对应原版 ItemSearchTable[1]（料理的配方数值范围 0.7 到 1.3 倍）。
const (
	foodSearchMin = 0.7
	foodSearchMax = 1.3
)

// foodCap 是料理材料数值的上限：原版 ItemRandTableForItem[9].maxnum / 1.3 取整，即 1059 / 1.3 = 814。
// 注意是 maxnum（1059），不是 num（905）；docs/synthesis.md 写的 696 是按 num 算的，已在文档里更正。
var foodCap = int(float64(rateTable[9].maxnum) / foodSearchMax)

// idealThresholds 对应原版 idel_num：按材料原子种类数 ideal 分，r 从高到低跨过哪个阈值就选第几档，
// 档号 idx 对应要找的原子数 k = ideal - idx。第 idx 档的阈值之下才会落到更后的档。
var idealThresholds = [maxIngredients + 1][maxIngredients]int{
	1: {0},
	2: {250, 0},
	3: {400, 150, 0},
	4: {700, 260, 70, 0},
	5: {740, 500, 200, 40, 0},
}

// matchValue 判断配方的一个原子数值 c 能否由材料值 v 合成（非守护兽的规则）。
//   - 道具：v/rate ≤ c ≤ min(v×rate, 1000)，rate 按 v 所在档算；上界用 float32 比较（原版 toplimit 是 float）。
//   - 料理：v 先夹到 foodCap（原版会修改共享的数值），再 0.7v ≤ c ≤ 1.3v。
func matchValue(v, c int, dish bool) bool {
	if dish {
		if v > foodCap {
			v = foodCap
		}
		return float64(c) <= float64(v)*foodSearchMax && float64(c) >= float64(v)*foodSearchMin
	}
	rate := rateOf(v)
	top := float32(float64(v) * rate)
	if top > 1000 {
		top = 1000
	}
	return float32(c) <= top && float64(c) >= float64(v)*(1/rate)
}

// retryMatch 复现一次 ITEM_merge_with_retry 的抽取：每轮抽 r∈[0,999] 定档，同一档只试一次，
// 抽到试过的就重抽；ideal 档都试过仍无候选则失败。候选非空时用 rnd 均匀选一个。
// cands 按原子数分组（见 recipeIndex.candidates）。
func retryMatch(cands *[6][]int, ideal int, rnd Rand) (int, bool) {
	var tried [maxIngredients]bool
	extracted := 0
	for extracted < ideal {
		r := rnd(1000)
		idx := 0
		for idx < ideal && r < idealThresholds[ideal][idx] {
			idx++
		}
		if tried[idx] {
			continue
		}
		tried[idx] = true
		extracted++
		if list := cands[ideal-idx]; len(list) > 0 {
			return list[rnd(len(list))], true
		}
	}
	return 0, false
}
