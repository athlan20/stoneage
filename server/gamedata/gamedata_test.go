package gamedata

import "testing"

func TestHometownMapNames(t *testing.T) {
	for i, h := range Hometowns {
		name, ok := MapName(h.Floor)
		if !ok || name == "" {
			t.Errorf("出生地 %d（%s）的 floor %d 没有地图名", i, h.Name, h.Floor)
		}
	}
	if n, _ := MapName(1006); n != "萨姆吉尔的村长家" {
		t.Errorf("1006 的地图名 %q", n)
	}
}

func TestFormulas(t *testing.T) {
	if got := MaxHP(8, 4, 4, 4); got != 44 {
		t.Errorf("MaxHP = %d", got)
	}
	if NextExp(1) != 2 || NextExp(2) != 6 || NextExp(3) != 18 {
		t.Errorf("经验表不对：%d %d %d", NextExp(1), NextExp(2), NextExp(3))
	}
	if NextExp(0) != -1 || NextExp(1000) != -1 {
		t.Error("越界应返回 -1")
	}
	if BodyImage(3, 2) != 100070 || FaceImage(3, 2, 1, 4) != 30359 {
		t.Errorf("图像编号 %d %d", BodyImage(3, 2), FaceImage(3, 2, 1, 4))
	}
}

// 按级累加的总经验要等于原版的节奏（STAGE-57 的对照数字）：升到 10 级 1,000、
// 30 级 81,000、60 级 1,296,000、140 级 1,224,160,000。此前把经验当累计值、
// 和每级需求直接比较，练到 30 级只要 10,272，比原版快约 8 倍。
func TestCumulativeExpTotals(t *testing.T) {
	want := map[int]int64{10: 1000, 30: 81000, 60: 1296000, 140: 1224160000}
	for level, total := range want {
		var sum int64
		for l := 1; l < level; l++ {
			sum += NextExp(l)
		}
		if sum != total {
			t.Errorf("升到 %d 级的按级总经验 = %d，应 %d", level, sum, total)
		}
	}
}

// 地点表（STAGE-40 起分两组）：编号从 1 连续，名字不重复；村里（1–4）在村内地图上、
// 那里不能有能抽到敌人的遇敌区域；野外练级点（5–8）在两座大岛上、要落在能抽到敌人的遇敌区域里。
func TestPlaces(t *testing.T) {
	names := map[string]bool{}
	for i, p := range Places {
		if p.ID != i+1 {
			t.Errorf("第 %d 个地点的编号是 %d，应连续从 1 开始", i+1, p.ID)
		}
		if p.Name == "" || names[p.Name] {
			t.Errorf("地点名为空或重复：%q", p.Name)
		}
		names[p.Name] = true
		if _, ok := MapName(p.Floor); !ok {
			t.Errorf("%s 的 floor %d 没有地图名", p.Name, p.Floor)
		}
		if got, ok := PlaceByID(p.ID); !ok || got != p {
			t.Errorf("PlaceByID(%d) = %v, %v", p.ID, got, ok)
		}
		area := EncounterAreaAt(p.Floor, p.X, p.Y)
		hasEnemy := false
		if area != nil {
			for _, g := range area.Groups {
				if g.ID == -1 {
					continue
				}
				if grp, ok := EnemyGroupByID(g.ID); ok && grp.AppearItem == -1 {
					hasEnemy = true // 简化判断：不需要道具就能出现的敌群算有敌人
				}
			}
		}
		if i < 4 { // 村里：站的位置不能有敌人
			if hasEnemy {
				t.Errorf("%s（%d,%d,%d）在遇敌区域里，村里不该有敌人", p.Name, p.Floor, p.X, p.Y)
			}
		} else { // 野外练级点：必须落在能抽到敌人的遇敌区域里，且在两座大岛上（世界图画得出来）
			if p.Floor != 100 && p.Floor != 200 {
				t.Errorf("%s 在 floor %d，野外地点要落在 100 或 200 两座岛上", p.Name, p.Floor)
			}
			if !hasEnemy {
				t.Errorf("%s（%d,%d,%d）不在任何能抽到敌人的遇敌区域里", p.Name, p.Floor, p.X, p.Y)
			}
		}
	}
	if _, ok := PlaceByID(0); ok {
		t.Error("编号 0 不应存在")
	}
	if _, ok := PlaceByID(len(Places) + 1); ok {
		t.Error("超出范围的编号不应存在")
	}
}
