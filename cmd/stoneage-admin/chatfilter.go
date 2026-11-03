package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/character"
	"github.com/athlan20/stoneage/server/wordfilter"
)

// STAGE-287：屏蔽词表的管理命令。
//   chatfilter test <文字>   试一句话
//   chatfilter reload        校验文件，再通知运行中的服务端重读

// loadChatFilter 按配置取词表：配了文件就读文件（读不了直接报错），没配就是内置词表。
func loadChatFilter(file string) (*wordfilter.Filter, string, error) {
	if file == "" {
		return wordfilter.Current(), "内置词表（STONEAGE_CHAT_FILTER_FILE 没有设置）", nil
	}
	f, err := wordfilter.LoadFile(file)
	if err != nil {
		return nil, "", err
	}
	return f, file, nil
}

func runChatFilter(ctx context.Context, db *pgxpool.Pool, file string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: chatfilter test <文字> | chatfilter reload")
	}
	switch args[0] {
	case "test":
		if len(args) < 2 {
			return fmt.Errorf("用法: chatfilter test <文字>")
		}
		return chatFilterTest(file, strings.Join(args[1:], " "))
	case "reload":
		return chatFilterReload(ctx, db, file)
	}
	return fmt.Errorf("不认识的子命令 %q（test / reload）", args[0])
}

func chatFilterTest(file, text string) error {
	f, source, err := loadChatFilter(file)
	if err != nil {
		return err
	}
	st := f.Stats()
	fmt.Printf("词表：%s（%d 个词，白名单 %d 条，忽略 %d 条）\n", source, st.Words, st.Allow, st.Skipped)
	fmt.Printf("聊天：%s\n", f.Mask(text))
	if f.Contains(text) {
		fmt.Println("名字：会被拒绝")
	} else {
		fmt.Println("名字：可以使用")
	}
	return nil
}

func chatFilterReload(ctx context.Context, db *pgxpool.Pool, file string) error {
	if file == "" {
		return fmt.Errorf("STONEAGE_CHAT_FILTER_FILE 没有设置，没有外部词表可重载")
	}
	f, err := wordfilter.LoadFile(file)
	if err != nil {
		return fmt.Errorf("词表文件有问题，没有通知服务端: %w", err)
	}
	if err := character.NotifyChatFilterReload(ctx, db, character.DefaultChatFilterNotifyChannel); err != nil {
		return err
	}
	st := f.Stats()
	fmt.Printf("词表 %s 校验通过（%d 个词，白名单 %d 条，忽略 %d 条），已通知运行中的服务端重读\n", file, st.Words, st.Allow, st.Skipped)
	return nil
}
