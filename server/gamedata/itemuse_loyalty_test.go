// 忠诚道具（STAGE-326）的单元测试：安抚之素、高级宠物食品的效果、世界中的目标类型、药师店的覆盖买价和封顶。

package gamedata

import "testing"

func TestUseEffectLoyaltyItems(t *testing.T) {
	// 安抚之素（1598）：忠30，只能在战斗外用，目标是他人（要选宠物）。
	def := mustItem(t, 1598)
	eff, ok := def.UseEffect()
	if !ok || eff.Loyalty != 30 || eff.HP != 0 || eff.MP != 0 || eff.Cure || eff.Revive {
		t.Fatalf("忠30 解析不对: %+v ok=%v", eff, ok)
	}
	if def.AbleUse != AbleUseField || def.BattleTarget() != TargetNone {
		t.Fatalf("安抚之素应只在战斗外用，战斗目标为 none: 场合 %d", def.AbleUse)
	}
	if ft := def.FieldTarget(); ft != TargetAlly {
		t.Fatalf("安抚之素世界中的目标类型应是 ally（选宠物），实际 %s", ft)
	}
	// 高级宠物食品（12763）：忠10，同样只在战斗外。
	if eff, ok := mustItem(t, 12763).UseEffect(); !ok || eff.Loyalty != 10 {
		t.Fatalf("忠10 解析不对: %+v ok=%v", eff, ok)
	}
	// 忠N 只在战斗外有效：普通的鱼（2436，可用场合为战斗外+战斗内）不因为“忠5”变成可用。
	if _, ok := mustItem(t, 2436).UseEffect(); ok {
		t.Fatal("普通的鱼（可用场合不是战斗外）不该因为忠5 变成可用")
	}
	// 普通的肉（1235）和复活药（1530）的世界目标类型不变：FieldTarget 与 BattleTarget 相同。
	for _, id := range []int{1235, 1530} {
		d := mustItem(t, id)
		if d.FieldTarget() != d.BattleTarget() {
			t.Fatalf("物品 %d 的世界目标类型不该和战斗的不同: %s / %s", id, d.FieldTarget(), d.BattleTarget())
		}
	}
}

func TestShopPriceOverrideAndSellBack(t *testing.T) {
	for _, npc := range []int{830, 884} {
		shop := NpcShopOf(npc)
		if shop == nil {
			t.Fatalf("NPC %d 不是商店", npc)
		}
		at := -1
		for i, id := range shop.Items {
			if id == 1598 {
				at = i
			}
		}
		if at < 0 {
			t.Fatalf("NPC %d 的商品里没有安抚之素", npc)
		}
		if _, price, ok := shop.BuyPrice(at); !ok || price != 10000 {
			t.Fatalf("NPC %d 的安抚之素买价应为 10000，实际 %d ok=%v", npc, price, ok)
		}
		// 覆盖价只作用于覆盖的商品：同店的其他商品仍按 基价 × buy_rate。
		other := shop.Items[0]
		tmpl := mustItem(t, other)
		if _, price, _ := shop.BuyPrice(0); price != int(float64(tmpl.Cost)*shop.BuyRate) {
			t.Fatalf("NPC %d 的 %d 买价不该被覆盖: %d", npc, other, price)
		}
		// 卖回价：基价 500 × sell_rate 0.2 = 100，不随买价覆盖变化。
		if p, ok := shop.SellPrice(1598); !ok || p != 100 {
			t.Fatalf("NPC %d 的安抚之素卖回价应为 100，实际 %d ok=%v", npc, p, ok)
		}
	}
}

func TestLoyaltyItemCap(t *testing.T) {
	// 忠诚 70 的宠物用安抚之素：增量 30 进可变部分（× 100），总值封顶 100。
	add := func(v int) int { return AddVariableAI(v, 30*100) }
	if got := LoyaltyWith(70, 0, add(0)); got != 100 {
		t.Fatalf("70 + 30 应封顶 100，实际 %d", got)
	}
	// 忠诚 40 用安抚之素：变成 70。
	if got := LoyaltyWith(40, 0, add(0)); got != 70 {
		t.Fatalf("40 + 30 应是 70，实际 %d", got)
	}
	// 可变部分本身有上限（±10000），再加也不会超过。
	if got := AddVariableAI(MaxVariableAI-100, 3000); got != MaxVariableAI {
		t.Fatalf("可变部分应封顶 %d，实际 %d", MaxVariableAI, got)
	}
}
