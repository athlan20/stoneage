package gamedata

import "testing"

func TestNpcPetshopCost(t *testing.T) {
	// 74 号「宠物店」（地图 130）：nomal_rate=1.0、special_rate=1.5、支持寄放。
	ps := NpcPetshopOf(74)
	if ps == nil {
		t.Fatal("74 号 NPC 应是宠物店")
	}
	if ps.NomalRate != 1.0 || ps.SpecialRate != 1.5 || !ps.Pool {
		t.Fatalf("参数应来自 ps 参数文件: %+v", ps)
	}
	// 卖价 = ((等级²×10 − 捕获等级²×10) + 等级×10) × 稀有系数 × 倍率 × 魅力系数（100 表示不打折）：
	// 1 级捕获的 1 级无稀有宠 = 10；10 级 = (1000−10+100) = 1090。
	if got := ps.SellCost(1, 1, 0, 2, 100); got != 10 {
		t.Errorf("1 级无稀有宠卖价应是 10，实际 %d", got)
	}
	if got := ps.SellCost(10, 1, 0, 2, 100); got != 1090 {
		t.Errorf("10 级宠卖价应是 1090，实际 %d", got)
	}
	// CHAR_RARE 0→1、1→5、2→8；其余原值（原版 rare 没有映射的情况直接当系数用）。
	for rare, want := range map[int]int{0: 10, 1: 50, 2: 80, 3: 30} {
		if got := ps.SellCost(1, 1, rare, 2, 100); got != want {
			t.Errorf("稀有 %d 的卖价应是 %d，实际 %d", rare, want, got)
		}
	}
}

func TestNpcPetshopSellCharm(t *testing.T) {
	// 魅力系数 = max(人物魅力 + 宠物忠诚, 20) / 2（原版 NPC_GetCost 末尾，整除）。
	if got := SellCharm(60, 27); got != 43 {
		t.Errorf("魅力 60、忠诚 27 的系数应是 43，实际 %d", got)
	}
	if got := SellCharm(0, 0); got != 10 {
		t.Errorf("下限 20 之后减半：应是 10，实际 %d", got)
	}
	// 实测：格鲁西斯（模板 193，稀有 0）1 级、店 74、魅力 60、忠诚 27 → 原版 4 金币。
	ps := NpcPetshopOf(74)
	if got := ps.SellCost(1, 1, 0, 193, SellCharm(60, 27)); got != 4 {
		t.Errorf("格鲁西斯 1 级的卖价应是原版的 4，实际 %d", got)
	}
}

func TestNpcPetshopPoolCostAndFlag(t *testing.T) {
	// 寄放费 = 50 + 等级×4（原版 NPC_GETPOOLCOST；参数 pool_cost 原版注释掉了没有用）。
	if got := PoolCost(1); got != 54 {
		t.Errorf("1 级寄放费应是 54，实际 %d", got)
	}
	if got := PoolCost(10); got != 90 {
		t.Errorf("10 级寄放费应是 90，实际 %d", got)
	}
	// 不支持寄放的店（原版 pool_flg=0）：柯奥的宠物店。
	if ps := NpcPetshopOf(271); ps == nil || ps.Pool {
		t.Errorf("271 号应是不支持寄放的宠物店: %+v", ps)
	}
	if MaxPoolPets != 15 || PooledSlotBase != 50 {
		t.Errorf("寄放池应为 15 格、槽位基数 50，实际 %d/%d", MaxPoolPets, PooledSlotBase)
	}
}
