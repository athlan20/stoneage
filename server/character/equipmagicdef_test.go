// STAGE-226：穿戴抗魔装备的加成随穿脱增减（原版 ITEM_MagicEquitWear / ITEM_MagicEquitReWear）。
// 加成从当前穿在身上的装备算，所以穿上计入、脱下（挪到背包格）就扣掉，不用另外记账。

package character

import "testing"

// 1291 伊兹魔铠：EA/WA/FI/WI/QU 各 20。
const magicArmor = 1291

func TestEquipMagicDefAddsAndRemoves(t *testing.T) {
	cases := []struct {
		name  string
		items []itemRec
		want  [5]int
	}{
		{"没穿装备", nil, [5]int{}},
		{"穿上一件", []itemRec{{Slot: 0, ItemID: magicArmor}}, [5]int{20, 20, 20, 20, 20}},
		{"穿两件累加", []itemRec{{Slot: 0, ItemID: magicArmor}, {Slot: 1, ItemID: magicArmor}}, [5]int{40, 40, 40, 40, 40}},
		{"背包里的不计", []itemRec{{Slot: firstBagSlot, ItemID: magicArmor}}, [5]int{}},
		{"穿着一件、背包里一件", []itemRec{{Slot: 0, ItemID: magicArmor}, {Slot: firstBagSlot, ItemID: magicArmor}}, [5]int{20, 20, 20, 20, 20}},
	}
	for _, c := range cases {
		if got := equipBonus(c.items).MagicDef; got != c.want {
			t.Errorf("%s：MagicDef = %v, want %v", c.name, got, c.want)
		}
	}
}

// 脱下：同一件装备从格 0 挪到背包格 5 后加成被扣掉（planMove 的结果直接喂给 equipBonus）。
func TestEquipMagicDefRemovedOnUnequip(t *testing.T) {
	items := []itemRec{{Slot: 0, ItemID: magicArmor}}
	if got := equipBonus(items).MagicDef[0]; got != 20 {
		t.Fatalf("穿上后地抗加成 = %d, want 20", got)
	}
	items[0].Slot = firstBagSlot
	if got := equipBonus(items).MagicDef; got != [5]int{} {
		t.Fatalf("脱下后 MagicDef = %v, want 全 0", got)
	}
}

// 战斗快照里人物的抗性 = 持久化抗性 + 穿着装备的抗魔（原版 battle_magic.c 防守时的合算）。
func TestBattleResistAddsEquipMagicDef(t *testing.T) {
	eq := equipBonus([]itemRec{{Slot: 0, ItemID: magicArmor}})
	if got := battleResist([4]int{10, 0, 0, 5}, eq); got != [4]int{30, 20, 20, 25} {
		t.Fatalf("battleResist = %v, want [30 20 20 25]", got)
	}
	if got := battleResist([4]int{10, 0, 0, 5}, equipMods{}); got != [4]int{10, 0, 0, 5} {
		t.Fatalf("没穿装备时 battleResist = %v, want 原值", got)
	}
}
