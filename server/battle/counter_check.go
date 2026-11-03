// 反击的成败与反击率（STAGE-292 从 rules.go 拆出）：反击系数表 CounterTbl、反击率（BATTLE_CounterCalc /
// BATTLE_CounterCheckPlayer / BATTLE_CounterCheckPet）。反击的循环与结算见 counter_chain.go、actions.go。

package battle

import "math"

// counterTbl 是反击系数表（battle_event.c 的 CounterTbl，行是反击方武器类别、列是被打方）。
// 列序是原版 battle_event.c 第 2743 行左右的 enum BATTLE_C_*（NONE=0、CLAW、AXE、CLUB、SPEAR、BOW、THROU、OTHER），
// 空手走 BATTLE_C_CLAW（下标 1），所以拳×拳 = 9（评审结论，STAGE-60）。
// 人物反击时概率 = 敏捷差系数 × 表里的值 × 0.1（BATTLE_CounterCheckPlayer）；
// 宠物和怪物反击不查这张表（BATTLE_CounterCheckPet 只用敏捷差系数）。
// 投掷武器（回旋标等）两边都不反击（BATTLE_IsThrowWepon）。
var counterTbl = [7][8]int{
	//       NONE CLAW AXE CLUB SPEAR BOW THROU OTHER
	/*NONE*/ {10, 9, 8, 8, 5, 0, 0, 0},
	/*CLAW*/ {10, 9, 7, 7, 6, 0, 0, 0},
	/*AXE*/ {9, 8, 10, 10, 7, 0, 0, 0},
	/*CLUB*/ {8, 8, 10, 10, 7, 0, 0, 0},
	/*SPEAR*/ {6, 6, 8, 8, 9, 0, 0, 0},
	/*BOW*/ {0, 0, 0, 0, 0, 0, 0, 0},
	/*THROU*/ {0, 0, 0, 0, 0, 0, 0, 0},
}

// counterCheck 是反击的成败判定（BATTLE_CounterCheck）。任一边拿投掷武器两边都不反击；
// 其余见 counterPer。
func (b *Battle) counterCheck(counter, target *Unit) bool {
	per := b.counterPer(counter, target)
	if per > 100 {
		per = 100
	}
	r := float64(b.roll(10000))
	if counter.Kind == KindPlayer { // BATTLE_CounterCheckPlayer：率为 0 时不反击
		return per > 0 && r < per*100
	}
	// BATTLE_CounterCheckPet：率为 0 时最低 1/10000（原版把 per 置 1，RAND(1,10000) <= per）
	if per <= 0 {
		return r <= 1
	}
	return r <= per*100
}

// counterPer 是反击率（百分点，未封顶）。人物反击（BATTLE_CounterCheckPlayer）：
// 敏捷差系数 × CounterTbl[反击方武器][被打方武器] × 0.1 + 运气；宠物和怪物反击（BATTLE_CounterCheckPet）
// 只用敏捷差系数，再加不防守战法的反击率。
func (b *Battle) counterPer(counter, target *Unit) float64 {
	if counter.Weapon == weaponThrou || target.Weapon == weaponThrou {
		return 0 // 投掷武器两边都不反击（BATTLE_IsThrowWepon）
	}
	at, df, divpara, root := counterDex(counter, target)
	big, small, wari := spread(at, df)
	work := math.Trunc((big - small) / divpara) // 原版 Work 是 int，截断
	if work < 0 {
		work = 0
	}
	per := work
	if root {
		per = math.Sqrt(work)
	}
	per *= wari
	if counter.Kind == KindPlayer {
		return per*float64(counterTbl[counter.Weapon][target.Weapon])*0.1 + float64(counter.Luck)
	}
	if counter.Kind == KindPet {
		per += float64(counter.stance.NoGuardCounter) // 不防守战法的反击率（BATTLE_CounterCheckPet）
	}
	return per
}

// counterDex 取反击判定的敏捷与参数（BATTLE_CounterCalc 的 else-if 链）：怪物反击宠物、非人物反击人物时
// 除数换成 10、不开根号；宠物反击怪物时被打方敏捷 ×0.8；人物反击非人物时被打方敏捷 ×0.6。
// 修正后的敏捷按原版 int 截断。
func counterDex(attacker, target *Unit) (at, df, divpara float64, root bool) {
	divpara, root = gCounterPara, true
	atMod, dfMod := 1.0, 1.0
	switch {
	case attacker.Kind == KindMonster && target.Kind == KindPet:
		divpara, root = 10, false
	case attacker.Kind == KindPet && target.Kind == KindMonster:
		dfMod = 0.8
	case attacker.Kind != KindPlayer && target.Kind == KindPlayer:
		divpara, root = 10, false
	case attacker.Kind == KindPlayer && target.Kind != KindPlayer:
		dfMod = 0.6
	}
	return math.Trunc(attacker.agilityAct() * atMod), math.Trunc(target.agilityHit() * dfMod), divpara, root
}
