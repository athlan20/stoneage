// 玩家给分身指定攻击目标的单元测试（STAGE-186）：可选指令列表、校验、指定的目标出手、没指定的仍随机、
// 目标倒下改选、不影响“队长交齐就结算”。

package battle

import (
	"reflect"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

func submitErr(b *Battle, actor string, cmd Cmd) *protocol.Error {
	_, perr := b.Submit(b.Turn(), actor, "attack", cmd)
	return perr
}

// 全部 4 个分身加宠物都能行动：optional 按单位号列出，needs 不变。
func TestCloneOptionalList(t *testing.T) {
	b := newCloneBattle(t, 2, rollRnd(9999))
	want := []string{"clone:1", "clone:2", "clone_pet:1", "clone_pet:2"}
	if got := b.Optional(); !reflect.DeepEqual(got, want) {
		t.Fatalf("optional = %v，应为 %v", got, want)
	}
	if needs := b.Needs(); len(needs) != 2 {
		t.Fatalf("needs = %v，分身不能进 needs", needs)
	}
	if b2 := newTestBattle(t, "win", rollRnd(9999)); b2.Optional() != nil {
		t.Fatalf("没有分身时 optional 应为空: %v", b2.Optional())
	}
}

// 睡着、倒下的分身不在 optional 里，提交回复 COMMAND_NOT_NEEDED。
func TestCloneOptionalSkipsDisabled(t *testing.T) {
	b := newCloneBattle(t, 2, rollRnd(9999))
	b.Unit(1).Status.Sleep = 3
	b.Unit(7).HP, b.Unit(7).Down = 0, true
	b.nextTurn()
	want := []string{"clone:2", "clone_pet:1"}
	if got := b.Optional(); !reflect.DeepEqual(got, want) {
		t.Fatalf("optional = %v，应为 %v", got, want)
	}
	for _, actor := range []string{"clone:1", "clone_pet:2"} {
		if perr := submitErr(b, actor, Cmd{Target: 15}); perr == nil || perr.Code != protocol.CodeCommandNotNeeded {
			t.Errorf("%s: %v，应为 COMMAND_NOT_NEEDED", actor, perr)
		}
	}
}

func TestCloneSubmitValidation(t *testing.T) {
	b := newCloneBattle(t, 1, rollRnd(9999))
	cases := []struct {
		name, actor, action string
		target              int
		code                string
	}{
		{"只能 attack", "clone:1", "guard", 15, protocol.CodeBadRequest},
		{"分身号越界", "clone:5", "attack", 15, protocol.CodeBadRequest},
		{"没有这个分身", "clone:2", "attack", 15, protocol.CodeCommandNotNeeded},
		{"目标范围", "clone:1", "attack", 20, protocol.CodeBadRequest},
		{"选了己方", "clone:1", "attack", 0, protocol.CodeInvalidTarget},
		{"空格子", "clone_pet:1", "attack", 19, protocol.CodeInvalidTarget},
		{"写法不规范", "clone:01", "attack", 15, protocol.CodeBadRequest},
	}
	for _, c := range cases {
		_, perr := b.Submit(b.Turn(), c.actor, c.action, Cmd{Target: c.target})
		if perr == nil || perr.Code != c.code {
			t.Errorf("%s: %v，应为 %s", c.name, perr, c.code)
		}
	}
	if _, perr := b.Submit(b.Turn()+1, "clone:1", "attack", Cmd{Target: 15}); perr == nil || perr.Code != protocol.CodeWrongTurn {
		t.Errorf("回合不对: %v，应为 WRONG_TURN", perr)
	}
	if len(b.cloneCmds) != 0 {
		t.Fatalf("校验失败不能留下指令: %v", b.cloneCmds)
	}
}

// 指定的目标按指定的打；改选以后以最后一次为准；提交分身指令不影响“队长交齐就结算”。
func TestCloneTargetsHonored(t *testing.T) {
	b := newCloneBattle(t, 2, rollRnd(9999))
	for _, no := range []int{15, 16} {
		b.Unit(no).HP, b.Unit(no).MaxHP = 100000, 100000
	}
	for _, actor := range []string{"clone:1", "clone_pet:2"} {
		next, perr := b.Submit(b.Turn(), actor, "attack", Cmd{Target: 16})
		if perr != nil || next != "player" {
			t.Fatalf("%s: next=%q err=%v，应为 player", actor, next, perr)
		}
	}
	if perr := submitErr(b, "clone:1", Cmd{Target: 15}); perr != nil { // 改选
		t.Fatal(perr)
	}
	if got := b.Submitted(); !got["clone:1"] || got["clone:2"] || !got["clone_pet:2"] || got["player"] {
		t.Fatalf("submitted = %v", got)
	}
	if b.AllSubmitted() {
		t.Fatal("队长还没交，不能结算")
	}
	submitAll(b, 15)
	if !b.AllSubmitted() {
		t.Fatal("队长交齐后应能结算，分身指令不影响")
	}
	round, _ := b.Settle()
	hit := map[int]int{}
	for _, e := range round.Events {
		if e.Kind == "attack" && e.Actor != nil && len(e.Effects) > 0 {
			hit[*e.Actor] = e.Effects[0].Target
		}
	}
	if hit[1] != 15 || hit[7] != 16 {
		t.Fatalf("指定的目标没生效: %v", hit)
	}
	if _, ok := hit[2]; !ok {
		t.Fatalf("没指定的分身仍要出手: %v", hit)
	}
}

// STAGE-312：默认目标按合击顺序定，不消耗随机数（指定与不指定都不抽）。
func TestCloneDefaultTargetUsesNoRnd(t *testing.T) {
	count := func(specify int) int {
		n := 0
		b := newCloneBattle(t, 2, func(k int) int { n++; return 9999 % k })
		actors := []string{"clone:1", "clone:2", "clone_pet:1", "clone_pet:2"}
		for _, a := range actors[:specify] {
			if perr := submitErr(b, a, Cmd{Target: 15}); perr != nil {
				t.Fatal(perr)
			}
		}
		n = 0
		b.planAuto()
		return n
	}
	if c0, c2, c4 := count(0), count(2), count(4); c0 != 0 || c2 != 0 || c4 != 0 {
		t.Fatalf("默认目标不该消耗随机数：全不指定 %d、指定 2 个 %d、全指定 %d", c0, c2, c4)
	}
}

// 指定的目标在行动前倒下，改选一个活着的，不丢行动。
func TestCloneTargetDownReselect(t *testing.T) {
	b := newCloneBattle(t, 1, rollRnd(9999))
	if perr := submitErr(b, "clone:1", Cmd{Target: 16}); perr != nil {
		t.Fatal(perr)
	}
	b.planAuto()
	if got := b.autoCmds[1].Target; got != 16 {
		t.Fatalf("planAuto 目标 = %d，应为指定的 16", got)
	}
	b.Unit(16).HP, b.Unit(16).Down = 0, true
	evs := b.autoAct(b.Unit(1))
	if len(evs) == 0 || evs[0].Kind != "attack" || evs[0].Effects[0].Target != 15 {
		t.Fatalf("目标倒下应改选 15: %+v", evs)
	}
}

// 下一回合指定的目标清空，回到合击顺序的默认目标。
func TestCloneCmdsResetEachTurn(t *testing.T) {
	b := newCloneBattle(t, 1, rollRnd(9999))
	if perr := submitErr(b, "clone:1", Cmd{Target: 16}); perr != nil {
		t.Fatal(perr)
	}
	b.nextTurn()
	if len(b.cloneCmds) != 0 || b.Submitted()["clone:1"] {
		t.Fatalf("新回合应清掉分身指令: %v", b.cloneCmds)
	}
}
