package gamedata

import "testing"

func TestNpcHealerCost(t *testing.T) {
	// 16 号「森林的护士」（地图 100）：level=1（没有免费线）、hp_rate=4.0、mp_rate=6.0。
	h := NpcHealerOf(16)
	if h == nil {
		t.Fatal("16 号 NPC 应是护士（森林的护士）")
	}
	if h.Level != 1 || h.HPRate != 4.0 || h.MPRate != 6.0 {
		t.Fatalf("参数应来自创建块（1/4.0/6.0），实际 %+v", h)
	}
	// 费用 = 等级 × 倍率：1 级 4 金币；至少 1。
	if cost, free := h.HPCost(1); cost != 4 || free {
		t.Errorf("1 级 HP 费用应是 4，实际 %d free=%v", cost, free)
	}
	if cost, _ := h.MPCost(2); cost != 12 {
		t.Errorf("2 级 MP 费用应是 12，实际 %d", cost)
	}
	if cost, _ := h.HPCost(0); cost != 1 {
		t.Errorf("费用至少 1（0 级也是 1），实际 %d", cost)
	}
}

func TestNpcHealerFreeLine(t *testing.T) {
	// 224 号「萨姆吉尔的护士」level=10：9 级以下免费，10 级起收费。
	h := NpcHealerOf(224)
	if h == nil || h.Level != 10 {
		t.Fatalf("224 号应是免费线 10 的护士: %+v", h)
	}
	if _, free := h.HPCost(9); !free {
		t.Error("9 级应免费")
	}
	if _, free := h.HPCost(10); free {
		t.Error("10 级应收费")
	}
	if NpcHealerOf(198) != nil {
		t.Error("198 是商店，不是护士")
	}
}
