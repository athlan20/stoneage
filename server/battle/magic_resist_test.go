// STAGE-228 魔抗系精灵（魔法防御、冰、电、火之精灵）：挂法、互斥、不掷骰、回合递减，以及魔抗对守方伤害的加成。
// 对照原版 battle_magic.c 的 MAGIC_MagicStatusChange_Battle、BATTLE_MultiMagicStatusChange 与 BATTLE_MultiAttMagic。

package battle

import (
	"reflect"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 四种魔抗系进协议列表（10.2），顺序在沉默之后；它们不算异常（原版 StatusAttackCheck 不看 MagicTbl）。
func TestResistStatusListNames(t *testing.T) {
	s := StatusSet{ResistMagic: 3, ResistIce: 3}
	want := []string{"magic_resist", "ice_resist"}
	if got := s.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("status 列表应为 %v，实际 %v", want, got)
	}
	if s.anyBad() {
		t.Fatalf("魔抗系不算异常状态")
	}
}

// 挂载不掷骰：掷到 99（任何百分比判定都会失败）也照样挂上；回合数是精灵的 turn（不 +1），加成是精灵的数值。
func TestResistLandsWithoutRoll(t *testing.T) {
	b := newBattleWithSide(t, magicSide(392), "win", rollRnd(99))
	you := b.PlayerUnit()
	tpl, _ := gamedata.MagicByID(392)
	b.castMagic(you, tpl, Cmd{Target: you.No})
	if you.Status.ResistMagic != 3 || you.Status.ResistPct != 20 {
		t.Fatalf("魔法防御应挂上 3 回合、加成 20: %+v", you.Status)
	}
}

// 互斥：带着任何一种魔抗系状态，再中别的魔抗系都不中（记 miss），已有的那种不被覆盖也不刷新回合。
func TestResistMutuallyExclusive(t *testing.T) {
	spells := map[string]int{"magic_resist": 392, "fire_resist": 419, "thunder_resist": 418, "ice_resist": 417}
	for held, heldID := range spells {
		for status, id := range spells {
			b := newBattleWithSide(t, magicSide(), "win", rollRnd(5))
			you := b.PlayerUnit()
			heldTpl, _ := gamedata.MagicByID(heldID)
			b.castMagic(you, heldTpl, Cmd{Target: you.No})
			*you.Status.resistField(held) = 1 // 把回合数压到 1，确认没被刷新
			tpl, _ := gamedata.MagicByID(id)
			evs := b.castMagic(you, tpl, Cmd{Target: you.No})
			if eff := effectOn(evs[0], you.No); eff == nil || eff.Result != "miss" {
				t.Errorf("已带 %s 时施放 %s 应 miss: %+v", held, status, evs[0].Effects)
			}
			if *you.Status.resistField(held) != 1 || (status != held && *you.Status.resistField(status) != 0) {
				t.Errorf("已带 %s 时施放 %s 状态不应变化: %+v", held, status, you.Status)
			}
		}
	}
}

// 回合递减：挂上 3 回合，回合末每次减一，减到 0 时发 status_remove；火抗挂着时不改任何数值。
func TestResistTurnsCountDown(t *testing.T) {
	b := newBattleWithSide(t, magicSide(419), "win", rollRnd(5))
	you := b.PlayerUnit()
	tpl, _ := gamedata.MagicByID(419)
	b.castMagic(you, tpl, Cmd{Target: you.No})
	if you.Status.ResistFire != 3 {
		t.Fatalf("火之精灵应挂上 3 回合: %+v", you.Status)
	}
	for _, want := range []int{2, 1, 0} {
		evs := b.statusTick()
		if got := you.Status.ResistFire; got != want {
			t.Fatalf("回合末后剩余回合应为 %d，实际 %d", want, got)
		}
		if want == 0 {
			if len(evs) == 0 {
				t.Fatalf("减到 0 应发 status_remove")
			}
			if eff := effectOn(evs[0], you.No); eff == nil || !reflect.DeepEqual(eff.StatusRemove, []string{"fire_resist"}) {
				t.Fatalf("减到 0 的 status_remove 应为 fire_resist: %+v", evs)
			}
		}
	}
}

// 全体：魔法防御 Lv2（393）挂到己方全体（人物和宠物都中），敌方不中。
func TestResistWholeSideHitsAllies(t *testing.T) {
	b := newBattleWithSide(t, magicSide(393), "win", rollRnd(5))
	you, pet := b.PlayerUnit(), b.PetUnit()
	m := sturdy(b, 15)
	tpl, _ := gamedata.MagicByID(393)
	b.castMagic(you, tpl, Cmd{Target: -1})
	if you.Status.ResistMagic != 3 || pet.Status.ResistMagic != 3 {
		t.Fatalf("己方全体应都挂上魔抗: 人物 %+v，宠物 %+v", you.Status, pet.Status)
	}
	if m.Status.hasResist() {
		t.Fatalf("敌方不应中魔抗: %+v", m.Status)
	}
}

// 魔抗加成进伤害（BATTLE_MultiAttMagic 的 def_magic_resist）：守方抗性 36 带魔抗 20% 时变成 43，
// 威力 150 的火精灵（Lv2、熟练度 80、随机项 0）APower 从 162 降到 133；火抗挂着、抗性为 0 时不变。
func TestResistScalesDefenderDamage(t *testing.T) {
	b, you, mon := fixFixture(t, 80, 36, 0)
	spell := fireSpell(150, 2)
	if got := resistOf(mon, 2); got != 36 {
		t.Fatalf("无魔抗时抗性应为 36，实际 %d", got)
	}
	if ap := b.magicPower(you, mon, spell, 2); ap != 162 {
		t.Fatalf("无魔抗时 APower 应为 162，实际 %d", ap)
	}
	mon.Status.ResistMagic, mon.Status.ResistPct = 3, 20
	if got := resistOf(mon, 2); got != 43 {
		t.Fatalf("魔抗 20%% 时抗性应为 43，实际 %d", got)
	}
	if ap := b.magicPower(you, mon, spell, 2); ap != 133 {
		t.Fatalf("魔抗 20%% 时 APower 应为 133，实际 %d", ap)
	}
	mon.Status = StatusSet{ResistFire: 3, ResistPct: 20}
	if ap := b.magicPower(you, mon, spell, 2); ap != 162 {
		t.Fatalf("火抗只占槽，APower 应仍为 162，实际 %d", ap)
	}
	mon.MagicResist[2] = 0
	mon.Status = StatusSet{ResistMagic: 3, ResistPct: 20}
	if got := resistOf(mon, 2); got != 0 {
		t.Fatalf("抗性为 0 时魔抗不加成，应为 0，实际 %d", got)
	}
}
