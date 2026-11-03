// Package storetest 给需要真实 PostgreSQL 的测试提供一个隔离的、已迁移好的 Store。
package storetest

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/store"
)

// EnvURL 指向一个可用的 PostgreSQL；没设置时依赖数据库的测试会被跳过。
const EnvURL = "STONEAGE_TEST_DATABASE_URL"

// Available 报告是否配置了测试数据库。
func Available() bool { return os.Getenv(EnvURL) != "" }

// New 在独立的 schema 里创建 Store 并执行迁移，测试结束时删除。
func New(t testing.TB) *store.Store {
	t.Helper()
	s := NewUnmigrated(t)
	if _, err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

// NewUnmigrated 同 New，但不执行迁移。
func NewUnmigrated(t testing.TB) *store.Store {
	t.Helper()
	raw := os.Getenv(EnvURL)
	if raw == "" {
		t.Skipf("未设置 %s，跳过需要数据库的测试", EnvURL)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), rand.Intn(100000))
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := store.Open(ctx, u.String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}
