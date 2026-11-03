package main

import (
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 手工构造的小脚本（不依赖真实数据）。块的台词只要非空就有效果：ACCEPT 和 REQUEST 要有感谢话，MESSAGE 要有台词。

func thanks() gamedata.EventMsg { return gamedata.EventMsg{Thanks: "谢谢", Nomal: "你好"} }

// accept 是一个 ACCEPT 块：条件、交出的完成事件（end_set）。
func accept(cond [][]gamedata.EventTerm, end ...int) gamedata.EventBlock {
	return gamedata.EventBlock{Type: "ACCEPT", Event: -1, Cond: cond, Msg: thanks(), EndSet: end}
}

// message 是一个 MESSAGE 块：条件、进行中的事件（-1 为无），以及给的道具。
func message(cond [][]gamedata.EventTerm, event int, getItem ...int) gamedata.EventBlock {
	b := gamedata.EventBlock{Type: "MESSAGE", Event: event, Cond: cond, Msg: thanks()}
	for _, id := range getItem {
		b.GetItem = append(b.GetItem, gamedata.ItemCount{ID: id, N: 1})
	}
	return b
}

func script(id int, file string, blocks ...gamedata.EventBlock) *gamedata.EventScript {
	return &gamedata.EventScript{ID: id, File: file, Blocks: blocks}
}

// 条件的简写：一组与的条件，外层一组即可。
func all(terms ...gamedata.EventTerm) [][]gamedata.EventTerm { return [][]gamedata.EventTerm{terms} }

func itemIs(id int) gamedata.EventTerm { return gamedata.EventTerm{Op: "ITEM", Cmp: "=", ID: id, N: 1} }
func endIs(n int) gamedata.EventTerm   { return gamedata.EventTerm{Op: "ENDEV", Cmp: "=", N: n} }
func lvOf(cmp string, n int) gamedata.EventTerm {
	return gamedata.EventTerm{Op: "LV", Cmp: cmp, N: n}
}
func petIs(template int) gamedata.EventTerm {
	return gamedata.EventTerm{Op: "PET", Pet: &gamedata.PetAsk{Cmp: "=", Level: 999, Template: template, Count: 1}}
}

// testEnv 是假的游戏数据：怪物 enemy → 模板（只有 55 → 30），可捕捉的宠物。
func testEnv(catchable ...int) Env {
	set := map[int]bool{}
	for _, t := range catchable {
		set[t] = true
	}
	return Env{
		EnemyTemplate: func(id int) (int, bool) {
			if id == 55 {
				return 30, true
			}
			return 0, false
		},
		PetCatchable: func(t int) bool { return set[t] },
	}
}

// allEnabled 是白名单（全部放开）和整本支持（全部放开）的假规则；enabled 为空时全部放开。
func rulesFor(enabled ...string) Rules {
	set := map[string]bool{}
	for _, f := range enabled {
		set[f] = true
	}
	return Rules{
		Enabled: func(f string) bool { return set[f] },
		Full:    func(string) bool { return false },
	}
}

// reached 报告事件号 n 是否可能完成。
func reached(c *Closure, n int) bool { return c.Has[Fact{Kind: KindEnd, N: n}] }

func TestChainReachableWhenAllScriptsEnabled(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/first", accept(all(itemIs(500)), 1)), // 500 没有脚本会给，视为外部可得
		script(2, "t/second", accept(all(endIs(1)), 2)),
		script(3, "t/third", accept(all(endIs(2)), 3)),
	}
	m := NewModel(scripts, nil, testEnv())
	c := m.CloseA(rulesFor("t/first", "t/second", "t/third"))
	for _, n := range []int{1, 2, 3} {
		if !reached(c, n) {
			t.Errorf("事件 %d 应该可达（链 A→B→C）", n)
		}
	}
}

func TestChainBreaksWhenMiddleScriptMissing(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/first", accept(all(itemIs(500)), 1)),
		script(2, "t/second", accept(all(endIs(1)), 2)),
		script(3, "t/third", accept(all(endIs(2)), 3)),
	}
	m := NewModel(scripts, nil, testEnv())
	c := m.CloseA(rulesFor("t/first", "t/third")) // 中间的 t/second 没放开
	if !reached(c, 1) {
		t.Errorf("事件 1 应该可达")
	}
	if reached(c, 3) {
		t.Errorf("中间脚本没放开，事件 3 不应可达")
	}
}

func TestItemFromAnotherScriptNeedsThatScript(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/getter", message(all(gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 999, N: 1}), -1, 700)), // 脚本给 700
		script(2, "t/user", accept(all(itemIs(700)), 5)),
	}
	m := NewModel(scripts, nil, testEnv())
	if reached(m.CloseA(rulesFor("t/user")), 5) {
		t.Errorf("700 只由 t/getter 给，t/getter 没放开时事件 5 不应可达")
	}
	c := m.CloseA(rulesFor("t/getter", "t/user"))
	if !reached(c, 5) {
		t.Errorf("t/getter 放开后事件 5 应可达")
	}
	if d := c.Derive(Fact{Kind: KindEnd, N: 5}); len(d.Units) != 2 || len(d.External) != 0 {
		t.Errorf("事件 5 的推导应包含两个块、没有外部道具，得到 %d 块、%d 外部", len(d.Units), len(d.External))
	}
}

func TestItemWithNoSourceIsExternal(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/user", accept(all(itemIs(800)), 6)), // 800 没有脚本会给：商店或战斗掉落
	}
	m := NewModel(scripts, nil, testEnv())
	c := m.CloseA(rulesFor("t/user"))
	if !reached(c, 6) {
		t.Fatalf("没有脚本给的道具视为外部可得，事件 6 应可达")
	}
	d := c.Derive(Fact{Kind: KindEnd, N: 6})
	if len(d.External) != 1 || d.External[0] != (Fact{Kind: KindItem, N: 800}) {
		t.Errorf("外部道具应记为 800，得到 %v", d.External)
	}
}

func TestPetNeedsGiverOrCatchable(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/pet", accept(all(petIs(30)), 7)),
		script(2, "t/giver", message(all(gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 999, N: 1}), -1)),
	}
	// 没有 GetPet 的块：脚本里没人给宠物 30（giver 不送宠物），宠物 30 也不能捕捉，事件 7 不可达。
	m := NewModel(scripts, nil, testEnv())
	if reached(m.CloseA(rulesFor("t/pet", "t/giver")), 7) {
		t.Errorf("宠物 30 既不能捕捉、脚本也不送，事件 7 不应可达")
	}
	// 用 GetPet 送：怪物 55 的模板是 30，t/giver 放开后可得。
	scripts[1] = script(2, "t/giver", gamedata.EventBlock{Type: "MESSAGE", Event: -1, Cond: all(gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 999, N: 1}), Msg: thanks(), GetPet: []int{55}})
	m = NewModel(scripts, nil, testEnv())
	if reached(m.CloseA(rulesFor("t/pet")), 7) {
		t.Errorf("t/giver 没放开，宠物 30 不可得，事件 7 不应可达")
	}
	if !reached(m.CloseA(rulesFor("t/pet", "t/giver")), 7) {
		t.Errorf("t/giver 放开后送出宠物 30，事件 7 应可达")
	}
	// 可捕捉的宠物 31：不需要脚本。
	scripts = []*gamedata.EventScript{script(1, "t/pet", accept(all(petIs(31)), 8))}
	m = NewModel(scripts, nil, testEnv(31))
	if !reached(m.CloseA(rulesFor("t/pet")), 8) {
		t.Errorf("可捕捉的宠物 31 视为可得，事件 8 应可达")
	}
}

func TestKeywordBlockIsExecutable(t *testing.T) {
	blk := accept(all(endIs(7)), 9)
	blk.Keyword = "芝麻开门"
	scripts := []*gamedata.EventScript{
		script(1, "t/pet", accept(all(itemIs(500)), 7)),
		script(2, "t/kw", blk),
	}
	m := NewModel(scripts, nil, testEnv())
	c := m.CloseA(rulesFor("t/pet", "t/kw"))
	if !reached(c, 9) {
		t.Errorf("暗号块在模型里当成可执行，事件 9 应可达")
	}
	d := c.Derive(Fact{Kind: KindEnd, N: 9})
	if len(d.Units) != 2 {
		t.Fatalf("事件 9 的推导应有两个块，得到 %d", len(d.Units))
	}
	found := false
	for _, u := range d.Units {
		found = found || u.Block.Keyword == "芝麻开门"
	}
	if !found {
		t.Errorf("推导里应包含暗号块")
	}
}

func TestUnsupportedBlockOnlyInC(t *testing.T) {
	bad := accept(all(itemIs(500)), 9)
	bad.Unsupported = "条件 DR<11 不支持"
	scripts := []*gamedata.EventScript{script(1, "t/bad", bad)}
	m := NewModel(scripts, nil, testEnv())
	if reached(m.CloseA(rulesFor("t/bad")), 9) {
		t.Errorf("不支持的块在场景 A 不执行")
	}
	allowB := rulesFor("t/bad")
	allowB.Full = func(string) bool { return true }
	if reached(m.CloseB(allowB), 9) {
		t.Errorf("不支持的块在场景 B 不执行（即使脚本整本支持的判断为真）")
	}
	if !reached(m.CloseC(rulesFor()), 9) {
		t.Errorf("场景 C 忽略 Unsupported，事件 9 应可达")
	}
}

func TestUnsupportedWithEmptyCondIsSatisfiedOnlyInC(t *testing.T) {
	empty := gamedata.EventBlock{Type: "ACCEPT", Event: -1, Msg: thanks(), EndSet: []int{12}, Unsupported: "条件 SP=0 不支持"}
	supported := gamedata.EventBlock{Type: "ACCEPT", Event: -1, Msg: thanks(), EndSet: []int{13}} // 没有条件：运行时永远不命中
	scripts := []*gamedata.EventScript{script(1, "t/empty", empty, supported)}
	m := NewModel(scripts, nil, testEnv())
	if c := m.CloseC(rulesFor()); !reached(c, 12) {
		t.Errorf("场景 C 里条件没解析的块视为可满足，事件 12 应可达")
	}
	if c := m.CloseC(rulesFor()); reached(c, 13) {
		t.Errorf("支持的块没有条件时和运行时一样永远不命中，事件 13 不应可达")
	}
}

func TestCycleTerminates(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/c1", accept(all(endIs(11)), 10)),
		script(2, "t/c2", accept(all(endIs(10)), 11)),
	}
	m := NewModel(scripts, nil, testEnv())
	c := m.CloseA(rulesFor("t/c1", "t/c2"))
	if reached(c, 10) || reached(c, 11) {
		t.Errorf("循环依赖没有入口，两者都不应可达")
	}
	scripts = append(scripts, script(3, "t/entry", accept(all(itemIs(900)), 11)))
	m = NewModel(scripts, nil, testEnv())
	c = m.CloseA(rulesFor("t/c1", "t/c2", "t/entry"))
	if !reached(c, 10) || !reached(c, 11) {
		t.Errorf("有入口后循环里的两个事件都应可达")
	}
}

func TestDelPetNeedsThePet(t *testing.T) {
	blk := accept(all(gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 1, N: 1}), 14)
	blk.DelPet = []gamedata.PetAsk{{Cmp: "=", Template: 33, Count: 1}}
	scripts := []*gamedata.EventScript{script(1, "t/delpet", blk)}
	m := NewModel(scripts, nil, testEnv())
	if reached(m.CloseA(rulesFor("t/delpet")), 14) {
		t.Errorf("要收的宠物 33 不可得，整笔交易不做，事件 14 不应可达")
	}
	m = NewModel(scripts, nil, testEnv(33))
	if !reached(m.CloseA(rulesFor("t/delpet")), 14) {
		t.Errorf("宠物 33 可捕捉，收得到，事件 14 应可达")
	}
}

func TestLevelsKeepTheLowestAlternative(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/lv1", accept(all(lvOf(">", 19), itemIs(500)), 20)), // 要 20 级
		script(2, "t/lv2", accept(all(lvOf(">", 4), itemIs(500)), 20)),  // 同一个事件的另一条路，要 5 级
		script(3, "t/chain", accept(all(endIs(20), lvOf(">", 9)), 21)),  // 再接一步，要 10 级
	}
	m := NewModel(scripts, nil, testEnv())
	lv := m.Levels(rulesFor("t/lv1", "t/lv2", "t/chain").AllowA())
	if got := lv[Fact{Kind: KindEnd, N: 20}]; got != 5 {
		t.Errorf("事件 20 的最低等级需求应为 5，得到 %d", got)
	}
	if got := lv[Fact{Kind: KindEnd, N: 21}]; got != 10 {
		t.Errorf("事件 21 的最低等级需求应为 10（链上的最大值），得到 %d", got)
	}
}

func TestLevelsAgreeWithClosure(t *testing.T) {
	scripts := []*gamedata.EventScript{
		script(1, "t/a", accept(all(itemIs(500)), 1), accept(all(endIs(2)), 3)),
		script(2, "t/b", accept(all(endIs(1)), 2)),
		script(3, "t/c", accept(all(endIs(11)), 10)),
		script(4, "t/d", accept(all(endIs(10)), 11)),
	}
	m := NewModel(scripts, nil, testEnv())
	r := rulesFor("t/a", "t/b", "t/c", "t/d")
	c := m.CloseA(r)
	lv := m.Levels(r.AllowA())
	for f := range c.Has {
		if _, ok := lv[f]; !ok && f.Kind == KindEnd {
			t.Errorf("闭包里成立的 %v，等级需求里没有", f)
		}
	}
	for f := range lv {
		if !c.Has[f] && !m.external(f) {
			t.Errorf("等级需求里有 %v，闭包里不成立", f)
		}
	}
}

// 条件里引用、但没有任何脚本设置的事件，由别的系统设（外部可得）；被脚本设置的事件不算外部，要看设置它的脚本放没放开。
func TestEventReferencedButNeverSetIsExternal(t *testing.T) {
	scripts := []*gamedata.EventScript{script(1, "t/ref", accept(all(endIs(50)), 51))}
	m := NewModel(scripts, nil, testEnv())
	c := m.CloseA(rulesFor("t/ref"))
	if !reached(c, 51) {
		t.Fatalf("事件 50 没有脚本设置，按外部可得，事件 51 应可达")
	}
	d := c.Derive(Fact{Kind: KindEnd, N: 51})
	if len(d.External) != 1 || d.External[0] != (Fact{Kind: KindEnd, N: 50}) {
		t.Errorf("外部事件应记为完成的 50，得到 %v", d.External)
	}

	scripts = []*gamedata.EventScript{
		script(1, "t/set", accept(all(itemIs(500)), 50)), // 事件 50 由这个脚本设置，它没放开
		script(2, "t/ref", accept(all(endIs(50)), 51)),
	}
	m = NewModel(scripts, nil, testEnv())
	if reached(m.CloseA(rulesFor("t/ref")), 51) {
		t.Errorf("事件 50 有脚本设置且没放开，不应按外部可得处理")
	}
}

// 暗号块进清单：有效果的暗号块写进报告的“需要说的暗号”。
func TestKeywordBlockListedInReport(t *testing.T) {
	kw := message(all(gamedata.EventTerm{Op: "ITEM", Cmp: "!=", ID: 1, N: 1}), -1, 777)
	kw.Keyword = "芝麻开门"
	m := NewModel([]*gamedata.EventScript{script(1, "t/kw", kw)}, nil, testEnv())
	md := renderReport(m, rulesFor("t/kw"))
	if !strings.Contains(md, "「芝麻开门」") || !strings.Contains(md, "道具 777") {
		t.Errorf("报告的暗号清单里应有“芝麻开门”和它给的道具 777")
	}
}
