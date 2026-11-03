// 战斗胜利的奖励（经验、金币、掉落、捕获的宠物）与经验倍数（STAGE-60 起按原版 gmsv/battle/battle.c 的胜利结算）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// rewards 汇总胜利的奖励（STAGE-60 起按原版）：
//   - 经验：每个怪按遇敌表的值（-1 按公式，STAGE-51 的 ENEMY_getExp），每个参战单位
//     （人物、出战宠物）各自按自己的等级算：比怪高 5 级以上开始递减（battle.c 胜利结算的
//     EXPGET_MAXLEVEL/DIV），最低 1；乘上全服经验倍数（STAGE-59，金币等其他奖励不受它影响）；
//     骑乘的宠物另走 rideExp（× 0.6）；
//   - 金币：人物等级 × BATTLEGOLD（setup.cf=10），补满不超过持有上限
//     （上限见 battleGoldGain）；
//   - 掉落：怪物出生时摇好的物品（enemy.c），最多 3 件（GETITEM_MAX），第 4 件起
//     一半概率换掉随机一件；背包满时的丢弃在 character 服务做（BATTLE_GetExpGold）。
//
// 升级的等级变化由 character 服务算好填回来（applyResult 的 levelUp）。
func (b *Battle) rewards() *Rewards {
	r := &Rewards{}
	player := b.PlayerUnit()
	if player != nil {
		r.Exp = append(r.Exp, RewardExp{Who: "player", Name: player.Name, Gain: b.unitExp(player),
			LevelFrom: player.Level, LevelTo: player.Level})
		if pet := b.PetUnit(); pet != nil { // STAGE-244：出战宠物按它自己的等级算，不复用人物那份
			r.Exp = append(r.Exp, RewardExp{Who: "pet", Slot: intPtr(pet.PetSlot), Name: pet.Name, Gain: b.unitExp(pet),
				LevelFrom: pet.Level, LevelTo: pet.Level})
		}
		if rider := b.RiderUnit(); rider != nil { // 骑着的宠物按自己的等级算、拿 0.6 倍（ride.go）
			r.Exp = append(r.Exp, RewardExp{Who: "pet", Slot: intPtr(rider.PetSlot), Name: rider.Name, Gain: b.rideExp(rider),
				LevelFrom: rider.Level, LevelTo: rider.Level})
		}
		r.Gold = battleGoldGain(player.Level, b.sides[0].Gold)
		r.Gold = b.settleStolenGold(r.Gold)
	}
	r.PetKills = b.petKills()
	for _, id := range distributeDrops(b.slain, b.Rnd) {
		name := ""
		if def, ok := gamedata.ItemByID(id); ok {
			name = def.Name
		}
		r.Items = append(r.Items, RewardItem{ItemID: id, Name: name, Count: 1})
	}
	for _, m := range b.captur {
		r.Pets = append(r.Pets, RewardPet{Slot: m.PetSlot, Name: m.Name, Level: m.Level})
	}
	return r
}

// unitExp 是参战单位 u 这场战斗拿到的经验（STAGE-244）：每只被击杀的怪都按 u 自己的等级算等级差
// （battle.c 胜利结算对每个参战单位各算一遍 b_level = CHAR_LV - enemylevel），求和后乘上全服倍数。
// 骑乘宠物在此基础上再乘 0.6，见 ride.go 的 rideExp。
func (b *Battle) unitExp(u *Unit) int64 {
	var base int64
	for _, m := range b.slain {
		base += levelDiffExp(u.Level, m.Level, killExp(m))
	}
	return applyExpRate(base, b.expRate())
}

// levelDiffExp 是等级差的Experience缩水（battle.c 胜利结算：人物比怪高 5 级以内全额，
// 每多 1 级少 1/15，最低 1）。
func levelDiffExp(playerLV, enemyLV int, exp int64) int64 {
	diff := playerLV - enemyLV
	if diff <= 5 {
		return exp
	}
	factor := 5 + 15 - diff
	if factor > 15 {
		factor = 15
	}
	if factor <= 0 {
		return 1
	}
	gain := exp * int64(factor) / 15
	if gain < 1 {
		return 1
	}
	return gain
}

// killExp 是击杀一只怪的基础经验：enemy.txt 写死了就用写死的，否则按公式（STAGE-51）。
func killExp(m *Unit) int64 {
	if m.EnemyExp >= 0 {
		return int64(m.EnemyExp)
	}
	return int64(gamedata.EnemyExp(m.Level, m.Rank))
}

// MaxHaveGold 是角色携带石币的上限：统一 1000 万，固定值，不随转生变化（docs/trade.md 第 6 条已拍板）。
// 商店、交易、战斗拾取和偷取、管理命令发石币都用这一个常量；原版 CHAR_getMaxHaveGold 的
// _FIX_MAX_GOLD 公式（1000000 + 转生 × 1800000）不再使用。
const MaxHaveGold int64 = 10_000_000

// battleGoldGain 是战斗胜利的金币增量：人物等级 × BATTLEGOLD（setup.cf 的 10）；
// 超过持有上限时直接设为上限（battle.c 的 _BATTLE_GOLD 分支：gold = MaxHaveGold），不进银行。
// 原版 _CHAR_AddGold（其他途径加金币）溢出的部分会进个人银行（CHAR_PERSONAGOLD，上限 5000 万），
// 新服务端还没有银行，这里只截断。
func battleGoldGain(level int, gold int64) int64 {
	gain := int64(level) * gamedata.BattleGold
	if gold+gain > MaxHaveGold {
		return MaxHaveGold - gold
	}
	return gain
}

// distributeDrops 把怪物身上摇好的掉落分给胜利的玩家（一期只有一个玩家）：
// 最多带 3 件（GETITEM_MAX），第 4 件起一半概率换掉随机一件（battle.c 的战利品分配）。
func distributeDrops(slain []*Unit, rnd func(n int) int) []int {
	slots := [3]int{-1, -1, -1}
	for _, m := range slain {
		for _, id := range m.Drops {
			placed := false
			for i := range slots {
				if slots[i] == -1 {
					slots[i] = id
					placed = true
					break
				}
			}
			if !placed && rnd(2) == 1 {
				slots[rnd(3)] = id
			}
		}
	}
	out := make([]int, 0, len(slots))
	for _, id := range slots {
		if id != -1 {
			out = append(out, id)
		}
	}
	return out
}

// expRate 返回这场战斗结算用的经验倍数：回调没接（测试直接用引擎）或值非正数时按 1 倍。
func (b *Battle) expRate() float64 {
	if b.Cbs.ExpRate == nil {
		return 1
	}
	if r := b.Cbs.ExpRate(); r > 0 {
		return r
	}
	return 1
}

// applyExpRate 把基础经验乘上全服倍数：小数部分舍去；基础经验大于 0 时至少给 1 点
// （倍数最低 0.1，打低级怪才不至于白打）。
func applyExpRate(base int64, rate float64) int64 {
	g := int64(float64(base) * rate)
	if base > 0 && g < 1 {
		g = 1
	}
	if g < 0 {
		g = 0
	}
	return g
}
