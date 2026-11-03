package character

// 真人队的练级传送开战（STAGE-275 第三轮 D1）：队长 leveling_start 开战时队员随队长移动，并和队长同场；
// 开关关闭时回退单人；队员自己发 leveling_start 仍被 PARTY_FOLLOWER 拒绝。

import (
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"
)

const levelingAreaID = "200-189-521-435-527-454" // 练级目录里的区域（协议用例 leveling_start.jsonl 用的）

// noPushOf 在 d 之内读这条连接的推送，出现 typ 就失败（其它推送忽略）。
func noPushOf(t *testing.T, c *client, typ string, d time.Duration) {
	t.Helper()
	for _, m := range c.pending {
		if m["type"] == typ {
			t.Fatalf("不应收到 %s: %v", typ, m)
		}
	}
	c.pending = nil
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()
	for {
		var m map[string]any
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			return
		}
		_ = json.Unmarshal(line, &m)
		if m["type"] == typ {
			t.Fatalf("不应收到 %s: %v", typ, m)
		}
	}
}

func TestPartyLevelingStartOnceFightsTogether(t *testing.T) {
	_, a, b, _ := realPartyEnvWith(t, rand.New(rand.NewPCG(21, 43)).IntN, time.Hour, nil)
	a.roundtrip(t, "leveling_start", `{"area_id":"`+levelingAreaID+`","mode":"once"}`, "")
	if start := a.drainTo(t, "battle_start"); start["kind"] != "party" || start["you"] != float64(0) {
		t.Fatalf("队长应收到 party 的 battle_start（you=0）: %v", start)
	}
	if mate := b.drainTo(t, "battle_start"); mate["kind"] != "party" || mate["you"] != float64(1) {
		t.Fatalf("队员应随队长同场（party，you=1）: %v", mate)
	}
}

func TestPartyLevelingStartSwitchOffFightsAlone(t *testing.T) {
	e, a, b, _ := realPartyEnvWith(t, rand.New(rand.NewPCG(21, 43)).IntN, time.Hour, nil)
	e.chars.partyPlayersOn.Store(false)
	a.roundtrip(t, "leveling_start", `{"area_id":"`+levelingAreaID+`","mode":"once"}`, "")
	if start := a.drainTo(t, "battle_start"); start["kind"] != "wild" {
		t.Fatalf("开关关闭时应是单人野外战斗: %v", start)
	}
	noPushOf(t, b, "battle_start", 300*time.Millisecond)
}

func TestPartyFollowerLevelingStartRejected(t *testing.T) {
	_, _, b, _ := realPartyEnvWith(t, rand.New(rand.NewPCG(21, 43)).IntN, time.Hour, nil)
	b.expectError(t, "leveling_start", `{"area_id":"`+levelingAreaID+`","mode":"once"}`, "PARTY_FOLLOWER")
}
