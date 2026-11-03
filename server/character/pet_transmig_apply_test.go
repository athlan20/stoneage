package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-235：转生结果写回宠物的测试（TRANS 0→1、1→2）。

func TestApplyPetTransmigTwice(t *testing.T) {
	p := petRec{
		Slot: 2, Level: 79, Exp: 12345, HP: 5, Rank: 3, Transmigration: 0,
		Base:  gamedata.Growth{Vital: 1, Strength: 1, Toughness: 1, Dexterity: 1},
		Stats: gamedata.Ability{Vital: 9, Strength: 9, Toughness: 9, Dexterity: 9},
	}
	check := func(stage string, wantTrans int) {
		t.Helper()
		if p.Level != 1 || p.Exp != 0 || p.Transmigration != wantTrans || (p.LimitLevel == nil || *p.LimitLevel != -1) {
			t.Fatalf("%s：Level=%d Exp=%d TRANS=%d LIMITLEVEL=%v", stage, p.Level, p.Exp, p.Transmigration, p.LimitLevel)
		}
		if p.Slot != 2 {
			t.Fatalf("%s：栏位被改成 %d，应保持 2", stage, p.Slot)
		}
	}

	// 第一次转生（TRANS 0→1）：能力 {300, 200, 190, 190}，最大 HP = (4×300 + 200 + 190 + 190) ÷ 100 = 17。
	r1 := gamedata.TransResult{Base: gamedata.Growth{Vital: 20, Strength: 20, Toughness: 19, Dexterity: 19}, Rank: 5, Stats: gamedata.Ability{Vital: 300, Strength: 200, Toughness: 190, Dexterity: 190}}
	applyPetTransmig(&p, r1)
	check("TRANS 0→1", 1)
	if p.HP != 17 || p.Rank != 5 || p.Base != r1.Base || p.Stats != r1.Stats {
		t.Fatalf("TRANS 0→1 结果不对：HP=%d Rank=%d Base=%+v Stats=%+v", p.HP, p.Rank, p.Base, p.Stats)
	}

	// 第二次转生（TRANS 1→2）：同样的写入，次数再加一，结果整体替换。
	r2 := gamedata.TransResult{Base: gamedata.Growth{Vital: 24, Strength: 24, Toughness: 23, Dexterity: 23}, Rank: 3, Stats: gamedata.Ability{Vital: 240, Strength: 240, Toughness: 230, Dexterity: 330}}
	applyPetTransmig(&p, r2)
	check("TRANS 1→2", 2)
	if p.HP != gamedata.PetMaxHP(r2.Stats) || p.Rank != 3 || p.Base != r2.Base || p.Stats != r2.Stats {
		t.Fatalf("TRANS 1→2 结果不对：HP=%d Rank=%d Base=%+v Stats=%+v", p.HP, p.Rank, p.Base, p.Stats)
	}
}
