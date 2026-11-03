package gamedata

import (
	"reflect"
	"strings"
	"testing"
)

// 里昂蛙王（精灵王的传说·前传，30605）是第一场放开的战斗 NPC（STAGE-96）：参数要和原版
// king/event69_5.arg 一致（阵容、道具、开战与胜利的台词、传送落点）。
func TestNpcEnemyFrogKing(t *testing.T) {
	ne, ok := NpcEnemyByFile("king/event69_5")
	if !ok {
		t.Fatal("没有 king/event69_5 的战斗参数")
	}
	npc, ok := NPCByID(ne.ID)
	if !ok || npc.Floor != 30605 || npc.Type != "战斗" || npc.Name != "里昂蛙王" {
		t.Fatalf("战斗参数对应的 NPC 不对：%+v", npc)
	}
	if ne.Unsupported != "" {
		t.Fatalf("里昂蛙王应该可以开战，却标了不支持：%s", ne.Unsupported)
	}
	if want := []int{1685, 1681, 1681, 1681, 1681, 1681, 1681, 1681, 1681, 1681}; !reflect.DeepEqual(ne.Enemies, want) {
		t.Errorf("阵容 = %v，要 %v", ne.Enemies, want)
	}
	if !reflect.DeepEqual(ne.Items, []int{19622}) || ne.Steal != -1 || !ne.OneBattle {
		t.Errorf("前置道具 %v steal %d one_battle %v，要 [19622] -1 true", ne.Items, ne.Steal, ne.OneBattle)
	}
	if ne.DieAct != 1 || ne.Warp == nil || *ne.Warp != (WarpPoint{Floor: 30607, X: 7, Y: 11}) {
		t.Errorf("赢了的处理 die_act %d warp %+v，要传送到 30607 的 7,11", ne.DieAct, ne.Warp)
	}
	if len(ne.Ask) != 2 || ne.Ask[0] == "" || ne.Denied == "" || ne.Already == "" || ne.Start == "" || ne.End == "" {
		t.Errorf("台词不全：ask %q denied %q already %q start %q end %q", ne.Ask, ne.Denied, ne.Already, ne.Start, ne.End)
	}
}

// 第一期只放里昂蛙王一个（白名单）；别的战斗 NPC 即使数据齐全也不放行。
func TestNpcEnemyWhitelist(t *testing.T) {
	if !NpcEnemyEnabled("king/event69_5") {
		t.Fatal("里昂蛙王应该放开")
	}
	if len(npcEnemyEnabled) != 1 {
		t.Errorf("白名单只应有里昂蛙王，实际 %d 个", len(npcEnemyEnabled))
	}
	if NpcEnemyEnabled("king/event69_13") {
		t.Error("黑暗精灵王（分路传送）不应放开")
	}
}

// 道场弟子（1021、3021）是候选阵容，转换时标“道场（未实现）”，不能被当作普通战斗。
func TestNpcEnemyGymIsUnsupported(t *testing.T) {
	gyms := 0
	for _, floor := range []int{1021, 3021} {
		for _, npc := range NPCsByFloor(floor) {
			if npc.Type != "战斗" {
				continue
			}
			gyms++
			ne, ok := NpcEnemyOf(npc.ID)
			if !ok || !strings.Contains(ne.Unsupported, "道场") {
				t.Fatalf("道场弟子 %d 应标道场不支持：%+v", npc.ID, ne)
			}
		}
	}
	if gyms != 200 {
		t.Errorf("道场弟子 %d 个，要 200 个（1021、3021 各 100）", gyms)
	}
}

// 战斗 NPC 的占用：同一个 NPC 只能被一场战斗登记；只有登记的那场能释放。
func TestNPCBoardOccupancy(t *testing.T) {
	b := NewNPCBoard()
	if b.Busy(7) {
		t.Fatal("新的板子上没有占用")
	}
	if !b.Occupy(7, "b_1") {
		t.Fatal("第一次登记应当成功")
	}
	if b.Occupy(7, "b_2") {
		t.Fatal("已被占用时再登记应当失败")
	}
	if !b.Busy(7) {
		t.Fatal("登记后应当显示占用")
	}
	b.Release(7, "b_2") // 不是它登记的：不动
	if !b.Busy(7) {
		t.Fatal("别的战斗不能释放占用")
	}
	b.Release(7, "b_1")
	if b.Busy(7) || !b.Occupy(7, "b_3") {
		t.Fatal("释放后应当可以再登记")
	}
}
