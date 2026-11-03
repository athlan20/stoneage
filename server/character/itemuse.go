// 使用消耗品（STAGE-30，docs/protocol.md 5.29 与 10.5 的 item 指令）：
// 战斗外 item_use 的校验与结算，以及战斗中道具消耗的写库。
// 效果与公式在 gamedata（itemuse.go），出处是原版服务端 item_event.c 的
// ITEM_useRecovery_Field：回复量 RAND(0.9N,1.1N)，HP 乘恢复率（人物 1+0.0001×体力、
// 宠物 1+0.00005×体力），MP 不乘；“全”是 HP 回满加 MP 回满（MP 只对人物）。

package character

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

type itemUseRequest struct {
	Slot   *int `json:"slot"`
	Target *int `json:"target"`
}

// handleItemUse 是 item_use（docs/protocol.md 5.29）：使用背包里的消耗品，
// 目标默认自己，1–MaxPetSlots 是宠物栏位 + 1（STAGE-346：1–20；一期没有队友，不再保留 6–10）。
func (s *Service) handleItemUse(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in itemUseRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := requiredItemSlot(in.Slot)
	if err != nil {
		return nil, err
	}
	if slot < firstBagSlot {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "使用物品要选背包格（5–19），装备格里的已经穿着")
	}
	target := 0 // 缺省自己
	if in.Target != nil {
		target = *in.Target
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var opErr error
	if err := l.Operate(func(r *record) {
		// 战斗中用回复道具走 battle_command 的 item 指令（协议 5.31、10.5）。
		// 注意：inBattle 返回的是 *protocol.Error，不能直接赋给 error 变量，否则 nil 也会变成非 nil 接口。
		if e := inBattle(l); e != nil {
			opErr = e
			return
		}
		if r.HP <= 0 {
			opErr = protocol.Errorf(protocol.CodeCharacterDead, "角色已经死亡，不能使用物品")
			return
		}
		out, opErr = s.useItem(ctx, l.p, r, slot, target)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return out, nil
}

// useItem 校验并结算一次战斗外的物品使用。调用时要持有角色的锁（在 Live.Operate 里）。
func (s *Service) useItem(ctx context.Context, p *Presence, r *record, slot, target int) (map[string]any, error) {
	it := findItem(r.Items, slot)
	if it == nil {
		return nil, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子没有物品", slot)
	}
	def, ok := gamedata.ItemByID(it.ItemID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "不认识的物品编号 %d", it.ItemID)
	}
	def = effectiveTemplate(def, it.Inlay) // 镶过的装备用实例的使用函数和参数（STAGE-298）
	if def.AbleUse == gamedata.AbleUseBattle {
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s 只能在战斗中使用", def.Name)
	}
	eff, usable := def.UseEffect()
	if !usable {
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s 现在还不能使用", def.Name)
	}
	if eff.Cure { // 解状态的物品原版只在战斗中生效（Field 分支为空），数据里也都是“战斗中”的场合
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s 只能在战斗中使用", def.Name)
	}
	if target < 0 || target > MaxPetSlots {
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "目标编号要在 0–%d 之间（0 自己，1–%d 是宠物栏位 + 1）", MaxPetSlots, MaxPetSlots)
	}
	if target != 0 && def.Target == gamedata.TargetMyself {
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%s 只能对自己使用", def.Name)
	}
	if target == 0 {
		if eff.SkillUp { // 英雄的祝福（STAGE-334）：给自己加一点未分配能力点，不看血量和气力
			return s.useSkillUp(ctx, p, r, slot)
		}
		if eff.NoEnemy { // 龙王的守护（STAGE-340）：使用后一段时间不遇敌
			return s.useNoEnemy(ctx, p, r, slot)
		}
		if eff.Loyalty > 0 { // 忠诚道具只给宠物用（STAGE-326）
			return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%s 只能给宠物使用，选 1–%d 号宠物", def.Name, MaxPetSlots)
		}
		return s.useItemOnPlayer(ctx, p, r, eff, slot)
	}
	pet, ok := r.petAt(target - 1)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%d 号目标没有宠物", target)
	}
	if eff.Loyalty > 0 { // 忠诚道具（STAGE-326）：目标宠物也不能倒下，其余规则同回血
		if pet.HP <= 0 {
			return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%s 已经倒下，不能作为目标", pet.Name)
		}
		return s.useLoyaltyOnPet(ctx, p, r, slot, pet, eff.Loyalty)
	}
	if pet.HP <= 0 {
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%s 已经倒下，不能作为目标", pet.Name)
	}
	if eff.HP == 0 && !eff.Full {
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%s 不能给宠物使用", def.Name)
	}
	return s.useItemOnPet(ctx, p, r, eff, slot, pet)
}

// petAt 取角色第 slot（0–19）栏的宠物。
func (r *record) petAt(slot int) (*petRec, bool) {
	for i := range r.Pets {
		if r.Pets[i].Slot == slot {
			return &r.Pets[i], true
		}
	}
	return nil, false
}

// useItemOnPlayer 给自己使用：回血（回魔），扣物品，推 status_changed 与 inventory_changed。
func (s *Service) useItemOnPlayer(ctx context.Context, p *Presence, r *record, eff gamedata.UseEffect, slot int) (map[string]any, error) {
	if playerNothingToRecover(r, eff) { // 血量、气力都已满：不扣道具、不改状态（STAGE-179）
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "血量和气力都是满的，用不着")
	}
	delta := 0
	if eff.Full {
		delta = r.maxHP() - r.HP
	} else if eff.HP > 0 {
		delta = gamedata.RecoveryPower(eff.HP, r.Stats.Vital, false, true, rand.IntN)
	}
	if over := r.HP + delta - r.maxHP(); over > 0 {
		delta -= over
	}
	if delta < 0 {
		delta = 0
	}
	r.HP += delta
	mpDelta := 0
	if eff.Full || eff.MP > 0 {
		gain := gamedata.RecoveryPower(eff.MP, r.Stats.Vital, false, false, rand.IntN)
		if eff.Full {
			gain = gamedata.NewPlayerMaxMP // 原版 power=100，人物的气力上限就是 100
		}
		if gain > gamedata.NewPlayerMaxMP-r.MP {
			gain = gamedata.NewPlayerMaxMP - r.MP
		}
		r.MP += gain
		mpDelta = gain
	}
	p.pushAll("status_changed", map[string]any{"status": r.status()})
	if err := s.consumeItem(ctx, p, r, slot); err != nil {
		return nil, err
	}
	out := map[string]any{"target": "player", "name": r.Name, "hp": r.HP, "delta": delta}
	if mpDelta > 0 {
		out["mp"], out["mp_delta"] = r.MP, mpDelta
	}
	return out, nil
}

// playerNothingToRecover 判断这个物品对自己有没有可回复的内容：回血的物品看血量，回气的看气力，“全”两样都看。
func playerNothingToRecover(r *record, eff gamedata.UseEffect) bool {
	hpRoom := r.HP < r.maxHP()
	mpRoom := r.MP < gamedata.NewPlayerMaxMP
	wantHP := eff.Full || eff.HP > 0
	wantMP := eff.Full || eff.MP > 0
	return !((wantHP && hpRoom) || (wantMP && mpRoom))
}

// useItemOnPet 给宠物使用：回血，扣物品，推 pets_changed 与 inventory_changed（血量写库）。
func (s *Service) useItemOnPet(ctx context.Context, p *Presence, r *record, eff gamedata.UseEffect, slot int, pet *petRec) (map[string]any, error) {
	maxHP := gamedata.PetMaxHP(pet.Stats)
	if pet.HP >= maxHP { // 宠物血量已满：不扣道具、不改状态（STAGE-179）
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s 血量是满的，用不着", pet.Name)
	}
	delta := gamedata.RecoveryPower(eff.HP, pet.Stats.Vital/100, true, true, rand.IntN)
	if eff.Full {
		delta = maxHP - pet.HP
	}
	if over := pet.HP + delta - maxHP; over > 0 {
		delta -= over
	}
	if delta < 0 {
		delta = 0
	}
	pet.HP += delta
	if err := s.consumeItem(ctx, p, r, slot); err != nil {
		return nil, err
	}
	s.savePets(p.charID, r.Pets) // 血量写库（finishBattle 的同款收尾，持锁同步写）
	p.pushAll("pets_changed", r.petsView())
	return map[string]any{"target": "pet", "slot": pet.Slot, "name": pet.Name, "hp": pet.HP, "delta": delta}, nil
}

// consumeItem 把一个格子上的物品数量减 1，用完删格，写库并推 inventory_changed。
// 调用时要持有角色的锁。失败时物品没有扣掉（错误返回给客户端，HP 的变化也一并放弃——
// 内存里的数值在返回前重置不掉，但这个请求失败后下一次 status 会读内存，允许这种极小概率的偏差）。
func (s *Service) consumeItem(ctx context.Context, p *Presence, r *record, slot int) error {
	return s.changeItems(ctx, p, r, func(items []itemRec, _ int) ([]itemRec, error) {
		next := make([]itemRec, 0, len(items))
		for _, it := range items {
			if it.Slot == slot {
				if c := it.Count - 1; c > 0 {
					it.Count = c
					next = append(next, it)
				}
				continue
			}
			next = append(next, it)
		}
		return next, nil
	})
}

// consumeBattleItems 消耗本回合 item 事件用掉的物品（战斗中 item 指令，协议 10.7）：
// 按“战斗里用掉”的槽位扣数量、写库并推 inventory_changed，要在 battle_round 之前调用。
// 调用时要持有角色的锁（settle 在锁内）。
func (s *Service) consumeBattleItems(p *Presence, events []battle.Event) {
	used := map[int]bool{}
	for _, ev := range events {
		if ev.Kind == "item" {
			used[ev.Slot] = true
		}
	}
	if len(used) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.changeItems(ctx, p, &p.rec, func(items []itemRec, _ int) ([]itemRec, error) {
		next := make([]itemRec, 0, len(items))
		for _, it := range items {
			if used[it.Slot] {
				if c := it.Count - 1; c > 0 {
					it.Count = c
					next = append(next, it)
				}
				continue
			}
			next = append(next, it)
		}
		return next, nil
	}); err != nil {
		// 引擎的快照已经置空，本场战斗不会重复使用；只记日志，下次进世界以库存为准。
		s.opts.Logger.Warn("保存战斗消耗的物品失败", "character", p.charID, "err", err)
	}
}
