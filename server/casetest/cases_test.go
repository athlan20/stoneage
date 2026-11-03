package casetest_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/app"
	"github.com/athlan20/stoneage/server/auth"
	"github.com/athlan20/stoneage/server/casetest"
	"github.com/athlan20/stoneage/server/character"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/store/storetest"
)

// 测试用的低成本 Argon2 参数。
var fastHash = auth.HashParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}

const casesDir = "../../docs/protocol/cases"

// enabledStages 是服务端已经实现的阶段。新的阶段完成时把它加进来，
// 它们的用例就会自动成为回归测试；用例的 meta.stage 要标实际实现它的阶段（STAGE-56）。
var enabledStages = map[string]bool{"STAGE-2": true, "STAGE-246": true, "STAGE-250": true, "STAGE-3": true, "STAGE-4": true, "STAGE-6": true, "STAGE-22": true, "STAGE-23": true, "STAGE-24": true, "STAGE-28": true, "STAGE-29": true, "STAGE-30": true, "STAGE-31": true, "STAGE-32": true, "STAGE-33": true, "STAGE-40": true, "STAGE-43": true, "STAGE-45": true, "STAGE-46": true, "STAGE-47": true, "STAGE-48": true, "STAGE-49": true, "STAGE-50": true, "STAGE-51": true, "STAGE-59": true, "STAGE-60": true, "STAGE-62": true, "STAGE-63": true, "STAGE-68": true, "STAGE-69": true, "STAGE-70": true, "STAGE-71": true, "STAGE-72": true, "STAGE-73": true, "STAGE-74": true, "STAGE-77": true, "STAGE-78": true, "STAGE-84": true, "STAGE-86": true, "STAGE-90": true, "STAGE-93": true, "STAGE-94": true, "STAGE-96": true, "STAGE-100": true, "STAGE-109": true, "STAGE-119": true, "STAGE-122": true, "STAGE-123": true, "STAGE-124": true, "STAGE-126": true, "STAGE-127": true, "STAGE-143": true, "STAGE-144": true, "STAGE-146": true, "STAGE-148": true, "STAGE-149": true, "STAGE-152": true, "STAGE-162": true, "STAGE-186": true, "STAGE-189": true, "STAGE-190": true, "STAGE-194": true, "STAGE-195": true, "STAGE-196": true, "STAGE-199": true, "STAGE-200": true, "STAGE-201": true, "STAGE-202": true, "STAGE-203": true, "STAGE-207": true, "STAGE-208": true, "STAGE-209": true, "STAGE-228": true, "STAGE-230": true, "STAGE-231": true, "STAGE-236": true, "STAGE-237": true, "STAGE-239": true, "STAGE-257": true, "STAGE-258": true, "STAGE-262": true, "STAGE-251": true, "STAGE-255": true, "STAGE-263": true, "STAGE-264": true, "STAGE-266": true, "STAGE-267": true, "STAGE-268": true, "STAGE-273": true, "STAGE-278": true, "STAGE-279": true, "STAGE-274": true, "STAGE-276": true, "STAGE-277": true, "STAGE-282": true, "STAGE-287": true, "STAGE-302": true, "STAGE-311": true, "STAGE-298": true, "STAGE-294": true, "STAGE-296": true, "STAGE-309": true, "STAGE-306": true, "STAGE-308": true, "STAGE-326": true, "STAGE-328": true, "STAGE-332": true, "STAGE-333": true}

// encounterIntervalMs 从用例的 server_config 里取遇敌间隔（毫秒）；没写时 0 取服务端默认。
func encounterIntervalMs(c *casetest.Case) int {
	if v, ok := c.Meta.ServerConfig["encounter_interval_ms"].(float64); ok {
		return int(v)
	}
	return 0
}

func TestEveryCaseHasStage(t *testing.T) {
	cases, err := casetest.LoadDir(casesDir)
	if err != nil || len(cases) == 0 {
		t.Fatalf("读取用例失败: %v (%d 个)", err, len(cases))
	}
	for _, c := range cases {
		if c.Meta.Stage == "" {
			t.Errorf("%s 缺少 meta.stage，没有它用例不会被任何阶段的回归测试执行", c.Name)
		}
		if c.Meta.Desc == "" {
			t.Errorf("%s 缺少 meta.desc", c.Name)
		}
	}
}

// TestProtocolCases 重放全部用例。设置 STONEAGE_UPDATE_CASES=battle 时进入记录模式：
// 不比对 s2c 期望，把服务端的真实回复写回用例文件（校准用例用，跑完记得 review diff）。
func TestProtocolCases(t *testing.T) {
	update := os.Getenv("STONEAGE_UPDATE_CASES")
	cases, err := casetest.LoadDir(casesDir)
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			if update != "" && update != "all" && !strings.HasPrefix(c.Name, update) {
				t.Skipf("只更新 %s 前缀的用例", update)
			}
			if !enabledStages[c.Meta.Stage] {
				t.Skipf("属于 %s，尚未实现", c.Meta.Stage)
			}
			if c.Meta.Manual {
				t.Skip("需要人工触发")
			}
			opts := gateway.Options{RegisterEnabled: true}
			if v, ok := c.Meta.ServerConfig["idle_timeout_seconds"].(float64); ok {
				opts.IdleTimeout = time.Duration(v * float64(time.Second))
			}
			if v, ok := c.Meta.ServerConfig["register_enabled"].(bool); ok {
				opts.RegisterEnabled = v
			}
			// 客户端版本要求（STAGE-100，hello_client_*、agent_client_too_old 用例）。
			if v, ok := c.Meta.ServerConfig["min_client_version"].(string); ok {
				opts.MinClientVersion = v
			}
			if v, ok := c.Meta.ServerConfig["latest_client_version"].(string); ok {
				opts.LatestClientVersion = v
			}
			// 注册频率上限默认每小时 50 次；register_rate_limited 用例用 server_config 收紧到 5 次。
			registerMax := 0
			if v, ok := c.Meta.ServerConfig["register_max_per_hour"].(float64); ok {
				registerMax = int(v)
			}
			// 精工用例（STAGE-298）：物品加载测试覆盖表（只补几件物品的类别码，同进程，编号互不冲突）。
			if v, ok := c.Meta.ServerConfig["inlay_overlay"].(bool); ok && v {
				tsv, err := os.ReadFile("testdata/inlay_overlay.tsv")
				if err != nil {
					t.Fatal(err)
				}
				if err := gamedata.LoadInlayOverlay(string(tsv)); err != nil {
					t.Fatal(err)
				}
			}
			srv := gateway.New(opts)
			runOpts := casetest.RunOptions{Log: t.Logf, Record: update != ""}
			// 有测试数据库时装上账号服务（每个用例一个独立、已迁移的空 schema）；
			// 没有数据库时只能跑不依赖账号的用例。
			if storetest.Available() {
				st := storetest.New(t)
				itemChannel := fmt.Sprintf("character_items_%d", time.Now().UnixNano())
				petChannel := fmt.Sprintf("character_pets_%d", time.Now().UnixNano())
				pointChannel := fmt.Sprintf("character_points_%d", time.Now().UnixNano())
				magicChannel := fmt.Sprintf("character_magics_%d", time.Now().UnixNano())
				settingsChannel := fmt.Sprintf("game_settings_%d", time.Now().UnixNano())
				questChannel := fmt.Sprintf("character_quests_%d", time.Now().UnixNano())
				rideChannel := fmt.Sprintf("character_ride_%d", time.Now().UnixNano())
				svcs, err := app.Install(srv, st.Pool, app.Options{RegisterEnabled: opts.RegisterEnabled, RegisterMax: registerMax, Hash: fastHash,
					NotifyChannel:  fmt.Sprintf("account_disabled_%d", time.Now().UnixNano()),
					ItemNotify:     itemChannel,
					PetNotify:      petChannel,
					PointNotify:    pointChannel,
					MagicNotify:    magicChannel,
					SettingsNotify: settingsChannel,
					QuestNotify:    questChannel,
					RideNotify:     rideChannel,
					// 遇敌两场战斗的间隔可用 server_config.encounter_interval_ms 调小（STAGE-40）；
					// 野外战斗的随机源按用例名固定种子，重放的怪物组成可复现。
					EncounterIntervalMs: encounterIntervalMs(c),
					WildRnd:             rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 43)).IntN,
					// 每日运气的掷骰按用例名固定种子（STAGE-61），重放的运气和后面的战斗结果可复现。
					LuckRnd: rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 44)).IntN,
					// 出生宠物资质按用例名固定种子（STAGE-62），重放的资质可复现。
					CreateRnd: rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 45)).IntN,
					// NPC 对话选句按用例名固定种子（STAGE-69），重放的窗口正文可复现。
					NpcTalkRnd: rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 46)).IntN,
					// 合成的随机按用例名固定种子（STAGE-207），重放的配方、失败回退和冷却还回的材料可复现。
					MergeRnd: rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 47)).IntN,
					// 战斗外精灵的回复量按用例名固定种子（STAGE-230）。
					MagicUseRnd: rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 48)).IntN})
				if err != nil {
					t.Fatal(err)
				}
				// 真人组队开关（STAGE-274）：game_settings 里没有这一行时关闭，用例要开就写进去再重读。
				if v, ok := c.Meta.ServerConfig["party.players_enabled"].(bool); ok && v {
					if _, err := st.Pool.Exec(context.Background(), `INSERT INTO game_settings (key, value, note) VALUES ('party.players_enabled', 1, '真人组队开关') ON CONFLICT (key) DO UPDATE SET value = 1`); err != nil {
						t.Fatal(err)
					}
					if err := svcs.Characters.ReloadGameSettings(context.Background()); err != nil {
						t.Fatal(err)
					}
				}
				if err := svcs.Start(context.Background()); err != nil {
					t.Fatal(err)
				}
				defer svcs.Stop()
				// give_pet 的宠物资质随机按用例名固定种子，重放的结果才可复现。
				petRnd := rand.New(rand.NewPCG(casetest.SeedForCase(c.Name), 42)).IntN
				runOpts.Admin = func(step casetest.Step) error {
					switch step.Do {
					case "give_item":
						return character.GiveItem(context.Background(), st.Pool, itemChannel, step.Character, step.ItemID, step.Count)
					case "give_pet":
						_, err := character.GivePetWithLoyalty(context.Background(), st.Pool, petChannel, step.Character, step.PetID, step.GivePetLevel(), step.Loyalty, petRnd)
						return err
					case "clear_pets":
						return character.ClearPets(context.Background(), st.Pool, step.Character)
					case "set_pet_transmigration": // 宠物转生次数（STAGE-196，骑乘的转生上限用例）
						return character.SetPetTransmigration(context.Background(), st.Pool, petChannel, step.Character, step.Slot, step.Trans)
					case "set_pet_level": // 宠物转生（STAGE-236）：造 79 级玛蕾等场景，资质设为 30
						return character.SetPetLevel(context.Background(), st.Pool, petChannel, step.Character, step.Slot, step.Level)
					case "give_points":
						return character.GivePoints(context.Background(), st.Pool, pointChannel, step.Character, step.Points)
					case "give_magic":
						return character.GiveMagic(context.Background(), st.Pool, magicChannel, step.Character, step.MagicID)
					case "start_battle":
						return svcs.Characters.StartBattle(step.Character, step.Scenario)
					case "set_setting":
						// 写库并同步重读这个进程里的设置，不靠通知到达（无竞态）。
						_, err := svcs.Characters.SetGameSetting(context.Background(), step.Key, step.Value.String(), "casetest")
						return err
					case "faint_pet":
						return character.FaintPet(context.Background(), st.Pool, petChannel, step.Character, step.Slot)
					case "give_quest":
						return character.SetQuest(context.Background(), st.Pool, questChannel, step.Character, step.QuestNo, step.QuestState)
					case "set_ride_level": // 骑乘训练师（STAGE-122）：直接设骑乘等级，同管理命令 ridelevel
						return character.SetRideLevel(context.Background(), st.Pool, rideChannel, step.Character, step.Level)
					case "set_level": // 人物等级（STAGE-123，验收骑乘要 2 级）：进入世界前设
						return character.SetCharacterLevel(context.Background(), st.Pool, step.Character, step.Level)
					case "set_transmigration": // 人物转生次数（STAGE-266，主城精灵王用例）：进入世界前设，用 trans
						return character.SetCharacterTransmigration(context.Background(), st.Pool, step.Character, step.Trans)
					case "set_hp": // 血量（STAGE-306，决斗用例造一击倒下的场面）：进入世界前设
						return character.SetCharacterHP(context.Background(), st.Pool, step.Character, step.HP)
					case "set_gold": // 石币（STAGE-328，丢弃石币用例）：进入世界前设
						return character.SetCharacterGold(context.Background(), st.Pool, step.Character, step.Gold)
					case "set_duel_point": // 决斗点数（STAGE-306）：进入世界前设
						return character.SetCharacterDuelPoint(context.Background(), st.Pool, step.Character, step.DuelPoint)
					case "move_character":
						return character.MoveCharacter(context.Background(), st.Pool, step.Character, step.Floor, step.X, step.Y)
					case "chat_mute": // 公开聊天禁言 seconds 秒（STAGE-278）
						_, err := character.ChatMuteFor(context.Background(), st.Pool, step.Character, time.Duration(step.Seconds)*time.Second)
						return err
					case "chat_unmute": // 解除公开聊天禁言（STAGE-278）
						return character.ChatUnmute(context.Background(), st.Pool, step.Character)
					default:
						return svcs.Auth.RunAdminAction(context.Background(), step.Do, step.Account)
					}
				}
			} else if c.Meta.Stage != "STAGE-2" {
				t.Skipf("需要数据库：设置 %s", storetest.EnvURL)
			}
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			go srv.Serve(ln)
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(ctx)
			}()
			runErr := c.Run(ln.Addr().String(), runOpts)
			if runOpts.Record {
				if err := c.Save(); err != nil { // 失败也保存：校准是迭代过程
					t.Fatal(err)
				}
			}
			if runErr != nil {
				t.Fatal(runErr)
			}
			ran++
		})
	}
	if ran == 0 {
		t.Fatal("没有执行任何用例")
	}
}
