// stoneage-casetest 是协议测试客户端：对一个正在运行的服务端逐条重放 docs/protocol/cases/ 里的协议用例，
// 把回复和期望逐字段比对（docs/server.md）。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/athlan20/stoneage/server/auth"
	"github.com/athlan20/stoneage/server/casetest"
	"github.com/athlan20/stoneage/server/character"
	"github.com/athlan20/stoneage/server/config"
	"github.com/athlan20/stoneage/server/store"
)

func main() {
	addr := flag.String("addr", "", "服务端地址，默认取 .env / 环境变量里的 STONEAGE_LISTEN")
	dir := flag.String("dir", "docs/protocol/cases", "用例目录")
	stages := flag.String("stages", "", "只跑这些阶段的用例，逗号分隔，如 STAGE-2,STAGE-3；默认全部")
	run := flag.String("run", "", "只跑名字匹配这个正则的用例")
	manual := flag.Bool("manual", false, "也跑标记为需要人工触发的用例")
	resetDB := flag.Bool("reset-db", false, "每个用例前清空业务数据（用 .env 里的数据库连接串，只允许本机的库，见 -force），让用例在空数据库上运行")
	force := flag.Bool("force", false, "允许 -reset-db 清空非本机的数据库（危险：会删掉那个库里的业务数据）")
	verbose := flag.Bool("v", false, "打印每条收发的消息（密码和令牌显示为 ***）")
	envFile := flag.String("env", ".env", ".env 文件路径")
	flag.Parse()

	cfg, err := config.Load(*envFile)
	if err != nil {
		die(err)
	}
	target := *addr
	if target == "" {
		target = cfg.Listen
	}
	cases, err := casetest.LoadDir(*dir)
	if err != nil {
		die(err)
	}
	var filter *regexp.Regexp
	if *run != "" {
		if filter, err = regexp.Compile(*run); err != nil {
			die(err)
		}
	}
	wantStage := map[string]bool{}
	for _, s := range strings.Split(*stages, ",") {
		if s = strings.TrimSpace(s); s != "" {
			wantStage[s] = true
		}
	}
	var st *store.Store
	if *resetDB {
		if !*force && !config.IsLocalDatabase(cfg.DatabaseURL) {
			die(fmt.Errorf("-reset-db 会清空数据库里的业务数据，但 %s 指向的不是本机（localhost、127.0.0.1、::1）；"+
				"确认要清空这个库的话，再加 -force", config.EnvDatabaseURL))
		}
		if st, err = store.Open(context.Background(), cfg.DatabaseURL, 10*time.Second); err != nil {
			die(err)
		}
		defer st.Close()
	}

	var passed, failed, skipped int
	for _, c := range cases {
		if (filter != nil && !filter.MatchString(c.Name)) || (len(wantStage) > 0 && !wantStage[c.Meta.Stage]) {
			continue
		}
		switch {
		case c.Meta.Manual && !*manual:
			fmt.Printf("SKIP  %-32s 需要人工触发（加 -manual 强制运行）\n", c.Name)
			skipped++
			continue
		case hasAdminStep(c) && st == nil:
			fmt.Printf("SKIP  %-32s 需要管理员操作，请加 -reset-db 让测试客户端直接操作数据库\n", c.Name)
			skipped++
			continue
		case len(c.Meta.ServerConfig) > 0:
			fmt.Printf("SKIP  %-32s 需要服务端配置 %v（go test ./server/... 会按需启动服务端运行它）\n", c.Name, c.Meta.ServerConfig)
			skipped++
			continue
		}
		if st != nil {
			if err := st.Reset(context.Background()); err != nil {
				die(err)
			}
		}
		opts := casetest.RunOptions{}
		if st != nil {
			// 用例里的 admin 行（如禁用账号、发物品）直接操作数据库；
			// 发物品的通知频道与服务端进程的配置一致（docs/server.md）。
			// start_battle 要让正在运行的服务端开战，走它的本地管理口（STONEAGE_ADMIN_LISTEN）。
			// give_pet 的宠物资质随机按用例名固定种子（STAGE-58），与进程内回归同源，黑盒重放才可复现。
			petRnd := rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 42)).IntN
			svc, err := auth.New(st.Pool, nil, auth.Options{})
			if err != nil {
				die(err)
			}
			opts.Admin = func(step casetest.Step) error {
				switch step.Do {
				case "give_item":
					return character.GiveItem(context.Background(), st.Pool, character.DefaultItemNotifyChannel, step.Character, step.ItemID, step.Count)
				case "give_points":
					return character.GivePoints(context.Background(), st.Pool, character.DefaultPointNotifyChannel, step.Character, step.Points)
				case "give_magic":
					return character.GiveMagic(context.Background(), st.Pool, character.DefaultMagicNotifyChannel, step.Character, step.MagicID)
				case "give_quest":
					return character.SetQuest(context.Background(), st.Pool, character.DefaultQuestNotifyChannel, step.Character, step.QuestNo, step.QuestState)
				case "set_ride_level": // 骑乘等级，同 stoneage-admin ridelevel（STAGE-122）
					return character.SetRideLevel(context.Background(), st.Pool, character.DefaultRideNotifyChannel, step.Character, step.Level)
				case "chat_mute": // 公开聊天禁言 seconds 秒（STAGE-278，同 stoneage-admin mute）
					_, err := character.ChatMuteFor(context.Background(), st.Pool, step.Character, time.Duration(step.Seconds)*time.Second)
					return err
				case "chat_unmute": // 解除公开聊天禁言（STAGE-278）
					return character.ChatUnmute(context.Background(), st.Pool, step.Character)
				case "give_pet":
					_, err := character.GivePetWithLoyalty(context.Background(), st.Pool, character.DefaultPetNotifyChannel, step.Character, step.PetID, step.GivePetLevel(), step.Loyalty, petRnd)
					return err
				case "faint_pet":
					return character.FaintPet(context.Background(), st.Pool, character.DefaultPetNotifyChannel, step.Character, step.Slot)
				case "clear_pets":
					return character.ClearPets(context.Background(), st.Pool, step.Character)
				case "set_pet_transmigration": // 宠物转生次数，同 STAGE-196 的进程内回归
					return character.SetPetTransmigration(context.Background(), st.Pool, character.DefaultPetNotifyChannel, step.Character, step.Slot, step.Trans)
				case "set_level": // 人物等级，进入世界前设（同进程内回归 cases_test.go）
					return character.SetCharacterLevel(context.Background(), st.Pool, step.Character, step.Level)
				case "set_gold": // 石币，进入世界前设（STAGE-328）
					return character.SetCharacterGold(context.Background(), st.Pool, step.Character, step.Gold)
				case "move_character":
					return character.MoveCharacter(context.Background(), st.Pool, step.Character, step.Floor, step.X, step.Y)
				case "start_battle", "set_setting":
					// 都要正在运行的服务端同步执行：开战要它的内存状态，set_setting
					// 要它立刻重读设置（黑盒重放不依赖通知到达的时机）。
					return adminAction(cfg.AdminListen, step)
				default:
					return svc.RunAdminAction(context.Background(), step.Do, step.Account)
				}
			}
		}
		if *verbose {
			opts.Log = func(f string, a ...any) { fmt.Printf("        "+f+"\n", a...) }
		}
		if err := c.Run(target, opts); err != nil {
			fmt.Printf("FAIL  %-32s %v\n", c.Name, err)
			failed++
			continue
		}
		fmt.Printf("PASS  %-32s %s\n", c.Name, c.Meta.Stage)
		passed++
	}
	fmt.Printf("\n通过 %d，失败 %d，跳过 %d\n", passed, failed, skipped)
	if failed > 0 {
		os.Exit(1)
	}
}

func hasAdminStep(c *casetest.Case) bool {
	for _, st := range c.Steps {
		if st.Dir == "admin" {
			return true
		}
	}
	return false
}

// adminAction 把需要运行中服务端执行的管理命令（目前只有 start_battle）发给它的本地管理口。
func adminAction(addr string, step casetest.Step) error {
	if addr == "" {
		return fmt.Errorf("服务端没有开管理口（STONEAGE_ADMIN_LISTEN / -admin-listen），无法 %s", step.Do)
	}
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("连接管理口 %s 失败: %w", addr, err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	req, _ := json.Marshal(map[string]any{"do": step.Do, "character": step.Character, "scenario": step.Scenario,
		"key": step.Key, "value": step.Value})
	if _, err := c.Write(append(req, '\n')); err != nil {
		return err
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return err
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("管理口回复不是 JSON: %s", strings.TrimSpace(string(line)))
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
