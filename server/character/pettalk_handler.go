package character

// STAGE-239：玩家和宠物对话的协议入口 pet_talk（docs/protocol.md 5.61，原版 gmsv/char/pet_event.c 的 PET_Talkfunc）。
// 选块和执行在 pettalk.go 的 talkPet，这里负责：世界中、操控方、战斗外；宠物要在地图上（出战或待命，原版是
// 面向地图上的宠物说话才触发）；然后在一个事务里写道具（writeItemDiff）和个体上限（writePetLimit），
// 提交后才改内存并推送 inventory_changed、pets_changed。

import (
	"context"
	"math/rand/v2"
	"sort"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// petTalkResult 是一次对话的结果：台词和个体上限是否被改写。
type petTalkResult struct {
	text         string
	limitChanged bool
}

// handlePetTalk 是 pet_talk（docs/protocol.md 5.61）。
func (s *Service) handlePetTalk(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in petSlotRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := petSlotArg(in.Slot)
	if err != nil {
		return nil, err
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var (
		res     petTalkResult
		talkErr error
	)
	if err := l.Operate(func(r *record) {
		if e := inBattle(l); e != nil { // 战斗中不能对话（docs/protocol.md 5.61）
			talkErr = e
			return
		}
		res, talkErr = s.talkPetInWorld(ctx, l.p, r, slot)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if talkErr != nil {
		return nil, talkErr
	}
	return map[string]any{"text": res.text, "limit_changed": res.limitChanged}, nil
}

// talkPetInWorld 检查宠物状态，执行一次对话并落库。调用时要持有角色的锁（在 Live.Operate 里）。
// 没有改动（只说话）时不开事务；道具栏位给不下时 talkPet 返回“．．．．！”且不改任何东西。
func (s *Service) talkPetInWorld(ctx context.Context, p *Presence, r *record, slot int) (petTalkResult, error) {
	target := findPet(r.Pets, slot)
	if target == nil {
		return petTalkResult{}, protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
	}
	if target.State != petStateBattle && target.State != petStateStandby {
		return petTalkResult{}, protocol.Errorf(protocol.CodeWrongState, "休息或骑乘中的宠物不在身边，不能对话")
	}
	pet := *target
	bag := cloneItems(r.Items)
	grant := func(ids []int) bool { // AddItem：先看空位够不够，不够整块失败（不改背包）
		if len(ids) > freeBagSlots(bag) {
			return false
		}
		for _, id := range ids {
			bag = addItem(bag, id)
		}
		sort.Slice(bag, func(i, j int) bool { return bag[i].Slot < bag[j].Slot })
		return true
	}
	text, changed := talkPet(&pet, r.Floor, rand.IntN, grant)
	itemsChanged := !sameItems(r.Items, bag)
	if !changed && !itemsChanged {
		return petTalkResult{text: text}, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return petTalkResult{}, err
	}
	defer tx.Rollback(ctx)
	if itemsChanged {
		if err := writeItemDiff(ctx, tx, r.ID, r.Items, bag); err != nil {
			return petTalkResult{}, err
		}
	}
	if changed {
		if err := writePetLimit(ctx, tx, r.ID, slot, pet.LimitLevel); err != nil {
			return petTalkResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return petTalkResult{}, err
	}

	if itemsChanged {
		r.Items = bag
		p.pushAll("inventory_changed", r.inventoryView())
	}
	if changed {
		next := append([]petRec(nil), r.Pets...)
		findPet(next, slot).LimitLevel = pet.LimitLevel
		r.Pets = next
		p.pushAll("pets_changed", r.petsView())
	}
	return petTalkResult{text: text, limitChanged: changed}, nil
}
