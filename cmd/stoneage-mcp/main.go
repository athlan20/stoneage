// stoneage-mcp 是石器时代的本地 MCP 服务（STAGE-300）：AI 助手通过标准输入输出调用它的工具玩游戏。
// 它是一个 agent 客户端，用 agent 令牌连游戏服务端，用法见 docs/mcp.md。
//
//	claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_xxx -- stoneage-mcp --server wss://主机/ws
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/athlan20/stoneage/internal/agentmcp"
)

func main() {
	server := flag.String("server", "", "游戏服务端地址：host:port 走 TCP，ws:// 或 wss:// 走 WebSocket")
	tokenFile := flag.String("token-file", "", "令牌所在文件；不写则读环境变量 STONEAGE_AGENT_TOKEN")
	wiki := flag.String("wiki", "", "石器百科的 http(s) 地址，只给 TCP 连接的 wiki_search 用（WebSocket 连接时用同一台主机）")
	logPath := flag.String("log", "", "协议日志文件（可选）。stdout 是 MCP 通道，日志只写这里")
	flag.Parse()

	fail := func(err error) {
		fmt.Fprintln(os.Stderr, "stoneage-mcp:", err)
		os.Exit(2)
	}
	if *server == "" {
		fail(errors.New("缺少 --server，例如 --server 127.0.0.1:7065 或 --server wss://主机/ws"))
	}
	token, err := readToken(*tokenFile)
	if err != nil {
		fail(err)
	}

	cfg := agentmcp.Config{Server: *server, Token: token, Wiki: *wiki}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			fail(fmt.Errorf("无法打开协议日志：%w", err))
		}
		defer f.Close()
		cfg.LogFile = f
	}

	srv := agentmcp.New(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintln(os.Stderr, "stoneage-mcp: 已启动，连接服务端 "+*server+"（第一次调用工具时才连接）")
	if err := srv.Run(ctx); err != nil {
		srv.Close()
		fmt.Fprintln(os.Stderr, "stoneage-mcp:", err)
		os.Exit(1)
	}
	srv.Close()
}

// readToken 读令牌：--token-file 优先，其次环境变量。错误信息不带令牌内容。
func readToken(path string) (string, error) {
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("读取令牌文件失败：%w", err)
		}
		if t := strings.TrimSpace(string(raw)); t != "" {
			return t, nil
		}
		return "", errors.New("令牌文件是空的")
	}
	if t := strings.TrimSpace(os.Getenv("STONEAGE_AGENT_TOKEN")); t != "" {
		return t, nil
	}
	return "", errors.New("没有令牌：设置环境变量 STONEAGE_AGENT_TOKEN，或用 --token-file 指定文件")
}
