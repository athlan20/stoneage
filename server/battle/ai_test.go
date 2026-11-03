// 怪物 AI 的单元测试（STAGE-60）：战术串解析、模式按权重抽取、目标过滤与 rn 随机化。
// 期望值按 gmsv/battle/battle_ai.c 手算：RAND(a,b) 取闭区间，引擎的 Rnd(n) 取 [0,n)。

package battle

import "testing"

func TestParseAIOptions(t *testing.T) {
	o := parseAIOptions("at:10;1;2|gu:1|es:1|rn:0|wa:0;0;0;0;0;0;0;")
	if o.at != [3]int{10, 1, 2} || o.gu != 1 || o.es != 1 || o.rn != 0 {
		t.Fatalf("解析结果不对: %+v", o)
	}
	// 原版 rn 为空值时按键不存在处理（默认 1）。
	if o := parseAIOptions("at:1;1;1|rn:|wa:0;0;0;0;0;0;0;"); o.rn != 1 {
		t.Fatalf("rn 为空应按默认 1，实际 %d", o.rn)
	}
	// 技能槽空串按 0（原版 wa 残缺时不报错，直接跳过）。
	if o := parseAIOptions("wa:0;;0;0;0;0;3;"); o.wa != [7]int{0, 0, 0, 0, 0, 0, 3} {
		t.Fatalf("wa 空槽解析不对: %v", o.wa)
	}
}

func TestAIOptionsNone(t *testing.T) {
	// 原版：攻击、防御、精灵、逃跑和技能权重全为 0 时打印“无指定任何的攻击方式”，不出招。
	if !parseAIOptions("at:0;0;0|gu:0|es:0|wa:0;0;0;0;0;0;0;").none() {
		t.Fatal("全零战术串应判为无动作")
	}
	if !parseAIOptions("").none() {
		t.Fatal("空战术串应判为无动作")
	}
	if parseAIOptions("at:0;0;0|gu:0|es:0|wa:0;0;0;0;0;0;1;").none() {
		t.Fatal("技能槽有权重时不算无动作")
	}
}

func TestAIPickByWeight(t *testing.T) {
	// 攻击 1、防御 1、逃跑 1：区间 [0,1) 攻击，[1,2) 防御，[2,3) 逃跑。
	o := parseAIOptions("at:1;1;1|gu:1|es:1|wa:0;0;0;0;0;0;0;")
	for r, want := range map[int]string{0: "attack", 1: "guard", 2: "escape"} {
		if got, _ := o.pick(r); got != want {
			t.Errorf("pick(%d) = %s，期望 %s", r, got, want)
		}
	}
	// 攻击 0 时不会抽到攻击（原版 at[0] != 0 才判）；技能槽 2 的权重 2 落在 [0,2)。
	o = parseAIOptions("at:0;0;0|gu:0|es:0|wa:0;2;0;0;0;0;0;")
	if mode, slot := o.pick(0); mode != "skill" || slot != 1 {
		t.Errorf("pick(0) = %s/%d，期望 skill/1", mode, slot)
	}
	if mode, slot := o.pick(1); mode != "skill" || slot != 1 {
		t.Errorf("pick(1) = %s/%d，期望 skill/1", mode, slot)
	}
}

// aiTestBattle 开一场野外战（人物 + 乌力宠物 对 两只怪），返回战斗和第一只怪。
func aiTestBattle(t *testing.T, rnd func(n int) int) (*Battle, *Unit) {
	t.Helper()
	b := newTestBattle(t, "win", rnd)
	for _, u := range b.units {
		if u != nil && u.Kind == KindMonster {
			return b, u
		}
	}
	t.Fatal("没有怪物单位")
	return nil, nil
}

func TestAIEmptyTacticsDoesNothing(t *testing.T) {
	b, mon := aiTestBattle(t, rollRnd(0))
	if _, ok := parseAIOptions("").decide(b, mon); ok {
		t.Fatal("战术串为空时怪物不应出招")
	}
}

func TestAIRandomAttackPicksFromTargets(t *testing.T) {
	b, mon := aiTestBattle(t, rollRnd(0))
	act, ok := parseAIOptions("at:1;1;1|gu:0|es:0|wa:0;0;0;0;0;0;0;").decide(b, mon)
	if !ok || act.kind != "attack" {
		t.Fatalf("普攻应出招: %+v ok=%v", act, ok)
	}
	if tgt := b.Unit(act.target); tgt == nil || tgt.Side() == mon.Side() {
		t.Fatalf("目标应是人物一方: %+v", act)
	}
}

func TestAITargetFilterFallsBackToAll(t *testing.T) {
	b, mon := aiTestBattle(t, rollRnd(0))
	// 只打宠物但宠物已经倒下：原版 cnt==0 时把过滤放宽成全部（at[1] = ALL 再循环）。
	b.PetUnit().Down = true
	act, ok := parseAIOptions("at:1;3;1|gu:0|es:0|wa:0;0;0;0;0;0;0;").decide(b, mon)
	if !ok || act.target != b.PlayerUnit().No {
		t.Fatalf("宠物倒下后应退回打人物: %+v ok=%v", act, ok)
	}
	// 只打人物但人物已经倒下、宠物还在：退回全部，打宠物。
	b.PetUnit().Down = false
	b.PlayerUnit().Down = true
	act, ok = parseAIOptions("at:1;2;1|gu:0|es:0|wa:0;0;0;0;0;0;0;").decide(b, mon)
	if !ok || act.target != b.PetUnit().No {
		t.Fatalf("人物倒下后应退回打宠物: %+v ok=%v", act, ok)
	}
}

func TestAIRnZeroAlwaysRandom(t *testing.T) {
	// 血最多选目标：rn 为 0 时 !RAND(0,0) 恒真，每次都随机挑（原版 _ENEMY_ATTACK_AI）。
	// rnd 对 n==6 出 1、其余出 0：rn=5 时 RAND(0,5) 出 1，不随机，按血最多选；
	// rn=0 时 RAND(0,0) 出 0，随机挑，RAND(0,1) 出 0，挑第一个。
	b, mon := aiTestBattle(t, func(n int) int {
		if n == 6 {
			return 1
		}
		return 0
	})
	b.PlayerUnit().HP, b.PetUnit().HP = 1, 999
	targets := b.aiTargets(mon, aiTargetAll)
	if len(targets) < 2 {
		t.Fatalf("测试需要至少两个目标，实际 %d", len(targets))
	}

	act, _ := parseAIOptions("at:1;1;2|gu:0|es:0|rn:5|wa:0;0;0;0;0;0;0;").decide(b, mon)
	if act.target != b.PetUnit().No {
		t.Errorf("rn=5 应按血最多选宠物，实际目标 %d", act.target)
	}
	act, _ = parseAIOptions("at:1;1;2|gu:0|es:0|rn:0|wa:0;0;0;0;0;0;0;").decide(b, mon)
	if act.target != targets[0].No {
		t.Errorf("rn=0 应随机挑第一个目标 %d，实际 %d", targets[0].No, act.target)
	}
}
