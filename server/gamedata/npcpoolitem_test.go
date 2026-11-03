package gamedata

import "testing"

// STAGE-309/347：道具仓库固定 50 格（不再随转生），宠物寄放池 min(转生×2+5, 15)，寄存费每件 50。
func TestPoolLimitsByTensei(t *testing.T) {
	cases := []struct{ tensei, items, pets int }{
		{0, 50, 5}, {1, 50, 7}, {4, 50, 13}, {5, 50, 15}, {9, 50, 15},
	}
	for _, c := range cases {
		if got := PoolItemLimit(c.tensei); got != c.items {
			t.Errorf("转生 %d 道具仓库 = %d，要 %d", c.tensei, got, c.items)
		}
		if got := PoolPetLimit(c.tensei); got != c.pets {
			t.Errorf("转生 %d 宠物寄放池 = %d，要 %d", c.tensei, got, c.pets)
		}
	}
	if PoolItemCost != 50 {
		t.Errorf("寄存费 = %d，要 50", PoolItemCost)
	}
}
