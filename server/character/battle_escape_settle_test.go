package character

// STAGE-252：人物逃跑成功后，本回合剩下的结算（中毒的怪物跳血、出战宠物的收尾）不能 panic，战斗正常结束。
// 逃跑成功时人物和出战宠物都离场（battle.escape），宠物仍是出战状态（STAGE-247），血量按 STAGE-245 收尾。

import (
	"testing"

	"github.com/athlan20/stoneage/server/battle"
)

// TestEscapeSettlesWithPoisonedEnemy 人物逃跑成功、敌方带毒：回合末中毒跳血不 panic，战斗以 escaped 结束，
// 宠物仍出战，血量写回。人物先手（逃跑先于宠物）与宠物先手两种顺序都要过。
func TestEscapeSettlesWithPoisonedEnemy(t *testing.T) {
	cases := []struct {
		name     string
		playerAG int // 人物敏捷：高于宠物 7 则人物先逃跑
	}{
		{"人物先手逃跑", 50},
		{"宠物先手", 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			side := faintSide(300, 200)
			side.Agility = c.playerAG
			b, err := battle.Build("b_escape_settle", side, "escape")
			if err != nil {
				t.Fatal(err)
			}
			b.Rnd = escapeRnd             // 逃跑必成功（同 battle 包测试的 chanceRnd）
			for no := 10; no < 20; no++ { // 敌方全部中毒，回合末跳血
				if u := b.Unit(no); u != nil && u.Kind == battle.KindMonster {
					u.Status.Poison = 5
				}
			}
			r := &record{Name: "勇者小明", Level: 1, HP: 300, Pets: []petRec{
				{Slot: 0, TemplateID: 1, Name: "乌力", Level: 5, HP: 200, State: petStateBattle},
			}}

			b.Submit(b.Turn(), "player", "escape", battle.Cmd{})
			b.Submit(b.Turn(), "pet", "attack", battle.Cmd{Target: 15})
			_, end := b.Settle() // 修复前这里 panic（hurt 对 PlayerUnit()==nil 取阵营）
			if end == nil || end.Result != "escaped" {
				t.Fatalf("结束 = %+v，want escaped", end)
			}

			(&Service{}).applyResult(r, b, end)

			if got := r.Pets[0].State; got != petStateBattle {
				t.Fatalf("逃跑后出战宠物状态 = %d，want 仍是出战 %d（STAGE-247）", got, petStateBattle)
			}
			if got := r.Pets[0].HP; got < 1 {
				t.Fatalf("逃跑后出战宠物 HP = %d，want 不低于 1（STAGE-245）", got)
			}
		})
	}
}

// escapeRnd 让逃跑判定必成功：逃跑概率的骰子（百分比）出 10，其余骰子出大值（同 battle 包测试的 chanceRnd）。
func escapeRnd(n int) int {
	if n == 100 {
		return 10
	}
	return 9999 % n
}
