package character

// STAGE-215：宠物起始快照的“不改它”保证，所有会更新 character_pets 的路径（协议操作、战后结算与写回、
// 整行重写、ON CONFLICT 写回、管理命令、宠物店存取）都要保留 origin_* 五列。这里逐条走真实路径，
// 写完直接读数据库核对起始值，而不是只看内存。

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// originTestPet 是这组测试用的宠物：乌力乌力（模板 2，和宠物店测试同一只）。
const originTestPet = 2

// originTestRide 是能骑的虎（模板 194，外形 3/颜色 2 能骑，docs/ride.md）。
const originTestRide = 194

// originSetup 建角色“勇者小明”（清掉出生送宠），用管理命令在 0 号栏给一只 template 号、level 级、
// 忠诚 loyalty 的宠物，进世界，返回它的起始值（从数据库读，作为之后各条路径的对照）。
func originSetup(t *testing.T, e *env, c *client, template, level, loyalty int) *PetOrigin {
	t.Helper()
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	if _, err := GivePetWithLoyalty(context.Background(), e.chars.db, DefaultPetNotifyChannel, "勇者小明",
		template, level, loyalty, rand.New(rand.NewPCG(5, 6)).IntN); err != nil {
		t.Fatal(err)
	}
	c.ok("enter_world", `{"slot":0}`)
	before := dbPetAt(t, e, "勇者小明", 0)
	if before.Origin == nil || before.Origin.Level != level {
		t.Fatalf("管理命令给的宠物应有起始等级 %d，实际 %+v", level, before.Origin)
	}
	if before.Origin.Stats != before.Stats {
		t.Fatalf("刚给的宠物起始能力应等于当前能力：起始 %+v，当前 %+v", before.Origin.Stats, before.Stats)
	}
	return before.Origin
}

// dbPetAt 直接从数据库读角色（按名字）某栏的宠物，不经过内存，用来核对写回的结果。
func dbPetAt(t *testing.T, e *env, character string, slot int) petRec {
	t.Helper()
	ctx := context.Background()
	var charID int64
	if err := e.chars.db.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, character).Scan(&charID); err != nil {
		t.Fatal(err)
	}
	pets, err := readPets(ctx, e.chars.db, charID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pets {
		if p.Slot == slot {
			return p
		}
	}
	t.Fatalf("数据库里 %s 的 %d 号栏没有宠物", character, slot)
	return petRec{}
}

// requireOrigin 核对数据库里 slot 栏宠物的起始值仍是 want（两边都是 nil 也算相同）。
func requireOrigin(t *testing.T, e *env, slot int, want *PetOrigin, why string) {
	t.Helper()
	got := dbPetAt(t, e, "勇者小明", slot).Origin
	same := got == nil && want == nil || got != nil && want != nil && *got == *want
	if !same {
		t.Errorf("%s：起始值被改了，数据库里 %+v，应为 %+v", why, got, want)
	}
}

// 升级（gainPetExp）后整张写回：起始值不变，等级和能力变了。
func TestOriginKeptOnLevelUpAndSave(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	withRec(t, e, "alice", func(r *record) {
		gainPetExp(&r.Pets[0], expTo(5), rand.New(rand.NewPCG(9, 9)).IntN)
		if r.Pets[0].Level <= 1 {
			t.Fatalf("应升级，实际 %d 级", r.Pets[0].Level)
		}
		e.chars.savePets(r.ID, r.Pets)
	})
	got := dbPetAt(t, e, "勇者小明", 0)
	if got.Level <= 1 || got.Stats == want.Stats {
		t.Fatalf("升级写回后数据库里的等级和能力应变了：%d 级、能力 %+v", got.Level, got.Stats)
	}
	requireOrigin(t, e, 0, want, "升级写回后")
}

// 战后结算（经验、击杀数，applyResult 同生产路径）再整张写回：起始值不变。
func TestOriginKeptOnBattleSettleAndSave(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	withRec(t, e, "alice", func(r *record) {
		end := &battle.EndView{Result: "win", Rewards: &battle.Rewards{
			Exp:      []battle.RewardExp{{Who: "pet", Slot: intPtr(0), Gain: expTo(3)}},
			PetKills: []battle.PetKill{{Slot: 0, Kills: 2}},
		}}
		e.chars.applyResult(r, &battle.Battle{Rnd: rand.New(rand.NewPCG(3, 4)).IntN}, end)
		e.chars.savePets(r.ID, r.Pets)
	})
	got := dbPetAt(t, e, "勇者小明", 0)
	if got.Level <= 1 || got.Kills != 2 {
		t.Fatalf("战后写回后应升级且击杀数为 2，实际 %d 级、击杀 %d", got.Level, got.Kills)
	}
	requireOrigin(t, e, 0, want, "战后结算写回后")
}

// 历史宠物（五列为空，迁移前写入的行）经过战后写回仍是空：不倒推、不补默认值。
func TestHistoricPetStaysNilAfterSave(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	originSetup(t, e, c, originTestPet, 1, 0)
	if _, err := e.chars.db.Exec(context.Background(), `UPDATE character_pets SET origin_level = NULL,
		origin_vital = NULL, origin_strength = NULL, origin_toughness = NULL, origin_dexterity = NULL`); err != nil {
		t.Fatal(err)
	}
	// 退出再进入，让内存里的宠物按库里的样子（空的起始值）重新读一遍。
	c.ok("leave_world", `{}`)
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) {
		if r.Pets[0].Origin != nil {
			t.Fatalf("历史宠物读进内存应是 nil，实际 %+v", r.Pets[0].Origin)
		}
		gainPetExp(&r.Pets[0], expTo(3), rand.New(rand.NewPCG(9, 9)).IntN)
		e.chars.savePets(r.ID, r.Pets)
	})
	if got := dbPetAt(t, e, "勇者小明", 0); got.Level <= 1 || got.Origin != nil {
		t.Fatalf("历史宠物写回后应升级、起始值仍为空，实际 %d 级、起始值 %+v", got.Level, got.Origin)
	}
}

// 任务送宠物的整行重写（writePetDiff 先删后插）：被重写的宠物仍带着原来的起始值。
func TestOriginKeptOnEventRewrite(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	ctx := context.Background()
	withRec(t, e, "alice", func(r *record) {
		before := append([]petRec(nil), r.Pets...)
		after := append([]petRec(nil), r.Pets...)
		after[0].State = petStateStandby // 栏位上的宠物变了，writePetDiff 会删掉它再插回来
		tx, err := e.chars.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := writePetDiff(ctx, tx, r.ID, before, after); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	})
	if got := dbPetAt(t, e, "勇者小明", 0); got.State != petStateStandby {
		t.Fatalf("整行重写应生效（状态变成待命），实际 %d", got.State)
	}
	requireOrigin(t, e, 0, want, "整行重写后")
}

// 写回时内存里的起始值和库里不一样：冲突走 DO UPDATE，它不列 origin_*，库里的起始值保留。
func TestSavePetConflictKeepsOriginColumns(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	withRec(t, e, "alice", func(r *record) {
		p := r.Pets[0]
		p.Name = "改过的名字" // 这一列会被 DO UPDATE 写回，证明确实走的是冲突更新
		p.Origin = &PetOrigin{Level: 99, Stats: gamedata.Ability{Vital: 1, Strength: 1, Toughness: 1, Dexterity: 1}}
		if err := e.chars.savePet(context.Background(), r.ID, p); err != nil {
			t.Fatal(err)
		}
	})
	if got := dbPetAt(t, e, "勇者小明", 0); got.Name != "改过的名字" {
		t.Fatalf("冲突写回应更新名字，实际 %q", got.Name)
	}
	requireOrigin(t, e, 0, want, "冲突写回后")
}

// 协议操作：设状态（pet_state）、改名（pet_rename）后起始值不变。
func TestOriginKeptOnStateAndRename(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	okCall(t, c, "pet_state", `{"slot":0,"state":"standby"}`)
	requireOrigin(t, e, 0, want, "设为待命后")
	okCall(t, c, "pet_rename", `{"slot":0,"name":"小乌"}`)
	if got := dbPetAt(t, e, "勇者小明", 0); got.Name != "小乌" {
		t.Fatalf("改名应生效，实际 %q", got.Name)
	}
	requireOrigin(t, e, 0, want, "改名后")
}

// 骑乘上下骑（pet_ride、pet_unride）后起始值不变。
func TestOriginKeptOnRide(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestRide, 1, 100)
	withRec(t, e, "alice", func(r *record) { r.RideLevel = 40 })
	okCall(t, c, "pet_ride", `{"slot":0}`)
	if got := dbPetAt(t, e, "勇者小明", 0); got.State != petStateRide {
		t.Fatalf("上骑后状态应是骑乘，实际 %d", got.State)
	}
	requireOrigin(t, e, 0, want, "上骑后")
	okCall(t, c, "pet_unride", `{}`)
	requireOrigin(t, e, 0, want, "下骑后")
}

// okCall 发一条会推送宠物页的操作（pet_state 等先推 pets_changed 再回应答），确认应答 ok。
func okCall(t *testing.T, c *client, typ, data string) {
	t.Helper()
	if m, _ := c.callp(typ, data); m["ok"] != true {
		t.Fatalf("%s 应成功: %v", typ, m)
	}
}

// 宠物店寄放、领取：宠物换栏位（寄放池 50+）再换回，起始值不变。
func TestOriginKeptOnPoolAndGet(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	withRec(t, e, "alice", func(r *record) { r.Floor = 130 }) // 宠物店所在地图（同 petshop_test）
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":1}`) // 寄放
	c.callp("npc_choose", `{"no":0}`) // 0 号宠物
	c.callp("npc_choose", `{"no":1}`) // 确认
	if got := dbPetAt(t, e, "勇者小明", gamedata.PooledSlotBase); got.State != petStatePooled {
		t.Fatalf("寄放后应在寄放池，实际状态 %d", got.State)
	}
	requireOrigin(t, e, gamedata.PooledSlotBase, want, "寄放后")

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":2}`) // 领取
	c.callp("npc_choose", `{"no":0}`) // 池位 0
	if got := dbPetAt(t, e, "勇者小明", 0); got.State != petStateRest {
		t.Fatalf("领取后应回到随身栏位，实际状态 %d", got.State)
	}
	requireOrigin(t, e, 0, want, "领取后")
}

// 宠物店学技能（petskillshop 的 UPDATE skills）后起始值不变。
func TestOriginKeptOnSkillLearn(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, petskillShopPetTemplate, 5, 0)
	withRec(t, e, "alice", func(r *record) {
		r.Floor, r.X, r.Y = 1003, 18, 13
		r.Gold = 10000
	})
	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petskillShopNPC))
	c.callp("npc_choose", `{"no":2}`) // 攻击
	c.callp("npc_choose", `{"no":1}`) // 0 号宠物（凯比）
	c.callp("npc_choose", `{"no":3}`) // 3 号技能栏位
	if m, _ := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("确认学习应成功: %v", m)
	}
	requireOrigin(t, e, 0, want, "学技能后")
}

// 转生次数设置（管理命令 set_pet_transmigration）和战斗中倒下（管理命令 faint_pet，只改血量）后起始值不变。
func TestOriginKeptOnAdminUpdates(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	want := originSetup(t, e, c, originTestPet, 1, 0)
	ctx := context.Background()
	if err := SetPetTransmigration(ctx, e.chars.db, DefaultPetNotifyChannel, "勇者小明", 0, 2); err != nil {
		t.Fatal(err)
	}
	if got := dbPetAt(t, e, "勇者小明", 0); got.Transmigration != 2 {
		t.Fatalf("转生次数应设为 2，实际 %d", got.Transmigration)
	}
	requireOrigin(t, e, 0, want, "设转生次数后")
	if err := FaintPet(ctx, e.chars.db, DefaultPetNotifyChannel, "勇者小明", 0); err != nil {
		t.Fatal(err)
	}
	if got := dbPetAt(t, e, "勇者小明", 0); got.HP != 0 {
		t.Fatalf("倒下后血量应为 0，实际 %d", got.HP)
	}
	requireOrigin(t, e, 0, want, "倒下后")
}
