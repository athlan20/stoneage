// 数据库通知（STAGE-28 背包、STAGE-31 宠物、STAGE-24 未分配点数、STAGE-48 精灵、STAGE-59 全服设置）：
// 管理命令在同一个事务里写库并 NOTIFY，运行中的服务端 LISTEN 收到后重读数据（角色数据推送给世界中的连接）。
// 频道共用一条监听连接，按频道分派。

package character

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Start 开始监听“背包变了”“宠物变了”“未分配点数变了”“精灵变了”“全服设置变了”和“任务进度变了”六个通知
// （服务端后台任务，规则同账号禁用通知：断线自动重连）。
func (s *Service) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	conn, err := s.listenNotify(ctx)
	if err != nil {
		cancel()
		return err
	}
	s.notifyStop = cancel
	s.notifyDone = make(chan struct{})
	go s.notifyLoop(ctx, conn)
	s.startAutosave(ctx)
	return nil
}

// Stop 停止监听并等待后台协程退出。
func (s *Service) Stop() {
	s.stopAutosave()
	if s.notifyStop != nil {
		s.notifyStop()
		<-s.notifyDone
		s.notifyStop = nil
	}
}

// listenNotify 占一条数据库连接，监听背包、宠物、未分配点数、精灵、全服设置和任务进度六个频道。
// LISTEN 建好之后把全服设置重读一遍：启动时是第一次加载，断线重连后也不漏掉断线期间改掉的设置
// （先 LISTEN 再核对，和账号禁用通知同一个道理）。
func (s *Service) listenNotify(ctx context.Context) (*pgx.Conn, error) {
	pc, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	conn := pc.Hijack()
	for _, channel := range []string{s.opts.ItemNotifyChannel, s.opts.PetNotifyChannel, s.opts.PointNotifyChannel, s.opts.GoldNotifyChannel, s.opts.MagicNotifyChannel, s.opts.SettingsNotifyChannel, s.opts.QuestNotifyChannel, s.opts.RideNotifyChannel, s.opts.ChatFilterNotifyChannel} {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			_ = conn.Close(context.Background())
			return nil, err
		}
	}
	// 读不到设置不拦着监听：经验按 1 倍处理，警告日志已由 ReloadGameSettings 记过。
	_ = s.ReloadGameSettings(ctx)
	return conn, nil
}

func (s *Service) notifyLoop(ctx context.Context, conn *pgx.Conn) {
	defer close(s.notifyDone)
	backoff := time.Second
	for {
		for {
			n, err := conn.WaitForNotification(ctx)
			if err != nil {
				break
			}
			s.dispatchNotify(ctx, n)
		}
		_ = conn.Close(context.Background())
		for {
			if ctx.Err() != nil {
				return
			}
			s.opts.Logger.Warn("角色数据通知的监听连接断开，正在重连")
			var err error
			if conn, err = s.listenNotify(ctx); err == nil {
				backoff = time.Second
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

// dispatchNotify 按频道分派：背包变了就重读背包，宠物变了就重读宠物，未分配点数变了就重读能力点，
// 全服设置变了就重读整张设置表。
func (s *Service) dispatchNotify(ctx context.Context, n *pgconn.Notification) {
	switch n.Channel {
	case s.opts.ItemNotifyChannel:
		s.reloadItems(ctx, n.Payload)
	case s.opts.PetNotifyChannel:
		s.reloadPets(ctx, n.Payload)
	case s.opts.PointNotifyChannel:
		s.reloadPoints(ctx, n.Payload)
	case s.opts.GoldNotifyChannel:
		s.reloadGold(n.Payload)
	case s.opts.MagicNotifyChannel:
		s.reloadMagics(ctx, n.Payload)
	case s.opts.SettingsNotifyChannel:
		_ = s.ReloadGameSettings(ctx)
	case s.opts.QuestNotifyChannel:
		s.reloadQuests(ctx, n.Payload)
	case s.opts.RideNotifyChannel:
		s.reloadRide(ctx, n.Payload)
	case s.opts.ChatFilterNotifyChannel:
		s.reloadChatFilter()
	}
}

// presenceOf 返回角色当前在世界中的登记；不在世界中（没有可通知的连接）时返回 nil。
func (s *Service) presenceOf(charID int64) *Presence {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.presences {
		if p.charID == charID {
			return p
		}
	}
	return nil
}
