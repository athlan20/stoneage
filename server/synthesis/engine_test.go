package synthesis

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-206 端到端：用真实数据的物品和宠物。道具/料理的样本编号来自 gamedata（运行时查过的数据）：
//   1234 小块肉（料理，肉 10）、1235 普通的肉（料理，肉 28）、1236 带骨的肉（料理，肉 70）；
//   1 贝诺美斯Lv1斧头（石 10、木 10、毒 10）、2 威金尼尔Lv1斧头（石 10、木 10、石化 10）、3 迪所斯Lv1斧头（石 10、木 10、混乱 10）；
//   1229 变幻的衣服（不能作材料）；1231 定情项链（不能作材料）。
//   宠物 1 是乌力，固定原子 石、木、皮、骨、线；宠物 0 不存在，视为没有固定原子。

func rngFor(seed uint64) Rand {
	return rand.New(rand.NewPCG(seed, 7)).IntN
}

func TestNewEngineIsShared(t *testing.T) {
	if NewEngine() != NewEngine() {
		t.Errorf("NewEngine 应只建一次，两次调用应返回同一个引擎")
	}
}

// 小块肉 + 普通的肉：肉 10、28 汇总为 29（tablenum 0，比为 1，29 = 28 + 10×0.1），料理非固定随机到 [26, 32]。
// 只有 一口肉 12889（肉 26）满足 0.7v ≤ 26 ≤ 1.3v，所以不论种子结果都是 12889。
func TestMergeFoodPairAlwaysMakesOneBite(t *testing.T) {
	e := NewEngine()
	for seed := uint64(0); seed < 200; seed++ {
		out, err := e.Merge([]int{1234, 1235}, 1, rngFor(seed))
		if err != nil {
			t.Fatalf("seed %d：不应出错：%v", seed, err)
		}
		if !out.OK || out.RecipeID != 12889 || out.Attempts != 1 || out.Kind != KindFood {
			t.Fatalf("seed %d：应得 料理 12889（1 次尝试），实际 %+v", seed, out)
		}
	}
}

// 三件料理：肉 10、28、70 汇总为 77（见 docs/synthesis.md 第 3.2 节的累加），随机到 [69, 85]，
// 候选是 12890（肉 55）和 12891（肉 90），两者都可能，结果只能是其中之一。
func TestMergeThreeFoodsPicksOneOfTwo(t *testing.T) {
	e := NewEngine()
	seen := map[int]bool{}
	for seed := uint64(0); seed < 200; seed++ {
		out, err := e.Merge([]int{1234, 1235, 1236}, 1, rngFor(seed))
		if err != nil || !out.OK {
			t.Fatalf("seed %d：应成功，实际 %+v err=%v", seed, out, err)
		}
		if out.RecipeID != 12890 && out.RecipeID != 12891 {
			t.Fatalf("seed %d：产物应为 12890 或 12891，实际 %d", seed, out.RecipeID)
		}
		seen[out.RecipeID] = true
	}
	if len(seen) != 2 {
		t.Errorf("200 个种子里两种产物都应出现，实际 %v", seen)
	}
}

// 非食物：每次结果都要满足通用性质（产物不是参与材料；产物的原子都在材料里），失败时尝试 5 次。
func TestMergeItemsProperties(t *testing.T) {
	e := NewEngine()
	materialAtoms := map[string]bool{"石": true, "木": true, "毒": true, "石化": true}
	var okCount, failCount int
	for seed := uint64(0); seed < 200; seed++ {
		out, err := e.Merge([]int{1, 2}, 1, rngFor(seed))
		if err != nil {
			t.Fatalf("seed %d：不应出错：%v", seed, err)
		}
		if out.Kind != KindItem {
			t.Fatalf("seed %d：斧头应是道具合成，实际 %v", seed, out.Kind)
		}
		if !out.OK {
			failCount++
			if out.Attempts != maxAttempts || out.RecipeID != 0 {
				t.Fatalf("seed %d：失败应为 5 次尝试且无产物，实际 %+v", seed, out)
			}
			continue
		}
		okCount++
		if out.RecipeID == 1 || out.RecipeID == 2 {
			t.Fatalf("seed %d：产物不能是参与材料，实际 %d", seed, out.RecipeID)
		}
		for _, ing := range gamedata.ItemIngredients(out.RecipeID) {
			if !materialAtoms[ing.Atom] {
				t.Fatalf("seed %d：产物 %d 的原子 %s 不在材料里", seed, out.RecipeID, ing.Atom)
			}
		}
	}
	if okCount == 0 || failCount == 0 {
		t.Errorf("200 个种子里应既有成功又有失败，实际 成功 %d、失败 %d", okCount, failCount)
	}
}

// 同一输入、同一种子，结果必须相同。
func TestMergeIsDeterministicPerSeed(t *testing.T) {
	e := NewEngine()
	for seed := uint64(0); seed < 50; seed++ {
		a, errA := e.Merge([]int{1, 2}, 1, rngFor(seed))
		b, errB := e.Merge([]int{1, 2}, 1, rngFor(seed))
		if a != b || errA != errB {
			t.Fatalf("seed %d：两次结果不同：%+v %+v", seed, a, b)
		}
	}
}

// 不能作材料的物品被忽略：[1, 1231, 2] 和 [1, 2] 用同一种子，结果相同。
func TestMergeIgnoresNonMaterials(t *testing.T) {
	e := NewEngine()
	for seed := uint64(0); seed < 50; seed++ {
		a, errA := e.Merge([]int{1, 2}, 1, rngFor(seed))
		b, errB := e.Merge([]int{1, 1231, 2}, 1, rngFor(seed))
		if a != b || errA != errB {
			t.Fatalf("seed %d：忽略不能作材料的物品后结果应相同：%+v 与 %+v", seed, a, b)
		}
	}
}

func TestMergeTooFew(t *testing.T) {
	e := NewEngine()
	for _, ids := range [][]int{nil, {1}, {1229}, {1, 1231}, {1229, 1231, 1}} {
		if _, err := e.Merge(ids, 1, rngFor(1)); !errors.Is(err, ErrTooFew) {
			t.Errorf("材料 %v：有效材料不足 2 件应返回 ErrTooFew，实际 %v", ids, err)
		}
	}
}

// 食物与道具混合：不论哪一件在前，都返回 ErrMixed；Kind 由第一件参与材料决定。
func TestMergeMixed(t *testing.T) {
	e := NewEngine()
	out, err := e.Merge([]int{1234, 1}, 1, rngFor(1))
	if !errors.Is(err, ErrMixed) || out.Kind != KindFood {
		t.Errorf("料理在前、道具在后：应返回 ErrMixed 且 Kind 为料理，实际 %+v err=%v", out, err)
	}
	out, err = e.Merge([]int{1, 1234}, 1, rngFor(1))
	if !errors.Is(err, ErrMixed) || out.Kind != KindItem {
		t.Errorf("道具在前、料理在后：应返回 ErrMixed 且 Kind 为道具，实际 %+v err=%v", out, err)
	}
}

func BenchmarkMerge(b *testing.B) {
	e := NewEngine()
	ids := []int{1, 2, 3}
	for i := 0; i < b.N; i++ {
		_, _ = e.Merge(ids, 1, rngFor(uint64(i)))
	}
}
