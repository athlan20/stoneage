package character

// STAGE-313：五兄弟之迷的任务 18、19（事件 28、31）端到端（纯逻辑，不连数据库）：用真实白名单、真实选块
// pickEventBlock 和真实交易 planEvent，每一步断言命中的块、道具和宠物增减、事件位变化。
// 反向用例覆盖等级不够、拿错信物、信物不齐、完成后再来。协议层（对话窗口、推送）不在这里测。
// 脚本的数据检查（白名单、整本支持、NPC 类型和地图、道具和宠物都查得到）见本文件末尾的 TestEv28ScriptData。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	ev28Gino    = 1450 // 吉诺（ev28_0，31701）：接任务 18
	ev28Kilos   = 1466 // 奇洛斯（ev28_end，31705）：收五个信物给 2517，再接任务 19
	ev28Proros  = 1472 // 浦洛斯（ev28_ed4，31706）：收 2517，给 2526 和灰人龙
	ev28Fake1   = 1445 // 31701 的错误选项（ev28_1）：只给错误信物 2508
	ev28Fake2   = 1452 // 31702 的错误选项（ev28_6）：只给错误信物 2510
	ev28Quest18 = 28   // 任务 18 的事件号
	ev28Quest19 = 31   // 任务 19 的事件号

	ev28Crystal = 2517 // 智慧的结晶
)

// ev28Tokens 是五层各自的正确信物 NPC 和信物（按楼层 1~5）。
var ev28Tokens = []struct{ npc, item int }{
	{1446, 2507}, // ev28_5，31701
	{1451, 2509}, // ev28_8，31702
	{1455, 2511}, // ev28_12，31703
	{1462, 2513}, // ev28_15，31704
	{1465, 2515}, // ev28_19，31705
}

// ev28Pets 是任务 19 的五个交付 NPC：脚本、NPC、领到的宠物模板和纪念品。
var ev28Pets = []struct {
	file     string
	npc      int
	template int
	memento  int
}{
	{"giiru/event/ev28_ed1", 1471, 91, 2518},
	{"giiru/event/ev28_ed2", 1473, 94, 2519},
	{"giiru/event/ev28_ed3", 1470, 92, 2520},
	{"giiru/event/ev28_ed4", 1472, 93, 2526},
	{"giiru/event/ev28_ed5", 1474, 95, 2525},
}

// ev28Pick 先确认 NPC 的脚本在真实白名单里，再用真实选块取命中的块（want 为块序号，-1 表示闲聊）。
func ev28Pick(t *testing.T, s *Service, r *record, npc, want int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	if !gamedata.NpcEventEnabled(script.File) {
		t.Fatalf("%s 不在白名单里，NPC %d 对话会说“还不能对话”", script.File, npc)
	}
	return kingPick(t, s, r, npc, want)
}

// ev28Walk 走完任务 18：吉诺接取，五层各拿一个正确信物（顺带踩一个错误选项再换回来），奇洛斯换 2517。
func ev28Walk(t *testing.T, s *Service, r *record) {
	t.Helper()
	kingRun(t, r, ev28Pick(t, s, r, ev28Gino, 0)) // REQUEST 28：接取
	if !r.eventDoing(ev28Quest18) {
		t.Fatalf("接取后事件 28 应进行中，实际 %v", r.EventFlags)
	}
	for i, tk := range ev28Tokens {
		kingRun(t, r, ev28Pick(t, s, r, tk.npc, 2)) // 没有任何信物：第 3 块（给信物）
		if countItem(r.Items, tk.item) != 1 {
			t.Fatalf("第 %d 层应拿到信物 %d，实际 %v", i+1, tk.item, r.Items)
		}
	}
	kingRun(t, r, ev28Pick(t, s, r, ev28Kilos, 0)) // ACCEPT 28：收五个信物，给 2517，完成
}

func ev28Hero() *record {
	r := kingRecord(nil, nil, nil)
	r.Level = 126
	return r
}

// 任务 18 正向：吉诺接取 → 五层信物 → 奇洛斯换 2517，信物全部收走，事件 28 完成。
func TestEv28Quest18WalksToCrystal(t *testing.T) {
	s := kingService()
	r := ev28Hero()
	ev28Walk(t, s, r)
	if countItem(r.Items, ev28Crystal) != 1 || !r.eventDone(ev28Quest18) || r.eventDoing(ev28Quest18) {
		t.Fatalf("应拿到 2517、事件 28 完成，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	for _, tk := range ev28Tokens {
		if countItem(r.Items, tk.item) != 0 {
			t.Fatalf("信物 %d 应被收走，实际 %v", tk.item, r.Items)
		}
	}
	ev28Pick(t, s, r, ev28Gino, 1) // 完成后吉诺只说感谢话（MESSAGE）
}

// 任务 19 正向：任务 18 完成后奇洛斯接任务 19（再给一个 2517 兜底）；五个交付 NPC 各领一只宠物和纪念品。
func TestEv28Quest19GivesEachDragon(t *testing.T) {
	for _, d := range ev28Pets {
		t.Run(d.file, func(t *testing.T) {
			s := kingService()
			r := ev28Hero()
			ev28Walk(t, s, r)
			kingRun(t, r, ev28Pick(t, s, r, ev28Kilos, 2)) // REQUEST 31：已有 2517，只接任务
			if !r.eventDoing(ev28Quest19) || countItem(r.Items, ev28Crystal) != 1 {
				t.Fatalf("接任务 19 后事件 31 应进行中、2517 还在，实际 %v flags=%v", r.Items, r.EventFlags)
			}
			kingRun(t, r, ev28Pick(t, s, r, d.npc, 0))
			if countItem(r.Items, ev28Crystal) != 0 || countItem(r.Items, d.memento) != 1 || len(r.Pets) != 1 {
				t.Fatalf("交 2517 后应得 %d 和一只宠物，实际 items=%v pets=%d", d.memento, r.Items, len(r.Pets))
			}
			if !r.eventDone(ev28Quest19) || r.eventDoing(ev28Quest19) {
				t.Fatalf("事件 31 应完成，实际 %v", r.EventFlags)
			}
			ev28Pick(t, s, r, d.npc, 1) // 拿到宠物后再来：命中第 2 块 MESSAGE（只说话），不会再给
			if len(r.Pets) != 1 {
				t.Fatalf("再对话不应重复给宠物，实际 %d 只", len(r.Pets))
			}
		})
	}
}

// 丢了 2517 的补救：任务 18 完成、2517 不在手上时，奇洛斯再给一个（REQUEST 31 的第 1 块）。
func TestEv28CrystalReissue(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{ev28Quest18}, nil)
	r.Level = 126
	kingRun(t, r, ev28Pick(t, s, r, ev28Kilos, 1))
	if countItem(r.Items, ev28Crystal) != 1 || !r.eventDoing(ev28Quest19) {
		t.Fatalf("应补一个 2517 并进入任务 19，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：等级不超过 30 时吉诺不接任务（REQUEST 的 LV>30 条件），什么都不改。
func TestEv28LevelGate(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil) // Level 1
	ev28Pick(t, s, r, ev28Gino, -1)
	if r.eventDoing(ev28Quest18) || len(r.Items) != 0 {
		t.Fatalf("1 级不应接到任务 18，实际 %v flags=%v", r.Items, r.EventFlags)
	}
	r.Level = 30
	ev28Pick(t, s, r, ev28Gino, -1) // 条件是 LV>30，30 级也不行
	r.Level = 31
	ev28Pick(t, s, r, ev28Gino, 0)
}

// 反向：错误选项只给错误信物（2508、2510），奇洛斯不认；走回正确选项会把错误信物换成正确的。
func TestEv28FakeTokenNotAccepted(t *testing.T) {
	s := kingService()
	r := ev28Hero()
	kingRun(t, r, ev28Pick(t, s, r, ev28Gino, 0))
	kingRun(t, r, ev28Pick(t, s, r, ev28Fake1, 2))
	if countItem(r.Items, 2508) != 1 || countItem(r.Items, 2507) != 0 {
		t.Fatalf("错误选项应给 2508，实际 %v", r.Items)
	}
	kingRun(t, r, ev28Pick(t, s, r, ev28Tokens[0].npc, 1)) // 有 2508：换成 2507
	if countItem(r.Items, 2508) != 0 || countItem(r.Items, 2507) != 1 {
		t.Fatalf("正确选项应把 2508 换成 2507，实际 %v", r.Items)
	}
	kingRun(t, r, ev28Pick(t, s, r, ev28Fake2, 2))
	if countItem(r.Items, 2510) != 1 {
		t.Fatalf("31702 的错误选项应给 2510，实际 %v", r.Items)
	}
}

// 反向：信物不齐（缺第 5 层）时奇洛斯不换 2517，事件 28 仍进行中。
func TestEv28IncompleteTokens(t *testing.T) {
	s := kingService()
	r := ev28Hero()
	kingRun(t, r, ev28Pick(t, s, r, ev28Gino, 0))
	for _, tk := range ev28Tokens[:4] {
		kingRun(t, r, ev28Pick(t, s, r, tk.npc, 2))
	}
	blk := ev28Pick(t, s, r, ev28Kilos, 3) // MESSAGE：催促，收走所有信物
	if blk.Type != "MESSAGE" {
		t.Fatalf("信物不齐应命中 MESSAGE 块，实际 %s", blk.Type)
	}
	if countItem(r.Items, ev28Crystal) != 0 || r.eventDone(ev28Quest18) {
		t.Fatalf("信物不齐不应完成任务 18，实际 %v flags=%v", r.Items, r.EventFlags)
	}
}

// 反向：任务 18 没完成时，五个交付 NPC 都不给宠物。
func TestEv28DragonNeedsQuest18(t *testing.T) {
	s := kingService()
	for _, d := range ev28Pets {
		r := kingRecord(kingItems(ev28Crystal), nil, []int{ev28Quest18})
		r.Level = 126
		ev28Pick(t, s, r, d.npc, -1)
		if len(r.Pets) != 0 {
			t.Fatalf("%s：任务 18 未完成不应给宠物", d.file)
		}
	}
}

// 数据检查：放开的 29 个脚本都在白名单里、整本支持；`ev28_23`（脚本里 DelItem 拼成 elItem）不放开；
// NPC 是任务类型且在正式地图上；用到的道具和宠物都查得到；五个交付 NPC 领到的宠物模板对得上。
func TestEv28ScriptData(t *testing.T) {
	opened := 0
	for npcID := 1445; npcID <= 1474; npcID++ {
		script, ok := gamedata.NpcEventOf(npcID)
		if !ok {
			t.Fatalf("NPC %d 没有任务脚本", npcID)
		}
		if script.File == "giiru/event/ev28_23" {
			if gamedata.NpcEventEnabled(script.File) {
				t.Errorf("%s 含不支持的块，不应放开", script.File)
			}
			continue
		}
		opened++
		if !gamedata.NpcEventEnabled(script.File) {
			t.Errorf("%s 应已放开", script.File)
		}
		npc, ok := gamedata.NPCByID(npcID)
		if !ok || npc.Kind != "changeevent" || !gamedata.FormalFloor(npc.Floor) {
			t.Errorf("%s 的 NPC %d 应是任务类型、在正式地图上，实际 %+v", script.File, npcID, npc)
		}
		for i := range script.Blocks {
			b := &script.Blocks[i]
			if b.Unsupported != "" {
				t.Errorf("%s 块 %d 不支持：%s", script.File, i, b.Unsupported)
			}
			for _, g := range b.Cond {
				for _, term := range g {
					if term.Op == "ITEM" {
						checkKingItem(t, script.File, term.ID)
					}
				}
			}
			for _, it := range append(append([]gamedata.ItemCount{}, b.GetItem...), b.DelItem...) {
				checkKingItem(t, script.File, it.ID)
			}
			for _, enemy := range b.GetPet {
				e, ok := gamedata.EnemyByID(enemy)
				if !ok {
					t.Errorf("%s 块 %d 的怪物 %d 不在敌人表里", script.File, i, enemy)
					continue
				}
				if _, ok := gamedata.PetByID(e.Template); !ok {
					t.Errorf("%s 块 %d 的怪物 %d 的宠物模板 %d 不在宠物表里", script.File, i, enemy, e.Template)
				}
			}
		}
	}
	if opened != 29 {
		t.Errorf("应放开 29 个脚本，实际 %d", opened)
	}
	for _, d := range ev28Pets {
		script, _ := gamedata.NpcEventOf(d.npc)
		if script.File != d.file {
			t.Fatalf("NPC %d 应对应 %s，实际 %s", d.npc, d.file, script.File)
		}
		enemy := script.Blocks[0].GetPet[0]
		if e, _ := gamedata.EnemyByID(enemy); e.Template != d.template {
			t.Errorf("%s 领到的宠物模板应是 %d，实际 %d", d.file, d.template, e.Template)
		}
	}
}
