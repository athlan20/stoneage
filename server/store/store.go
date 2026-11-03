// Package store 是 PostgreSQL 访问层：连接池、带版本记录的表结构迁移。
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 持有数据库连接池。
type Store struct {
	Pool *pgxpool.Pool
}

// Open 连接数据库。数据库容器刚启动时可能还没就绪，所以在 wait 时间内反复重试。
func Open(ctx context.Context, databaseURL string, wait time.Duration) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("数据库连接串不正确: %w", redactURLError(err))
	}
	deadline := time.Now().Add(wait)
	for {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return &Store{Pool: pool}, nil
			}
			pool.Close()
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, fmt.Errorf("无法连接数据库 %s:%d: %w", cfg.ConnConfig.Host, cfg.ConnConfig.Port, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// redactURLError 避免 url.Parse 的错误信息把整条连接串（含密码）带进日志。
func redactURLError(err error) error {
	var pe interface{ Unwrap() error }
	if errors.As(err, &pe) && pe.Unwrap() != nil {
		return pe.Unwrap()
	}
	return errors.New("格式错误")
}

// Close 关闭连接池。
func (s *Store) Close() { s.Pool.Close() }

// RecordStart 记录一次服务端启动，返回累计启动次数。数据重启后仍在，可用来确认数据持久化。
func (s *Store) RecordStart(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO server_meta (key, value) VALUES ('start_count', '1')
		ON CONFLICT (key) DO UPDATE SET value = (server_meta.value::int + 1)::text, updated_at = now()
		RETURNING value::int`).Scan(&n)
	return n, err
}

// Reset 清空所有业务表（保留 schema_migrations 和 server_meta），让协议用例在空数据库上运行。
func (s *Store) Reset(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `
		SELECT quote_ident(tablename) FROM pg_tables
		WHERE schemaname = current_schema() AND tablename NOT IN ('schema_migrations', 'server_meta')`)
	if err != nil {
		return err
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, t := range tables {
		if _, err := s.Pool.Exec(ctx, "TRUNCATE "+t+" RESTART IDENTITY CASCADE"); err != nil {
			return err
		}
	}
	// game_settings 也被清了，把设置恢复成默认值（迁移 0009 的种子），上一个用例改过的倍数不带到下一个。
	if _, err := s.Pool.Exec(ctx, `
		INSERT INTO game_settings (key, value, note) VALUES ('exp_rate', 1, '战斗经验倍数')`); err != nil {
		return err
	}
	return nil
}
