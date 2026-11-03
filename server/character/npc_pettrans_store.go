package character

// STAGE-236：宠物转生的写库（同一个事务里删掉玛蕾、更新被转生的宠物）。
// 原版是先删玛蕾再算出新宠物；这里两步在一个事务里做完，中途失败整个回滚（裁定：不改变可见行为）。

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
)

// savePetTransmig 写入转生结果：删掉玛蕾所在的栏位；被转生宠物的等级、经验、资质、能力、品质、血量、
// 转生次数和等级上限整行更新。名字、模板、技能、击杀数、起始值（origin_*）不动（原版 GetNewPet 只改这些字段）。
func savePetTransmig(ctx context.Context, tx pgx.Tx, charID int64, mala, target petRec) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM character_pets WHERE character_id = $1 AND slot = $2`, charID, mala.Slot); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE character_pets SET
			level = $3, exp = $4,
			vital = $5, strength = $6, toughness = $7, dexterity = $8,
			base_vital = $9, base_strength = $10, base_toughness = $11, base_dexterity = $12,
			rank = $13, hp = $14, transmigration = $15, limit_level = $16
		WHERE character_id = $1 AND slot = $2`,
		charID, target.Slot, target.Level, target.Exp,
		target.Stats.Vital, target.Stats.Strength, target.Stats.Toughness, target.Stats.Dexterity,
		target.Base.Vital, target.Base.Strength, target.Base.Toughness, target.Base.Dexterity,
		target.Rank, target.HP, target.Transmigration, target.LimitLevel)
	return err
}

// STAGE-237：转生成功后的写库还要删掉灵石和项链（planPetTransItems），并把事件 71 设为进行中（NPC_NowEventSetFlg）。
// 这些都和宠物的写入放在同一个事务里，要么全变，要么全不变。

// commitPetTransmute 是 case 4 的写库：在一个事务里读出宠物和背包（加行锁），算出转生结果，删掉玛蕾、更新被转生的宠物、
// 删掉范围内的道具、设事件 71 进行中；提交成功后才改内存并推送（pets_changed、inventory_changed、status_changed、quests_changed）。
// 返回 false 表示 planPetTransmute 不通过（找不到玛蕾、模板不认识等，原版直接返回 FALSE），什么都没改。
// 调用时要持有角色的锁（p.mu）。
func (s *Service) commitPetTransmute(ctx context.Context, p *Presence, r *record, slot int, rnd gamedata.Rnd) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	pets, err := readPets(ctx, tx, r.ID, true)
	if err != nil {
		return false, err
	}
	items, err := readItems(ctx, tx, r.ID, true)
	if err != nil {
		return false, err
	}
	res, ok := planPetTransmute(pets, slot, rnd)
	if !ok {
		return false, nil
	}
	if err := savePetTransmig(ctx, tx, r.ID, res.Removed, res.Target); err != nil {
		return false, err
	}
	nextItems := planPetTransItems(items)
	if err := writeItemDiff(ctx, tx, r.ID, items, nextItems); err != nil {
		return false, err
	}
	before := progressOf(r)
	after := progressOf(r)
	applyEventEdit(after, editSetDoing, petTransEventNo)
	if err := writeStateDiff(ctx, tx, questTable, r.ID, before.quests, after.quests); err != nil {
		return false, err
	}
	if err := writeStateDiff(ctx, tx, eventTable, r.ID, before.events, after.events); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	oldQuests := r.Quests
	statusBefore, magicBefore := r.status().Derived, r.magicSlots()
	r.Pets = res.Next
	r.Items = nextItems
	r.Quests, r.EventFlags = stateRecs(after.quests), stateRecs(after.events)
	p.pushAll("pets_changed", r.petsView())
	if !sameItems(items, nextItems) {
		p.pushAll("inventory_changed", r.inventoryView())
	}
	if statusAfter := r.status().Derived; statusAfter != statusBefore || r.magicSlots() != magicBefore {
		p.pushAll("status_changed", map[string]any{"status": r.status()})
	}
	if changed := changedQuests(oldQuests, r.Quests); len(changed) > 0 {
		p.pushAll("quests_changed", map[string]any{"quests": r.questsView(), "changed": changed})
	}
	return true, nil
}
