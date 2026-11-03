package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-251：交叉校验。目录（生成器的 Python 口径）里的每个可遇点，都要过服务端真正用的 buildAreaView，
// 否则目录里能选、开战却说不可用。这里不带道具，口径与目录一致。
func TestPetEncounterSpotsMatchServerAreas(t *testing.T) {
	spots := gamedata.PetEncounterSpots()
	if len(spots) == 0 {
		t.Fatal("1 级宠物目录没有可遇点")
	}
	for _, sp := range spots {
		area, ok := gamedata.EncounterAreaByKey(sp.AreaID)
		if !ok {
			t.Errorf("区域 %s 不存在", sp.AreaID)
			continue
		}
		view, ok := buildAreaView(sp.Floor, area, map[int]bool{})
		if !ok || !view.Available {
			t.Errorf("区域 %s（宠物 %d）服务端判定不可用", sp.AreaID, sp.PetID)
			continue
		}
		if view.Anchor != (pointView{X: sp.Anchor.X, Y: sp.Anchor.Y}) {
			t.Errorf("区域 %s 落点：目录 (%d,%d)，服务端 (%d,%d)", sp.AreaID, sp.Anchor.X, sp.Anchor.Y, view.Anchor.X, view.Anchor.Y)
		}
		if view.Rect != (rectView{X1: sp.Rect.X1, Y1: sp.Rect.Y1, X2: sp.Rect.X2, Y2: sp.Rect.Y2}) {
			t.Errorf("区域 %s 矩形：目录 %+v，服务端 %+v", sp.AreaID, sp.Rect, view.Rect)
		}
		if view.MaxEnemies != sp.MaxEnemies {
			t.Errorf("区域 %s 同场上限：目录 %d，服务端 %d", sp.AreaID, sp.MaxEnemies, view.MaxEnemies)
		}
		if !areaHasTemplate(view, sp.PetID) {
			t.Errorf("区域 %s 的敌人里没有宠物模板 %d", sp.AreaID, sp.PetID)
		}
	}
}

func TestPetEncounterCatalogViewShape(t *testing.T) {
	v := petEncounterCatalogView()
	if v["catalog_version"] == "" {
		t.Error("catalog_version 为空")
	}
	pets, ok := v["pets"].([]petCatalogPetView)
	if !ok || len(pets) != len(gamedata.PetEncounterPets()) {
		t.Fatalf("pets 不是完整的目录：%T", v["pets"])
	}
	spots, ok := v["spots"].([]petCatalogSpotView)
	if !ok || len(spots) != len(gamedata.PetEncounterSpots()) {
		t.Fatalf("spots 不是完整的目录：%T", v["spots"])
	}
	for _, s := range spots {
		if s.AlsoLevel1 == nil {
			t.Errorf("宠物 %d 的 also_level1 是 nil，要序列化成 []", s.PetID)
		}
	}
}

func TestPetEncounterStartRejectsUnknownPetWithoutSideEffects(t *testing.T) {
	// 目录外的模板编号要回 PET_ENCOUNTER_NOT_FOUND；这里只查目录查找，不碰角色状态。
	if _, ok := gamedata.PetEncounterSpotFor(-1, gamedata.PetEncounterSpots()[0].AreaID); ok {
		t.Error("不存在的宠物不该有可遇点")
	}
}
