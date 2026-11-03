// 场地属性与属性反转（STAGE-200，对照 gmsv/battle/battle_magic.c 的 BATTLE_FieldAttChange、
// BATTLE_MultiAttReverse，battle_event.c 的 BATTLE_FieldAttAdjust、BATTLE_AttrAdjust，battle.c 的
// BATTLE_AttReverse 与回合末递减）的引擎测试。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// mustMagic 取精灵模板，表里没有就失败。
func mustMagic(t *testing.T, id int) gamedata.MagicTemplate {
	t.Helper()
	m, ok := gamedata.MagicByID(id)
	if !ok {
		t.Fatalf("精灵 %d 不在表里", id)
	}
	return m
}

// 彩虹的精灵（240）的目标类型是 any：己方或敌方任意一个活着的单位都能选，倒下的和空格子回复 INVALID_TARGET。
func TestAnyTargetRules(t *testing.T) {
	b := newBattleWithSide(t, magicSide(240), "win", rollRnd(9999))
	sturdy(b, 15)
	cases := []struct {
		name   string
		target int
		want   string
	}{
		{"己方人物（活着）", 0, ""},
		{"己方宠物（活着）", 5, ""},
		{"敌方怪物（活着）", 15, ""},
		{"空格子", 1, protocol.CodeInvalidTarget},
		{"没有目标", -1, protocol.CodeBadRequest},
	}
	for _, tc := range cases {
		_, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: tc.target})
		if tc.want == "" {
			if perr != nil {
				t.Fatalf("%s 应接受，实际 %s", tc.name, perr.Code)
			}
			continue
		}
		expectCode(t, perr, tc.want)
	}
	// 倒下的单位不能选：把怪物标成倒下再选它。
	b2 := newBattleWithSide(t, magicSide(240), "win", rollRnd(9999))
	sturdy(b2, 15).Down = true
	_, perr := b2.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15})
	expectCode(t, perr, protocol.CodeInvalidTarget)
}

// 公式（BATTLE_AttrAdjust）：攻击者火 80、被打者火 20，伤害 100。相克部分：
// 火打火 1.0、火打无 1.5、无打火 0.6、无打无 1.0，sum = 100·(80·20·1.0 + 80·80·1.5 + 20·20·0.6 + 20·80·1.0)
// = 1304000，×1e-4 = 130（无场地）。场地火 60：比值 (0.5 + 80·60·0.00005) / (0.5 + 20·60·0.00005)
// = 0.74 / 0.56，130 × 0.74 / 0.56 = 171.8，截断为 171。
func TestFieldAttrAdjustRatio(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	atk := &Unit{Kind: KindPlayer, Elem: Elements{Fire: 80}}
	tgt := &Unit{Kind: KindMonster, Elem: Elements{Fire: 20}}
	if got := b.attrAdjust(atk, tgt, 100); got != 130 {
		t.Fatalf("无场地时伤害 = %d，期望 130（与改动前一致）", got)
	}
	b.setField(mustMagic(t, 210)) // 火炎的精灵 Lv1：火 60，3 回合
	if got := b.attrAdjust(atk, tgt, 100); got != 171 {
		t.Fatalf("火 60 场地下伤害 = %d，期望 171", got)
	}
}

// 无属性场地（调和）时两边都是 0.5，比值为 1，伤害与没有场地时完全一样。
func TestFieldNoneLeavesDamageUnchanged(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	atk := &Unit{Kind: KindPlayer, Elem: Elements{Earth: 60, Water: 40}}
	tgt := &Unit{Kind: KindMonster, Elem: Elements{Fire: 50, Wind: 30}}
	before := b.attrAdjust(atk, tgt, 137)
	b.setField(mustMagic(t, 230)) // 调和的精灵：场地无属性
	if got := b.attrAdjust(atk, tgt, 137); got != before {
		t.Fatalf("无属性场地伤害 = %d，应与无场地的 %d 相同", got, before)
	}
}

// 递减（battle.c 回合末）：设置 3 回合，第 1、2 回合末仍在，第 3 回合末恢复成无属性，并发一个 field 事件。
func TestFieldCountsDownAndRecovers(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	evs := b.setField(mustMagic(t, 210))
	if len(evs) != 1 || evs[0].Kind != "field" || evs[0].Field == nil || evs[0].Field.Attr != "fire" ||
		evs[0].Field.Pow != 60 || evs[0].Field.Left != 3 {
		t.Fatalf("施放后应发一个 fire 60 left 3 的 field 事件: %+v", evs)
	}
	for turn := 1; turn <= 2; turn++ {
		if got := b.tickField(); got != nil {
			t.Fatalf("第 %d 回合末不应恢复: %+v", turn, got)
		}
		if b.field.Attr != "fire" {
			t.Fatalf("第 %d 回合末场地应仍是火: %+v", turn, b.field)
		}
	}
	got := b.tickField()
	if len(got) != 1 || got[0].Kind != "field" || got[0].Field == nil || got[0].Field.Attr != "none" {
		t.Fatalf("第 3 回合末应发恢复事件（attr none）: %+v", got)
	}
	if b.field.Attr != "" || b.fieldView() != nil {
		t.Fatalf("恢复后应是无属性: %+v", b.field)
	}
	if got := b.tickField(); got != nil {
		t.Fatalf("无属性时递减不应有事件: %+v", got)
	}
}

// 整体覆盖（BATTLE_FieldAttChange）：先火 60（3 回合），过一回合后再用水 40（5 回合），属性、威力、回合都换成新的。
func TestFieldReplacesWholeState(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	b.setField(mustMagic(t, 210))
	b.tickField()
	b.setField(gamedata.MagicTemplate{Func: gamedata.MagicFuncField, Elem: "水", Power: 40, Turns: 5})
	if b.field != (fieldState{Attr: "water", Pow: 40, Left: 5}) {
		t.Fatalf("覆盖后应是水 40 剩 5 回合: %+v", b.field)
	}
}

// 调和（230）：把场地设成无属性，之后两边都按 0.5 算。
func TestHarmonyClearsField(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	b.setField(mustMagic(t, 200)) // 水的精灵 Lv1
	evs := b.setField(mustMagic(t, 230))
	if b.field.Attr != "" || b.fieldPower(b.PlayerUnit()) != fieldBottom {
		t.Fatalf("调和后应无属性: %+v", b.field)
	}
	if len(evs) != 1 || evs[0].Field == nil || evs[0].Field.Attr != "none" {
		t.Fatalf("调和应发 attr none 的 field 事件: %+v", evs)
	}
}

// 反转（BATTLE_MultiAttReverse + BATTLE_AttReverse）：连用三次，属性分别是 互换 → 不变（仍是换过的）→ 换回原样。
// 原版行为，别“修正”。地 60 水 40 火 20 风 10 起步。
func TestReverseThreeTimes(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	u := b.PlayerUnit()
	u.Elem = Elements{Earth: 60, Water: 40, Fire: 20, Wind: 10}
	steps := []struct {
		name     string
		elem     Elements
		reversed bool
		add      bool
	}{
		{"第一次：标志开，地火、水风互换", Elements{Earth: 20, Water: 10, Fire: 60, Wind: 40}, true, true},
		{"第二次：标志关，属性不变", Elements{Earth: 20, Water: 10, Fire: 60, Wind: 40}, false, false},
		{"第三次：标志开，再互换换回原样", Elements{Earth: 60, Water: 40, Fire: 20, Wind: 10}, true, true},
	}
	for _, st := range steps {
		eff := b.reverseUnit(u)
		if u.Elem != st.elem || u.Reversed != st.reversed {
			t.Fatalf("%s：属性 %+v 标志 %v，应为 %+v 标志 %v", st.name, u.Elem, u.Reversed, st.elem, st.reversed)
		}
		if eff.Result != "none" {
			t.Fatalf("%s：效果 result 应为 none，实际 %s", st.name, eff.Result)
		}
		if st.add && (len(eff.StatusAdd) != 1 || eff.StatusAdd[0] != "reversed") {
			t.Fatalf("%s：应挂上 reversed: %+v", st.name, eff)
		}
		if !st.add && (len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "reversed") {
			t.Fatalf("%s：应去掉 reversed: %+v", st.name, eff)
		}
	}
}

// 反转的标志进状态标记（协议 10.2 的 status 里是 reversed），客户端据此画“反”字。
func TestReversedShowsInStatus(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(9999))
	u := b.PlayerUnit()
	b.reverseUnit(u)
	if got := u.statusList(); len(got) != 1 || got[0] != "reversed" {
		t.Fatalf("反转后状态应是 [reversed]: %v", got)
	}
	b.reverseUnit(u)
	if got := u.statusList(); len(got) != 0 {
		t.Fatalf("标志关后状态应为空: %v", got)
	}
}

// 施放场地精灵（210，火 60，3 回合）：先出 magic 事件（func 为 field，不用选目标，扣气力），
// 紧跟一个 field 事件；之后场地对双方都生效。
func TestCastFieldSpellSetsFieldForBoth(t *testing.T) {
	b := newBattleWithSide(t, magicSide(210), "win", rollRnd(9999))
	you := b.PlayerUnit()
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: -1}); perr != nil {
		t.Fatalf("场地精灵不用选目标，应被接受: %v", perr)
	}
	evs := b.act(you)
	if len(evs) != 2 || evs[0].Kind != "magic" || evs[0].Magic == nil || evs[0].Magic.Func != "field" {
		t.Fatalf("应先有 magic(field) 事件，再有 field 事件: %+v", evs)
	}
	if evs[1].Kind != "field" || evs[1].Field == nil || *evs[1].Field != (FieldView{Attr: "fire", Pow: 60, Left: 3}) {
		t.Fatalf("field 事件应是火 60 剩 3 回合: %+v", evs[1])
	}
	if you.MP != 100-4 {
		t.Fatalf("场地精灵 Lv1 的气力应扣 4: %d", you.MP)
	}
	you.Elem.Fire = 80 // 火 80 的单位在火 60 场地上的加成 0.5 + 80·60·0.00005 = 0.74
	if got := b.fieldPower(you); got != 0.74 {
		t.Fatalf("火 80 单位在火 60 场地上的加成 = %v，期望 0.74", got)
	}
	if v := b.fieldView(); v == nil || *v != (FieldView{Attr: "fire", Pow: 60, Left: 3}) {
		t.Fatalf("场地视图应是火 60 剩 3 回合: %+v", v)
	}
}

// 彩虹的精灵（240）：所选单位（己方或敌方）翻一次反转，效果 result 为 none，挂上 reversed；
// 只影响那一个单位，另一方不动。
func TestRainbowReversesOneUnit(t *testing.T) {
	b := newBattleWithSide(t, magicSide(240), "win", rollRnd(9999))
	sturdy(b, 15)
	enemy := b.Unit(15)
	enemy.Elem = Elements{Earth: 60, Water: 40, Fire: 20, Wind: 10}
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15}); perr != nil {
		t.Fatalf("彩虹可以选敌方，应被接受: %v", perr)
	}
	evs := b.act(b.PlayerUnit())
	eff := effectOn(evs[0], 15)
	if eff == nil || eff.Result != "none" || len(eff.StatusAdd) != 1 || eff.StatusAdd[0] != "reversed" {
		t.Fatalf("彩虹应对目标挂上 reversed: %+v", evs[0].Effects)
	}
	if enemy.Elem != (Elements{Earth: 20, Water: 10, Fire: 60, Wind: 40}) || !enemy.Reversed {
		t.Fatalf("敌方属性应互换: %+v reversed=%v", enemy.Elem, enemy.Reversed)
	}
	if b.PlayerUnit().Reversed || len(evs[0].Effects) != 1 {
		t.Fatalf("彩虹是单人，己方不动: %+v", evs[0].Effects)
	}
}

// 极光的精灵（241）：选一个单位，落在它所在的一方全体；敌方的怪物全部翻转，己方不动。
func TestAuroraReversesWholeSide(t *testing.T) {
	b := newBattleWithSide(t, magicSide(241), "win", rollRnd(9999))
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15}); perr != nil {
		t.Fatalf("极光选敌方一个，应被接受: %v", perr)
	}
	evs := b.act(b.PlayerUnit())
	var enemies []int
	for _, x := range b.units {
		if x != nil && x.Side() == 1 && x.Alive() {
			enemies = append(enemies, x.No)
			if !x.Reversed {
				t.Fatalf("敌方 %d 应被反转", x.No)
			}
		}
	}
	if len(enemies) == 0 || len(evs[0].Effects) != len(enemies) {
		t.Fatalf("极光应对敌方全体各有一条效果（敌方 %v）: %+v", enemies, evs[0].Effects)
	}
	if b.PlayerUnit().Reversed || b.PetUnit().Reversed {
		t.Fatalf("极光落在敌方，己方不动")
	}
}

// 反转的目标不能是倒下的单位：提交时 INVALID_TARGET；就算执行时倒下了也不结算。
func TestReverseSkipsDeadTarget(t *testing.T) {
	b := newBattleWithSide(t, magicSide(240), "win", rollRnd(9999))
	sturdy(b, 15).Down = true
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: 15}); perr == nil {
		t.Fatalf("倒下的敌方不能选")
	}
	if got := b.reverseTargets(Cmd{Target: 15}, true); got != nil {
		t.Fatalf("倒下的目标不反转: %v", got)
	}
}

// 回合末递减走 Settle（resolve.go）：施放的那一回合末也算一回合，所以 3 回合的场地在第 3 回合末恢复。
func TestFieldTicksAtRoundEnd(t *testing.T) {
	b := newBattleWithSide(t, magicSide(210), "win", rollRnd(9999))
	sturdy(b, 15)
	b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: -1})
	b.Submit(1, "pet", "guard", Cmd{})
	round, end := b.Settle()
	if end != nil || b.field.Left != 2 {
		t.Fatalf("第 1 回合末场地应剩 2 回合: left=%d end=%v", b.field.Left, end)
	}
	if !hasFieldEvent(round.Events, "fire") {
		t.Fatalf("第 1 回合应有火场地的 field 事件: %+v", round.Events)
	}
	for turn := 2; turn <= 3; turn++ {
		b.Submit(turn, "player", "guard", Cmd{})
		b.Submit(turn, "pet", "guard", Cmd{})
		round, end = b.Settle()
		if end != nil {
			t.Fatalf("第 %d 回合不应结束", turn)
		}
		if turn == 2 && (b.field.Attr != "fire" || b.field.Left != 1) {
			t.Fatalf("第 2 回合末应剩 1 回合: %+v", b.field)
		}
	}
	if b.field.Attr != "" || !hasFieldEvent(round.Events, "none") {
		t.Fatalf("第 3 回合末应恢复无属性并发 none 事件: %+v %+v", b.field, round.Events)
	}
}

func hasFieldEvent(evs []Event, attr string) bool {
	for _, ev := range evs {
		if ev.Kind == "field" && ev.Field != nil && ev.Field.Attr == attr {
			return true
		}
	}
	return false
}

// 场地随 battle_start、battle_turn 和 battle 查询推送（协议 10.3、10.4、10.11）：有属性时带 field，无属性时省略。
func TestFieldInSnapshots(t *testing.T) {
	b := newBattleWithSide(t, magicSide(210), "win", rollRnd(9999))
	if v := b.StartView(); v.Field != nil || b.TurnView(0, 0).Field != nil || b.QueryView(0, 0).Field != nil {
		t.Fatalf("无场地时三处都应省略 field")
	}
	b.setField(mustMagic(t, 210))
	want := FieldView{Attr: "fire", Pow: 60, Left: 3}
	if v := b.StartView(); v.Field == nil || *v.Field != want {
		t.Fatalf("battle_start 应带场地: %+v", v.Field)
	}
	if v := b.TurnView(0, 0); v.Field == nil || *v.Field != want {
		t.Fatalf("battle_turn 应带场地: %+v", v.Field)
	}
	if v := b.QueryView(0, 0); v.Field == nil || *v.Field != want {
		t.Fatalf("battle 查询应带场地: %+v", v.Field)
	}
}
