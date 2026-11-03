// STAGE-201 精灵的魔障、沉默、虚弱（对照 gmsv/battle/battle_event.c 的 StatusTbl、battle.c 的 BATTLE_StatusSeq、
// battle_magic.c 的 BATTLE_MultiStatusRecovery）：状态字段与解除规则、挂上的回合数、冻结、沉默与魔障的效果。

package battle

import (
	"reflect"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 三种新状态进协议列表（10.2），顺序在乱之后、防御之前。
func TestNewStatusListNames(t *testing.T) {
	s := StatusSet{Weaken: 4, Barrier: 2, Nocast: 3}
	want := []string{"weaken", "barrier", "nocast"}
	if got := s.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("status 列表应为 %v，实际 %v", want, got)
	}
	if !s.anyBad() {
		t.Fatalf("带着新状态应算作异常（StatusAttackCheck 的“已有异常”）")
	}
}

// 净化（cure，道具与精灵共用的名字解法）：点名能解三种新状态；“全”不解它们（原版“全”只到乱为止）。
func TestNewStatusCureByName(t *testing.T) {
	s := StatusSet{Poison: 2, Weaken: 4, Barrier: 2, Nocast: 3}
	if got := s.cure("all"); !reflect.DeepEqual(got, []string{"poison"}) {
		t.Fatalf("全 只应解毒，实际 %v", got)
	}
	if s.Weaken != 4 || s.Barrier != 2 || s.Nocast != 3 {
		t.Fatalf("全 不应动新状态: %+v", s)
	}
	for name, field := range map[string]*int{"weaken": &s.Weaken, "barrier": &s.Barrier, "nocast": &s.Nocast} {
		if got := s.cure(name); !reflect.DeepEqual(got, []string{name}) || *field != 0 {
			t.Fatalf("点名 %s 应解掉，实际 %v，字段 %d", name, got, *field)
		}
	}
}

// 解状态精灵（cureMagic）：按 cureOrder 取最后一个有回合数的状态，精灵要正好是它；
// “全”只看前六种，所以带着沉默等新状态时“全”没有效果（原版 tostatus <= CHAR_WORKCONFUSION）。
func TestNewStatusCureMagicRule(t *testing.T) {
	cases := []struct {
		name string
		s    StatusSet
		want string
		got  []string
	}{
		{"只有沉默，点名沉默", StatusSet{Nocast: 3}, "nocast", []string{"nocast"}},
		{"毒加沉默，沉默是最后一种，点名毒解不掉", StatusSet{Poison: 2, Nocast: 3}, "poison", nil},
		{"毒加沉默，点名沉默解掉", StatusSet{Poison: 2, Nocast: 3}, "nocast", []string{"nocast"}},
		{"毒加沉默，全 不解（最后一种超过乱）", StatusSet{Poison: 2, Nocast: 3}, "all", nil},
		{"只有虚弱，全 不解", StatusSet{Weaken: 4}, "all", nil},
		{"只有虚弱，点名虚弱解掉", StatusSet{Weaken: 4}, "weaken", []string{"weaken"}},
		{"只有毒，全 照常解", StatusSet{Poison: 2}, "all", []string{"poison"}},
	}
	for _, c := range cases {
		s := c.s
		if got := s.cureMagic(c.want); !reflect.DeepEqual(got, c.got) {
			t.Errorf("%s：cureMagic(%s) 应为 %v，实际 %v", c.name, c.want, c.got, got)
		}
	}
}

// debuffTemplate 是三种状态精灵的模板（精灵表的参数：障 turn 1 成 50、虚 turn 3 成 50、默 turn 3 成 50）。
func debuffTemplate(status string, turns int) gamedata.MagicTemplate {
	return gamedata.MagicTemplate{Func: gamedata.MagicFuncStatus, Status: status, Turns: turns, Success: 50}
}

// 挂上的回合数：魔障、虚弱是精灵的 turn + 1，沉默照 turn（MAGIC_Barrier、MAGIC_Weaken 与 MAGIC_Nocast 的差别）。
func TestDebuffTurnsForTargetAndCaster(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	you := b.PlayerUnit()
	cases := []struct {
		status string
		turns  int
		want   int
	}{
		{"barrier", 1, 2},
		{"weaken", 3, 4},
		{"nocast", 3, 3},
	}
	for _, c := range cases {
		m := sturdy(b, 15)
		m.Status = StatusSet{}
		ev := Event{Kind: "magic"}
		b.magicStatus(you, m, debuffTemplate(c.status, c.turns), &ev)
		got := map[string]int{"barrier": m.Status.Barrier, "weaken": m.Status.Weaken, "nocast": m.Status.Nocast}[c.status]
		if got != c.want {
			t.Errorf("%s：挂上回合应为 %d，实际 %d", c.status, c.want, got)
		}
		if eff := effectOn(ev, 15); eff == nil || eff.Result != "none" || !reflect.DeepEqual(eff.StatusAdd, []string{c.status}) {
			t.Errorf("%s：应有挂上的效果: %+v", c.status, ev.Effects)
		}
	}
}

// 沉默不中宠物（MAGIC_Nocast 的 CHAR_TYPEPET 排除）：判定照常做，宠物记 miss、不挂状态；怪物照中。
func TestNocastSkipsPetTarget(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	you, pet := b.PlayerUnit(), b.PetUnit()
	sturdy(b, pet.No)
	ev := Event{Kind: "magic"}
	b.magicStatus(you, pet, debuffTemplate("nocast", 3), &ev)
	if pet.Status.Nocast != 0 {
		t.Fatalf("宠物不应中沉默: %+v", pet.Status)
	}
	if eff := effectOn(ev, pet.No); eff == nil || eff.Result != "miss" {
		t.Fatalf("宠物应记 miss: %+v", ev.Effects)
	}
}

// 已经带着异常状态（这里是毒）的目标，魔障、沉默、虚弱都不中。
func TestDebuffSkipsTargetWithStatus(t *testing.T) {
	for _, status := range []string{"barrier", "weaken", "nocast"} {
		b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
		m := sturdy(b, 15)
		m.Status.Poison = 2
		ev := Event{Kind: "magic"}
		b.magicStatus(b.PlayerUnit(), m, debuffTemplate(status, 3), &ev)
		if eff := effectOn(ev, 15); eff == nil || eff.Result != "miss" {
			t.Errorf("%s：已中毒的目标应 miss: %+v", status, ev.Effects)
		}
		if m.Status.Poison != 2 || m.Status.Barrier+m.Status.Weaken+m.Status.Nocast != 0 {
			t.Errorf("%s：状态不应改变: %+v", status, m.Status)
		}
	}
}

// 魔障（障）：带着魔障的单位不能行动（BATTLE_CanMoveCheck），怪物这回合记 skip，原因是 barrier。
func TestBarrierStopsAction(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	m := sturdy(b, 15)
	m.Status.Barrier = 2
	if m.Status.CanAct() {
		t.Fatalf("带着魔障不应能行动")
	}
	evs := b.act(m)
	if len(evs) != 1 || evs[0].Kind != "skip" || evs[0].Reason != "barrier" {
		t.Fatalf("应记一个 barrier 的 skip: %+v", evs)
	}
}

// 带着魔障的忠犬不替主人挡伤害（BATTLE_GuardianCheck 的 CHAR_WORKBARRIER）。
func TestBarrierPetDoesNotGuard(t *testing.T) {
	b := skillTestBattle(t, 20)
	pet, boar, owner := b.PetUnit(), b.Unit(15), b.PlayerUnit()
	if _, perr := b.Submit(b.Turn(), "player", "guard", Cmd{}); perr != nil {
		t.Fatalf("人物防御提交失败: %v", perr)
	}
	if _, perr := b.Submit(b.Turn(), "pet", "skill", Cmd{Skill: 1, Target: 15}); perr != nil {
		t.Fatalf("忠犬提交失败: %v", perr)
	}
	b.beginRound()
	if g := b.guardianOf(boar, owner); g != pet {
		t.Fatalf("没有魔障时忠犬应替挡，实际 %v", g)
	}
	pet.Status.Barrier = 2
	if g := b.guardianOf(boar, owner); g != nil {
		t.Fatalf("带着魔障的忠犬不应替挡，实际 %v", g)
	}
}

// 沉默（默）：人物带着沉默时交 magic 回复 MAGIC_UNUSABLE；攻击、防御照常（物品、宠物技能不受影响）。
func TestNocastRejectsMagicOnly(t *testing.T) {
	b := newBattleWithSide(t, magicSide(130), "win", rollRnd(5))
	you := b.PlayerUnit()
	you.Status.Nocast = 3
	_, perr := b.Submit(b.Turn(), "player", "magic", Cmd{Magic: 1, Target: 15})
	expectCode(t, perr, "MAGIC_UNUSABLE")
	_, perr = b.Submit(b.Turn(), "player", "attack", Cmd{Target: 15})
	expectCode(t, perr, "")
}

// 沉默只挡精灵：道具（普通的肉，槽位 5）照常能提交。
func TestNocastAllowsItem(t *testing.T) {
	side := magicSide(130)
	side.Items[5] = BattleItem{ID: 1235, Name: "普通的肉"}
	b := newBattleWithSide(t, side, "win", rollRnd(5))
	b.PlayerUnit().Status.Nocast = 3
	_, perr := b.Submit(b.Turn(), "player", "item", Cmd{Slot: 5})
	expectCode(t, perr, "")
}

// 回合末冻结（BATTLE_StatusSeq）：虚弱挂着，连过 5 个回合仍是 4；同时中毒的毒的回合数也卡在原值。
func TestWeakenFreezesTurnsOfAllStatuses(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	m := sturdy(b, 15)
	m.HP, m.MaxHP = 500, 500
	m.Status.Weaken, m.Status.Poison = 4, 3
	for i := 0; i < 5; i++ {
		b.statusTick()
	}
	if m.Status.Weaken != 4 || m.Status.Poison != 3 {
		t.Fatalf("虚弱和毒都应冻在原值: %+v", m.Status)
	}
}

// 魔障挂着也冻住：回合数不减，单位一直不能行动。
func TestBarrierFreezesAndBlocksAction(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	m := sturdy(b, 15)
	m.Status.Barrier = 2
	for i := 0; i < 5; i++ {
		b.statusTick()
	}
	if m.Status.Barrier != 2 || m.Status.CanAct() {
		t.Fatalf("魔障应冻在 2 且不能行动: %+v", m.Status)
	}
}

// 冻住的单位计数减到 0 时照样记 status_remove（原版给客户端的“解除”），内部值仍是原值。
func TestFrozenUnitStillReportsRemoval(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	m := sturdy(b, 15)
	m.Status.Weaken, m.Status.Confusion = 4, 1
	ev := Event{Kind: "status_tick"}
	any := false
	b.tickDown(m, &ev, &any)
	if m.Status.Confusion != 1 || !any {
		t.Fatalf("冻住的混乱应保持 1: %+v", m.Status)
	}
	eff := effectOn(ev, 15)
	if eff == nil || len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "confusion" {
		t.Fatalf("应记混乱的 status_remove: %+v", ev.Effects)
	}
}

// 沉默不冻结：正常每回合减一，3 回合后自动解除。
func TestNocastExpiresAfterThreeRounds(t *testing.T) {
	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	m := sturdy(b, 15)
	m.Status.Nocast = 3
	want := []int{2, 1, 0}
	for i, w := range want {
		b.statusTick()
		if m.Status.Nocast != w {
			t.Fatalf("第 %d 回合末沉默应为 %d，实际 %d", i+1, w, m.Status.Nocast)
		}
	}
}

// 净化精灵解掉对应的新状态：虚（408）、障（404）、默（406、411）；高等净化（60，“全”）解不掉魔障。
// 沉默的人提交不了精灵（上面的测试），这里直接结算 castMagic，测的是净化本身的效果。
func TestCureSpellsRemoveNewStatuses(t *testing.T) {
	cases := []struct {
		name  string
		magic int
		set   func(s *StatusSet)
		check func(s StatusSet) bool
	}{
		{"净化虚弱(单人)", 408, func(s *StatusSet) { s.Weaken = 4 }, func(s StatusSet) bool { return s.Weaken == 0 }},
		{"净化魔障(单人)", 404, func(s *StatusSet) { s.Barrier = 2 }, func(s StatusSet) bool { return s.Barrier == 0 }},
		{"净化沉默(单人)", 406, func(s *StatusSet) { s.Nocast = 3 }, func(s StatusSet) bool { return s.Nocast == 0 }},
		{"破除沉默(全体)", 411, func(s *StatusSet) { s.Nocast = 3 }, func(s StatusSet) bool { return s.Nocast == 0 }},
	}
	for _, c := range cases {
		b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
		you := b.PlayerUnit()
		c.set(&you.Status)
		tpl, ok := gamedata.MagicByID(c.magic)
		if !ok {
			t.Fatalf("%s：编号 %d 不在表里", c.name, c.magic)
		}
		b.castMagic(you, tpl, Cmd{Target: -1})
		if !c.check(you.Status) {
			t.Errorf("%s：没有解掉: %+v", c.name, you.Status)
		}
	}

	b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
	you := b.PlayerUnit()
	you.Status.Barrier = 2
	tpl, _ := gamedata.MagicByID(60)
	b.castMagic(you, tpl, Cmd{Target: -1})
	if you.Status.Barrier != 2 {
		t.Fatalf("高等净化（全）不应解掉魔障: %+v", you.Status)
	}
}

// 魔障（397）：敌方全体，挂上 2 回合（turn 1 + 1）；带着异常状态的目标不中；挂上的怪物这回合不能行动，
// 净化魔障（404）解掉后能行动。
func TestBarrierSpellHitsEnemySideAndBlocks(t *testing.T) {
	b := newBattleWithSide(t, magicSide(397), "win", rollRnd(5))
	m1, m2 := sturdy(b, 15), sturdy(b, 16)
	m2.Status.Poison = 2
	you := b.PlayerUnit()
	if _, perr := b.Submit(b.Turn(), "player", "magic", Cmd{Magic: 1, Target: -1}); perr != nil {
		t.Fatalf("魔障应可以不带目标施放: %v", perr)
	}
	b.act(you)
	if m1.Status.Barrier != 2 || m1.Status.CanAct() {
		t.Fatalf("敌方第一只应挂上魔障 2 回合: %+v", m1.Status)
	}
	if m2.Status.Barrier != 0 {
		t.Fatalf("已中毒的敌人不应中魔障: %+v", m2.Status)
	}
	if evs := b.act(m1); len(evs) != 1 || evs[0].Kind != "skip" || evs[0].Reason != "barrier" {
		t.Fatalf("带魔障的怪物这回合应 skip: %+v", evs)
	}
	tpl, _ := gamedata.MagicByID(404)
	b.castMagic(you, tpl, Cmd{Target: -1})
	if you.Status.Barrier != 0 {
		t.Fatalf("净化魔障应解掉魔障: %+v", you.Status)
	}
	if !you.Status.CanAct() {
		t.Fatalf("解掉魔障后应能行动")
	}
}
