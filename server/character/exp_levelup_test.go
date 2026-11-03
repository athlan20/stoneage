package character

import "testing"

// STAGE-57 按级扣除经验：每升一级从经验里扣掉这一级所需的量，剩下的留给下一级
// （原版 CHAR_LevelUpCheck 调 CHAR_HandleExp），不再把经验当跨级累计值。
func TestLevelUpDeductsPerLevel(t *testing.T) {
	cases := []struct {
		name   string
		from   int
		exp    int64
		perPt  int
		to     int
		points int
		remain int64
	}{
		{"差一点不够", 1, 1, 3, 1, 0, 1},
		{"正好升一级清零", 1, 2, 3, 2, 3, 0},
		{"连升两级带余数", 1, 10, 3, 3, 6, 2}, // 10-2-6=2，不够 18
		{"宠物按本级攒", 5, 70, 0, 6, 0, 3},  // 70-67=3
		{"大额连升到 10 级", 1, 1000, 3, 10, 27, 0},
		{"大额差 1 点到 10 级", 1, 999, 3, 9, 24, 343}, // 999-656=343，不够 344
		{"满级不再升也不再扣", 160, 91800799, 3, 160, 0, 91800799},
	}
	for _, c := range cases {
		to, points, remain := levelUp(c.from, c.exp, c.perPt)
		if to != c.to || points != c.points || remain != c.remain {
			t.Errorf("%s：levelUp(%d, %d, %d) = %d, %d, %d，期望 %d, %d, %d",
				c.name, c.from, c.exp, c.perPt, to, points, remain, c.to, c.points, c.remain)
		}
	}
}
