// 真人同场战斗的宠物忠诚结算（STAGE-275 第二轮，docs/social-players.md 2.6）：每个真人各自的出战宠物按自己的结果记账。
// 仍在场的真人和逃跑离场的真人（逃跑前是正常参战）结算；掉线（offline）与超时踢出（timeout，forced_out 无奖励无惩罚）不结算。

package battle

// settlePartyLoyalty 是真人同场时 settleLoyalty 的分支。胜利时每只宠物按全场击杀记一次（同单人，敌人比宠物高级时多加）；
// 失败时被打飞的真人，他的出战宠物减一次（同单人的 AI_FIX_PLAYERULTIMATE，按这个真人的等级算）。
func (b *Battle) settlePartyLoyalty() {
	for no := 0; no < partyMaxMates+1; no++ {
		if !b.settlesLoyalty(no) {
			continue
		}
		pet := b.petOf(no)
		if pet == nil {
			pet = b.finalPet(no) // 逃跑时一起离场的出战宠物（同单人的 leftPet，STAGE-247）
		}
		if pet == nil {
			continue
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
			if h := b.FinalHuman(no); h != nil && h.Flown {
				b.addLoyalty(pet, loyalFlown/b.ownerLevelDivFor(no))
			}
		}
	}
}

// settlesLoyalty 判断真人 no 的宠物要不要在结束时结算：还在场，或者是逃跑离场的。
func (b *Battle) settlesLoyalty(no int) bool {
	switch b.left[no] {
	case "":
		return b.FinalHuman(no) != nil
	case "escape":
		return true
	}
	return false
}
