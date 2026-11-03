package character

// STAGE-216：pets 响应和 pets_changed 推送都由 petsView 构造（docs/protocol.md 5.19、5.20），
// 宠物升级以后两边的 growth、quality 必须一致。需要数据库（同 pet_origin_keep_test.go）。

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestPetGrowthSameInResponseAndPush(t *testing.T) {
	e := newEnv(t)
	c := e.loggedIn("alice")
	originSetup(t, e, c, originTestPet, 1, 0) // 1 级起养
	// 在内存里把 0 号宠物升到 5 级（经验的生产路径），然后改名，改名会推一次 pets_changed。
	withRec(t, e, "alice", func(r *record) {
		gainPetExp(&r.Pets[0], expTo(5), rand.New(rand.NewPCG(9, 9)).IntN)
		e.chars.savePets(r.ID, r.Pets) // 写回数据库，改名时重新读
	})
	_, pushes := c.callp("pet_rename", `{"slot":0,"name":"小白"}`)

	var pushed []any
	for _, p := range pushes {
		if p["type"] == "pets_changed" {
			pushed = p["data"].(map[string]any)["pets"].([]any)
		}
	}
	if len(pushed) != 1 {
		t.Fatalf("改名后应推一次 pets_changed（1 只宠物），实际 %v", pushes)
	}
	pets := c.ok("pets", `{}`)["pets"].([]any)
	if len(pets) != 1 {
		t.Fatalf("pets 应有 1 只宠物，实际 %d 只", len(pets))
	}
	fromResponse := pets[0].(map[string]any)
	fromPush := pushed[0].(map[string]any)
	for _, key := range []string{"growth", "quality"} {
		if !reflect.DeepEqual(fromResponse[key], fromPush[key]) {
			t.Errorf("%s 不一致：pets 响应 %v，pets_changed 推送 %v", key, fromResponse[key], fromPush[key])
		}
	}
	// 确认两边算出的确实是成长率，不是都为 null 的假一致。
	growth := fromPush["growth"].(map[string]any)
	if growth["basis"] != growthRaised {
		t.Errorf("basis = %v，应为 raised（1 级起养）", growth["basis"])
	}
	if rates := growth["rates"].(map[string]any); rates["hp"] == nil {
		t.Errorf("升了级，rates.hp 应有数，实际 %v", rates)
	}
}
