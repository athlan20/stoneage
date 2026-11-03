package character

// STAGE-266：人物转生的数值公式（纯函数，无数据库、无随机）。
// 出处：original-data/gmsv/npc/npc_transmigration.c 的 NPC_TransmigrationStatus（844–933）
// 与 NPC_TransCalculation（804–822）。转生次数用 characters.transmigration，累计任务数和累计等级是
// characters.trans_quest_total / trans_level_total（迁移 0027），原版把它们打包在 CHAR_TRANSEQUATION 里。

import (
	"errors"
	"math"
)

const (
	// TenseiMaxTrans 是人物转生次数上限（任务书）。注意：原版 version.h 定义了 _TRANS_6，原版最多 6 转，
	// 第 6 次还有特殊公式，这里先按任务书的 5 次。
	TenseiMaxTrans = 5
	// tenseiQuestPerTrans 是每次转生给累计任务数加的值（任务书）。
	tenseiQuestPerTrans = 20
	// tenseiLevelCap 是计入累计等级的等级上限（原版 npc_transmigration.c:871）。
	tenseiLevelCap = 130
	// tenseiLevelBase 是公式里按转生次数扣的等级基数：扣 n×85（原版 NPC_TransCalculation）。
	tenseiLevelBase = 85
	// tenseiSkillPerTrans 是转生后未分配能力点为 n×10（原版 npc_transmigration.c:487、732）。
	tenseiSkillPerTrans = 10
)

// ErrTenseiMaxed 表示转生次数已到上限，不能再转。
var ErrTenseiMaxed = errors.New("转生次数已达上限")

// TenseiInput 是转生前的角色数值。
type TenseiInput struct {
	Vital, Strength, Toughness, Dexterity int // 四项能力（不含未分配的能力点）
	Level                                 int // 转生时的等级
	Transmigration                        int // 转生前的转生次数
	TransQuestTotal, TransLevelTotal      int // 转生前的累计任务数、累计等级
}

// TenseiResult 是转生后的数值。TransQuestTotal、TransLevelTotal 已经加上本次转生的部分。
type TenseiResult struct {
	Vital, Strength, Toughness, Dexterity int
	Transmigration                        int
	TransQuestTotal, TransLevelTotal      int
	SkillPoints                           int // 转生后的未分配能力点，n×10
	Ans                                   int // 公式结果：四项按比例分配的总量
}

// CalcTensei 算出转生后的数值，步骤与原版一致：先累加，再代入公式。
// 转生次数已到 TenseiMaxTrans 时返回 ErrTenseiMaxed。
func CalcTensei(in TenseiInput) (TenseiResult, error) {
	if in.Transmigration >= TenseiMaxTrans {
		return TenseiResult{}, ErrTenseiMaxed
	}
	n := in.Transmigration + 1
	quest := in.TransQuestTotal + tenseiQuestPerTrans
	level := in.TransLevelTotal + min(in.Level, tenseiLevelCap)
	total := in.Vital + in.Strength + in.Toughness + in.Dexterity

	ans := int(float64(total)/12 + float64(quest)/4 + float64(level-n*tenseiLevelBase)/4)
	res := TenseiResult{
		Transmigration:  n,
		TransQuestTotal: quest,
		TransLevelTotal: level,
		SkillPoints:     n * tenseiSkillPerTrans,
		Ans:             ans,
	}
	if total == 0 {
		// 四项合计为 0 时没有比例可分，四项都是 0（原版除以 0 没有保护，这里不 panic）。
		return res, nil
	}
	res.Vital = tenseiShare(in.Vital, total, ans)
	res.Strength = tenseiShare(in.Strength, total, ans)
	res.Toughness = tenseiShare(in.Toughness, total, ans)
	res.Dexterity = tenseiShare(in.Dexterity, total, ans)
	return res, nil
}

// tenseiShare 按比例分配一项：该项/总点数 × ans，四舍五入到整点，负数归 0。
func tenseiShare(part, total, ans int) int {
	v := math.Floor(float64(part)/float64(total)*float64(ans) + 0.5)
	if v < 0 {
		return 0
	}
	return int(v)
}
