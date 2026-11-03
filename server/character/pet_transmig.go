// 宠物的转生次数（STAGE-196，docs/ride.md 第 9 条）：骑乘检查用，见 pets_ride.go。
// 转生本身（NPC_TransmigrationAddPet）还没做，这里只提供测试用的管理命令。

package character

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SetPetTransmigration 是测试用的管理命令（casetest 的 set_pet_transmigration 行）：直接设角色 character
// 栏位 slot 上宠物的转生次数，在同一个事务里 NOTIFY，运行中的服务端收到后重读宠物并推送 pets_changed。
func SetPetTransmigration(ctx context.Context, db *pgxpool.Pool, channel, character string, slot, trans int) error {
	if trans < 0 {
		return fmt.Errorf("转生次数不能是负数：%d", trans)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, character).Scan(&charID)
	if err != nil {
		return fmt.Errorf("找不到角色 %s", character)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE character_pets SET transmigration = $3 WHERE character_id = $1 AND slot = $2`,
		charID, slot, trans)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s 的 %d 号栏位没有宠物", character, slot)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetPetLevel 是测试用的管理命令（casetest 的 set_pet_level 行）：直接设角色栏位 slot 上宠物的等级，并把四项资质
// 设成 30（转生的资质下限是 15，用例里要造 79 级玛蕾时用得上；原版的 givepet 不能给超过模板上限的等级）。
// 和 SetPetTransmigration 一样在同一个事务里 NOTIFY。
func SetPetLevel(ctx context.Context, db *pgxpool.Pool, channel, character string, slot, level int) error {
	if level < 1 {
		return fmt.Errorf("等级要在 1 以上：%d", level)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, character).Scan(&charID)
	if err != nil {
		return fmt.Errorf("找不到角色 %s", character)
	}
	tag, err := tx.Exec(ctx, `UPDATE character_pets SET level = $3, base_vital = 30, base_strength = 30,
			base_toughness = 30, base_dexterity = 30
		WHERE character_id = $1 AND slot = $2`, charID, slot, level)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s 的 %d 号栏位没有宠物", character, slot)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
