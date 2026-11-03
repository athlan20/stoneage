package casetest_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/casetest"
)

// STAGE-36：战斗协议用例（docs/protocol/cases/battle_*.jsonl）的格式检查。
// 这里不跑服务端，只按 docs/protocol.md 第 10 章检查用例本身：单位号、事件种类、状态取值、
// 错误码是否都在文档里、己方/对方单位的血量字段是否符合“敌人血量隐藏”的规则。
// 服务端战斗引擎（STAGE-41）实现后，这些用例由 TestProtocolCases 真正重放。

var (
	unitKinds   = set("player", "pet", "monster")
	statusNames = set("poison", "paralysis", "sleep", "stone", "drunk", "confusion", "weaken", "barrier", "nocast", "magic_resist", "fire_resist", "thunder_resist", "ice_resist", "guard", "buff", "debuff", "reversed", "absorb", "reflect", "vanish")
	eventKinds  = set("attack", "combo", "counter", "guard", "magic", "item", "skill", "capture", "escape", "swap_pet", "skip", "status_tick", "disobey", "pet_escape", "field")
	resultNames = set("hit", "crit", "miss", "dodge", "block", "heal", "none", "absorb", "reflect", "vanish")
	endResults  = set("win", "lose", "escaped", "draw")
)

func set(v ...string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		m[s] = true
	}
	return m
}

type obj = map[string]any

func get(m obj, key string) obj    { v, _ := m[key].(obj); return v }
func str(m obj, key string) string { v, _ := m[key].(string); return v }
func list(m obj, key string) []any { v, _ := m[key].([]any); return v }

// number 取数值；样例写成 "*=9" 的通配当作 0 看待（只检查字段存在）。
func number(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case string:
		return 0, strings.HasPrefix(x, "*")
	}
	return 0, false
}

func TestBattleCasesFollowProtocol(t *testing.T) {
	cases, err := casetest.LoadDir(casesDir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile("../../docs/protocol.md")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, c := range cases {
		if !strings.HasPrefix(c.Name, "battle_") {
			continue
		}
		seen++
		t.Run(c.Name, func(t *testing.T) {
			v := &battleChecker{t: t, doc: string(doc), units: map[int]bool{}}
			for _, s := range c.Steps {
				if s.Dir == "admin" || len(s.Msg) == 0 {
					continue
				}
				var m obj
				if err := json.Unmarshal(s.Msg, &m); err != nil {
					t.Fatalf("第 %d 行: %v", s.Line, err)
				}
				v.line = s.Line
				if s.Dir == "s2c" {
					v.reply(m)
				}
			}
		})
	}
	if seen < 6 {
		t.Fatalf("只找到 %d 个 battle_ 用例，应当覆盖胜利、失败、逃跑、捕获、事件展示、出招校验、agent", seen)
	}
}

type battleChecker struct {
	t     *testing.T
	doc   string
	line  int
	you   int
	units map[int]bool // 场上出现过的单位号（开局的 units 加上 joined）
	turn  int
}

func (v *battleChecker) fail(format string, args ...any) {
	v.t.Errorf("第 %d 行: %s", v.line, fmt.Sprintf(format, args...))
}

func (v *battleChecker) reply(m obj) {
	if e := get(m, "error"); e != nil {
		code := str(e, "code")
		if !strings.Contains(v.doc, "`"+code+"`") {
			v.fail("错误码 %s 没有写在 docs/protocol.md 里", code)
		}
		return
	}
	d := get(m, "data")
	switch str(m, "type") {
	case "battle_start":
		v.start(d)
	case "battle_turn":
		turn, _ := number(d["turn"])
		if turn < 1 {
			v.fail("turn 从 1 开始")
		}
		needs := list(d, "needs")
		if len(needs) > 2 {
			v.fail("needs 最多 player、pet 两项")
		}
		for i, n := range needs {
			if want := []string{"player", "pet"}[i]; len(needs) == 2 && n != want {
				v.fail("needs 的顺序必须是 player、pet，实际 %v", needs)
			}
		}
	case "battle_round":
		v.round(d)
	case "battle_end":
		if r := str(d, "result"); !endResults[r] {
			v.fail("battle_end.result %q 不在文档里", r)
		}
	}
	if b := get(d, "battle"); b != nil { // enter_world 里的快照
		v.units = map[int]bool{}
		v.addUnits(list(b, "units"))
	}
	if str(m, "type") == "battle" && d["in_battle"] == true {
		v.addUnits(list(d, "units"))
	}
}

func (v *battleChecker) start(d obj) {
	you, ok := number(d["you"])
	if !ok || you < 0 || you > 19 {
		v.fail("you 必须是 0–19 的单位号")
	}
	v.you = you
	v.units = map[int]bool{}
	v.addUnits(list(d, "units"))
	if !v.units[you] {
		v.fail("you=%d 不在 units 里", you)
	}
}

func (v *battleChecker) addUnits(units []any) {
	for _, u := range units {
		um, _ := u.(obj)
		no, ok := number(um["no"])
		if !ok || no < 0 || no > 19 {
			v.fail("单位号必须是 0–19: %v", um["no"])
			continue
		}
		v.units[no] = true
		if k := str(um, "kind"); !unitKinds[k] {
			v.fail("单位 %d 的 kind %q 不在文档里", no, k)
		}
		for _, s := range list(um, "status") {
			if name, _ := s.(string); !statusNames[name] {
				v.fail("单位 %d 的状态 %q 不在文档里", no, s)
			}
		}
		if _, ok := um["hp_pct"]; !ok {
			v.fail("单位 %d 缺少 hp_pct", no)
		}
		mine := no/10 == v.you/10
		_, hasHP := um["hp"]
		if mine != hasHP {
			v.fail("单位 %d：己方单位才有 hp/max_hp（己方=%v，有 hp=%v）", no, mine, hasHP)
		}
	}
}

func (v *battleChecker) round(d obj) {
	known := func(what string, v2 any) {
		no, ok := number(v2)
		if !ok || !v.units[no] {
			v.fail("%s 指向的单位 %v 没有出现过", what, v2)
		}
	}
	for _, e := range list(d, "events") {
		ev, _ := e.(obj)
		kind := str(ev, "kind")
		if !eventKinds[kind] {
			v.fail("事件种类 %q 不在文档里", kind)
		}
		if a, ok := ev["actor"]; ok {
			known("actor", a)
		} else if kind != "status_tick" && kind != "field" { // 场地事件没有 actor（STAGE-200）
			v.fail("%s 事件必须有 actor", kind)
		}
		for _, j := range list(ev, "joined") {
			v.addUnits([]any{j})
		}
		for _, l := range list(ev, "left") {
			known("left", l)
		}
		for _, x := range list(ev, "effects") {
			ef, _ := x.(obj)
			known("effect.target", ef["target"])
			if r := str(ef, "result"); !resultNames[r] {
				v.fail("效果 result %q 不在文档里", r)
			}
			for _, key := range []string{"delta", "hp_pct", "down"} {
				if _, ok := ef[key]; !ok {
					v.fail("效果缺少 %s", key)
				}
			}
			no, _ := number(ef["target"])
			if _, hasHP := ef["hp"]; hasHP && no/10 != v.you/10 {
				v.fail("对方单位 %d 的效果里不应该有 hp", no)
			}
		}
	}
	if len(list(d, "state")) == 0 && len(list(d, "events")) == 0 {
		v.fail("battle_round 没有 events 也没有 state")
	}
	for _, s := range list(d, "state") {
		sm, _ := s.(obj)
		known("state", sm["no"])
	}
}
