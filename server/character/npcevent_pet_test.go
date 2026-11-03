package character

// 任务脚本的宠物（STAGE-197，docs/quest.md）：条件（纯逻辑，不需要数据库）。
// 收宠物、送宠物的流程测试（需要数据库）在 npcevent_pet_act_test.go。

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

// petFor 造一只随身宠物（栏位、模板、等级、是不是任务送的、状态）。
func petFor(slot, template, level int, given bool) petRec {
	return petRec{Slot: slot, TemplateID: template, Level: level, EventGiven: given, State: petStateRest}
}

// petRecord 造一个只带宠物的角色记录（条件测试用）。
func petRecord(pets ...petRec) *record {
	return &record{Level: 50, Pets: pets}
}

// condHolds 用一条 EVENT 的写法（单个候选组、组内 &）判断条件，和脚本里的写法一致。
func condHolds(r *record, terms ...gamedata.EventTerm) bool {
	return eventCondHolds(r, [][]gamedata.EventTerm{terms}, time.Now())
}

func petTerm(cmp string, level, template, count int) gamedata.EventTerm {
	return gamedata.EventTerm{Op: "PET", Pet: &gamedata.PetAsk{Cmp: cmp, Level: level, Template: template, Count: count}}
}

// PET>0-560*4：有 3 只 560 号不命中，有 4 只才命中；别的编号不算。
func TestPetCondCountsTemplateAndNumber(t *testing.T) {
	want4 := petTerm(">", 0, 560, 4)
	three := petRecord(petFor(0, 560, 10, false), petFor(1, 560, 10, false), petFor(2, 560, 10, false), petFor(3, 999, 10, false))
	if condHolds(three, want4) {
		t.Fatalf("只有 3 只 560 号，PET>0-560*4 不应命中")
	}
	four := petRecord(petFor(0, 560, 10, false), petFor(1, 560, 10, false), petFor(2, 560, 10, false), petFor(3, 560, 10, false))
	if !condHolds(four, want4) {
		t.Fatalf("有 4 只 560 号，PET>0-560*4 应该命中")
	}
	if !condHolds(petRecord(petFor(0, 560, 10, false)), petTerm(">", 0, 560, 0)) {
		t.Fatalf("数量缺省（0）按至少 1 只算")
	}
}

// 等级比较的三种比较符，比的是宠物等级：PET<30-720、PET=77-12、PET>29-191。
func TestPetCondLevelComparators(t *testing.T) {
	cases := []struct {
		name  string
		term  gamedata.EventTerm
		level int
		want  bool
	}{
		{"小于 30，29 级", petTerm("<", 30, 720, 1), 29, true},
		{"小于 30，30 级不算", petTerm("<", 30, 720, 1), 30, false},
		{"等于 77，77 级", petTerm("=", 77, 12, 1), 77, true},
		{"等于 77，78 级不算", petTerm("=", 77, 12, 1), 78, false},
		{"大于 29，30 级", petTerm(">", 29, 191, 1), 30, true},
		{"大于 29，29 级不算", petTerm(">", 29, 191, 1), 29, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			template := tc.term.Pet.Template
			r := petRecord(petFor(0, template, tc.level, false))
			if got := condHolds(r, tc.term); got != tc.want {
				t.Fatalf("命中应该是 %v，实际 %v", tc.want, got)
			}
		})
	}
}

// 任务送的宠物（event_given）不数：只有它的话 PET>0-560 不命中；混在一起只数普通的。
func TestPetCondSkipsEventGiven(t *testing.T) {
	term := petTerm(">", 0, 560, 1)
	if condHolds(petRecord(petFor(0, 560, 10, true)), term) {
		t.Fatalf("任务送的宠物不应该被普通 PET 条件数到")
	}
	mixed := petRecord(petFor(0, 560, 10, true), petFor(1, 560, 10, false))
	if !condHolds(mixed, petTerm(">", 0, 560, 1)) {
		t.Fatalf("一只普通的 560 号应该满足 PET>0-560")
	}
	if condHolds(mixed, petTerm(">", 0, 560, 2)) {
		t.Fatalf("普通的只有 1 只，PET>0-560*2 不应命中")
	}
}

// 骑乘和出战的宠物照样算；寄放在宠物店（不在随身栏位）的不算。
func TestPetCondCountsRideBattleNotPooled(t *testing.T) {
	ride := petFor(0, 560, 10, false)
	ride.State = petStateRide
	battle := petFor(1, 560, 10, false)
	battle.State = petStateBattle
	if !condHolds(petRecord(ride, battle), petTerm(">", 0, 560, 2)) {
		t.Fatalf("骑乘和出战的宠物都应该算")
	}
	pooled := petFor(gamedata.PooledSlotBase, 560, 10, false)
	pooled.State = petStatePooled
	if condHolds(petRecord(pooled), petTerm(">", 0, 560, 1)) {
		t.Fatalf("寄放在宠物店的宠物不应该算")
	}
}

// 没有宠物时 PET 条件不成立。
func TestPetCondNoPets(t *testing.T) {
	if condHolds(petRecord(), petTerm(">", 0, 560, 1)) {
		t.Fatalf("没有宠物时 PET 条件不应成立")
	}
}
