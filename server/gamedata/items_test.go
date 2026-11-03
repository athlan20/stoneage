package gamedata

import (
	"reflect"
	"testing"
)

func TestItemByID(t *testing.T) {
	// itemset.txt 的第 0 行就是小斧头（docs/ui.md 背包页的示例物品）。
	it, ok := ItemByID(0)
	if !ok {
		t.Fatal("物品编号 0 应该存在")
	}
	want := ItemTemplate{Name: "小斧头", Effect: "攻 +9 防 -3 敏 -3", Type: 1, Cost: 65, Level: 0, Attack: 9, Defence: -3, Agility: -3, MagicID: -1, CanMergeFrom: true, Durability: 200000, MaxDurability: 200000} // 耐久是 STAGE-209（第 30、31 列）；基价 65 是 STAGE-70 的商店买卖价（itemset.txt 第 18 列）；可作材料是 STAGE-205（第 82 列为 1）
	if it != want {
		t.Errorf("小斧头解析错了: %+v，应为 %+v", it, want)
	}
	if lv, ok := ItemByID(140); !ok || lv.Level != 25 || lv.Attack != 20 {
		t.Errorf("大的棍棒应要求 25 级、攻击 20，现在 %+v", lv)
	}
	// STAGE-190：芙雷雅Lv1铠2 自带滋润的精灵 Lv1（魔法 10），施放耗 MP 8；小斧头没有精灵。
	if m, ok := ItemByID(1012); !ok || m.MagicID != 10 || m.MagicMP != 8 {
		t.Errorf("1012 应带精灵 10、MP 8，现在 %+v", m)
	}
	if _, ok := ItemByID(1 << 30); ok {
		t.Error("不存在的物品编号不应找到")
	}
}

func TestItemsUniqueAndCount(t *testing.T) {
	ItemByID(0) // 触发加载
	if len(itemTable) < 10000 {
		t.Errorf("物品表应有上万条，现在 %d", len(itemTable))
	}
}

func TestEquipPlaces(t *testing.T) {
	cases := map[int][]int{
		1:  {SlotHand},                       // 斧
		2:  {SlotHand},                       // 棍
		4:  {SlotHand},                       // 弓
		6:  {SlotHead},                       // 头盔
		7:  {SlotBody},                       // 铠甲
		8:  {SlotAccessory1, SlotAccessory2}, // 手镯
		15: {SlotAccessory1, SlotAccessory2}, // 护身符
		16: nil,                              // 其他（不能穿）
		20: nil,                              // 料理
	}
	for typ, want := range cases {
		if got := EquipPlaces(typ); !reflect.DeepEqual(got, want) {
			t.Errorf("类型 %d 的部位 = %v，应为 %v", typ, got, want)
		}
	}
}
