package character

// STAGE-245：战斗中倒下（HP 归零）的宠物，战斗结束后不论胜负都回到 1 血、清掉倒下状态，能再设为出战；
// 骑乘的宠物同样处理，不在这场战斗里的随身宠物也处理（对照原版 battle.c 的 BATTLE_Exit），寄放的宠物不动。

import (
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// faintSide 造一个人物加一只出战宠物的战斗输入（数值同 battle 包测试的 testSide）。
func faintSide(playerHP, petHP int) *battle.SideInput {
	return &battle.SideInput{
		Name: "勇者小明", Level: 1, HP: playerHP, MaxHP: 300, MP: 100, MaxMP: 100,
		Attack: 30, Defense: 20, Agility: 4, Charm: 60,
		Elem: battle.Elements{Earth: 60, Water: 40},
		BattlePet: &battle.PetInput{
			Slot: 0, TemplateID: 1, Name: "乌力", Level: 5, HP: petHP, MaxHP: 200,
			Attack: 25, Defense: 20, Agility: 7, Elem: battle.Elements{Earth: 80, Water: 20},
			SkillSlots: 7, Skills: [7]int{1, -1, -1, -1, -1, -1, -1},
			LoyaltyBase: 100, // 忠诚满，一直听话
		},
	}
}

// fightOut 每回合人物和宠物都打第一个活着的敌人，打到战斗结束（最多 40 回合）。
func fightOut(t *testing.T, b *battle.Battle) *battle.EndView {
	t.Helper()
	for turn := 0; turn < 40; turn++ {
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		b.Submit(b.Turn(), "player", "attack", battle.Cmd{Target: target})
		b.Submit(b.Turn(), "pet", "attack", battle.Cmd{Target: target})
		if _, end := b.Settle(); end != nil {
			return end
		}
	}
	t.Fatal("40 回合内没有结束")
	return nil
}

// petEndOf 取战斗里某个栏位宠物的结束状态；没有出过场返回 false。
func petEndOf(b *battle.Battle, slot int) (battle.PetEnd, bool) {
	for _, e := range b.PetEnds() {
		if e.Slot == slot {
			return e, true
		}
	}
	return battle.PetEnd{}, false
}

// TestPetFaintedInBattleBackToOneHP 出战宠物战斗中倒下：胜利和失败结束后都是 1 血，能再设为出战；
// 不在这场战斗里的休息、待命宠物：HP 0 的同样回到 1，HP 大于 0 的不变；寄放的宠物不动。
func TestPetFaintedInBattleBackToOneHP(t *testing.T) {
	cases := []struct {
		name, scenario string
		playerHP       int
		want           string
	}{
		{"胜利", "win_faint", 300, "win"},
		{"失败", "lose", 1, "lose"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := battle.Build("b_pet_faint", faintSide(c.playerHP, 1), c.scenario)
			if err != nil {
				t.Fatal(err)
			}
			r := &record{Name: "勇者小明", Level: 1, HP: c.playerHP, Pets: []petRec{
				{Slot: 0, TemplateID: 1, Name: "乌力", Level: 5, HP: 1, State: petStateBattle},
				{Slot: 1, TemplateID: 2, Name: "休息", Level: 5, HP: 0, State: petStateRest},
				{Slot: 2, TemplateID: 3, Name: "待命", Level: 5, HP: 34, State: petStateStandby},
				{Slot: gamedata.PooledSlotBase, TemplateID: 4, Name: "寄放", Level: 5, HP: 0, State: petStatePooled},
			}}
			end := fightOut(t, b)
			if end.Result != c.want {
				t.Fatalf("结果 = %s，want %s（剧本没按预期打）", end.Result, c.want)
			}
			if e, ok := petEndOf(b, 0); !ok || e.HP != 0 {
				t.Fatalf("前提：出战宠物应在战斗中倒下（HP 0），实际 %+v ok=%v", e, ok)
			}

			(&Service{}).applyResult(r, b, end)

			if got := r.Pets[0].HP; got != 1 {
				t.Fatalf("倒下的出战宠物战后 HP = %d，want 1", got)
			}
			if got := r.Pets[1].HP; got != 1 {
				t.Errorf("不在战斗里的休息宠物 HP = %d，want 1（原版 BATTLE_Exit 遍历全部宠物）", got)
			}
			if got := r.Pets[2].HP; got != 34 {
				t.Errorf("HP 大于 0 的待命宠物 HP = %d，want 不变 34", got)
			}
			if got := r.Pets[3].HP; got != 0 {
				t.Errorf("寄放的宠物 HP = %d，want 不动 0（原版 CHAR_MAILMODE 跳过）", got)
			}
			// 战后能再设为出战：先把它放回休息，再设出战不该被“已倒下”拦住。
			r.Pets[0].State = petStateRest
			if _, err := planPetState(r.Pets, 0, petStateBattle); err != nil {
				t.Fatalf("战后 1 血的宠物应能设为出战：%v", err)
			}
		})
	}
}

// TestRiderFaintedBackToOneHP 骑乘的宠物在战斗中倒下：战斗结束回到 1 血，仍是骑乘状态。
func TestRiderFaintedBackToOneHP(t *testing.T) {
	side := faintSide(40, 200)
	side.Rider = &battle.PetInput{
		Slot: 1, TemplateID: 194, Name: "金格萨贝鲁", Level: 1, HP: 1, MaxHP: 40,
		Attack: 10, Defense: 10, Agility: 5, Elem: battle.Elements{Earth: 50, Water: 50},
		SkillSlots: 7, Skills: [7]int{-1, -1, -1, -1, -1, -1, -1},
		LoyaltyBase: 100,
	}
	b, err := battle.Build("b_ride_faint", side, "lose")
	if err != nil {
		t.Fatal(err)
	}
	r := &record{Name: "勇者小明", Level: 1, HP: 40, Pets: []petRec{
		{Slot: 0, TemplateID: 1, Name: "乌力", Level: 5, HP: 200, State: petStateBattle},
		{Slot: 1, TemplateID: 194, Name: "金格萨贝鲁", Level: 1, HP: 1, State: petStateRide},
	}}
	end := fightOut(t, b)
	if e, ok := petEndOf(b, 1); !ok || e.HP != 0 {
		t.Fatalf("前提：骑乘的宠物应在战斗中倒下（HP 0），实际 %+v ok=%v", e, ok)
	}

	(&Service{}).applyResult(r, b, end)

	if got := r.Pets[1].HP; got != 1 {
		t.Fatalf("骑乘的宠物倒下后战后 HP = %d，want 1", got)
	}
	if got := r.Pets[1].State; got != petStateRide {
		t.Fatalf("骑乘的宠物战后状态 = %d，want 仍是骑乘 %d", got, petStateRide)
	}
}
