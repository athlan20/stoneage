package gamedata

import (
	"reflect"
	"testing"
)

// STAGE-235：转生重算的测试。手算过程写在各用例注释里，出处同 pettrans.go。

func zeroRnd(int) int  { return 0 }
func maxRnd(n int) int { return n - 1 }

func TestTransAnswerHand(t *testing.T) {
	cases := []struct {
		name                            string
		total1, total2, level, rank, tr int
		want                            int
	}{
		// 见 TransAnswer 注释：9 + 80 + 0 = 89。
		{"基准", 150, 80, 100, 0, 0, 89},
		// 玛蕾合计 100：t = 1，t⁵ = 1，× 1.3 取整得 1；level 130 时 (130−100) ÷ 11 = 2；ans = 1 + 150 + 2 = 153。
		// TRANS 0 上限 150，所以得 150；TRANS 1 上限 200，得 153。
		{"TRANS 0 超上限", 100, 150, 130, 0, 0, 150},
		{"TRANS 1 不超上限", 100, 150, 130, 0, 1, 153},
		// 玛蕾 150：9；被转生宠物合计 150：150；level 130：2；ans = 161。TRANS 0 截成 150，TRANS 1 为 161。
		{"TRANS 1 上限 200", 150, 150, 130, 0, 1, 161},
		{"TRANS 0 同输入截成 150", 150, 150, 130, 0, 0, 150},
		// 玛蕾合计 0：t = 0 小于 1，额外项为 0；ans = 80 + (100−100) ÷ 11 = 80。
		{"玛蕾为 0", 0, 80, 100, 0, 0, 80},
		// 品质 5：Fx = (int)(0 × 1.2) + 5 = 5；(130−100) ÷ 5 = 6；ans = 9 + 80 + 6 = 95。
		{"品质 5", 150, 80, 130, 5, 0, 95},
		// 品质 6：Fx = (int)(−1 × 1.2) + 5 = −1 + 5 = 4（C 的 int 截断朝 0）；30 ÷ 4 = 7；ans = 9 + 80 + 7 = 96。
		{"品质 6 的 Fx 为 4", 150, 80, 130, 6, 0, 96},
		// 等级 200 先限制到 130，结果同 130：ans = 9 + 80 + 2 = 91。
		{"等级超过 130 按 130", 150, 80, 200, 0, 0, 91},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TransAnswer(c.total1, c.total2, c.level, c.rank, c.tr); got != c.want {
				t.Fatalf("TransAnswer = %d，want %d", got, c.want)
			}
		})
	}
}

// 手算（基准输入）：mala 合计 150 不变，pet 合计 80，ans = 89，分母 150 + 80×4 = 470。
// 分配：vital (40 + 20×4)·89/470 = 10680/470 = 22.7 → 22；str 同 vital → 22；
// tgh (35 + 20×4)·89/470 = 10235/470 = 21.8 → 21；dex 同 tgh → 21。work = {22, 22, 21, 21}。
func TestTransmigrateHandCalc(t *testing.T) {
	mala := Growth{40, 40, 35, 35}
	pet := Growth{20, 20, 20, 20}
	tmpl := PetTemplate{InitNum: 10}

	// rnd 全给 0：每项 ±2 得 −2，base = {20, 20, 19, 19}，合计 78，品质 5（78 < 80）。
	// 10 点全给 Vital（rnd(4) = 0）：资质 {30, 20, 19, 19}，能力 = 10 × 资质。
	got := Transmigrate(mala, pet, tmpl, 100, 0, 0, zeroRnd)
	want := TransResult{
		Answer: 89,
		Base:   Growth{20, 20, 19, 19},
		Rank:   5,
		Stats:  Ability{300, 200, 190, 190},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("zeroRnd: got %+v, want %+v", got, want)
	}

	// rnd 全给最大值：rnd(5) = 4 → +2，base = {24, 24, 23, 23}，合计 94，品质 3（94 ≥ 85）。
	// 10 点全给 Dexterity（rnd(4) = 3）：资质 {24, 24, 23, 33}，能力 {240, 240, 230, 330}。
	got = Transmigrate(mala, pet, tmpl, 100, 0, 0, maxRnd)
	want = TransResult{
		Answer: 89,
		Base:   Growth{24, 24, 23, 23},
		Rank:   3,
		Stats:  Ability{240, 240, 230, 330},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("maxRnd: got %+v, want %+v", got, want)
	}
}

// 原版随机数的调用顺序和范围：先四次 RAND(0,4)（范围 0–4），再十次 RAND(0,3)（范围 0–3）。
func TestTransNewPetRndOrder(t *testing.T) {
	var seen []int
	rec := func(n int) int {
		seen = append(seen, n)
		return 0
	}
	TransNewPet(Growth{22, 22, 21, 21}, PetTemplate{InitNum: 10}, rec)
	want := []int{5, 5, 5, 5, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("rnd 调用 %v，want %v", seen, want)
	}
}

// 品质门槛（GetNewPet 的 ranktbl）：130→0，100→1，95→2，85→3，80→4，其余（≥ 0）→5；合计为负数时品质 0。
func TestTransRankBoundaries(t *testing.T) {
	cases := map[int]int{130: 0, 129: 1, 100: 1, 99: 2, 95: 2, 94: 3, 85: 3, 84: 4, 80: 4, 79: 5, 0: 5, -1: 0}
	for sum, want := range cases {
		if got := transRank(sum); got != want {
			t.Errorf("transRank(%d) = %d，want %d", sum, got, want)
		}
	}
}

// 合计为 0 时原版会得到 NaN，这里不能除零。等级 1 时配额为负数，原版也是这样（C 的整数除法朝 0 截断）。
func TestTransmigrateZeroTotal(t *testing.T) {
	got := Transmigrate(Growth{}, Growth{}, PetTemplate{InitNum: 10}, 1, 0, 0, zeroRnd)
	if got.Answer != -9 { // (1−100) ÷ 11 = −9
		t.Fatalf("Answer = %d，want -9", got.Answer)
	}
	if got.Base != (Growth{-2, -2, -2, -2}) {
		t.Fatalf("Base = %+v", got.Base)
	}
}
