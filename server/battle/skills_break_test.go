// 破除防御与落马术的测试（STAGE-291 批 A）：对照原版 BATTLE_S_GBreak、BATTLE_S_FallGround。
package battle

import "testing"

// 破除防御打没防御的怪：伤害 0，显示没打中（原版 BATTLE_RET_MISS）。
func TestGuardBreakMissesUnguardedTarget(t *testing.T) {
	b := skillTestBattle(t, 3)
	bigHP(b, 15)
	ev, ok := petEvent(playTurn(t, b, 15))
	if !ok {
		t.Fatal("破除防御没有宠物事件")
	}
	if damageTo(ev, 15) != 0 {
		t.Errorf("没防御的怪受伤 %d，want 0", damageTo(ev, 15))
	}
	if !hasResult(ev, 15, "miss") {
		t.Errorf("没防御的怪应显示没打中，效果 %+v", ev.Effects)
	}
}

// 破除防御打防御中的怪：无视防御，伤害不低于同样的普攻打在防御中的怪（普攻要吃防御减伤）。
func TestGuardBreakIgnoresGuard(t *testing.T) {
	normal := skillTestBattle(t, 1)
	bigHP(normal, 15)
	normal.Unit(15).Status.Guard = true
	nEv, _ := petEvent(playTurn(t, normal, 15))

	brk := skillTestBattle(t, 3)
	bigHP(brk, 15)
	brk.Unit(15).guardRound = true
	bEv := brk.guardBreak(brk.PetUnit(), 15, "破除防御") // 直接结算：回合开始会按计划重算防御指令

	if got, base := damageTo(bEv, 15), damageTo(nEv, 15); got <= base {
		t.Errorf("破防打防御中的怪 %d，不高于普攻的 %d", got, base)
	}
}

// 落马术是攻击力 −30% 的一次攻击：伤害比同样的普攻低。
func TestFallGroundWeakerThanNormal(t *testing.T) {
	normal := skillTestBattle(t, 1)
	bigHP(normal, 15)
	nEv, _ := petEvent(playTurn(t, normal, 15))

	fall := skillTestBattle(t, 210)
	bigHP(fall, 15)
	fEv, ok := petEvent(playTurn(t, fall, 15))
	if !ok {
		t.Fatal("落马术没有宠物事件")
	}
	if got, base := damageTo(fEv, 15), damageTo(nEv, 15); got <= 0 || got >= base {
		t.Errorf("落马术伤害 %d，want 大于 0 且小于普攻的 %d", got, base)
	}
}

// hasResult 判断事件里目标 no 的效果是否是 result。
func hasResult(ev Event, no int, result string) bool {
	for _, e := range ev.Effects {
		if e.Target == no && e.Result == result {
			return true
		}
	}
	return false
}
