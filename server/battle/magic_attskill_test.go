package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 精灵版宠物技能（STAGE-202）的测试：五个技能各一个，事件是 skill 且名字是技能名；突击的蓄力跨回合；
// 背水之战、不防守战法的整回合效果登记在人物身上。rollRnd(9999) 让闪避、暴击都不触发（同 magic_def_test.go）。

// attSkillCast 直接调用精灵版技能的结算（不经过提交和 castMagic），返回事件。
func attSkillCast(t *testing.T, b *Battle, id, target int) Event {
	t.Helper()
	tpl, ok := gamedata.MagicByID(id)
	if !ok {
		t.Fatalf("精灵 %d 不在表里", id)
	}
	return b.castAttSkill(b.PlayerUnit(), tpl, Cmd{Target: target})
}

// 破除防御（326）、一击必杀（328）打单体：事件是 skill，名字是技能名，对目标扣了血。
func TestAttSkillSingleTargetHits(t *testing.T) {
	cases := []struct {
		name string
		id   int
	}{
		{"破除防御", 326},
		{"一击必杀", 328},
	}
	for _, tc := range cases {
		b := newBattleWithSide(t, magicSide(tc.id), "win", rollRnd(9999))
		enemy := enemiesOf(b)[0]
		tough(enemy)
		if tc.id == 326 { // STAGE-291：破除防御只打防御中的怪（原版 BATTLE_S_GBreak，没防御算没打中）
			enemy.guardRound = true // 破防按本回合交的防御指令判定（STAGE-291 批 B）
		}
		before := enemy.HP
		ev := attSkillCast(t, b, tc.id, enemy.No)
		skill, _ := gamedata.PetSkillByID(map[int]int{326: 3, 328: 40}[tc.id])
		if ev.Kind != "skill" || ev.Name != skill.Name || ev.Name != tc.name {
			t.Fatalf("%s：事件 = %s %q，应为 skill %q", tc.name, ev.Kind, ev.Name, tc.name)
		}
		if ev.Actor == nil || *ev.Actor != b.PlayerUnit().No {
			t.Fatalf("%s：行动者应是人物，实际 %+v", tc.name, ev.Actor)
		}
		eff := effectOn(ev, enemy.No)
		if eff == nil || eff.Delta >= 0 || enemy.HP >= before {
			t.Fatalf("%s：目标应受伤，效果 %+v，血量 %d → %d", tc.name, eff, before, enemy.HP)
		}
	}
}

// 突击（327）：第一回合只蓄力、没有伤害（事件只有名字，人物的 pend 记着），下一回合不用指令，按蓄好的出手。
func TestAttSkillChargeStrikesNextRound(t *testing.T) {
	b := newBattleWithSide(t, magicSide(327), "win", rollRnd(9999))
	you := b.PlayerUnit()
	enemy := enemiesOf(b)[0]
	tough(enemy)
	before := enemy.HP
	ev := attSkillCast(t, b, 327, enemy.No)
	if ev.Kind != "skill" || ev.Name != "突击" || len(ev.Effects) != 0 {
		t.Fatalf("蓄力回合应只有名字、没有效果: %+v", ev)
	}
	if you.pend == nil || you.pend.kind != pendCharge {
		t.Fatalf("人物应记着蓄力: %+v", you.pend)
	}
	if enemy.HP != before {
		t.Fatalf("蓄力回合不应有伤害")
	}
	evs := b.act(you) // 下一回合：不用指令，pend 里的出手继续
	if len(evs) != 1 || evs[0].Kind != "skill" || evs[0].Name != "突击" {
		t.Fatalf("下一回合应是突击的出手: %+v", evs)
	}
	if effectOn(evs[0], enemy.No) == nil || enemy.HP >= before {
		t.Fatalf("突击的出手应打到目标上: %+v，血量 %d → %d", evs[0].Effects, before, enemy.HP)
	}
	if you.pend != nil {
		t.Fatalf("出手后蓄力应清掉")
	}
}

// 背水之战（329）：事件是技能名；整回合的防御力按 防%-35 变化（登记在人物身上，beginRound 时生效）。
func TestAttSkillPowerBalanceRegistersDefense(t *testing.T) {
	b := newBattleWithSide(t, magicSide(329), "win", rollRnd(9999))
	you := b.PlayerUnit()
	enemy := enemiesOf(b)[0]
	tough(enemy)
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: enemy.No}); perr != nil {
		t.Fatalf("提交背水之战失败: %v", perr)
	}
	b.beginRound()
	if you.stance.DefensePct != -35 {
		t.Fatalf("背水之战的防御力变化 = %d%%，应为 -35%%", you.stance.DefensePct)
	}
	ev := b.castAttSkill(you, attMagic(t, 329), Cmd{Target: enemy.No})
	if ev.Kind != "skill" || ev.Name != "背水之战其之１" {
		t.Fatalf("背水之战事件 = %s %q", ev.Kind, ev.Name)
	}
}

// 不防守战法（330）：不用目标，事件是技能名、没有效果；整回合的回避、反击率登记在人物身上并且可以反击。
func TestAttSkillNoGuardRegistersDodgeAndCounter(t *testing.T) {
	b := newBattleWithSide(t, magicSide(330), "win", rollRnd(9999))
	you := b.PlayerUnit()
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: -1}); perr != nil {
		t.Fatalf("提交不防守战法失败: %v", perr)
	}
	b.beginRound()
	if !you.stance.NoGuard || you.stance.NoGuardDuck != 30 || you.stance.NoGuardCounter != 50 {
		t.Fatalf("不防守战法的登记 = %+v，应为可以反击、回避 +30、反击 +50", you.stance)
	}
	ev := attSkillCast(t, b, 330, -1)
	if ev.Kind != "skill" || ev.Name != "不防守战法" || len(ev.Effects) != 0 {
		t.Fatalf("不防守战法事件 = %+v", ev)
	}
}

// 精灵版技能的目标校验与龙阵相同走目标类型：不防守战法是 none（不带 target），其余是敌方一个。
func TestAttSkillTargetFollowsSkill(t *testing.T) {
	cases := map[int]gamedata.TargetType{326: gamedata.TargetEnemy, 327: gamedata.TargetEnemy,
		328: gamedata.TargetEnemy, 329: gamedata.TargetEnemy, 330: gamedata.TargetNone}
	for id, want := range cases {
		if got := attMagic(t, id).BattleTarget(); got != want {
			t.Errorf("精灵 %d 的目标类型 = %s，应为 %s", id, got, want)
		}
	}
}

func attMagic(t *testing.T, id int) gamedata.MagicTemplate {
	t.Helper()
	tpl, ok := gamedata.MagicByID(id)
	if !ok {
		t.Fatalf("精灵 %d 不在表里", id)
	}
	return tpl
}

// 经过提交和出招（castMagic）的精灵版技能：扣气力（10），只有 skill 事件，没有 magic 事件。
func TestAttSkillViaMagicCostsMPAndReturnsSkillEvent(t *testing.T) {
	b := newBattleWithSide(t, magicSide(326), "win", rollRnd(9999))
	you := b.PlayerUnit()
	enemy := enemiesOf(b)[0]
	tough(enemy)
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 1, Target: enemy.No}); perr != nil {
		t.Fatalf("提交精灵版技能失败: %v", perr)
	}
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "skill" || evs[0].Name != "破除防御" {
		t.Fatalf("应只有一个 skill（破除防御）事件: %+v", evs)
	}
	if you.MP != 100-10 {
		t.Fatalf("气力应扣 10，实际 %d", you.MP)
	}
}
