package battle

import "testing"

// STAGE-209：装备耐久接入 physHurt 的集成测试（直接调 physHurt，不走整回合）。

func crushBattle(t *testing.T, armorDur, armorMax int) (*Battle, *Unit, *Unit) {
	t.Helper()
	side := testSide()
	side.Equips[crushSlotBody] = &EquipInput{Slot: crushSlotBody, Name: "铁甲", Durability: armorDur, MaxDurability: armorMax}
	side.Equips[crushSlotHand] = &EquipInput{Slot: crushSlotHand, Name: "斧", Durability: 200000, MaxDurability: 200000}
	ms := monsters(1)
	b := New("b_crush", "test", side, ms, func(int) int { return 0 })
	return b, b.PlayerUnit(), ms[0]
}

// 怪物打人物：防具和（人物的）武器耐久都要下降，范围按 crushBreadths 与 crushRand 的区间。
func TestItemCrushOnMonsterHit(t *testing.T) {
	b, you, m := crushBattle(t, 200, 200)
	var ev Event
	b.physHurt(m, you, 40, "hit", &ev)
	st := b.EquipState()
	// 防具：伤害 40 → 基数 (200-40)/20 = 8，扣 [8, 8+ceil(3.2)] = [8, 12]。
	if d := 200 - st[crushSlotBody].Durability; d < 8 || d > 12 {
		t.Errorf("防具扣了 %d，要在 [8,12]", d)
	}
	// 武器：这里是人物挨打，武器不扣（攻击方是怪物，没有 crushOwner）。
	if st[crushSlotHand].Durability != 200000 {
		t.Errorf("怪物攻击时人物武器不应扣：%d", st[crushSlotHand].Durability)
	}
	// 人物打怪物：武器扣，防具不扣。伤害 40 → 武器基数 1，扣 [1,2]。
	before := st[crushSlotBody].Durability
	b.physHurt(you, m, 40, "hit", &ev)
	st = b.EquipState()
	if d := 200000 - st[crushSlotHand].Durability; d < 1 || d > 2 {
		t.Errorf("武器扣了 %d，要在 [1,2]", d)
	}
	if st[crushSlotBody].Durability != before {
		t.Errorf("人物攻击时防具不应再扣：%d -> %d", before, st[crushSlotBody].Durability)
	}
}

// 伤害为 0 不结算（原版 damage > 0 才调用）。
func TestItemCrushSkipsZeroDamage(t *testing.T) {
	b, you, m := crushBattle(t, 200, 200)
	var ev Event
	b.physHurt(m, you, 0, "hit", &ev)
	if st := b.EquipState(); st[crushSlotBody].Durability != 200 || len(ev.Crush) != 0 {
		t.Errorf("伤害 0 不应扣耐久：%+v %+v", st[crushSlotBody], ev.Crush)
	}
}

// 受损等级跨过 70% 线时事件里带红色提示。
func TestItemCrushLevelNoteOnEvent(t *testing.T) {
	b, you, m := crushBattle(t, 72, 100)
	var ev Event
	b.physHurt(m, you, 100, "hit", &ev) // 基数 5，扣 5–7，剩 65–67%
	if len(ev.Crush) != 1 || ev.Crush[0].Text != "铁甲是受损的。" || !ev.Crush[0].Red {
		t.Fatalf("提示 %+v，要一条红色的“铁甲是受损的。”", ev.Crush)
	}
}

// 耗尽：标记 Gone，事件里有黄色的消失提示；原来的 SideInput 快照不变。
func TestItemCrushGoneOnEvent(t *testing.T) {
	b, you, m := crushBattle(t, 3, 100)
	var ev Event
	b.physHurt(m, you, 100, "hit", &ev)
	st := b.EquipState()
	if !st[crushSlotBody].Gone || len(ev.Crush) != 1 || ev.Crush[0].Text != "铁甲因过度损坏而消失。" || ev.Crush[0].Red {
		t.Fatalf("应消失：state=%+v notes=%+v", st[crushSlotBody], ev.Crush)
	}
	if b.sides[0].Equips[crushSlotBody].Durability != 3 {
		t.Errorf("SideInput 快照不应被改：%+v", b.sides[0].Equips[crushSlotBody])
	}
}

// 怪物不损坏：怪物没有装备耐久，人物打它也只扣人物自己的武器。
func TestItemCrushSkipsMonsters(t *testing.T) {
	b, you, m := crushBattle(t, 200, 200)
	var ev Event
	b.physHurt(you, m, 40, "hit", &ev)
	if m.crushOwner || m.crush[crushSlotBody] != nil {
		t.Errorf("怪物不应有装备耐久")
	}
	if st := b.EquipState(); st[crushSlotBody].Durability != 200 {
		t.Errorf("人物攻击时防具不应扣：%+v", st[crushSlotBody])
	}
}
