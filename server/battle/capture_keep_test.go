package battle

import (
	"encoding/json"
	"strings"
	"testing"
)

func intp(v int) *int { return &v }

// TestParseCaptureKeep：null、没带、四个合法键通过；越界、类型不对、未知键回复 BAD_REQUEST。
func TestParseCaptureKeep(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		if k, perr := ParseCaptureKeep(json.RawMessage(raw)); k != nil || perr != nil {
			t.Errorf("%q 应等同不限，实际 %+v %v", raw, k, perr)
		}
	}
	k, perr := ParseCaptureKeep(json.RawMessage(`{"vital":-2,"strength":0,"toughness":-4,"dexterity":-1}`))
	if perr != nil || *k.Vital != -2 || *k.Strength != 0 || *k.Toughness != -4 || *k.Dexterity != -1 {
		t.Fatalf("合法 keep 解析错: %+v %v", k, perr)
	}
	if k, perr := ParseCaptureKeep(json.RawMessage(`{}`)); perr != nil || k == nil || k.Vital != nil {
		t.Errorf("空对象应是四项都不限: %+v %v", k, perr)
	}
	for _, raw := range []string{
		`{"vital":1}`, `{"vital":-5}`, `{"dexterity":-9}`, `{"vital":"-2"}`, `{"vital":1.5}`,
		`{"agility":-1}`, `[]`, `"x"`,
	} {
		if _, perr := ParseCaptureKeep(json.RawMessage(raw)); perr == nil || perr.Code != "BAD_REQUEST" {
			t.Errorf("%s 应回复 BAD_REQUEST，实际 %v", raw, perr)
		}
	}
}

// TestCaptureKeepJudge：AND、边界 −4 与 0、不限不筛、理由写第一个不达标的项。
func TestCaptureKeepJudge(t *testing.T) {
	q := func(v, s, to, d int) []Quality {
		return []Quality{{"vital", 0, v}, {"strength", 0, s}, {"toughness", 0, to}, {"dexterity", 0, d}}
	}
	cases := []struct {
		name   string
		keep   *CaptureKeep
		q      []Quality
		ok     bool
		reason string
	}{
		{"nil 接收者不筛", nil, q(-4, -4, -4, -4), true, ""},
		{"四项不限不筛，即使低于 -4", &CaptureKeep{}, q(-9, -9, -9, -9), true, ""},
		{"下限 -4 的边界保留", &CaptureKeep{Vital: intp(-4)}, q(-4, 0, 0, 0), true, ""},
		{"低于 -4 的丢弃", &CaptureKeep{Vital: intp(-4)}, q(-5, 0, 0, 0), false, "体力 −5 < −4"},
		{"0 只留满的", &CaptureKeep{Strength: intp(0)}, q(0, 0, 0, 0), true, ""},
		{"0 下的丢弃", &CaptureKeep{Strength: intp(0)}, q(0, -1, 0, 0), false, "腕力 −1 < 0"},
		{"没设的项不看", &CaptureKeep{Vital: intp(-2)}, q(-2, -4, -4, -4), true, ""},
		{"AND：体力合格、速度不合格", &CaptureKeep{Vital: intp(-2), Dexterity: intp(-1)}, q(-1, -4, -4, -2), false, "速度 −2 < −1"},
		{"AND：多项都不合格只报第一项", &CaptureKeep{Vital: intp(-2), Toughness: intp(-1)}, q(-3, -4, -2, -4), false, "体力 −3 < −2"},
		{"全部合格", &CaptureKeep{Vital: intp(-2), Strength: intp(-1), Toughness: intp(-4), Dexterity: intp(0)}, q(0, -1, -4, 0), true, ""},
	}
	for _, c := range cases {
		reason, ok := c.keep.Judge(c.q)
		if ok != c.ok || reason != c.reason {
			t.Errorf("%s：Judge = (%q, %v)，应为 (%q, %v)", c.name, reason, ok, c.reason, c.ok)
		}
	}
}

// TestCaptureKeepDiscardEvent：抓成功但被筛掉：ok 为真、没有 pet_slot、带 discarded、怪物离场但不进捕获名单。
func TestCaptureKeepDiscardEvent(t *testing.T) {
	b := newTestBattle(t, "capture", chanceRnd()) // 捕捉必成功
	b.PlayerUnit().Attack = 5                     // 先打残，见 TestBattleCapture
	keep := &CaptureKeep{Vital: intp(-2)}
	var got *CaptureKeep
	b.Cbs.PlaceCaptured = func(m *Unit, k *CaptureKeep) (int, *Discarded) {
		got = k
		return -1, &Discarded{Name: m.Name, Level: m.Level, Reason: "体力 −3 < −2"}
	}
	for turn := 1; turn <= 2; turn++ {
		b.Submit(turn, "player", "attack", Cmd{Target: 15})
		b.Submit(turn, "pet", "guard", Cmd{})
		b.Settle()
	}
	b.Submit(3, "player", "capture", Cmd{Target: 15, Keep: keep})
	b.Submit(3, "pet", "guard", Cmd{})
	round, _ := b.Settle()
	var cap *Event
	for i, e := range round.Events {
		if e.Kind == "capture" {
			cap = &round.Events[i]
		}
	}
	if cap == nil || cap.OK == nil || !*cap.OK {
		t.Fatalf("应有一条抓成功的 capture 事件: %+v", cap)
	}
	if got != keep {
		t.Errorf("回调应收到这次的 keep，实际 %+v", got)
	}
	if cap.Discarded == nil || cap.Discarded.Reason != "体力 −3 < −2" {
		t.Fatalf("应带 discarded: %+v", cap.Discarded)
	}
	if len(b.Captured()) != 0 {
		t.Errorf("被筛掉的不进捕获名单，实际 %d 只", len(b.Captured()))
	}
	if len(cap.Left) != 1 || cap.Left[0] != 15 {
		t.Errorf("怪物应离场（left 含 15），实际 %v", cap.Left)
	}
	raw, _ := json.Marshal(cap)
	if s := string(raw); strings.Contains(s, `"pet_slot"`) || !strings.Contains(s, `"discarded"`) {
		t.Errorf("事件 JSON 应有 discarded、没有 pet_slot: %s", s)
	}
}
