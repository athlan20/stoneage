// 分身队的单元测试（STAGE-162）：站位、自动出招、不领奖励、不碰忠诚、倒下与逃跑的收场。

package battle

import (
	"encoding/json"
	"strings"
	"testing"
)

// cloneSide 是一个分身的快照：人物加一只宠物，数值和队长一样（testSide）。
func cloneSide(name string) *SideInput {
	s := testSide()
	s.Name = name
	s.Bench = [PetSlots]*PetInput{}
	return s
}

func newCloneBattle(t *testing.T, n int, rnd func(n int) int) *Battle {
	t.Helper()
	b := newTestBattle(t, "win", rnd)
	var clones []*SideInput
	for i := 1; i <= n; i++ {
		clones = append(clones, cloneSide("勇者小明·分身"+string(rune('0'+i))))
	}
	b.AddClones(clones)
	return b
}

func TestCloneLayout(t *testing.T) {
	b := newCloneBattle(t, 4, rollRnd(9999))
	for i := 1; i <= 4; i++ {
		h, p := b.Unit(i), b.Unit(5+i)
		if h == nil || h.Kind != KindPlayer || !h.Auto || !strings.Contains(h.Name, "分身") {
			t.Fatalf("单位 %d 应是分身人物: %+v", i, h)
		}
		if p == nil || p.Kind != KindPet || !p.Auto {
			t.Fatalf("单位 %d 应是分身的宠物: %+v", 5+i, p)
		}
	}
	if u := b.Unit(0); u.Auto || b.Unit(5).Auto {
		t.Fatal("队长和队长的宠物不能是自动单位")
	}
	if needs := b.Needs(); len(needs) != 2 {
		t.Fatalf("分身不进 needs: %v", needs)
	}
	// 视图：分身带 auto，队长不带；己方都有准确血量。
	raw, _ := json.Marshal(b.StartView().Units)
	var units []map[string]any
	if err := json.Unmarshal(raw, &units); err != nil {
		t.Fatal(err)
	}
	if len(units) != 12 { // 己方 5 人 + 5 宠，敌方 2
		t.Fatalf("单位数 = %d，应为 12", len(units))
	}
	for _, u := range units {
		no := int(u["no"].(float64))
		_, hasAuto := u["auto"]
		wantAuto := no >= 1 && no <= 4 || no >= 6 && no <= 9
		if hasAuto != wantAuto {
			t.Errorf("单位 %d 的 auto 字段 = %v，应为 %v", no, hasAuto, wantAuto)
		}
		if no < 10 && u["hp"] == nil {
			t.Errorf("己方单位 %d 缺少 hp", no)
		}
	}
}

// 队长交完指令就能结算，分身和分身的宠物各出一手攻击，不用等它们。
func TestCloneActsAutomatically(t *testing.T) {
	b := newCloneBattle(t, 4, rollRnd(9999))
	for _, no := range []int{15, 16} { // 敌人血厚一点，免得队长一方先把它们打光、分身没有目标
		b.Unit(no).HP, b.Unit(no).MaxHP = 100000, 100000
	}
	submitAll(b, 15)
	if !b.AllSubmitted() {
		t.Fatal("队长交齐指令后应能结算")
	}
	round, _ := b.Settle()
	acted := map[int]bool{}
	for _, e := range round.Events {
		if e.Kind == "attack" && e.Actor != nil {
			acted[*e.Actor] = true
		}
	}
	for _, no := range []int{1, 2, 3, 4, 6, 7, 8, 9} {
		if !acted[no] {
			t.Errorf("分身单位 %d 本回合没有出手: %v", no, acted)
		}
	}
}

// 分身不领奖励：奖励里只有队长和队长的宠物，击杀数只记队长的宠物。
func TestCloneNoRewards(t *testing.T) {
	b := newCloneBattle(t, 4, rollRnd(9999))
	for turn := 0; turn < 40 && !b.Over(); turn++ {
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		submitAll(b, target)
		_, end := b.Settle()
		if end == nil {
			continue
		}
		if end.Result != "win" {
			t.Fatalf("结果 = %s，应为 win", end.Result)
		}
		if got := len(end.Rewards.Exp); got != 2 {
			t.Fatalf("经验条目 = %d，只应有队长和队长的宠物: %+v", got, end.Rewards.Exp)
		}
		for _, e := range end.Rewards.Exp {
			if strings.Contains(e.Name, "分身") {
				t.Fatalf("分身不能领经验: %+v", e)
			}
		}
		for _, k := range end.Rewards.PetKills {
			if k.Slot != 0 {
				t.Fatalf("击杀数只记队长的宠物（栏位 0）: %+v", end.Rewards.PetKills)
			}
		}
		for _, p := range b.petsActed {
			if p.Auto {
				t.Fatal("分身的宠物不能记入出过手的宠物")
			}
		}
		return
	}
	t.Fatal("40 回合内没有打赢")
}

// 分身和分身的宠物倒下，不扣队长宠物的忠诚，也不产生忠诚变化的推送。
func TestCloneDownKeepsLoyalty(t *testing.T) {
	b := newCloneBattle(t, 1, rollRnd(9999))
	leaderPet := b.Unit(5)
	before := leaderPet.loyVar
	b.onDown(b.Unit(1))
	b.onDown(b.Unit(6))
	b.addLoyalty(b.Unit(6), loyalWin)
	if leaderPet.loyVar != before || b.Unit(6).loyVar != 0 || b.loyalChanged {
		t.Fatalf("分身倒下不应改变忠诚: 队长宠物 %d→%d，分身宠物 %d，changed=%v",
			before, leaderPet.loyVar, b.Unit(6).loyVar, b.loyalChanged)
	}
	// 队长倒下照旧扣队长宠物的忠诚（原有规则不变）。
	b.onDown(b.Unit(0))
	if leaderPet.loyVar == before {
		t.Fatal("队长倒下应该扣队长宠物的忠诚")
	}
}

// 己方落败只看队长和队长的宠物：它们都倒下，分身还站着也输。
func TestCloneDoesNotKeepSideAlive(t *testing.T) {
	b := newCloneBattle(t, 2, rollRnd(9999))
	b.Unit(0).Down, b.Unit(5).Down = true, true
	if !b.sideGone(0) {
		t.Fatal("队长和宠物都倒下，己方应判落败")
	}
	if end := b.finish(); end == nil || end.Result != "lose" {
		t.Fatalf("结果 = %+v，应为 lose", end)
	}
}

// 分身倒下不影响战斗继续：队长和宠物还站着就接着打。
func TestCloneDownDoesNotEndBattle(t *testing.T) {
	b := newCloneBattle(t, 2, rollRnd(9999))
	for _, no := range []int{1, 2, 6, 7} {
		b.Unit(no).Down = true
	}
	if b.sideGone(0) {
		t.Fatal("队长还站着，己方不应判落败")
	}
}

// 队长逃跑成功：分身和它们的宠物一起离场，battle_round 的 left 里都有。
func TestCloneEscapeTakesTeam(t *testing.T) {
	b := newCloneBattle(t, 2, chanceRnd())
	b.Submit(b.Turn(), "player", "escape", Cmd{Target: -1})
	b.Submit(b.Turn(), "pet", "guard", Cmd{Target: -1})
	round, end := b.Settle()
	if end == nil || end.Result != "escaped" {
		t.Fatalf("应当逃跑成功: %+v", end)
	}
	var left []int
	for _, e := range round.Events {
		if e.Kind == "escape" && e.Actor != nil && *e.Actor == 0 {
			left = e.Left
		}
	}
	want := map[int]bool{0: true, 5: true, 1: true, 2: true, 6: true, 7: true}
	if len(left) != len(want) {
		t.Fatalf("left = %v，应为 %v", left, want)
	}
	for _, no := range left {
		if !want[no] {
			t.Fatalf("left 里多了 %d: %v", no, left)
		}
	}
	for no := range want {
		if b.Unit(no) != nil {
			t.Errorf("单位 %d 应已离场", no)
		}
	}
}

// 睡着的分身这回合不出手；定好的目标倒下后按合击顺序顺延。
func TestCloneRetargetsAndSkips(t *testing.T) {
	b := newCloneBattle(t, 2, rollRnd(9999))
	b.Unit(2).Status.Sleep = 2
	b.beginRound()
	if b.autoCmds[2] != nil {
		t.Fatal("睡着的分身不应有指令")
	}
	if b.autoCmds[1] == nil {
		t.Fatal("清醒的分身应有指令")
	}
	// 合击顺序的第一个是 16；把它打倒，顺延到下一个 15。
	dead := b.Unit(b.autoCmds[1].Target)
	if dead.No != 16 {
		t.Fatalf("默认目标 = %d，应为合击顺序的第一个 16", dead.No)
	}
	dead.Down = true
	evs := b.autoAct(b.Unit(1))
	if len(evs) == 0 || evs[0].Kind != "attack" || evs[0].Effects[0].Target != 15 {
		t.Fatalf("应顺延到合击顺序的下一个 15: %+v", evs)
	}
	// 两个都没了：没有目标可打，跳过。
	b.Unit(15).Down = true
	if evs := b.autoAct(b.Unit(1)); len(evs) != 1 || evs[0].Kind != "skip" {
		t.Fatalf("没有敌人时应跳过: %+v", evs)
	}
}

// STAGE-312：没有方案时，分身和分身宠物默认打合击顺序的第一个敌人（前排从上到下、再后排），不再各自随机。
func TestCloneDefaultsToComboOrder(t *testing.T) {
	b := newCloneBattle(t, 4, rollRnd(9999))
	planNow(b)
	for _, no := range cloneNos {
		expectCmd(t, b, no, Cmd{Action: "attack", Target: 16})
	}
	// 16 倒下后，全部改打顺序里的下一个 15。
	b.Unit(16).Down = true
	planNow(b)
	for _, no := range cloneNos {
		expectCmd(t, b, no, Cmd{Action: "attack", Target: 15})
	}
}

// STAGE-312：合击打开时，默认目标一致的分身和分身宠物能跟队长凑成合击。
func TestCloneDefaultsComboEvent(t *testing.T) {
	b := newCloneBattle(t, 4, comboRnd(1, 0)) // 起头判定出 1，必定成功
	b.Combo = true
	for _, no := range []int{15, 16} { // 血厚一点，免得第一个人的一下就把目标打倒、后面的组员没得出手
		b.Unit(no).HP, b.Unit(no).MaxHP = 100000, 100000
	}
	submitAll(b, 16)
	round, _ := b.Settle()
	for _, e := range round.Events {
		if e.Kind != "combo" {
			continue
		}
		for _, eff := range e.Effects {
			if eff.Target != 16 {
				t.Fatalf("合击打到了 %d 号，应全部打合击顺序的第一个 16: %+v", eff.Target, e)
			}
		}
		for _, no := range e.Actors {
			if no >= 1 && no <= 4 || no >= 6 && no <= 9 {
				return // 有分身或分身宠物参与
			}
		}
		t.Fatalf("combo 事件里没有分身参与: %+v", e)
	}
	t.Fatalf("没有 combo 事件: %+v", round.Events)
}
