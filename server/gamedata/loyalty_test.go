// 忠诚的拆分（STAGE-127）：LoyaltyBase + LoyaltyWith 必须和 Loyalty 的结果一致。

package gamedata

import "testing"

func TestLoyaltySplitMatchesWhole(t *testing.T) {
	for _, host := range []int{1, 10, 35, 60, 150} {
		for _, charm := range []int{0, 20, 60, 100} {
			for _, trans := range []int{0, 1} {
				for _, pet := range []int{1, 5, 30} {
					for _, variable := range []int{-10000, -2000, -5, 0, 5, 300, 10000} {
						want := Loyalty(host, charm, trans, pet, 100, variable)
						base, bonus := LoyaltyBase(host, charm, trans, pet, 100)
						if got := LoyaltyWith(base, bonus, variable); got != want {
							t.Fatalf("host=%d charm=%d trans=%d pet=%d var=%d：拆分 %d，整体 %d",
								host, charm, trans, pet, variable, got, want)
						}
					}
				}
			}
		}
	}
}

// 可变部分会让忠诚在 0–100 之间变化，不会越界。
func TestLoyaltyWithClamps(t *testing.T) {
	if got := LoyaltyWith(100, 0, 10000); got != 100 {
		t.Errorf("上限：得到 %d，want 100", got)
	}
	if got := LoyaltyWith(10, 0, -10000); got != 0 {
		t.Errorf("下限：得到 %d，want 0", got)
	}
}
