// stoneage-admin 是账号管理命令行：查看账号、禁用/启用账号、解除登录锁定，发物品/宠物/点数/精灵，改全服设置。
// 直接操作 .env 里配置的数据库，不需要服务端在运行；禁用账号、改设置都通过数据库通知让运行中的服务端立即生效。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/athlan20/stoneage/server/auth"
	"github.com/athlan20/stoneage/server/character"
	"github.com/athlan20/stoneage/server/config"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/store"
)

const usage = `用法: stoneage-admin [-env .env] <命令>

  accounts                   列出所有账号（不含密码）
  disable <账号> [原因]      禁用账号：在线连接立即被断开，之后无法登录
  enable <账号>              启用账号
  unlock <账号>              解除登录锁定
  give <角色名> <物品编号> [数量]  给角色发物品，放进第一个空的背包格（STAGE-28）
  move <角色名> <地图> <x> <y>  把角色搬到指定地图和坐标（只写库：角色在线时要先离开世界，冒烟用，STAGE-96）
  givepet <角色名> <宠物编号> [等级]  给角色一只宠物，随机资质，放进第一个空的宠物栏位（STAGE-31）
  givepoint <角色名> <点数>  给角色未分配的能力点，用 addpoint 加到能力上（STAGE-24）
  ridelevel <角色名> <等级>  直接设骑乘等级，0/40/80/120/200（STAGE-122，骑乘训练师的课程，免费）
  level <角色名> <等级>      直接设人物等级，1–160（STAGE-156，只写库：角色在线时要先离开世界，同 move）
  givemagic <角色名> <精灵编号>  给角色一个精灵，放进第一个空的精灵栏（STAGE-48）
  setpettrans <角色名> <栏位> <次数>  直接设随身宠物的转生次数（栏位 0–19，STAGE-196；写库并通知运行中的服务端，转生次数超过 2 次不能骑）
  quest <角色名> <任务编号> doing|done|clear  设任务进度：进行中、已完成或清除（STAGE-86，任务表见 server/gamedata/quests.tsv）
  grant ...                  批量发放物资（STAGE-241）：--kind item|pet|magic|point --batch 批次号 人群参数 [--yes]，详见 maintain/README.md
  settings                   列出全服设置和当前值（STAGE-59）
  set <设置名> <值>          改全服设置（如 set exp_rate 2，0.1–100、最多两位小数），
                             运行中的服务端收到通知立即生效，不用重启
  mute <角色名> --minutes N [--reason 文字]  公开聊天禁言 N 分钟（STAGE-278，挡本图/世界/队伍频道，不影响私聊）
  unmute <角色名>            立即解除公开聊天禁言（STAGE-278）
  mutes                      列出仍在禁言的角色和到期时间（STAGE-278）
  chatfilter test <文字>     按 .env 的 STONEAGE_CHAT_FILTER_FILE（没配则内置词表）试一句话：显示聊天里会变成什么样、作为名字会不会被拒绝（STAGE-287）
  chatfilter reload          校验词表文件后通知运行中的服务端重读，不用重启；文件有问题时不通知，服务端也继续用旧词表（STAGE-287）
`

func main() {
	envFile := flag.String("env", ".env", ".env 文件路径")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*envFile, args); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(envFile string, args []string) error {
	cfg, err := config.Load(envFile)
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("%s 没有设置", config.EnvDatabaseURL)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL, 10*time.Second)
	if err != nil {
		return err
	}
	defer st.Close()
	svc, err := auth.New(st.Pool, nil, auth.Options{})
	if err != nil {
		return err
	}
	need := func(n int) error {
		if len(args) < n+1 {
			return fmt.Errorf("缺少参数\n\n%s", usage)
		}
		return nil
	}
	switch args[0] {
	case "accounts":
		list, err := svc.ListAccounts(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-16s %-6s %-20s %-20s %s\n", "账号", "状态", "创建时间", "最后登录", "备注")
		for _, a := range list {
			state, note := "正常", ""
			if a.Disabled {
				state, note = "禁用", a.DisabledReason
			}
			if a.LockedUntil != nil {
				note = strings.TrimSpace(note + " 锁定至 " + a.LockedUntil.Local().Format("15:04:05"))
			}
			last := "-"
			if a.LastLoginAt != nil {
				last = a.LastLoginAt.Local().Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-16s %-6s %-20s %-20s %s\n", a.Name, state, a.CreatedAt.Local().Format("2006-01-02 15:04:05"), last, note)
		}
		return nil
	case "disable":
		if err := need(1); err != nil {
			return err
		}
		reason := "管理员禁用"
		if len(args) > 2 {
			reason = strings.Join(args[2:], " ")
		}
		return report(svc.SetDisabled(ctx, args[1], true, reason), "已禁用 "+args[1])
	case "enable":
		if err := need(1); err != nil {
			return err
		}
		return report(svc.SetDisabled(ctx, args[1], false, ""), "已启用 "+args[1])
	case "unlock":
		if err := need(1); err != nil {
			return err
		}
		return report(svc.Unlock(ctx, args[1]), "已解除 "+args[1]+" 的登录锁定")
	case "give":
		if err := need(2); err != nil {
			return err
		}
		itemID, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("物品编号要是整数: %w", err)
		}
		count := 1
		if len(args) > 3 {
			if count, err = strconv.Atoi(args[3]); err != nil {
				return fmt.Errorf("数量要是整数: %w", err)
			}
		}
		if err := character.GiveItem(ctx, st.Pool, character.DefaultItemNotifyChannel, args[1], itemID, count); err != nil {
			return err
		}
		def, _ := gamedata.ItemByID(itemID)
		fmt.Printf("已把 %s（编号 %d）×%d 发给 %s\n", def.Name, itemID, count, args[1])
		return nil
	case "givepoint":
		if err := need(2); err != nil {
			return err
		}
		points, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("点数要是整数: %w", err)
		}
		if err := character.GivePoints(ctx, st.Pool, character.DefaultPointNotifyChannel, args[1], points); err != nil {
			return err
		}
		fmt.Printf("已给 %s %d 点未分配点数（用 addpoint 加到能力上）\n", args[1], points)
		return nil
	case "ridelevel":
		if err := need(2); err != nil {
			return err
		}
		level, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("骑乘等级要是整数: %w", err)
		}
		if err := character.SetRideLevel(ctx, st.Pool, character.DefaultRideNotifyChannel, args[1], level); err != nil {
			return err
		}
		fmt.Printf("已把 %s 的骑乘等级设为 %d（运行中的服务端会立即推送给在线的连接）\n", args[1], level)
		return nil
	case "level":
		if err := need(2); err != nil {
			return err
		}
		level, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("人物等级要是整数: %w", err)
		}
		if err := character.SetCharacterLevel(ctx, st.Pool, args[1], level); err != nil {
			return err
		}
		fmt.Printf("已把 %s 的人物等级设为 %d（只写库，角色在线时要先离开世界，下次进入世界生效）\n", args[1], level)
		return nil
	case "givepet":
		if err := need(2); err != nil {
			return err
		}
		petID, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("宠物编号要是整数: %w", err)
		}
		level := gamedata.NewPetLevel
		if len(args) > 3 {
			if level, err = strconv.Atoi(args[3]); err != nil {
				return fmt.Errorf("等级要是整数: %w", err)
			}
		}
		rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
		slot, err := character.GivePet(ctx, st.Pool, character.DefaultPetNotifyChannel, args[1], petID, level, rng.IntN)
		if err != nil {
			return err
		}
		pet, _ := gamedata.PetByID(petID)
		fmt.Printf("已给 %s 一只 %s（编号 %d，Lv %d），放进 %d 号宠物栏位\n", args[1], pet.Name, petID, level, slot)
		return nil
	case "givemagic":
		if err := need(2); err != nil {
			return err
		}
		magicID, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("精灵编号要是整数: %w", err)
		}
		if err := character.GiveMagic(ctx, st.Pool, character.DefaultMagicNotifyChannel, args[1], magicID); err != nil {
			return err
		}
		t, _ := gamedata.MagicByID(magicID)
		fmt.Printf("已给 %s 精灵 %s（编号 %d）\n", args[1], t.Name, magicID)
		return nil
	case "setpettrans":
		if err := need(3); err != nil {
			return err
		}
		name, slot, trans, err := parsePetTrans(args[1:])
		if err != nil {
			return err
		}
		if err := character.SetPetTransmigration(ctx, st.Pool, character.DefaultPetNotifyChannel, name, slot, trans); err != nil {
			return err
		}
		fmt.Printf("已把 %s 的 %d 号宠物转生次数设为 %d（运行中的服务端会立即推送给在线的连接）\n", name, slot, trans)
		return nil
	case "move":
		if err := need(4); err != nil {
			return err
		}
		floor, errFloor := strconv.Atoi(args[2])
		x, errX := strconv.Atoi(args[3])
		y, errY := strconv.Atoi(args[4])
		if err := errors.Join(errFloor, errX, errY); err != nil {
			return fmt.Errorf("地图编号和坐标要是整数: %w", err)
		}
		if err := character.MoveCharacter(ctx, st.Pool, args[1], floor, x, y); err != nil {
			return err
		}
		fmt.Printf("已把 %s 搬到地图 %d（%d,%d）\n", args[1], floor, x, y)
		return nil
	case "quest":
		if err := need(3); err != nil {
			return err
		}
		no, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("任务编号要是整数: %w", err)
		}
		if err := character.SetQuest(ctx, st.Pool, character.DefaultQuestNotifyChannel, args[1], no, args[3]); err != nil {
			return err
		}
		t, _ := gamedata.QuestByNo(no)
		fmt.Printf("已把 %s 的任务 %s（编号 %d）设为 %s\n", args[1], t.Name, no, args[3])
		return nil
	case "grant":
		return runGrant(ctx, st.Pool, args[1:])
	case "settings":
		list, err := character.ListGameSettings(ctx, st.Pool)
		if err != nil {
			return err
		}
		fmt.Printf("%-10s %-8s %-20s %-20s %s\n", "设置", "值", "说明", "更新时间", "改的人")
		for _, g := range list {
			fmt.Printf("%-10s %-8g %-20s %-20s %s\n",
				g.Key, g.Value, g.Note, g.UpdatedAt.Local().Format("2006-01-02 15:04:05"), g.UpdatedBy)
		}
		return nil
	case "mute":
		return runMute(ctx, st.Pool, args[1:])
	case "unmute":
		return runUnmute(ctx, st.Pool, args[1:])
	case "mutes":
		return runMutes(ctx, st.Pool)
	case "chatfilter":
		return runChatFilter(ctx, st.Pool, cfg.ChatFilterFile, args[1:])
	case "set":
		if err := need(2); err != nil {
			return err
		}
		value, err := character.ParseSettingValue(args[1], args[2])
		if err != nil {
			return err
		}
		by := os.Getenv("USER")
		if by == "" {
			by = "stoneage-admin"
		}
		if err := character.WriteGameSetting(ctx, st.Pool, character.DefaultSettingsNotifyChannel, args[1], value, by); err != nil {
			return err
		}
		fmt.Printf("已把 %s 改成 %g（运行中的服务端立即生效）\n", args[1], value)
		return nil
	}
	return fmt.Errorf("不认识的命令 %q\n\n%s", args[0], usage)
}

func report(err error, ok string) error {
	if err == nil {
		fmt.Println(ok)
	}
	return err
}
