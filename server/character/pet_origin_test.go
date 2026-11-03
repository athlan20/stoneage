package character

// STAGE-215：宠物起始快照的创建路径，每一条“新宠物进库”的路径（管理命令、建角色送宠、宠物店买入、
// 战斗捕捉、任务送宠）都要写下起始值；历史宠物没有起始值；迁移的 CHECK 拒绝只写一部分的行。
// 更新路径的保证见 pet_origin_keep_test.go。

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// dbPets 直接从数据库读角色（按名字）的全部宠物，按栏位排序。
func dbPets(t *testing.T, e *env, character string) []petRec {
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
	return pets
}

// requireFreshOrigin 核对一只刚进库的宠物：起始等级和能力等于它现在的等级和能力（没有升过级、没有战斗）。
func requireFreshOrigin(t *testing.T, p petRec, why string) {
	t.Helper()
	if p.Origin == nil {
		t.Errorf("%s：%s 应有起始值，实际为空", why, p.Name)
		return
	}
	if p.Origin.Level != p.Level || p.Origin.Stats != p.Stats {
		t.Errorf("%s：%s 的起始值 %+v 应等于当前等级 %d、能力 %+v", why, p.Name, p.Origin, p.Level, p.Stats)
	}
}

// 管理命令 givepet（GivePetWithLoyalty → insertPetRow）：起始值是生成时的等级和能力（和同一随机源算出的一致）。
func TestGivePetOriginIsGeneratedValues(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")

	const tmplID, level = originTestPet, 7
	tmpl, _ := gamedata.PetByID(tmplID)
	_, wantStats := gamedata.NewPetGrowth(tmpl, level, rand.New(rand.NewPCG(1, 2)).IntN)
	if _, err := GivePet(context.Background(), e.chars.db, DefaultPetNotifyChannel, "勇者小明", tmplID, level,
		rand.New(rand.NewPCG(1, 2)).IntN); err != nil {
		t.Fatal(err)
	}
	pets := dbPets(t, e, "勇者小明")
	if len(pets) != 1 {
		t.Fatalf("应只有刚给的 1 只宠物，实际 %d 只", len(pets))
	}
	p := pets[0]
	if p.Stats != wantStats {
		t.Fatalf("宠物能力 %+v 应是生成时的 %+v", p.Stats, wantStats)
	}
	want := &PetOrigin{Level: level, Stats: wantStats}
	if p.Origin == nil || *p.Origin != *want {
		t.Fatalf("起始值 = %+v，应为 %+v", p.Origin, want)
	}
}

// 建角色送的 3 只宠物（insertPetRow，STAGE-62）：都有起始值，等级是出生等级，能力是生成时的。
func TestCreateStartPetsHaveOrigin(t *testing.T) {
	e := newEnv(t)
	e.chars.opts.CreateRnd = rand.New(rand.NewPCG(1, 2)).IntN
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	c.ok("enter_world", `{"slot":0}`)
	pets := dbPets(t, e, "勇者小明")
	if len(pets) != 3 {
		t.Fatalf("建角色应送 3 只宠物，实际 %d 只", len(pets))
	}
	for _, p := range pets {
		requireFreshOrigin(t, p, "建角色送的宠物")
		if p.Origin != nil && p.Origin.Level != gamedata.NewPetLevel {
			t.Errorf("出生宠物的起始等级应是 %d，实际 %d", gamedata.NewPetLevel, p.Origin.Level)
		}
	}
}

// 宠物店买入（petBuy → insertPetRow）：起始值是买入时生成的 NewPetLevel 级和能力。
func TestPetshopBuyHasOrigin(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	withRec(t, e, "alice", func(r *record) { r.Floor = 130 })

	c.callp("talk", fmt.Sprintf(`{"npc":%d}`, petshopNPC))
	c.callp("npc_choose", `{"no":4}`) // 买
	c.callp("npc_choose", `{"no":1}`) // 第一只在售的宠物
	if m, _ := c.callp("npc_choose", `{"no":1}`); m["ok"] != true {
		t.Fatalf("确认买入应成功: %v", m)
	}
	pets := dbPets(t, e, "勇者小明")
	if len(pets) != 1 {
		t.Fatalf("买入后应有 1 只宠物，实际 %d 只", len(pets))
	}
	requireFreshOrigin(t, pets[0], "宠物店买入")
	if pets[0].Origin != nil && pets[0].Origin.Level != gamedata.NewPetLevel {
		t.Errorf("买入宠物的起始等级应是 %d，实际 %d", gamedata.NewPetLevel, pets[0].Origin.Level)
	}
}

// 战斗捕捉（placeCapturedPet）：起始值是被抓时怪物的等级和 Ability，写库后读回仍是这个值。
func TestCapturedPetOriginIsMonsterLevelAndAbility(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)

	tmpl, _ := gamedata.PetByID(1)
	m := battle.NewMonster(tmpl, 3, 15, rand.New(rand.NewPCG(4, 4)).IntN, nil)
	var slot int
	withRec(t, e, "alice", func(r *record) {
		slot = placeCapturedPet(r, m)
		if slot < 0 {
			t.Fatal("捕捉应有空栏位")
		}
		e.chars.savePets(r.ID, r.Pets)
	})
	got := dbPetAt(t, e, "勇者小明", slot)
	want := &PetOrigin{Level: m.Level, Stats: m.Ability}
	if got.Origin == nil || *got.Origin != *want {
		t.Fatalf("捕捉入库后的起始值 = %+v，应为被抓时怪物的 %+v", got.Origin, want)
	}
	if got.Stats != m.Ability {
		t.Errorf("捕捉入库的能力 %+v 应等于怪物的 %+v", got.Stats, m.Ability)
	}
}

// 任务送宠物（GetPet → newEventPet → insertEventPet）：起始值是送出时生成的等级和能力，流程同
// TestGetPetFlowGivesPetAfterDelPet（ACCEPT 不打任务标记，起始值照写）。
func TestGetPetFlowKeepsOrigin(t *testing.T) {
	const file = "seimu/event/event02_1"
	npc := pagedFileNPC(t, file)
	setups := []func(*env){
		clearPetsSetup(t),
		putItemSetup(t, 5, 2607, 1),
		givePetSetup(t, 221, 25),
		givePetSetup(t, 222, 25),
		questStateSetup(t, 4, questDoing),
	}
	e, c := pagedEnv(t, npc, []string{file}, setups...)
	expectWindow(t, talkTo(c, npc.ID), "\n我拜托你捕抓类似鱼状的恐龙", 2)
	choose(c, 1)
	pets := dbPets(t, e, "勇者小明")
	if len(pets) != 1 {
		t.Fatalf("交易后应剩 1 只（送出的宠物），实际 %d 只", len(pets))
	}
	requireFreshOrigin(t, pets[0], "任务送出的宠物")
}

// 只写一部分起始值列的 INSERT 被数据库拒绝（CHECK 要么全空、要么全有，且等级至少 1）；全空和全有都能写。
func TestPetOriginColumnsAllOrNothing(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	ctx := context.Background()
	var charID int64
	if err := e.chars.db.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, "勇者小明").Scan(&charID); err != nil {
		t.Fatal(err)
	}
	full := []any{1, 100, 100, 100, 100}
	rejected := []struct {
		name   string
		origin []any
	}{
		{"只有等级", full[:1]},
		{"缺敏捷", full[:4]},
		{"等级为 0", []any{0, 100, 100, 100, 100}},
	}
	for _, tc := range rejected {
		if err := insertPartialOrigin(ctx, e.chars.db, charID, 3, tc.origin...); err == nil {
			t.Errorf("%s：起始值只写一部分，应被数据库拒绝", tc.name)
		}
	}
	if err := insertPartialOrigin(ctx, e.chars.db, charID, 3); err != nil {
		t.Errorf("五列全空（历史宠物）应能写入：%v", err)
	}
	if err := insertPartialOrigin(ctx, e.chars.db, charID, 4, full...); err != nil {
		t.Errorf("五列全有应能写入：%v", err)
	}
	// 读回来：全空的是历史宠物（Origin 为 nil，不倒推），全有的是结构。
	for _, p := range dbPets(t, e, "勇者小明") {
		switch p.Slot {
		case 3:
			if p.Origin != nil {
				t.Errorf("历史宠物（五列为空）读出来应是 nil，实际 %+v", p.Origin)
			}
		case 4:
			if want := (&PetOrigin{Level: 1, Stats: gamedata.Ability{Vital: 100, Strength: 100, Toughness: 100, Dexterity: 100}}); p.Origin == nil || *p.Origin != *want {
				t.Errorf("五列全有应读出 %+v，实际 %+v", want, p.Origin)
			}
		}
	}
}

// rawPetColumns 是不带起始值的整行列（insertPartialOrigin 测试用）。
const rawPetColumns = `character_id, slot, template_id, name, level, exp,
	vital, strength, toughness, dexterity, base_vital, base_strength, base_toughness, base_dexterity,
	rank, variable_ai, hp, state, skills`

// originColumnNames 是起始值的五列名，顺序同 originArgs。
var originColumnNames = []string{"origin_level", "origin_vital", "origin_strength", "origin_toughness", "origin_dexterity"}

// insertPartialOrigin 直接 INSERT 一只宠物，只写前 len(origin) 个起始值列（测试 CHECK 用）。
func insertPartialOrigin(ctx context.Context, db *pgxpool.Pool, charID int64, slot int, origin ...any) error {
	cols := rawPetColumns
	args := []any{charID, slot, 2, "测试", 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 1, 0, []int32{-1, -1, -1, -1, -1, -1, -1}}
	for i, v := range origin {
		cols += ", " + originColumnNames[i]
		args = append(args, v)
	}
	marks := make([]string, len(args))
	for i := range args {
		marks[i] = fmt.Sprintf("$%d", i+1)
	}
	_, err := db.Exec(ctx, "INSERT INTO character_pets ("+cols+") VALUES ("+strings.Join(marks, ", ")+")", args...)
	return err
}
