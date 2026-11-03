package gamedata

// 魅力与运气（STAGE-61，出处：gmsv/char/char.c 的 CHAR_setLuck、CHAR_AddCharm，item/item.c 的 ITEM_equipEffect）。
// 魅力是存下来的基础值（新角色 60，0–100），战斗中的增减直接改它；
// 运气是每个游戏日第一次进入世界时随机的一档（1–5），同一天内重登不变。
// 装备的魅力、运气修正加在基础值上，结果夹在各自的范围内（ITEM_equipEffect 的 min/max）。

const (
	CharmMin = 0   // 魅力下限
	CharmMax = 100 // 魅力上限
	LuckMin  = 1   // 运气（有效值）下限，ITEM_equipEffect 对 WORKFIXLUCK 的 min
	LuckMax  = 5   // 运气（有效值）上限
)

// NewPlayerCharm 是新角色的魅力基础值（出处：gmsv/char/char.c 的 CHAR_createNewChar）。
const NewPlayerCharm = 60

// 战斗带来的魅力增减（battle.c 的 CH_FIX_PLAYERLEVELUP、CH_FIX_PLAYERDEAD、CH_FIX_PLAYEULTIMATE）。
// 倒下时还要按等级减半（见 character 包的 charmOnDeath）；逃跑、捕捉不改魅力。
const (
	CharmOnLevelUp = 2  // 人物升级（战斗胜利获得经验后，升了至少一级，只加一次）
	CharmOnDeath   = -2 // 人物在野外战斗中被打倒
	CharmOnFly     = -4 // 人物在野外战斗中被打飞（STAGE-63，替代普通被打倒的 -2）
)

// luckTable 是每日运气的档位与概率（char.c 的 lucktbl，注释里的 40%、30%……）：
// 掷一个 0–99 的数 r，第一个满足 r >= prob 的档位生效。
var luckTable = [...]struct{ prob, luck int }{
	{60, 1}, // 40%
	{30, 2}, // 30%
	{10, 3}, // 20%
	{3, 4},  // 7%
	{0, 5},  // 3%
}

// RollLuck 由 0–99 的随机数 r 得出当日运气（1–5 档）。随机源由调用方给，便于测试固定。
func RollLuck(r int) int {
	for _, t := range luckTable {
		if r >= t.prob {
			return t.luck
		}
	}
	return LuckMax // r 为负数时不会走到这里（luckTable 最后一档的 prob 是 0）
}

// EffectiveCharm 是魅力的有效值：基础值加穿着的装备修正，夹在 0–100（CHAR_AddCharm 与 ITEM_equipEffect）。
func EffectiveCharm(base, equip int) int {
	return min(max(base+equip, CharmMin), CharmMax)
}

// ClampCharm 把魅力基础值夹在 0–100；战斗中的增减每一步都夹一次（CHAR_AddCharm）。
func ClampCharm(v int) int {
	return min(max(v, CharmMin), CharmMax)
}

// EffectiveLuck 是运气的有效值：基础值加穿着的装备修正，夹在 1–5（ITEM_equipEffect 对 WORKFIXLUCK 的限制）。
// 基础值还没掷过（0）时也落在 1，战斗公式读到的总是 1–5。
func EffectiveLuck(base, equip int) int {
	return min(max(base+equip, LuckMin), LuckMax)
}
