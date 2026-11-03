// 真人同场战斗（STAGE-275，docs/social-players.md 2.6、docs/protocol.md 10.14）：一场战斗容纳 2–5 个真人，
// 队长占 0 号，队员占 1–4 号，每人带自己的出战宠物（宠物号 = 主人号 + 5）。
// Battle.party 为 false 的单人战斗和分身队战斗不经过这里的分支，行为与之前完全一致。
//
// 引擎对上层（队伍对象、character 服务）暴露的接口与调用顺序：
//
//	开战    NewParty(id, scene, leader, mates, monsters, rnd)
//	        leader 是队长快照，mates 是 1–4 号队员快照（各带自己的背包、精灵和备用宠物）。
//	        Clock 默认 time.Now，测试换成注入的时钟。
//	每回合  1) SubmitFor(no, turn, actor, action, cmd)：真人 no 交指令（actor 为 player / pet）。
//	           返回 next（pet / wait）或协议错误；本回合第一条被接受的指令起算 PartyCommandWait（120 秒）截止。
//	        2) ExpireDue(now)：到截止时仍欠指令的真人被踢出（timeout），返回 Departure 列表；
//	           队长被踢时 Disband 为真，上层解散队伍。没有默认指令。
//	        3) ExpireBattle(now)：整场时限（PartyTimeLimit，1 小时）到了返回 true，战斗以 draw 结束（Over 为真）。
//	        4) Offline(no)：上层发现真人掉线时调用，返回 Departure；掉线不结算，宠物随之退场，血量保留。
//	        5) TakeDepartures()：取走之后产生的所有离场（escape / timeout / offline），每次取完清空。
//	           上层据此离队、解散（队长离场）、推送 battle_leave / party_changed。
//	        6) AllSubmitted() 为真（progress() 的 Done 等于 Total）时调用 Settle()，结算一回合，
//	           返回 battle_round 的内容；真人同场时 Settle 的第二个返回值恒为 nil，每人的 battle_end 由 EndFor 给出。
//	结束    Over() 为真后，对每名真人调用 EndFor(no)：
//	        逃跑、超时离场的当场给出 escaped / forced_out，掉线返回 nil（不推送）；在场的按全场结果给出（含奖励）。
//	        写回数据按人取：FinalHuman(no)（血量魔法）、PetEndsFor(no)（宠物）、EquipStateFor(no)（装备耐久）、
//	        PlayerMagicFor(no)（精灵熟练度）、Captured()（捕获的怪物，按 captors 归捕获者）。
//	注意    离场的真人再交指令得到 COMMAND_NOT_NEEDED；离场只影响它自己，其余真人的指令与结算照常。
//	        Settle 之前 AllSubmitted 为假时不要结算（会按未交处理）。
package battle

import (
	"fmt"
	"time"
)

// 交指令的截止时间与整场时限（原版 battle.c:2869-2870 的 120 秒、BATTLE_TIME_LIMIT 的 3600 秒）。
const (
	PartyCommandWait = 120 * time.Second
	PartyTimeLimit   = 3600 * time.Second
)

// PartyMate 是开战时一名队员的快照。No 为 1–4（队长固定 0，由 NewParty 的 leader 参数给出）。
type PartyMate struct {
	No   int
	Side *SideInput
}

// Mate 是战斗中的一名队员（队长以外）。人物与宠物离场后，它们的指针仍由 Battle.finalHuman 保留给结算。
type Mate struct {
	No    int
	Side  *SideInput
	needs []string // 本回合需要交的指令（同 Battle.needs，按人算）
}

// NewParty 开一场真人同场战斗。leader 是队长的快照（人物 0 号、出战宠物 5 号），mates 是 1–4 号的队员。
// 怪物的排法同 New。开战时 Clock 为 time.Now，测试可以换成注入的时钟。
func NewParty(id, scene string, leader *SideInput, mates []PartyMate, monsters []*Unit, rnd func(n int) int) (*Battle, error) {
	if len(mates) > partyMaxMates {
		return nil, fmt.Errorf("真人队员最多 %d 人，给了 %d 人", partyMaxMates, len(mates))
	}
	seen := map[int]bool{}
	for _, pm := range mates {
		if pm.No < 1 || pm.No > partyMaxMates || seen[pm.No] || pm.Side == nil {
			return nil, fmt.Errorf("队员的号码 %d 不合法或重复", pm.No)
		}
		seen[pm.No] = true
	}
	b := New(id, scene, leader, monsters, rnd)
	b.Kind = "party"
	b.initParty()
	for _, pm := range mates {
		b.addMate(pm)
	}
	b.refreshNeeds()
	return b, nil
}

// initParty 把刚建好的战斗转成真人同场（NewParty 与决斗的 NewDuel 共用）。
func (b *Battle) initParty() {
	b.party = true
	b.Clock = time.Now
	b.startedAt = b.now()
	b.mateCmds = map[int]*Cmd{}
	b.left = map[int]string{}
	b.finalHuman = map[int]*Unit{}
	b.killers = map[*Unit][]int{}
	b.captors = map[*Unit]int{}
	b.finalPets = map[int]*Unit{}
}

// addMate 把一名队员（或决斗里对方的真人）放上场：人物、出战宠物和备用宠物栏位。
func (b *Battle) addMate(pm PartyMate) {
	b.mates[pm.No] = &Mate{No: pm.No, Side: pm.Side}
	hu := humanUnit(pm.Side, pm.No)
	hu.crushOwner, hu.crush = true, copyEquips(pm.Side.Equips) // 队员本人的装备也会损坏（STAGE-209）
	b.units[pm.No] = hu
	for _, p := range pm.Side.Bench { // 备用宠物：换宠时取用，血量和忠诚随战斗保留（同队长的 New）
		if p != nil {
			b.matePets[pm.No][p.Slot] = petUnit(p, pm.No+firstPetNo)
		}
	}
	if p := pm.Side.BattlePet; p != nil {
		u := petUnit(p, pm.No+firstPetNo)
		b.units[pm.No+firstPetNo] = u
		b.matePets[pm.No][p.Slot] = u
	}
}

// partyMaxMates 是队员人数上限（队长以外）：队伍 5 人，含队长。
const partyMaxMates = 4

// IsParty 表示这是真人同场的战斗。
func (b *Battle) IsParty() bool { return b.party }

// now 返回引擎的当前时间：测试注入的时钟，没有时用 time.Now。
func (b *Battle) now() time.Time {
	if b.Clock != nil {
		return b.Clock()
	}
	return time.Now()
}

// isMateUnit 判断单位是不是队员（人物 1–4 或队员宠物 6–9）。队长和分身、怪物都不是。
func (b *Battle) isMateUnit(u *Unit) bool {
	if !b.party || u == nil || u.Auto || u.Kind == KindMonster {
		return false
	}
	o := ownerOf(u.No) // 队长（0 号及其宠物 5 号）的主人号是 0，不是队员
	return o != 0 && b.mates[o] != nil
}

// commandOf 是单位本回合的人物或宠物指令（队员从 mateCmds 取，其余同 cmds）。
func (b *Battle) commandOf(u *Unit) *Cmd {
	if b.isMateUnit(u) {
		return b.mateCmds[u.No]
	}
	if u.Kind == KindPet {
		return b.cmds["pet"]
	}
	return b.cmds["player"]
}

// refreshNeeds 按每个在场真人的状态重算本回合的 needs：能行动的人物要人物指令，
// 有出战宠物且能行动（没在蓄力）的要宠物指令。队长的 needs 仍放在 b.needs，队员放在各自的 Mate 里。
func (b *Battle) refreshNeeds() {
	b.needs = b.needs[:0]
	if u := b.PlayerUnit(); u != nil && u.Alive() && u.Status.CanAct() {
		b.needs = append(b.needs, "player")
	}
	// 蓄力或隐身中的宠物不要指令，回合开始直接按之前定下的出手（STAGE-76）。
	if p := b.commandPet(); p != nil && p.Alive() && p.Status.CanAct() && p.pend == nil {
		b.needs = append(b.needs, "pet")
	}
	for _, m := range b.mates {
		if m == nil {
			continue
		}
		m.needs = nil
		if h := b.units[m.No]; h != nil && h.Alive() && h.Status.CanAct() {
			m.needs = append(m.needs, "player")
		}
		if p := b.units[m.No+5]; p != nil && p.Alive() && p.Status.CanAct() && p.pend == nil {
			m.needs = append(m.needs, "pet")
		}
	}
}

// mateCmd 取队员某个身份（player / pet）本回合的指令。
func (b *Battle) mateCmd(no int, actor string) *Cmd {
	if actor == "pet" {
		return b.mateCmds[no+5]
	}
	return b.mateCmds[no]
}

// matesSubmitted 表示所有还需要交指令的队员都已经交齐。
func (b *Battle) matesSubmitted() bool {
	for _, m := range b.mates {
		if m == nil {
			continue
		}
		for _, actor := range m.needs {
			if b.mateCmd(m.No, actor) == nil {
				return false
			}
		}
	}
	return true
}

// ownerNo 是单位的主人（人物单位号）：队员的宠物是它的队员，其余是队长（b.You）。宠物打主人（loyalOwner）时用。
func (b *Battle) ownerNo(u *Unit) int {
	if b.isMateUnit(u) && u.Kind == KindPet {
		return u.No - 5
	}
	return b.You
}
