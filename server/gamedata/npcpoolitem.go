package gamedata

// 道具仓库（STAGE-309，原版 npc_poolitemshop.c，docs/protocol.md 5.60）的数值。
// 寄存费每件 50 石币（原版 pis_* 参数 cost:50，每次寄存一件，按件收费）；
// 格数：道具仓库固定 50 格（STAGE-347，原版随转生 min(转生×4+10, 30) 的规则不再沿用），
// 宠物寄放池 min(转生×2+5, 15)（原版 char_base.c）。

// PoolItemCost 是每件寄存的费用。
const PoolItemCost = 50

// MaxPoolItems 是道具仓库的格数（STAGE-347：固定 50 格，不随转生变化；数据库 character_pool_items.slot 允许 0–49）。
const MaxPoolItems = 50

// PoolItemLimit 返回道具仓库可用的格数。STAGE-347 起固定 50 格，所有角色（含已转生的老角色）
// 都一样；参数 tensei 保留是为了调用处不必改，也不再影响结果。
func PoolItemLimit(tensei int) int {
	return MaxPoolItems
}

// PoolPetLimit 返回转生次数为 tensei 时宠物寄放池可用的格数（宠物店和原版一致的公式）。
func PoolPetLimit(tensei int) int {
	return clampPoolLimit(tensei*2+5, MaxPoolPets)
}

func clampPoolLimit(n, max int) int {
	if n < 0 {
		return 0
	}
	if n > max {
		return max
	}
	return n
}
