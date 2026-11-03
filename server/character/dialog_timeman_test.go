package character

// 报时 NPC 的台词跟着服务端时钟走（STAGE-69，docs/protocol.md 5.33）：talkText 取服务端当前时间，
// 洞窟守门人（1063，time=AFTER）在游戏时刻 126~300 说 main_msg，其余时候说 change_msg。

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
)

func TestTimemanTalkFollowsClock(t *testing.T) {
	// 912766409+5400 是 handletime.c 的纪元：纪元后 1055 秒是游戏时刻 200，264 秒是游戏时刻 50。
	epoch := int64(912766409 + 5400)
	s := &Service{opts: Options{NpcTalkRnd: func(n int) int { return 0 }}}
	talk := gamedata.NpcTalkOf(1063)
	npc := gamedata.NPC{ID: 1063}

	s.opts.Now = func() time.Time { return time.Unix(epoch+1055, 0) }
	if got := s.talkText(npc); got != talk.Lines[0] {
		t.Errorf("游戏时刻 200 在 AFTER 之内，应说 main_msg %q，实际 %q", talk.Lines[0], got)
	}
	s.opts.Now = func() time.Time { return time.Unix(epoch+264, 0) }
	if got := s.talkText(npc); got != talk.Change[0] {
		t.Errorf("游戏时刻 50 在 AFTER 之外，应说 change_msg %q，实际 %q", talk.Change[0], got)
	}
}
