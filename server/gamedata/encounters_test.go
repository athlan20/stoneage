package gamedata

// 遇敌数据表的解析与查找（STAGE-40）。数值对照 tools/gen-encounters.py 的输出抽查。

import "testing"

func TestEncounterAreaAtVillages(t *testing.T) {
	cases := []struct {
		name              string
		floor, x, y       int
		wantID            int
		wantMax           int
		wantMissingGroups bool // 找到的区域里有没有引用缺失敌群的槽位（生成器保留原文，运行时跳过）
	}{
		// 萨姆吉尔村门外（mapwarp：出村到 100 岛 637,491），区域 21：昆依/凯比特 1–3 级。
		{"萨姆吉尔门外", 100, 638, 491, 21, 2, true},
		// 玛丽娜丝渔村门外（100 岛 79,614），区域 28：乌力等 1–2 级。
		{"玛丽娜丝门外", 100, 79, 614, 28, 2, false},
		// 加加村门外（200 岛 588,313），区域 87：乌宝宝/威威 1–2 级。
		{"加加村门外", 200, 588, 313, 87, 2, true},
		// 卡鲁它那村门外（200 岛 301,640），区域 95：克雷尔/里斯基 1–3 级。
		{"卡鲁它那门外", 200, 301, 640, 95, 2, false},
	}
	for _, tc := range cases {
		a := EncounterAreaAt(tc.floor, tc.x, tc.y)
		if a == nil {
			t.Fatalf("%s (%d,%d,%d)：不在任何遇敌区域里", tc.name, tc.floor, tc.x, tc.y)
		}
		if a.ID != tc.wantID {
			t.Errorf("%s：命中的是区域 %d（zorder %d），期望 %d", tc.name, a.ID, a.ZOrder, tc.wantID)
		}
		if a.MaxEnemies != tc.wantMax {
			t.Errorf("%s：最多敌人数 %d，期望 %d", tc.name, a.MaxEnemies, tc.wantMax)
		}
		hasMissing := false
		for _, g := range a.Groups {
			if g.ID != -1 && g.Weight > 0 {
				if _, ok := EnemyGroupByID(g.ID); !ok {
					hasMissing = true
				}
			}
		}
		if hasMissing != tc.wantMissingGroups {
			t.Errorf("%s：缺失敌群槽位 = %v，期望 %v", tc.name, hasMissing, tc.wantMissingGroups)
		}
	}
}

func TestEncounterAreaInsideVillageFloor(t *testing.T) {
	// 村庄室内地图（含村长家）的区域都是空敌群（原版 ENEMY_getEnemy 抽不出敌群时不遇敌）：
	// encounter on 要按“没有敌人”处理。
	for _, floor := range []int{1000, 2000, 3000, 4000, 1006, 2006, 3006, 4006} {
		a := EncounterAreaAt(floor, 20, 20)
		if a == nil {
			continue // 有些格子不在任何区域里，同样没有敌人
		}
		for _, g := range a.Groups {
			if g.ID == -1 {
				continue
			}
			if _, ok := EnemyGroupByID(g.ID); ok {
				t.Errorf("地图 %d 的区域 %d 引用了能抽到的敌群 %d，村里不该有敌人", floor, a.ID, g.ID)
			}
		}
	}
}

func TestEnemyVariantFields(t *testing.T) {
	// 敌人 119（区域 21 的昆依）：模板 113、等级 1、同场上限 2。
	e, ok := EnemyByID(119)
	if !ok {
		t.Fatal("没有敌人 119")
	}
	if e.Template != 113 || e.LVMin != 1 || e.LVMax != 1 || e.CreateMax != 2 {
		t.Errorf("敌人 119 = %+v", e)
	}
	if _, ok := PetByID(e.Template); !ok {
		t.Errorf("敌人 119 的模板 113 不在宠物模板表里")
	}
	// 敌人 140（区域 28 的乌力）：模板 1、等级 1。
	e, _ = EnemyByID(140)
	if e.Template != 1 || e.LVMin != 1 || e.LVMax != 1 {
		t.Errorf("敌人 140 = %+v", e)
	}
	// 随机替换候选：945 展开成 4 个候选。
	e, _ = EnemyByID(945)
	if len(e.RandomOf) != 4 || e.RandomOf[0] != 739 {
		t.Errorf("敌人 945 的随机替换候选 = %v", e.RandomOf)
	}
}

func TestEnemyGroupRefWeights(t *testing.T) {
	// 敌群 88（区域 21）：敌人 119，权重 1。
	g, ok := EnemyGroupByID(88)
	if !ok {
		t.Fatal("没有敌群 88")
	}
	if g.Enemies[0].ID != 119 || g.Enemies[0].Weight != 1 || g.Enemies[1].ID != -1 {
		t.Errorf("敌群 88 = %+v", g)
	}
}

// STAGE-152：区域带上 CEP 上下限、原始行号与稳定 ID。
func TestEncounterAreaKeyAndCEP(t *testing.T) {
	a := EncounterAreaAt(100, 650, 500)
	if a == nil {
		t.Fatal("(100,650,500) 不在区域里")
	}
	if a.Key != "100-21-582-399-783-560" {
		t.Errorf("区域 21 的稳定 ID = %q", a.Key)
	}
	if a.CEPMin != 1 || a.CEPMax != 5 {
		t.Errorf("区域 21 的 CEP = %d→%d，期望 1→5", a.CEPMin, a.CEPMax)
	}
	if a.NeverEncounter() {
		t.Error("区域 21 应当会遇敌")
	}
	if a.SourceLine == 0 || a.SourceSeq < 0 {
		t.Errorf("区域 21 的原始定位 = 行 %d、序号 %d", a.SourceLine, a.SourceSeq)
	}
	// 兜底全图区域 #36 是 CEP 0→0 的安全区。
	safe := EncounterAreaAt(100, 1, 1)
	if safe == nil || !safe.NeverEncounter() {
		t.Errorf("(100,1,1) 应当命中 CEP 0→0 的 #36，实际 %+v", safe)
	}
}

// 区域编号在不同地图上重复（430、639），稳定 ID 必须能区分，且全局唯一。
func TestEncounterAreaKeysUnique(t *testing.T) {
	seen := map[string]bool{}
	byKey := map[string]*EncounterArea{}
	var dup430, dup639 []string
	EncounterAreasOn(100) // 先触发加载
	for _, a := range encountAreas {
		if a.Key == "" {
			t.Fatalf("区域 #%d（地图 %d）没有稳定 ID", a.ID, a.Floor)
		}
		if seen[a.Key] {
			t.Fatalf("稳定 ID 重复：%s", a.Key)
		}
		seen[a.Key] = true
		byKey[a.Key] = a
		switch a.ID {
		case 430:
			dup430 = append(dup430, a.Key)
		case 639:
			dup639 = append(dup639, a.Key)
		}
	}
	if len(dup430) != 2 || len(dup639) != 2 {
		t.Errorf("430/639 各出现两次，实际 %d/%d：%v %v", len(dup430), len(dup639), dup430, dup639)
	}
	for _, k := range append(dup430, dup639...) {
		got, ok := EncounterAreaByKey(k)
		if !ok || got.Key != k {
			t.Errorf("EncounterAreaByKey(%q) = %+v, %v", k, got, ok)
		}
	}
	if _, ok := EncounterAreaByKey("100-99999-0-0-0-0"); ok {
		t.Error("不存在的稳定 ID 不该命中")
	}
}

// 区域行按地图取，保持文件顺序（重叠平局靠前的先出现）。
func TestEncounterAreasOnFloorOrder(t *testing.T) {
	areas := EncounterAreasOn(100)
	if len(areas) != 45 {
		t.Fatalf("地图 100 的区域行 = %d，期望 45", len(areas))
	}
	for i := 1; i < len(areas); i++ {
		if areas[i-1].SourceSeq >= areas[i].SourceSeq {
			t.Fatalf("第 %d 条与第 %d 条的原始序号不递增", i-1, i)
		}
	}
	if got := EncounterAreasOn(999999); got != nil {
		t.Errorf("不存在的地图应当返回 nil，实际 %v", got)
	}
}

func TestMapSizeAndClamp(t *testing.T) {
	if w, h, ok := MapSize(100); !ok || w != 800 || h != 800 {
		t.Errorf("地图 100 的尺寸 = %d×%d, %v", w, h, ok)
	}
	if w, h, ok := MapSize(200); !ok || w != 800 || h != 1200 {
		t.Errorf("地图 200 的尺寸 = %d×%d, %v", w, h, ok)
	}
	if _, _, ok := MapSize(999999); ok {
		t.Error("不存在的地图不该有尺寸")
	}
	// 地图 500 的 (1,1)-(800,800) 要裁掉越界的最后一行一列。
	x1, y1, x2, y2, ok := ClampToMap(500, 1, 1, 800, 800)
	if !ok || x1 != 1 || y1 != 1 || x2 != 799 || y2 != 799 {
		t.Errorf("裁剪结果 = (%d,%d)-(%d,%d), %v", x1, y1, x2, y2, ok)
	}
	// 整块在地图外 → 裁完为空。
	if _, _, _, _, ok := ClampToMap(500, 900, 900, 950, 950); ok {
		t.Error("完全在地图外的矩形应当裁成空")
	}
	// 负坐标裁到 0。
	x1, y1, x2, y2, ok = ClampToMap(100, -5, -5, 10, 10)
	if !ok || x1 != 0 || y1 != 0 || x2 != 10 || y2 != 10 {
		t.Errorf("负坐标裁剪 = (%d,%d)-(%d,%d), %v", x1, y1, x2, y2, ok)
	}
}
