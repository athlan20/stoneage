package gamedata

import "testing"

// 出生送宠物（STAGE-62）：出生地宠物按出生地，配套虎和雷龙按身体图像编号查 MetamoList。
func TestNewPlayerPetIDs(t *testing.T) {
	cases := []struct {
		name           string
		hometown, b, c int
		want           [3]int
	}{
		// 外形 3（酷哥）颜色 2 → 身体图像 100070，MetamoList 落在黄行第 3 列：虎 194、雷龙 253。
		{"萨姆吉尔", 0, 3, 2, [3]int{2, 194, 253}},
		{"玛丽娜丝", 1, 3, 2, [3]int{112, 194, 253}},
		{"加加村", 2, 3, 2, [3]int{102, 194, 253}},
		{"卡鲁它那", 3, 3, 2, [3]int{34, 194, 253}},
		// 小矮子、颜色 0 → 100000，红行第 0 列：虎 193、雷龙 252。
		{"小矮子红", 0, 0, 0, [3]int{2, 193, 252}},
		// 赛亚人、颜色 1 → 100025，红行第 1 列：虎 193、雷龙 255。
		{"赛亚人绿", 0, 1, 1, [3]int{2, 193, 255}},
		// 辫子男孩、颜色 3 → 100055，红行第 2 列：虎 193、雷龙 251。
		{"辫子男孩灰", 0, 2, 3, [3]int{2, 193, 251}},
		// 手套女（外形 10）颜色 0 → 100200，红行第 10 列：虎 193、雷龙 253。
		{"手套女红", 0, 10, 0, [3]int{2, 193, 253}},
		// 辣妹（外形 11）颜色 2 → 100230，红行第 11 列：虎 193、雷龙 255。
		{"辣妹黄", 0, 11, 2, [3]int{2, 193, 255}},
	}
	for _, tc := range cases {
		got := NewPlayerPetIDs(tc.hometown, tc.b, tc.c)
		if len(got) != 3 {
			t.Errorf("%s：应返回 3 只，得到 %v", tc.name, got)
			continue
		}
		if [3]int{got[0], got[1], got[2]} != tc.want {
			t.Errorf("%s：得到 %v，应为 %v", tc.name, got, tc.want)
		}
	}
}

// 出生送的宠物模板都要存在且能当宠物（PETFLG > 0）。
func TestNewPlayerPetTemplatesArePets(t *testing.T) {
	for hometown := 0; hometown < len(Hometowns); hometown++ {
		for body := 0; body < BodyCount; body++ {
			for color := 0; color < ColorCount; color++ {
				ids := NewPlayerPetIDs(hometown, body, color)
				if len(ids) != 3 {
					t.Fatalf("出生地 %d 外形 %d 颜色 %d 没算出 3 只宠物", hometown, body, color)
				}
				for _, id := range ids {
					tmpl, ok := PetByID(id)
					if !ok {
						t.Errorf("宠物模板 %d 不存在", id)
						continue
					}
					if tmpl.PetFlag <= 0 {
						t.Errorf("模板 %d（%s）PETFLG=%d，不能当宠物", id, tmpl.Name, tmpl.PetFlag)
					}
				}
			}
		}
	}
}

func TestNewPlayerPetIDsOutOfRange(t *testing.T) {
	if NewPlayerPetIDs(9, 0, 0) != nil {
		t.Error("出生地越界应返回 nil")
	}
	if NewPlayerPetIDs(0, BodyCount, 0) != nil {
		t.Error("外形越界应返回 nil")
	}
	if NewPlayerPetIDs(0, 0, ColorCount) != nil {
		t.Error("颜色越界应返回 nil")
	}
}
