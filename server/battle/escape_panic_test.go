package battle

import "testing"

// STAGE-252：人物逃跑成功后，本回合剩下的单位（出战宠物出手、中毒跳血）不能再对“已离场的人物”取阵营。
// 路径 (a)：人物逃跑成功后，出战宠物还要出手。Settle 在人物离场后会停止行动（resolve.go 的 sideGone 判断），
// 这里直接调 act 让宠物出手，模拟“逃跑之后仍有单位行动”，确认 effect/hurt 等处不会 panic。
func TestEscapeThenPetActsNoPanic(t *testing.T) {
	b := newTestBattle(t, "escape", chanceRnd())
	mon := firstMonster(t, b)
	pet := b.PetUnit()
	b.Submit(b.Turn(), "player", "escape", Cmd{})
	b.Submit(b.Turn(), "pet", "attack", Cmd{Target: mon.No})
	if ev := b.escape(b.PlayerUnit()); ev.OK == nil || !*ev.OK {
		t.Fatalf("逃跑应成功：%+v", ev)
	}
	if b.PlayerUnit() != nil {
		t.Fatal("逃跑后人物应已离场")
	}
	b.act(pet) // 离场后的宠物出手：攻击或被攻击都不能对“没有人物”的阵营取值
}

// STAGE-252 路径 (b)：人物逃跑后，回合末 statusTick 让中毒的怪物跳血，走到 hurt。
func TestEscapeThenPoisonTickNoPanic(t *testing.T) {
	b := newTestBattle(t, "escape", chanceRnd())
	firstMonster(t, b).Status.Poison = 5
	b.Submit(b.Turn(), "player", "escape", Cmd{})
	b.Submit(b.Turn(), "pet", "guard", Cmd{})
	_, end := b.Settle()
	if end == nil || end.Result != "escaped" {
		t.Fatalf("end = %+v, want escaped", end)
	}
}
