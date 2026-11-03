package gamedata

import "testing"

// 出生礼包里的物品都要在物品表里；马祖的棍棒 1 级可穿、精灵耗气 0。
func TestNewPlayerGifts(t *testing.T) {
	if len(NewPlayerGifts) > 15 {
		t.Fatalf("出生礼包 %d 件，超过背包 15 格", len(NewPlayerGifts))
	}
	for _, g := range NewPlayerGifts {
		if _, ok := ItemByID(g.ItemID); !ok || g.Count < 1 {
			t.Errorf("出生礼包物品 %d ×%d 无效", g.ItemID, g.Count)
		}
	}
	club, _ := ItemByID(2690)
	if club.Name != "马祖的棍棒" || club.Level != 1 {
		t.Errorf("2690 = %+v，want 马祖的棍棒 Level 1", club)
	}
}
