package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/character"
)

// STAGE-278：公开聊天禁言的管理命令（docs/server.md）。
//   mute <角色名> --minutes N [--reason 文字]   禁言，从现在起 N 分钟（1 到 43200）
//   unmute <角色名>                             立即解除
//   mutes                                       列出仍在禁言的角色和到期时间

// parseMuteArgs 解析 mute 的参数（不含命令名）：角色名是第一个非选项参数，选项可以放在任何位置。
func parseMuteArgs(args []string) (name string, minutes int, reason string, err error) {
	minutes = 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--minutes" || a == "--reason":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("%s 后面要有值", a)
			}
			i++
			if a == "--minutes" {
				if minutes, err = strconv.Atoi(args[i]); err != nil {
					return "", 0, "", fmt.Errorf("--minutes 要是整数: %w", err)
				}
			} else {
				reason = args[i]
			}
		case strings.HasPrefix(a, "--"):
			return "", 0, "", fmt.Errorf("不认识的选项 %s", a)
		case name == "":
			name = a
		default:
			return "", 0, "", fmt.Errorf("多余的参数 %s", a)
		}
	}
	if name == "" {
		return "", 0, "", fmt.Errorf("要写角色名")
	}
	if minutes < 1 || minutes > 43200 {
		return "", 0, "", fmt.Errorf("--minutes 要在 1 到 43200 之间（最长 30 天）")
	}
	return name, minutes, reason, nil
}

// runMute 执行 mute（不含命令名的参数）。
func runMute(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	name, minutes, reason, err := parseMuteArgs(args)
	if err != nil {
		return err
	}
	until, err := character.ChatMuteFor(ctx, pool, name, time.Duration(minutes)*time.Minute)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("已禁言 %s，到 %s 解除（运行中的服务端立即生效，只挡公开频道）", name, until.Local().Format("2006-01-02 15:04:05"))
	if reason != "" {
		msg += "，原因：" + reason
	}
	fmt.Println(msg)
	return nil
}

// runUnmute 执行 unmute（不含命令名的参数）。
func runUnmute(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：unmute <角色名>")
	}
	if err := character.ChatUnmute(ctx, pool, args[0]); err != nil {
		return err
	}
	fmt.Printf("已解除 %s 的禁言\n", args[0])
	return nil
}

// runMutes 列出仍在禁言的角色。
func runMutes(ctx context.Context, pool *pgxpool.Pool) error {
	list, err := character.ListMutes(ctx, pool)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("当前没有禁言的角色")
		return nil
	}
	fmt.Printf("%-16s %s\n", "角色", "解除时间")
	for _, m := range list {
		fmt.Printf("%-16s %s\n", m.Name, m.Until.Local().Format("2006-01-02 15:04:05"))
	}
	return nil
}
