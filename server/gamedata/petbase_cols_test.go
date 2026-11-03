package gamedata

import "testing"

// 技能在 petbase.tsv 的第 19–25 列（风属性之后），稀有度是最后一列（STAGE-73）。
// 数据对照原版服务端 enemybase.txt：乌力乌力（2）的 PETSKILL1、2 是 1、2；格鲁西斯（193）RARE=0；
// 史多罗（152）RARE=2 而品质不是 2。
func TestPetbaseSkillAndRareColumns(t *testing.T) {
	tm, ok := PetByID(2)
	if !ok {
		t.Fatal("2 号模板应存在")
	}
	if want := [PetSkillSlots]int{1, 2, -1, -1, -1, -1, -1}; tm.Skills != want {
		t.Errorf("乌力乌力的技能应是 %v，实际 %v", want, tm.Skills)
	}
	if tm.Wind != 0 || tm.Fire != 60 {
		t.Errorf("属性列不该错位：火 60、风 0，实际 火 %d、风 %d", tm.Fire, tm.Wind)
	}
	if got, _ := PetByID(193); got.Rare != 0 {
		t.Errorf("格鲁西斯的稀有度应是 0，实际 %d", got.Rare)
	}
	if got, _ := PetByID(152); got.Rare != 2 {
		t.Errorf("史多罗的稀有度应是 2，实际 %d", got.Rare)
	}
}
