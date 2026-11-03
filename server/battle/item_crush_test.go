package battle

import (
	"encoding/json"
	"testing"
)

// STAGE-209：装备耐久的纯函数测试（docs/durability.md 第 2 节的边界）。

// crushRnd 是固定返回 v 的随机数（battle_test.go 的 fixedRnd 是另一种签名，这里不冲突）。
func crushRnd(v int) func(n int) int { return func(n int) int { return v } }

func allEquipped(int) bool { return true }

func onlySlots(slots ...int) func(int) bool {
	return func(s int) bool {
		for _, x := range slots {
			if x == s {
				return true
			}
		}
		return false
	}
}

// 防守方选栏：rnd(100) 的 50/17/17/16 分布边界。
func TestPickDefenseSlotBoundaries(t *testing.T) {
	cases := []struct{ roll, want int }{
		{0, crushSlotBody}, {49, crushSlotBody},
		{50, crushSlotHead}, {66, crushSlotHead},
		{67, crushSlotAcc1}, {83, crushSlotAcc1},
		{84, crushSlotAcc2}, {99, crushSlotAcc2},
	}
	for _, c := range cases {
		if got := pickDefenseSlot(crushRnd(c.roll), allEquipped); got != c.want {
			t.Errorf("roll %d：选栏 %d，要 %d", c.roll, got, c.want)
		}
	}
}

// 选中的栏没装备就按 (栏+1)%5 往后找，跳过武器栏；全空返回 -1。
func TestPickDefenseSlotFallback(t *testing.T) {
	// 身体空：BODY(1) → ARM(2，跳过) → DEC1(3)。
	if got := pickDefenseSlot(crushRnd(0), onlySlots(crushSlotAcc1, crushSlotHead)); got != crushSlotAcc1 {
		t.Errorf("身体空应落到饰品1，得 %d", got)
	}
	// 饰品2 空：DEC2(4) → HEAD(0)。
	if got := pickDefenseSlot(crushRnd(90), onlySlots(crushSlotHead)); got != crushSlotHead {
		t.Errorf("饰品2 空应绕回头，得 %d", got)
	}
	// 只有武器：防守方不挑武器，返回 -1。
	if got := pickDefenseSlot(crushRnd(0), onlySlots(crushSlotHand)); got != -1 {
		t.Errorf("只有武器时防守方应得 -1，得 %d", got)
	}
	if got := pickDefenseSlot(crushRnd(0), func(int) bool { return false }); got != -1 {
		t.Errorf("全空应得 -1，得 %d", got)
	}
}

// 攻击方只看武器栏。
func TestPickAttackSlot(t *testing.T) {
	if got := pickAttackSlot(onlySlots(crushSlotHand, crushSlotBody)); got != crushSlotHand {
		t.Errorf("有武器应选武器栏，得 %d", got)
	}
	if got := pickAttackSlot(onlySlots(crushSlotBody)); got != -1 {
		t.Errorf("没武器应得 -1，得 %d", got)
	}
}

// 扣减基数（原版 breadnums），含整数除法向零截断与 <=0 取 1 的边界。
func TestCrushBreadths(t *testing.T) {
	weapon := []struct{ dmg, want int }{{49, 1}, {50, 1}, {89, 1}, {90, 1}, {130, 2}, {170, 3}}
	for _, c := range weapon {
		if got := crushBreadths(crushSlotHand, c.dmg); got != c.want {
			t.Errorf("武器 伤害 %d：基数 %d，要 %d", c.dmg, got, c.want)
		}
	}
	armor := []struct{ dmg, want int }{{0, 10}, {100, 5}, {180, 1}, {199, 0}, {200, 0}, {210, 0}, {300, 1}}
	for _, c := range armor {
		if got := crushBreadths(crushSlotBody, c.dmg); got != c.want {
			t.Errorf("防具 伤害 %d：基数 %d，要 %d", c.dmg, got, c.want)
		}
	}
}

// RAND(b, b*1.4) 的区间：b=0 时为 0，b=1 为 {1,2}，b=5 为 {5,6,7}。
func TestCrushRandRange(t *testing.T) {
	low := func(n int) int { return 0 }
	high := func(n int) int { return n - 1 }
	if got := crushRand(low, 0); got != 0 {
		t.Errorf("b=0 最小应为 0，得 %d", got)
	}
	if got := crushRand(low, 1); got != 1 {
		t.Errorf("b=1 最小应为 1，得 %d", got)
	}
	if got := crushRand(high, 1); got != 2 {
		t.Errorf("b=1 最大应为 2，得 %d", got)
	}
	if got := crushRand(high, 5); got != 7 {
		t.Errorf("b=5 最大应为 7，得 %d", got)
	}
	if got := crushRand(low, 5); got != 5 {
		t.Errorf("b=5 最小应为 5，得 %d", got)
	}
}

// 等级阈值 70/50/30。
func TestCrushLevelThresholds(t *testing.T) {
	cases := []struct{ defs, want int }{
		{100, 0}, {70, 0}, {69, 1}, {50, 1}, {49, 2}, {30, 2}, {29, 3}, {0, 3},
	}
	for _, c := range cases {
		if got := crushLevelOf(c.defs); got != c.want {
			t.Errorf("剩余 %d%%：等级 %d，要 %d", c.defs, got, c.want)
		}
	}
}

// 最大耐久为 0 的物品不损坏。
func TestApplyCrushNoMax(t *testing.T) {
	it := &EquipInput{Name: "旧物", Durability: 0, MaxDurability: 0}
	if notes := applyCrush(it, crushSlotBody, 100, crushRnd(0)); notes != nil || it.Durability != 0 {
		t.Errorf("最大耐久 0 不应损坏：%+v %v", it, notes)
	}
}

// 扣耐久后等级跨过 70% 线时提示一次（红色），同一等级不重复提示。
func TestApplyCrushLevelNote(t *testing.T) {
	it := &EquipInput{Name: "铁甲", Durability: 72, MaxDurability: 100}
	notes := applyCrush(it, crushSlotBody, 100, crushRnd(0)) // 基数 5，扣 5，剩 67%
	if it.Durability != 67 || it.CrushLevel != crushLevelDamaged {
		t.Fatalf("耐久 %d 等级 %d，要 67 / 1", it.Durability, it.CrushLevel)
	}
	if len(notes) != 1 || notes[0].Text != "铁甲是受损的。" || !notes[0].Red {
		t.Fatalf("提示 %+v，要一条红色的“铁甲是受损的。”", notes)
	}
	if again := applyCrush(it, crushSlotBody, 100, crushRnd(0)); again != nil {
		t.Errorf("同一等级不应再提示：%+v", again)
	}
}

// 耗尽：消失（黄色提示），标记 Gone。
func TestApplyCrushGone(t *testing.T) {
	it := &EquipInput{Name: "铁甲", Durability: 3, MaxDurability: 100}
	notes := applyCrush(it, crushSlotBody, 100, crushRnd(0))
	if !it.Gone || it.Durability != 0 || it.CrushLevel != crushLevelGone {
		t.Fatalf("应消失：%+v", it)
	}
	if len(notes) != 1 || notes[0].Text != "铁甲因过度损坏而消失。" || notes[0].Red {
		t.Fatalf("提示 %+v，要一条黄色的消失提示", notes)
	}
}

// 武器的扣减：伤害 50 基数 1，扣 1。
func TestApplyCrushWeapon(t *testing.T) {
	it := &EquipInput{Name: "斧", Durability: 200000, MaxDurability: 200000}
	applyCrush(it, crushSlotHand, 50, crushRnd(0))
	if it.Durability != 199999 {
		t.Errorf("武器耐久 %d，要 199999", it.Durability)
	}
}

// 协议字段名：battle_round 的 crush 每项是 {"text","red"}，客户端按小写反序列化（docs/protocol.md 10.7，STAGE-225）。
func TestCrushNoteJSONKeys(t *testing.T) {
	b, err := json.Marshal([]CrushNote{{Text: "铁甲是受损的。", Red: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `[{"text":"铁甲是受损的。","red":true}]`; got != want {
		t.Fatalf("crush 的 JSON 是 %s，要 %s", got, want)
	}
}
