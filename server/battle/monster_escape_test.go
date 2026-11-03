package battle

import "testing"

// STAGE-176：怪物自己逃跑（AI 选了 escape）时，只有这只怪物离场；我方人物和宠物不跟着走，战斗也不因此结束。
func TestMonsterEscapeLeavesOnlyItself(t *testing.T) {
	b := newTestBattle(t, "escape", chanceRnd()) // 逃跑必成功
	mon := firstMonster(t, b)

	ev := b.monsterEscape(mon)
	if ev.OK == nil || !*ev.OK {
		t.Fatalf("怪物逃跑应成功：%+v", ev)
	}
	if len(ev.Left) != 1 || ev.Left[0] != mon.No {
		t.Fatalf("怪物逃跑的离场 = %v，want [%d]（不能带走我方单位）", ev.Left, mon.No)
	}
	if b.over {
		t.Fatal("怪物逃跑不应结束战斗")
	}
	if b.Unit(0) == nil || b.Unit(5) == nil {
		t.Fatal("人物和宠物应留在场上")
	}
}

// 敌人全部逃走：战斗按 draw 收场（没分胜负），不是 escaped（自己逃离）。
func TestAllMonstersEscapeIsDraw(t *testing.T) {
	b := newTestBattle(t, "escape", chanceRnd())
	for {
		mon := firstMonsterOrNil(b)
		if mon == nil {
			break
		}
		if ev := b.monsterEscape(mon); ev.OK == nil || !*ev.OK {
			t.Fatalf("怪物逃跑应成功：%+v", ev)
		}
	}
	end := b.finish()
	if end == nil || end.Result != "draw" {
		t.Fatalf("敌人全部逃走应判 draw，实际 %+v", end)
	}
}

func firstMonster(t *testing.T, b *Battle) *Unit {
	t.Helper()
	if m := firstMonsterOrNil(b); m != nil {
		return m
	}
	t.Fatal("场上没有怪物")
	return nil
}

func firstMonsterOrNil(b *Battle) *Unit {
	for no := 10; no < 20; no++ {
		if u := b.Unit(no); u != nil && u.Kind == KindMonster {
			return u
		}
	}
	return nil
}

// STAGE-193：一只怪逃走、其余被打死：结算是胜利（逃走的不算存活的敌人），不是平局。
func TestFledMonsterAndKilledRestIsWin(t *testing.T) {
	b := newTestBattle(t, "escape", chanceRnd())
	fled := firstMonster(t, b)
	if ev := b.monsterEscape(fled); ev.OK == nil || !*ev.OK {
		t.Fatalf("怪物逃跑应成功：%+v", ev)
	}
	for rest := firstMonsterOrNil(b); rest != nil; rest = firstMonsterOrNil(b) { // 剩下的全部打死
		rest.HP = 0
		b.slain = append(b.slain, rest)
		b.leave(rest) // 死掉的单位在结算里不再占位
	}
	if len(b.slain) == 0 {
		t.Fatal("应当有被打死的怪")
	}
	end := b.finish()
	if end == nil || end.Result != "win" {
		t.Fatalf("一只逃走、其余打死应判 win，实际 %+v", end)
	}
}
