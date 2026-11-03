// Package battle 是服务端的战斗引擎（STAGE-43 起，docs/protocol.md 第 10 章）。
// 规则出处是原版 gmsv/battle；引擎只管一场战斗内部的单位、指令和结算，
// 不碰网络与数据库——开战入口、推送和结果写回在 character 服务里。
package battle

import "github.com/athlan20/stoneage/server/gamedata"

// Kind 是单位的种类（docs/protocol.md 10.2）。
type Kind uint8

const (
	KindPlayer Kind = iota
	KindPet
	KindMonster
)

func (k Kind) String() string {
	return [...]string{"player", "pet", "monster"}[k]
}

// Elements 是地水火风属性，取值 0–100（人物是建角点数 × 10）。
type Elements struct {
	Earth int `json:"earth"`
	Water int `json:"water"`
	Fire  int `json:"fire"`
	Wind  int `json:"wind"`
}

// 场上的格子（docs/protocol.md 10.1）：每方 10 格，人 0–4、宠物 5–9，对方整体加 10。
const (
	unitsPerSide = 10
	sideOffset   = 10
	firstPetNo   = 5
	enemySide    = 1 // 野外战斗里怪物固定在对方（1 号方）
)

// PetSlots 是随身宠物栏位数（STAGE-346：20 个，编号 0–19，同 character.MaxPetSlots）。
// 场上每方只有 5 个宠物格子，栏位号只用来记住“这只宠物原来在哪个栏位”，不决定格子号。
const PetSlots = 20

// Unit 是战场上一个单位的数据。玩家和宠物的值来自角色快照，怪物由剧本生成。
type Unit struct {
	No          int
	Kind        Kind
	Name        string
	Level       int
	HP, MaxHP   int
	MP, MaxMP   int // 人物单位
	Attack      int
	Defense     int
	Agility     int
	Charm       int // 人物的有效魅力（捕捉概率用，STAGE-61）
	Luck        int // 人物的有效运气 1–5（暴击、闪避、反击、逃跑、异常状态用；宠物和怪物为 0，STAGE-61）
	CaptureRate int // 怪物的捕捉难度（enemybase 的 GET，越小越好抓）
	Rank        int // 怪物的品质（经验的品质系数用）
	// 四项能力的点数（人物的建角点数；宠物按 1/100 点折算），中毒扣血等公式用。
	Vital, Strength, Toughness, Dexterity int
	Elem                                  Elements
	Status                                StatusSet
	Def                                   DefenseCounters // 防御次数（STAGE-199，magic_def.go）
	actedRound                            bool            // STAGE-292：本回合已出过手（反击判定用）
	comAtk                                bool            // STAGE-292：本回合出手的指令是攻击（攻击类技能出手后算攻击，原版 COM1=ATTACK）
	guardRound                            bool            // STAGE-291 批 B：本回合交的指令是防御（原版 COM1 == GUARD，整回合有效），破防、落马术的防御判定用
	Down                                  bool
	PetSlot                               int              // 宠物单位：主人的宠物栏位；怪物 -1
	TemplateID                            int              // 宠物/怪物的模板编号（捕捉时按它生成宠物）
	Growth                                gamedata.Growth  // 怪物的资质（捕捉时要带上，STAGE-50）
	Ability                               gamedata.Ability // 怪物生成时的四项能力（1/100 点），捕捉时原样带走（STAGE-33）
	SkillSlots                            int              // 宠物的技能栏位数
	Skills                                [gamedata.PetSkillSlots]int
	// 魔法熟练度与抗性（STAGE-226，下标地水火风，原版 CHAR_EARTH_EXP / CHAR_EARTH_RESIST / *_ATTMAGIC_EXP /
	// *_DEFMAGIC_EXP）：MagicExp 是攻击时用的熟练度，MagicResist 是防守时的抗性。人物用快照里的持久化值（抗性已含
	// 抗魔装备）；怪物的熟练度 LV×0.9、抗性 LV×0.5 现算不存（NewMonster）；宠物全 0（原版宠物没有这些字段的赋值）。
	// MagicAttExp、MagicDefExp 是熟练度的经验，只有人物有，战斗里成长（STAGE-227，magic_grow.go）。
	MagicExp, MagicResist, MagicAttExp, MagicDefExp [4]int
	magicEq                                         [4]int // 抗性里抗魔装备的那部分（STAGE-227）：成长只动去掉它之后的基础值

	// 怪物的 AI 输入与战利品（STAGE-60）：AI 是解析好的战术串；EnemyExp 是遇敌表里这只怪
	// 的经验（-1 按公式算）；Drops 是出生时摇好的掉落物品编号；Weapon / WeaponCritical
	// 是怪物样式武器的反击系数类别与暴击加成。人物单位的武器由 SideInput 带进来。
	AI             aiOptions
	EnemyExp       int
	Drops          []int
	Weapon         weaponClass
	WeaponCritical int
	// WeaponMotion 是出手表现用的武器身份（STAGE-167，weapon.go）：fist、axe、club、spear、claw、
	// bow、throw_axe、throw_stone、boomerang。和 Weapon 分开——那一张是反击系数表的分类，
	// 把三种投掷武器并成一类，表现层分不开。空字符串按空手处理。
	WeaponMotion string

	// 人物被打飞（STAGE-63）：本场有一下伤害打飞了它（flyCheck），倒下时魅力多扣；
	// Overkill 是打飞的累计溢出值，跨战斗保留，由角色记录带入、结束时写回。
	Flown    bool
	Overkill int

	// 宠物的击杀数（STAGE-150）：Acted 是本场出过手（同一只宠物只记一次），Kills 是本场击败的怪物数，
	// 胜利时由 rewards 结算，写回角色的宠物记录。
	Acted bool
	Kills int

	// 分身队的单位（STAGE-162，auto.go）：分身人物及其宠物由服务端自动出招，不领奖励、不记击杀与忠诚。
	Auto bool
	// 分身人物带的精灵栏位（STAGE-195，同 SideInput.Magics）：自动战斗方案里的精灵动作用它（auto_plan.go）。
	Magics [5]int

	// 属性反转的标志（STAGE-200，magic_field.go）：开时四属性已经互换过，状态标记 reversed。
	Reversed bool

	// 骑乘的宠物（STAGE-124，ride.go）：只有人物单位有。骑着的宠物不占格子，不出手、不被选为目标；
	// 它的能力并进人物，挨打时血量按防御比例分摊。
	mount *Unit

	// 宠物技能的状态（STAGE-76）：act 是这一下攻击的临时修正，stance 是整回合的修正，
	// pend 是跨回合保存的蓄力或隐身中的出手。
	act    skillMods
	stance stanceMods
	pend   *pending
	plan   *monsterPlan // 怪物本回合提前定好的行动（合击检查要先看后面的怪物，combo.go）
	obey   *obeyPlan    // 宠物本回合的听话判定（loyalty.go）

	// 宠物的忠诚（STAGE-127，loyalty.go）：基础值与转生加成不变，可变部分 loyVar 随战斗增减；
	// recovered 记这场战斗里是否已因回复加过忠诚（每场一次）。
	loyBase, loyBonus, loyVar int
	recovered                 bool
	pvp                       bool // STAGE-306：决斗里的单位（异常状态不计等级差，duel_rules.go）

	// 人物本人的装备耐久（STAGE-209，item_crush_hook.go）：crushOwner 为真的才结算；crush 按栏位放装备，nil 为空栏。
	crushOwner bool
	crush      [crushSlotCount]*EquipInput
}

// weaponClass 是反击系数表（CounterTbl）的行列：攻守双方手里是什么武器
// （battle_event.c 的 BATTLE_C_*）。长矛在原版没映射（BATTLE_ItemType2ItemMap
// 没写 ITEM_SPEAR），落回 NONE——照搬。
type weaponClass int

const (
	weaponNone  weaponClass = iota // 无武器或长矛
	weaponClaw                     // 空手 / 拳套
	weaponAxe                      // 斧
	weaponClub                     // 棍棒
	weaponSpear                    // 枪（未用，长矛映射到 NONE）
	weaponBow                      // 弓
	weaponThrou                    // 投掷（回旋标、投掷斧、投掷石）
)

// weaponClassOf 把物品类型（itemset.txt 的 ItemType）换算成反击系数类别
// （BATTLE_ItemType2ItemMap）。
func weaponClassOf(itemType int) weaponClass {
	switch itemType {
	case gamedata.ItemTypeFist:
		return weaponClaw
	case gamedata.ItemTypeAxe:
		return weaponAxe
	case gamedata.ItemTypeClub:
		return weaponClub
	case gamedata.ItemTypeBow:
		return weaponBow
	case gamedata.ItemTypeBoomerang, gamedata.ItemTypeBoundThrow, gamedata.ItemTypeBreakThrow:
		return weaponThrou
	}
	return weaponNone
}

// Side 返回单位所在的方（0 或 1）。
func (u *Unit) Side() int { return u.No / unitsPerSide }

// Alive 表示单位还在场上战斗（没倒下也没离场）。
func (u *Unit) Alive() bool { return !u.Down }

// HPPct 是 0–100 的血量百分比；活着的单位至少是 1（敌人血量是隐藏信息，协议 10.2）。
func (u *Unit) HPPct() int {
	if u.Down {
		return 0
	}
	// STAGE-159：MaxHP 异常（<1）时按 1 算，不让除零 panic 带走整个服务端。
	pct := u.HP * 100 / max(u.MaxHP, 1)
	if pct < 1 {
		return 1
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// StatusSet 是单位的状态标记（docs/protocol.md 10.2）。异常状态的字段是**剩余回合数**
// （0 表示没有，照原版 CHAR_setWorkInt(StatusTbl[i], 回合数) 的存法）；guard 是防御动作，
// buff / debuff 是精灵的增益减益（STAGE-48 接入），这三个只有有无。
type StatusSet struct {
	Poison                                            int
	Paralysis                                         int
	Sleep                                             int
	Stone                                             int
	Drunk                                             int
	Confusion                                         int
	Weaken                                            int // STAGE-201：虚弱（虚）
	Barrier                                           int // STAGE-201：魔障（障）
	Nocast                                            int // STAGE-201：沉默（默）
	ResistMagic, ResistFire, ResistThunder, ResistIce int // STAGE-228：魔抗、火抗、电抗、冰抗的剩余回合（原版 MagicTbl，同一单位只能带一种）
	ResistPct                                         int // STAGE-228：魔抗的加成百分比（原版 CHAR_OTHERSTATUSNUMS；火、电、冰抗也记，但不生效）
	Guard                                             bool
	Buff, Debuff                                      bool
}

// List 按固定顺序给出协议里的状态名列表。
func (s StatusSet) List() []string {
	out := make([]string, 0, 9)
	add := func(turns int, name string) {
		if turns > 0 {
			out = append(out, name)
		}
	}
	add(s.Poison, "poison")
	add(s.Paralysis, "paralysis")
	add(s.Sleep, "sleep")
	add(s.Stone, "stone")
	add(s.Drunk, "drunk")
	add(s.Confusion, "confusion")
	add(s.Weaken, "weaken")
	add(s.Barrier, "barrier")
	add(s.Nocast, "nocast")
	add(s.ResistMagic, "magic_resist")
	add(s.ResistFire, "fire_resist")
	add(s.ResistThunder, "thunder_resist")
	add(s.ResistIce, "ice_resist")
	if s.Guard {
		out = append(out, "guard")
	}
	if s.Buff {
		out = append(out, "buff")
	}
	if s.Debuff {
		out = append(out, "debuff")
	}
	return out
}

// CanAct 表示单位能不能自己行动：睡眠、石化、麻痹、魔障（STAGE-201）时不能（docs/protocol.md 10.4；原版 BATTLE_CanMoveCheck）。
func (s StatusSet) CanAct() bool {
	return s.Sleep <= 0 && s.Stone <= 0 && s.Paralysis <= 0 && s.Barrier <= 0
}

// NewMonster 按模板和等级生成一只怪物单位（资质随机，同 ENEMY_createPetFromEnemyIndex 的前半段）。
// rnd(n) 返回 [0, n) 的随机整数。variant 是遇敌表里这只怪对应的敌人变体（STAGE-60）：
// 带它的 AI 战术串、经验值和掉落；剧本怪物传 nil（默认普攻随机目标，不掉落）。
func NewMonster(t gamedata.PetTemplate, level int, no int, rnd func(n int) int, variant *gamedata.EnemyVariant) *Unit {
	base, ability := gamedata.NewPetGrowth(t, level, rnd)
	u := &Unit{
		No: no, Kind: KindMonster, Name: t.Name, Level: level,
		HP: gamedata.PetMaxHP(ability), MaxHP: gamedata.PetMaxHP(ability),
		Attack: gamedata.PetAttack(ability), Defense: gamedata.PetDefense(ability),
		Agility:     gamedata.PetAgility(ability),
		Vital:       ability.Vital / 100,
		Strength:    ability.Strength / 100,
		Toughness:   ability.Toughness / 100,
		Dexterity:   ability.Dexterity / 100,
		Elem:        Elements{Earth: t.Earth, Water: t.Water, Fire: t.Fire, Wind: t.Wind},
		CaptureRate: t.Get, Rank: gamedata.PetRank(t), PetSlot: -1, TemplateID: t.ID, Growth: base, Ability: ability,
		SkillSlots: t.SkillSlots,
	}
	for i, id := range t.Skills {
		u.Skills[i] = id
	}
	// 魔法熟练度 LV×0.9、抗性 LV×0.5（原版 battle_magic.c 赋给 int，小数截掉，等级为正所以就是整除）。
	for i := range u.MagicExp {
		u.MagicExp[i] = level * 9 / 10
		u.MagicResist[i] = level / 2
	}
	if variant == nil {
		u.AI = defaultAIOptions()
		u.EnemyExp = -1
		return u
	}
	u.AI = parseAIOptions(variant.TacticsOption)
	u.EnemyExp = variant.Exp
	// 掉落在出生时摇好（enemy.c 的 ENEMY_createPetFromEnemyIndex：RAND(0,999) < 概率）。
	for _, d := range variant.Drops {
		if d.Item > 0 && d.Prob != 0 && rnd(1000) < d.Prob {
			u.Drops = append(u.Drops, d.Item)
		}
	}
	// 武器样式 1–7 给怪物配一把底武（enemy.c 的 style → wepon），带出反击系数与暴击加成。
	if w := styleWeapon(variant.Style); w != nil {
		u.Weapon = weaponClassOf(w.Type)
		u.WeaponCritical = w.Critical
	}
	return u
}

// styleWeapon 是怪物样式对应的武器模板（enemy.c 的 switch(style)：0/100/200/400/500/700/600）。
func styleWeapon(style int) *gamedata.ItemTemplate {
	id := map[int]int{1: 0, 2: 100, 3: 200, 4: 400, 5: 500, 6: 700, 7: 600}[style]
	if id == 0 && style != 1 {
		return nil
	}
	if t, ok := gamedata.ItemByID(id); ok {
		return &t
	}
	return nil
}
