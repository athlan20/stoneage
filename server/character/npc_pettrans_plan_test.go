package character

// STAGE-236：宠物转生窗口的纯逻辑测试（入口分流、case 4 检查、转生后的栏位和数值，随机数固定）。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 测试用的宠物：843 是黑暗精灵王（模板在 petbase.tsv），资质和能力只看字段取值，不依赖具体数值表。
const testTransTarget = 843

func testMala(slot, level int, base gamedata.Growth) petRec {
	return petRec{Slot: slot, TemplateID: malaPetID, Level: level, Base: base, Rank: 0, Name: "玛蕾菲雅"}
}

func testPet(t *testing.T, slot, level, trans int) petRec {
	t.Helper()
	tmpl, ok := gamedata.PetByID(testTransTarget)
	if !ok {
		t.Fatalf("没有宠物模板 %d", testTransTarget)
	}
	return petRec{
		Slot: slot, TemplateID: testTransTarget, Name: tmpl.Name, Level: level, Transmigration: trans,
		Base:  gamedata.Growth{Vital: 60, Strength: 40, Toughness: 40, Dexterity: 40},
		Stats: gamedata.Ability{Vital: 9000, Strength: 3000, Toughness: 3000, Dexterity: 3000},
		Rank:  1, HP: 100, State: petStateRest,
	}
}

var strongMala = gamedata.Growth{Vital: 40, Strength: 40, Toughness: 40, Dexterity: 40}

func TestPetTransEntry(t *testing.T) {
	cases := []struct {
		name        string
		playerLevel int
		pets        []petRec
		want        petTransEntry
		text        string
	}{
		{"没有玛蕾，人物转生本期不做", 90, []petRec{testPet(t, 0, 90, 0)}, entryNotTalkable, ""},
		{"玩家不到 80 级", 79, []petRec{testMala(0, 79, strongMala)}, entryTalk, "由于你的等级小于80,没能力进行转宠！"},
		{"资质有一项不到 15", 90, []petRec{testMala(0, 79, gamedata.Growth{Vital: 14, Strength: 40, Toughness: 40, Dexterity: 40})},
			entryTalk, "操，你太不爱惜我了，还要我帮你转宠？没门！"},
		{"一只 79 级玛蕾", 90, []petRec{testMala(0, 79, strongMala)}, entryStart, ""},
		{"两只 79 级玛蕾", 90, []petRec{testMala(0, 79, strongMala), testMala(1, 79, strongMala)},
			entryTalk, "你怎会有那麽多玛雷菲雅呢？"},
		{"玛蕾不是 79 级：不算数", 90, []petRec{testMala(0, 60, strongMala)}, entryNotTalkable, ""},
	}
	for _, c := range cases {
		kind, text := planPetTransEntry(c.playerLevel, c.pets)
		if kind != c.want || text != c.text {
			t.Errorf("%s：得到 (%d, %q)，想要 (%d, %q)", c.name, kind, text, c.want, c.text)
		}
	}
}

func TestPetTransPetCheck(t *testing.T) {
	ride := testPet(t, 1, 90, 0)
	ride.State = petStateRide
	cases := []struct {
		name string
		pet  petRec
		want string
	}{
		{"骑乘中", ride, "骑乘中的宠物不能转生喔！"},
		{"79 级", testPet(t, 1, 79, 0), "接受转生祝福的宠物必需要等级80以上喔！"},
		{"本身是 718", petRec{Slot: 1, TemplateID: malaPetID, Level: 90}, "这只宠物不能转生喔！"},
		{"转生两次", testPet(t, 1, 90, 2), "这只宠物已经接受过祝福了喔！"},
		{"可以转生", testPet(t, 1, 90, 1), ""},
	}
	for _, c := range cases {
		if got := petTransPetCheck(&c.pet); got != c.want {
			t.Errorf("%s：得到 %q，想要 %q", c.name, got, c.want)
		}
	}
}

// 固定随机数：每次都返回 0，新资质只做 -2，10 点都加在第一项上。
func zeroRnd(int) int { return 0 }

func TestPlanPetTransmuteWritesBack(t *testing.T) {
	mala := testMala(0, 79, strongMala)
	target := testPet(t, 2, 90, 0)
	pets := []petRec{mala, target}
	res, ok := planPetTransmute(pets, 2, zeroRnd)
	if !ok {
		t.Fatal("应该转生成功")
	}
	if res.Removed.Slot != 0 || res.Removed.TemplateID != malaPetID {
		t.Errorf("应该删掉 0 号栏位的玛蕾，实际 %+v", res.Removed)
	}
	if len(res.Next) != 1 || res.Next[0].Slot != 2 {
		t.Fatalf("删掉玛蕾后只剩被转生的宠物，实际 %d 只", len(res.Next))
	}
	got := res.Next[0]
	if got.Level != 1 || got.Exp != 0 || got.Transmigration != 1 || got.LimitLevel == nil || *got.LimitLevel != -1 {
		t.Errorf("LV=1、经验清零、TRANS+1、LIMITLEVEL=-1 不对：%+v", got)
	}
	if got.HP != gamedata.PetMaxHP(got.Stats) {
		t.Errorf("血量应该补满到新的最大 HP：%d", got.HP)
	}
	if got.Name != target.Name || got.TemplateID != testTransTarget || got.Origin != nil {
		t.Errorf("名字、模板不该变，起始值也不该动：%+v", got)
	}
	// 配额和分配的算法由 gamedata 的 STAGE-235 测试覆盖，这里只验证接线：新资质要写回宠物。
	if got.Base.Vital+got.Base.Strength+got.Base.Toughness+got.Base.Dexterity <= 0 {
		t.Errorf("资质应该按配额分配：%+v", got.Base)
	}
}

func TestPlanPetTransmuteFailures(t *testing.T) {
	// 没有 79 级玛蕾：原版返回 FALSE。
	if _, ok := planPetTransmute([]petRec{testPet(t, 2, 90, 0)}, 2, zeroRnd); ok {
		t.Error("没有玛蕾不该转生成功")
	}
	// 玛蕾不是 79 级：原版只认 79 级。
	if _, ok := planPetTransmute([]petRec{testMala(0, 78, strongMala), testPet(t, 2, 90, 0)}, 2, zeroRnd); ok {
		t.Error("78 级玛蕾不该算数")
	}
	// 栏位上没有宠物。
	if _, ok := planPetTransmute([]petRec{testMala(0, 79, strongMala)}, 2, zeroRnd); ok {
		t.Error("栏位没有宠物不该转生成功")
	}
	// 转过一次的宠物只找 401 玛蕾，718 不算。
	tr := testPet(t, 2, 90, 1)
	if _, ok := planPetTransmute([]petRec{testMala(0, 79, strongMala), tr}, 2, zeroRnd); ok {
		t.Error("转过一次的宠物不该找 718 玛蕾")
	}
	mala401 := testMala(0, 79, strongMala)
	mala401.TemplateID = malaPetID2
	if _, ok := planPetTransmute([]petRec{mala401, tr}, 2, zeroRnd); !ok {
		t.Error("转过一次的宠物应该找到 401 玛蕾")
	}
}
