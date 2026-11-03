package gamedata

import "testing"

// 目录的完整性与准入（STAGE-161）。这些断言就是验收标准里「环、孤儿、重复 floor、越界落点
// 都能被自动校验拦下」的运行期版本：生成脚本已经挡过一次，这里保证不手改生成文件也能发现。
func TestTravelCatalogShape(t *testing.T) {
	nodes := TravelCatalog()
	if len(nodes) == 0 {
		t.Fatal("目录为空")
	}
	if TravelCatalogVersion() == "" {
		t.Error("目录版本号为空")
	}

	byID := make(map[string]*TravelNode, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		if _, ok := byID[node.ID]; ok {
			t.Errorf("节点 id 重复：%s", node.ID)
		}
		byID[node.ID] = node
	}

	roots := 0
	for _, node := range nodes {
		if node.ParentID == nil {
			roots++
			continue
		}
		if _, ok := byID[*node.ParentID]; !ok {
			t.Errorf("节点 %s 的父节点 %s 不存在", node.ID, *node.ParentID)
		}
	}
	if roots == 0 {
		t.Error("没有根节点")
	}

	// 每个节点沿父链都要能走到根，走不到就是成环。
	for _, node := range nodes {
		seen := map[string]bool{}
		cur := node.ID
		for {
			if seen[cur] {
				t.Errorf("节点 %s 的上级链路成环", node.ID)
				break
			}
			seen[cur] = true
			up := byID[cur].ParentID
			if up == nil {
				break
			}
			cur = *up
		}
	}

	// 同级 order 必须是 0..n-1，否则客户端排序会错位。
	next := map[string]int{}
	for _, node := range nodes {
		parent := ""
		if node.ParentID != nil {
			parent = *node.ParentID
		}
		if node.Order != next[parent] {
			t.Errorf("节点 %s 在 %q 下的 order 是 %d，应为 %d", node.ID, parent, node.Order, next[parent])
		}
		next[parent] = node.Order + 1
	}
}

// 可传送的 floor 集合必须和目录里的地图节点一一对应，平面表也必须是同一批。
func TestTravelCatalogMatchesWarpable(t *testing.T) {
	formal := map[int]string{}
	for _, node := range TravelCatalog() {
		if node.Floor == nil {
			continue
		}
		if prev, ok := formal[*node.Floor]; ok {
			t.Errorf("地图 %d 在目录里出现了两次（%s / %s）", *node.Floor, prev, node.Name)
		}
		formal[*node.Floor] = node.Name
	}

	maps := WarpableMaps()
	if len(maps) != len(formal) {
		t.Errorf("平面表 %d 张，目录地图节点 %d 张", len(maps), len(formal))
	}
	last := 0
	for _, m := range maps {
		if m.Floor <= last {
			t.Errorf("平面表没有按 floor 升序：%d 在 %d 之后", m.Floor, last)
		}
		last = m.Floor
		if _, ok := formal[m.Floor]; !ok {
			t.Errorf("平面表里的 %d 不在目录里", m.Floor)
		}
		if m.Name == "" {
			t.Errorf("平面表里的 %d 没有名字", m.Floor)
		}
		if !WarpableFloor(m.Floor) {
			t.Errorf("平面表里的 %d 不能传", m.Floor)
		}
	}
}

// 每张正式地图都要有边界内的落点；落点的格子必须在地图尺寸之内。
func TestTravelCatalogLandings(t *testing.T) {
	for _, node := range TravelCatalog() {
		if node.Floor == nil {
			continue
		}
		floor := *node.Floor
		x, y, ok := LandingOf(floor)
		if !ok {
			t.Errorf("正式地图 %d（%s）没有落点", floor, node.Name)
			continue
		}
		w, h, has := MapSize(floor)
		if !has {
			t.Errorf("正式地图 %d（%s）没有尺寸数据", floor, node.Name)
			continue
		}
		if x < 0 || x >= w || y < 0 || y >= h {
			t.Errorf("地图 %d 的落点 (%d,%d) 越界，尺寸 %d×%d", floor, x, y, w, h)
		}
	}
}

// 非正式地图不进目录，即使落点表里被人为塞了一条也不能传（STAGE-161 验收第 5 条）。
func TestTravelCatalogRejectsExcluded(t *testing.T) {
	excluded := []int{777, 501, 516, 517, 518, 519, 520, 521, 522, 999, 5561, 5581}
	for _, floor := range excluded {
		if WarpableFloor(floor) {
			t.Errorf("测试图/占位图 %d 仍可传送", floor)
		}
		if _, _, ok := LandingOf(floor); ok {
			t.Errorf("测试图/占位图 %d 仍有落点", floor)
		}
		for _, m := range WarpableMaps() {
			if m.Floor == floor {
				t.Errorf("平面表里出现了 %d", floor)
			}
		}
	}

	// 人为给一张排除地图塞落点：准入看目录，不看落点表，所以它依然传不了。
	const wan = 777
	travelCatalogLandings[wan] = [2]int{25, 25}
	defer delete(travelCatalogLandings, wan)
	if WarpableFloor(wan) {
		t.Error("塞了落点之后测试图变得可传了")
	}
	if _, _, ok := LandingOf(wan); ok {
		t.Error("塞了落点之后测试图能拿到落点")
	}
}

// 抽查几张关键地图：主区域、四个出生村、村长家、洞窟多层、同名地图都必须在目录里。
func TestTravelCatalogSpotChecks(t *testing.T) {
	want := map[int]string{
		100:   "萨伊那斯",
		200:   "加鲁卡",
		300:   "吉鲁岛",
		400:   "沙姆岛",
		5540:  "波拉岛",
		500:   "五十年前波拉岛",
		1000:  "萨姆吉尔村",
		2000:  "玛丽娜丝渔村",
		3000:  "加加村",
		4000:  "卡鲁它那村",
		1006:  "萨姆吉尔的村长家",
		10001: "阿布的洞窟地下１楼",
		10007: "阿布的洞窟地下７楼",
		117:   "迷宫 117",
		122:   "迷宫 122",
	}
	for floor, name := range want {
		if !WarpableFloor(floor) {
			t.Errorf("地图 %d（%s）不在目录里", floor, name)
			continue
		}
		if got := TravelNodeName(floor); got != name {
			t.Errorf("地图 %d 的目录名是 %q，期望 %q", floor, got, name)
		}
	}

	// 同名地图必须靠 floor 区分：4 张「迷宫」的目录名互不相同。
	seen := map[string]int{}
	for _, floor := range []int{117, 118, 119, 122} {
		name := TravelNodeName(floor)
		if prev, ok := seen[name]; ok {
			t.Errorf("地图 %d 与 %d 的目录名都是 %q", prev, floor, name)
		}
		seen[name] = floor
	}
}

// 层级深度要真的分层：萨伊那斯 → 萨姆吉尔村 → 村长家。
func TestTravelCatalogHierarchy(t *testing.T) {
	byID := map[string]TravelNode{}
	for _, node := range TravelCatalog() {
		byID[node.ID] = node
	}
	child := byID["map:1006"]
	if child.Floor == nil || *child.Floor != 1006 {
		t.Fatal("找不到村长家的节点")
	}
	if child.ParentID == nil || *child.ParentID != "map:1000" {
		t.Fatalf("村长家的父节点是 %v，期望 map:1000", child.ParentID)
	}
	village := byID[*child.ParentID]
	if village.ParentID == nil || *village.ParentID != "group:sainasu" {
		t.Fatalf("萨姆吉尔村的父节点是 %v，期望 group:sainasu", village.ParentID)
	}
	if root := byID[*village.ParentID]; root.Floor == nil || *root.Floor != 100 {
		t.Fatalf("萨伊那斯根节点的 floor 不对：%v", root.Floor)
	}
}
