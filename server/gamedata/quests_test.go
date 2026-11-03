package gamedata

import "testing"

// 任务表来自 jobdaily.txt：109 个原版任务（第 61 号事件号为 0，跳过）加 STAGE-333 的自定义任务 111，共 110 个，按编号升序，名字没有连续空格。
func TestQuestTable(t *testing.T) {
	qs := Quests()
	if len(qs) != 110 {
		t.Fatalf("任务表应有 110 个任务，实际 %d", len(qs))
	}
	for i := 1; i < len(qs); i++ {
		if qs[i].No <= qs[i-1].No {
			t.Fatalf("任务应按编号升序：%d 在 %d 后面", qs[i].No, qs[i-1].No)
		}
	}
	if q, ok := QuestByNo(1); !ok || q.Name != "成人仪式" || q.EndEvent != 4 || q.NowEvent != 4 {
		t.Errorf("任务 1 不对：%+v %v", q, ok)
	}
	if q, ok := QuestByNo(3); !ok || q.Name != "猜谜3兄弟 拿蓝暴" {
		t.Errorf("任务 3 的名字应压成一个空格：%+v %v", q, ok)
	}
	if _, ok := QuestByNo(61); ok {
		t.Errorf("任务 61 的事件号为 0，不应收录")
	}
	if _, ok := QuestByNo(0); ok {
		t.Errorf("任务 0 不存在")
	}
}
