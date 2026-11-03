// 复活类道具（STAGE-262 A 段，对照原版 battle_item.c 的 ITEM_useRessurect 与 ITEM_ResAndDef）的引擎测试：
// 只对倒下的己方单位生效，活着的、敌方的、怪物都拒绝；回复量同复活精灵，封顶最大血量、至少 1；
// 真诚的眼泪、铁壁之石复活后挂上防御次数。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// reviveItemSide 在 testSide 的基础上把几件复活道具放进背包：5 号复活药(100)、6 号复活药(500)、
// 7 号真诚的眼泪（复活 100 + 吸收 1 回合）、8 号铁壁之石（复活 500 + 无效 2 回合）。
func reviveItemSide() *SideInput {
	side := testSide()
	side.Items[5] = BattleItem{ID: 1530, Name: "复活药(100)"}
	side.Items[6] = BattleItem{ID: 1532, Name: "复活药(500)"}
	side.Items[7] = BattleItem{ID: 1815, Name: "真诚的眼泪"}
	side.Items[8] = BattleItem{ID: 1821, Name: "铁壁之石"}
	return side
}

// 复活药(100) 对倒下的宠物（最大血量 200）：RAND(90,110) 取 100（rollRnd(10) 让 Rnd(21) 出 10），
// 血量回到 100，道具扣 1，宠物活着，事件 result 为 heal、down 为假。
func TestReviveItemRevivesDownPet(t *testing.T) {
	b := newBattleWithSide(t, reviveItemSide(), "win", rollRnd(10))
	you, pet := b.PlayerUnit(), b.PetUnit()
	pet.HP, pet.Down = 0, true
	if _, perr := b.Submit(1, "player", "item", Cmd{Slot: 5, Target: 5}); perr != nil {
		t.Fatalf("复活药应能救倒下的宠物: %v", perr)
	}
	evs := b.act(you)
	if len(evs) != 1 || evs[0].Kind != "item" || evs[0].Slot != 5 {
		t.Fatalf("应产生一个带槽位 5 的 item 事件: %+v", evs)
	}
	eff := effectOn(evs[0], 5)
	if eff == nil || eff.Result != "heal" || eff.Delta != 100 || eff.Down || eff.HP == nil || *eff.HP != 100 {
		t.Fatalf("应有复活回 100 血、down 为假的效果: %+v", evs[0].Effects)
	}
	if !pet.Alive() || pet.HP != 100 {
		t.Fatalf("宠物应活着、血量 100: %+v", pet)
	}
	if it := b.sides[0].Items[5]; it.Name != "" {
		t.Fatalf("消耗后快照应置空: %+v", it)
	}
}

// 复活药(500) 对最大血量 200 的宠物：回复超过上限，封顶为最大血量。
func TestReviveItemCapsAtMaxHP(t *testing.T) {
	b := newBattleWithSide(t, reviveItemSide(), "win", rollRnd(50))
	you, pet := b.PlayerUnit(), b.PetUnit()
	pet.HP, pet.Down = 0, true
	if _, perr := b.Submit(1, "player", "item", Cmd{Slot: 6, Target: 5}); perr != nil {
		t.Fatalf("复活药(500) 应能救宠物: %v", perr)
	}
	evs := b.act(you)
	if eff := effectOn(evs[0], 5); eff == nil || eff.Delta != 200 || pet.HP != pet.MaxHP {
		t.Fatalf("血量应封顶为最大血量 200: pet=%+v effects=%+v", pet, evs[0].Effects)
	}
}

// 复活量至少 1：RAND 落在 0 时（复活量为 0），仍然回 1 血，不会是倒下的 0 血。
func TestReviveAmountAtLeastOne(t *testing.T) {
	b := newBattleWithSide(t, testSide(), "win", rollRnd(0))
	pet := b.PetUnit()
	pet.HP, pet.Down = 0, true
	var ev Event
	b.reviveAmount(pet, 1, false, &ev)
	if pet.Down || pet.HP != 1 {
		t.Fatalf("复活量为 0 时也应回 1 血: %+v", pet)
	}
}

// 百分比：参数 25% 按最大血量 200 算 50，RAND(45,55) 取 50（rollRnd(5) 让 Rnd(11) 出 5）。
// 定值 50 在同样的随机下也是 50，两种写法对同一个最大血量给出一样的结果。
func TestReviveAmountPercentOfMaxHP(t *testing.T) {
	cases := []struct {
		name    string
		power   int
		percent bool
	}{
		{"百分比 25%", 25, true},
		{"定值 50", 50, false},
	}
	for _, c := range cases {
		b := newBattleWithSide(t, testSide(), "win", rollRnd(5))
		pet := b.PetUnit()
		pet.HP, pet.Down = 0, true
		var ev Event
		b.reviveAmount(pet, c.power, c.percent, &ev)
		if pet.Down || pet.HP != 50 {
			t.Errorf("%s：血量应为 50，实际 %+v", c.name, pet)
		}
	}
}

// 复活药的目标校验：活着的己方单位（人、宠物）、敌方单位（活着或倒下）、缺省目标都拒绝，道具不扣。
func TestReviveItemRejectsLivingEnemyAndMissingTarget(t *testing.T) {
	b := newBattleWithSide(t, reviveItemSide(), "win", rollRnd(9999))
	sturdy(b, 15).Down = true // 倒下的怪物
	cases := []struct {
		name   string
		target int
		want   string
	}{
		{"缺省目标", -1, protocol.CodeInvalidTarget},
		{"活着的宠物", 5, protocol.CodeInvalidTarget},
		{"活着的人物", 0, protocol.CodeInvalidTarget},
		{"倒下的怪物", 15, protocol.CodeInvalidTarget},
		{"活着的敌人", 10, protocol.CodeInvalidTarget},
	}
	for _, tc := range cases {
		_, perr := b.Submit(1, "player", "item", Cmd{Slot: 5, Target: tc.target})
		expectCode(t, perr, tc.want)
		if perr == nil {
			t.Fatalf("%s 不能用复活药", tc.name)
		}
		if it := b.sides[0].Items[5]; it.Name == "" {
			t.Fatalf("%s：被拒后道具不应扣掉", tc.name)
		}
	}
}

// 真诚的眼泪（复活 100、吸收 1 回合）：复活后宠物挂上吸收次数 1，效果里带 status_add。
// 铁壁之石（复活 500、无效 2 回合）：挂上无效次数 2。
func TestReviveItemDefendAfterRevive(t *testing.T) {
	cases := []struct {
		name  string
		slot  int
		kind  string
		turns int
	}{
		{"真诚的眼泪", 7, defAbsorb, 1},
		{"铁壁之石", 8, defVanish, 2},
	}
	for _, c := range cases {
		b := newBattleWithSide(t, reviveItemSide(), "win", rollRnd(10))
		you, pet := b.PlayerUnit(), b.PetUnit()
		pet.HP, pet.Down = 0, true
		if _, perr := b.Submit(1, "player", "item", Cmd{Slot: c.slot, Target: 5}); perr != nil {
			t.Fatalf("%s 应能救宠物: %v", c.name, perr)
		}
		evs := b.act(you)
		if eff := effectOn(evs[0], 5); eff == nil || eff.Result != "heal" || eff.Down {
			t.Fatalf("%s：应有复活的效果: %+v", c.name, evs[0].Effects)
		}
		if n := *pet.Def.counter(c.kind); n != c.turns {
			t.Errorf("%s：%s 次数 %d，want %d", c.name, c.kind, n, c.turns)
		}
		if !pet.Alive() {
			t.Errorf("%s：宠物应活着", c.name)
		}
	}
}
