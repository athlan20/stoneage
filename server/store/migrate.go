package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLockID 是迁移用的 advisory lock，避免两个服务端同时启动时重复迁移。
const migrationLockID = 7065001

// Migration 是一个带版本号的迁移脚本，文件名形如 0001_baseline.sql。
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrations 返回内置的全部迁移，按版本排序。
func Migrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		base := strings.TrimSuffix(e.Name(), ".sql")
		num, name, ok := strings.Cut(base, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v <= 0 {
			return nil, fmt.Errorf("迁移文件名必须形如 0001_name.sql: %s", e.Name())
		}
		body, err := fs.ReadFile(migrationFiles, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: v, Name: name, SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i, m := range out {
		if m.Version != i+1 {
			return nil, fmt.Errorf("迁移版本必须从 1 开始连续编号，第 %d 个是 %04d_%s", i+1, m.Version, m.Name)
		}
	}
	return out, nil
}

// Migrate 把数据库升级到最新版本，返回这次新应用的迁移。每个迁移在一个事务里执行，
// 已应用的版本记录在 schema_migrations 表里。数据库版本比服务端还新时拒绝启动。
func (s *Store) Migrate(ctx context.Context) ([]Migration, error) {
	all, err := Migrations()
	if err != nil {
		return nil, err
	}
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return nil, err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer     PRIMARY KEY,
		name       text        NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	var current int
	if err := conn.QueryRow(ctx, "SELECT COALESCE(max(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return nil, err
	}
	if current > len(all) {
		return nil, fmt.Errorf("数据库表结构版本（%d）比服务端支持的（%d）新，请升级服务端", current, len(all))
	}
	var applied []Migration
	for _, m := range all[current:] {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return applied, err
		}
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("迁移 %04d_%s 失败: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.Version, m.Name); err != nil {
			_ = tx.Rollback(ctx)
			return applied, err
		}
		if err := tx.Commit(ctx); err != nil {
			return applied, err
		}
		applied = append(applied, m)
	}
	return applied, nil
}

// SchemaVersion 返回数据库当前的表结构版本（还没迁移过时为 0）。
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.Pool.QueryRow(ctx, "SELECT COALESCE(max(version), 0) FROM schema_migrations").Scan(&v)
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		return 0, nil
	}
	return v, err
}
