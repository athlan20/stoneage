package character

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetCharacterLevel 是测试用的管理命令（casetest 的 set_level 行，STAGE-123）：直接把角色等级写进数据库。
// 不 NOTIFY：只用在进入世界之前（用例里放在 enter_world 之前），进入世界时才读这个等级。
func SetCharacterLevel(ctx context.Context, db *pgxpool.Pool, character string, level int) error {
	if level < 1 || level > 160 {
		return fmt.Errorf("等级要在 1–160 之间，不能是 %d", level)
	}
	tag, err := db.Exec(ctx, `UPDATE characters SET level = $2 WHERE name = $1`, character, level)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.Join(pgx.ErrNoRows, fmt.Errorf("找不到角色 %s", character))
	}
	return nil
}

// SetCharacterTransmigration 是测试用的管理命令（casetest 的 set_transmigration 行，STAGE-266）：直接把角色的转生次数
// 写进数据库（0–5）。同 SetCharacterLevel，只用在进入世界之前。
func SetCharacterTransmigration(ctx context.Context, db *pgxpool.Pool, character string, trans int) error {
	if trans < 0 || trans > TenseiMaxTrans {
		return fmt.Errorf("转生次数要在 0–%d 之间，不能是 %d", TenseiMaxTrans, trans)
	}
	tag, err := db.Exec(ctx, `UPDATE characters SET transmigration = $2 WHERE name = $1`, character, trans)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.Join(pgx.ErrNoRows, fmt.Errorf("找不到角色 %s", character))
	}
	return nil
}

// SetCharacterHP 是测试用的管理命令（casetest 的 set_hp 行，STAGE-306）：直接把角色的血量写进数据库（1 起），
// 决斗用例用它造“一击就倒下”的场面。同 SetCharacterLevel，只用在进入世界之前。
func SetCharacterHP(ctx context.Context, db *pgxpool.Pool, character string, hp int) error {
	if hp < 1 {
		return fmt.Errorf("血量要至少 1，不能是 %d", hp)
	}
	tag, err := db.Exec(ctx, `UPDATE characters SET hp = $2 WHERE name = $1`, character, hp)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.Join(pgx.ErrNoRows, fmt.Errorf("找不到角色 %s", character))
	}
	return nil
}

// SetCharacterDuelPoint 是测试用的管理命令（casetest 的 set_duel_point 行，STAGE-306）：直接设决斗点数（0–1 亿）。
func SetCharacterDuelPoint(ctx context.Context, db *pgxpool.Pool, character string, dp int) error {
	if dp < 0 || dp > 100000000 {
		return fmt.Errorf("决斗点数要在 0–100000000 之间，不能是 %d", dp)
	}
	tag, err := db.Exec(ctx, `UPDATE characters SET duel_point = $2, duel_point_at = CASE WHEN duel_point = $2 THEN duel_point_at ELSE now() END WHERE name = $1`, character, dp)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.Join(pgx.ErrNoRows, fmt.Errorf("找不到角色 %s", character))
	}
	return nil
}
