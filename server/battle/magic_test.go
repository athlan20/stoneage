// 精灵的状态、解状态与复活（STAGE-77，对照 gmsv/battle/battle_magic.c 的 MAGIC_StatusChange_Battle、
// MAGIC_StatusRecovery_Battle、MAGIC_Ressurect_Battle）的引擎测试：成功率、挂上与解除的规则、
// 目标校验、复活的回复量，以及醉（闪避）和混乱（随机目标）对怪物的影响。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// magicSide 在 testSide 的基础上把精灵放进栏位 1 起的几格（栏位 0 是岩石撞击）。
func magicSide(ids ...int) *SideInput {
	side := testSide()
	for i, id := range ids {
		side.Magics[i+1] = id
	}
	return side
}

// sturdy 把怪物的抗性固定成体力占比 0.25（抗性 10）、等级与玩家相同，状态精灵的成功率就是 偏移 − 10。
func sturdy(b *Battle, no int) *Unit {
	m := b.Unit(no)
	m.Vital, m.Strength, m.Toughness, m.Dexterity = 10, 10, 10, 10
	m.Level = b.PlayerUnit().Level
	return m
}

// effectOn 取事件里对某个单位的效果；没有时返回 nil。
func effectOn(ev Event, target int) *Effect {
	for i := range ev.Effects {
		if ev.Effects[i].Target == target {
			return &ev.Effects[i]
		}
	}
	return nil
}

func expectCode(t *testing.T, err *protocol.Error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("应接受，实际 %s（%s）", err.Code, err.Message)
		}
		return
	}
	if err == nil || err.Code != want {
		t.Fatalf("错误码应为 %s，实际 %+v", want, err)
	}
}

// 猛毒的精灵 Lv1（130）：单人，中毒 3 回合，成功率偏移 20；怪物抗性 10、等级相同时 per = 10，
// rollRnd(5) 让 RAND(1,100) 出 6，中；rollRnd(50) 出 51，不中。
func TestStatusSpellPoisonsEnemy(t *testing.T) {
	b := newBattleWithSide(t, magicSide(130), "win", rollRnd(5))
	m := sturdy(b, 15)
	you := b.PlayerUnit()
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15})
	expectCode(t, perr, "")
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "magic" || evs[0].Name != "猛毒的精灵 Lv1" {
		t.Fatalf("应产生一个 magic 事件: %+v", evs)
	}
	eff := effectOn(evs[0], 15)
	if eff == nil || eff.Result != "none" || len(eff.StatusAdd) != 1 || eff.StatusAdd[0] != "poison" {
		t.Fatalf("应有挂上中毒的效果: %+v", evs[0].Effects)
	}
	if m.Status.Poison != 3 {
		t.Fatalf("中毒回合应为精灵的 3: %+v", m.Status)
	}
	if you.MP != 100-4 {
		t.Fatalf("气力应扣 4（猛毒 Lv1）: %d", you.MP)
	}
}

func TestStatusSpellMissesWhenRollFails(t *testing.T) {
	b := newBattleWithSide(t, magicSide(130), "win", rollRnd(50))
	m := sturdy(b, 15)
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15}); perr != nil {
		t.Fatalf("提交被拒: %v", perr)
	}
	evs := b.act(b.PlayerUnit())
	eff := effectOn(evs[0], 15)
	if eff == nil || eff.Result != "miss" || len(eff.StatusAdd) != 0 {
		t.Fatalf("没中应记 miss 且不挂状态: %+v", evs[0].Effects)
	}
	if m.Status.Poison != 0 {
		t.Fatalf("没中不应中毒: %+v", m.Status)
	}
}

// 已经带着任何异常状态的目标不会再中（BATTLE_StatusAttackCheck 一开始就返回 FALSE）。
func TestStatusSpellSkipsTargetWithStatus(t *testing.T) {
	b := newBattleWithSide(t, magicSide(130), "win", rollRnd(5))
	m := sturdy(b, 15)
	m.Status.Sleep = 2
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15}); perr != nil {
		t.Fatalf("提交被拒: %v", perr)
	}
	evs := b.act(b.PlayerUnit())
	if eff := effectOn(evs[0], 15); eff == nil || eff.Result != "miss" {
		t.Fatalf("已睡着的目标应 miss: %+v", evs[0].Effects)
	}
	if m.Status.Poison != 0 || m.Status.Sleep != 2 {
		t.Fatalf("状态不应改变: %+v", m.Status)
	}
}

// 毒雾的精灵 Lv6（412）：一方全体，打的是对方全体，己方一个都不碰；中毒 6 回合。
func TestStatusSpellHitsEnemySideOnly(t *testing.T) {
	b := newBattleWithSide(t, magicSide(412), "win", rollRnd(5))
	m1, m2 := sturdy(b, 15), sturdy(b, 16)
	you, pet := b.PlayerUnit(), b.PetUnit()
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: -1}); perr != nil {
		t.Fatalf("全体的精灵不带目标应被接受: %v", perr)
	}
	evs := b.act(you)
	for _, m := range []*Unit{m1, m2} {
		if m.Status.Poison != 6 {
			t.Fatalf("敌方 %d 应中毒 6 回合: %+v", m.No, m.Status)
		}
	}
	if you.Status.Poison != 0 || pet.Status.Poison != 0 {
		t.Fatalf("己方不应中毒: 人物 %+v 宠物 %+v", you.Status, pet.Status)
	}
	if len(evs[0].Effects) != 2 {
		t.Fatalf("应只有两个敌方效果: %+v", evs[0].Effects)
	}
}

func TestStatusSubmitRejectsWrongTargets(t *testing.T) {
	b := newBattleWithSide(t, magicSide(130), "win", rollRnd(5))
	sturdy(b, 15)
	cases := []struct {
		name   string
		target int
		want   string
	}{
		{"己方人物", 0, protocol.CodeInvalidTarget},
		{"己方宠物", 5, protocol.CodeInvalidTarget},
		{"没有单位的格子", 3, protocol.CodeInvalidTarget},
		{"敌方", 15, ""},
	}
	for _, tc := range cases {
		_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: tc.target})
		if tc.want == "" {
			expectCode(t, perr, "")
			b.cmds["player"] = nil // 撤销，便于后面的用例继续
			continue
		}
		expectCode(t, perr, tc.want)
	}
	b.Unit(15).Down = true
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15})
	expectCode(t, perr, protocol.CodeInvalidTarget)
}

// 净化精灵(毒)Lv1（70）：单人，解毒。只解己方，对没有中毒的目标没有效果。
func TestCureSpellRemovesPoisonFromAlly(t *testing.T) {
	b := newBattleWithSide(t, magicSide(70), "win", rollRnd(9999))
	you := b.PlayerUnit()
	you.Status.Poison = 3
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 0}); perr != nil {
		t.Fatalf("解自己的毒应被接受: %v", perr)
	}
	evs := b.act(you)
	eff := effectOn(evs[0], 0)
	if eff == nil || len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "poison" {
		t.Fatalf("应有解毒的效果: %+v", evs[0].Effects)
	}
	if you.Status.Poison != 0 {
		t.Fatalf("毒应被解除: %+v", you.Status)
	}
	enemy := newBattleWithSide(t, magicSide(70), "win", rollRnd(9999))
	_, perr := enemy.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15})
	expectCode(t, perr, protocol.CodeInvalidTarget)
}

func TestCureSpellWithoutMatchingStatusDoesNothing(t *testing.T) {
	b := newBattleWithSide(t, magicSide(70), "win", rollRnd(9999))
	you := b.PlayerUnit()
	you.Status.Paralysis = 2 // 中的是麻痹，净化(毒)解不掉
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 0}); perr != nil {
		t.Fatalf("提交被拒: %v", perr)
	}
	evs := b.act(you)
	if len(evs[0].Effects) != 0 {
		t.Fatalf("没有对应的状态时不应有效果: %+v", evs[0].Effects)
	}
	if you.Status.Paralysis != 2 {
		t.Fatalf("麻痹不应被解除: %+v", you.Status)
	}
}

// 高等净化精灵Lv1（60）：全部状态，只解一种，原版按 毒、麻、眠、石、醉、乱 的顺序取最后一个有回合数的。
func TestCureAllSpellClearsOneStatus(t *testing.T) {
	b := newBattleWithSide(t, magicSide(60), "win", rollRnd(9999))
	you := b.PlayerUnit()
	you.Status.Poison, you.Status.Confusion = 3, 2
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 0}); perr != nil {
		t.Fatalf("提交被拒: %v", perr)
	}
	evs := b.act(you)
	eff := effectOn(evs[0], 0)
	if eff == nil || len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "confusion" {
		t.Fatalf("全部状态精灵应解掉最后一个（乱）: %+v", evs[0].Effects)
	}
	if you.Status.Poison != 3 || you.Status.Confusion != 0 {
		t.Fatalf("毒应保留: %+v", you.Status)
	}
}

// 解状态的规则（BATTLE_MultiStatusRecovery）：tostatus 是最后一个有回合数的状态；
// 指定某一种时，只有它正好是 tostatus 才解。
func TestStatusSetCureMagicRule(t *testing.T) {
	s := StatusSet{Poison: 3, Confusion: 2}
	if got := s.cureMagic("poison"); got != nil {
		t.Fatalf("乱是 tostatus，毒的精灵不解: %v", got)
	}
	if got := s.cureMagic("all"); len(got) != 1 || got[0] != "confusion" || s.Poison != 3 {
		t.Fatalf("全部状态应只解乱: %v %+v", got, s)
	}
	if got := s.cureMagic("all"); len(got) != 1 || got[0] != "poison" || s.Poison != 0 {
		t.Fatalf("再解一次应解毒: %v %+v", got, s)
	}
	var none StatusSet
	if got := none.cureMagic("all"); got != nil {
		t.Fatalf("没有状态时不解: %v", got)
	}
}

// 醉（BATTLE_DuckCheck 的 RAND(20,30)）：攻击方醉了，对方更容易躲开。骰子固定在 2000 附近，
// 没醉时闪避率约 0，醉了加 20–30 的基数后闪避。
func TestDrunkAttackerIsDodgedMore(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", func(n int) int { return 1999 % n })
	you := b.PlayerUnit()
	m := b.Unit(15)
	m.Agility = you.Agility
	if b.duckCheck(m, you) {
		t.Fatalf("没醉时不应躲开")
	}
	m.Status.Drunk = 3
	if !b.duckCheck(m, you) {
		t.Fatalf("醉了的攻击应能被躲开")
	}
}

// 混乱（乱）的怪物攻击随机目标：默认 AI 在骰子取最大时打宠物（5 号），混乱后随机打的是 16 号。
func TestConfusedMonsterAttacksRandomTarget(t *testing.T) {
	maxRnd := func(n int) int { return n - 1 }
	plain := newBattleWithSide(t, testSide(), "win", maxRnd)
	m := plain.Unit(15)
	m.AI = defaultAIOptions()
	evs := plain.monsterAct(m)
	if len(evs) != 1 || evs[0].Kind != "attack" || evs[0].Effects[0].Target != 5 {
		t.Fatalf("没混乱时应打宠物 5: %+v", evs)
	}

	confused := newBattleWithSide(t, testSide(), "win", maxRnd)
	m = confused.Unit(15)
	m.AI = defaultAIOptions()
	m.Status.Confusion = 1
	evs = confused.monsterAct(m)
	if len(evs) != 1 || evs[0].Kind != "attack" || evs[0].Effects[0].Target != 16 {
		t.Fatalf("混乱的怪物应随机打 16: %+v", evs)
	}
}

// STAGE-190：装备自带的精灵按栏位的 MP 扣气力（滋润的精灵 Lv1 魔法表里是 6，装备上写的是 8），其余栏位仍按魔法表。
func TestEquipMagicUsesSlotMP(t *testing.T) {
	side := magicSide(10)
	side.MagicMP[1], side.MagicMPSet[1] = 8, true
	b := newBattleWithSide(t, side, "win", fixedRnd(1, 2, 3, 4))
	you := b.PlayerUnit()
	you.MP = 7
	_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: you.No})
	expectCode(t, perr, protocol.CodeNotEnoughMP)
	you.MP = 100
	_, perr = b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: you.No})
	expectCode(t, perr, "")
}
