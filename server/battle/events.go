// 战斗推送的内容（docs/protocol.md 10.3、10.7、10.9）：battle_start 的单位、
// battle_round 的事件与效果、battle_end 的结果。视图按“观看者是哪一方”隐藏敌人的精确血量。
package battle

// UnitView 是 battle_start.units、joined 和 battle 查询里的一个单位。
// 己方单位（和观看者同一方）带 hp/max_hp，自己的人物单位还带 mp/max_mp。
type UnitView struct {
	No     int      `json:"no"`
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Level  int      `json:"level"`
	HP     *int     `json:"hp,omitempty"`
	MaxHP  *int     `json:"max_hp,omitempty"`
	HPPct  int      `json:"hp_pct"`
	MP     *int     `json:"mp,omitempty"`
	MaxMP  *int     `json:"max_mp,omitempty"`
	Status []string `json:"status"`
	Ride   bool     `json:"ride"`
	// STAGE-250：Mount 是骑着的宠物，只有骑乘的人物带，没骑乘时省略（见 Mount）。
	Mount    *Mount   `json:"mount,omitempty"`
	Elements Elements `json:"elements"`
	Auto     bool     `json:"auto,omitempty"` // 分身队的单位（分身人物和它的宠物）为 true，其余不带这个字段（STAGE-162）
}

// STAGE-250：Mount 是人物骑着的宠物的战斗状态（docs/protocol.md 10.2）。骑宠不作为单位出现，
// 它的血量由这里给出：和人物同一方（观看者这边）带准确的 hp、max_hp；对方的人物只给 hp_pct。
type Mount struct {
	Name  string `json:"name"`
	HP    *int   `json:"hp,omitempty"`
	MaxHP *int   `json:"max_hp,omitempty"`
	HPPct int    `json:"hp_pct"`
}

// Effect 是一个事件里对一个目标产生的结果（docs/protocol.md 10.7）。
type Effect struct {
	Target int `json:"target"`
	// Intent 是出手时**想打的那个目标**（STAGE-167）。忠犬代受、守护之类让实际承伤者变成别人时，
	// Target 是真正挨打的单位，Intent 是原目标；两者相同时不写。客户端据此把动作和伤害画到不同人身上。
	Intent       *int     `json:"intent,omitempty"`
	Result       string   `json:"result"`
	Delta        int      `json:"delta"`
	MPDelta      int      `json:"mp_delta,omitempty"` // 气力回复的增量（道具回复 MP 用，STAGE-30）；血量变化在 delta
	HP           *int     `json:"hp,omitempty"`
	HPPct        int      `json:"hp_pct"`
	Down         bool     `json:"down"`
	StatusAdd    []string `json:"status_add,omitempty"`
	StatusRemove []string `json:"status_remove,omitempty"`
	// Flown 是这一次被打飞（STAGE-167，A05）：打飞和普通倒下不一样，客户端要沿攻击方向离场，
	// 不能只画成「血空变灰」。
	Flown bool `json:"flown,omitempty"`
}

// Event 是结算事件序列里的一条。actor 为 nil 表示系统产生的事件（status_tick）。
type Event struct {
	Kind    string     `json:"kind"`
	Actor   *int       `json:"actor,omitempty"`
	Actors  []int      `json:"actors,omitempty"` // 合击（combo）的全部出手者，按出手顺序；effects 与它一一对应
	Effects []Effect   `json:"effects,omitempty"`
	Left    []int      `json:"left,omitempty"`
	Joined  []UnitView `json:"joined,omitempty"`
	Name    string     `json:"name,omitempty"`
	Target  *int       `json:"target,omitempty"`
	OK      *bool      `json:"ok,omitempty"`
	// PetSlot 是这次事件涉及的宠物栏位（0–19；swap_pet 收回时是 −1）。0 号栏位也要出现，
	// 所以用指针：nil 才表示“没有栏位”（抓取失败、资质放生，STAGE-268）。
	PetSlot *int `json:"pet_slot,omitempty"`
	// Discarded 只在捕捉成功但资质不合格时带（STAGE-264，10.7）：此时没有 pet_slot。
	Discarded *Discarded `json:"discarded,omitempty"`
	Slot      int        `json:"slot,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	// Weapon 是这次出手的武器身份（STAGE-167，只有 attack / combo 带）：fist、axe、club、spear、
	// claw、bow、throw_axe、throw_stone、boomerang。客户端按它挑动作和轨迹，不靠名字猜。
	Weapon string `json:"weapon,omitempty"`
	// Magic 是这次施法的精灵身份与表现类别（STAGE-167，只有 magic 带）；Name 只用来显示。
	Magic *MagicView `json:"magic,omitempty"`
	// Row 是回旋镖这次扫过的横排（0–3，STAGE-167，只有 boomerang 事件带）；
	// 客户端按它把武器画到那一排，而不是按 effects 里的编号自己推。
	Row *int `json:"row,omitempty"`
	// Field 是场地变化或恢复的事件内容（STAGE-200，只有 kind 为 field 的事件带）：attr 为 none 表示无属性。
	Field *FieldView `json:"field,omitempty"`
	// Crush 是这次伤害落下后装备耐久的提示（STAGE-209，item_crush.go）：消失（黄）或受损等级变了（红）。
	Crush []CrushNote `json:"crush,omitempty"`
}

// MagicView 是 magic 事件的表现信息（STAGE-167）：客户端用它区分属性、作用范围和效果类别，
// 不用中文名字做映射（同名精灵、改名的精灵都不可靠）。
type MagicView struct {
	ID     int    `json:"id"`
	Func   string `json:"func"`             // attack / heal / heal_other / revive / cure / status
	Elem   string `json:"elem,omitempty"`   // 攻击系的属性：earth / water / fire / wind
	All    bool   `json:"all"`              // 作用范围是不是全体
	Status string `json:"status,omitempty"` // 状态精灵挂的状态、解状态精灵解的状态
}

// StateEntry 是 battle_round.state 里一个单位的最终状态。
type StateEntry struct {
	No     int      `json:"no"`
	HP     *int     `json:"hp,omitempty"`
	MP     *int     `json:"mp,omitempty"`
	HPPct  int      `json:"hp_pct"`
	Status []string `json:"status"`
	Down   bool     `json:"down"`
	Mount  *Mount   `json:"mount,omitempty"` // STAGE-250：骑着的宠物的血量，每回合刷新；没骑乘时省略
}

// RewardExp 是 battle_end.rewards.exp 里的一项。
type RewardExp struct {
	Who string `json:"who"`
	// Slot 是宠物那一项的宠物栏位（0–19）；人物那一项没有这个字段。0 号栏位也要出现（STAGE-268）。
	Slot        *int   `json:"slot,omitempty"`
	Name        string `json:"name"`
	Gain        int64  `json:"gain"`
	LevelFrom   int    `json:"level_from"`
	LevelTo     int    `json:"level_to"`
	SkillPoints int    `json:"skill_points,omitempty"`
}

// RewardItem 是 rewards.items 里的一项。
type RewardItem struct {
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

// RewardPet 是 rewards.pets 里的一项（捕捉到的宠物）。
type RewardPet struct {
	Slot  int    `json:"slot"`
	Name  string `json:"name"`
	Level int    `json:"level"`
}

// Rewards 是胜利的奖励。
type Rewards struct {
	Exp   []RewardExp  `json:"exp,omitempty"`
	Gold  int64        `json:"gold,omitempty"`
	Items []RewardItem `json:"items,omitempty"`
	Pets  []RewardPet  `json:"pets,omitempty"`
	// 宠物击杀数（STAGE-150）：只给角色服务写回宠物记录，不进协议。
	PetKills []PetKill `json:"-"`
}

// PetKill 是一只宠物本场的击杀数增量（STAGE-150）。
type PetKill struct {
	Slot  int
	Kills int
}

// Penalty 是失败的惩罚。Respawn 是失败后人物所在的地点，由 character 服务填：被打飞是送回的落点，
// 被打死是原地不动的当前位置（STAGE-194）。InPlace 只在被打死原地复活时为真。
type Penalty struct {
	Respawn  any   `json:"respawn"`
	InPlace  bool  `json:"in_place,omitempty"`
	GoldLost int64 `json:"gold_lost"`
	ExpLost  int64 `json:"exp_lost"`
}

// Round 是一回合的结算结果（battle_round 的 data）。
type Round struct {
	BattleID string       `json:"battle_id"`
	Turn     int          `json:"turn"`
	Events   []Event      `json:"events"`
	State    []StateEntry `json:"state"`
}

// EndView 是 battle_end 的 data。Rewards / Penalty 只在对应结果时填；NPC 只在 NPC 发起的战斗赢了且有结束台词时填。
type EndView struct {
	BattleID string   `json:"battle_id"`
	Result   string   `json:"result"`
	Turn     int      `json:"turn"`
	Rewards  *Rewards `json:"rewards,omitempty"`
	Penalty  *Penalty `json:"penalty,omitempty"`
	NPC      *NPCEnd  `json:"npc,omitempty"`
	// DuelPoint 只在决斗（STAGE-306）带：本场决斗点数的变化与写回后的总数，由 character 服务按写回结果填。
	DuelPoint *DuelPointView `json:"duel_point,omitempty"`
}

// view 生成一个单位的协议内容。mySide 是观看者的一方；you 是观看者自己人物的单位号。
func (u *Unit) view(mySide, you int) UnitView {
	v := UnitView{
		No: u.No, Kind: u.Kind.String(), Name: u.Name, Level: u.Level,
		HPPct: u.HPPct(), Status: u.statusList(), Ride: u.ridden(), Mount: u.mountView(mySide),
		Elements: u.Elem, Auto: u.Auto,
	}
	if u.Side() == mySide {
		hp, maxHP := u.HP, u.MaxHP
		v.HP, v.MaxHP = &hp, &maxHP
		if u.No == you {
			mp, maxMP := u.MP, u.MaxMP
			v.MP, v.MaxMP = &mp, &maxMP
		}
	}
	return v
}

// stateEntry 生成 battle_round.state 里的一项，血量字段的规则同 view。
func (u *Unit) stateEntry(mySide, you int) StateEntry {
	e := StateEntry{No: u.No, HPPct: u.HPPct(), Status: u.statusList(), Down: u.Down, Mount: u.mountView(mySide)}
	if u.Side() == mySide {
		hp := u.HP
		e.HP = &hp
		if u.No == you {
			mp := u.MP
			e.MP = &mp
		}
	}
	return e
}

// STAGE-250：mountView 生成人物骑着的宠物的协议内容，没骑乘时返回 nil，字段省略。
// 宠物和人物同一方时带准确血量，否则只给百分比（同 view 的隐藏规则）。
func (u *Unit) mountView(mySide int) *Mount {
	if !u.ridden() {
		return nil
	}
	pet := u.mount
	v := &Mount{Name: pet.Name, HPPct: pet.HPPct()}
	if u.Side() == mySide {
		hp, maxHP := pet.HP, pet.MaxHP
		v.HP, v.MaxHP = &hp, &maxHP
	}
	return v
}

// intPtr 返回 v 的指针。JSON 里 0 号栏位与“没有这个字段”要区分开（STAGE-268）。
func intPtr(v int) *int { return &v }
