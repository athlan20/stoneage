package gamedata

// STAGE-235：宠物转生后的数值重算（纯函数，不碰数据库和网络）。
// 出处：gmsv/npc/npc_transmigration.c 的 NPC_PetTransManStatus（取值、按比例分配的顺序），
// gmsv/char/char_data.c 的 NPC_PetTransManGetAns（配额），gmsv/char/enemy.c 的 GetNewPet（新资质、品质、能力）。
// 原版 PETTRANS_PetTransManStatus（char_data.c）没有调用方，不照它做。
// 转生窗口、删除玛蕾、发新宠物、写日志不在这里，见 server/character（pet_transmig_apply.go）。

// Rnd 是随机数来源：返回 [0, n) 的整数。生产用真随机，测试时换成固定序列。
type Rnd func(n int) int

const (
	transLVCap    = 130 // NPC_PetTransManGetAns：等级超过 130 按 130
	transTotalCap = 150 // NPC_PetTransManStatus：玛蕾的资质合计超过 150 按 150
	transBaseLV   = 100 // NPC_PetTransManGetAns：TransLV
)

// transRankMin 是 GetNewPet 的品质门槛：资质合计 ≥ 门槛即得到该下标的品质（0–5）。
// 注意这张表和 PetRank（ENEMY_getRank，门槛 100/95/90/85/80）不同，转生只用这一张。
var transRankMin = [...]int{130, 100, 95, 85, 80, 0}

// TransResult 是转生重算的结果，交给 character 包写回宠物。
type TransResult struct {
	Answer int     // 配额（NPC_PetTransManGetAns）：转生后四项资质的合计上限
	Base   Growth  // 新宠物的资质（CHAR_ALLOCPOINT）
	Rank   int     // 新宠物的品质（CHAR_PETRANK）
	Stats  Ability // 新宠物的四项能力（1/100 点，等级 1）
}

// TransAnswer 是 NPC_PetTransManGetAns：算转生后的配额。
// total1 是玛蕾的资质合计（已限制在 0–150），total2 是被转生宠物的资质合计，level 和 rank 是它的等级和品质，
// trans 是它的转生次数：0 时配额上限 150，转生过的上限 200（原版 _PET_2TRANS）。
//
// 手算对照（total1=150, total2=80, level=100, rank=0, trans=0）：
//
//	t = 150/100 = 1.5，t⁵ = 7.59375，≥ 1，所以 × 1.3 = 9.871875，取整得 9；
//	Fx = (int)((5−0) × 1.2) + 5 = 6 + 5 = 11；(100−100) ÷ 11 = 0；
//	ans = 9 + 80 + 0 = 89，没超过 150，结果 89。
func TransAnswer(total1, total2, level, rank, trans int) int {
	if level > transLVCap {
		level = transLVCap
	}
	// 原版是 float 运算：玛蕾合计 ÷ 100 的五次方，小于 1 当 0，否则 × 1.3，再截成整数。
	t := float32(total1) / 100
	t = t * t * t * t * t
	extra := 0
	if t >= 1 {
		extra = int(float32(float64(t) * 1.3))
	}
	fx := int(float64(5-rank)*1.2) + 5
	ans := extra + total2 + (level-transBaseLV)/fx
	limit := 150
	if trans != 0 {
		limit = 200
	}
	if ans > limit {
		ans = limit
	}
	return ans
}

// Transmigrate 是转生重算的全过程（NPC_PetTransManStatus 里“算数值”的部分）。
// mala 是玛蕾的资质，pet 是被转生宠物的资质（都是 CHAR_ALLOCPOINT），tmpl 是被转生宠物的模板（取 INITNUM），
// petLevel、petRank、petTrans 是被转生宠物现在的等级、品质、转生次数。
func Transmigrate(mala, pet Growth, tmpl PetTemplate, petLevel, petRank, petTrans int, rnd Rnd) TransResult {
	total1 := clamp(mala.sum(), 0, transTotalCap)
	total2 := pet.sum()
	ans := TransAnswer(total1, total2, petLevel, petRank, petTrans)
	base, stats, rank := TransNewPet(transShare(ans, total1, total2, mala, pet), tmpl, rnd)
	return TransResult{Answer: ans, Base: base, Rank: rank, Stats: stats}
}

// transShare 把配额按两只宠物的资质比例分到四项：
// 每项 = ans × (玛蕾的项 + 宠物的项 × 4) ÷ (total1 + total2 × 4)，截成整数。
// 原版是 float 运算后截断，这里两边都是整数，结果一样。分母为 0 时原版得到 NaN，这里给全 0。
func transShare(ans, total1, total2 int, mala, pet Growth) Growth {
	total := total1 + total2*4
	if total == 0 {
		return Growth{}
	}
	share := func(m, p int) int { return ans * (m + p*4) / total }
	return Growth{
		Vital:     share(mala.Vital, pet.Vital),
		Strength:  share(mala.Strength, pet.Strength),
		Toughness: share(mala.Toughness, pet.Toughness),
		Dexterity: share(mala.Dexterity, pet.Dexterity),
	}
}

// TransNewPet 是 GetNewPet：work 是分到的资质。先每项 ±2 随机得到新资质（即 CHAR_ALLOCPOINT），
// 资质合计定品质；再随机 10 点逐个加到四项上，能力 = INITNUM × 资质。
// 随机数顺序同原版：先四次 RAND(0,4)，再十次 RAND(0,3)。
//
// 能力公式：原版是 ((LV−1) × LVUPPOINT + INITNUM) × 资质，宏 PARAM_CAL 的参数不用，四项都乘同一个数。
// 这里 LV = 1，所以乘的是 INITNUM。
func TransNewPet(work Growth, tmpl PetTemplate, rnd Rnd) (base Growth, stats Ability, rank int) {
	base = Growth{
		Vital:     work.Vital + rnd(5) - 2,
		Strength:  work.Strength + rnd(5) - 2,
		Toughness: work.Toughness + rnd(5) - 2,
		Dexterity: work.Dexterity + rnd(5) - 2,
	}
	rank = transRank(base.sum())
	g := base
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
	perLevel := tmpl.InitNum
	stats = Ability{
		Vital:     perLevel * g.Vital,
		Strength:  perLevel * g.Strength,
		Toughness: perLevel * g.Toughness,
		Dexterity: perLevel * g.Dexterity,
	}
	return base, stats, rank
}

// transRank 按 transRankMin 定品质。原版循环找不到门槛时品质保持 0，所以合计为负数时是 0。
func transRank(sum int) int {
	for i, min := range transRankMin {
		if sum >= min {
			return i
		}
	}
	return 0
}

// sum 是四项资质的合计。
func (g Growth) sum() int { return g.Vital + g.Strength + g.Toughness + g.Dexterity }
