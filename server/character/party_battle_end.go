package character

// 真人同场战斗的结束与写回（STAGE-275，docs/social-players.md 2.6，docs/protocol.md 10.14）：
// 每人各收一条 battle_end（只含本人份额），人物、宠物、魔法、装备、奖励各自写回，在线状态恢复。
// 掉线的人不推送、只写回血量和宠物（10.14 掉线）。

import (
	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// finish 结束整场战斗：还在场的真人按全场结果结算（胜负、奖励各自算），然后战斗摘掉。调用时持有 lockAll。
func (pb *partyBattle) finish() {
	pb.stopTimers()
	pb.done = true
	for _, no := range pb.active() {
		pb.endMember(no, pb.b.EndFor(no), true)
	}
	if pb.b.IsDuel() {
		pb.s.finishDuelLog(pb)
	}
}

// endMember 把一名真人从这场战斗里放出来：写回他自己的数据，推 battle_end（push 为真且 end 不为空），
// 摘掉战斗状态并推送这个人的数据变化。调用时持有 lockAll；号位在这里标记为离场。
func (pb *partyBattle) endMember(no int, end *battle.EndView, push bool) {
	s, p := pb.s, pb.seats[no]
	pb.gone[no] = true
	if pb.b.IsDuel() {
		pb.endDuelMember(no, end, push)
		return
	}
	hu := pb.b.FinalHuman(no)
	flown := hu != nil && hu.Flown && end != nil && end.Result == "lose" // 倒下且被打飞：writeBackMember 会换图
	dropped := s.writeBackMember(&p.rec, pb.b, no, end)
	crushed := applyEquipCrushFor(&p.rec, pb.b, no)
	s.saveItemCrush(p.charID, crushed)
	if push && end != nil {
		p.pushAll("battle_end", end)
	}
	p.battle = nil
	if end != nil { // STAGE-282：留给迟到的 battle_ack 认账（与单人 finishBattle 一致）
		p.lastBattleTurn = end.Turn
	}
	p.players.moved(p) // 战斗结束可能被打飞换图，等级也可能变
	p.players.touch(p)
	if push { // push 为假是掉线：连接正在断开，不再给它推送（否则写消息失败会打 WARN）
		if no == 0 && p.encounterOn {
			// 队长的遇敌开关：打完一场接着开下一场（同单人，见 finishBattle 的遇敌段）。
			if flown && end != nil && end.Result == "lose" {
				p.stopEncounter(encounterOffLose)
			} else {
				s.scheduleEncounter(p)
			}
		}
		p.pushAll("status_changed", map[string]any{"status": p.rec.status()})
		if pb.b.PetsChanged() {
			p.pushAll("pets_changed", p.rec.petsView())
		}
		if flown {
			// 打飞换了位置：只推本人的位置（队员的跟随由队长的 position 推送统一处理，不在持锁时再走跟随）。
			p.pushAll("position_changed", followPositionOf(&p.rec))
		}
	}
	s.saveProgress(p) // STAGE-290：升级、经验、金币立即存盘
	s.savePets(p.charID, p.rec.Pets)
	s.saveSkillPoints(p.charID, p.rec.SkillPoints)
	if len(dropped) > 0 {
		s.saveBattleItems(p.charID, dropped)
	}
	if push && (len(dropped) > 0 || len(crushed) > 0) {
		p.pushAll("inventory_changed", p.rec.inventoryView())
	}
}

// writeBackMember 把这名真人（号位 no）的血量、魔法、宠物写回人物记录；end 不为空时再按他自己的结局结算
// （奖励、倒下的惩罚）。返回装进背包的掉落。掉线（end 为空）只写血量和宠物。
func (s *Service) writeBackMember(r *record, b *battle.Battle, no int, end *battle.EndView) []itemRec {
	hu := b.FinalHuman(no)
	if hu != nil {
		r.HP, r.MP, r.Overkill = hu.HP, hu.MP, hu.Overkill
	}
	if r.HP < 1 {
		r.HP = 1 // 掉线时倒下的人回 1 血（exitBattleOffline 的规则）
	}
	r.MagicExp, r.MagicResist, r.MagicAttExp, r.MagicDefExp = b.PlayerMagicFor(no)
	applyPetEnds(r, b.PetEndsFor(no))
	if end == nil {
		return nil
	}
	if end.Result == "lose" {
		if hu != nil && hu.Alive() {
			r.HP = max(r.HP, 1) // 没倒下的人不受惩罚，血量照存
			return nil
		}
		s.applyMemberDefeat(r, hu, end)
		return nil
	}
	return applyMemberRewards(r, b, end)
}

// applyMemberDefeat 是倒下的真人的失败结算：原地复活或被打飞（同单人 applyResult 的失败段）。
func (s *Service) applyMemberDefeat(r *record, hu *battle.Unit, end *battle.EndView) {
	flown := hu != nil && hu.Flown
	if flown {
		f, x, y, spOK := SavepointRespawn(r)
		if !spOK {
			h := gamedata.Hometowns[r.Hometown]
			f, x, y = h.Floor, h.X, h.Y
		}
		r.Floor, r.X, r.Y, r.Direction = f, x, y, gamedata.DefaultDirection
	}
	r.HP = 1
	r.Charm = gamedata.ClampCharm(r.Charm + charmOnDeath(r.Level, flown))
	if end.Penalty != nil {
		end.Penalty.Respawn = r.position()
		end.Penalty.InPlace = !flown
	}
}

// applyMemberRewards 是胜利时这名真人自己份额的写回：经验（只给出手者，引擎已按人算好）、宠物击杀、金钱和掉落。
// 对照 applyResult 的胜利段；返回装进背包的掉落。
func applyMemberRewards(r *record, b *battle.Battle, end *battle.EndView) []itemRec {
	if end.Rewards == nil {
		return nil
	}
	for i, e := range end.Rewards.Exp {
		if e.Who == "player" {
			r.Exp += e.Gain
			to, points, remain := levelUp(r.Level, r.Exp, 3)
			if to > r.Level {
				r.Charm = gamedata.ClampCharm(r.Charm + gamedata.CharmOnLevelUp)
			}
			r.Level, r.SkillPoints, r.Exp = to, r.SkillPoints+points, remain
			end.Rewards.Exp[i].LevelTo = to
			end.Rewards.Exp[i].SkillPoints = points
		} else if e.Who == "pet" {
			for j := range r.Pets {
				if e.Slot != nil && r.Pets[j].Slot == *e.Slot {
					end.Rewards.Exp[i].LevelTo = gainPetExp(&r.Pets[j], e.Gain, b.Rnd)
				}
			}
		}
	}
	for _, k := range end.Rewards.PetKills {
		for j := range r.Pets {
			if r.Pets[j].Slot == k.Slot {
				r.Pets[j].Kills += k.Kills
			}
		}
	}
	r.Gold += end.Rewards.Gold
	var dropped []itemRec
	end.Rewards.Items, dropped = placeRewardItems(r, end.Rewards.Items)
	return dropped
}

// applyEquipCrushFor 是真人 no 的装备耐久写回（同 applyItemCrush，取他自己的装备）。返回变化的行，供写库。
func applyEquipCrushFor(r *record, b *battle.Battle, no int) []itemCrushWrite {
	var out []itemCrushWrite
	for slot, e := range b.EquipStateFor(no) {
		if e == nil {
			continue
		}
		it := findItem(r.Items, slot)
		if it == nil {
			continue
		}
		if !e.Gone && it.Durability == e.Durability && it.CrushLevel == e.CrushLevel {
			continue
		}
		out = append(out, itemCrushWrite{Slot: slot, Gone: e.Gone, Durability: e.Durability, CrushLevel: e.CrushLevel})
		if e.Gone {
			r.Items = removeSlot(r.Items, slot)
			continue
		}
		it.Durability, it.CrushLevel = e.Durability, e.CrushLevel
	}
	return out
}
