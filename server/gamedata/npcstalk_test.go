package gamedata

// 抽查 npc-talk.json（STAGE-69，docs/protocol.md 5.33）：村民按逗号分句、告示牌保留
// 字面 \n、报时带 time/main_msg/change_msg；摆设没有记录（运行时回「……」）。

import (
	"strings"
	"testing"
	"time"
)

func TestVillagerTalkLines(t *testing.T) {
	// 村长家的厨师（225）：原版参数是逗号分开的多句，talk 时随机取一句。
	npc, ok := NPCByID(225)
	if !ok || npc.Name != "村长家的厨师" || npc.Kind != "npcgen_man" {
		t.Fatalf("编号 225 应是村长家的厨师（npcgen_man），实际 %+v", npc)
	}
	talk := NpcTalkOf(225)
	if talk == nil || len(talk.Lines) == 0 {
		t.Fatalf("村民 225 应有对话参数: %+v", talk)
	}
	for _, line := range talk.Lines {
		if strings.Contains(line, ",") {
			t.Errorf("对话参数应已按逗号分句，这句里还有逗号: %q", line)
		}
		if strings.TrimSpace(line) == "" {
			t.Errorf("对话参数里不应有空句: %+v", talk.Lines)
		}
	}
	// 随机源取到的一定是其中一句。
	got := talk.TalkText(func(n int) int { return len(talk.Lines) - 1 })
	found := false
	for _, line := range talk.Lines {
		if line == got {
			found = true
		}
	}
	if !found {
		t.Errorf("TalkText 应返回参数里的原句，实际 %q（候选 %v）", got, talk.Lines)
	}
}

func TestSignboardTalkText(t *testing.T) {
	// 竞技场前的看板（194）：原文保存，`\n` 是字面两个字符，TalkText 才转成换行。
	talk := NpcTalkOf(194)
	if talk == nil || talk.Text == "" {
		t.Fatalf("告示牌 194 应有文字参数: %+v", talk)
	}
	if !strings.Contains(talk.Text, `\n`) {
		t.Errorf("告示牌原文应保留字面 \\n（原版 signb 文件的样子），实际 %q", talk.Text)
	}
	if !strings.Contains(talk.TalkText(nil), "\n") {
		t.Errorf("TalkText 应把字面 \\n 转成真换行: %q", talk.Text)
	}
}

func TestTimeManTalk(t *testing.T) {
	// 洞窟守门人（1063，STAGE-69 新收录的报时 NPC）：time=AFTER，main_msg 和 change_msg 各一句。
	npc, ok := NPCByID(1063)
	if !ok || npc.Kind != "npcgen_timeman" || npc.Type != "报时" {
		t.Fatalf("编号 1063 应是报时 NPC，实际 %+v", npc)
	}
	talk := NpcTalkOf(1063)
	if talk == nil || talk.Time != "AFTER" || len(talk.Lines) != 1 || len(talk.Change) != 1 {
		t.Fatalf("洞窟守门人应有 time=AFTER 和两句参数: %+v", talk)
	}
	if got := talk.TalkText(nil); got != talk.Lines[0] {
		t.Errorf("静态 TalkText 应保留 main_msg 优先口径 %q，实际 %q", talk.Lines[0], got)
	}
}

func TestMugonAndMissingTalkSayDefault(t *testing.T) {
	// 摆设（1062 乌力，npcgen_mugon）没有参数记录；查不到参数的 NPC 都回「……」。
	npc, ok := NPCByID(1062)
	if !ok || npc.Kind != "npcgen_mugon" {
		t.Fatalf("编号 1062 应是摆设，实际 %+v", npc)
	}
	if talk := NpcTalkOf(1062); talk != nil {
		t.Errorf("摆设不应有对话参数记录，实际 %+v", talk)
	}
	if got := (*NpcTalk)(nil).TalkText(nil); got != MugonLine {
		t.Errorf("没有参数的 NPC 应回「%s」，实际 %q", MugonLine, got)
	}
	if TalkBroken() {
		t.Error("npc-talk.json 应能正常加载")
	}
}

// gameTimeAt 返回游戏时刻为 hour 的现实时间：纪元同 handletime.c，取这个时刻的第一秒。
func gameTimeAt(t *testing.T, hour int) time.Time {
	t.Helper()
	sec := (hour*5400 + 1023) / 1024 // 向上取整，保证这一秒正好落在第 hour 个时刻里
	now := time.Unix(912766409+5400+int64(sec), 0)
	if got := lsHour(now); got != hour {
		t.Fatalf("游戏时刻 %d 换算后是 %d", hour, got)
	}
	return now
}

func TestLSHourFollowsHandletime(t *testing.T) {
	// handletime.c 的 RealTimeToLSTime：纪元起每 5400 秒一天，一天分 1024 个时刻。
	era := int64(912766409 + 5400)
	for _, c := range []struct {
		sec  int64
		hour int
	}{{0, 0}, {2700, 512}, {5399, 1023}, {5400, 0}, {8100, 512}} {
		if got := lsHour(time.Unix(era+c.sec, 0)); got != c.hour {
			t.Errorf("纪元后 %d 秒应是游戏时刻 %d，实际 %d", c.sec, c.hour, got)
		}
	}
}

func TestTimemanActiveBoundaries(t *testing.T) {
	// npc_timeman.c 用严格不等号：时段两端的时刻本身不在时段内；跨午夜的时段从两头算。
	for _, c := range []struct {
		period string
		hour   int
		want   bool
	}{
		{"AFTER", 125, false}, {"AFTER", 126, false}, {"AFTER", 127, true}, {"AFTER", 299, true}, {"AFTER", 300, false},
		{"ALLNOON", 0, false}, {"ALLNOON", 1, true}, {"ALLNOON", 299, true}, {"ALLNOON", 300, false},
		{"ALLNOON", 701, false}, {"ALLNOON", 702, true}, {"ALLNOON", 1023, true},
		{"AM", 124, true}, {"AM", 125, false}, {"AM", 501, false}, {"AM", 502, true},
		{"FREE", 0, false}, {"FREE", 1, true},
		{"XYZ", 200, true}, // 未识别的时段沿用 main_msg，不把对话吞掉
	} {
		if got := timemanActive(c.period, c.hour); got != c.want {
			t.Errorf("时段 %s 在游戏时刻 %d：want %v, got %v", c.period, c.hour, c.want, got)
		}
	}
}

func TestTalkTextAtFollowsTimeman(t *testing.T) {
	// 洞窟守门人（1063，time=AFTER）：游戏时刻 126~300 说 main_msg，其余时候说 change_msg。
	talk := NpcTalkOf(1063)
	rnd := func(n int) int { return 0 }
	if got := talk.TalkTextAt(gameTimeAt(t, 200), rnd); got != talk.Lines[0] {
		t.Errorf("时段内应说 main_msg %q，实际 %q", talk.Lines[0], got)
	}
	if got := talk.TalkTextAt(gameTimeAt(t, 50), rnd); got != talk.Change[0] {
		t.Errorf("时段外应说 change_msg %q，实际 %q", talk.Change[0], got)
	}
}

func TestTalkTextAtFallbacks(t *testing.T) {
	day, night := gameTimeAt(t, 200), gameTimeAt(t, 50) // 200 在 AFTER 之内，50 在之外
	rnd := func(n int) int { return 0 }
	both := &NpcTalk{Lines: []string{"main"}, Time: "AFTER", Change: []string{"change"}}
	onlyMain := &NpcTalk{Lines: []string{"main"}, Time: "AFTER"}
	onlyChange := &NpcTalk{Time: "AFTER", Change: []string{"change"}}
	noTime := &NpcTalk{Lines: []string{"main"}, Change: []string{"change"}}
	unknown := &NpcTalk{Lines: []string{"main"}, Time: "XYZ", Change: []string{"change"}}
	sign := &NpcTalk{Text: `第一行\n第二行`, Time: "AFTER", Change: []string{"change"}}
	for _, c := range []struct {
		name string
		talk *NpcTalk
		now  time.Time
		want string
	}{
		{"时段内说 main_msg", both, day, "main"},
		{"时段外说 change_msg", both, night, "change"},
		{"只有 main_msg 时全天说它", onlyMain, night, "main"},
		{"只有 change_msg 时全天说它", onlyChange, day, "change"},
		{"没有时段时不切换", noTime, night, "main"},
		{"未识别时段沿用 main_msg", unknown, night, "main"},
		{"告示牌不受时段影响", sign, night, "第一行\n第二行"},
		{"没有参数回默认话", nil, day, MugonLine},
	} {
		if got := c.talk.TalkTextAt(c.now, rnd); got != c.want {
			t.Errorf("%s：want %q, got %q", c.name, c.want, got)
		}
	}
}
