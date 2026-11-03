package gamedata

import "testing"

// STAGE-251：1 级宠物目录的数据口径。生成器（tools/gen-pet-encounters.py）产出的表要满足这里的不变式，
// 服务端能否开战的交叉校验在 server/character/encounter_pet_catalog_test.go。

func TestPetEncounterPetsSortedAndUnique(t *testing.T) {
	pets := PetEncounterPets()
	if len(pets) == 0 {
		t.Fatal("1 级宠物目录是空的")
	}
	seen := map[int]bool{}
	for i, p := range pets {
		if seen[p.ID] {
			t.Errorf("模板 %d 重复", p.ID)
		}
		seen[p.ID] = true
		if i > 0 {
			prev := pets[i-1]
			if prev.SortKey > p.SortKey || (prev.SortKey == p.SortKey && prev.ID > p.ID) {
				t.Errorf("目录没有按拼音序（GBK 字节序）排好：%s(%d) 排在 %s(%d) 前面", prev.Name, prev.ID, p.Name, p.ID)
			}
		}
	}
}

func TestPetEncounterOnlyRealPets(t *testing.T) {
	// 深蓝色石头（599）是 1 级可遇的非宠物模板，必须剔除。
	if _, ok := PetEncounterPetByID(599); ok {
		t.Error("非宠物模板 599（深蓝色石头）不该进目录")
	}
	for _, p := range PetEncounterPets() {
		if t2, ok := PetByID(p.ID); !ok || t2.PetFlag != 1 {
			t.Errorf("目录里的模板 %d（%s）不是 PETFLG=1 的宠物", p.ID, p.Name)
		}
	}
}

func TestPetEncounterSpotsAreUsable(t *testing.T) {
	counts := map[int]int{}
	for _, s := range PetEncounterSpots() {
		counts[s.PetID]++
		if _, ok := PetEncounterPetByID(s.PetID); !ok {
			t.Errorf("可遇点引用了目录外的宠物 %d", s.PetID)
		}
		if !FormalFloor(s.Floor) {
			t.Errorf("区域 %s 在地图 %d 上，不在快捷传送目录里", s.AreaID, s.Floor)
		}
		area, ok := EncounterAreaByKey(s.AreaID)
		if !ok {
			t.Errorf("区域 %s 不存在", s.AreaID)
			continue
		}
		if area.ZOrder <= 0 || area.NeverEncounter() {
			t.Errorf("区域 %s 不会遇敌（zorder=%d，CEP 上限=%d）", s.AreaID, area.ZOrder, area.CEPMax)
		}
		if !area.Contains(s.Anchor.X, s.Anchor.Y) {
			t.Errorf("区域 %s 的落点 (%d,%d) 不在区域里", s.AreaID, s.Anchor.X, s.Anchor.Y)
		}
		if s.Anchor.X < s.Rect.X1 || s.Anchor.X > s.Rect.X2 || s.Anchor.Y < s.Rect.Y1 || s.Anchor.Y > s.Rect.Y2 {
			t.Errorf("区域 %s 的落点 (%d,%d) 不在裁后的矩形里", s.AreaID, s.Anchor.X, s.Anchor.Y)
		}
		if got := EncounterAreaAt(s.Floor, s.Anchor.X, s.Anchor.Y); got == nil || got.Key != s.AreaID {
			t.Errorf("落点 (%d,%d) 实际归属的区域不是 %s", s.Anchor.X, s.Anchor.Y, s.AreaID)
		}
	}
	for _, p := range PetEncounterPets() {
		if p.SpotCount != counts[p.ID] {
			t.Errorf("%s 的 SpotCount=%d，实际可遇点 %d", p.Name, p.SpotCount, counts[p.ID])
		}
		if p.SpotCount == 0 {
			t.Errorf("%s 没有可遇点却在目录里", p.Name)
		}
	}
}

func TestPetEncounterSearchTargets(t *testing.T) {
	// 任务书的验收例子：民间叫法与系列标签要能从数据里找到。
	want := map[string]int{"蓝人龙": 92, "红蛙": 234, "金鸡": 292, "滑板": 81}
	for alias, id := range want {
		if !hasAliasFor(alias, id) {
			t.Errorf("俗称「%s」应挂在模板 %d 上", alias, id)
		}
	}
	for _, id := range []int{91, 92, 94} {
		if p, ok := PetEncounterPetByID(id); !ok || p.Series != "人龙系" {
			t.Errorf("模板 %d 应属于人龙系", id)
		}
	}
	if p, ok := PetEncounterPetByID(92); !ok || len(p.Aliases) == 0 {
		t.Error("扬奇洛斯应带俗称")
	}
}

func TestPetEncounterSpotFor(t *testing.T) {
	spots := PetEncounterSpots()
	if len(spots) == 0 {
		t.Fatal("没有可遇点")
	}
	first := spots[0]
	got, ok := PetEncounterSpotFor(first.PetID, first.AreaID)
	if !ok || got.Anchor != first.Anchor {
		t.Errorf("按宠物与区域找不到第一个可遇点")
	}
	if _, ok := PetEncounterSpotFor(first.PetID, "no-such-area"); ok {
		t.Error("不存在的区域不该找到可遇点")
	}
}

func hasAliasFor(alias string, id int) bool {
	p, ok := PetEncounterPetByID(id)
	if !ok {
		return false
	}
	for _, a := range p.Aliases {
		if a == alias {
			return true
		}
	}
	return false
}
