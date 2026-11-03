// 宠物的忠诚（STAGE-127）：战斗里忠诚低的宠物不一定听指挥，以及忠诚的增减。
// 对照原版 gmsv/battle/battle.c 的 BATTLE_PetLoyalCheck（听话判定）、CHAR_BATTLEFLG_AIBAD（忠诚不足）
// 和 include/battle.h 的 AI_FIX_*（增减）。忠诚（0–100）由 gamedata.Loyalty 现算，战斗里变的只有可变部分。
//
// 听话判定在宠物交了指令、能行动、没有在蓄力或隐身时做，结算开始时按需定下（同怪物的 takePlan，
// 合击检查要先看后面的单位）。忠诚 ≥ 80 时一定听话，这里直接跳过（原版仍会摇一次骰子，但结果不变）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// 忠诚的增减（battle.h 的 AI_FIX_*，单位 1/100 点，同 gamedata.AddVariableAI）。
const (
	loyalWin        = 1         // AI_FIX_PETWIN：胜利，在场的宠物每打倒一只敌人加一次
	loyalWinHigher  = 2 * 10    // AI_FIX_PETGOLDWIN：打倒的敌人比宠物等级高时加这个（代替 loyalWin）
	loyalRecovered  = 10        // AI_FIX_PETRECOVERY：被道具或精灵回复，每场战斗一次
	loyalRevived    = 3 * 100   // AI_FIX_PETRESSURECT：被复活精灵救起
	loyalOwnerHit   = -2 * 100  // AI_FIX_SEKKAN：主人攻击自己的宠物
	loyalFlown      = -10 * 100 // AI_FIX_PLAYERULTIMATE：主人被打飞，出战的宠物减这个（主人 10 级以下减半）
	loyalPlayerDead = -1 * 100  // AI_FIX_PLAYERDEAD：主人在野外战斗倒下，出战的宠物减这个（主人 10 级以下减半）
	loyalPetDead    = -5 * 100  // AI_FIX_PETDEAD：宠物倒下，它自己减这个（主人 10 级以下减半）
)

// loyalMode 是宠物这回合不听话的模式（PETAI_MODE_*）。
type loyalMode uint8

const (
	loyalNormal   loyalMode = iota // 听话
	loyalRetarget                  // 随机换一个目标（PETAI_MODE_TARGETRANDOM）
	loyalRandom                    // 乱来：随机出一招（PETAI_MODE_RANDOMACT）
	loyalOwner                     // 打主人（PETAI_MODE_OWNERATTACK）
	loyalEnemy                     // 打敌人（PETAI_MODE_ENEMYATTACK）
	loyalEscape                    // 跑掉，离开战场（PETAI_MODE_ESCAPE）
	loyalNoSkill                   // 没有任何技能，不出手（PETAI_MODE_NOACT）
)

// reason 是协议 disobey 事件的 reason。
func (m loyalMode) reason() string {
	return [...]string{"", "retarget", "random", "owner", "enemy", "escape", "no_skill"}[m]
}

// obeyPlan 是宠物这回合的听话判定结果：cmd 是实际要执行的指令（听话时就是提交的指令；
// Action 为 none 表示这一回合不出手，lost_escape 表示跑掉）。mode 为 loyalNormal 以外时宠物忠诚不足。
type obeyPlan struct {
	cmd  *Cmd
	mode loyalMode
}

// obey 取宠物本回合的听话判定：没定过就现在定（loyaltyCheck）。
func (b *Battle) obey(u *Unit) *obeyPlan {
	if u.obey == nil {
		u.obey = b.loyaltyCheck(u, b.commandOf(u))
	}
	return u.obey
}

// loyalty 是宠物现在的忠诚（0–100）。
func (b *Battle) loyalty(u *Unit) int {
	return gamedata.LoyaltyWith(u.loyBase, u.loyBonus, u.loyVar)
}

// addLoyalty 给宠物的忠诚加上 delta（CHAR_PetAddVariableAi）。只对宠物有效。
func (b *Battle) addLoyalty(u *Unit, delta int) {
	if u == nil || u.Kind != KindPet || u.Auto || delta == 0 { // 分身的宠物不记忠诚（STAGE-162）
		return
	}
	u.loyVar = gamedata.AddVariableAI(u.loyVar, delta)
	b.loyalChanged = true
}

// loyaltyCheck 是 BATTLE_PetLoyalCheck：摇骰子决定宠物这回合听不听话，不听话就改写它的指令。
// 没有指令、不能行动、蓄力或隐身中的宠物不判定。
func (b *Battle) loyaltyCheck(u *Unit, cmd *Cmd) *obeyPlan {
	listen := &obeyPlan{cmd: cmd, mode: loyalNormal}
	if cmd == nil || u.pend != nil || !u.Alive() || !u.Status.CanAct() {
		return listen
	}
	ai := b.loyalty(u)
	if ai >= 80 {
		return listen
	}
	mode := pickLoyalMode(ai, b.roll(100))
	if mode == loyalNormal {
		return listen
	}
	if !petHasSkill(u) { // 没有任何技能：不论抽中哪一种，都是不出手（原版 PETAI_MODE_NOACT）
		mode = loyalNoSkill
	}
	return &obeyPlan{cmd: b.disobeyCmd(u, cmd, mode), mode: mode}
}

// pickLoyalMode 按忠诚 ai 和骰子 dice（RAND(1,100)，即 1–100）定不听话的模式，分档同原版：
// 80 以上听话；40–79 有机会随机换目标；20–39 七成乱来；10–19 八成打主人、其余打敌人；
// 10 以下六成打主人、四成跑掉。原版的判定是 Rand < N，所以概率比整数百分比少 1%（如 70–79 档是 9%）。
func pickLoyalMode(ai, dice int) loyalMode {
	switch {
	case ai >= 80:
		return loyalNormal
	case ai >= 40:
		if dice < retargetLimit(ai) {
			return loyalRetarget
		}
	case ai >= 20:
		if dice < 70 {
			return loyalRandom
		}
	case ai >= 10:
		if dice < 80 {
			return loyalOwner
		}
		return loyalEnemy
	default:
		if dice < 60 {
			return loyalOwner
		}
		return loyalEscape
	}
	return loyalNormal
}

// retargetLimit 是 40–79 档换目标的骰子上限（骰子小于它才换）。
func retargetLimit(ai int) int {
	switch {
	case ai >= 70:
		return 10
	case ai >= 60:
		return 20
	case ai >= 50:
		return 35
	}
	return 50
}

// petHasSkill 表示宠物至少有一个认识的技能。
func petHasSkill(u *Unit) bool {
	for _, id := range u.Skills {
		if _, ok := gamedata.PetSkillByID(id); ok {
			return true
		}
	}
	return false
}

// battleSkillSlots 是宠物能在战斗中用的技能栏（0 起的下标，同 Unit.Skills）。
func battleSkillSlots(u *Unit) []int {
	var out []int
	for i, id := range u.Skills {
		if sk, ok := gamedata.PetSkillByID(id); ok && sk.Field == gamedata.PetSkillFieldBattle {
			out = append(out, i)
		}
	}
	return out
}

// disobeyCmd 把不听话的宠物原本的指令改成它这回合实际要做的（BATTLE_PetLoyalCheck 的 switch）。
// 挑目标用随机，与原版的 BATTLE_DefaultAttacker 一样只看还站着、没隐身的单位。
func (b *Battle) disobeyCmd(u *Unit, cmd *Cmd, mode loyalMode) *Cmd {
	none := &Cmd{Action: "none"}
	switch mode {
	case loyalRetarget:
		// 防御或者打的是自己：这回合不出手；否则在原来目标所在的那一方里重新挑一个。
		if cmd.Action == "guard" || cmd.Target == u.No {
			return none
		}
		t := b.randomTarget(targetSide(cmd.Target))
		if t == nil {
			return none
		}
		next := *cmd
		next.Target = t.No
		return &next
	case loyalRandom:
		// 打的是自己：不出手；否则随机用一个战斗中能用的技能，打对方一方的随机单位。
		slots := battleSkillSlots(u)
		if cmd.Target == u.No || len(slots) == 0 {
			return none
		}
		t := b.randomTarget(1 - u.Side())
		if t == nil {
			return none
		}
		return &Cmd{Action: "skill", Skill: slots[b.Rnd(len(slots))] + 1, Target: t.No}
	case loyalOwner:
		return &Cmd{Action: "attack", Target: b.ownerNo(u)}
	case loyalEnemy:
		t := b.randomTarget(1 - u.Side())
		if t == nil {
			return none
		}
		return &Cmd{Action: "attack", Target: t.No}
	case loyalEscape:
		return &Cmd{Action: "lost_escape"}
	}
	return none
}

// targetSide 是单位号所在的方（原版 BATTLE_TargetAdjust 的 toSide：0–9 是 0 方，其余是 1 方）。
func targetSide(no int) int {
	if no >= 0 && no < unitsPerSide {
		return 0
	}
	return 1
}

// clearGuardian 撤销这只宠物本回合登记的忠犬（STAGE-127）：原版在 PETAI_MODE_RANDOMACT 分支清掉
// CHAR_BATTLEFLG_GUARDIAN（battle.c:5839），乱来的宠物这回合不再替主人挡刀。它和原版一样在宠物
// 出手（act）时清，所以主人在这之前挨打仍然由它替挡。
func (b *Battle) clearGuardian(u *Unit) {
	for slot, no := range b.guards {
		if no == u.No {
			delete(b.guards, slot)
		}
	}
}

// disobeyEvent 是宠物不听话的提示（协议 10.7 的 disobey），紧接在它这回合的行动之前。
func disobeyEvent(u *Unit, mode loyalMode) Event {
	no := u.No
	return Event{Kind: "disobey", Actor: &no, Reason: mode.reason()}
}

// petLostEscape 是宠物不听话跑掉（原版 BATTLE_LostEscape）：宠物离开战场，人物不跑，战斗继续。
// 宠物的血量和忠诚照常留在它的单位上，结束时写回。
func (b *Battle) petLostEscape(u *Unit) Event {
	no := u.No
	b.leave(u)
	return Event{Kind: "pet_escape", Actor: &no, Left: []int{no}}
}

// onRecovered 是宠物被道具或精灵回复（血量加上了）：每场战斗只加一次忠诚（AI_FIX_PETRECOVERY）。
func (b *Battle) onRecovered(u *Unit) {
	if u.Kind == KindPet && !u.recovered {
		u.recovered = true
		b.addLoyalty(u, loyalRecovered)
	}
}

// settleLoyalty 在战斗结束时给宠物的忠诚记账（battle.c 的胜利结算与人物被打飞）。
// 胜利：在场的宠物每打倒一只敌人加一次，敌人比宠物高级时多加（AI_FIX_PETGOLDWIN）；
// 失败且人物是被打飞的：出战的宠物减一次（AI_FIX_PLAYERULTIMATE，人物 10 级以下减半）。
func (b *Battle) settleLoyalty() {
	if b.party { // 真人同场：每个真人各自的宠物（party_loyalty.go）
		b.settlePartyLoyalty()
		return
	}
	pet := b.PetUnit()
	if pet == nil {
		return
	}
	switch b.result {
	case "win":
		for _, m := range b.slain {
			if m.Level > pet.Level {
				b.addLoyalty(pet, loyalWinHigher)
			} else {
				b.addLoyalty(pet, loyalWin)
			}
		}
	case "lose":
		if b.PlayerFlown() {
			b.addLoyalty(pet, loyalFlown/b.ownerLevelDiv())
		}
	}
}

// onDown 是单位新倒下时的忠诚扣减（BATTLE_NormalDeadExtra，只在野外战斗里）：
// 人物倒下，出战的宠物减 loyalPlayerDead；宠物倒下，它自己减 loyalPetDead。
// 被打飞的人物不在这里扣（BATTLE_UltimateExtra 的那一支），它的扣减在 settleLoyalty 结算。
// 目前的战斗都是野外或 NPC 发起的，没有原版的无风险（norisk，道场）和人物对人物战斗，所以不用判断。
func (b *Battle) onDown(u *Unit) {
	if b.duel != nil { // 决斗：记决斗点数，没有宠物忠诚下降（pk.md 2.1，duel_rules.go）
		b.duelOnDown(u)
		return
	}
	if u.Auto { // 分身倒下不影响队长宠物的忠诚（STAGE-162）
		return
	}
	owner := b.actorNo(u) // 真人同场时是这个队员的宠物（party_actor.go）
	switch u.Kind {
	case KindPlayer:
		if !u.Flown {
			b.addLoyalty(b.petOf(owner), loyalPlayerDead/b.ownerLevelDivFor(owner))
		}
	case KindPet:
		b.addLoyalty(u, loyalPetDead/b.ownerLevelDivFor(owner))
	}
}

// ownerLevelDiv 是忠诚扣减的除数：主人 10 级及以下减半（原版的 levelflg，取值 1 或 2）。
func (b *Battle) ownerLevelDiv() int { return b.ownerLevelDivFor(0) }

// ownerLevel 是主人（人物）的等级。STAGE-252：人物逃跑离场后 PlayerUnit 为 nil，退回开战时的快照 sides[0]。
func (b *Battle) ownerLevel() int { return b.ownerLevelOf(0) }
