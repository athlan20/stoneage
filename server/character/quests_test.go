package character

import (
	"context"
	"encoding/json"
	"testing"
)

// questsView 进行中在前，同一状态内按编号；不在任务表里的编号不列出。
func TestQuestsViewDoingFirstThenByNo(t *testing.T) {
	r := &record{Quests: []questRec{
		{No: 1, State: questDone},
		{No: 2, State: questDoing},
		{No: 3, State: questDone},
		{No: 4, State: questDoing},
		{No: 999, State: questDoing}, // 任务表里没有
	}}
	got := r.questsView()
	want := []questView{
		{No: 2, Name: "猜谜3兄弟", State: "doing"},
		{No: 4, Name: "卡坦 找4只鲨鱼", State: "doing"},
		{No: 1, Name: "成人仪式", State: "done"},
		{No: 3, Name: "猜谜3兄弟 拿蓝暴", State: "done"},
	}
	if len(got) != len(want) {
		t.Fatalf("任务条数不对：%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 条：want %+v，got %+v", i, want[i], got[i])
		}
	}
}

// 没有任何任务时 quests 是 []，不是 null（客户端按数组读）。
func TestQuestsViewEmptyEncodesAsArray(t *testing.T) {
	b, err := json.Marshal(map[string]any{"quests": (&record{}).questsView()})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"quests":[]}` {
		t.Fatalf("空任务列表应编码成 []，实际 %s", b)
	}
}

// changedQuests：状态变了、新增、清除的编号都算变化，没变的不算，按编号升序。
func TestChangedQuests(t *testing.T) {
	before := []questRec{{No: 1, State: questDoing}, {No: 2, State: questDone}, {No: 5, State: questDoing}}
	after := []questRec{{No: 1, State: questDone}, {No: 2, State: questDone}, {No: 3, State: questDoing}}
	got := changedQuests(before, after)
	want := []int{1, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("变化的编号 want %v，got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("变化的编号 want %v，got %v", want, got)
		}
	}
	if none := changedQuests(after, after); len(none) != 0 {
		t.Fatalf("没有变化时应为空，实际 %v", none)
	}
}

// 管理命令设进度，在线的角色立刻收到 quests_changed：全量列表加变化的编号。
func TestSetQuestReachesOnlinePlayer(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.setQuest("勇者小明", 1, "doing")
	push := c.recv()
	if push["type"] != "quests_changed" {
		t.Fatalf("应收到 quests_changed，实际 %v", push)
	}
	d := push["data"].(map[string]any)
	quests := d["quests"].([]any)
	if len(quests) != 1 || quests[0].(map[string]any)["state"] != "doing" || quests[0].(map[string]any)["name"] != "成人仪式" {
		t.Fatalf("推送的任务列表不对：%v", quests)
	}
	if changed := d["changed"].([]any); len(changed) != 1 || changed[0].(float64) != 1 {
		t.Fatalf("changed 应是 [1]，实际 %v", changed)
	}
}

// 同一个状态再设一次不推送；clear 后任务从列表消失，changed 里仍有这个编号。
func TestSetQuestSameStateNoPushAndClear(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	e.setQuest("勇者小明", 2, "done")
	c.recv()
	e.setQuest("勇者小明", 2, "done")
	c.noMessage()
	e.setQuest("勇者小明", 2, "clear")
	d := c.recv()["data"].(map[string]any)
	if quests := d["quests"].([]any); len(quests) != 0 {
		t.Fatalf("清除后列表应为空，实际 %v", quests)
	}
	if changed := d["changed"].([]any); len(changed) != 1 || changed[0].(float64) != 2 {
		t.Fatalf("清除的编号应在 changed 里，实际 %v", changed)
	}
}

// 离线时设的进度只写库，下次进入世界时可见；进行中排在已完成前面。
func TestSetQuestOfflineShowsOnEnter(t *testing.T) {
	e := newPointsEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	e.setQuest("勇者小明", 1, "done")
	e.setQuest("勇者小明", 3, "doing")
	c.ok("enter_world", `{"slot":0}`)
	got := c.ok("quests", `{}`)["quests"].([]any)
	if len(got) != 2 || got[0].(map[string]any)["no"].(float64) != 3 || got[1].(map[string]any)["no"].(float64) != 1 {
		t.Fatalf("进入世界后应先列进行中的 3、再列已完成的 1，实际 %v", got)
	}
}

// setQuest 走管理命令的同一条路径（写库并 NOTIFY）。
func (e *env) setQuest(name string, no int, state string) {
	e.t.Helper()
	if err := SetQuest(context.Background(), e.chars.db, DefaultQuestNotifyChannel, name, no, state); err != nil {
		e.t.Fatal(err)
	}
}
