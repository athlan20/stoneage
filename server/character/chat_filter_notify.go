package character

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/wordfilter"
)

// STAGE-287：屏蔽词表重载。词表文件由 .env 的 STONEAGE_CHAT_FILTER_FILE 指定；运营改完文件后
// 用 stoneage-admin chatfilter reload 发一条数据库通知，运行中的服务端重读文件，不用重启。

// DefaultChatFilterNotifyChannel 是“屏蔽词表要重载”的 PostgreSQL 通知频道。
const DefaultChatFilterNotifyChannel = "chat_filter_reload"

// NotifyChatFilterReload 通知运行中的服务端重读屏蔽词表文件。
func NotifyChatFilterReload(ctx context.Context, db *pgxpool.Pool, channel string) error {
	_, err := db.Exec(ctx, `SELECT pg_notify($1, '')`, channel)
	return err
}

// reloadChatFilter 重读屏蔽词表文件；读不到或文件有问题时保留现有词表，只记警告日志。
func (s *Service) reloadChatFilter() {
	if s.opts.ChatFilterFile == "" {
		return
	}
	st, err := wordfilter.ReloadFile(s.opts.ChatFilterFile)
	if err != nil {
		s.opts.Logger.Warn("重载屏蔽词表失败，继续使用现有词表", "file", s.opts.ChatFilterFile, "error", err)
		return
	}
	s.opts.Logger.Info("屏蔽词表已重载", "file", s.opts.ChatFilterFile, "words", st.Words, "allow", st.Allow, "skipped", st.Skipped)
}
