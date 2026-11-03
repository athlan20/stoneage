package character

// 战斗 NPC（STAGE-96，docs/battle-npc.md 5.2–5.5，协议 5.33、5.35、10.3、10.9）：里昂蛙王（30605）的台词窗口、
// 是/否确认、开战推送、占用（一个人在打时别人得到占用台词，掉线后释放）、胜利后传送并释放占用、确认后再检查，
// 以及白名单、道场弟子不支持、steal 收道具的时机。

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

const frogOrb = 19622 // 黑蛙王的金珠：里昂蛙王开战的前置道具

// frogNPC 返回里昂蛙王（30605 的战斗 NPC）的编号。
func frogNPC(t *testing.T) int {
	t.Helper()
	ne, ok := gamedata.NpcEnemyByFile("king/event69_5")
	if !ok {
		t.Fatal("没有里昂蛙王的战斗参数")
	}
	return ne.ID
}

// newEnemyEnv 同 newPointsEnv，可以换上测试用的设置（比如白名单）。
func newEnemyEnv(t *testing.T, setup ...func(*Service)) *env {
	t.Helper()
	e := newEnv(t, setup...)
	if err := e.chars.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.chars.Stop)
	return e
}

// enterAt 准备勇者小明站在 (floor, x, y) 并进入世界；withOrb 为真时背包 5 号格里有一个金珠。
// 宠物清空，让开战的结果只取决于剧本的种子。
func enterAt(t *testing.T, floor, x, y int, withOrb bool, setup ...func(*Service)) (*env, *client) {
	t.Helper()
	e := newEnemyEnv(t, setup...)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", floor, x, y); err != nil {
		t.Fatal(err)
	}
	if withOrb {
		putItemSetup(t, 5, frogOrb, 1)(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// frogEnv 让勇者小明站在里昂蛙王身边（30605，7,9）。
func frogEnv(t *testing.T, withOrb bool, setup ...func(*Service)) (*env, *client) {
	t.Helper()
	return enterAt(t, 30605, 7, 9, withOrb, setup...)
}

// talkFrog 和里昂蛙王说话，返回这次 talk 收到的推送。
func talkFrog(t *testing.T, c *client) []map[string]any {
	t.Helper()
	m, pushes := c.callp("talk", `{"npc":`+strconv.Itoa(frogNPC(t))+`}`)
	if m["ok"] != true {
		t.Fatalf("talk 应成功: %v", m)
	}
	return pushes
}

// findPush 返回推送里第一个指定类型的 data；没有返回 nil。
func findPush(pushes []map[string]any, typ string) map[string]any {
	for _, m := range pushes {
		if m["type"] == typ {
			d, _ := m["data"].(map[string]any)
			return d
		}
	}
	return nil
}

// onlyWindow 断言推送里恰好有一个 npc_window，返回它的 data。
func onlyWindow(t *testing.T, pushes []map[string]any) map[string]any {
	t.Helper()
	var wins []map[string]any
	for _, m := range pushes {
		if m["type"] == "npc_window" {
			wins = append(wins, m["data"].(map[string]any))
		}
	}
	if len(wins) != 1 {
		t.Fatalf("应推一个 npc_window，实际 %v", pushes)
	}
	return wins[0]
}

// options 取窗口的选项数量（窗口的 options 一定是数组，不是 null）。
func options(t *testing.T, win map[string]any) []any {
	t.Helper()
	opts, ok := win["options"].([]any)
	if !ok {
		t.Fatalf("options 应是数组: %v", win)
	}
	return opts
}

// itemCount 数数据库里角色持有某个道具的数量（断言背包用）。
func (e *env) itemCount(t *testing.T, name string, itemID int) int {
	t.Helper()
	var n int
	if err := e.chars.db.QueryRow(context.Background(), `SELECT COALESCE(SUM(ci.count), 0) FROM character_items ci
		JOIN characters c ON c.id = ci.character_id WHERE c.name = $1 AND ci.item_id = $2`, name, itemID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// waitUntil 等条件成立（服务端在另一个协程里处理的收场，比如掉线，要等它做完）。
func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("3 秒内没有等到：%s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 没有金珠：talk 弹拒绝台词，没有选项，不开战，背包不变。
func TestEnemyFrogDeniedWithoutOrb(t *testing.T) {
	_, c := frogEnv(t, false)
	win := onlyWindow(t, talkFrog(t, c))
	if win["name"] != "里昂蛙王" || win["type"] != "战斗" {
		t.Errorf("窗口的名字和类型不对: %v", win)
	}
	if text := win["text"]; text != "这里不是你们人类该来的地方，快滚！" {
		t.Errorf("拒绝台词不对: %v", text)
	}
	if opts := options(t, win); len(opts) != 0 {
		t.Errorf("拒绝窗口没有选项，实际 %v", opts)
	}
	if q, _ := c.roundtrip(t, "battle", `{}`, ""); q["in_battle"] != false {
		t.Errorf("没有金珠时不应开战: %v", q)
	}
	c.callp("npc_close", `{}`)
}

// 有金珠：talk 弹两行确认（是/否）；选“否”只关窗口，不开战。
func TestEnemyFrogAskThenDecline(t *testing.T) {
	_, c := frogEnv(t, true)
	win := onlyWindow(t, talkFrog(t, c))
	if text := win["text"]; text != "该死的人类，偷了我的东西还敢回来？\n今天给我逮到了，一定要好好教训你！" {
		t.Errorf("确认台词不对（两行按换行连起来）: %q", text)
	}
	opts := options(t, win)
	if len(opts) != 2 || opts[0].(map[string]any)["label"] != "是" || opts[1].(map[string]any)["label"] != "否" {
		t.Fatalf("选项应是 1 是、2 否: %v", opts)
	}
	_, pushes := c.callp("npc_choose", `{"no":2}`)
	if closed := findPush(pushes, "npc_window_closed"); closed["reason"] != WindowReasonChosen {
		t.Errorf("选否应推 CHOSEN 关窗: %v", pushes)
	}
	if findPush(pushes, "battle_start") != nil {
		t.Error("选否不应开战")
	}
}

// 有金珠：选“是”先关窗（CHOSEN），再推 battle_start：kind npc，npc.say 是开战台词，敌方 10 个（1 黑蛙王 + 9 黑蛙）。
// 金珠没配 steal，开战后还在背包里；NPC 被这场战斗占用。
func TestEnemyFrogFight(t *testing.T) {
	e, c := frogEnv(t, true)
	talkFrog(t, c)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	if closed := findPush(pushes, "npc_window_closed"); closed["reason"] != WindowReasonChosen {
		t.Errorf("选是应先推 CHOSEN 关窗: %v", pushes)
	}
	start := findPush(pushes, "battle_start")
	if start == nil {
		t.Fatalf("选是应推 battle_start: %v", pushes)
	}
	if start["kind"] != "npc" {
		t.Errorf("kind 应是 npc: %v", start["kind"])
	}
	npc, _ := start["npc"].(map[string]any)
	if npc["id"] != float64(frogNPC(t)) || npc["name"] != "里昂蛙王" || npc["say"] != "看我怎么修理你们这些人类！" {
		t.Errorf("battle_start 的 npc 不对: %v", npc)
	}
	monsters := 0
	for _, u := range start["units"].([]any) {
		if u.(map[string]any)["kind"] == "monster" {
			monsters++
		}
	}
	if monsters != 10 {
		t.Errorf("敌方应有 10 个单位，实际 %d", monsters)
	}
	if q, _ := c.roundtrip(t, "battle", `{}`, ""); q["npc"] == nil {
		t.Errorf("battle 查询应带 npc: %v", q)
	} else if q["npc"].(map[string]any)["say"] != nil {
		t.Errorf("battle 查询的 npc 不带台词: %v", q["npc"])
	}
	if !e.chars.npcs.Busy(frogNPC(t)) {
		t.Error("开战后里昂蛙王应被占用")
	}
	if n := e.itemCount(t, "勇者小明", frogOrb); n != 1 {
		t.Errorf("没配 steal 的道具开战后还在背包里，实际 %d 个", n)
	}
}

// 窗口开着时金珠被丢掉，再选“是”：重新检查，不开战，换成拒绝台词（STAGE-96 的确认后再检查）。
func TestEnemyFrogChooseRechecks(t *testing.T) {
	_, c := frogEnv(t, true)
	talkFrog(t, c)
	c.callp("item_drop", `{"slot":5,"confirm":"yes"}`)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	if findPush(pushes, "battle_start") != nil {
		t.Fatalf("金珠没了就不能开战: %v", pushes)
	}
	if win := onlyWindow(t, pushes); win["text"] != "这里不是你们人类该来的地方，快滚！" {
		t.Errorf("应换成拒绝台词: %v", win)
	}
}

// 一个人在打里昂蛙王时，第二个人 talk 得到占用台词（没有选项）；第一个人掉线（收场）后占用释放，第二个人又能打。
func TestEnemyFrogBusy(t *testing.T) {
	e := newEnemyEnv(t)
	a := e.loggedIn("alice")
	a.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	b := e.loggedIn("bob")
	b.ok("create_character", createJSON(0, "勇者小红", 0))
	clearStartPets(t, e, "勇者小红")
	for _, name := range []string{"勇者小明", "勇者小红"} {
		if err := MoveCharacter(context.Background(), e.chars.db, name, 30605, 7, 9); err != nil {
			t.Fatal(err)
		}
		e.exec(t, `INSERT INTO character_items (character_id, slot, item_id, count)
			VALUES ((SELECT id FROM characters WHERE name = $1), 5, $2, 1)`, name, frogOrb)
	}
	a.ok("enter_world", `{"slot":0}`)
	b.ok("enter_world", `{"slot":0}`)

	talkFrog(t, a)
	a.callp("npc_choose", `{"no":1}`)
	win := onlyWindow(t, talkFrog(t, b))
	if win["text"] != "等我解决这群人类后马上就轮到你！" || len(options(t, win)) != 0 {
		t.Errorf("被占用时应说占用台词，没有选项: %v", win)
	}
	b.callp("npc_close", `{}`)

	a.conn.Close() // 第一个人掉线：战斗收场，占用释放
	waitUntil(t, "里昂蛙王的占用释放", func() bool { return !e.chars.npcs.Busy(frogNPC(t)) })
	if win := onlyWindow(t, talkFrog(t, b)); len(options(t, win)) != 2 {
		t.Errorf("占用释放后应又弹确认窗口: %v", win)
	}
}

// npc_win 剧本开的里昂蛙王战斗打赢：battle_end 带结束台词，随后 position_changed 到 30607（7,11），
// 占用释放，NPC 留在 30605，还能再打（talk 又弹确认）。
func TestEnemyFrogWinWarps(t *testing.T) {
	e, c := frogEnv(t, true)
	if err := e.chars.StartBattle("勇者小明", "npc_win"); err != nil {
		t.Fatal(err)
	}
	start := c.drainTo(t, "battle_start")
	if npc, _ := start["npc"].(map[string]any); npc["say"] != "看我怎么修理你们这些人类！" {
		t.Errorf("开战台词不对: %v", start["npc"])
	}
	c.drainTo(t, "battle_turn")
	end := c.playWildToEnd(t)
	if end["result"] != "win" {
		t.Fatalf("npc_win 应该打赢: %v", end["result"])
	}
	if npc, _ := end["npc"].(map[string]any); npc["say"] != "我不甘心啊！…" {
		t.Errorf("battle_end 应带结束台词: %v", end["npc"])
	}
	pos := c.drainTo(t, "position_changed")
	if pos["floor"] != float64(30607) || pos["x"] != float64(7) || pos["y"] != float64(11) {
		t.Errorf("赢了应传送到 30607 的 7,11: %v", pos)
	}
	if e.chars.npcs.Busy(frogNPC(t)) {
		t.Error("赢了占用应释放")
	}
	if n := e.itemCount(t, "勇者小明", frogOrb); n != 1 {
		t.Errorf("没配 steal 的金珠赢了还在背包里，实际 %d 个", n)
	}
	if floor := e.npcFloor(t, frogNPC(t)); floor != 30605 {
		t.Errorf("赢了 NPC 留在原地（30605），实际 %d", floor)
	}
	// 人已经到了 30607，隔着地图对 30605 的 NPC 说话回 NPC_NOT_HERE（回去再打，走的是同一条 talk）。
	c.expectError(t, "talk", `{"npc":`+strconv.Itoa(frogNPC(t))+`}`, "NPC_NOT_HERE")
}

// npcFloor 取 NPC 当前所在的地图（NPC 表里的位置）。
func (e *env) npcFloor(t *testing.T, id int) int {
	t.Helper()
	n, ok := e.chars.npcs.ByID(id)
	if !ok {
		t.Fatalf("NPC %d 不在 NPC 表里", id)
	}
	return n.Floor
}

// 白名单没放开的战斗 NPC（测试换成空白名单）回“还不能对话”，即使前置道具齐全。
func TestEnemyNotWhitelisted(t *testing.T) {
	_, c := frogEnv(t, true, func(s *Service) {
		s.opts.NpcEnemyEnabled = func(string) bool { return false }
	})
	c.expectError(t, "talk", `{"npc":`+strconv.Itoa(frogNPC(t))+`}`, "NPC_NOT_TALKABLE")
}

// 道场弟子（1021 的战斗 NPC）标了不支持：白名单写成全放开也不放行，talk 回“还不能对话”。
func TestEnemyGymNotTalkable(t *testing.T) {
	gym := 0
	for _, n := range gamedata.NPCsByFloor(1021) {
		if n.Type == "战斗" {
			gym = n.ID
			break
		}
	}
	if gym == 0 {
		t.Fatal("1021 应有道场弟子")
	}
	_, c := enterAt(t, 1021, 10, 10, false, func(s *Service) {
		s.opts.NpcEnemyEnabled = func(string) bool { return true }
	})
	c.expectError(t, "talk", `{"npc":`+strconv.Itoa(gym)+`}`, "NPC_NOT_TALKABLE")
}

// steal：开战时（steal=0）不收，赢了（steal=1）才收，收走的推 inventory_changed 由调用方做。
func TestEnemyStealTiming(t *testing.T) {
	e, _ := frogEnv(t, true)
	p := e.presence(t, "勇者小明")
	spec := &gamedata.NpcEnemy{Items: []int{frogOrb}, Steal: stealWin}
	p.mu.Lock()
	early := e.chars.takeEnemyItems(p, spec, stealStart)
	won := e.chars.takeEnemyItems(p, spec, stealWin)
	p.mu.Unlock()
	if early || !won {
		t.Errorf("开战时不该收、赢了才收：early=%v won=%v", early, won)
	}
	if n := e.itemCount(t, "勇者小明", frogOrb); n != 0 {
		t.Errorf("赢了收走后金珠应没了，实际 %d 个", n)
	}
}

// presence 取在世界中的角色（测试直接动它的背包时用）。
func (e *env) presence(t *testing.T, name string) *Presence {
	t.Helper()
	e.chars.mu.Lock()
	defer e.chars.mu.Unlock()
	for _, p := range e.chars.presences {
		if p.rec.Name == name {
			return p
		}
	}
	t.Fatalf("%s 不在世界中", name)
	return nil
}
