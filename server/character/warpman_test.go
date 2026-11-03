package character

// 传送员（STAGE-74，docs/protocol.md 5.42）：满足条件免费确认后传送（位置推两边、
// 遇敌关）、持票传送收走门票、等级/物品不满足走付费或拒绝、钱不够不传、
// unsupported（任务条件）的传送员跳过；战斗中与观察方被拒。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 萨姆吉尔村的「斗技场向导」是 195 号（地图 1000）：FREE 要持 ITEM=2597 门票，
// 传送去地图 130 的斗技场，传送时收走门票（DelItem）。
const warpNPC = 195

func warpData(t *testing.T) *gamedata.NpcWarpman {
	t.Helper()
	w := gamedata.NpcWarpmanOf(warpNPC)
	if w == nil {
		t.Fatal("195 号 NPC 应是传送员（斗技场向导）")
	}
	return w
}

func TestWarpmanWithTicket(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`) // 萨姆吉尔村

	warpData(t)
	// 给参赛证 2597 和一张别村的票 2521（DelItem 收走后者，2597 是参赛证不消耗）。
	withRec(t, e, "alice", func(r *record) {
		r.Items = append(r.Items, itemRec{Slot: 5, ItemID: 2597, Count: 1}, itemRec{Slot: 6, ItemID: 2521, Count: 1})
	})
	if m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, warpNPC)); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 应成功并弹确认窗: %v %v", m, pushes)
	} else if opts := pushes[0]["data"].(map[string]any)["options"].([]any); len(opts) != 1 {
		t.Fatalf("持票应只有一个传送选项: %v", pushes[0])
	}
	// 选传送：位置到 130、门票被收走、窗口随 MOVED 关。
	found2597 := false
	if m, pushes := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("传送应成功: %v", m)
	} else if len(pushes) < 2 {
		t.Fatalf("传送应推 inventory_changed 与 position_changed: %v", pushes)
	}
	withRec(t, e, "alice", func(r *record) {
		if r.Floor != 130 {
			t.Errorf("应传到斗技场（130），实际 %d", r.Floor)
		}
		for _, it := range r.Items {
			if it.ItemID == 2521 {
				t.Errorf("别村的票应被收走（DelItem），实际 %v", r.Items)
			}
			if it.ItemID == 2597 {
				found2597 = true
			}
		}
		if !found2597 {
			t.Errorf("参赛证 2597 不该被收走: %v", r.Items)
		}
		if r.Gold != 1_000_000 {
			t.Errorf("持票免费，金币不该动: %d", r.Gold)
		}
	})
}

func TestWarpmanWithoutTicketAndUnsupported(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	// 没票：talk 回 NPC_NOT_TALKABLE 且不弹窗（免费条件不满足且不能付费），choose 报 WRONG_STATE。
	if m, pushes := c.callp("talk", fmt.Sprintf(`{"npc":%d}`, warpNPC)); m["ok"] != false {
		t.Fatalf("没票 talk 应被拒: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "NPC_NOT_TALKABLE" {
		t.Fatalf("应回 NPC_NOT_TALKABLE: %v", m)
	} else if len(pushes) != 0 {
		t.Fatalf("没票不应弹窗: %v", pushes)
	}
	c.wantErr(t, "npc_choose", `{"no":1}`, "WRONG_STATE")

	// 依赖庄园条件的传送员跳过（MANOR 本期不支持）：242 号，站在它的地图上也不放行。
	manor, ok := gamedata.NPCByID(242)
	if !ok {
		t.Fatal("找不到 242 号 NPC")
	}
	if w := gamedata.NpcWarpmanOf(242); w == nil || !w.Unsupported {
		t.Fatalf("242 号传送员应是 unsupported（依赖庄园条件），实际 %+v", w)
	}
	withRec(t, e, "alice", func(r *record) { r.Floor, r.X, r.Y = manor.Floor, 10, 10 })
	if m := c.call("talk", `{"npc":242}`); m["ok"] != false {
		t.Fatalf("unsupported 传送员应被拒: %v", m)
	} else if code := m["error"].(map[string]any)["code"]; code != "NPC_NOT_TALKABLE" {
		t.Errorf("应回 NPC_NOT_TALKABLE: %v", m)
	}
}

func TestWarpmanPaidAndFreeLevelGate(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)

	// 19 号「矿工」（地图 100）：LV>0 免费（1 级也满足）。
	withRec(t, e, "alice", func(r *record) { r.Floor, r.X, r.Y = 100, 278, 325 })
	floorAfter := 0
	w := gamedata.NpcWarpmanOf(19)
	if w == nil {
		t.Fatal("19 号应是矿工传送员")
	}
	if m, pushes := c.callp("talk", `{"npc":19}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("talk 矿工应成功: %v %v", m, pushes)
	}
	if m, _ := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("免费传送应成功: %v", m)
	}
	withRec(t, e, "alice", func(r *record) { floorAfter = r.Floor })
	if floorAfter != w.Warp[0] {
		t.Errorf("应传到 %d，实际 %d", w.Warp[0], floorAfter)
	}

	// 98 号「洞窟管理员的长男」（地图 200）：LV>4，1 级不满足且 MONEY=-1 → 不弹窗。
	withRec(t, e, "alice", func(r *record) { r.Floor, r.X, r.Y = 200, 10, 10 })
	if gamedata.NpcWarpmanOf(98) == nil {
		t.Fatal("98 号应是洞窟管理员的长男")
	}
	if m, pushes := c.callp("talk", `{"npc":98}`); m["ok"] != false || len(pushes) != 0 {
		t.Fatalf("等级不够且不能付费时应回 NPC_NOT_TALKABLE 且不弹窗: %v %v", m, pushes)
	} else if code := m["error"].(map[string]any)["code"]; code != "NPC_NOT_TALKABLE" {
		t.Fatalf("应回 NPC_NOT_TALKABLE: %v", m)
	}
	// 升到 5 级后可以免费传。
	withRec(t, e, "alice", func(r *record) { r.Level = 5 })
	if m, pushes := c.callp("talk", `{"npc":98}`); m["ok"] != true || len(pushes) != 1 {
		t.Fatalf("5 级应弹确认窗: %v %v", m, pushes)
	} else if text := pushes[0]["data"].(map[string]any)["text"].(string); strings.Contains(text, "付费") {
		t.Errorf("LV>4 满足后应免费: %v", pushes[0])
	}
}

// TestWarpmanBlockedInBattle：战斗中 talk 被拒。
func TestWarpmanBlockedInBattle(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	c.okp("warp", `{"place":1}`)

	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "talk", fmt.Sprintf(`{"npc":%d}`, warpNPC), "IN_BATTLE")
	c.drainToEnd()
}

// STAGE-177：已经开着别的窗口时，被拒的传送员 talk 不弹窗、不关掉原来的窗口（不推 npc_window_closed）。
func TestWarpmanRejectKeepsOpenWindow(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) { r.Floor, r.X, r.Y = 200, 10, 10 })
	live := e.chars.LiveOf(e.session("alice"))
	if err := live.Operate(func(*record) { live.p.npcWindow = &npcWindow{npc: 1} }); err != nil {
		t.Fatal(err)
	}

	if m, pushes := c.callp("talk", `{"npc":98}`); m["ok"] != false || len(pushes) != 0 {
		t.Fatalf("被拒的 talk 不应推送任何窗口变化: %v %v", m, pushes)
	}
	if err := live.Operate(func(*record) {
		if live.p.npcWindow == nil || live.p.npcWindow.npc != 1 {
			t.Errorf("原来的窗口应保持不变，实际 %+v", live.p.npcWindow)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
