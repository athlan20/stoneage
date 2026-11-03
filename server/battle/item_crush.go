package battle

// STAGE-209：装备耐久（原版“道具损坏”，battle_event.c 的 _TAKE_ITEMDAMAGE 分支，docs/durability.md）。
// 这里是纯函数：挑哪件装备、扣多少、等级怎么变、消失怎么处理。随机数由调用方注入（rnd(n) 返回 [0,n)）。
// 只有人物的装备损坏，宠物和怪物不进这里；接入战斗结算的部分在 item_crush_hook.go。

// 装备栏位（与原版 CHAR_HEAD/BODY/ARM/DECORATION1/DECORATION2 的顺序一致，见 gamedata/equip.go）。
const (
	crushSlotHead  = 0
	crushSlotBody  = 1
	crushSlotHand  = 2 // 武器（原版 CHAR_ARM）
	crushSlotAcc1  = 3
	crushSlotAcc2  = 4
	crushSlotCount = 5
)

// 受损等级（原版 aszCrushTbl）：完好、受损、毁坏、不堪使用，消失时记 4（碎片，只在结算中用）。
const (
	crushLevelOK      = 0
	crushLevelDamaged = 1
	crushLevelBroken  = 2
	crushLevelWorn    = 3
	crushLevelGone    = 4
)

var crushLevelText = [...]string{"完好", "受损", "毁坏", "不堪使用", "碎片"}

// EquipInput 是一件装备的耐久状态。Durability 为当前耐久，MaxDurability 为 0 的不损坏。
type EquipInput struct {
	Slot          int
	Name          string
	Durability    int
	MaxDurability int
	CrushLevel    int
	Gone          bool // 本场战斗中耗尽被删除
}

// CrushNote 是结算时要发给人物的系统消息（原版 CHAR_talkToCli）。Red 为真用红色，否则黄色。
type CrushNote struct {
	Text string `json:"text"`
	Red  bool   `json:"red"`
}

// pickDefenseSlot 按原版 BATTLE_ItemCrushCheck(flg=1) 挑防守方的装备栏：rnd(100) 落在
// 0–49 身体、50–66 头、67–83 饰品1、84–99 饰品2；选中的栏没装备就按 (栏+1)%5 往后找，
// 最多 5 次，跳过武器栏。找不到返回 -1。has 报告某栏有没有装备。
func pickDefenseSlot(rnd func(n int) int, has func(slot int) bool) int {
	roll := rnd(100)
	var slot int
	switch {
	case roll < 50:
		slot = crushSlotBody
	case roll < 67:
		slot = crushSlotHead
	case roll < 84:
		slot = crushSlotAcc1
	default:
		slot = crushSlotAcc2
	}
	for i := 0; i < crushSlotCount; i++ {
		if has(slot) && slot != crushSlotHand {
			return slot
		}
		slot = (slot + 1) % crushSlotCount
	}
	return -1
}

// pickAttackSlot 按原版 BATTLE_ItemCrushCheck(flg=2)：攻击方只看武器栏，有武器就是它。
func pickAttackSlot(has func(slot int) bool) int {
	if has(crushSlotHand) {
		return crushSlotHand
	}
	return -1
}

// crushBreadths 是扣减的基数（原版 BATTLE_ItemCrush 的 breadnums）。
// 武器：(伤害-50)/40，<=0 取 1。防具：(200-伤害)/20，<0 取 1。
// 原版在 `if(Damages>=200) breadnums=1;` 之后紧接着又赋值，前一句是死代码，这里照搬结果，不实现它。
// 整数除法同 C 一样向零截断，所以 200<伤害<220 时防具基数为 0。
func crushBreadths(slot, damage int) int {
	if slot == crushSlotHand {
		if b := (damage - 50) / 40; b > 0 {
			return b
		}
		return 1
	}
	if b := (200 - damage) / 20; b >= 0 {
		return b
	}
	return 1
}

// crushRand 是原版 RAND(b, b*1.4) 宏的结果：b + (int)((1.4b-(b-1)) * rand()/(RAND_MAX+1))，
// 即 [b, b+ceil(0.4b)] 之内的整数（b=0 时为 0）。rnd(n) 给出 [0,n) 的均匀整数，这里用 2^30 档细分成 u∈[0,1)。
func crushRand(rnd func(n int) int, b int) int {
	const steps = 1 << 30
	u := float64(rnd(steps)) / steps
	return b + int((1.4*float64(b)-float64(b-1))*u)
}

// crushLevelOf 按剩余耐久比例定等级（原版 defs = 耐久*100/最大耐久）：>=70 完好，50–69 受损，30–49 毁坏，<30 不堪使用。
func crushLevelOf(defs int) int {
	switch {
	case defs >= 70:
		return crushLevelOK
	case defs >= 50:
		return crushLevelDamaged
	case defs >= 30:
		return crushLevelBroken
	default:
		return crushLevelWorn
	}
}

// applyCrush 对一件装备扣耐久并定等级（原版 BATTLE_ItemCrush 的扣减部分）。
// 最大耐久不足 1 的物品不损坏；耐久耗尽则标记 Gone 并给出“消失”消息（黄色）；
// 否则等级变了且剩余不到 70% 时更新等级并给出“是X的”消息（红色）。
func applyCrush(it *EquipInput, slot, damage int, rnd func(n int) int) []CrushNote {
	if it == nil || it.Gone || it.MaxDurability < 1 {
		return nil
	}
	amount := crushRand(rnd, crushBreadths(slot, damage))
	it.Durability -= amount
	if it.Durability <= 0 {
		it.Durability = 0
		it.Gone = true
		it.CrushLevel = crushLevelGone
		return []CrushNote{{Text: it.Name + "因过度损坏而消失。"}}
	}
	defs := it.Durability * 100 / it.MaxDurability
	level := crushLevelOf(defs)
	if level != it.CrushLevel && defs < 70 {
		it.CrushLevel = level
		return []CrushNote{{Text: it.Name + "是" + crushLevelText[level] + "的。", Red: true}}
	}
	return nil
}
