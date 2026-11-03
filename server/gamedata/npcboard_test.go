package gamedata

import "testing"

// boardNPC 是测试用的 NPC：编号 225，村长家的厨师，初始在地图 1006（docs/protocol.md 5.32 的例子）。
const boardNPC = 225

// 三个位置，轮流用：地图 2000、3000、4000。
var boardPoints = []WarpPoint{{Floor: 2000, X: 1, Y: 2}, {Floor: 3000, X: 3, Y: 4}, {Floor: 4000, X: 5, Y: 6}}

func TestNPCBoardWarpRotates(t *testing.T) {
	b := NewNPCBoard()
	want := []WarpPoint{boardPoints[0], boardPoints[1], boardPoints[2], boardPoints[0], boardPoints[1]}
	for i, w := range want {
		from, to, ok := b.Warp(boardNPC, boardPoints)
		if !ok {
			t.Fatalf("第 %d 次移动失败", i+1)
		}
		if to.Floor != w.Floor || to.X != w.X || to.Y != w.Y {
			t.Fatalf("第 %d 次移到 %d.%d.%d，want %d.%d.%d", i+1, to.Floor, to.X, to.Y, w.Floor, w.X, w.Y)
		}
		if from.ID != boardNPC || from.Name != to.Name {
			t.Fatalf("第 %d 次移动前后的 NPC 不对：%+v -> %+v", i+1, from, to)
		}
	}
}

func TestNPCBoardMoveVisibleOnBothFloors(t *testing.T) {
	b := NewNPCBoard()
	orig, _ := NPCByID(boardNPC)
	if _, _, ok := b.Warp(boardNPC, boardPoints[:1]); !ok {
		t.Fatal("移动失败")
	}
	if got := ids(b.OnFloor(orig.Floor)); contains(got, boardNPC) {
		t.Fatalf("原地图 %d 还列着它：%v", orig.Floor, got)
	}
	list := b.OnFloor(2000)
	if !contains(ids(list), boardNPC) {
		t.Fatalf("新地图 2000 没有列出它：%v", ids(list))
	}
	for _, n := range list {
		if n.ID == boardNPC && (n.X != 1 || n.Y != 2) {
			t.Fatalf("新地图上的坐标是 %d,%d，want 1,2", n.X, n.Y)
		}
	}
	if n, ok := b.ByID(boardNPC); !ok || n.Floor != 2000 || n.X != 1 || n.Y != 2 {
		t.Fatalf("ByID 没有读到移动后的位置：%+v %v", n, ok)
	}
	if got := b.ByName(orig.Name); len(got) == 0 || got[0].Floor != 2000 {
		t.Fatalf("ByName 没有读到移动后的位置：%+v", got)
	}
}

func TestNPCBoardFreshBoardHasStaticPositions(t *testing.T) {
	orig, _ := NPCByID(boardNPC)
	b := NewNPCBoard()
	if _, _, ok := b.Warp(boardNPC, boardPoints); !ok {
		t.Fatal("移动失败")
	}
	// 另一份新的位置表不受影响（服务重启后回到静态表）。
	fresh, ok := NewNPCBoard().ByID(boardNPC)
	if !ok || fresh != orig {
		t.Fatalf("新位置表不是静态位置：%+v，want %+v", fresh, orig)
	}
}

func TestNPCBoardWarpRejectsEmptyAndUnknown(t *testing.T) {
	b := NewNPCBoard()
	if _, _, ok := b.Warp(boardNPC, nil); ok {
		t.Fatal("空的位置列表不应该移动")
	}
	if _, _, ok := b.Warp(-1, boardPoints); ok {
		t.Fatal("不存在的 NPC 不应该移动")
	}
}

func ids(list []NPC) []int {
	out := make([]int, 0, len(list))
	for _, n := range list {
		out = append(out, n.ID)
	}
	return out
}

func contains(list []int, id int) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}
