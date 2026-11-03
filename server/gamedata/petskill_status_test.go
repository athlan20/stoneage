package gamedata

import "testing"

// TestParseStatusChangeRows 对照 petskill.txt 的六个异常攻击（STAGE-75）。
func TestParseStatusChangeRows(t *testing.T) {
	want := map[int]StatusChangeArg{
		60:  {Status: "poison", Turn: 3, AtkPct: -30},    // 毒攻击
		61:  {Status: "poison", Turn: 5, AtkPct: -50},    // 猛毒攻击
		80:  {Status: "stone", Turn: 3, AtkPct: -30},     // 石化攻击
		90:  {Status: "confusion", Turn: 3, AtkPct: -30}, // 混乱攻击
		100: {Status: "drunk", Turn: 3, AtkPct: -30},     // 泥醉攻击
		110: {Status: "sleep", Turn: 3, AtkPct: -30},     // 催眠攻击
	}
	for id, w := range want {
		s, ok := PetSkillByID(id)
		if !ok {
			t.Fatalf("技能 %d 不存在", id)
		}
		got, ok := s.ParseStatusChange()
		if !ok {
			t.Errorf("技能 %d（%s）解析失败：%q", id, s.Name, s.Arg)
			continue
		}
		if got != w {
			t.Errorf("技能 %d（%s）= %+v，期望 %+v", id, s.Name, got, w)
		}
	}
}

// TestParseStatusChangeDefaults 没写 turn 时用原版默认的 3 回合；“全”不是异常攻击的状态。
func TestParseStatusChangeDefaults(t *testing.T) {
	got, ok := PetSkill{Arg: "眠 攻%-30"}.ParseStatusChange()
	if !ok || got.Turn != statusChangeDefaultTurn || got.Status != "sleep" || got.AtkPct != -30 {
		t.Errorf("眠 攻%%-30 = %+v, %v，期望回合数默认 %d", got, ok, statusChangeDefaultTurn)
	}
	if _, ok := (PetSkill{Arg: "全 turn 3"}).ParseStatusChange(); ok {
		t.Error("“全”不应被当成异常攻击的状态")
	}
	if _, ok := (PetSkill{Arg: ""}).ParseStatusChange(); ok {
		t.Error("空参数不应解析成功")
	}
}
