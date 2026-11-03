package character

// STAGE-234：宠物的个体等级上限（原版 CHAR_LIMITLEVEL）与宠物对话的执行（原版 gmsv/char/pet_event.c 的
// PET_Talkfunc 和 PetTalk_RunEvent）。对话的数据和选块在 gamedata/pettalk.go。
//
// 入口是协议 pet_talk（STAGE-239，pettalk_handler.go）：它提供 grant（AddItem 动作的道具发放，栏位满时返回 false，
// 整块失败），并在事务里把改动的上限写库。

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
)

// petTalkBusy 是对话块失败时宠物说的话（原版 PET_Talkfunc 的 "．．．．！"）。
const petTalkBusy = "．．．．！"

// petLimit 是宠物的有效等级上限：对话改写过的用个体值（character_pets.limit_level），没改过的（NULL）用模板的
// LIMITLEVEL。原版创建宠物时把模板的 LIMITLEVEL 复制到 CHAR_LIMITLEVEL（enemy.c），这里不复制，改用 NULL 表示
// “还是模板的值”，模板值不变时结果一样。0 为没有上限。
func petLimit(p petRec, tmpl gamedata.PetTemplate) int {
	if p.LimitLevel != nil {
		return *p.LimitLevel
	}
	return tmpl.LimitLevel
}

// talkPet 在内存里执行一次宠物对话：挑块（gamedata.PetTalkPick），块带 AddItem 时先发道具（grant 返回 false，
// 即栏位满，整块失败：原版 Action_RunDoEventAction 返回 FALSE，不改上限，说 petTalkBusy），再按 LimitLevel
// 改个体上限。返回要说的台词和上限是否被改写（改写了调用方要写库）。没有对话的宠物不说话。
func talkPet(p *petRec, floor int, rnd func(n int) int, grant func(items []int) bool) (text string, changed bool) {
	tmpl, ok := gamedata.PetByID(p.TemplateID)
	if !ok {
		return "", false
	}
	table, ok := gamedata.PetTalkOf(p.TemplateID)
	if !ok {
		return "", false
	}
	block, text, ok := gamedata.PetTalkPick(table, p.Level, petLimit(*p, tmpl), floor, rnd)
	if !ok {
		return "", false
	}
	if block != nil && len(block.Items) > 0 && (grant == nil || !grant(block.Items)) {
		return petTalkBusy, false
	}
	if block == nil || block.Limit <= 0 {
		return text, false
	}
	limit := block.Limit
	p.LimitLevel = &limit
	return text, true
}

// writePetLimit 把宠物的个体等级上限写进库（在事务里用）。limit 为 nil 时写 NULL，表示回到模板的上限。
func writePetLimit(ctx context.Context, tx pgx.Tx, charID int64, slot int, limit *int) error {
	_, err := tx.Exec(ctx, `UPDATE character_pets SET limit_level = $3 WHERE character_id = $1 AND slot = $2`,
		charID, slot, limit)
	return err
}
