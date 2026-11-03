// 出手表现用的武器身份（STAGE-167，docs/battle-ui.md「九种基础攻击形态」）。
//
// 和反击系数表用的 weaponClass（unit.go）不是一回事：那一张要合并同类项来查 CounterTbl，
// 这一张要能区分「投掷斧 / 投掷石 / 回旋镖」三种轨迹完全不同的武器，长矛也要单独认出来。
// 客户端只认这里的取值挑动作，不按物品名或伤害值猜。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// 武器身份，取值直接写进协议的 `weapon` 字段。
const (
	motionFist       = "fist"        // 空手、拳套
	motionAxe        = "axe"         // 斧（近战劈砍）
	motionClub       = "club"        // 棍棒（近战钝击）
	motionSpear      = "spear"       // 长矛（近战直刺）
	motionClaw       = "claw"        // 爪（多段划击）
	motionBow        = "bow"         // 弓（原地放箭）
	motionThrowAxe   = "throw_axe"   // 投掷斧头（原地抛出）
	motionThrowStone = "throw_stone" // 投掷石（原地抛出）
	motionBoomerang  = "boomerang"   // 回旋镖（横扫一排再回手）
)

// weaponMotion 把物品类型（itemset.txt 的 ItemType）换成出手表现用的武器身份（STAGE-167）。
//
// 和 weaponClassOf 的差别：长矛（ItemTypeSpear）在这里单独认（反击表里它落回 NONE，是原版没写映射）；
// 投掷的三类在这里分开。认不出来的物品类型一律按空手（fist）——中性反馈，但仍然保留真实来源与结果。
func weaponMotion(itemType int) string {
	switch itemType {
	case gamedata.ItemTypeAxe:
		return motionAxe
	case gamedata.ItemTypeClub:
		return motionClub
	case gamedata.ItemTypeSpear:
		return motionSpear
	case gamedata.ItemTypeBow:
		return motionBow
	case gamedata.ItemTypeBoomerang:
		return motionBoomerang
	case gamedata.ItemTypeBoundThrow:
		return motionThrowAxe
	case gamedata.ItemTypeBreakThrow:
		return motionThrowStone
	}
	return motionFist
}

// motionOf 是手里武器的表现身份。宠物没有装备，按空手（爪咬/扑撞由客户端按 kind 细分）。
func (u *Unit) motionOf() string {
	if u.WeaponMotion == "" {
		return motionFist
	}
	return u.WeaponMotion
}

// isBoomerang 判断这次普攻是不是回旋镖（要横扫一排）。
func (u *Unit) isBoomerang() bool {
	return u.motionOf() == motionBoomerang
}

// rowOf 把站位编号换成横排号（STAGE-167）：每方 10 格，0–4 是人物排、5–9 是宠物排，
// 整场 20 格共 4 排。原版 BATTLE_COM_BOOMERANG 就是先 `defNo /= 5` 定排。
func rowOf(no int) int { return no / 5 }

// boomerangRow 是回旋镖扫过一排的站位顺序（原版 battle.c 的 BoomerangVsTbl[4][5]）。
//
// 表里存的是「视觉上从右往左」的偏移序 {4,2,0,1,3}；出手方不同，遍历方向相反，
// 所以同一个视觉扫法在编号上表现为不同的顺序：
//   - 我方（side 0）从 k=0 往 4 走；
//   - 敌方（side 1）从 k=4 往 0 走。
//
// 这给出的固定回归样例：我方打敌宠排是 19→17→15→16→18、我方打敌人物排是 14→12→10→11→13；
// 敌方打我宠排是 8→6→5→7→9。不能在所有阵型里无条件照抄这些数字，必须按 row 与 side 现算。
func boomerangRow(row, side int) []int {
	base := [5]int{4, 2, 0, 1, 3}
	out := make([]int, 0, 5)
	if side == 1 {
		for k := 4; k >= 0; k-- {
			out = append(out, row*5+base[k])
		}
		return out
	}
	for k := 0; k < 5; k++ {
		out = append(out, row*5+base[k])
	}
	return out
}
