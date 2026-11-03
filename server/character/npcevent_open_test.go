package character

// STAGE-214：配置开关 STONEAGE_NPC_EVENT_OPEN=all（这里用 gamedata.SetNpcEventOpen 模拟）放开整本支持的任务脚本，
// 对话从“还不能对话”变成正常说话；设回默认后又被拒。Options.NpcEventEnabled 保持零值，走 gamedata 的判断。

import (
	"context"
	"strconv"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

func TestNpcEventOpenSwitchTalk(t *testing.T) {
	gamedata.SetNpcEventOpen("")
	t.Cleanup(func() { gamedata.SetNpcEventOpen("") })
	// 现找一个整本都支持、但不在白名单里的脚本（写死脚本会被后续需求放开，seimu/event/event02_1 就这样）。
	file := openSwitchFile(t)
	npc := pagedFileNPC(t, file)
	_, c := openSwitchEnv(t, npc)
	talk := `{"npc":` + strconv.Itoa(npc.ID) + `}`

	c.wantErr(t, "talk", talk, "NPC_NOT_TALKABLE")
	gamedata.SetNpcEventOpen("all")
	if m, pushes := c.callp("talk", talk); m["ok"] != true || windowData(pushes) == nil {
		t.Fatalf("开关放开后应能对话: %v %v", m, pushes)
	}
	closeWindow(c)
	gamedata.SetNpcEventOpen("")
	c.wantErr(t, "talk", talk, "NPC_NOT_TALKABLE")
}

// openSwitchFile 找一个整本支持、不在白名单里、有 NPC 的脚本（按 NPC 编号顺序取第一个，结果稳定）。
func openSwitchFile(t *testing.T) string {
	t.Helper()
	for _, s := range gamedata.NpcEventScripts() {
		if len(s.Blocks) == 0 || gamedata.NpcEventEnabled(s.File) {
			continue
		}
		full := true
		for i := range s.Blocks {
			if s.Blocks[i].Unsupported != "" {
				full = false
				break
			}
		}
		if !full {
			continue
		}
		if _, ok := gamedata.NPCByID(s.ID); ok {
			return s.File
		}
	}
	t.Fatal("找不到整本支持、还没放开的脚本")
	return ""
}

// openSwitchEnv 准备勇者小明站在脚本 NPC 所在的地图上，进入世界（同 pagedEnv，但不换白名单钩子）。
func openSwitchEnv(t *testing.T, npc gamedata.NPC) (*env, *client) {
	t.Helper()
	e := newEnv(t)
	if err := e.chars.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.chars.Stop)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	if err := MoveCharacter(context.Background(), e.chars.db, "勇者小明", npc.Floor, 14, 6); err != nil {
		t.Fatal(err)
	}
	c.ok("enter_world", `{"slot":0}`)
	return e, c
}
