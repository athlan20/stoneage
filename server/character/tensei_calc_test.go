package character

import (
	"errors"
	"testing"
)

// STAGE-266：转生公式的单测，算例来自需求（ans 与四项合计要对）。

// tenseiCheck 校验转生结果：ans、累计值、转生次数、能力点，以及四项合计与 ans 的偏差不超过 2。
func tenseiCheck(t *testing.T, in TenseiInput, wantAns, wantTrans, wantQuest, wantLevel int) TenseiResult {
	t.Helper()
	res, err := CalcTensei(in)
	if err != nil {
		t.Fatalf("CalcTensei 返回错误：%v", err)
	}
	if res.Ans != wantAns {
		t.Errorf("ans = %d，want %d", res.Ans, wantAns)
	}
	if res.Transmigration != wantTrans {
		t.Errorf("转生次数 = %d，want %d", res.Transmigration, wantTrans)
	}
	if res.TransQuestTotal != wantQuest || res.TransLevelTotal != wantLevel {
		t.Errorf("累计任务/等级 = %d/%d，want %d/%d", res.TransQuestTotal, res.TransLevelTotal, wantQuest, wantLevel)
	}
	if res.SkillPoints != wantTrans*10 {
		t.Errorf("能力点 = %d，want %d", res.SkillPoints, wantTrans*10)
	}
	sum := res.Vital + res.Strength + res.Toughness + res.Dexterity
	if d := sum - res.Ans; sum != 0 && (d > 2 || d < -2) {
		t.Errorf("四项合计 %d 与 ans %d 偏差 %d，超过 2", sum, res.Ans, d)
	}
	return res
}

// 首转：四项和 257，累计任务 0→20，累计等级 0→80，n=1，ans=25。
func TestTenseiFirstTrans(t *testing.T) {
	in := TenseiInput{Vital: 60, Strength: 60, Toughness: 70, Dexterity: 67, Level: 80}
	res := tenseiCheck(t, in, 25, 1, 20, 80)
	// 60/257×25≈5.84→6，70→6.81→7，67→6.52→7
	if res.Vital != 6 || res.Strength != 6 || res.Toughness != 7 || res.Dexterity != 7 {
		t.Errorf("四项 = %d/%d/%d/%d，want 6/6/7/7", res.Vital, res.Strength, res.Toughness, res.Dexterity)
	}
}

// 二转：四项和 422，累计任务 20→40，累计等级 80→210（第二次加 130，等级 150 按 130 算），n=2，ans=55。
func TestTenseiSecondTrans(t *testing.T) {
	in := TenseiInput{Vital: 100, Strength: 100, Toughness: 110, Dexterity: 112, Level: 150,
		Transmigration: 1, TransQuestTotal: 20, TransLevelTotal: 80}
	res := tenseiCheck(t, in, 55, 2, 40, 210)
	// 100/422×55≈13.03→13，110→14.34→14，112→14.60→15
	if res.Vital != 13 || res.Strength != 13 || res.Toughness != 14 || res.Dexterity != 15 {
		t.Errorf("四项 = %d/%d/%d/%d，want 13/13/14/15", res.Vital, res.Strength, res.Toughness, res.Dexterity)
	}
}

// 五转满配：四项和 566，累计任务 80→100，累计等级 520→650（加 130），n=5，ans=128。
// 任务书写的是“650-140=510”，但等级只按 130 计入，前值应为 520 才得到 650，这里按 520 写。
func TestTenseiFifthFull(t *testing.T) {
	in := TenseiInput{Vital: 140, Strength: 140, Toughness: 140, Dexterity: 146, Level: 130,
		Transmigration: 4, TransQuestTotal: 80, TransLevelTotal: 520}
	res := tenseiCheck(t, in, 128, 5, 100, 650)
	// 140/566×128≈31.66→32（三项），146→33.02→33；合计 129，与 128 只差 1
	if res.Vital != 32 || res.Strength != 32 || res.Toughness != 32 || res.Dexterity != 33 {
		t.Errorf("四项 = %d/%d/%d/%d，want 32/32/32/33", res.Vital, res.Strength, res.Toughness, res.Dexterity)
	}
	if res.SkillPoints != 50 {
		t.Errorf("能力点 = %d，want 50", res.SkillPoints)
	}
}

// 旧转生次数已到上限（5 次）时不能转，返回错误且结果为空。
func TestTenseiMaxedRefused(t *testing.T) {
	in := TenseiInput{Vital: 60, Strength: 60, Toughness: 70, Dexterity: 67, Level: 130,
		Transmigration: TenseiMaxTrans, TransQuestTotal: 100, TransLevelTotal: 650}
	res, err := CalcTensei(in)
	if !errors.Is(err, ErrTenseiMaxed) {
		t.Fatalf("err = %v，want ErrTenseiMaxed", err)
	}
	if res != (TenseiResult{}) {
		t.Errorf("不可转时结果应为空，得到 %+v", res)
	}
}

// 等级超过 130 只按 130 累计：等级 200 和等级 130 的结果必须一样。
func TestTenseiLevelCapped(t *testing.T) {
	base := TenseiInput{Vital: 60, Strength: 60, Toughness: 70, Dexterity: 67}
	capped := base
	capped.Level = 130
	over := base
	over.Level = 200
	want, err := CalcTensei(capped)
	if err != nil {
		t.Fatal(err)
	}
	got, err := CalcTensei(over)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("等级 200 的结果 %+v，与等级 130 的 %+v 不同", got, want)
	}
	if got.TransLevelTotal != 130 {
		t.Errorf("累计等级 = %d，want 130", got.TransLevelTotal)
	}
}

// 四项合计为 0 不 panic，四项全部为 0；ans 仍按公式算（只剩累计项：20/4 + (80-85)/4 = 3.75 → 3）。
func TestTenseiZeroTotal(t *testing.T) {
	res := tenseiCheck(t, TenseiInput{Level: 80}, 3, 1, 20, 80)
	if res.Vital != 0 || res.Strength != 0 || res.Toughness != 0 || res.Dexterity != 0 {
		t.Errorf("总点数为 0 时四项应全为 0，得到 %+v", res)
	}
}

// 负的分项归 0：体力为负时按比例算出的负数不能写进去。
func TestTenseiNegativeShareClampedToZero(t *testing.T) {
	in := TenseiInput{Vital: -20, Strength: 100, Toughness: 100, Dexterity: 100, Level: 80}
	res, err := CalcTensei(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Vital != 0 {
		t.Errorf("体力 = %d，负数应归 0", res.Vital)
	}
	if res.Ans != 27 {
		t.Errorf("ans = %d，want 27", res.Ans)
	}
}
