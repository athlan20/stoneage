package character

import (
	"encoding/json"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 不连数据库：直接用内存里的角色检查宠物的协议内容（公式、技能栏位、状态、跳过未知模板）。
func TestPetsViewShape(t *testing.T) {
	r := &record{Level: 1, Charm: gamedata.NewPlayerCharm, Pets: []petRec{
		{
			Slot: 3, TemplateID: 1, Name: "乌力", Level: 5, Exp: 0,
			Stats: gamedata.Ability{Vital: 300, Strength: 200, Toughness: 100, Dexterity: 500},
			HP:    0, State: petStateBattle,
			Skills: [gamedata.PetSkillSlots]int{1, -1, 0, -1, -1, -1, -1},
		},
		{Slot: 4, TemplateID: 1 << 30, Name: "不认识", Level: 1}, // 模板不在表里：跳过
	}}
	out := r.petsView()
	if out["max_slots"] != MaxPetSlots {
		t.Errorf("max_slots = %v，应为 %d", out["max_slots"], MaxPetSlots)
	}
	pets := out["pets"].([]petView)
	if len(pets) != 1 {
		t.Fatalf("应只有 1 只宠物（未知模板跳过），现在 %d", len(pets))
	}
	p := pets[0]
	if p.Slot != 3 || p.Species != "乌力" || p.SpeciesID != 1 || p.Level != 5 {
		t.Errorf("基本信息不对: %+v", p)
	}
	if p.State != "battle" || p.HP != 0 || p.MaxHP != 20 {
		t.Errorf("状态或 HP 不对: state=%s hp=%d max=%d", p.State, p.HP, p.MaxHP)
	}
	if p.Attack != 2 || p.Defense != 1 || p.Agility != 5 {
		t.Errorf("衍生数值不对: 攻击 %d 防御 %d 敏捷 %d", p.Attack, p.Defense, p.Agility)
	}
	if p.Loyalty != 8 || p.NextExp != 67 {
		t.Errorf("忠诚 %d（应 8）、下一级经验 %d（应 67）", p.Loyalty, p.NextExp)
	}
	if p.Elements != (Elements{Earth: 80, Water: 20, Fire: 0, Wind: 0}) {
		t.Errorf("地水火风不对: %+v", p.Elements)
	}
	// 技能栏位保留位置：第 3 栏是“待机”（编号 0），第 2 栏空着不写。目标类型：攻击打敌方，待机不需要目标。
	want := []petSkillView{
		{Slot: 1, ID: 1, Name: "攻击", TargetType: gamedata.TargetEnemy},
		{Slot: 3, ID: 0, Name: "待机", TargetType: gamedata.TargetNone},
	}
	if len(p.Skills) != len(want) || p.Skills[0] != want[0] || p.Skills[1] != want[1] {
		t.Errorf("技能 %+v，应为 %+v", p.Skills, want)
	}
}

// 没有宠物时 pets 是 []（不是 null），客户端按 max_slots 补空栏位。
func TestEmptyPetsEncodeAsArray(t *testing.T) {
	raw, err := json.Marshal((&record{}).petsView())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"max_slots":20,"pets":[]}` {
		t.Errorf("空宠物列表编码为 %s", raw)
	}
}

// 到了等级上限的宠物显示满级（next_exp 为 -1），和角色的规则一致；上限按模板（STAGE-33）。
func TestNextPetExpAtMaxLevel(t *testing.T) {
	limit := gamedata.PetLevelCap
	if got := nextPetExp(limit, limit); got != -1 {
		t.Errorf("%d 级的下一级经验 = %d，应为 -1", limit, got)
	}
	if got := nextPetExp(limit-1, limit); got != gamedata.NextExp(limit-1) || got <= 0 {
		t.Errorf("%d 级的下一级经验 = %d", limit-1, got)
	}
	if got := nextPetExp(10, 10); got != -1 { // 玛蕾菲雅这类模板填了 10 级上限
		t.Errorf("上限 10 级的宠物在 10 级时下一级经验 = %d，应为 -1", got)
	}
}
