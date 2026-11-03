package auth

import (
	"context"
	"sync"
)

// keyLock 是按键加锁的互斥锁：同一个键的临界区一个接一个执行，不同的键互不影响；
// 等待时如果 ctx 取消（连接断开、服务关闭）就放弃。没人用的键会自动清理。
//
// 登录要“先查次数、再校验密码、最后记一次失败”，这三步必须对同一个账号、同一个 IP 串行，
// 否则同时发来的很多次尝试会在任何一次记下失败之前都通过次数检查（STAGE-7）。
type keyLock struct {
	mu sync.Mutex
	m  map[string]*keyEntry
}

type keyEntry struct {
	ch   chan struct{} // 容量 1，持有者往里放一个值
	refs int           // 持有者加等待者的数量
}

func newKeyLock() *keyLock { return &keyLock{m: map[string]*keyEntry{}} }

// lock 取得 key 的锁，返回解锁函数；等待期间 ctx 取消则返回 ctx 的错误。
func (k *keyLock) lock(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	e := k.m[key]
	if e == nil {
		e = &keyEntry{ch: make(chan struct{}, 1)}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()

	select {
	case e.ch <- struct{}{}:
		return func() { <-e.ch; k.release(key, e) }, nil
	case <-ctx.Done():
		k.release(key, e)
		return nil, ctx.Err()
	}
}

func (k *keyLock) release(key string, e *keyEntry) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if e.refs--; e.refs == 0 {
		delete(k.m, key)
	}
}

// lockAll 按给定顺序依次取得多把锁；调用方在各处必须用相同的顺序（先 IP 后账号），避免死锁。
func (k *keyLock) lockAll(ctx context.Context, keys ...string) (func(), error) {
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, key := range keys {
		u, err := k.lock(ctx, key)
		if err != nil {
			release()
			return nil, err
		}
		unlocks = append(unlocks, u)
	}
	return release, nil
}
