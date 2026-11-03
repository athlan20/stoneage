// stoneage-server 是新的游戏服务端：在 TCP 上用 JSON Lines 协议（docs/protocol.md）与客户端通信，
// 数据存在 PostgreSQL。配置只有两项，见 docs/server.md。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/athlan20/stoneage/docs"
	"github.com/athlan20/stoneage/server/app"
	"github.com/athlan20/stoneage/server/config"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/store"
	"github.com/athlan20/stoneage/server/webpages"
	"github.com/athlan20/stoneage/server/wordfilter"
)

const serverName = "stoneage-server/0.1.0"

func main() {
	envFile := flag.String("env", ".env", ".env 文件路径（不存在则只用环境变量和默认值）")
	protocolLog := flag.String("protocol-log", "", "把收发的每条协议消息写到这个文件，- 表示标准错误；密码和令牌一律显示为 ***")
	tempSchema := flag.Bool("temp-schema", false, "在数据库里新建一个临时 schema 来运行，退出时删除（联调和冒烟测试用，不碰开发库里的数据）")
	printDB := flag.Bool("print-db", false, "按行输出连接串里的用户名、密码、库名、主机、端口后退出（供启动脚本使用）")
	adminListen := flag.String("admin-listen", "", "本地管理口的地址（如 127.0.0.1:7066，供协议用例的 start_battle 用）；留空取 .env 的 STONEAGE_ADMIN_LISTEN，都不写就不开")
	shutdownStdin := flag.Bool("shutdown-stdin", false, "标准输入关闭时正常停服（子进程联调专用）")
	flag.Parse()

	cfg, err := config.Load(*envFile)
	if err != nil {
		fatal("配置不正确", err)
	}
	if *printDB {
		parts, err := config.ParseDatabaseURL(cfg.DatabaseURL)
		if err != nil {
			fatal("配置不正确", err)
		}
		fmt.Printf("%s\n%s\n%s\n%s\n%s\n", parts.User, parts.Password, parts.Name, parts.Host, parts.Port)
		return
	}
	if cfg.DatabaseURL == "" {
		fatal("配置不正确", fmt.Errorf("%s 没有设置（见 .env.example）", config.EnvDatabaseURL))
	}
	logNpcEventOpen(gamedata.SetNpcEventOpen(cfg.NpcEventOpen)) // STAGE-214：任务脚本开关，默认不放开额外脚本
	// STAGE-287：指定了屏蔽词表就必须读得进来，否则起不来，免得悄悄只剩内置的几个词。
	if cfg.ChatFilterFile != "" {
		st, err := wordfilter.ReloadFile(cfg.ChatFilterFile)
		if err != nil {
			fatal("屏蔽词表加载失败", err)
		}
		slog.Info("屏蔽词表已加载", "file", cfg.ChatFilterFile, "words", st.Words, "allow", st.Allow, "skipped", st.Skipped)
	}
	// STAGE-297：精工覆盖表；没配就用随程序发布的（只有表头）。编号不存在、读不出来都起不来。
	if err := loadInlayOverlay(cfg.InlayOverlayFile); err != nil {
		fatal("精工覆盖表加载失败", err)
	}
	if err := runWithStdin(cfg, *protocolLog, *tempSchema, *adminListen, *shutdownStdin); err != nil {
		fatal("服务端退出", err)
	}
}

// logNpcEventOpen 按任务脚本开关的解析结果打启动日志（STAGE-214）：默认不打；放开额外脚本一律是警告，提醒仅供测试。
func logNpcEventOpen(sum gamedata.NpcEventOpenSummary) {
	switch sum.Mode {
	case "all":
		slog.Warn(fmt.Sprintf("STONEAGE_NPC_EVENT_OPEN=all：额外放开 %d 个任务脚本，仅供测试", sum.Extra))
	case "list":
		slog.Warn(fmt.Sprintf("STONEAGE_NPC_EVENT_OPEN：额外放开 %d 个任务脚本，仅供测试", sum.Extra))
	}
	for _, name := range sum.Unknown {
		slog.Warn("找不到任务脚本：" + name)
	}
}

func run(cfg config.Config, protocolLog string, tempSchema bool, adminListen string) error {
	return runWithStdin(cfg, protocolLog, tempSchema, adminListen, false)
}

func runWithStdin(cfg config.Config, protocolLog string, tempSchema bool, adminListen string, shutdownStdin bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if shutdownStdin {
		go func() { _, _ = io.Copy(io.Discard, os.Stdin); stop() }()
	}

	dbURL := cfg.DatabaseURL
	if tempSchema {
		var cleanup func()
		var err error
		if dbURL, cleanup, err = useTempSchema(ctx, dbURL); err != nil {
			return err
		}
		defer cleanup()
		// schema 名打出来（listen=xxx 一样的做法），冒烟脚本把它拼进管理命令的连接串里。
		if u, uerr := url.Parse(dbURL); uerr == nil {
			slog.Info("使用临时 schema，退出时删除", "schema", u.Query().Get("search_path"))
		} else {
			slog.Info("使用临时 schema，退出时删除")
		}
	}
	st, err := store.Open(ctx, dbURL, 30*time.Second)
	if err != nil {
		return err
	}
	defer st.Close()
	applied, err := st.Migrate(ctx)
	if err != nil {
		return err
	}
	for _, m := range applied {
		slog.Info("数据库已升级", "version", m.Version, "name", m.Name)
	}
	version, _ := st.SchemaVersion(ctx)
	starts, err := st.RecordStart(ctx)
	if err != nil {
		return err
	}

	trace, closeTrace, err := openProtocolLog(protocolLog)
	if err != nil {
		return err
	}
	defer closeTrace()

	srv := gateway.New(gateway.Options{
		ServerName:          serverName,
		DocsRev:             docs.Rev(), // 协议和手册的修订号，hello 里返回（STAGE-188）
		RegisterEnabled:     true,
		Trace:               trace,
		TrustedProxies:      cfg.TrustedProxies,
		MinClientVersion:    cfg.MinClientVersion,
		LatestClientVersion: cfg.LatestClientVersion,
		RateLimitMode:       cfg.RateLimitMode,
		RateLimitRate:       cfg.RateLimitRate,
		RateLimitBurst:      cfg.RateLimitBurst,
	})
	// 协议文档、agent 手册、参考脚本和石器百科和 WebSocket 共用一个端口（STAGE-118）。
	webpages.Install(srv, cfg.WikiCDN)
	services, err := app.Install(srv, st.Pool, app.Options{RegisterEnabled: true, EncounterIntervalMs: cfg.EncounterIntervalMs, RideLevelGap: cfg.RideLevelGap,
		AgentMaxOnline: time.Duration(cfg.AgentMaxOnlineMinutes) * time.Minute,
		Autosave:       autosaveInterval(cfg.AutosaveSeconds),
		NewPlayerGifts: gamedata.NewPlayerGifts, ChatFilterFile: cfg.ChatFilterFile})
	if err != nil {
		return err
	}
	if err := services.Start(ctx); err != nil {
		return err
	}
	defer services.Stop()
	adminAddr := adminListen
	if adminAddr == "" {
		adminAddr = cfg.AdminListen
	}
	admin, err := services.StartAdmin(adminAddr)
	if err != nil {
		return err
	}
	defer admin.Close()
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	// WebSocket 入口和 TCP 入口同时开好，再打印“已启动”，冒烟脚本从这一行拿两个地址。
	var wsLn net.Listener
	wsAddr := ""
	if cfg.WSListen != "" {
		if wsLn, err = net.Listen("tcp", cfg.WSListen); err != nil {
			_ = ln.Close()
			return err
		}
		wsAddr = wsLn.Addr().String()
	}
	slog.Info("服务端已启动", "listen", ln.Addr().String(), "ws_listen", wsAddr, "schema_version", version, "start_count", starts, "protocol_log", protocolLog != "", "trusted_proxies", len(cfg.TrustedProxies))

	served := make(chan error, 2) // 每个入口各报一次结束原因
	go func() { served <- srv.Serve(ln) }()
	if wsLn != nil {
		go func() { served <- srv.ServeWS(wsLn) }()
	}
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	slog.Info("收到退出信号，正在通知所有连接并关闭")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func openProtocolLog(path string) (io.Writer, func(), error) {
	switch path {
	case "":
		return nil, func() {}, nil
	case "-":
		return os.Stderr, func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

func fatal(what string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
	os.Exit(1)
}

// autosaveInterval 把 .env 的秒数换成定时存盘间隔（STAGE-290）：0 表示关闭，对应 Options 里的负数。
func autosaveInterval(seconds int) time.Duration {
	if seconds <= 0 {
		return -1
	}
	return time.Duration(seconds) * time.Second
}
