package character

import (
	"context"
	"math/rand/v2"
	"testing"
)

// 出生送宠物（STAGE-62）：每个出生地建一个角色，进世界后 0、1、2 号栏分别是对应的出生地宠物、
// 配套虎、配套雷龙，都是 1 级、HP 满、休息。外形 3/颜色 2（身体图像 100070）配套虎 194、雷龙 253。
func TestCreateGivesStartPets(t *testing.T) {
	e := newEnv(t)
	e.chars.opts.CreateRnd = rand.New(rand.NewPCG(1, 2)).IntN
	cases := []struct {
		account  string
		hometown int
		want     [3]int
	}{
		{"alice", 0, [3]int{2, 194, 253}},
		{"bob", 1, [3]int{112, 194, 253}},
		{"carol", 2, [3]int{102, 194, 253}},
		{"dave", 3, [3]int{34, 194, 253}},
	}
	for _, tc := range cases {
		c := e.loggedIn(tc.account)
		c.ok("create_character", createJSON(0, "勇者"+tc.account, tc.hometown))
		c.ok("enter_world", `{"slot":0}`)
		pets := startPetsOf(t, c.ok("pets", `{}`))
		if len(pets) != 3 {
			t.Fatalf("%s：应有 3 只宠物，实际 %d 只", tc.account, len(pets))
		}
		for i, want := range tc.want {
			p := pets[i]
			if got := int(p["slot"].(float64)); got != i {
				t.Errorf("%s：第 %d 只应在 %d 号栏，实际 %d", tc.account, i, i, got)
			}
			if got := int(p["species_id"].(float64)); got != want {
				t.Errorf("%s：%d 号栏 species_id 应为 %d，实际 %d", tc.account, i, want, got)
			}
			if got := int(p["level"].(float64)); got != 1 {
				t.Errorf("%s：%d 号栏等级应为 1，实际 %d", tc.account, i, got)
			}
			hp, max := int(p["hp"].(float64)), int(p["max_hp"].(float64))
			if hp != max || hp <= 0 {
				t.Errorf("%s：%d 号栏 HP 应为满值，hp=%d max=%d", tc.account, i, hp, max)
			}
			if p["state"] != "rest" {
				t.Errorf("%s：%d 号栏状态应为 rest，实际 %v", tc.account, i, p["state"])
			}
		}
		c.ok("leave_world", `{}`)
	}
}

// 同一出生地、同一外形建多个角色，宠物资质随机，各不相同。
func TestCreateStartPetGrowthIsRandom(t *testing.T) {
	e := newEnv(t)
	e.chars.opts.CreateRnd = rand.New(rand.NewPCG(7, 8)).IntN
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者甲", 0))
	c.ok("create_character", createJSON(1, "勇者乙", 0))
	base0 := startPetBase(t, e, "勇者甲", 0)
	base1 := startPetBase(t, e, "勇者乙", 0)
	if base0 == base1 {
		t.Errorf("同出生地同外形的两只宠物资质应不同，都是 %v", base0)
	}
}

// 已有的角色进游戏不补发：原版只在建角时送宠物。这里删掉出生宠物后反复进出游戏，宠物页仍是空的。
func TestEnterWorldDoesNotGrantStartPets(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者甲", 0))
	clearStartPets(t, e, "勇者甲")
	for i := 1; i <= 2; i++ {
		c.ok("enter_world", `{"slot":0}`)
		if pets := startPetsOf(t, c.ok("pets", `{}`)); len(pets) != 0 {
			t.Fatalf("第 %d 次进游戏不应补发宠物，实际 %d 只", i, len(pets))
		}
		c.ok("leave_world", `{}`)
	}
}

func startPetsOf(t *testing.T, data map[string]any) []map[string]any {
	t.Helper()
	raw, ok := data["pets"].([]any)
	if !ok {
		t.Fatalf("pets 字段结构不对: %v", data["pets"])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(map[string]any))
	}
	return out
}

// startPetBase 直接读数据库里某角色某栏宠物的资质（base_vital 等），用来验证随机性。
func startPetBase(t *testing.T, e *env, character string, slot int) [4]int {
	t.Helper()
	var v, s, tough, d int
	err := e.chars.db.QueryRow(context.Background(),
		`SELECT base_vital, base_strength, base_toughness, base_dexterity FROM character_pets
		 JOIN characters ON characters.id = character_pets.character_id
		 WHERE characters.name = $1 AND character_pets.slot = $2`, character, slot).Scan(&v, &s, &tough, &d)
	if err != nil {
		t.Fatal(err)
	}
	return [4]int{v, s, tough, d}
}

// clearStartPets 删掉角色出生时送的宠物（STAGE-62），让不涉及出生送宠物的测试维持原来的前提。
func clearStartPets(t *testing.T, e *env, character string) {
	t.Helper()
	if err := ClearPets(context.Background(), e.chars.db, character); err != nil {
		t.Fatal(err)
	}
}
