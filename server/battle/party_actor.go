// 真人同场战斗里“谁的人物、背包、精灵、宠物”的读取（STAGE-275 第二轮）。
// 单人战斗的读取都落在队长（sides[0]、PlayerUnit、commandPet）上；真人同场时每个真人各读自己的：
// 队员的物品、精灵、捕捉、换宠按自己的快照和宠物栏位走。no 是人物单位号（队长 0，队员 1–4）。
// 单人战斗与分身队里 no 永远是 0，这里的函数都退回原来的队长读法，行为不变。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// actorNo 是单位的主人（人物单位号）：真人同场时队员及其宠物是各自的号，其余（队长、分身、怪物）都是 0。
func (b *Battle) actorNo(u *Unit) int {
	if b.isMateUnit(u) {
		return ownerOf(u.No)
	}
	return 0
}

// sideOfNo 返回真人 no 的开战快照（背包、精灵、备用宠物都在这里）：队长是 sides[0]，队员是自己的快照。
func (b *Battle) sideOfNo(no int) *SideInput {
	if b.party && no != 0 && b.mates[no] != nil {
		return b.mates[no].Side
	}
	return b.sides[0]
}

// humanOf 返回真人 no 的人物单位；他不在场（逃跑、超时、掉线）时返回 nil。
func (b *Battle) humanOf(no int) *Unit {
	if no == 0 {
		return b.PlayerUnit()
	}
	return b.units[no]
}

// petOf 返回真人 no 站在格子里的出战宠物；没有时返回 nil。队长的宠物同 commandPet。
func (b *Battle) petOf(no int) *Unit {
	if no == 0 {
		return b.commandPet()
	}
	if p := b.units[no+firstPetNo]; p != nil && p.Kind == KindPet {
		return p
	}
	return nil
}

// petSlots 返回真人 no 的宠物栏位（同 SideInput.Bench 的 20 个栏位，装着在场或换下的宠物单位，血量和忠诚随战斗保留）。
func (b *Battle) petSlots(no int) *[PetSlots]*Unit {
	if no == 0 {
		return &b.pets
	}
	return &b.matePets[no]
}

// humanNos 返回要逐个登记或结算的真人号。单人战斗只有队长（不论他在不在场，同原来的读法）；
// 真人同场时是还在场上的真人，离场的人不再出指令、不再登记效果。
func (b *Battle) humanNos() []int {
	if !b.party {
		return []int{0}
	}
	return b.inBattleHumans()
}

// slotMagicFor 取真人 no 的精灵栏位上的模板，MP 用他自己快照里这个栏位的施放消耗。
func (b *Battle) slotMagicFor(no, slot int) (gamedata.MagicTemplate, bool) {
	side := b.sideOfNo(no)
	return b.magicInSide(side, side.Magics, slot)
}

// ownerLevelOf 是真人 no 的等级：在场时取人物当前等级，不在场时取开战时的快照。
func (b *Battle) ownerLevelOf(no int) int {
	if u := b.humanOf(no); u != nil {
		return u.Level
	}
	return b.sideOfNo(no).Level
}

// ownerLevelDivFor 是真人 no 的忠诚扣减除数（主人 10 级及以下减半，同 ownerLevelDiv）。
func (b *Battle) ownerLevelDivFor(no int) int {
	if lv := b.ownerLevelOf(no); lv <= 10 {
		return 2
	}
	return 1
}

// withoutActor 返回去掉某个身份（player / pet）之后的需要指令列表，队员换宠后新宠物本回合不能行动。
func withoutActor(list []string, actor string) []string {
	out := list[:0]
	for _, a := range list {
		if a != actor {
			out = append(out, a)
		}
	}
	return out
}

// placeCaptured 返回捕捉成功后给怪物安排宠物栏位的回调。队长（和单人战斗）用 PlaceCaptured；
// 真人同场时队员捕捉的宠物归他自己，用 PlaceCapturedFor（没有设置时返回 nil，这次只记录，不入栏位）。
func (b *Battle) placeCaptured(u *Unit) func(*Unit, *CaptureKeep) (int, *Discarded) {
	owner := b.actorNo(u)
	if owner == 0 {
		if b.Cbs.PlaceCaptured == nil {
			return nil
		}
		return b.Cbs.PlaceCaptured
	}
	if b.Cbs.PlaceCapturedFor == nil {
		return nil
	}
	return func(m *Unit, keep *CaptureKeep) (int, *Discarded) {
		return b.Cbs.PlaceCapturedFor(owner, m, keep)
	}
}
