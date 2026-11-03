// STAGE-226：装备抗魔参数的解析（原版 ITEM_MagicEquitWear 的限幅与键名）。

package gamedata

import "testing"

func TestEquipMagicDefFromItemset(t *testing.T) {
	// 1291 伊兹魔铠：参数 EA:20|WA:20|FI:20|WI:20|QU:20。
	it, ok := ItemByID(1291)
	if !ok {
		t.Fatal("物品 1291 不在 itemset 里")
	}
	if got, want := it.EquipMagicDef(), [5]int{20, 20, 20, 20, 20}; got != want {
		t.Fatalf("EquipMagicDef = %v, want %v", got, want)
	}
}

func TestEquipMagicDefLimit(t *testing.T) {
	cases := []struct {
		arg  string
		want [5]int
	}{
		{"", [5]int{}},
		{"WAST:-1", [5]int{}}, // 不认识的键不计
		{"EA:100|WA:-100", [5]int{100, -100, 0, 0, 0}},   // 边界值本身算
		{"EA:101|WA:-101|FI:50", [5]int{0, 0, 50, 0, 0}}, // 超限的那一项整项不计，不截到 100
		{"EA:abc|WI:7|QU:-3", [5]int{0, 0, 0, 7, -3}},    // 非数字不计
	}
	for _, c := range cases {
		got := ItemTemplate{Argument: c.arg}.EquipMagicDef()
		if got != c.want {
			t.Errorf("参数 %q：EquipMagicDef = %v, want %v", c.arg, got, c.want)
		}
	}
}
