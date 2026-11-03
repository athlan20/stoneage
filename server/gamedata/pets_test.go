package gamedata

import "testing"

func TestPetByID(t *testing.T) {
	// enemybase.txt 的第一行就是乌力（docs/ui.md 宠物页的示例宠物）。
	p, ok := PetByID(1)
	if !ok {
		t.Fatal("宠物编号 1 应该存在")
	}
	want := PetTemplate{
		ID: 1, Name: "乌力", Image: 100250, SkillSlots: 7, PetFlag: 1, Size: 0, LimitLevel: 0,
		InitNum: 10, LvUpPoint: 4, Base: Growth{20, 12, 15, 25}, ModAI: 150, Get: 11,
		Earth: 80, Water: 20, Fire: 0, Wind: 0, Rare: 0, Skills: [PetSkillSlots]int{1, -1, -1, -1, -1, -1, -1},
	}
	if p != want {
		t.Errorf("乌力解析错了:\n got %+v\nwant %+v", p, want)
	}
	if _, ok := PetByID(1 << 30); ok {
		t.Error("不存在的宠物编号不应找到")
	}
}

func TestPetTableCount(t *testing.T) {
	PetByID(1) // 触发加载
	if len(petTable) != 952 {
		t.Errorf("宠物模板应有 952 条，现在 %d", len(petTable))
	}
}

func TestPetSkillSlotsKeepGaps(t *testing.T) {
	// 旧数据里有技能栏位中间空着的宠物（如 7 行）：栏位位置要保留，不能往前挤。
	gap := 0
	for _, p := range petTable {
		seenEmpty := false
		for _, s := range p.Skills {
			if s < 0 {
				seenEmpty = true
			} else if seenEmpty {
				gap++
				break
			}
		}
	}
	if gap != 7 {
		t.Errorf("有空栏位夹在技能中间的宠物应有 7 个，现在 %d", gap)
	}
}

func TestPetSkillName(t *testing.T) {
	if name, ok := PetSkillName(1); !ok || name != "攻击" {
		t.Errorf("技能 1 = %q, %v，应为 攻击", name, ok)
	}
	if name, ok := PetSkillName(0); !ok || name != "待机" {
		t.Errorf("技能 0 = %q, %v，应为 待机", name, ok)
	}
	if _, ok := PetSkillName(1 << 30); ok {
		t.Error("不存在的技能编号不应找到")
	}
}
