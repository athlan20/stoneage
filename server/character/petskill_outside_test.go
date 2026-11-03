package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// 不连数据库：直接用内存里的角色检查战斗外使用宠物技能的各个失败分支（STAGE-144）。
func TestCheckPetSkillOutside(t *testing.T) {
	r := &record{Pets: []petRec{
		// 0 号宠物：1 号栏位攻击（战斗技能），2 号栏位加工（战斗外技能），3 号栏位空。
		{Slot: 0, TemplateID: 1, Name: "乌力", Skills: [gamedata.PetSkillSlots]int{1, 200, -1, -1, -1, -1, -1}},
	}}
	cases := []struct {
		name  string
		slot  int
		skill int
		code  string
	}{
		{"没有宠物的栏位", 1, 1, protocol.CodeNoPet},
		{"技能栏位是空的", 0, 3, protocol.CodeSkillUnusable},
		{"战斗技能战斗外不能用", 0, 1, protocol.CodeSkillUnusable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			perr := checkPetSkillOutside(r, c.slot, c.skill)
			if perr == nil {
				t.Fatalf("应该失败（%s），现在通过了", c.code)
			}
			if perr.Code != c.code {
				t.Errorf("错误码 %s，应为 %s（%s）", perr.Code, c.code, perr.Message)
			}
			if perr.Message == "" {
				t.Error("失败要有给玩家看的说明")
			}
		})
	}
}

// 加工、料理是战斗外技能，检查通过（由 merge.go 合成，STAGE-207）；技能栏位的判断见 mergeSkillAt。
func TestCheckPetSkillOutsideMergeSkillPasses(t *testing.T) {
	r := &record{Pets: []petRec{
		{Slot: 0, TemplateID: 1062, Name: "凯比", Skills: [gamedata.PetSkillSlots]int{1, 2, 200, 201, -1, -1, -1}},
	}}
	for _, skill := range []int{3, 4} {
		if perr := checkPetSkillOutside(r, 0, skill); perr != nil {
			t.Errorf("技能栏位 %d 是合成，检查应通过，得到 %s（%s）", skill, perr.Code, perr.Message)
		}
		if !mergeSkillAt(&r.Pets[0], skill) {
			t.Errorf("技能栏位 %d 应判为合成", skill)
		}
	}
	if mergeSkillAt(&r.Pets[0], 1) || mergeSkillAt(&r.Pets[0], 5) || mergeSkillAt(nil, 3) {
		t.Error("攻击、空栏位、没有宠物都不是合成")
	}
}

// 场合只决定能不能在战斗外用：攻击是战斗场合，加工、料理是战斗外场合（与 petskill.txt 一致）。
func TestPetSkillFieldForOutsideUse(t *testing.T) {
	for _, id := range []int{1, 20, 60} {
		sk, ok := gamedata.PetSkillByID(id)
		if !ok || sk.Field != gamedata.PetSkillFieldBattle {
			t.Errorf("技能 %d 应是战斗场合，得到 %+v", id, sk)
		}
	}
	for _, id := range []int{200, 201} {
		sk, ok := gamedata.PetSkillByID(id)
		if !ok || sk.Field != gamedata.PetSkillFieldMap || sk.Func != "PETSKILL_Merge" {
			t.Errorf("技能 %d 应是战斗外场合的 PETSKILL_Merge，得到 %+v", id, sk)
		}
	}
}
