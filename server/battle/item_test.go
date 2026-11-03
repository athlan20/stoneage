// 道具指令（STAGE-30，docs/protocol.md 10.5 的 item 与 10.7 的 item 事件）的引擎测试：
// 提交校验的错误码、回复与解状态的事件、消耗后快照置空。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// itemSide 在 testSide 的基础上把两件测试物品放进背包：5 号回复肉（体40）、6 号解状态的肉（毒）。
func itemSide() *SideInput {
	side := testSide()
	side.Items[5] = BattleItem{ID: 1235, Name: "普通的肉"}
	side.Items[6] = BattleItem{ID: 1247, Name: "治疗中毒的肉"}
	return side
}

func TestSubmitItemChecks(t *testing.T) {
	b := newBattleWithSide(t, itemSide(), "win", rollRnd(9999))
	you := b.PlayerUnit()
	you.HP = 100 // 留出回复空间
	cases := []struct {
		name string
		cmd  Cmd
		want string
	}{
		{"空槽", Cmd{Slot: 7, Target: -1}, protocol.CodeItemUnusable},
		{"装备格", Cmd{Slot: 2, Target: -1}, protocol.CodeBadRequest},
		{"敌方目标", Cmd{Slot: 5, Target: 15}, protocol.CodeInvalidTarget},
		{"给宠物用回复药是允许的", Cmd{Slot: 5, Target: 5}, ""},
		{"缺省目标是·自己", Cmd{Slot: 5, Target: -1}, ""},
	}
	for _, tc := range cases {
		if _, err := b.Submit(1, "player", "item", tc.cmd); err == nil {
			if tc.want != "" {
				t.Fatalf("%s: 应拒绝 %s", tc.name, tc.want)
			}
			// 提交成功会占用指令：撤销以便下一个用例继续
			b.cmds["player"] = nil
			b.needs = []string{"player", "pet"}
			continue
		} else if err.Code != tc.want {
			t.Fatalf("%s: 错误码 = %s，应为 %s（%s）", tc.name, err.Code, tc.want, err.Message)
		}
	}
}

func TestUseItemHeals(t *testing.T) {
	b := newBattleWithSide(t, itemSide(), "win", rollRnd(9999))
	you := b.PlayerUnit()
	you.HP = 100 // 缺 200 血，回复量足够时 delta 应顶满
	if _, err := b.Submit(1, "player", "item", Cmd{Slot: 5}); err != nil {
		t.Fatalf("提交回复肉被拒: %v", err)
	}
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "item" || evs[0].Name != "普通的肉" {
		t.Fatalf("应产生一个 item 事件: %+v", evs)
	}
	if evs[0].Slot != 5 {
		t.Fatalf("事件应带消耗的槽位 5: %+v", evs[0])
	}
	var eff *Effect
	for i := range evs[0].Effects {
		if evs[0].Effects[i].Target == 0 {
			eff = &evs[0].Effects[i]
		}
	}
	if eff == nil || eff.Result != "heal" || eff.Delta <= 0 {
		t.Fatalf("应有对目标 0 的 heal 效果: %+v", evs[0].Effects)
	}
	if you.HP != 100+eff.Delta {
		t.Fatalf("血量应加上 delta: hp=%d delta=%d", you.HP, eff.Delta)
	}
	if it := b.sides[0].Items[5]; it.Name != "" {
		t.Fatalf("消耗后快照应置空: %+v", it)
	}
}

func TestUseItemCuresStatus(t *testing.T) {
	b := newBattleWithSide(t, itemSide(), "win", rollRnd(9999))
	you := b.PlayerUnit()
	you.Status.Poison = 5 // 开局中毒
	if _, err := b.Submit(1, "player", "item", Cmd{Slot: 6}); err != nil {
		t.Fatalf("提交解状态的肉被拒: %v", err)
	}
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "item" || evs[0].Name != "治疗中毒的肉" {
		t.Fatalf("应产生一个 item 事件: %+v", evs)
	}
	var eff *Effect
	for i := range evs[0].Effects {
		if evs[0].Effects[i].Target == 0 {
			eff = &evs[0].Effects[i]
		}
	}
	if eff == nil || eff.Result != "none" || len(eff.StatusRemove) != 1 || eff.StatusRemove[0] != "poison" {
		t.Fatalf("应有解状态效果: %+v", evs[0].Effects)
	}
	if you.Status.Poison != 0 {
		t.Fatalf("毒应被解除: %+v", you.Status)
	}
}

func TestUseItemFullHealsWholeSide(t *testing.T) {
	side := testSide()
	side.Items[5] = BattleItem{ID: 2013, Name: "莉儿的爱心糖果"} // 解“全”，全己方
	side.Items[6] = BattleItem{ID: 1232, Name: "结婚蛋糕"}       // 体600，全己方、战斗专用
	b := newBattleWithSide(t, side, "win", rollRnd(9999))
	you, pet := b.PlayerUnit(), b.PetUnit()
	you.HP, pet.HP = 50, 80
	you.Status.Drunk = 3
	// 结婚蛋糕不带 target（Target -1）：对己方全体回血。
	if _, err := b.Submit(1, "player", "item", Cmd{Slot: 6, Target: -1}); err != nil {
		t.Fatalf("提交蛋糕被拒: %v", err)
	}
	evs := b.act(you)
	var healed, healedPet bool
	for _, ev := range evs {
		for _, e := range ev.Effects {
			if e.Result == "heal" && e.Target == 0 && e.Delta > 0 {
				healed = true
			}
			if e.Result == "heal" && e.Target == pet.No && e.Delta > 0 {
				healedPet = true
			}
		}
	}
	if !healed || !healedPet {
		t.Fatalf("全己方的蛋糕应回自己和宠物: %+v", evs)
	}
	if you.HP != you.MaxHP || pet.HP != pet.MaxHP {
		t.Fatalf("体600 应把双方血量回满: %d/%d", you.HP, pet.HP)
	}
	_ = gamedata.RecoveryPower // 保持 import（公式在 gamedata 里测）
}
