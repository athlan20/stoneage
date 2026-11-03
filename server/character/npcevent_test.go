package character

// 任务 NPC 对话（STAGE-90 验收，第一批「成人仪式」）：仪式的审判（1 号脚本）、仪式审判的差使（2 号脚本）的完整流程。
// 编号从 gamedata 里按名字和地图找，不写死。准备数据（背包、任务进度、魅力）放在进入世界之前，
// 让内存里的角色数据和数据库一致。

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const ceremonyFloor = 10204 // 仪式的审判和仪式审判的差使所在的地图

// npcNamed 按名字在地图里找任务 NPC 的编号。
func npcNamed(t *testing.T, floor int, name string) int {
	t.Helper()
	for _, n := range gamedata.NPCsByFloor(floor) {
		if n.Name == name && n.Kind == "changeevent" {
			return n.ID
		}
	}
	t.Fatalf("地图 %d 找不到任务 NPC「%s」", floor, name)
	return 0
}

// adultEnv 准备勇者小明站在 10204 图里，setup 在进入世界之前执行（只能写数据库），然后进入世界。
func adultEnv(t *testing.T, setup ...func(e *env)) (*env, *client) {
	t.Helper()
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", ceremonyFloor, 14, 6); err != nil {
		t.Fatal(err)
	}
	for _, f := range setup {
		f(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// exec 在数据库里执行一条准备数据的语句（只用于 setup）。
func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.chars.db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

// putItemSetup 往背包的一格里放道具（setup 用）。
func putItemSetup(t *testing.T, slot, itemID, count int) func(*env) {
	return func(e *env) {
		e.exec(t, `INSERT INTO character_items (character_id, slot, item_id, count)
			VALUES ((SELECT id FROM characters WHERE name = '勇者小明'), $1, $2, $3)`, slot, itemID, count)
	}
}

// fullBagSetup 把背包的格子（firstBagSlot 到 MaxItemSlot）全部占满，道具都是 item（setup 用）。
func fullBagSetup(t *testing.T, item int) func(*env) {
	return func(e *env) {
		for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
			putItemSetup(t, slot, item, 1)(e)
		}
	}
}

// questDoingSetup 让角色已经接了成人仪式（setup 用）。
func questDoingSetup(t *testing.T) func(*env) {
	return func(e *env) {
		e.exec(t, `INSERT INTO character_quests (character_id, quest_no, state)
			VALUES ((SELECT id FROM characters WHERE name = '勇者小明'), 1, $1)`, questDoing)
	}
}

// itemCounts 读数据库里角色的道具数（按编号合计）。
func (e *env) itemCounts(t *testing.T) map[int]int {
	t.Helper()
	rows, err := e.chars.db.Query(context.Background(), `SELECT ci.item_id, SUM(ci.count)::int FROM character_items ci
		JOIN characters c ON c.id = ci.character_id WHERE c.name = '勇者小明' GROUP BY ci.item_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		out[id] = n
	}
	return out
}

// questRows 读数据库里角色的任务进度（任务编号 → 状态）。
func (e *env) questRows(t *testing.T) map[int]int {
	t.Helper()
	rows, err := e.chars.db.Query(context.Background(), `SELECT cq.quest_no, cq.state FROM character_quests cq
		JOIN characters c ON c.id = cq.character_id WHERE c.name = '勇者小明'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var no, st int
		if err := rows.Scan(&no, &st); err != nil {
			t.Fatal(err)
		}
		out[no] = st
	}
	return out
}

// windowData 从推送里取 npc_window 的 data（没有则返回 nil）。
func windowData(pushes []map[string]any) map[string]any {
	for _, p := range pushes {
		if p["type"] == "npc_window" {
			return p["data"].(map[string]any)
		}
	}
	return nil
}

// hasPush 报告推送里有没有某种类型。
func hasPush(pushes []map[string]any, typ string) bool {
	for _, p := range pushes {
		if p["type"] == typ {
			return true
		}
	}
	return false
}

// expectWindow 检查窗口的正文（取前缀，跳过换行后的部分）和选项个数。
func expectWindow(t *testing.T, pushes []map[string]any, prefix string, options int) {
	t.Helper()
	d := windowData(pushes)
	if d == nil {
		t.Fatalf("应推送 npc_window（%s），实际推送 %v", prefix, pushes)
	}
	if text, _ := d["text"].(string); !strings.HasPrefix(text, prefix) {
		t.Fatalf("窗口正文应以「%s」开头，实际「%v」", prefix, d["text"])
	}
	if got := len(d["options"].([]any)); got != options {
		t.Fatalf("窗口选项应有 %d 个，实际 %d 个: %v", options, got, d["options"])
	}
}

// talkTo 和 NPC 说话（编号用 strconv 拼进请求）。
func talkTo(c *client, npc int) []map[string]any {
	_, pushes := c.callp("talk", `{"npc":`+strconv.Itoa(npc)+`}`)
	return pushes
}

// 完整流程（验收 1、2、3、4）：接任务（是）→ 领仪玉 → 交仪玉（是）→ 完成后再说话只剩闲谈。
func TestAdultCeremonyFullFlow(t *testing.T) {
	e, c := adultEnv(t)
	judge := npcNamed(t, ceremonyFloor, "仪式的审判")
	messenger := npcNamed(t, ceremonyFloor, "仪式审判的差使")

	// 没接过：审判官问“要不要开始成人礼”，选 1 是。
	expectWindow(t, talkTo(c, judge), "俺可是来考验你是否通过成人", 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, "在这条路尽头的一尊石像上", 0)
	if !hasPush(pushes, "quests_changed") {
		t.Fatalf("接任务后应推 quests_changed（任务页、消息栏）: %v", pushes)
	}
	if got := e.questRows(t); got[1] != questDoing {
		t.Fatalf("成人仪式（任务 1）应是进行中，实际 %v", got)
	}

	// 进行中：使者发 1 个仪玉（2417）。
	expectWindow(t, talkTo(c, messenger), "你可是来举行仪式", 2)
	_, pushes = c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, "那么，给你１个仪玉", 0)
	if !hasPush(pushes, "inventory_changed") {
		t.Fatalf("领到仪玉后应推 inventory_changed: %v", pushes)
	}
	if got := e.itemCounts(t)[2417]; got != 1 {
		t.Fatalf("应有 1 个仪玉，实际 %d", got)
	}

	// 持有 1 个仪玉回去交：收走仪玉，给 2418，任务变成已完成。
	expectWindow(t, talkTo(c, judge), "还挺厉害的嘛", 2)
	_, pushes = c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, "这么一来，你也是个顶天立地的成人了", 0)
	if got := e.itemCounts(t); got[2417] != 0 || got[2418] != 1 {
		t.Fatalf("应收走 1 个仪玉、给 1 件 2418，实际 %v", got)
	}
	if got := e.questRows(t); got[1] != questDone {
		t.Fatalf("成人仪式应是已完成，实际 %v", got)
	}
	if !hasPush(pushes, "quests_changed") || !hasPush(pushes, "inventory_changed") {
		t.Fatalf("交任务后应推 inventory_changed 和 quests_changed: %v", pushes)
	}

	// 完成之后：审判官只剩闲谈（原版的 ENDEV 块）。
	expectWindow(t, talkTo(c, judge), "以后也要不断地求进步", 0)
}

// 拒绝接任务：选否不记录进度，窗口关闭（原版 REQMAINMSG 的 NO 分支什么都不做）。
func TestAdultCeremonyDeclineKeepsNothing(t *testing.T) {
	e, c := adultEnv(t)
	judge := npcNamed(t, ceremonyFloor, "仪式的审判")
	talkTo(c, judge)
	_, pushes := c.callp("npc_choose", `{"no":2}`)
	if windowData(pushes) != nil {
		t.Fatalf("选否后不应再说话，实际 %v", pushes)
	}
	if got := e.questRows(t); len(got) != 0 {
		t.Fatalf("选否后不应有任务进度，实际 %v", got)
	}
}

// 没进行中的角色找使者领不到仪玉，只听到闲聊句（验收 2 的后半句）。
func TestMessengerGivesNothingWithoutQuest(t *testing.T) {
	e, c := adultEnv(t)
	messenger := npcNamed(t, ceremonyFloor, "仪式审判的差使")
	expectWindow(t, talkTo(c, messenger), "我是审判仪式的使者", 0)
	if got := e.itemCounts(t); len(got) != 0 {
		t.Fatalf("没接任务不应领到道具，实际 %v", got)
	}
}

// 背包一格都没有空：说 ItemFullMsg 原话，不给东西、不改进度（验收 2、4）。只需 1 格，所以占满全部格子才会满。
func TestMessengerBagFullChangesNothing(t *testing.T) {
	e, c := adultEnv(t, questDoingSetup(t), fullBagSetup(t, 2418)) // 全部占满（2418 不是仪玉）
	messenger := npcNamed(t, ceremonyFloor, "仪式审判的差使")
	expectWindow(t, talkTo(c, messenger), "你可是来举行仪式", 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, "你的道具袋没有空间了", 0)
	if got := e.itemCounts(t); got[2417] != 0 || got[2418] != MaxItemSlot-firstBagSlot+1 || len(got) != 1 {
		t.Fatalf("背包满时不应给东西（只有占位的道具），实际 %v", got)
	}
	if got := e.questRows(t); got[1] != questDoing {
		t.Fatalf("背包不够时进度不应变，实际 %v", got)
	}
}

// 持有 2 个仪玉（旧存档多出来的）交审判官：只收 1 个，剩下的留在背包里（验收 4）。
func TestJudgeTakesOnlyOneChip(t *testing.T) {
	e, c := adultEnv(t, questDoingSetup(t), putItemSetup(t, firstBagSlot, 2417, 2))
	judge := npcNamed(t, ceremonyFloor, "仪式的审判")
	expectWindow(t, talkTo(c, judge), "还挺厉害的嘛", 2)
	_, pushes := c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, "这么一来，你也是个顶天立地的成人了", 0)
	if got := e.itemCounts(t); got[2417] != 1 || got[2418] != 1 {
		t.Fatalf("应只收 1 个仪玉、剩 1 个，并给 1 件 2418，实际 %v", got)
	}
	if got := e.questRows(t); got[1] != questDone {
		t.Fatalf("成人仪式应是已完成，实际 %v", got)
	}
}

// 进行中却没有仪玉：找审判官不算完成，也不给 2418（验收 3）。
func TestJudgeWithoutChipNotDone(t *testing.T) {
	e, c := adultEnv(t, questDoingSetup(t))
	judge := npcNamed(t, ceremonyFloor, "仪式的审判")
	talkTo(c, judge)
	if got := e.itemCounts(t); len(got) != 0 {
		t.Fatalf("没有仪玉时不应有道具变化，实际 %v", got)
	}
	if got := e.questRows(t); got[1] != questDoing {
		t.Fatalf("没有仪玉时仪式应仍是进行中，实际 %v", got)
	}
}

// 放弃：进行中选“是”，说 EndStopMsg、任务消失、魅力减 1；选“否”说 NoStopMsg，什么都不变。
func TestAdultCeremonyGiveUp(t *testing.T) {
	e, c := adultEnv(t, questDoingSetup(t), func(e *env) {
		e.exec(t, `UPDATE characters SET charm = 5 WHERE name = '勇者小明'`)
	})
	judge := npcNamed(t, ceremonyFloor, "仪式的审判")

	expectWindow(t, talkTo(c, judge), "怎么啦！想打退堂鼓了吗", 2)
	_, pushes := c.callp("npc_choose", `{"no":2}`)
	expectWindow(t, pushes, "俗话说的好", 0)
	if got := e.questRows(t); got[1] != questDoing {
		t.Fatalf("选否后仍应进行中，实际 %v", got)
	}

	talkTo(c, judge)
	_, pushes = c.callp("npc_choose", `{"no":1}`)
	expectWindow(t, pushes, "我看你还是回去多练练吧", 0)
	if got := e.questRows(t); len(got) != 0 {
		t.Fatalf("放弃后任务应消失，实际 %v", got)
	}
	c.ok("leave_world", `{}`) // 离开世界时保存（魅力只在保存时写库）
	c.ok("enter_world", `{"slot":0}`)
	var charm int
	if err := e.chars.db.QueryRow(context.Background(), `SELECT charm FROM characters WHERE name = '勇者小明'`).Scan(&charm); err != nil {
		t.Fatal(err)
	}
	if charm != 4 {
		t.Fatalf("放弃后魅力应从 5 减到 4，实际 %d", charm)
	}
}

// 没有放开的任务脚本（萨姆吉尔的村长，村长家）：对话回“还不能对话”（验收之外的兜底）。
func TestUnlistedEventNpcCannotTalk(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", 1006, 16, 16); err != nil {
		t.Fatal(err)
	}
	c.ok("enter_world", `{"slot":0}`)
	chief := npcNamed(t, 1006, "萨姆吉尔的村长")
	c.wantErr(t, "talk", `{"npc":`+strconv.Itoa(chief)+`}`, "NPC_NOT_TALKABLE")
}
