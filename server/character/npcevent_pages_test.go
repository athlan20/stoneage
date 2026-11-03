package character

// 任务 NPC 的分页台词（STAGE-97 验收）。纯逻辑：窗口的选项随页变化、翻页只换页（eventPageOpts、turn、eventTexts）。
// 流程（需要数据库）：测试里把两个脚本放进白名单（Options.NpcEventEnabled，正式白名单要等放开的需求才改），
// 其余行为照常。编号从 gamedata 里按脚本文件找。

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 测试用的两个脚本：库伊老爷爷的“拿护身符”（king/event69_1，任务 49 进行中、任务 1 已完成，3 页问话），
// 塔姆塔姆村的草药（jaruga/event/event01_2，任务 30 进行中，2 页问话 + 4 页感谢话）。
const (
	pagedAmuletFile = "king/event69_1"
	pagedHerbFile   = "jaruga/event/event01_2"
	amuletItem      = 19621 // 护身符
	herbItem        = 2401  // 草药
)

// eventPageOptsOf 取窗口的选项编号和文字（只看 No、Label），方便比较。
func eventPageOptsOf(ev *eventSession) []npcOption {
	var out []npcOption
	for _, o := range eventPageOpts(ev) {
		out = append(out, o.npcOption)
	}
	return out
}

// 选项随页变化：不是最后一页只有“下一页”；最后一页的问答是“是、否”；第 2 页起多“上一页”（放最后）。
func TestEventPageOptions(t *testing.T) {
	ask := func(page, n int) *eventSession {
		return &eventSession{ask: askAccept, pages: make([]string, n), page: page}
	}
	read := func(page, n int, keyword bool) *eventSession {
		return &eventSession{ask: askRead, pages: make([]string, n), page: page, keyword: keyword}
	}
	cases := []struct {
		name string
		ev   *eventSession
		want []string
	}{
		{"问话第 1 页（共 3 页）", ask(0, 3), []string{"下一页"}},
		{"问话第 2 页", ask(1, 3), []string{"下一页", "上一页"}},
		{"问话最后一页", ask(2, 3), []string{"是", "否", "上一页"}},
		{"只有一页的问话", ask(0, 1), []string{"是", "否"}},
		{"读完的台词第 1 页", read(0, 2, false), []string{"下一页"}},
		{"读完的台词最后一页", read(1, 2, false), []string{"上一页"}},
		{"只有一页的读完台词", read(0, 1, false), nil},
		{"读完的最后一页带暗号", read(1, 2, true), []string{"说暗号", "上一页"}},
		{"读完的只有一页带暗号", read(0, 1, true), []string{"说暗号"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := eventPageOptsOf(tc.ev)
			if len(got) != len(tc.want) {
				t.Fatalf("选项应是 %v，实际 %v", tc.want, got)
			}
			for i, o := range got {
				if o.No != i+1 || o.Label != tc.want[i] {
					t.Fatalf("第 %d 个选项应是 %d %s，实际 %d %s", i+1, i+1, tc.want[i], o.No, o.Label)
				}
			}
		})
	}
}

// 问话的选项作用：下一页、上一页不是问答；是/否只在最后一页出现（选项编号 1、2 对应的作用）。
func TestEventPageActs(t *testing.T) {
	ev := &eventSession{ask: askAccept, pages: make([]string, 3), page: 1}
	ev.opts = eventPageOpts(ev)
	if ev.opts[0].act != actNext || ev.opts[1].act != actPrev {
		t.Fatalf("中间页的选项应是下一页、上一页，实际 %+v", ev.opts)
	}
	last := ev.turn(actNext)
	last.opts = eventPageOpts(last)
	if last.opts[0].act != actYes || last.opts[1].act != actNo || last.opts[2].act != actPrev {
		t.Fatalf("最后一页的选项应是是、否、上一页，实际 %+v", last.opts)
	}
	if back := last.turn(actPrev); back.page != 1 {
		t.Fatalf("上一页应回到第 2 页，实际第 %d 页", back.page+1)
	}
}

// 台词的页取法：分页台词按页序；单页台词只有一页；没有台词返回 nil；金币占位每页都替换。
func TestEventMsgPages(t *testing.T) {
	m := gamedata.EventMsg{
		Request: "单页问话",
		Paged:   map[string][]string{"accept": {"甲", "乙 %8d", "丙"}, "thanks": {"谢"}},
	}
	if got := m.Pages("accept"); len(got) != 3 || got[2] != "丙" {
		t.Fatalf("分页台词应按页序返回，实际 %v", got)
	}
	if got := m.Pages("request"); len(got) != 1 || got[0] != "单页问话" {
		t.Fatalf("单页台词应是一页，实际 %v", got)
	}
	if got := m.Pages("nomal"); got != nil {
		t.Fatalf("没有台词应返回 nil，实际 %v", got)
	}
	if got := eventTexts(m, "accept", 15); got[1] != "乙 15" {
		t.Fatalf("每一页的金币占位都要替换，实际 %v", got)
	}
	if got := eventTexts(m, "nomal", 15); len(got) != 0 {
		t.Fatalf("没有台词时不应有页，实际 %v", got)
	}
}

// pagedFileNPC 按脚本文件找 NPC（编号和所在地图）。
func pagedFileNPC(t *testing.T, file string) gamedata.NPC {
	t.Helper()
	for id := 1; id <= 5000; id++ {
		ev, ok := gamedata.NpcEventOf(id)
		if !ok || ev.File != file {
			continue
		}
		if npc, ok := gamedata.NPCByID(id); ok {
			return npc
		}
	}
	t.Fatalf("找不到脚本 %s 的 NPC", file)
	return gamedata.NPC{}
}

// pagedEnv 准备勇者小明站在脚本 NPC 所在的地图上（进入世界之前写数据库，setup 同 adultEnv），白名单里放进 files。
func pagedEnv(t *testing.T, npc gamedata.NPC, files []string, setup ...func(*env)) (*env, *client) {
	t.Helper()
	e := newEnv(t, func(s *Service) {
		s.opts.NpcEventEnabled = func(file string) bool {
			for _, f := range files {
				if f == file {
					return true
				}
			}
			return gamedata.NpcEventEnabled(file)
		}
	})
	if err := e.chars.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.chars.Stop)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", npc.Floor, 14, 6); err != nil {
		t.Fatal(err)
	}
	for _, f := range setup {
		f(e)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// questStateSetup 让角色的某个任务是给定的状态（setup 用，任务编号见 server/gamedata/quests.tsv）。
func questStateSetup(t *testing.T, quest, state int) func(*env) {
	return func(e *env) {
		e.exec(t, `INSERT INTO character_quests (character_id, quest_no, state)
			VALUES ((SELECT id FROM characters WHERE name = '勇者小明'), $1, $2)`, quest, state)
	}
}

// choose 选一个选项（编号用 strconv 拼进请求），返回推送。
func choose(c *client, no int) []map[string]any {
	_, pushes := c.callp("npc_choose", `{"no":`+strconv.Itoa(no)+`}`)
	return pushes
}

// closeWindow 关掉窗口（npc_close），返回推送。
func closeWindow(c *client) []map[string]any {
	_, pushes := c.callp("npc_close", `{}`)
	return pushes
}

// 问话分页（验收 1）：库伊老爷爷的“拿护身符”分 3 页，翻到最后一页选“是”才给护身符；
// 中途关窗、选“否”都什么都不发生；翻回上一页再看得到原来的内容。
func TestPagedAcceptFlow(t *testing.T) {
	npc := pagedFileNPC(t, pagedAmuletFile)
	e, c := pagedEnv(t, npc, []string{pagedAmuletFile},
		questStateSetup(t, 1, questDone), questStateSetup(t, 49, questDoing))

	// 第 1 页只有“下一页”；第 2 页多“上一页”；最后一页是“是、否、上一页”。
	expectWindow(t, talkTo(c, npc.ID), "好像有个叫新藏的年轻人", 1)
	expectWindow(t, choose(c, 1), "什么，有只卡卡金宝挡住洞口", 2)
	expectWindow(t, choose(c, 1), "咦，我刚刚是不是", 3)
	expectWindow(t, choose(c, 3), "什么，有只卡卡金宝挡住洞口", 2) // 上一页回看
	if pushes := closeWindow(c); !hasPush(pushes, "npc_window_closed") {
		t.Fatalf("关窗应推 npc_window_closed: %v", pushes)
	}
	if got := e.itemCounts(t); len(got) != 0 {
		t.Fatalf("中途关窗不应给东西，实际 %v", got)
	}

	// 翻到最后一页选“否”：窗口关掉，什么都不给。
	talkTo(c, npc.ID)
	choose(c, 1)
	choose(c, 1)
	if pushes := choose(c, 2); windowData(pushes) != nil {
		t.Fatalf("问话选否后不应再说话，实际 %v", pushes)
	}
	if got := e.itemCounts(t); len(got) != 0 {
		t.Fatalf("选否不应给东西，实际 %v", got)
	}

	// 翻到最后一页选“是”：交易成功，给护身符，之后是感谢话（只有一页，没有选项）。
	talkTo(c, npc.ID)
	choose(c, 1)
	choose(c, 1)
	expectWindow(t, choose(c, 1), "你可以试着拿给卡卡金宝看看", 0)
	if got := e.itemCounts(t); got[amuletItem] != 1 {
		t.Fatalf("选是后应得到 1 个护身符，实际 %v", got)
	}
}

// 感谢话分页（验收 1 的后半句，草药）：问话翻完选“是”，之后的感谢话也能一页页翻；翻页不改背包和进度，
// 最后一页只剩“上一页”，关窗后什么都不变。
func TestPagedThanksFlow(t *testing.T) {
	npc := pagedFileNPC(t, pagedHerbFile)
	e, c := pagedEnv(t, npc, []string{pagedHerbFile}, questStateSetup(t, 30, questDoing))

	expectWindow(t, talkTo(c, npc.ID), "哎呀，居然会有人来到这里呢", 1)
	expectWindow(t, choose(c, 1), "喔～～你是来和我拿草药的啊", 3)
	expectWindow(t, choose(c, 3), "哎呀，居然会有人来到这里呢", 1) // 上一页回到第 1 页，只剩“下一页”
	expectWindow(t, choose(c, 1), "喔～～你是来和我拿草药的啊", 3)

	// 选“是”：先交易（草药），再说第 1 页感谢话，带“下一页”。
	pushes := choose(c, 1)
	expectWindow(t, pushes, "原来如此，我知道了", 1)
	if !hasPush(pushes, "inventory_changed") {
		t.Fatalf("交易成功后应推 inventory_changed: %v", pushes)
	}
	if got := e.itemCounts(t); got[herbItem] != 1 {
		t.Fatalf("应得到 1 个草药，实际 %v", got)
	}

	// 感谢话 4 页：中间页是“下一页、上一页”，最后一页只有“上一页”，翻页不再给东西。
	expectWindow(t, choose(c, 1), "好啦，我这就把药交给你啦", 2)
	expectWindow(t, choose(c, 1), "这草药对心脏病的治疗效果特别好", 2)
	expectWindow(t, choose(c, 1), "对了，这药我做了不少", 1)
	expectWindow(t, choose(c, 1), "这草药对心脏病的治疗效果特别好", 2)
	if pushes := closeWindow(c); !hasPush(pushes, "npc_window_closed") {
		t.Fatalf("关窗应推 npc_window_closed: %v", pushes)
	}
	if got := e.itemCounts(t); got[herbItem] != 1 || len(got) != 1 {
		t.Fatalf("翻感谢话不应再给东西，实际 %v", got)
	}
}
