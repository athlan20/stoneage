package character

// STAGE-306：决斗的结束与写回（docs/pk.md 2.1、2.2、4.3，协议 5.74 与 10.14）。
// 每名真人离开战斗（打完、逃跑、被踢出、掉线）时：血量魔法、宠物、装备耐久、精灵熟练度各自写回，
// 决斗点数按引擎的账本结算并写库，接受决斗的开关自动关。没有经验、石币、掉落，也没有魅力和送回记录点的惩罚；
// 倒下的人物 1 血留在原地。一场决斗在 duel_logs 里一行，开战时插入，结束时补全。

import (
	"context"
	"encoding/json"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/duel"
)

// duelLogTimeout 是写 duel_logs 的数据库超时；日志写失败只记 WARN，不影响战斗。
const duelLogTimeout = 3 * time.Second

// duelMemberLog 是 duel_logs.members 的一项。
type duelMemberLog struct {
	No     int    `json:"no"`
	CharID int64  `json:"char_id"`
	Name   string `json:"name"`
}

// duelChangeLog 是 duel_logs.dp_changes 的一项。
type duelChangeLog struct {
	No     int    `json:"no"`
	Name   string `json:"name"`
	Change int    `json:"change"`
	Total  int    `json:"total"`
}

// endDuelMember 把一名真人从决斗里放出来（endMember 的决斗分支）。调用时持有 lockAll，号位已标记离场。
func (pb *partyBattle) endDuelMember(no int, end *battle.EndView, push bool) {
	s, p, b := pb.s, pb.seats[no], pb.b
	r := &p.rec
	if hu := b.FinalHuman(no); hu != nil {
		r.HP, r.MP = hu.HP, hu.MP
	}
	if r.HP < 1 {
		r.HP = 1 // 倒下的人物 1 血留在原地（pk.md 2.1）
	}
	r.MagicExp, r.MagicResist, r.MagicAttExp, r.MagicDefExp = b.PlayerMagicFor(no)
	applyPetEnds(r, b.PetEndsFor(no))
	before := r.DuelPoint
	r.DuelPoint = duel.Settle(before, b.DuelDelta(no))
	change := &battle.DuelPointView{Change: r.DuelPoint - before, Total: r.DuelPoint}
	if end != nil {
		end.DuelPoint = change
	}
	pb.dpChanges = append(pb.dpChanges, duelChangeLog{No: no, Name: r.Name, Change: change.Change, Total: change.Total})
	s.saveDuelPoint(p.charID, r.DuelPoint)
	crushed := applyEquipCrushFor(r, b, no)
	s.saveItemCrush(p.charID, crushed)
	if push && end != nil {
		p.pushAll("battle_end", end)
	}
	p.battle = nil
	if end != nil {
		p.lastBattleTurn = end.Turn // 留给迟到的 battle_ack 认账
	}
	switchWas := r.SocialDuel
	r.SocialDuel = false // 每打完一场自动关（胜负平、逃跑、被踢都算，pk.md 2.1）
	p.players.moved(p)
	p.players.touch(p)
	if push {
		if switchWas {
			p.pushAll("social_changed", map[string]any{"social": socialView(r)})
		}
		p.pushAll("status_changed", map[string]any{"status": r.status()})
		p.pushAll("pets_changed", r.petsView())
		if len(crushed) > 0 {
			p.pushAll("inventory_changed", r.inventoryView())
		}
		if p.encounterOn {
			s.scheduleEncounter(p) // 战斗前开着自动遇敌的，打完接着遇敌
		}
	}
	s.saveProgress(p)
	s.savePets(p.charID, r.Pets)
}

// saveDuelPoint 把决斗点数写库（立即写，不等存盘）。数值变了才更新 duel_point_at（强者表同分排序用，STAGE-308）。失败只记日志：内存里的值仍是结算后的。
func (s *Service) saveDuelPoint(charID int64, dp int) {
	ctx, cancel := context.WithTimeout(context.Background(), duelLogTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE characters SET duel_point = $2, duel_point_at = CASE WHEN duel_point = $2 THEN duel_point_at ELSE now() END WHERE id = $1`, charID, dp); err != nil {
		s.opts.Logger.Warn("决斗点数写库失败", "character", charID, "err", err)
	}
}

// startDuelLog 在开战时插入 duel_logs 一行（battle_id、地图、双方成员）。调用时持有全员的 mu。
func (s *Service) startDuelLog(b *battle.Battle, seats []*Presence) {
	var members []duelMemberLog
	floor := 0
	for no, p := range seats {
		if p == nil {
			continue
		}
		if no == 0 {
			floor = p.rec.Floor
		}
		members = append(members, duelMemberLog{No: no, CharID: p.charID, Name: p.rec.Name})
	}
	raw, _ := json.Marshal(members)
	ctx, cancel := context.WithTimeout(context.Background(), duelLogTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, `INSERT INTO duel_logs (battle_id, started_at, floor, members) VALUES ($1, $2, $3, $4::jsonb)`,
		b.ID, s.opts.Now(), floor, string(raw)); err != nil {
		s.opts.Logger.Warn("决斗日志写库失败", "battle", b.ID, "err", err)
	}
}

// finishDuelLog 在决斗结束时补全 duel_logs：结束时间、每人的 DP 变化、结果（以发起方的角度：win / lose / draw）。
func (s *Service) finishDuelLog(pb *partyBattle) {
	raw, _ := json.Marshal(pb.dpChanges)
	ctx, cancel := context.WithTimeout(context.Background(), duelLogTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE duel_logs SET ended_at = $2, dp_changes = $3::jsonb, result = $4 WHERE battle_id = $1`,
		pb.b.ID, s.opts.Now(), string(raw), pb.b.Result()); err != nil {
		s.opts.Logger.Warn("决斗日志写库失败", "battle", pb.b.ID, "err", err)
	}
}
