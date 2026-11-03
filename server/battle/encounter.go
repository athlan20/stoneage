// 野外遇敌战斗的生成（STAGE-40）：按遇敌区域抽敌群、按敌群抽敌人，
// 算法照 gmsv/char/enemy.c 的 ENEMY_getEnemy（含 ENEMY_RandomEnemyArray 的替换）
// 逐步移植；怪物的等级和属性公式与捕捉、剧本共用 NewMonster。
// 与原版的一处偏差：区域引用的敌群在表里缺失时（如区域 21 的 1230），
// 原版抽中它的一场会“没有遇到”，这里直接把它从抽签里剔掉，让权重集中在存在的敌群上。

package battle

import (
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
)

// WildSpec 是一场野外遇敌的输入：站在哪个遇敌区域，以及背包里的物品编号
// （敌群的“持有才出现 / 持有不出现”道具条件用，同原版 CHAR_getItemIndex 的遍历）。
type WildSpec struct {
	Area      *gamedata.EncounterArea
	HeldItems map[int]bool
}

// 挑好的一个敌人槽位：敌群里的敌人（已过 RandomEnemy 替换）和它的生成权重。
type wildSlot struct {
	variant *gamedata.EnemyVariant
	weight  int
}

// BuildWild 按遇敌表开一场野外战斗。rnd(n) 返回 [0, n) 的随机整数。
// 抽不出敌群或敌人（比如区域是空敌群，村子里就是这种行）时返回 false。
func BuildWild(id, scene string, side *SideInput, spec *WildSpec, rnd func(n int) int) (*Battle, bool) {
	slots, ok := wildEnemySlots(spec, rnd)
	if !ok {
		return nil, false
	}
	monsters := wildMonsters(slots, rnd)
	if len(monsters) == 0 {
		return nil, false
	}
	b := New(id, scene, side, monsters, rnd)
	b.Combo = true // STAGE-126：真实遇敌有合击
	return b, true
}

// wildMonsters 把抽好的敌人按模板造成怪物单位（单人野外和真人同场共用）。
func wildMonsters(slots []*gamedata.EnemyVariant, rnd func(n int) int) []*Unit {
	monsters := make([]*Unit, 0, len(slots))
	for _, e := range slots {
		t, ok := gamedata.PetByID(e.Template)
		if !ok {
			// 数据问题：enemy.txt 引用了 enemybase.txt 里没有的模板。跳过这只，
			// 让这场照常开起来（原版 ENEMYTEMP_CHECKINDEX 不通过时这一只不入场）。
			continue
		}
		monsters = append(monsters, NewMonster(t, enemyLevel(e, rnd), -1, rnd, e))
	}
	return monsters
}

// wildEnemySlots 抽出这一场的敌人列表：先抽敌群（按区域权重），再抽每个敌人
// （按敌群权重和同场上限）。对应 ENEMY_getEnemy 的两段抽签。
func wildEnemySlots(spec *WildSpec, rnd func(n int) int) ([]*gamedata.EnemyVariant, bool) {
	// 第一段：抽敌群。道具条件不满足或表里缺失的敌群不参加抽签。
	type groupSlot struct {
		group  *gamedata.EnemyGroup
		weight int
	}
	var groups []groupSlot
	total := 0
	for _, ref := range spec.Area.Groups {
		if ref.ID == -1 {
			continue
		}
		g, ok := gamedata.EnemyGroupByID(ref.ID)
		if !ok {
			continue
		}
		if g.AppearItem != -1 && !spec.HeldItems[g.AppearItem] {
			continue
		}
		if g.NotAppearItem != -1 && spec.HeldItems[g.NotAppearItem] {
			continue
		}
		groups = append(groups, groupSlot{group: g, weight: ref.Weight})
		total += ref.Weight
	}
	if len(groups) == 0 {
		return nil, false
	}
	picked := groups[weightedPick(groups, func(g groupSlot) int { return g.weight }, total, rnd)].group

	// 第二段：收集敌群里的敌人槽位（编号 945–956、964–969 的先按 RandomEnemyTbl 换成随机候选），
	// 同场敌人总数不超过各槽位 CREATEMAXNUM 之和。
	var slots []wildSlot
	createNum := 0
	for _, ref := range picked.Enemies {
		if ref.ID == -1 {
			continue
		}
		e, ok := gamedata.EnemyByID(ref.ID)
		if !ok {
			continue
		}
		if len(e.RandomOf) > 0 {
			if cand, ok := gamedata.EnemyByID(e.RandomOf[rnd(len(e.RandomOf))]); ok {
				e = cand
			}
		}
		slots = append(slots, wildSlot{variant: e, weight: ref.Weight})
		createNum += e.CreateMax
	}
	if len(slots) == 0 {
		return nil, false
	}
	entryMax := spec.Area.MaxEnemies
	if createNum < entryMax {
		entryMax = createNum
	}
	if entryMax < 1 {
		entryMax = 1
	}
	count := 1 + rnd(entryMax) // RAND(1, entrymax)

	// 逐个抽敌人。同一敌人受 CREATEMAXNUM×同名槽位数封顶；大型敌人（模板 SIZE=1）
	// 全场最多 5 只，且第 6 个起要和前排（前 5 个里的普通体型）换位（ENEMY_getEnemy 的收尾循环）。
	out := make([]*gamedata.EnemyVariant, 0, count)
	bigCnt := 0
	sameSlots := map[int]int{} // 敌人编号 → 敌群里解析到它的槽位数
	for _, s := range slots {
		sameSlots[s.variant.ID]++
	}
	totalWeights := sumWeights(slots)
	for i, loops := 0, 0; i < count && loops < 100; loops++ {
		e := slots[weightedPick(slots, func(s wildSlot) int { return s.weight }, totalWeights, rnd)].variant
		same := 0
		for _, prev := range out {
			if prev.ID == e.ID {
				same++
			}
		}
		if same >= e.CreateMax*sameSlots[e.ID] {
			continue
		}
		var res seatResult
		out, res = placeEnemy(out, e, &bigCnt)
		switch res {
		case seatFull:
			count-- // 大型已满 5 只：这一只不出，目标数减一
			continue
		case seatNoSwap:
			continue
		}
		i++
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// weightedPick 按权重抽一项：r 在 [0, total) 里均匀取，从头累加权重，
// 权重非 0 且 r 落进累计区间时选中；一路都没中时取最后一项（RAND 累加命中的原版行为，
// 零权重的末项也可能因此被抽中）。total 是各项权重之和；全为零时取最后一项。
func weightedPick[T any](items []T, weight func(T) int, total int, rnd func(n int) int) int {
	if total <= 0 {
		return len(items) - 1
	}
	r := rnd(total)
	acc := 0
	for i := 0; i < len(items)-1; i++ {
		acc += weight(items[i])
		if weight(items[i]) != 0 && r < acc {
			return i
		}
	}
	return len(items) - 1
}

func sumWeights(slots []wildSlot) int {
	total := 0
	for _, s := range slots {
		total += s.weight
	}
	return total
}

// EncounterScene 返回野外战斗的场景名：优先用地图名（战场标题用），没有地图名的写“野外”。
func EncounterScene(floor int) string {
	if name, ok := gamedata.MapName(floor); ok && name != "" {
		return name
	}
	return fmt.Sprintf("野外")
}
