package character

import (
	"github.com/athlan20/stoneage/server/gamedata"
	"testing"
)

func TestQuestNPCRelationsComeFromActualScripts(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) { r.Quests = []questRec{{No: 1, State: questDoing}} })
	m := c.call("quest_npcs", `{"no":1}`)
	if m["ok"] != true {
		t.Fatal(m)
	}
	data := m["data"].(map[string]any)
	if data["state"] != "doing" {
		t.Fatal(data)
	}
	q, _ := gamedata.QuestByNo(1)
	if len(data["npcs"].([]any)) == 0 { t.Fatal("成人仪式应能从正式脚本得到 NPC 关系") }
	for _, v := range data["npcs"].([]any) {
		n := v.(map[string]any)
		ev, ok := gamedata.NpcEventOf(int(n["id"].(float64)))
		if !ok || !e.chars.npcEventOpen(ev.File) {
			t.Fatalf("未知或未开放脚本: %v", n)
		}
		found := false
		for _, b := range ev.Blocks {
			if questBlockMentions(b, q) {
				found = true
			}
		}
		if !found {
			t.Fatalf("关系无依据: %v", n)
		}
	}
	c.wantErr(t, "quest_npcs", `{"no":999999}`, "BAD_REQUEST")
}

func TestQuestBlockDoesNotInventReference(t *testing.T) {
	q := gamedata.Quest{NowEvent: 4, EndEvent: 4}
	if questBlockMentions(gamedata.EventBlock{Event: 9}, q) {
		t.Fatal("无关事件")
	}
	if !questBlockMentions(gamedata.EventBlock{Cond: [][]gamedata.EventTerm{{{Op: "NOWEV", N: 4}}}}, q) {
		t.Fatal("漏掉真实条件引用")
	}
}
