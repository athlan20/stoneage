package character

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// 超级饲育员（1763，1003 地图的 (18,13)）是宠物技能店，对话距离 2；缺陷 STAGE-263 的复现点是落点 (16,21)。
func approachFixture(t *testing.T, x, y int) (*Service, *Presence, gamedata.NPC) {
	t.Helper()
	npc, ok := gamedata.NPCByID(1763)
	if !ok || npc.Floor != 1003 {
		t.Fatalf("没有 1763 号 NPC 或不在 1003 图：%+v", npc)
	}
	s := &Service{npcs: gamedata.NewNPCBoard(), presences: map[int64]*Presence{}}
	p := &Presence{rec: record{Floor: 1003, X: x, Y: y}}
	return s, p, npc
}

// 另一个角色放在 (x,y) 上（floor 图）。
func placeOther(s *Service, id int64, floor, x, y int) {
	s.presences[id] = &Presence{rec: record{ID: id, Floor: floor, X: x, Y: y}}
}

func TestApproachNPCWalksNextToNPC(t *testing.T) {
	s, p, npc := approachFixture(t, 16, 21)
	if talkReach(npc) != petskillShopReach {
		t.Fatalf("宠物技能店应登记对话距离 %d，得到 %d", petskillShopReach, talkReach(npc))
	}
	if !s.approachNPC(p, &p.rec, npc, talkReach(npc)) {
		t.Fatal("离 NPC 8 格，应该走过去")
	}
	// 离 (16,21) 最近的相邻格是 (17,14)：切比雪夫距离 7，其余相邻格更远或同距但在它之后。
	if p.rec.X != 17 || p.rec.Y != 14 {
		t.Fatalf("应走到 (17,14)，实际 (%d,%d)", p.rec.X, p.rec.Y)
	}
}

func TestApproachNPCSkipsOccupiedCell(t *testing.T) {
	s, p, npc := approachFixture(t, 16, 21)
	placeOther(s, 2, 1003, 17, 14) // 同图别的角色占着最近的格
	if !s.approachNPC(p, &p.rec, npc, talkReach(npc)) {
		t.Fatal("还有空格，应该走过去")
	}
	if p.rec.X != 18 || p.rec.Y != 14 {
		t.Fatalf("(17,14) 被占，应走到 (18,14)，实际 (%d,%d)", p.rec.X, p.rec.Y)
	}
}

func TestApproachNPCIgnoresOtherFloor(t *testing.T) {
	s, p, npc := approachFixture(t, 16, 21)
	placeOther(s, 2, 1006, 17, 14) // 别的图上的角色不占这里的格
	s.approachNPC(p, &p.rec, npc, talkReach(npc))
	if p.rec.X != 17 || p.rec.Y != 14 {
		t.Fatalf("别的图的角色不该挡路，应走到 (17,14)，实际 (%d,%d)", p.rec.X, p.rec.Y)
	}
}

func TestApproachNPCNoSpaceKeepsPosition(t *testing.T) {
	s, p, npc := approachFixture(t, 16, 21)
	id := int64(10)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx != 0 || dy != 0 {
				id++
				placeOther(s, id, 1003, npc.X+dx, npc.Y+dy)
			}
		}
	}
	if s.approachNPC(p, &p.rec, npc, talkReach(npc)) {
		t.Fatal("四周全被占，不该移动")
	}
	if p.rec.X != 16 || p.rec.Y != 21 {
		t.Fatalf("没有空格时位置不能变，实际 (%d,%d)", p.rec.X, p.rec.Y)
	}
	// 回退：NPC 自己的距离检查照常回复“太远了”（talk 和 npc_choose 都走这一条）。
	perr := checkPetskillShopReach(&p.rec, npc)
	if perr == nil || perr.Code != protocol.CodeActionNotAllowed || perr.Message != "离超级饲育员太远了，走近一点再说" {
		t.Fatalf("没有空格时应回复太远了，得到 %+v", perr)
	}
}

func TestApproachNPCWithinReachOrNoReachDoesNothing(t *testing.T) {
	s, p, npc := approachFixture(t, 17, 15) // 切比雪夫距离 2：已经够得着
	if s.approachNPC(p, &p.rec, npc, talkReach(npc)) || p.rec.X != 17 || p.rec.Y != 15 {
		t.Fatalf("已在对话距离内，不该移动：(%d,%d)", p.rec.X, p.rec.Y)
	}
	far, _ := gamedata.NPCByID(221) // 店长：没有距离检查，talkReach 为 0
	if talkReach(far) != 0 {
		t.Fatalf("店长不该登记对话距离")
	}
	p.rec.X, p.rec.Y = 16, 21
	if s.approachNPC(p, &p.rec, far, talkReach(far)) || p.rec.X != 16 {
		t.Fatal("没有对话距离的 NPC 不该移动角色")
	}
}
