package character

// NPC 移位（STAGE-93，docs/quest.md）：giiru 的沉船脚本（sink055）里，NPC「多洛加」（1415，地图 30012）交付 2709 之后
// 移到 NpcWarp 的第一个位置 30001.10.6。验收：原地图的连接收到新的 NPC 列表、原地图上打开着它的窗口被关掉；
// 新地图上的连接收到它；原地图再找它回 NPC_NOT_HERE；选否不移动。编号、地图、坐标都从数据里取，不写死在别处。

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	sinkNPC       = 1415  // 多洛加：giiru 沉船脚本（sink055）的 NPC，初始在 30012 的 (7,4)
	sinkFloor     = 30012 // NPC 的初始地图
	sinkWarpFloor = 30001 // NpcWarp 的第一个位置 30001.10.6
)

// placeChar 把角色放到地图上（只写数据库，要在 enter_world 之前）。
func placeChar(t *testing.T, e *env, name string, floor, x, y int) {
	t.Helper()
	if err := MoveCharacter(context.Background(), e.chars.db, name, floor, x, y); err != nil {
		t.Fatal(err)
	}
}

// npcsCall 在世界中取 npcs，返回 编号 → 坐标，以及这次请求前收到的推送。
func npcsCall(t *testing.T, c *client) (map[int][2]int, []map[string]any) {
	t.Helper()
	m, pushes := c.callp("npcs", "{}")
	if m["ok"] != true {
		t.Fatalf("npcs 应成功: %v", m)
	}
	out := map[int][2]int{}
	for _, v := range m["data"].(map[string]any)["npcs"].([]any) {
		n := v.(map[string]any)
		out[int(n["id"].(float64))] = [2]int{int(n["x"].(float64)), int(n["y"].(float64))}
	}
	return out, pushes
}

// pushData 返回第一条该类型推送的 data；没有时返回 nil。
func pushData(pushes []map[string]any, typ string) map[string]any {
	for _, p := range pushes {
		if p["type"] == typ {
			d, _ := p["data"].(map[string]any)
			return d
		}
	}
	return nil
}

// TestNpcWarpMovesNpcForEveryone：交付后 NPC 对全服生效。a 交付；b 原地图上开着窗口；c 在新地图上。
func TestNpcWarpMovesNpcForEveryone(t *testing.T) {
	e := newPointsEnv(t)
	a, b, c := e.loggedIn("alice"), e.loggedIn("bob"), e.loggedIn("carol")
	a.ok("create_character", createJSON(0, "勇者小明", 0))
	b.ok("create_character", createJSON(0, "路人乙", 0))
	c.ok("create_character", createJSON(0, "路人丙", 0))
	placeChar(t, e, "勇者小明", sinkFloor, 7, 5)
	placeChar(t, e, "路人乙", sinkFloor, 8, 5)
	placeChar(t, e, "路人丙", sinkWarpFloor, 9, 5)
	putItemSetup(t, firstBagSlot, 2709, 1)(e) // 交付用的道具（sink055 的第一个块）
	a.ok("enter_world", `{"slot":0}`)
	b.ok("enter_world", `{"slot":0}`)
	c.ok("enter_world", `{"slot":0}`)

	if pos, _ := npcsCall(t, b); pos[sinkNPC] != [2]int{7, 4} {
		t.Fatalf("移位前 %d 应在 %d 的 (7,4)，实际 %v", sinkNPC, sinkFloor, pos[sinkNPC])
	}
	if !hasPush(talkTo(b, sinkNPC), "npc_window") { // b 先开着它的窗口
		t.Fatal("b 和 NPC 说话应开窗口")
	}

	// a 交付：是 → 收走 2709、给 2712，NPC 移到 30001.10.6。
	expectWindow(t, talkTo(a, sinkNPC), "\n\n好像喉咙干得动不了的样子", 2)
	_, pushes := a.callp("npc_choose", `{"no":1}`)
	if !hasPush(pushes, "inventory_changed") {
		t.Fatalf("交付后应推 inventory_changed: %v", pushes)
	}
	if d := pushData(pushes, "npcs_changed"); d == nil || d["floor"] != float64(sinkFloor) {
		t.Fatalf("触发的玩家在原地图，应收到原地图的 npcs_changed: %v", pushes)
	}

	// b：原地图的窗口被关掉（MOVED），原地图的列表不再有它。
	pos, pushes := npcsCall(t, b)
	if d := pushData(pushes, "npc_window_closed"); d == nil || d["reason"] != WindowReasonMoved {
		t.Fatalf("原地图上打开着它的窗口应以 MOVED 关闭，实际推送 %v", pushes)
	}
	if d := pushData(pushes, "npcs_changed"); d == nil || d["floor"] != float64(sinkFloor) {
		t.Fatalf("b 应收到原地图的 npcs_changed: %v", pushes)
	}
	if _, ok := pos[sinkNPC]; ok {
		t.Fatalf("原地图 %d 还列着它: %v", sinkFloor, pos)
	}

	// c：新地图上收到它，坐标是脚本里的第一个位置。
	pos, pushes = npcsCall(t, c)
	if d := pushData(pushes, "npcs_changed"); d == nil || d["floor"] != float64(sinkWarpFloor) {
		t.Fatalf("c 应收到新地图的 npcs_changed: %v", pushes)
	}
	if got := pos[sinkNPC]; got != [2]int{10, 6} {
		t.Fatalf("新地图 %d 上它应在 (10,6)，实际 %v", sinkWarpFloor, got)
	}

	// a 已经不在它身边：原地图上再找它回 NPC_NOT_HERE。
	if m, _ := a.callp("talk", `{"npc":`+strconv.Itoa(sinkNPC)+`}`); errCode(m) != "NPC_NOT_HERE" {
		t.Fatalf("原地图上对话应回 NPC_NOT_HERE，实际 %v", m)
	}
}

// TestNpcWarpNotOnDecline：选否不交付，NPC 不动，任何人都不收到 npcs_changed。
func TestNpcWarpNotOnDecline(t *testing.T) {
	e := newPointsEnv(t)
	a := e.loggedIn("alice")
	a.ok("create_character", createJSON(0, "勇者小明", 0))
	placeChar(t, e, "勇者小明", sinkFloor, 7, 5)
	putItemSetup(t, firstBagSlot, 2709, 1)(e)
	a.ok("enter_world", `{"slot":0}`)

	expectWindow(t, talkTo(a, sinkNPC), "\n\n好像喉咙干得动不了的样子", 2)
	_, pushes := a.callp("npc_choose", `{"no":2}`)
	if hasPush(pushes, "npcs_changed") {
		t.Fatalf("选否不应移动 NPC: %v", pushes)
	}
	if pos, _ := npcsCall(t, a); pos[sinkNPC] != [2]int{7, 4} {
		t.Fatalf("选否后它应还在 (7,4)，实际 %v", pos[sinkNPC])
	}
}

// TestKeywordReplySettlesNPCWarp：暗号回复结束时也结算登记过的 NPC 移位（handleNpcReply 和 talk / choose 一样 defer settleNPCWarps）。
// 数据里还没有带 NpcWarp 的暗号块，这里在说暗号之前直接给连接登记一次移位：说对了暗号，NPC 应当当场移走，不等下一次对话。
func TestKeywordReplySettlesNPCWarp(t *testing.T) {
	npc := oevOneNPC(t)
	e, c := keywordEnv(t, npc, map[int]int{2552: 1})
	if !hasPush(talkTo(c, npc.ID), "npc_window") {
		t.Fatal("和 NPC 说话应开窗口")
	}
	if _, pushes := c.callp("npc_choose", `{"no":1}`); !hasPush(pushes, "npc_window") {
		t.Fatalf("选“说暗号”应开文字输入窗口: %v", pushes)
	}

	l := e.chars.LiveOf(e.session("alice"))
	if l == nil {
		t.Fatal("alice 应在世界中")
	}
	if err := l.Operate(func(*record) {
		l.warps = append(l.warps, npcWarp{npc: npc.ID, points: []gamedata.WarpPoint{{Floor: sinkWarpFloor, X: 10, Y: 6}}})
	}); err != nil {
		t.Fatal(err)
	}

	pushes := sayKeyword(c, "近藏")
	if d := pushData(pushes, "npcs_changed"); d == nil || d["floor"] != float64(npc.Floor) {
		t.Fatalf("说对暗号的回复结束时应结算移位，原地图应收到 npcs_changed: %v", pushes)
	}
	pos, _ := npcsCall(t, c)
	if _, ok := pos[npc.ID]; ok {
		t.Fatalf("原地图 %d 还列着它: %v", npc.Floor, pos)
	}
}
