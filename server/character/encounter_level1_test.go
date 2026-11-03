// STAGE-286：遇敌区域的 level1_pets 验收。数据必须与 1 级宠物目录一致，无宠物点的区域是空数组而不是 null。

package character

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

const area28Key = "100-28-11-567-155-707" // 萨伊那斯（区域 28），1 级宠物 212 及其他 3 只

// catalogLevel1Names 按目录独立算出一个区域的 1 级宠物名（宠物本身 + AlsoLevel1，去重）。
func catalogLevel1Names(areaID string) map[string]bool {
	names := map[string]bool{}
	for _, sp := range gamedata.PetEncounterSpots() {
		if sp.AreaID != areaID {
			continue
		}
		if p, ok := gamedata.PetEncounterPetByID(sp.PetID); ok {
			names[p.Name] = true
		}
		for _, n := range sp.AlsoLevel1 {
			names[n] = true
		}
	}
	return names
}

// 整张图逐区域核对：有宠物点的区域与目录一致（且不重复），没有的区域必须是 []。
func TestEncounterAreasLevel1PetsMatchCatalog(t *testing.T) {
	_, c := encounterEnv(t, time.Second, 22)
	c.warpTo(t, 5) // 萨姆吉尔近郊（地图 100），这张图有 1 级宠物点
	got := c.ok("encounter_areas", `{}`)
	list, _ := got["areas"].([]any)
	if len(list) == 0 {
		t.Fatal("地图 100 应当有遇敌区域")
	}
	withPets := 0
	for _, raw := range list {
		a, _ := raw.(map[string]any)
		id, _ := a["id"].(string)
		lp, ok := a["level1_pets"].([]any)
		if !ok {
			t.Fatalf("区域 %s 的 level1_pets 不是数组（可能是 null）：%v", id, a["level1_pets"])
		}
		want := catalogLevel1Names(id)
		if len(want) == 0 {
			if len(lp) != 0 {
				t.Errorf("区域 %s 没有 1 级宠物点，level1_pets 应为空：%v", id, lp)
			}
			continue
		}
		withPets++
		seen := map[string]bool{}
		for _, v := range lp {
			name, _ := v.(string)
			if seen[name] {
				t.Errorf("区域 %s 的 level1_pets 有重复：%s", id, name)
			}
			seen[name] = true
			if !want[name] {
				t.Errorf("区域 %s 多出目录里没有的宠物 %s", id, name)
			}
		}
		if len(seen) != len(want) {
			t.Errorf("区域 %s 的 level1_pets = %v，目录里有 %d 只", id, lp, len(want))
		}
	}
	if withPets == 0 {
		t.Fatal("地图 100 应当至少有一个带 1 级宠物的区域")
	}
}

// 多宠物区域要全部列出：地图 100 的 28 号区域除了 212 号宠物，还有 AlsoLevel1 里的其他宠物。
func TestEncounterAreasLevel1PetsListsAllInMultiPetArea(t *testing.T) {
	_, c := encounterEnv(t, time.Second, 22)
	c.warpTo(t, 5)
	got := c.ok("encounter_areas", `{}`)
	a := areaByNumber(t, got, 28)
	lp, _ := a["level1_pets"].([]any)
	want := catalogLevel1Names(area28Key)
	if len(want) < 2 {
		t.Fatalf("测试前提不成立：区域 28 应当有多只 1 级宠物，目录给出 %v", want)
	}
	if len(lp) != len(want) {
		t.Fatalf("区域 28 的 level1_pets = %v，目录里有 %d 只", lp, len(want))
	}
	for _, v := range lp {
		if !want[v.(string)] {
			t.Errorf("区域 28 多出宠物 %v", v)
		}
	}
}

// 直接在索引上核对：目录里的每个可遇点，宠物名都必须出现在它所在区域的索引里。
func TestLevel1PetIndexCoversEverySpot(t *testing.T) {
	for _, sp := range gamedata.PetEncounterSpots() {
		p, ok := gamedata.PetEncounterPetByID(sp.PetID)
		if !ok {
			t.Fatalf("可遇点的宠物 %d 不在目录里", sp.PetID)
		}
		found := false
		for _, n := range level1PetsOn(sp.Floor)[sp.AreaID] {
			if n == p.Name {
				found = true
			}
		}
		if !found {
			t.Errorf("地图 %d 区域 %s 的索引里没有宠物 %s", sp.Floor, sp.AreaID, p.Name)
		}
	}
	if level1PetsOn(-1) != nil {
		t.Error("没有宠物点的地图应当返回 nil")
	}
}
