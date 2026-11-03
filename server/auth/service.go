// Package auth 实现账号注册、登录、频率限制、锁定、重复登录挤下线和审计（STAGE-3，
// docs/protocol.md 5.3、5.4）。密码只以 Argon2id 哈希保存，任何日志、审计和数据库字段里都不出现明文。
package auth

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// 业务错误码（docs/protocol.md 5.3、5.4）。
const (
	CodeRegisterDisabled = "REGISTER_DISABLED"
	CodeInvalidAccount   = "INVALID_ACCOUNT"
	CodeInvalidPassword  = "INVALID_PASSWORD"
	CodeAccountExists    = "ACCOUNT_EXISTS"
	CodeWrongCredentials = "WRONG_CREDENTIALS"
	CodeAccountLocked    = "ACCOUNT_LOCKED"
	CodeAccountDisabled  = "ACCOUNT_DISABLED"
)

// Limits 是防暴力破解的默认值（docs/protocol.md 第 7 节），需要调整时改这里。
type Limits struct {
	LoginFailWindow time.Duration // 同一账号统计失败次数的时间窗口
	LoginFailMax    int           // 窗口内失败这么多次就锁定
	LockDuration    time.Duration // 锁定多久
	IPFailWindow    time.Duration // 同一 IP 统计失败次数的时间窗口
	IPFailMax       int           // 窗口内失败这么多次就限制该 IP 登录
	RegisterWindow  time.Duration // 同一 IP 统计注册次数的时间窗口
	RegisterMax     int           // 窗口内最多注册几次
}

// DefaultLimits：账号 10 分钟失败 5 次锁 15 分钟；IP 10 分钟失败 20 次限制；IP 每小时注册 50 次。
var DefaultLimits = Limits{
	LoginFailWindow: 10 * time.Minute, LoginFailMax: 5, LockDuration: 15 * time.Minute,
	IPFailWindow: 10 * time.Minute, IPFailMax: 20,
	RegisterWindow: time.Hour, RegisterMax: 50,
}

// Options 是服务的参数，零值字段取默认值。
type Options struct {
	Limits          Limits
	Hash            HashParams
	RegisterEnabled bool
	// BeforeKick 在重复登录挤掉旧连接、或账号被禁用踢掉连接之前调用，用来保存连接的角色数据（STAGE-4 设置）。
	BeforeKick func(old *gateway.Session)
	// TokenCheckInterval 是检查在线 agent 的令牌有没有到期（或被吊销）的间隔，默认 60 秒（docs/protocol.md 第 7 节）。
	TokenCheckInterval time.Duration
	// AgentMaxOnline 是 agent 连续在线的上限，0 表示不限制（STAGE-118，配置 STONEAGE_AGENT_MAX_ONLINE_MINUTES）。
	AgentMaxOnline time.Duration
	// NotifyChannel 是“账号被禁用”通知用的 PostgreSQL 频道，默认 account_disabled。
	// 频道名是整个数据库共用的，所以多个测试同时跑时要各用各的。
	NotifyChannel string
	Now           func() time.Time // 测试用，默认 time.Now
	Logger        *slog.Logger
}

// Service 是账号服务。
type Service struct {
	db   *pgxpool.Pool
	hub  *gateway.Hub
	opts Options
	sem  chan struct{} // 限制同时进行的 Argon2 计算，防止内存被打爆

	// dummyHash 用来校验不存在的账号，让“账号不存在”和“密码错误”耗时一致。
	dummyHash string
	// kickMu 只保护 enter 里“找出同账号同身份的旧连接并登记新连接”这一小步（保证两条连接同时登录时只留下一条），
	// 保存数据、断开连接都在锁外做（STAGE-13）：保存慢只影响那个账号。
	kickMu sync.Mutex
	// attempts 让同一 IP、同一账号的登录和注册一个接一个处理（STAGE-7）：
	// 检查次数、校验密码、记录失败这几步中间不能插进别的尝试。
	attempts *keyLock

	afterVerify func() // 测试钩子：密码校验刚做完时调用
	beforeEnter func() // 测试钩子：数据库检查都做完、连接还没登记到账号名下时调用

	stopListen context.CancelFunc // Start 之后才有
	listenDone chan struct{}
	expiryDone chan struct{}
}

// New 创建账号服务。
func New(db *pgxpool.Pool, hub *gateway.Hub, opts Options) (*Service, error) {
	if opts.Limits == (Limits{}) {
		opts.Limits = DefaultLimits
	}
	if opts.Hash == (HashParams{}) {
		opts.Hash = DefaultHashParams
	}
	if opts.NotifyChannel == "" {
		opts.NotifyChannel = DefaultNotifyChannel
	}
	if opts.TokenCheckInterval <= 0 {
		opts.TokenCheckInterval = time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	dummy, err := Hash("dummy-password-for-timing", opts.Hash)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, hub: hub, opts: opts, sem: make(chan struct{}, runtime.NumCPU()), dummyHash: dummy, attempts: newKeyLock()}, nil
}

// Install 把 register、login、agent_login 和令牌管理注册到网络层。
func (s *Service) Install(srv *gateway.Server) {
	srv.Handle("register", s.handleRegister)
	srv.Handle("login", s.handleLogin)
	srv.Handle("agent_login", s.handleAgentLogin)
	srv.Handle("create_agent_token", s.handleCreateToken)
	srv.Handle("agent_tokens", s.handleTokens)
	srv.Handle("revoke_agent_token", s.handleRevokeToken)
}

func (s *Service) hash(ctx context.Context, password string) (string, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return Hash(password, s.opts.Hash)
}

func (s *Service) verify(ctx context.Context, encoded, password string) (bool, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return Verify(encoded, password), nil
}

func seconds(d time.Duration) int {
	n := int((d + time.Second - 1) / time.Second)
	if n < 1 {
		n = 1
	}
	return n
}

func rateLimited(retry time.Duration, what string) *protocol.Error {
	return protocol.Errorf(protocol.CodeRateLimited, "%s，请稍后再试", what).
		WithData(map[string]any{"retry_after_seconds": seconds(retry)})
}

// Register 注册账号。注册成功后不会自动登录。
func (s *Service) Register(ctx context.Context, ip, account, password string) error {
	name := NormalizeAccount(account)
	// 同一 IP 的注册串行处理，“每小时 50 次”才不会被并发绕过。
	unlock, err := s.attempts.lockAll(ctx, "register-ip:"+ip)
	if err != nil {
		return err
	}
	defer unlock()
	// 记录审计不随连接断开而取消：已经做完的尝试必须计数。
	rctx := context.WithoutCancel(ctx)
	fail := func(e *protocol.Error) error {
		s.audit(rctx, "register", name, ip, false, e.Code)
		return e
	}
	if !s.opts.RegisterEnabled {
		return fail(protocol.Errorf(CodeRegisterDisabled, "服务器暂未开放注册"))
	}
	if !ValidAccount(name) {
		return fail(protocol.Errorf(CodeInvalidAccount, "账号需要 3–15 个字符，以字母开头，只能包含小写字母、数字和下划线"))
	}
	if !ValidPassword(password) {
		return fail(protocol.Errorf(CodeInvalidPassword, "密码需要 %d–%d 个字符，且不能有首尾空格", PasswordMin, PasswordMax))
	}
	now := s.opts.Now()
	n, oldest, err := s.countRegistrations(ctx, ip, now.Add(-s.opts.Limits.RegisterWindow))
	if err != nil {
		return err
	}
	if n >= s.opts.Limits.RegisterMax {
		return fail(rateLimited(oldest.Add(s.opts.Limits.RegisterWindow).Sub(now), "注册太频繁"))
	}
	exists, err := s.accountExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return fail(protocol.Errorf(CodeAccountExists, "账号已存在"))
	}
	encoded, err := s.hash(ctx, password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(rctx, `INSERT INTO accounts (name, password_hash, created_at, created_ip) VALUES ($1, $2, $3, $4)`, name, encoded, now, ip)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" { // 并发注册同名账号
		return fail(protocol.Errorf(CodeAccountExists, "账号已存在"))
	}
	if err != nil {
		return err
	}
	s.audit(rctx, "register", name, ip, true, "ok")
	return nil
}

// Login 校验账号密码；成功时返回账号 id，调用方负责把连接标记为已登录并挤掉旧连接。
func (s *Service) Login(ctx context.Context, ip, account, password string) (accountID int64, name string, err error) {
	name = NormalizeAccount(account)
	// 锁定记录和审计里存的是 auditName 之后的名字（去掉空字符、截断），查询时也要用同一个，
	// 否则带空字符的账号名会让数据库报错（STAGE-8）。
	key := auditName(name)
	// 同一 IP、同一账号的尝试一个接一个处理：检查次数 → 校验密码 → 记录失败，中间不能插进别的尝试
	// （STAGE-7）。先锁 IP 再锁账号，所有调用顺序一致，不会死锁。
	unlock, err := s.attempts.lockAll(ctx, "login-ip:"+ip, "login-account:"+key)
	if err != nil {
		return 0, "", err
	}
	defer unlock()
	// 校验密码之后的记录不随连接断开而取消：做完的尝试必须计数，否则发完就断线可以白猜。
	rctx := context.WithoutCancel(ctx)
	fail := func(e *protocol.Error) (int64, string, error) {
		s.audit(rctx, "login", name, ip, false, e.Code)
		return 0, "", e
	}
	lim := s.opts.Limits
	now := s.opts.Now()

	// 1. 同一 IP 失败太多：限制登录。
	n, oldest, err := s.countIPFailures(ctx, ip, now.Add(-lim.IPFailWindow))
	if err != nil {
		return 0, "", err
	}
	if n >= lim.IPFailMax {
		return fail(rateLimited(oldest.Add(lim.IPFailWindow).Sub(now), "登录失败次数过多"))
	}
	// 2. 账号被锁定：即使密码正确也不能登录。
	until, locked, err := s.lockedUntil(ctx, key, now)
	if err != nil {
		return 0, "", err
	}
	if locked {
		return fail(protocol.Errorf(CodeAccountLocked, "登录失败次数过多，账号已被暂时锁定").
			WithData(map[string]any{"retry_after_seconds": seconds(until.Sub(now))}))
	}
	// 3. 校验密码。账号不存在、格式不对都按“密码错误”处理，耗时也一样。
	acct, found, err := s.findAccount(ctx, name)
	if err != nil {
		return 0, "", err
	}
	hash := s.dummyHash
	if found {
		hash = acct.hash
	}
	ok, err := s.verify(ctx, hash, password)
	if err != nil {
		return 0, "", err
	}
	if s.afterVerify != nil {
		s.afterVerify()
	}
	if !found || !ok {
		s.audit(rctx, "login", name, ip, false, CodeWrongCredentials)
		if err := s.maybeLock(rctx, name, now); err != nil {
			s.opts.Logger.Error("记录登录锁定失败", "err", err)
		}
		return 0, "", protocol.Errorf(CodeWrongCredentials, "账号或密码错误")
	}
	// 4. 密码正确之后才告知账号被禁用，避免外人借此探测账号。
	if acct.disabled {
		return fail(protocol.Errorf(CodeAccountDisabled, "账号已被禁用"))
	}
	if _, err := s.db.Exec(rctx, `UPDATE accounts SET last_login_at = $2, last_login_ip = $3 WHERE id = $1`, acct.id, now, ip); err != nil {
		return 0, "", err
	}
	s.audit(rctx, "login", name, ip, true, "ok")
	return acct.id, name, nil
}
