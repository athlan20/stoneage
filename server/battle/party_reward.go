// 真人同场战斗的结算（STAGE-275，docs/social-players.md 2.6、docs/battle-party-research.md 第 1–2、5 节）：
// 经验只给击杀者（组合技的成员各得一份），掉落每件随机分给击杀者中还活着的一人，捕获的宠物归捕获者。
// 每人各收一条 battle_end（EndFor），只含本人的份额；死亡的真人拿不到任何奖励（原版 BATTLE_GetExpGold）。
package battle

import "github.com/athlan20/stoneage/server/gamedata"

// recordKillers 记下击倒怪物的出手单位（击杀归属）。没有出手者（状态致死等）时记为队长，同单人战斗的结果。
func (b *Battle) recordKillers(m *Unit) {
	nos := append([]int(nil), b.acting...)
	if len(nos) == 0 {
		nos = []int{0}
	}
	b.killers[m] = nos
}

// ownerOf 是单位所属的真人号：人物是自己，宠物是主人（宠物号 - 5）。
// 决斗里对方的真人占 10–14，宠物 15–19，主人号 = 10 + 栏位（STAGE-306）。
func ownerOf(no int) int { return no/unitsPerSide*unitsPerSide + no%unitsPerSide%5 }

// killedBy 判断怪物 m 是不是由单位 no 击倒的（组合技成员和出手者都算）。
func (b *Battle) killedBy(m *Unit, no int) bool {
	for _, k := range b.killers[m] {
		if k == no {
			return true
		}
	}
	return false
}

// assignDrops 把本场击杀的怪物掉落分给击杀者中还活着的真人（每件单独随机一人，同 2.6）。
// 在战斗结束判定胜利时调用一次，结果存进 partyDrops；死亡的击杀者拿不到，候选为空时这件掉落不发。
func (b *Battle) assignDrops() {
	b.partyDrops = map[int][]int{}
	for _, m := range b.slain {
		for _, id := range m.Drops {
			var cands []int
			for _, k := range b.killers[m] {
				h := ownerOf(k)
				if u := b.FinalHuman(h); u != nil && u.Alive() && !containsInt(cands, h) {
					cands = append(cands, h)
				}
			}
			if len(cands) == 0 {
				continue
			}
			pick := cands[b.Rnd(len(cands))]
			b.partyDrops[pick] = append(b.partyDrops[pick], id)
		}
	}
	for no, ids := range b.partyDrops {
		b.partyDrops[no] = capDrops(ids, b.Rnd)
	}
}

// capDrops 是单人的背包上限（最多 3 件，第 4 件起一半概率换掉随机一件），同 distributeDrops 的规则。
func capDrops(ids []int, rnd func(n int) int) []int {
	slots := [3]int{-1, -1, -1}
	for _, id := range ids {
		placed := false
		for i := range slots {
			if slots[i] == -1 {
				slots[i] = id
				placed = true
				break
			}
		}
		if !placed && rnd(2) == 1 {
			slots[rnd(3)] = id
		}
	}
	var out []int
	for _, id := range slots {
		if id != -1 {
			out = append(out, id)
		}
	}
	return out
}

func containsInt(list []int, x int) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}

// partyRewards 是真人 no 在胜利时的奖励份额：经验（人物和他的宠物，各按击杀者算）、金币、掉落、捕获的宠物。
func (b *Battle) partyRewards(no int) *Rewards {
	r := &Rewards{}
	h := b.FinalHuman(no)
	if h == nil {
		return r
	}
	rate := b.expRate()
	var humanExp int64
	var petExp int64
	pet := b.units[no+5]
	if pet == nil {
		pet = b.finalPet(no)
	}
	for _, m := range b.slain {
		if b.killedBy(m, no) {
			humanExp += levelDiffExp(h.Level, m.Level, killExp(m))
		}
		if pet != nil && b.killedBy(m, pet.No) {
			petExp += levelDiffExp(pet.Level, m.Level, killExp(m))
		}
	}
	r.Exp = append(r.Exp, RewardExp{Who: "player", Name: h.Name, Gain: applyExpRate(humanExp, rate),
		LevelFrom: h.Level, LevelTo: h.Level})
	if pet != nil && petExp > 0 {
		r.Exp = append(r.Exp, RewardExp{Who: "pet", Slot: intPtr(pet.PetSlot), Name: pet.Name, Gain: applyExpRate(petExp, rate),
			LevelFrom: pet.Level, LevelTo: pet.Level})
	}
	side := b.mateSide(no)
	r.Gold = battleGoldGain(h.Level, side.Gold)
	for _, id := range b.partyDrops[no] {
		name := ""
		if def, ok := gamedata.ItemByID(id); ok {
			name = def.Name
		}
		r.Items = append(r.Items, RewardItem{ItemID: id, Name: name, Count: 1})
	}
	for _, m := range b.captur {
		if owner, ok := b.captors[m]; ok && ownerOf(owner) == no {
			r.Pets = append(r.Pets, RewardPet{Slot: m.PetSlot, Name: m.Name, Level: m.Level})
		}
	}
	for _, p := range b.petsActed {
		if ownerOf(p.No) == no && p.Alive() && p.Kills > 0 {
			r.PetKills = append(r.PetKills, PetKill{Slot: p.PetSlot, Kills: p.Kills})
		}
	}
	return r
}

// mateSide 返回真人 no 的开战快照（队长是 sides[0]）。
func (b *Battle) mateSide(no int) *SideInput {
	if no == 0 {
		return b.sides[0]
	}
	return b.mates[no].Side
}

// finalPet 返回真人 no 离场后的出战宠物（离场时宠物从场上移走，结算仍要它的经验）。
func (b *Battle) finalPet(no int) *Unit {
	if no == 0 {
		return b.leftPet
	}
	return b.finalPets[no]
}

// EndFor 是真人 no 的 battle_end 内容（10.14）：离场的按离场原因给出（escape、timeout 的结局；offline 不推送，返回 nil），
// 战斗在场的按全场结果给出。战斗还没结束且人还在场时返回 nil。
func (b *Battle) EndFor(no int) *EndView {
	switch b.left[no] {
	case "escape":
		return &EndView{BattleID: b.ID, Result: "escaped", Turn: b.turn}
	case "timeout":
		return &EndView{BattleID: b.ID, Result: "forced_out", Turn: b.turn}
	case "offline":
		return nil
	}
	if !b.over {
		return nil
	}
	if b.duel != nil {
		return b.duelEnd(no)
	}
	h := b.FinalHuman(no)
	if h == nil {
		return nil
	}
	v := &EndView{BattleID: b.ID, Result: b.result, Turn: b.turn}
	switch b.result {
	case "win":
		if h.Alive() {
			v.Rewards = b.partyRewards(no)
		} else {
			v.Rewards = &Rewards{} // 死亡的真人没有奖励（BATTLE_GetExpGold 直接返回 0）
		}
		if b.NPC != nil && b.NPC.End != "" {
			v.NPC = &NPCEnd{NPCRef: b.NPC.NPCRef, Say: b.NPC.End}
		}
	case "lose":
		v.Penalty = &Penalty{}
	}
	return v
}
