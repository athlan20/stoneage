package character

// STAGE-247：出战的宠物在战斗中逃跑，结束后仍是出战（原版 BATTLE_Escape → BATTLE_Exit 不改 CHAR_DEFAULTPET，
// 只有玩家自己改“出战”或宠物被击飞、年兽吓跑等情况才会变）。胜、负、平局的出战状态不变；
// 0 血逃跑后 HP 仍是 1 且仍出战（STAGE-245）；待命的宠物不因为这场战斗变成休息。

import (
	"testing"

	"github.com/athlan20/stoneage/server/battle"
)

// escapeOut 每回合人物逃跑、宠物攻击第一个活着的敌人，直到战斗结束（逃跑没成功就下一回合再试，最多 40 回合）。
func escapeOut(t *testing.T, b *battle.Battle) *battle.EndView {
	t.Helper()
	for turn := 0; turn < 40; turn++ {
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		b.Submit(b.Turn(), "player", "escape", battle.Cmd{})
		b.Submit(b.Turn(), "pet", "attack", battle.Cmd{Target: target})
		if _, end := b.Settle(); end != nil {
			return end
		}
	}
	t.Fatal("40 回合内没有结束")
	return nil
}

// guardOut 每回合人物和宠物都防御，不出手打怪，直到战斗结束（showcase 第二回合怪物全部逃走，结果是平局）。
func guardOut(t *testing.T, b *battle.Battle) *battle.EndView {
	t.Helper()
	for turn := 0; turn < 40; turn++ {
		b.Submit(b.Turn(), "player", "guard", battle.Cmd{})
		b.Submit(b.Turn(), "pet", "guard", battle.Cmd{})
		if _, end := b.Settle(); end != nil {
			return end
		}
	}
	t.Fatal("40 回合内没有结束")
	return nil
}

// escapeRecord 造一个带出战、休息、待命三只宠物的角色（出战的是栏位 0，血量 petHP）。
func escapeRecord(playerHP, petHP int) *record {
	return &record{Name: "勇者小明", Level: 1, HP: playerHP, Pets: []petRec{
		{Slot: 0, TemplateID: 1, Name: "乌力", Level: 5, HP: petHP, State: petStateBattle},
		{Slot: 1, TemplateID: 2, Name: "休息", Level: 5, HP: 30, State: petStateRest},
		{Slot: 2, TemplateID: 3, Name: "待命", Level: 5, HP: 34, State: petStateStandby},
	}}
}

// TestBattlePetKeepsActiveState 出战的宠物逃跑、胜利、失败、平局后都仍是出战；休息和待命的宠物状态不变。
func TestBattlePetKeepsActiveState(t *testing.T) {
	cases := []struct {
		name     string
		scenario string
		playerHP int
		outFn    func(*testing.T, *battle.Battle) *battle.EndView
		want     string
	}{
		{"逃跑", "escape", 300, escapeOut, "escaped"},
		{"胜利", "win", 300, fightOut, "win"},
		{"失败", "lose", 1, fightOut, "lose"},
		{"平局", "showcase", 300, guardOut, "draw"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := battle.Build("b_pet_escape", faintSide(c.playerHP, 200), c.scenario)
			if err != nil {
				t.Fatal(err)
			}
			r := escapeRecord(c.playerHP, 200)
			end := c.outFn(t, b)
			if end.Result != c.want {
				t.Fatalf("结果 = %s，want %s（剧本没按预期打）", end.Result, c.want)
			}

			(&Service{}).applyResult(r, b, end)

			if got := r.Pets[0].State; got != petStateBattle {
				t.Errorf("%s后出战宠物状态 = %s，want 出战", c.name, petStateNames[got])
			}
			if got := r.Pets[0].HP; got < 1 {
				t.Errorf("%s后出战宠物血量 = %d，want 大于 0", c.name, got)
			}
			if got := r.Pets[1].State; got != petStateRest {
				t.Errorf("%s后休息宠物状态 = %s，want 休息", c.name, petStateNames[got])
			}
			if got := r.Pets[2].State; got != petStateStandby {
				t.Errorf("%s后待命宠物状态 = %s，want 待命", c.name, petStateNames[got])
			}
		})
	}
}

// TestEscapeWithFaintedPetKeepsActive 出战的宠物在战斗中倒下后，人物逃跑：战后 1 血，仍是出战，能再设为出战。
func TestEscapeWithFaintedPetKeepsActive(t *testing.T) {
	b, err := battle.Build("b_pet_escape_faint", faintSide(300, 1), "escape")
	if err != nil {
		t.Fatal(err)
	}
	r := escapeRecord(300, 1)
	// 先让人物和宠物都攻击，直到出战宠物倒下（HP 0），再下一回合逃跑。
	for turn := 0; ; turn++ {
		if e, ok := petEndOf(b, 0); ok && e.HP == 0 {
			break
		}
		if turn >= 40 {
			t.Fatal("40 回合内出战宠物没有倒下")
		}
		b.Submit(b.Turn(), "player", "attack", battle.Cmd{Target: 15})
		b.Submit(b.Turn(), "pet", "attack", battle.Cmd{Target: 15})
		if _, end := b.Settle(); end != nil {
			t.Fatalf("前提：战斗在宠物倒下前就结束了（%s）", end.Result)
		}
	}
	end := escapeOut(t, b)
	if end.Result != "escaped" {
		t.Fatalf("结果 = %s，want escaped", end.Result)
	}

	(&Service{}).applyResult(r, b, end)

	if got := r.Pets[0].HP; got != 1 {
		t.Fatalf("倒下后逃跑的出战宠物战后 HP = %d，want 1（STAGE-245）", got)
	}
	if got := r.Pets[0].State; got != petStateBattle {
		t.Fatalf("倒下后逃跑的出战宠物状态 = %s，want 出战", petStateNames[got])
	}
}
