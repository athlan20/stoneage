package character

// STAGE-238：精灵王任务 49→54 的协议端到端（需要数据库）。真实的 warp、talk、npc_choose，断言每一步的任务进度、
// 道具、宠物和推送。选块和交易都由服务端决定；这里只替玩家一路点“是”（或“下一页”）。
// 纯逻辑的同一条链在 npcevent_king_test.go（TestKingChain49To50、TestKingChain51To54）。
// 任务 51 的“进行中”在真实流程里由宠物转生设置（STAGE-237，见 npc_pettrans_items 用例），这里用管理命令的同一条路径
// （SetQuest：写库并 NOTIFY）在中途设上，之后的 52–54 都是真实对话。

import (
	"strconv"
	"testing"
)

// 精灵的羽毛 13061–13064 是随机给一件，这里合起来数。
var kingFeathers = []int{13061, 13062, 13063, 13064}

// kingChainItems 是 51 期间要用的原石和宝石（掉落来源，见 server/gamedata/enemies.tsv）：19633–19636 原石、19637–19640 宝石。
// 开局就放进背包；海精灵的两次对话会按块的条件先换四色原石、再做灵力铠，中间不需要再补道具。
func kingChainItems(t *testing.T) func(*env) {
	return func(e *env) {
		for i, id := range []int{19633, 19634, 19635, 19636, 19637, 19638, 19639, 19640} {
			putItemSetup(t, firstBagSlot+i, id, 1)(e)
		}
	}
}

// kingLastWindow 取推送里最后一个 npc_window（一次响应里可能先关旧窗口、再开新窗口）；没有则返回 nil。
func kingLastWindow(pushes []map[string]any) map[string]any {
	var last map[string]any
	for _, p := range pushes {
		if p["type"] == "npc_window" {
			last = p["data"].(map[string]any)
		}
	}
	return last
}

// kingVisit 传送到 floor（warp，免费、没有移动过程），和 npc 说话，一路点“下一页”或“是”，直到窗口上没有这两个选项为止
// （读完最后一页时只剩“上一页”，不再点）。返回这次访问的全部推送（按顺序）。
func kingVisit(t *testing.T, c *client, floor, npc int) []map[string]any {
	t.Helper()
	if m, _ := c.callp("warp", `{"floor":`+strconv.Itoa(floor)+`}`); m["ok"] != true {
		t.Fatalf("传送到地图 %d 应成功: %v", floor, m)
	}
	var all []map[string]any
	pushes := talkTo(c, npc)
	all = append(all, pushes...)
	for i := 0; i < 8; i++ {
		no := kingAdvanceNo(kingLastWindow(pushes))
		if no == 0 {
			return all
		}
		_, pushes = c.callp("npc_choose", `{"no":`+strconv.Itoa(no)+`}`)
		all = append(all, pushes...)
	}
	t.Fatalf("NPC %d 的窗口连续 8 次仍能往下走，流程没有结束", npc)
	return nil
}

// kingAdvanceNo 在窗口的选项里找往下走的那一项（“下一页”或“是”），返回它的编号；没有则返回 0。
func kingAdvanceNo(w map[string]any) int {
	if w == nil {
		return 0
	}
	opts, _ := w["options"].([]any)
	for _, o := range opts {
		m := o.(map[string]any)
		if label, _ := m["label"].(string); label == "下一页" || label == "是" {
			return int(m["no"].(float64))
		}
	}
	return 0
}

// kingFeatherCount 数背包里的精灵的羽毛（13061–13064 任一件）。
func kingFeatherCount(got map[int]int) int {
	n := 0
	for _, id := range kingFeathers {
		n += got[id]
	}
	return n
}

// 任务 49 → 54 从头到尾走一遍（协议层）：
//
//	49：祖父接 69；新藏给金珠、蛙王换黑玉、新藏接 70（收黑玉）、新藏结束 69；祖母给项链和玛蕾菲雅，任务 50 完成。
//	51：中途设进行中（转生）；海精灵两次（原石换四色原石、四色原石加宝石做灵力铠，任务 52、53 完成）。
//	54：玛蕾菲雅接 72（任务 54 进行中）、再对话给羽毛（任务 51 完成）；四个守护各给一枚魔法戒指；
//	    柯黑穆肯给宠物 484 并结束 72（任务 54 完成）。
func TestKingChain49To54FlowDB(t *testing.T) {
	e, c := kingEnv(t, 2000, questStateSetup(t, 1, questDone), clearPetsSetup(t), kingChainItems(t))

	// 49：祖父（2000）接任务 69。
	pushes := kingVisit(t, c, 2000, kingGrandpa)
	if got := e.questRows(t); got[49] != questDoing {
		t.Fatalf("接了 69，任务 49 应进行中，实际 %v", got)
	}
	if !hasPush(pushes, "quests_changed") {
		t.Fatalf("接任务后应推 quests_changed: %v", pushes)
	}

	// 49→50：新藏（30602）给金珠，蛙王（30607）换黑玉，新藏接 70 收走黑玉，新藏结束 69。
	kingVisit(t, c, 30602, kingShin)
	if got := e.itemCounts(t); got[kingFrogGold] != 1 {
		t.Fatalf("新藏应给金珠 19622，实际 %v", got)
	}
	kingVisit(t, c, 30607, kingFrog)
	if got := e.itemCounts(t); got[kingFrogGold] != 0 || got[kingBlackJade] != 1 {
		t.Fatalf("蛙王应收金珠、给黑玉，实际 %v", got)
	}
	kingVisit(t, c, 30602, kingShin) // 接 70
	if got := e.itemCounts(t); got[kingBlackJade] != 0 {
		t.Fatalf("接 70 应收走黑玉，实际 %v", got)
	}
	if got := e.questRows(t); got[50] != questDoing || got[49] != questDoing {
		t.Fatalf("接 70 后 49、50 都应进行中，实际 %v", got)
	}
	kingVisit(t, c, 30602, kingShin) // 结束 69
	if got := e.questRows(t); got[49] != questDone {
		t.Fatalf("新藏结束后任务 49 应已完成，实际 %v", got)
	}

	// 50：祖母（2000）给项链和玛蕾菲雅（宠物 1479），任务 50 完成。
	pushes = kingVisit(t, c, 2000, kingGrandma)
	if got := e.itemCounts(t); got[kingNecklace] != 1 {
		t.Fatalf("应拿到项链 19624，实际 %v", got)
	}
	if got := e.petRows(t); len(got) != 1 {
		t.Fatalf("应有 1 只宠物（玛蕾菲雅），实际 %v", got)
	}
	if got := e.questRows(t); got[50] != questDone {
		t.Fatalf("任务 50 应已完成，实际 %v", got)
	}
	for _, typ := range []string{"inventory_changed", "pets_changed", "quests_changed"} {
		if !hasPush(pushes, typ) {
			t.Fatalf("祖母给完应推 %s: %v", typ, pushes)
		}
	}

	// 51：宠物转生设进行中（真实流程见 npc_pettrans_items 用例），这里用管理命令的同一条路径设上。
	// 写库之后离开世界再进入，让内存里的进度和数据库一致（测试环境里运行中的重读通知不可用）。
	c.callp("leave_world", `{}`)
	e.setQuest("勇者小明", 51, "doing")
	if got := e.questRows(t); got[51] != questDoing {
		t.Fatalf("任务 51 应进行中，实际 %v", got)
	}
	c.ok("enter_world", `{"slot":0}`)

	// 51：海精灵（30619）第一次：四原石换四色原石（19645），原石收走，52、53 还没完成。
	kingVisit(t, c, 30619, kingMarina)
	if got := e.itemCounts(t); got[kingFourColor] != 1 || got[19633] != 0 {
		t.Fatalf("应收走四原石、给四色原石 19645，实际 %v", got)
	}
	// 51：海精灵第二次：四宝石加四色原石换灵力铠（1292），事件 122、123 完成，即任务 52、53。
	kingVisit(t, c, 30619, kingMarina)
	if got := e.itemCounts(t); got[kingArmor] != 1 || got[kingFourColor] != 0 || got[19637] != 0 {
		t.Fatalf("应只剩灵力铠 1292，实际 %v", got)
	}
	if got := e.questRows(t); got[52] != questDone || got[53] != questDone || got[51] != questDoing {
		t.Fatalf("做完灵力铠后 52、53 应完成、51 仍进行中，实际 %v", got)
	}
	kingVisit(t, c, 30619, kingSamgil) // 萨姆吉尔只确认进度，不发奖

	// 54：玛蕾菲雅（30691）第一次：71 进行中、72 没接，接任务 72。
	kingVisit(t, c, 30691, kingMaleifa)
	if got := e.questRows(t); got[54] != questDoing || got[51] != questDoing {
		t.Fatalf("玛蕾菲雅接 72 后 54、51 都应进行中，实际 %v", got)
	}
	// 玛蕾菲雅第二次：71、72 都进行中，给羽毛（随机一件），任务 51 完成，54 仍进行中。
	kingVisit(t, c, 30691, kingMaleifa)
	if got := e.questRows(t); got[51] != questDone || got[54] != questDoing {
		t.Fatalf("给羽毛后任务 51 应完成、54 仍进行中，实际 %v", got)
	}
	if got := e.itemCounts(t); kingFeatherCount(got) != 1 {
		t.Fatalf("应拿到 1 件精灵的羽毛（13061–13064），实际 %v", got)
	}

	// 54：四个守护各给一枚魔法戒指（19641–19644）。
	for _, g := range []struct{ floor, npc, ring int }{
		{30696, kingYarus, kingFirstRing},
		{30697, kingWatiel, kingRingFromWat},
		{30698, kingFeary, kingFirstRing + 2},
		{30699, kingWindeth, kingFirstRing + 3},
	} {
		kingVisit(t, c, g.floor, g.npc)
		if got := e.itemCounts(t); got[g.ring] != 1 {
			t.Fatalf("守护 %d 应给戒指 %d，实际 %v", g.npc, g.ring, got)
		}
	}

	// 54：柯黑穆肯（30670）给宠物 484（邦达霍恩），结束 72，任务 54 完成。
	kingVisit(t, c, 30670, kingKohemut)
	if got := e.questRows(t); got[54] != questDone {
		t.Fatalf("柯黑穆肯结束后任务 54 应已完成，实际 %v", got)
	}
	if got := e.petRows(t); len(got) != 2 {
		t.Fatalf("应有 2 只宠物（玛蕾菲雅、邦达霍恩），实际 %v", got)
	}

	// 全部完成后再说话只闲谈：不再发道具和宠物，任务进度不变。
	before := e.itemCounts(t)
	kingVisit(t, c, 30670, kingKohemut)
	kingVisit(t, c, 30691, kingMaleifa)
	kingVisit(t, c, 30696, kingYarus)
	if got := e.itemCounts(t); len(got) != len(before) {
		t.Fatalf("完成后闲谈不应改动背包，之前 %v 之后 %v", before, got)
	}
	if got := e.petRows(t); len(got) != 2 {
		t.Fatalf("完成后闲谈不应多宠物，实际 %v", got)
	}
	for _, q := range []int{49, 50, 51, 52, 53, 54} {
		if got := e.questRows(t); got[q] != questDone {
			t.Fatalf("任务 %d 应已完成，实际 %v", q, got)
		}
	}
}
