package store

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestMigrationFiles(t *testing.T) {
	ms, err := Migrations()
	if err != nil || len(ms) == 0 {
		t.Fatalf("%v %v", ms, err)
	}
	if ms[0].Version != 1 || ms[0].Name != "baseline" {
		t.Errorf("第一个迁移应是 0001_baseline: %+v", ms[0])
	}
}

// testStore 在一个独立的 schema 里打开 Store，需要环境变量 STONEAGE_TEST_DATABASE_URL 指向可用的 PostgreSQL。
func testStore(t *testing.T) *Store {
	t.Helper()
	raw := os.Getenv("STONEAGE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("未设置 STONEAGE_TEST_DATABASE_URL，跳过需要数据库的测试")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), rand.Intn(1000))
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})
	u, _ := url.Parse(raw)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := Open(ctx, u.String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestMigrateAndRecordStart(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if v, _ := s.SchemaVersion(ctx); v != 0 {
		t.Errorf("迁移前版本应为 0，实际 %d", v)
	}
	applied, err := s.Migrate(ctx)
	if err != nil || len(applied) == 0 {
		t.Fatalf("首次迁移: %v %v", applied, err)
	}
	if again, err := s.Migrate(ctx); err != nil || len(again) != 0 {
		t.Fatalf("重复迁移应无事可做: %v %v", again, err)
	}
	all, _ := Migrations()
	if v, _ := s.SchemaVersion(ctx); v != len(all) {
		t.Errorf("版本 = %d，期望 %d", v, len(all))
	}
	for want := 1; want <= 3; want++ {
		if n, err := s.RecordStart(ctx); err != nil || n != want {
			t.Fatalf("RecordStart = %d, %v，期望 %d", n, err, want)
		}
	}
}

func TestNewerDatabaseRefused(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES (999, 'future')"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Migrate(ctx); err == nil {
		t.Error("数据库版本比服务端新时应拒绝")
	}
}

func TestReset(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "CREATE TABLE scratch (id serial PRIMARY KEY, v text)"); err != nil {
		t.Fatal(err)
	}
	_, _ = s.Pool.Exec(ctx, "INSERT INTO scratch (v) VALUES ('x')")
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM scratch").Scan(&n)
	if n != 0 {
		t.Errorf("Reset 后业务表应为空，实际 %d 行", n)
	}
	if v, _ := s.SchemaVersion(ctx); v == 0 {
		t.Error("Reset 不应清掉迁移记录")
	}
	// 全服设置被清后要恢复默认值，上一个用例改过的倍数不带到下一个（STAGE-59）。
	var rate float64
	if err := s.Pool.QueryRow(ctx, `SELECT value FROM game_settings WHERE key = 'exp_rate'`).Scan(&rate); err != nil {
		t.Fatalf("Reset 后 game_settings 里应有 exp_rate: %v", err)
	}
	if rate != 1 {
		t.Errorf("Reset 后 exp_rate 应恢复默认 1，实际 %g", rate)
	}
}
