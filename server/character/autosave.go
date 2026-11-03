package character

// 定时存盘（STAGE-290）：角色的等级、经验、HP/MP、金币、位置原本只在离开世界时写库，
// 服务端被强杀、崩溃或机器重启时，玩家这次登录以来的进度全丢（回档）。
// 现在每隔 AutosaveInterval 把世界中的角色存一遍，战斗结束（升级、拿经验的时刻）也立即存一次，
// 异常退出最多丢最近一个间隔的进度。存盘失败只记日志，下一轮再试。

import (
	"context"
	"time"
)

// DefaultAutosaveInterval 是定时存盘的默认间隔。
const DefaultAutosaveInterval = 3 * time.Minute

// autosaveTimeout 是一次存盘最多等数据库多久；持着角色锁写库，不能无限等。
const autosaveTimeout = 5 * time.Second

// startAutosave 启动定时存盘协程；间隔为负表示关闭。
func (s *Service) startAutosave(ctx context.Context) {
	interval := s.opts.AutosaveInterval
	if interval == 0 {
		interval = DefaultAutosaveInterval
	}
	if interval < 0 {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.autosaveStop = cancel
	s.autosaveDone = make(chan struct{})
	go func() {
		defer close(s.autosaveDone)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.autosaveAll()
			}
		}
	}()
}

// stopAutosave 停止定时存盘并等协程退出。
func (s *Service) stopAutosave() {
	if s.autosaveStop != nil {
		s.autosaveStop()
		<-s.autosaveDone
		s.autosaveStop = nil
	}
}

// autosaveAll 把世界中的所有角色存一遍。逐个加角色锁、写库、放锁，一个角色慢不拖累其他角色的请求。
func (s *Service) autosaveAll() {
	s.mu.Lock()
	all := make([]*Presence, 0, len(s.presences))
	for _, p := range s.presences {
		all = append(all, p)
	}
	s.mu.Unlock()
	failed := 0
	for _, p := range all {
		p.mu.Lock()
		if !p.leaving && !s.saveProgress(p) {
			failed++
		}
		p.mu.Unlock()
	}
	if failed > 0 {
		s.opts.Logger.Warn("定时存盘有角色保存失败，下一轮再试", "failed", failed, "online", len(all))
	}
}

// saveProgress 把角色写回数据库，失败只记日志（返回是否成功）。调用时要持有 p.mu；
// 写库和离开世界的存盘（leave）都在锁里，所以不会拿旧值盖掉新值。
func (s *Service) saveProgress(p *Presence) bool {
	ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
	defer cancel()
	if err := s.save(ctx, &p.rec, s.opts.Now()); err != nil {
		s.opts.Logger.Error("存盘失败", "character", p.charID, "err", err)
		return false
	}
	return true
}
