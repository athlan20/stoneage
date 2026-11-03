package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// statusChangeRnd 给异常攻击用的确定随机源：1 万面骰取 9999（不闪避、不暴击），
// 百分骰（状态命中、防御挡伤）取 statusDie，其他取 0。
func statusChangeRnd(statusDie int) func(n int) int {
	return func(n int) int {
		switch n {
		case 10000:
			return 9999
		case 100:
			return statusDie
		}
		return 0
	}
}

// statusChangeSetup 给出同级、同四项的攻守双方：抗性 10，命中率 30 − 10 = 20。
func statusChangeSetup(rnd func(n int) int) (*Battle, *Unit, *Unit) {
	b := newFormulaBattle(rnd)
	at, df := b.Unit(0), b.Unit(15)
	at.Level, df.Level = 10, 10
	at.Attack = 100
	at.Vital, at.Strength, at.Toughness, at.Dexterity = 10, 10, 10, 10
	df.Vital, df.Strength, df.Toughness, df.Dexterity = 10, 10, 10, 10
	df.Defense = 0
	df.HP, df.MaxHP = 500, 500
	return b, at, df
}

func petSkillByIDForTest(t *testing.T, id int) gamedata.PetSkill {
	t.Helper()
	s, ok := gamedata.PetSkillByID(id)
	if !ok {
		t.Fatalf("技能 %d 不存在", id)
	}
	return s
}

// statusTurnsForTest 读出一个异常状态的剩余回合数（协议状态名）。
func statusTurnsForTest(s StatusSet, name string) int {
	switch name {
	case "poison":
		return s.Poison
	case "stone":
		return s.Stone
	case "confusion":
		return s.Confusion
	case "drunk":
		return s.Drunk
	case "sleep":
		return s.Sleep
	}
	return -1
}

// TestStatusChangeSkillsApplyStatus 覆盖 petskill.txt 里的六个异常攻击：命中后上对应状态，
// 持续 turn + 1 回合（原版 gBattleStausTurn + 1）。
func TestStatusChangeSkillsApplyStatus(t *testing.T) {
	cases := []struct {
		id     int
		name   string
		status string
		turns  int
	}{
		{60, "毒攻击", "poison", 4},
		{61, "猛毒攻击", "poison", 6},
		{80, "石化攻击", "stone", 4},
		{90, "混乱攻击", "confusion", 4},
		{100, "泥醉攻击", "drunk", 4},
		{110, "催眠攻击", "sleep", 4},
	}
	for _, c := range cases {
		b, at, df := statusChangeSetup(statusChangeRnd(18)) // 百分骰 19 < 20：命中
		ev := b.castSkill(at, petSkillByIDForTest(t, c.id), Cmd{Target: df.No})
		if got := statusTurnsForTest(df.Status, c.status); got != c.turns {
			t.Errorf("%s：%s 回合数 = %d，期望 %d", c.name, c.status, got, c.turns)
		}
		if !effectHasStatusAdd(ev, df.No, c.status) {
			t.Errorf("%s：事件里应带 status_add %s：%+v", c.name, c.status, ev.Effects)
		}
	}
}

// effectHasStatusAdd 表示事件里目标 target 的效果带了 status_add 状态。
func effectHasStatusAdd(ev Event, target int, status string) bool {
	for _, eff := range ev.Effects {
		if eff.Target != target {
			continue
		}
		for _, s := range eff.StatusAdd {
			if s == status {
				return true
			}
		}
	}
	return false
}

// TestStatusChangeAttackIsScaled 攻击力按“攻%”打折：毒攻击（攻% -30）的伤害等于攻击力 70 的通常攻击。
func TestStatusChangeAttackIsScaled(t *testing.T) {
	// 状态骰取 99，保证不上状态，只比较伤害。
	bSkill, atSkill, dfSkill := statusChangeSetup(statusChangeRnd(99))
	bSkill.castSkill(atSkill, petSkillByIDForTest(t, 60), Cmd{Target: dfSkill.No})
	lostSkill := dfSkill.MaxHP - dfSkill.HP

	bPlain, atPlain, dfPlain := statusChangeSetup(statusChangeRnd(99))
	atPlain.Attack = 70
	bPlain.castSkill(atPlain, normalAttackSkillForTest(), Cmd{Target: dfPlain.No})
	lostPlain := dfPlain.MaxHP - dfPlain.HP

	if lostSkill <= 0 {
		t.Fatalf("毒攻击应造成伤害，实际 %d", lostSkill)
	}
	if lostSkill != lostPlain {
		t.Errorf("毒攻击（攻 100，攻%% -30）伤害 = %d，应等于攻击力 70 的通常攻击 %d", lostSkill, lostPlain)
	}
}

// normalAttackSkillForTest 是通常攻击（PETSKILL_NormalAttack）的技能模板。
func normalAttackSkillForTest() gamedata.PetSkill {
	return gamedata.PetSkill{ID: 1, Name: "攻击", Func: "PETSKILL_NormalAttack"}
}

// TestStatusChangeMissesWhenRollFails 状态骰没中（命中率 20，骰 20 不小于 20）：只掉血，不上状态。
func TestStatusChangeMissesWhenRollFails(t *testing.T) {
	b, at, df := statusChangeSetup(statusChangeRnd(19))
	ev := b.castSkill(at, petSkillByIDForTest(t, 60), Cmd{Target: df.No})
	if df.Status.Poison != 0 {
		t.Errorf("状态骰没中，不应中毒，实际回合数 %d", df.Status.Poison)
	}
	if effectHasStatusAdd(ev, df.No, "poison") {
		t.Errorf("状态骰没中，事件里不应有 status_add：%+v", ev.Effects)
	}
	if df.HP >= df.MaxHP {
		t.Error("攻击仍应造成伤害")
	}
}

// TestStatusChangeNoStackOnBadStatus 目标已带着异常状态（石化），毒攻击不再上状态，石化保持原回合数。
func TestStatusChangeNoStackOnBadStatus(t *testing.T) {
	b, at, df := statusChangeSetup(statusChangeRnd(0))
	df.Status.Stone = 2
	b.castSkill(at, petSkillByIDForTest(t, 60), Cmd{Target: df.No})
	if df.Status.Poison != 0 {
		t.Errorf("已有石化，不应再中毒，实际 %d", df.Status.Poison)
	}
	if df.Status.Stone != 2 {
		t.Errorf("石化回合数应保持 2，实际 %d", df.Status.Stone)
	}
}

// TestStatusChangeNoStatusWhenBlocked 目标防御中，攻击被挡住（骰 1 落在 ≤ 25 的完全挡住档），不上状态。
func TestStatusChangeNoStatusWhenBlocked(t *testing.T) {
	b, at, df := statusChangeSetup(statusChangeRnd(0))
	df.Status.Guard = true
	ev := b.castSkill(at, petSkillByIDForTest(t, 60), Cmd{Target: df.No})
	if df.Status.Poison != 0 {
		t.Errorf("攻击被挡住，不应中毒，实际 %d", df.Status.Poison)
	}
	if len(ev.Effects) == 0 || ev.Effects[0].Result != "block" {
		t.Errorf("应是挡住的效果：%+v", ev.Effects)
	}
}
