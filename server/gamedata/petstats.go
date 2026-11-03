package gamedata

// 宠物的资质、能力与衍生数值（STAGE-31，docs/ui.md 宠物页）。
// 出处：gmsv/char/enemy.c 的 ENEMY_createPetFromEnemyIndex、ENEMY_getRank，
// gmsv/char/char.c 的 CHAR_initcharWorkInt（宠物忠诚分支）与 CHAR_complianceParameter（最大 HP）。
//
// 单位：宠物的能力按原版服务端的做法存成“点数 × 100”（1/100 点），下面的 Ability 也是这个单位；
// 显示时舍去小数，所以攻击、防御等函数直接返回整数。

// MaxUpLevel 是转生主人的忠诚加成用的等级上限（gmsv/include/char_base.h 的 CHAR_MAXUPLEVEL）。
const MaxUpLevel = 140

// NewPetLevel 是 stoneage-admin givepet 不写等级时的等级（gmsv/setup.cf 的 PETLV=1）。
const NewPetLevel = 1

// Growth 是四项资质：体力、腕力、耐久力、速度。模板里是基准值，生成宠物时在它上面随机。
type Growth struct {
	Vital, Strength, Toughness, Dexterity int
}

// Ability 是宠物的四项能力，单位 1/100 点。
type Ability struct {
	Vital, Strength, Toughness, Dexterity int
}

// NewPetGrowth 按 ENEMY_createPetFromEnemyIndex 的顺序生成一只宠物的资质和能力。
// rnd(n) 返回 [0, n) 的随机整数，测试时可以换成固定的序列。
//
// 资质：模板值每项随机加 -2 到 +2，结果是 base（原版服务端存进 CHAR_ALLOCPOINT，之后升级分配点数用）；
// 然后随机给四项里的某一项加 1，重复 10 次，得到计算能力用的资质。
// 能力 = ((等级 − 1) × LVUPPOINT + INITNUM) × 计算用的资质（1/100 点）。
func NewPetGrowth(t PetTemplate, level int, rnd func(n int) int) (base Growth, ability Ability) {
	g := Growth{
		Vital:     t.Base.Vital + rnd(5) - 2,
		Strength:  t.Base.Strength + rnd(5) - 2,
		Toughness: t.Base.Toughness + rnd(5) - 2,
		Dexterity: t.Base.Dexterity + rnd(5) - 2,
	}
	base = g
	for i := 0; i < 10; i++ {
		switch rnd(4) {
		case 0:
			g.Vital++
		case 1:
			g.Strength++
		case 2:
			g.Toughness++
		case 3:
			g.Dexterity++
		}
	}
	perLevel := (level-1)*t.LvUpPoint + t.InitNum
	ability = Ability{
		Vital:     perLevel * g.Vital,
		Strength:  perLevel * g.Strength,
		Toughness: perLevel * g.Toughness,
		Dexterity: perLevel * g.Dexterity,
	}
	return base, ability
}

// PetLevelCap 是没有填等级上限的宠物能升到的最高级。原版开了 _USER_EXP_CF：宠物超过“一般等级”
// （gmsv/setup.cf 的 LEVEL=140）还要升，得满足宠物转生条件（PETTRANS=-1 时看 CHAR_LIMITLEVEL），
// 所以普通宠物停在 140 级。
const PetLevelCap = 140

// PetMaxLevel 是这种宠物的等级上限：模板填了 LIMITLEVEL 的按它（到了上限经验清零，
// CHAR_LevelUpCheck），否则 PetLevelCap。
func PetMaxLevel(t PetTemplate) int {
	return PetMaxLevelFor(t.LimitLevel)
}

// PetMaxLevelFor 按宠物的有效等级上限（模板的或个体的，见 character 层的 petLimit）算最高级；0 为没有上限。
func PetMaxLevelFor(limit int) int {
	if limit > 0 {
		return limit
	}
	return PetLevelCap
}

// petGrowthRate 是升级成长系数的区间（百分比，含两端），下标是品质 0–5（CHAR_PetLevelUp 的 RankRandTbl）。
// 品质数值越大（资质越差）系数反而越高，原版如此。
var petGrowthRate = [...][2]int{{450, 500}, {470, 520}, {490, 540}, {510, 560}, {530, 580}, {550, 600}}

// PetLevelUp 是宠物升一级时四项能力的增量（1/100 点），照 CHAR_PetLevelUp 的顺序：
// 先把 10 个随机点逐个随机分到四项上，再在品质对应的区间里随机一个成长系数，
// 每项增量 = 资质 × 系数 + 随机点 × 系数，截成整数。base 是生成时保存的资质（CHAR_ALLOCPOINT）。
// 原版用 float 运算后截断，这里同样用 float32，避免边界值差 1。
func PetLevelUp(base Growth, rank int, rnd func(n int) int) Ability {
	var extra [4]float32
	for i := 0; i < 10; i++ {
		extra[rnd(4)]++
	}
	if rank < 0 || rank >= len(petGrowthRate) {
		rank = 0
	}
	lo, hi := petGrowthRate[rank][0], petGrowthRate[rank][1]
	rate := float32(float64(lo+rnd(hi-lo+1)) * 0.01)
	grow := func(b int, e float32) int { return int(float32(b)*rate + e*rate) }
	return Ability{
		Vital:     grow(base.Vital, extra[0]),
		Strength:  grow(base.Strength, extra[1]),
		Toughness: grow(base.Toughness, extra[2]),
		Dexterity: grow(base.Dexterity, extra[3]),
	}
}

// Add 返回两组能力逐项相加的结果。
func (a Ability) Add(b Ability) Ability {
	return Ability{a.Vital + b.Vital, a.Strength + b.Strength, a.Toughness + b.Toughness, a.Dexterity + b.Dexterity}
}

// PetRank 是宠物的品质（0–5，CHAR_PETRANK）。ENEMY_getRank：模板四项资质之和越高，品质越小（越好）。
// 合计 ≥ 100 为 0，≥ 95 为 1，≥ 90 为 2，≥ 85 为 3，≥ 80 为 4，其余为 5。
func PetRank(t PetTemplate) int {
	thresholds := [...]int{100, 95, 90, 85, 80, 0}
	sum := t.Base.Vital + t.Base.Strength + t.Base.Toughness + t.Base.Dexterity
	for i, min := range thresholds {
		if sum >= min {
			return i
		}
	}
	return 0
}

// PetAttack 是宠物的攻击：腕力 + 耐久力×0.1 + 体力×0.1 + 速度×0.05（点数）。
// 能力是 1/100 点，所以这里换成整数运算：(100·腕力 + 10·耐久 + 10·体力 + 5·速度) ÷ 10000，结果截成整数。
func PetAttack(a Ability) int {
	return (100*a.Strength + 10*a.Toughness + 10*a.Vital + 5*a.Dexterity) / 10000
}

// PetDefense 是宠物的防御：耐久力 + 腕力×0.1 + 体力×0.1 + 速度×0.05（点数，算法同 PetAttack）。
func PetDefense(a Ability) int {
	return (100*a.Toughness + 10*a.Strength + 10*a.Vital + 5*a.Dexterity) / 10000
}

// PetAgility 是宠物的敏捷：等于速度（点数）。
func PetAgility(a Ability) int { return a.Dexterity / 100 }

// PetMaxHP 是宠物的最大生命值：(体力×4 + 腕力 + 耐久力 + 速度) 按点数计，结果截成整数。
// STAGE-159：至少 1 点——坏数据（能力被写成 0）算出 0 会让战斗里的血量百分比除零。
func PetMaxHP(a Ability) int {
	return max(1, (4*a.Vital+a.Strength+a.Toughness+a.Dexterity)/100)
}

// 忠诚的可变部分（CHAR_VARIABLEAI，1/100 点）：宠物每升一级加 5 点（battle.h 的 AI_FIX_PETLEVELUP），
// 范围 ±100 点（char_base.h 的 CHAR_MAXVARIABLEAI / CHAR_MINVARIABLEAI）。
const (
	PetLevelUpAI  = 5 * 100
	MaxVariableAI = 100 * 100
	MinVariableAI = -100 * 100
)

// AddVariableAI 给忠诚的可变部分加上 delta 并限制在范围内（CHAR_PetAddVariableAi）。
func AddVariableAI(v, delta int) int { return clamp(v+delta, MinVariableAI, MaxVariableAI) }

// Loyalty 是宠物的忠诚（0–100），由服务端每次现算，不存（docs/ui.md 宠物页）。
// 公式按 CHAR_initcharWorkInt 的顺序：主人等级 × 主人魅力 × 1.1 ÷ (宠物等级 × MODAI) × 100，
// 截成整数后不超过 100；加上可变部分 × 0.01；主人转生过的再加 (140 − 主人等级) ÷ 2；最后限制在 0–100。
// 浮点运算的顺序和原版服务端一致，避免边界值差 1。
func Loyalty(hostLevel, hostCharm, hostTransmigration, petLevel, modAI, variable int) int {
	base, bonus := LoyaltyBase(hostLevel, hostCharm, hostTransmigration, petLevel, modAI)
	return LoyaltyWith(base, bonus, variable)
}

// LoyaltyBase 是忠诚里不随可变部分变化的两项：基础值（截成整数后不超过 100）和转生加成
// （主人转生过的 (140 − 主人等级) ÷ 2，否则为 0）。战斗里可变部分会变，所以两项分开给出（STAGE-127）。
func LoyaltyBase(hostLevel, hostCharm, hostTransmigration, petLevel, modAI int) (base, bonus int) {
	if modAI <= 0 {
		modAI = 100
	}
	base = int(float64(hostLevel) * float64(hostCharm) * 1.10 / float64(petLevel*modAI) * 100)
	if base > 100 {
		base = 100
	}
	if hostTransmigration > 0 {
		bonus = (MaxUpLevel - hostLevel) / 2
	}
	return base, bonus
}

// LoyaltyWith 由基础值、转生加成和可变部分算出忠诚（0–100）：可变部分 × 0.01 取整后加到基础值上，
// 再加转生加成，限制在 0–100。浮点运算的顺序和原版服务端一致，避免边界值差 1。
func LoyaltyWith(base, bonus, variable int) int {
	ai := int(float64(base) + float64(variable)*0.01)
	return clamp(ai+bonus, 0, 100)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
