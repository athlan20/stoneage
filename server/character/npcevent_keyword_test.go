package character

// 任务 NPC 的暗号（STAGE-92 验收）：全岛巡回活动第一站（jaruga/event/oev_1）的检查员。测试里把这一个脚本放进
// 白名单（Options.NpcEventEnabled，正式白名单要等 STAGE-94 才放开），其余行为照常。编号从 gamedata 里按脚本文件找。

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const oevOneFile = "jaruga/event/oev_1" // 第一检查站（A）：暗号「近藏」，持有第一站证明石（2552）时收走它、给第二站的证明石（2553）

// oevOneNPC 找到第一检查站的 NPC（编号和所在地图）。
func oevOneNPC(t *testing.T) gamedata.NPC {
	t.Helper()
	for id := 1; id <= 5000; id++ {
		ev, ok := gamedata.NpcEventOf(id)
		if !ok || ev.File != oevOneFile {
			continue
		}
		if npc, ok := gamedata.NPCByID(id); ok {
			return npc
		}
	}
	t.Fatalf("找不到脚本 %s 的 NPC", oevOneFile)
	return gamedata.NPC{}
}

// newKeywordEnv 同 newPointsEnv，另外把第一检查站的脚本放进白名单。
func newKeywordEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, func(s *Service) {
		s.opts.NpcEventEnabled = func(file string) bool {
			return file == oevOneFile || gamedata.NpcEventEnabled(file)
		}
	})
	if err := e.chars.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.chars.Stop)
	return e
}

// keywordEnv 准备勇者小明站在检查站所在的地图上（进入世界之前写数据库），背包里按 items 放道具（道具编号 → 数量）。
func keywordEnv(t *testing.T, npc gamedata.NPC, items map[int]int) (*env, *client) {
	t.Helper()
	e := newKeywordEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", npc.Floor, 14, 6); err != nil {
		t.Fatal(err)
	}
	slot := 0
	for id, n := range items {
		putItemSetup(t, firstBagSlot+slot, id, n)(e)
		slot++
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}

// sayKeyword 对窗口说一句话（npc_reply），返回推送。
func sayKeyword(c *client, text string) []map[string]any {
	_, pushes := c.callp("npc_reply", `{"text":`+strconv.Quote(text)+`}`)
	return pushes
}

// 说暗号的完整流程（验收 1、2）：闲聊窗口上有“说暗号”；选了以后是文字输入窗口（accepts_text）；
// 说错一个字（大小写、空格不同）只说闲聊句、窗口上仍有“说暗号”，什么都不改；说对了收走证明石、给下一站的证明石。
func TestKeywordFullFlow(t *testing.T) {
	npc := oevOneNPC(t)
	e, c := keywordEnv(t, npc, map[int]int{2552: 1})

	pushes := talkTo(c, npc.ID)
	d := windowData(pushes)
	if d == nil || len(d["options"].([]any)) != 1 || d["accepts_text"] != nil {
		t.Fatalf("闲聊窗口应只有“说暗号”一个选项，且不接受文字: %v", pushes)
	}
	if opt := d["options"].([]any)[0].(map[string]any); opt["label"] != "说暗号" || opt["no"] != float64(1) {
		t.Fatalf("选项应是 1 说暗号: %v", opt)
	}

	_, pushes = c.callp("npc_choose", `{"no":1}`)
	if !hasPush(pushes, "npc_window_closed") {
		t.Fatalf("选“说暗号”应先关掉闲聊窗口: %v", pushes)
	}
	if d := windowData(pushes); d == nil || d["accepts_text"] != true || len(d["options"].([]any)) != 0 {
		t.Fatalf("选“说暗号”后应推文字输入窗口（accepts_text，没有选项）: %v", pushes)
	}

	for _, wrong := range []string{"近藏 ", " 近藏", "近藏。", "近蔵", "近藏　"} {
		pushes = sayKeyword(c, wrong)
		if d := windowData(pushes); d == nil || len(d["options"].([]any)) != 1 {
			t.Fatalf("说错了（%q）应回到闲聊窗口（带“说暗号”）: %v", wrong, pushes)
		}
		if hasPush(pushes, "inventory_changed") {
			t.Fatalf("说错了（%q）不应该改背包: %v", wrong, pushes)
		}
		if got := e.itemCounts(t); got[2552] != 1 || got[2553] != 0 {
			t.Fatalf("说错了（%q）背包不该变: %v", wrong, got)
		}
		_, pushes = c.callp("npc_choose", `{"no":1}`)
		if !hasPush(pushes, "npc_window") {
			t.Fatalf("再选“说暗号”应开文字输入窗口: %v", pushes)
		}
	}

	// 说对了：先关窗口，再交易（收走 2552、给 2553），最后说 NomalWindowMsg。
	pushes = sayKeyword(c, "近藏")
	if !hasPush(pushes, "npc_window_closed") || !hasPush(pushes, "inventory_changed") {
		t.Fatalf("说对了应关窗口并推背包变化: %v", pushes)
	}
	// 暗号块没有事件号，永远不算完成，窗口上仍有“说暗号”（再说一次不会有块命中，只说闲聊句）。
	expectWindow(t, pushes, "\n正确答案", 1)
	if got := e.itemCounts(t); got[2552] != 0 || got[2553] != 1 {
		t.Fatalf("说对了应收走 2552、给 2553，实际 %v", got)
	}
}

// 没有证明石（验收 1）：说对了暗号也没有块命中，只说闲聊句；背包不变，窗口上仍有“说暗号”。
func TestKeywordWithoutStoneGetsNothing(t *testing.T) {
	npc := oevOneNPC(t)
	e, c := keywordEnv(t, npc, nil)

	talkTo(c, npc.ID)
	c.callp("npc_choose", `{"no":1}`)
	pushes := sayKeyword(c, "近藏")
	if hasPush(pushes, "inventory_changed") {
		t.Fatalf("没有证明石说对暗号也不该给东西: %v", pushes)
	}
	if d := windowData(pushes); d == nil || len(d["options"].([]any)) != 1 {
		t.Fatalf("没有块命中应说闲聊句，窗口上仍有“说暗号”: %v", pushes)
	}
	if got := e.itemCounts(t); len(got) != 0 {
		t.Fatalf("背包应该还是空的，实际 %v", got)
	}
}

// 选项和窗口模式的边界：“说暗号”只有 1 号；文字输入窗口上选项回 NPC_BAD_CHOICE（窗口保持打开）。
func TestKeywordChoiceValidation(t *testing.T) {
	npc := oevOneNPC(t)
	_, c := keywordEnv(t, npc, map[int]int{2552: 1})

	talkTo(c, npc.ID)
	c.wantErr(t, "npc_choose", `{"no":2}`, "NPC_BAD_CHOICE")
	c.callp("npc_choose", `{"no":1}`)
	c.wantErr(t, "npc_choose", `{"no":1}`, "NPC_BAD_CHOICE")
	if m, _ := c.callp("npc_reply", `{"text":"近藏"}`); m["ok"] != true {
		t.Fatalf("文字输入窗口应接受暗号: %v", m)
	}
}

// 空白的回复按普通的文字回复处理：整段都是空白时 BAD_REQUEST（不会去匹配）。
func TestKeywordBlankReplyRejected(t *testing.T) {
	npc := oevOneNPC(t)
	_, c := keywordEnv(t, npc, map[int]int{2552: 1})

	talkTo(c, npc.ID)
	c.callp("npc_choose", `{"no":1}`)
	c.wantErr(t, "npc_reply", `{"text":"   "}`, "BAD_REQUEST")
}
