// Battle 是一场战斗的完整状态（docs/protocol.md 10.1 的回合时序）：
// battle_turn → battle_command(人物) → battle_command(宠物) → battle_progress →
// battle_round → battle_ack → 下一回合或 battle_end。
// 引擎不做任何 IO；推进和推送由 character 服务驱动。

package battle

import (
	"sort"
	"time"
)

// Phase 是回合内的阶段：收出招（command）或等播放确认（replay）。
type Phase uint8

const (
	PhaseCommand Phase = iota
	PhaseReplay
)

// Cmd 是一条已提交的出招指令。
type Cmd struct {
	Actor   string       // "player" / "pet"
	Action  string       // attack magic item guard capture escape swap_pet skill
	Target  int          // 单位号；没有目标时 -1
	Magic   int          // 精灵栏位 0–4
	Slot    int          // 道具的背包槽位 5–19
	Skill   int          // 宠物技能栏位 1–7
	PetSlot int          // swap_pet 的宠物栏位 0–19，-1 收回
	Keep    *CaptureKeep // capture 的资质筛选，nil 为不限（STAGE-264）
}

// BattleItem 是战斗里一件可用的物品：物品编号（查使用效果）与名字（battle_round 的事件里显示）。
type BattleItem struct {
	ID   int
	Name string
}

// SideInput 是开战时一方（一期只有一个玩家）的快照，由 character 服务从角色数据构造。
type SideInput struct {
	Name      string
	Level     int
	HP, MaxHP int
	MP, MaxMP int
	Attack    int
	Defense   int
	Agility   int
	Charm     int // 有效魅力（基础值加装备修正，STAGE-61）
	Luck      int // 有效运气 1–5（STAGE-61）
	Overkill  int // 打飞的累计溢出值（STAGE-63，跨战斗保留）
	// 四项能力的点数（中毒扣血等公式用，同 record.Stats）。
	Vital, Strength, Toughness, Dexterity int
	Elem                                  Elements
	// 魔法熟练度与抗性（STAGE-226，下标地水火风，同 Unit）：MagicResist 已含抗魔装备。
	MagicExp, MagicResist, MagicAttExp, MagicDefExp [4]int
	MagicEquip                                      [4]int // 抗魔装备的加成（已含在 MagicResist 里，STAGE-227：成长时要扣掉）
	// BattlePet 是出战宠物（可为 nil）；Bench 是其余栏位的宠物快照（换宠用），
	// nil 表示那个栏位没有宠物。
	BattlePet *PetInput
	Bench     [PetSlots]*PetInput
	// Rider 是骑着的宠物（STAGE-149，ride.go）：不上格子、不进替补，人物的数值与它合成一个单位。
	// 它和出战的 BattlePet 可以同时存在，出战的那只照常有自己的宠物指令；nil 表示没有骑乘。
	Rider *PetInput
	// Items 是背包槽位 5–19 上的物品（道具指令用，STAGE-30）；Name 为空表示空槽。
	// 使用中消耗时引擎把对应槽位置空，写库与推送由 character 服务按 battle_round 事件里的 slot 做。
	Items [20]BattleItem
	// Magics 是人物的精灵栏位（0–4，-1 为空），出招用 magic 栏位号指定。
	Magics [5]int
	// MagicMP 是各栏位施放要耗的气力，MagicMPSet 为 true 的栏位才用它（STAGE-190：装备自带的精灵以装备的
	// ITEM_MAGICUSEMP 为准）；没设的栏位按魔法表里的 MP（单元测试和老用法只填 Magics）。
	MagicMP    [5]int
	MagicMPSet [5]bool
	// 手里武器的物品类型与暴击修正（STAGE-60：反击系数表与暴击加成用）；空手时 Type 为 -1。
	WeaponType     int
	WeaponCritical int
	// Equips 是人物装备栏（0–4，STAGE-209）的耐久快照，空栏为 nil；只有人物本人的装备会损坏。
	Equips [crushSlotCount]*EquipInput
	// Gold 与 Transmigration 是开战时的金币和转生次数（胜利金币补到持有上限用，STAGE-60）。
	Gold           int64
	Transmigration int
	// DuelPoint 是开战时的决斗点数（STAGE-306，决斗里记账用）。
	DuelPoint int
}

// PetInput 是一只宠物（出战或备用）的快照。
type PetInput struct {
	Slot       int
	TemplateID int
	Name       string
	Level      int
	HP, MaxHP  int
	Attack     int
	Defense    int
	Agility    int
	// 四项能力的点数（宠物的 Ability 是 1/100 点，这里已折算）。
	Vital, Strength, Toughness, Dexterity int
	Elem                                  Elements
	Skills                                [7]int
	SkillSlots                            int
	// 忠诚（STAGE-127）：基础值与转生加成（gamedata.LoyaltyBase），可变部分（CHAR_VARIABLEAI，1/100 点）。
	LoyaltyBase, LoyaltyBonus int
	VariableAI                int
}

// Battle 是一场战斗。
type Battle struct {
	lastAtk      atkRec // STAGE-292：本次出手的攻击结果，反击循环用（counter_chain.go）
	stolenGold   int64  // STAGE-291：本场偷到的石币（捐献），结算时计入胜利奖励（skills_steal.go、rewards.go）
	ID           string
	Kind         string    // "wild"，NPC 发起的战斗为 "npc"（SetNPC）
	NPC          *NPCFight // NPC 发起的战斗才有（STAGE-96）
	Scene        string
	You          int // 玩家人物的单位号
	AllowEscape  bool
	AllowCapture bool

	units [20]*Unit
	pets  [PetSlots]*Unit // 玩家方每个宠物栏位的宠物单位（在场或换下），血量和忠诚随战斗保留（STAGE-127）；栏位数同 SideInput.Bench
	turn  int
	phase Phase

	needs       []string        // 本回合需要交的指令，按 player、pet 顺序
	cmds        map[string]*Cmd // 已提交的指令
	sides       [2]*SideInput   // 玩家方的原始快照（换宠时取备用宠物）
	slain       []*Unit         // 本场被击杀的怪物（结算奖励用）
	petsActed   []*Unit         // 本场出过手的宠物（击杀数结算用，换下战场的也在内）
	captur      []*Unit         // 本场捕捉到的怪物
	leftPet     *Unit           // 逃跑成功时一起离场的出战宠（STAGE-247）：离场后 commandPet 找不到它，结算仍认它是出战
	escapeTries map[int]int     // 每个单位已尝试逃跑的次数（BATTLE_EscapeCheck 的累加）
	guards      map[int]int     // 本回合忠犬替挡的登记：主人单位号 → 忠犬单位号（beginRound 填）
	autoCmds    map[int]*Cmd    // 本回合分身及其宠物的自动指令：单位号 → 指令（planAuto 填，auto.go）
	optional    []string        // 本回合玩家可以指定目标的分身（clone:N / clone_pet:N），不进 needs（clone_cmd.go）
	cloneCmds   map[int]*Cmd    // 玩家给分身指定的目标：单位号 → 指令，没指定的由 planAuto 按合击顺序选（STAGE-312）
	plan        *AutoPlan       // 队长交给分身队的自动战斗方案（STAGE-195，auto_plan.go）；nil 时分身普通攻击
	field       fieldState      // 场地属性（STAGE-200，magic_field.go）：整场战斗一个，双方共用

	Rnd func(n int) int // 随机源，测试传固定序列
	// crushRnd 是装备耐久专用的随机源（STAGE-209，按战斗编号播种）：不占用 Rnd，既有战斗的随机序列不变。
	crushRnd func(n int) int
	Cbs      Callbacks // 外部决策回调（捕捉后安排栏位等），New 之后由调用方设置
	// FleeTurn 是剧本指定的“怪物在这一回合全部逃走”（> 0 时生效，展示 draw 的用例用）。
	FleeTurn int
	// Combo 打开合击（STAGE-126，combo.go）：真实遇敌与 NPC 战斗开，剧本默认关，免得改变已校准的协议用例。
	Combo bool

	loyalChanged bool   // 本场有宠物的忠诚变过（loyalty.go），结束时要推 pets_changed
	result       string // 战斗结果（结束前为空）
	over         bool

	// 真人同场（party.go，STAGE-275）：party 为 false 时下面的字段都不用。
	party      bool
	mates      [15]*Mate           // 队员按人物单位号索引（1–4；决斗里对方的真人占 10–14）
	mateCmds   map[int]*Cmd        // 队员的指令，按单位号（人物 1–4、宠物 6–9）
	left       map[int]string      // 已离场的真人：人物单位号 → 离场原因（escape / timeout / offline）
	finalHuman map[int]*Unit       // 离场真人的人物单位，结算读最后的血量
	departs    []Departure         // 本回合还没取走的离场（TakeDepartures）
	deadline   time.Time           // 本回合交指令的截止时间，零值为还没有人交过
	startedAt  time.Time           // 开战时间，整场时限从这里算
	Clock      func() time.Time    // 时钟（测试注入），nil 用 time.Now
	acting     []int               // 正在结算的出手单位号（击杀归属，party_reward.go）
	killers    map[*Unit][]int     // 怪物 → 击倒它的出手单位号
	captors    map[*Unit]int       // 捕获的怪物 → 捕获者的单位号
	finalPets  map[int]*Unit       // 离场的队员的出战宠物（结算的经验用）
	matePets   [15][PetSlots]*Unit // 队员的宠物栏位（队员号 × 宠物栏位），同 pets，换宠与血量保留用
	partyDrops map[int][]int       // 胜利时每个真人分到的掉落（assignDrops 填）

	duel    *duelState // STAGE-306：决斗（双真人方，duel.go）；非决斗为 nil
	chkSide int        // 正在校验指令的真人所在的方（checkParamsFor 设置，validTarget 用）
}

// New 开始一场战斗：side 是玩家方的快照，monsters 是怪物的初始数据（顺序占对方的格子）。
// 怪物从对方的宠物排（15 号）开始占格，同一方最多 10 个单位。
func New(id, scene string, side *SideInput, monsters []*Unit, rnd func(n int) int) *Battle {
	b := &Battle{
		ID: id, Kind: "wild", Scene: scene, You: 0,
		AllowEscape: true, AllowCapture: true,
		cmds: map[string]*Cmd{}, escapeTries: map[int]int{},
		sides: [2]*SideInput{side, nil}, Rnd: rnd, crushRnd: newCrushRand(id),
	}
	b.units[0] = humanUnit(side, 0)
	for _, p := range side.Bench {
		if p != nil {
			b.pets[p.Slot] = petUnit(p, firstPetNo)
		}
	}
	if p := side.Rider; p != nil { // 骑着的宠物不上格子（ride.go），人物的 mount 指向它
		b.pets[p.Slot] = petUnit(p, firstPetNo)
		b.units[0].mount = b.pets[p.Slot]
	}
	if p := side.BattlePet; p != nil {
		b.pets[p.Slot] = petUnit(p, firstPetNo)
		b.units[firstPetNo] = b.pets[p.Slot]
	}
	for i, m := range monsters {
		// 怪物从对方的宠物排（15 号）开始占格，排满了接着用后排（10–14），同一方最多 10 只（NPC 战斗的阵容可以到 10 只）。
		m.No = enemySide*sideOffset + (firstPetNo+i)%unitsPerSide
		b.units[m.No] = m
	}
	b.nextTurn()
	return b
}

// humanUnit 按一方的快照生成人物单位；队长和分身（auto.go）共用。
func humanUnit(side *SideInput, no int) *Unit {
	u := &Unit{
		No: no, Kind: KindPlayer, Name: side.Name, Level: side.Level,
		HP: side.HP, MaxHP: side.MaxHP, MP: side.MP, MaxMP: side.MaxMP,
		Attack: side.Attack, Defense: side.Defense, Agility: side.Agility, Charm: side.Charm, Luck: side.Luck,
		Vital: side.Vital, Strength: side.Strength, Toughness: side.Toughness, Dexterity: side.Dexterity,
		Elem: side.Elem, PetSlot: -1, Overkill: side.Overkill,
		MagicExp: side.MagicExp, MagicResist: side.MagicResist, MagicAttExp: side.MagicAttExp, MagicDefExp: side.MagicDefExp,
		magicEq: side.MagicEquip,
		Weapon:  weaponClaw, WeaponCritical: side.WeaponCritical,
	}
	if side.WeaponType >= 0 {
		u.Weapon = weaponClassOf(side.WeaponType)
		u.WeaponMotion = weaponMotion(side.WeaponType)
	}
	if no == 0 { // STAGE-209：人物本人的装备耐久（分身的装备不结算，本身也不写回）
		u.crushOwner, u.crush = true, copyEquips(side.Equips)
	}
	return u
}

func petUnit(p *PetInput, no int) *Unit {
	u := &Unit{
		No: no, Kind: KindPet, Name: p.Name, Level: p.Level,
		HP: p.HP, MaxHP: p.MaxHP, Attack: p.Attack, Defense: p.Defense, Agility: p.Agility,
		Vital: p.Vital, Strength: p.Strength, Toughness: p.Toughness, Dexterity: p.Dexterity,
		Elem: p.Elem, PetSlot: p.Slot, TemplateID: p.TemplateID, SkillSlots: p.SkillSlots,
		Down:    p.HP <= 0, // 血量为 0 的宠物进场就是倒下的：不能行动，可以被复活精灵选中（STAGE-77）
		loyBase: p.LoyaltyBase, loyBonus: p.LoyaltyBonus, loyVar: p.VariableAI,
	}
	u.Skills = p.Skills
	return u
}

// swapIn 把栏位上的宠物换上场（换宠）：技能和状态按开战时的快照重来，血量、倒下与忠诚的可变部分
// 沿用它在本场战斗里最后的值（同原版：宠物换下去再换回来，血量不回满）。
func (b *Battle) swapIn(slot int, in *PetInput, no int) *Unit {
	return b.swapInFor(0, slot, in, no)
}

// swapInFor 是真人 owner 的换宠（队员的宠物放在 matePets 里，队长的在 pets 里）。
func (b *Battle) swapInFor(owner, slot int, in *PetInput, no int) *Unit {
	slots := b.petSlots(owner)
	old := slots[slot]
	u := petUnit(in, no)
	u.pvp = b.duel != nil
	if old != nil {
		u.HP, u.Down, u.loyVar, u.recovered = old.HP, old.Down, old.loyVar, old.recovered
	}
	slots[slot] = u
	return u
}

// Turn 返回当前回合号（从 1 开始）。
func (b *Battle) Turn() int { return b.turn }

// Phase 返回当前阶段。
func (b *Battle) Phase() Phase { return b.phase }

// Result 返回战斗结果（win/lose/escaped/draw）；战斗没结束时为空。
func (b *Battle) Result() string { return b.result }

// Over 表示战斗已经结束（battle_end 已可以发出）。
func (b *Battle) Over() bool { return b.over }

// Captured 返回本场捕捉到的怪物（结束写回时入库）。
func (b *Battle) Captured() []*Unit { return b.captur }

// Slain 返回本场被击杀的怪物（结算奖励用）。
func (b *Battle) Slain() []*Unit { return b.slain }

// PetsChanged 判断战斗结束后要不要推 pets_changed：胜利、失败或捕捉到宠物时
// 宠物的血量、经验或列表一定有变化；逃跑和平局一般没有（showcase 用例）。
func (b *Battle) PetsChanged() bool {
	return b.result == "win" || b.result == "lose" || len(b.captur) > 0 || b.loyalChanged
}

// PetEnd 是一只宠物在战斗结束时的状态，写回宠物表用：血量、忠诚的可变部分，以及它是不是还在场上。
type PetEnd struct {
	Slot       int
	HP         int
	VariableAI int
	Active     bool
}

// PetEnds 给出玩家方每只在这场战斗里出过场的宠物的结束状态（含中途换下、跑掉的，STAGE-127）。
func (b *Battle) PetEnds() []PetEnd { return b.PetEndsFor(0) }

// PetEndsFor 是真人 no 的宠物的结束状态（真人同场时每个队员各自写回；单人战斗的 no 只能是 0）。
func (b *Battle) PetEndsFor(no int) []PetEnd {
	var out []PetEnd
	for slot, u := range b.petSlots(no) {
		if u == nil {
			continue
		}
		// 出战 = 结束时还站在格子里的，或者是离场时一起走的（逃跑 STAGE-247，真人同场的离场见 finalPet）。
		out = append(out, PetEnd{Slot: slot, HP: u.HP, VariableAI: u.loyVar, Active: b.petOf(no) == u || b.finalPet(no) == u})
	}
	return out
}

// Unit 返回某个单位号的单位；那个格子没有单位时返回 nil。
func (b *Battle) Unit(no int) *Unit {
	if no < 0 || no >= 20 {
		return nil
	}
	return b.units[no]
}

// PlayerUnit 返回玩家人物单位。
func (b *Battle) PlayerUnit() *Unit { return b.units[b.You] }

// playerSide 返回玩家方的阵营号。STAGE-252：人物逃跑离场后 PlayerUnit 为 nil，比较阵营不能再依赖它，
// 所以用开战时固定的人物单位号（b.You）算阵营，与人物在不在场无关。
func (b *Battle) playerSide() int { return b.You / unitsPerSide }

// PlayerFlown 表示玩家人物在这场战斗里被打飞过（倒下时魅力多扣，STAGE-63）。
func (b *Battle) PlayerFlown() bool {
	u := b.PlayerUnit()
	return u != nil && u.Flown
}

// PetUnit 返回站在格子里的出战宠物单位；没有时返回 nil。骑着的宠物不在这里，见 RiderUnit。
func (b *Battle) PetUnit() *Unit {
	return b.commandPet()
}

// RiderUnit 返回骑着的宠物单位（即使已经倒下，结算时要写回血量和经验）；没有骑乘时返回 nil。
func (b *Battle) RiderUnit() *Unit {
	if pu := b.PlayerUnit(); pu != nil {
		return pu.mount
	}
	return nil
}

// commandPet 返回站在格子里、能下指令的出战宠物；骑着的宠物不在格子里，返回 nil。
func (b *Battle) commandPet() *Unit {
	if p := b.units[b.You/unitsPerSide*unitsPerSide+firstPetNo]; p != nil && p.Kind == KindPet {
		return p
	}
	return nil
}

// Needs 返回本回合需要交的指令（player / pet）。
func (b *Battle) Needs() []string { return b.needs }

// Submitted 返回各身份是否已交指令。
func (b *Battle) Submitted() map[string]bool {
	out := map[string]bool{}
	for _, actor := range []string{"player", "pet"} {
		out[actor] = b.cmds[actor] != nil
	}
	for _, actor := range b.optional { // 分身已指定目标的（STAGE-186）
		if no, ok := cloneActor(actor); ok {
			out[actor] = b.cloneCmds[no] != nil
		}
	}
	return out
}

// AllSubmitted 表示本回合玩家的全部指令已经交齐，可以结算。真人同场时要等所有还在场的真人都交齐（party.go）。
func (b *Battle) AllSubmitted() bool {
	for _, actor := range b.needs {
		if b.cmds[actor] == nil {
			return false
		}
	}
	return b.matesSubmitted()
}

// nextTurn 进入下一回合：清掉上一回合的指令和防御，重算 needs。
func (b *Battle) nextTurn() {
	b.turn++
	b.phase = PhaseCommand
	b.cmds = map[string]*Cmd{}
	b.cloneCmds = map[int]*Cmd{}
	for _, u := range b.units {
		if u != nil {
			u.Status.Guard = false
		}
	}
	b.planOptional()
	b.refreshNeeds()
	if b.party {
		b.mateCmds = map[int]*Cmd{}
		b.deadline = time.Time{}
	}
}

// AckedCount / AddAck 由 character 服务记录回放确认（它知道有几条连接在线）；
// 引擎只负责回合内容，协议 10.8 的“所有在线连接都确认”在接线层判断。

// ActingOrder 给出本回合的行动顺序：敏捷从高到低，相同的按单位号从小到大。
func (b *Battle) actingOrder() []*Unit {
	out := make([]*Unit, 0, 10)
	for _, u := range b.units {
		if u != nil && u.Alive() {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ai, aj := out[i].agilityAct(), out[j].agilityAct(); ai != aj {
			return ai > aj
		}
		return out[i].No < out[j].No
	})
	return out
}

// sideGone 表示一方已经没有还站着的单位（全倒下或全部离场）。分身队的单位不数：它们只是帮手，
// 队长和队长的宠物都倒下就算己方落败（STAGE-162，auto.go）。
// STAGE-169：己方只数人物——人物倒下就算落败，宠物还有血也不继续打；怪物方仍是全灭才算。
func (b *Battle) sideGone(side int) bool {
	for _, u := range b.units {
		if u == nil || u.Auto || u.Side() != side || !u.Alive() {
			continue
		}
		if (side == 0 || b.duel != nil) && u.Kind != KindPlayer { // 决斗里两方都只数人物（STAGE-306）
			continue
		}
		return false
	}
	return true
}

// leave 把单位从场上移走（逃跑、被捕捉）。
func (b *Battle) leave(u *Unit) {
	b.units[u.No] = nil
}
