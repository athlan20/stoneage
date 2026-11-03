package character

// 存档点（STAGE-72，docs/protocol.md 5.40）：talk 储存点弹记录窗口，交一件指定物品
// 记录成功（收走物品、savepoint 写库），战败送回存档点落点；没有合格物品不记录不消耗；
// 重新登录后存档点还在。

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// 萨姆吉尔村的储存点是 205 号（地图 1000），记录要交凯比特的肉（候选 1930–1935 之一）。
const saveNPC = 205

func TestSavepointRecordsAndTakesItem(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`) // 萨姆吉尔村

	sp := gamedata.NpcSavepointOf(saveNPC)
	if sp == nil {
		t.Fatal("205 号 NPC 应是存档点（萨姆吉尔的储存点）")
	}
	// 给一件候选物品（1930）。
	withRec(t, e, "alice", func(r *record) {
		r.Items = append(r.Items, itemRec{Slot: 5, ItemID: sp.Groups[0][0].Item, Count: 1})
	})
	if m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, saveNPC)); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 存档点应成功: %v %v", m, pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "凯比特的肉") {
		t.Errorf("记录窗口应是 Request 文案（要凯比特的肉）: %v", pushes[0])
	}

	// 选记录：收走物品、存档点写库、窗口变成功话。
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("记录应成功: %v", m)
	} else if len(pushes) != 2 || pushes[0]["type"] != "inventory_changed" || pushes[1]["type"] != "npc_window" {
		t.Fatalf("记录成功应推 inventory_changed 和 OkMsg 窗口: %v", pushes)
	} else if text := pushes[1]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "自由进出") {
		t.Errorf("成功窗口应是 OkMsg 原话: %v", pushes[1])
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Savepoint != saveNPC {
			t.Errorf("存档点应记在角色上，实际 %d", r.Savepoint)
		}
		if len(r.Items) != 0 {
			t.Errorf("记录要收走物品，实际 %v", r.Items)
		}
	})
	// 重新登录（leave + enter）后存档点还在。
	c.ok("leave_world", `{}`)
	c.ok("characters", `{}`)
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		if r.Savepoint != saveNPC {
			t.Errorf("重新进入后存档点应还在（数据库列），实际 %d", r.Savepoint)
		}
	})
}

func TestSavepointRejectsWithoutItem(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	sp := gamedata.NpcSavepointOf(saveNPC)
	// 给一件不在候选里的物品。
	withRec(t, e, "alice", func(r *record) {
		r.Items = append(r.Items, itemRec{Slot: 5, ItemID: sp.Groups[0][0].Item + 9999, Count: 1})
	})
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, saveNPC))
	if m := c.call("npc_choose", `{"no":1}`); m["ok"] != false {
		t.Fatalf("没有合格物品应失败: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "ACTION_NOT_ALLOWED" {
		t.Errorf("应回 ACTION_NOT_ALLOWED: %v", m)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Savepoint != 0 || len(r.Items) != 1 {
			t.Errorf("被拒的记录不该动数据: savepoint=%d items=%v", r.Savepoint, r.Items)
		}
	})
}

// TestRespawnAtSavepointAfterDefeat 是验收 1/2：记录后战败送回存档点，没记录的照旧回出生地。
func TestRespawnAtSavepointAfterDefeat(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 被打飞的战败回城走 applyResult 的 lose 分支（STAGE-194：只有被打飞才送回）。
	respawnAfterLose(t, e, "勇者小明", true)
	if f, _, _, _ := e.queryPos("勇者小明"); f != 1006 {
		t.Fatalf("没记录时战败应回出生村长家（1006），实际 %d", f)
	}

	// 记录加加村的储存点（419 号，地图 3000），战败后送回那里。
	const jiajiaSave = 419
	sp := gamedata.NpcSavepointOf(jiajiaSave)
	if sp == nil {
		t.Fatal("419 号应是加加村的储存点")
	}
	withRec(t, e, "alice", func(r *record) { r.Savepoint = jiajiaSave })
	respawnAfterLose(t, e, "勇者小明", true)
	if f, _, _, _ := e.queryPos("勇者小明"); f != sp.Born[0] {
		t.Fatalf("记录后战败应回存档点（地图 %d），实际 %d", sp.Born[0], f)
	}
	if _, _, _, hp := e.queryPos("勇者小明"); hp != 1 {
		t.Errorf("战败血量应剩 1，实际 %d", hp)
	}
}

// respawnAfterLose 直接跑结算的战败分支（applyResult），然后按生产路径写库（save）。
// flown 为真时人物是被打飞的（只有这样才送回记录点，STAGE-194）。
func respawnAfterLose(t *testing.T, e *env, name string, flown bool) {
	t.Helper()
	withRec(t, e, "alice", func(r *record) {
		e.chars.applyResult(r, loseBattle(t, flown), &battle.EndView{Result: "lose", Penalty: &battle.Penalty{}})
		if err := e.chars.save(context.Background(), r, time.Now()); err != nil {
			t.Fatal(err)
		}
	})
	// 数据库里读回来的位置也应一致（applyResult 改的是内存，这里验证 save 写对列）。
	if _, err := e.chars.findBySlot(context.Background(), e.session("alice").AccountID(), 0); err != nil {
		t.Fatal(err)
	}
	_ = name
}

// TestSavepointGroupNeedsAllItems 是 GetItem 的 AND 组：候选之间任一组齐全即可，组内每件都要有，
// 同一组的几件不能占同一格（原版 NPC_UsedCheck / NPC_AndReduce）。
func TestSavepointGroupNeedsAllItems(t *testing.T) {
	sp := &gamedata.NpcSavepoint{Groups: [][]gamedata.SavepointItem{
		{{Item: 1803, Count: 1}, {Item: 1851, Count: 1}}, // 第一候选：两件都要
		{{Item: 1930, Count: 1}},                         // 第二候选：一件
	}}
	rec := func(items ...itemRec) *record { return &record{Items: items} }

	if _, _, ok := pickSavepointGroup(rec(itemRec{Slot: 5, ItemID: 1803, Count: 1}), sp); ok {
		t.Fatal("只有组里的一件时不该通过")
	}
	g, slots, ok := pickSavepointGroup(rec(
		itemRec{Slot: 5, ItemID: 1803, Count: 1}, itemRec{Slot: 6, ItemID: 1851, Count: 1}), sp)
	if !ok || len(g) != 2 || len(slots) != 2 || slots[0] == slots[1] {
		t.Fatalf("两件齐全应选第一候选，各占一格: ok=%v group=%v slots=%v", ok, g, slots)
	}
	if g, slots, ok := pickSavepointGroup(rec(itemRec{Slot: 7, ItemID: 1930, Count: 1}), sp); !ok || g[0].Item != 1930 || slots[0] != 7 {
		t.Fatalf("只有第二候选的物品应选第二候选: ok=%v group=%v slots=%v", ok, g, slots)
	}
	// 同一件物品不能被同一格重复计两次：一格 1990 不够两件。
	dup := &gamedata.NpcSavepoint{Groups: [][]gamedata.SavepointItem{{{Item: 1990, Count: 1}, {Item: 1990, Count: 1}}}}
	if _, _, ok := pickSavepointGroup(rec(itemRec{Slot: 5, ItemID: 1990, Count: 1}), dup); ok {
		t.Error("一格不能当两件用")
	}
	// 没有 GetItem 的存档点不要东西，直接记录。
	if g, slots, ok := pickSavepointGroup(rec(), &gamedata.NpcSavepoint{}); !ok || g != nil || slots != nil {
		t.Errorf("没有 GetItem 应直接通过: ok=%v", ok)
	}
}

// TestSavepointRecordedAgainSaysNormalAndKeepsItems 是已记录过的存档点：talk 说 NomalMsg，
// 选记录也只说这句，不收物品（原版 npc_savepoint.c 的「已经记录了」分支）。
func TestSavepointRecordedAgainSaysNormalAndKeepsItems(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	sp := gamedata.NpcSavepointOf(saveNPC)
	withRec(t, e, "alice", func(r *record) {
		r.Savepoint = saveNPC
		r.Items = append(r.Items, itemRec{Slot: 5, ItemID: sp.Groups[0][0].Item, Count: 1})
	})
	if _, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, saveNPC)); len(pushes) != 1 {
		t.Fatalf("talk 应推一个窗口: %v", pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); !strings.Contains(text, "已经记录了") {
		t.Errorf("已记录过应说 NomalMsg: %v", pushes[0])
	}
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("已记录过再选记录应成功（只说话）: %v", m)
	} else if len(pushes) != 1 || pushes[0]["type"] != "npc_window" {
		t.Errorf("已记录过不该推 inventory_changed: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if len(r.Items) != 1 {
			t.Errorf("已记录过不该收物品，实际 %v", r.Items)
		}
	})
}
