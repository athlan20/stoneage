package character

// STAGE-266：人物转生的写库。能力、等级、事件清空、下骑和赠宠放在同一个事务里，
// 要么全变要么全不变（原版会出现“数据已改但送宠失败”的半成品，这里不复刻）。

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
)

// errTenseiPetsFull 表示宠物栏 5 格全满，整次转生拒绝、什么都没改。
var errTenseiPetsFull = errors.New("宠物栏已满")

// tenseiResult 是提交成功后推送要用的信息。
type tenseiResult struct {
	petName string // 赠的宠物名
}

// commitTensei 在一个事务里完成转生：读宠物（加行锁）→ 算新状态（tenseiApplyRecord）→ 写角色、下骑的宠物、
// 事件进度 → 放一只赠宠；提交成功后才把结果换到 r 上。宠物栏满返回 errTenseiPetsFull，其他错误原样返回，r 都不变。
// floor 是精灵王所在的主城，决定送哪只宠。调用时要持有角色的锁（p.mu）。
func (s *Service) commitTensei(ctx context.Context, r *record, floor int, rnd gamedata.Rnd) (tenseiResult, error) {
	var res tenseiResult
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)
	pets, err := readPets(ctx, tx, r.ID, true)
	if err != nil {
		return res, err
	}
	free := tenseiFreeSlot(pets)
	if free < 0 {
		return res, errTenseiPetsFull
	}
	next := *r
	next.Pets = pets
	if err := tenseiApplyRecord(&next); err != nil {
		return res, err
	}
	petID, ok := tenseiPresentOf(floor, next.Transmigration)
	if !ok {
		return res, fmt.Errorf("地图 %d 在第 %d 转没有赠宠", floor, next.Transmigration)
	}
	tmpl, ok := gamedata.PetByID(petID)
	if !ok {
		return res, fmt.Errorf("宠物编号 %d 不存在", petID)
	}
	if err := saveTenseiChar(ctx, tx, &next); err != nil {
		return res, err
	}
	for _, p := range next.Pets { // 下骑：骑着的宠物回到休息
		if old := findPet(r.Pets, p.Slot); old != nil && old.State != p.State {
			if _, err := tx.Exec(ctx, `UPDATE character_pets SET state = $3 WHERE character_id = $1 AND slot = $2`,
				r.ID, p.Slot, p.State); err != nil {
				return res, err
			}
		}
	}
	if err := insertPetRow(ctx, tx, r.ID, free, tmpl, tenseiPresentLevel, rnd); err != nil {
		return res, err
	}
	before, after := progressOf(r), planTenseiEvents(r, next.Transmigration)
	if err := writeStateDiff(ctx, tx, questTable, r.ID, before.quests, after.quests); err != nil {
		return res, err
	}
	if err := writeStateDiff(ctx, tx, eventTable, r.ID, before.events, after.events); err != nil {
		return res, err
	}
	next.Pets, err = readPets(ctx, tx, r.ID, false)
	if err != nil {
		return res, err
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	next.Quests, next.EventFlags = stateRecs(after.quests), stateRecs(after.events)
	if pet := findPet(next.Pets, free); pet != nil {
		res.petName = pet.Name
	}
	*r = next
	return res, nil
}

// saveTenseiChar 写转生改动的角色字段：四项能力、转生次数、等级、经验、能力点、HP 和两个累计值。
func saveTenseiChar(ctx context.Context, tx pgx.Tx, r *record) error {
	tag, err := tx.Exec(ctx, `UPDATE characters SET
			vital = $2, strength = $3, toughness = $4, dexterity = $5, transmigration = $6,
			level = $7, exp = $8, skill_points = $9, hp = $10, trans_quest_total = $11, trans_level_total = $12
		WHERE id = $1`,
		r.ID, r.Stats.Vital, r.Stats.Strength, r.Stats.Toughness, r.Stats.Dexterity, r.Transmigration,
		r.Level, r.Exp, r.SkillPoints, r.HP, r.TransQuestTotal, r.TransLevelTotal)
	if err == nil && tag.RowsAffected() == 0 {
		return errCharacterGone
	}
	return err
}
