package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
)

// useTempSchema 在连接串指向的库里新建一个临时 schema，返回指向它的连接串和清理函数（删除这个 schema）。
// 联调和冒烟测试用：每次都在一个空的 schema 里跑真实的服务端，限流计数、账号、角色互不影响，也不碰开发库里的数据。
func useTempSchema(ctx context.Context, rawURL string) (string, func(), error) {
	schema := fmt.Sprintf("e2e_%d_%d", time.Now().UnixNano(), rand.Intn(100000))
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = admin.Close(ctx)
		return "", nil, err
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		_ = admin.Close(ctx)
		return "", nil, err
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	cleanup := func() {
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(bg, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(bg)
	}
	return u.String(), cleanup, nil
}
