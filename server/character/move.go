package character

// MoveCharacter 是管理命令（协议用例的 admin 行 move_character）：把角色挪到指定位置，
// 直接写库。用例在角色进入世界之前用它（在线角色的内存不会被改，用例流程保证这一点）。
import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MoveCharacter(ctx context.Context, db *pgxpool.Pool, character string, floor, x, y int) error {
	_, err := db.Exec(ctx,
		`UPDATE characters SET floor = $2, x = $3, y = $4 WHERE name = $1`, character, floor, x, y)
	return err
}
