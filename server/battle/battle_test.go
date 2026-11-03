// 引擎的单元测试（STAGE-43）：单位布局、指令校验的错误码、一次完整战斗的推进。
// 数值公式（伤害、命中、暴击）在 STAGE-44 对照原版后另有测试。

package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// testSide 构造一个最小的一方：人物 + 一只宠物 + 一只备用宠物。
// 数值故意调强，让 win / capture 这些用例能稳定打完；lose 用 weakSide。
func testSide() *SideInput {
	side := &SideInput{
		Name: "勇者小明", Level: 1, HP: 300, MaxHP: 300, MP: 100, MaxMP: 100,
		Attack: 30, Defense: 20, Agility: 4, Charm: 60,
		Elem: Elements{Earth: 60, Water: 40},
		BattlePet: &PetInput{
			Slot: 0, TemplateID: 1, Name: "乌力", Level: 5, HP: 200, MaxHP: 200,
			Attack: 25, Defense: 20, Agility: 7, Elem: Elements{Earth: 80, Water: 20},
			SkillSlots: 7, Skills: [7]int{1, -1, -1, -1, -1, -1, -1},
			LoyaltyBase: 100, // 忠诚满，一直听话（STAGE-127）
		},
	}
	side.Bench[1] = &PetInput{
		Slot: 1, TemplateID: 11, Name: "布比", Level: 3, HP: 150, MaxHP: 150,
		Attack: 20, Defense: 15, Agility: 5, Elem: Elements{Fire: 50, Wind: 50},
		LoyaltyBase: 100,
	}
	for i := range side.Magics {
		side.Magics[i] = -1
	}
	side.Magics[0] = 301 // 岩石撞击（气力 10；MP 300 够用）
	return side
}

// weakSide 是数值很弱的一方（lose 测试用：怪物两三下就能打倒）。
func weakSide() *SideInput {
	side := testSide()
	side.HP, side.MaxHP, side.Attack, side.Defense = 4, 4, 2, 0
	side.BattlePet.HP, side.BattlePet.MaxHP = 4, 4
	return side
}

// fixedRnd 是固定序列的随机源。
func fixedRnd(vals ...int) func(n int) int {
	i := 0
	return func(n int) int {
		if n <= 0 {
			return 0
		}
		v := vals[i%len(vals)] % n
		i++
		return v
	}
}

// rollRnd 让所有骰子都出 roll：判定类（闪避、暴击、反击的 d10000）出大值就不触发，
// 二选一类的随机取小值。这样测试不依赖引擎里随机调用的顺序。
func rollRnd(roll int) func(n int) int {
	return func(n int) int {
		return roll % n
	}
}

// chanceRnd 让 d100（逃跑、捕捉）都出 10（roll 后 11，小于逃跑的 26 和打残后捕捉的 15 左右），
// 其余骰子出大值不触发判定。
func chanceRnd() func(n int) int {
	return func(n int) int {
		if n == 100 {
			return 10
		}
		return 9999 % n
	}
}

func newTestBattle(t *testing.T, scenario string, rnd func(n int) int) *Battle {
	return newBattleWithSide(t, testSide(), scenario, rnd)
}

func newBattleWithSide(t *testing.T, side *SideInput, scenario string, rnd func(n int) int) *Battle {
	t.Helper()
	b, err := Build("b_1", side, scenario)
	if err != nil {
		t.Fatal(err)
	}
	b.Rnd = rnd // 覆盖剧本的种子随机源，让结果可控
	return b
}

func TestBattleLayout(t *testing.T) {
	b := newTestBattle(t, "win", fixedRnd(1, 2, 3, 4))
	if b.You != 0 {
		t.Fatalf("you = %d，应为 0", b.You)
	}
	if u := b.Unit(0); u == nil || u.Kind != KindPlayer || u.Name != "勇者小明" {
		t.Fatalf("单位 0 应是玩家勇者小明")
	}
	if u := b.Unit(5); u == nil || u.Kind != KindPet || u.Name != "乌力" {
		t.Fatalf("单位 5 应是宠物乌力")
	}
	for _, no := range []int{15, 16} {
		if u := b.Unit(no); u == nil || u.Kind != KindMonster {
			t.Fatalf("单位 %d 应是怪物", no)
		}
	}
	if needs := b.Needs(); len(needs) != 2 || needs[0] != "player" || needs[1] != "pet" {
		t.Fatalf("needs = %v，应为 [player pet]", needs)
	}
}

func TestBattleSubmitChecks(t *testing.T) {
	b := newTestBattle(t, "no_escape", fixedRnd(1, 2, 3, 4))
	code := func(perr *protocol.Error) string {
		if perr == nil {
			return ""
		}
		return perr.Code
	}
	if _, perr := b.Submit(9, "player", "attack", Cmd{Target: 15}); code(perr) != protocol.CodeWrongTurn {
		t.Fatalf("turn 不对：got %v want WRONG_TURN", perr)
	}
	if _, perr := b.Submit(1, "pet", "attack", Cmd{Target: 15}); code(perr) != protocol.CodeCommandOrder {
		t.Fatalf("先交宠物：got %v want COMMAND_ORDER", perr)
	}
	if _, perr := b.Submit(1, "player", "attack", Cmd{Target: 99}); code(perr) != protocol.CodeBadRequest {
		t.Fatalf("target 越界：got %v want BAD_REQUEST", perr)
	}
	if _, perr := b.Submit(1, "player", "attack", Cmd{Target: 3}); code(perr) != protocol.CodeInvalidTarget {
		t.Fatalf("空格子：got %v want INVALID_TARGET", perr)
	}
	if _, perr := b.Submit(1, "player", "attack", Cmd{Target: 10}); code(perr) != protocol.CodeInvalidTarget {
		t.Fatalf("对方空格子：got %v want INVALID_TARGET", perr)
	}
	if _, perr := b.Submit(1, "player", "escape", Cmd{}); code(perr) != protocol.CodeActionNotAllowed {
		t.Fatalf("no_escape：got %v want ACTION_NOT_ALLOWED", perr)
	}
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 4, Target: 15}); code(perr) != protocol.CodeMagicUnusable {
		t.Fatalf("空精灵栏：got %v want MAGIC_UNUSABLE", perr)
	}
	if _, perr := b.Submit(1, "player", "magic", Cmd{Magic: 0, Target: 3}); code(perr) != protocol.CodeInvalidTarget {
		t.Fatalf("有精灵但选了空格子：got %v want INVALID_TARGET", perr)
	}
	if _, perr := b.Submit(1, "player", "item", Cmd{Slot: 7}); code(perr) != protocol.CodeItemUnusable {
		t.Fatalf("没有物品：got %v want ITEM_UNUSABLE", perr)
	}
	if _, perr := b.Submit(1, "player", "swap_pet", Cmd{PetSlot: 3}); code(perr) != protocol.CodePetUnavailable {
		t.Fatalf("空宠物栏：got %v want PET_UNAVAILABLE", perr)
	}
	next, perr := b.Submit(1, "player", "attack", Cmd{Target: 15})
	if perr != nil || next != "pet" {
		t.Fatalf("合法指令：next=%v err=%v", next, perr)
	}
	// 宠物指令交上之前，人物指令可以重交：覆盖先交的 attack，回复还是 pet；重交照样校验。
	if next, perr := b.Submit(1, "player", "guard", Cmd{}); perr != nil || next != "pet" {
		t.Fatalf("重交人物指令：next=%v err=%v", next, perr)
	}
	if _, perr := b.Submit(1, "player", "attack", Cmd{Target: 5}); code(perr) != protocol.CodeInvalidTarget {
		t.Fatalf("重交不合法：got %v want INVALID_TARGET", perr)
	}
	if b.cmds["player"] == nil || b.cmds["player"].Action != "guard" {
		t.Fatalf("重交不通过时应保留先交的 guard")
	}
	if _, perr := b.Submit(1, "pet", "skill", Cmd{Skill: 5, Target: 15}); code(perr) != protocol.CodeSkillUnusable {
		t.Fatalf("空技能栏：got %v want SKILL_UNUSABLE", perr)
	}
	if _, perr := b.Submit(1, "pet", "attack", Cmd{Target: 15}); perr != nil {
		t.Fatalf("宠物指令不应报错: %v", perr)
	}
}

// TestBattleResubmitLocked 宠物指令交上以后，或者人物指令用了换宠（宠物本回合不需要指令），人物指令不能再改。
func TestBattleResubmitLocked(t *testing.T) {
	isSubmitted := func(perr *protocol.Error) bool {
		return perr != nil && perr.Code == protocol.CodeCommandSubmitted
	}
	b := newTestBattle(t, "no_escape", fixedRnd(1, 2, 3, 4))
	if _, perr := b.Submit(1, "player", "attack", Cmd{Target: 15}); perr != nil {
		t.Fatalf("人物攻击：%v", perr)
	}
	if _, perr := b.Submit(1, "pet", "attack", Cmd{Target: 15}); perr != nil {
		t.Fatalf("宠物攻击：%v", perr)
	}
	if _, perr := b.Submit(1, "player", "guard", Cmd{}); !isSubmitted(perr) {
		t.Fatalf("宠物指令交上后改人物指令：got %v want COMMAND_SUBMITTED", perr)
	}

	swap := newTestBattle(t, "win", rollRnd(9999))
	if next, perr := swap.Submit(1, "player", "swap_pet", Cmd{PetSlot: 1}); perr != nil || next != "wait" {
		t.Fatalf("换宠：next=%v err=%v", next, perr)
	}
	if _, perr := swap.Submit(1, "player", "guard", Cmd{}); !isSubmitted(perr) {
		t.Fatalf("换宠后改人物指令：got %v want COMMAND_SUBMITTED", perr)
	}
}

// submitAll 按本回合 needs 依次交指令（玩家倒了就只剩宠物出招）。
func submitAll(b *Battle, target int) {
	if b.PlayerUnit().Alive() {
		b.Submit(b.Turn(), "player", "attack", Cmd{Target: target})
	}
	if p := b.PetUnit(); p != nil && p.Alive() {
		if b.cmds["pet"] == nil && needsPet(b) {
			b.Submit(b.Turn(), "pet", "attack", Cmd{Target: target})
		}
	}
}

func needsPet(b *Battle) bool {
	for _, a := range b.Needs() {
		if a == "pet" {
			return true
		}
	}
	return false
}

// TestBattleWin 推一场到胜利：回合号递增、事件有序、结束推 win 与奖励。
func TestBattleWin(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(9999)) // 判定必 hit；AI 固定选 0 号目标
	for turn := 1; turn <= 40; turn++ {
		if b.Over() {
			break
		}
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		submitAll(b, target)
		if !b.AllSubmitted() {
			t.Fatal("交齐了但 AllSubmitted 为假")
		}
		round, end := b.Settle()
		if round.Turn != turn {
			t.Fatalf("round.Turn = %d, want %d", round.Turn, turn)
		}
		if len(round.Events) == 0 {
			t.Fatalf("turn %d 没有事件", turn)
		}
		for _, e := range round.Events {
			if e.Kind == "attack" && (e.Effects == nil || len(e.Effects) == 0) {
				t.Fatalf("attack 事件没有效果: %+v", e)
			}
		}
		if end != nil {
			if end.Result != "win" {
				t.Fatalf("结果 = %s, want win", end.Result)
			}
			if end.Rewards == nil || len(end.Rewards.Exp) != 2 {
				t.Fatalf("win 应有经验和奖励: %+v", end.Rewards)
			}
			return
		}
	}
	t.Fatal("40 回合内没有打赢")
}

// TestBattleLose 玩家倒下时结束为 lose。
func TestBattleLose(t *testing.T) {
	b := newBattleWithSide(t, weakSide(), "lose", rollRnd(9999))
	for i := 0; i < 40 && !b.Over(); i++ {
		target := 15
		if u := b.Unit(15); u == nil || !u.Alive() {
			target = 16
		}
		if b.PlayerUnit().Alive() {
			if _, perr := b.Submit(b.Turn(), "player", "attack", Cmd{Target: target}); perr != nil {
				t.Fatalf("turn %d: %v", i, perr)
			}
		}
		if needsPet(b) {
			b.Submit(b.Turn(), "pet", "attack", Cmd{Target: target}) // 不防御：guard 会挡住全部伤害，打不出失败
		}
		if !b.AllSubmitted() {
			t.Fatal("没交齐")
		}
		_, end := b.Settle()
		if end != nil {
			if end.Result != "lose" {
				t.Fatalf("结果 = %s, want lose", end.Result)
			}
			if end.Penalty == nil {
				t.Fatal("lose 应有 penalty")
			}
			return
		}
	}
	t.Fatal("40 回合内没分出胜负")
}

// TestBattleEscape 逃跑成功时事件里人物和宠物都离场，结果 escaped。
func TestBattleEscape(t *testing.T) {
	b := newTestBattle(t, "escape", chanceRnd()) // 逃跑必成功
	b.Submit(b.Turn(), "player", "escape", Cmd{})
	b.Submit(b.Turn(), "pet", "guard", Cmd{})
	round, end := b.Settle()
	if end == nil || end.Result != "escaped" {
		t.Fatalf("end = %+v, want escaped", end)
	}
	var escaped *Event
	for i, e := range round.Events {
		if e.Kind == "escape" && e.OK != nil && *e.OK {
			escaped = &round.Events[i]
		}
	}
	if escaped == nil {
		t.Fatal("没有逃跑成功的事件")
	}
	if len(escaped.Left) != 2 || escaped.Left[0] != 0 || escaped.Left[1] != 5 {
		t.Fatalf("逃跑离场 = %v, want [0 5]", escaped.Left)
	}
}

// TestBattleSwapPet 换宠：旧宠物离场、新宠物上场、事件里带 joined。
func TestBattleSwapPet(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(9999))
	next, perr := b.Submit(1, "player", "swap_pet", Cmd{PetSlot: 1})
	if perr != nil || next != "wait" {
		t.Fatalf("换宠 next=%v err=%v", next, perr)
	}
	if !b.AllSubmitted() {
		t.Fatal("换宠后这回合不需要宠物指令")
	}
	round, _ := b.Settle()
	var swap *Event
	for i, e := range round.Events {
		if e.Kind == "swap_pet" {
			swap = &round.Events[i]
		}
	}
	if swap == nil {
		t.Fatalf("应有换宠事件: %+v", round.Events)
	}
	if len(swap.Left) != 1 || swap.Left[0] != 5 {
		t.Fatalf("旧宠物应离场: %v", swap.Left)
	}
	if len(swap.Joined) != 1 || swap.Joined[0].Name != "布比" {
		t.Fatalf("新宠物应上场: %+v", swap.Joined)
	}
	if pet := b.PetUnit(); pet == nil || pet.PetSlot != 1 {
		t.Fatalf("出战的应是 1 号栏布比: %+v", pet)
	}
}

// TestBattleCapture 捕捉成功：怪物离场、进捕获名单、奖励里带宠物。
func TestBattleCapture(t *testing.T) {
	b := newTestBattle(t, "capture", chanceRnd()) // 捕捉必成功
	b.PlayerUnit().Attack = 5                     // 攻 30 会把 3 级乌力一击打死，捕捉要先打残
	b.Cbs.PlaceCaptured = func(m *Unit, _ *CaptureKeep) (int, *Discarded) { return 1, nil }
	// 捕捉前先打两回合（BATTLE_CaptureCheck：概率 ≈ 10 − HP²/MaxHP，满血是负的，
	// 这只 3 级乌力要打到三成血左右才转正）。
	for turn := 1; turn <= 2; turn++ {
		b.Submit(turn, "player", "attack", Cmd{Target: 15})
		b.Submit(turn, "pet", "guard", Cmd{})
		b.Settle()
		if u := b.Unit(15); u == nil || !u.Alive() {
			t.Fatalf("第 %d 回合把 15 号打死了", turn)
		}
	}
	b.Submit(3, "player", "capture", Cmd{Target: 15})
	b.Submit(3, "pet", "guard", Cmd{}) // 别把剩下的 1 级乌力一击打死，捕捉后战斗要继续
	round, end := b.Settle()
	if end != nil || b.Over() {
		t.Fatalf("捕捉后战斗应继续: %+v", end)
	}
	var cap *Event
	for i, e := range round.Events {
		if e.Kind == "capture" {
			cap = &round.Events[i]
		}
	}
	if cap == nil || cap.OK == nil || !*cap.OK {
		t.Fatalf("应有捕捉成功事件: %+v", round.Events)
	}
	if len(b.Captured()) != 1 || b.Captured()[0].No != 15 {
		t.Fatalf("捕获名单: %+v", b.Captured())
	}
	// 磨死剩下的怪，确认奖励里有捕捉到的宠物。
	for i := 0; i < 40 && !b.Over(); i++ {
		b.Submit(b.Turn(), "player", "attack", Cmd{Target: 16})
		b.Submit(b.Turn(), "pet", "attack", Cmd{Target: 16})
		_, end := b.Settle()
		if end != nil {
			if end.Rewards == nil || len(end.Rewards.Pets) != 1 {
				t.Fatalf("奖励里应有捕捉的宠物: %+v", end.Rewards)
			}
			return
		}
	}
	t.Fatal("40 回合没打完")
}

// TestMonsterSkillSlot 校验用例里给出的技能栏：乌力的技能 1 是“攻击”，其余栏位空。
func TestMonsterSkillSlot(t *testing.T) {
	tmpl, ok := gamedata.PetByID(1)
	if !ok {
		t.Fatal("模板 1（乌力）不在表里")
	}
	if tmpl.Skills[0] < 0 {
		t.Fatalf("乌力技能栏 1 应有技能: %+v", tmpl.Skills)
	}
}

// STAGE-159：MaxHP 为 0 的单位算血量百分比不能除零 panic。
func TestHPPctZeroMaxHP(t *testing.T) {
	u := &Unit{HP: 0, MaxHP: 0}
	if got := u.HPPct(); got != 1 {
		t.Errorf("HPPct = %d，应为 1", got)
	}
}

// STAGE-169：人物倒下就算落败，宠物还有血也不继续打。
func TestPlayerDownEndsBattleEvenIfPetAlive(t *testing.T) {
	b := newTestBattle(t, "win", rollRnd(9999))
	pet := b.Unit(5)
	b.Unit(0).Down = true
	if pet == nil || !pet.Alive() {
		t.Fatal("宠物应还活着")
	}
	if !b.sideGone(0) {
		t.Fatal("人物倒下，己方应判落败")
	}
	b.Submit(b.Turn(), "pet", "attack", Cmd{Target: 15})
	round, end := b.Settle()
	if end == nil || end.Result != "lose" {
		t.Fatalf("end = %+v，应为 lose", end)
	}
	for _, e := range round.Events {
		if e.Actor != nil && *e.Actor == 5 {
			t.Fatalf("人物倒下后宠物不应再出手: %+v", e)
		}
	}
}
