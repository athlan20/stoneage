package character

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-230：战斗外治愈、滋润、恩惠、复活类的单元测试（不连数据库；宠物回血要写库，放在协议用例里测）。
// 精灵栏 0 是治愈的精灵 Lv1（id 0，威力 80，MP 6），1 是滋润的精灵 Lv1（id 10，威力 65，MP 6），
// 2 是复活精灵 Lv1（id 50，MP 8），3 是岩石撞击（301，战斗精灵）。
func healRecord() *record {
	r := metamoRecord()
	r.Magics = [5]int{0, 10, 50, 301, -1}
	r.MP = 50
	r.Stats.Vital, r.Stats.Strength, r.Stats.Toughness, r.Stats.Dexterity = 30, 10, 10, 10 // 最大血量 150
	return r
}

// 随机源：rolled 为 0 时回复量取下限 RAND(0.9p) 即 int(0.9×80)=72，为 n-1 时取上限。
func healService(max bool) *Service {
	return &Service{opts: Options{MagicUseRnd: func(n int) int {
		if max {
			return n - 1
		}
		return 0
	}}}
}

// 治愈：只回自己，回复量 RAND(72, 88)，封顶最大血量；MP 照扣；台词里的数字就是实际回复量。
func TestHealSelfAmountAndCap(t *testing.T) {
	r := healRecord()
	r.HP = 1
	max := r.maxHP()
	out, perr := healService(false).useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(0)}, time.Unix(1000, 0))
	if perr != nil {
		t.Fatalf("治愈失败：%v", perr)
	}
	want := min(1+72, max)
	if r.HP != want {
		t.Errorf("回血后 HP = %d，应为 %d（最大血量 %d）", r.HP, want, max)
	}
	heal := out["heal"].(map[string]any)
	if heal["amount"] != want-1 || heal["to"] != magicToSelf {
		t.Errorf("heal = %+v，应为 amount %d、to self", heal, want-1)
	}
	if r.MP != 44 {
		t.Errorf("MP = %d，应为 44（50-6）", r.MP)
	}
}

// 满血时治愈照扣 MP，回复 0（台词也照样是 0）。
func TestHealSelfFullHP(t *testing.T) {
	r := healRecord()
	r.HP = r.maxHP()
	out, perr := healService(true).useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(0)}, time.Unix(1000, 0))
	if perr != nil {
		t.Fatalf("治愈失败：%v", perr)
	}
	if heal := out["heal"].(map[string]any); heal["amount"] != 0 || r.HP != r.maxHP() {
		t.Errorf("满血回复：heal=%+v HP=%d", heal, r.HP)
	}
	if r.MP != 44 {
		t.Errorf("MP = %d，应为 44", r.MP)
	}
}

// 治愈和恩惠战斗外只回自己：带了 to 或 target 也忽略，不会因为对象无效失败。
func TestHealSelfIgnoresObject(t *testing.T) {
	r := healRecord()
	r.HP = 1
	in := magicUseRequest{Slot: slotPtr(0), To: magicToPet, Target: slotPtr(3)}
	if _, perr := healService(false).useMagic(newMetamoPresence(), r, in, time.Unix(1000, 0)); perr != nil {
		t.Fatalf("治愈带了无效对象仍应成功：%v", perr)
	}
	if r.HP == 1 {
		t.Errorf("治愈没有回血")
	}
}

// 滋润对象无效（栏位没有宠物、队友）：MP 先扣，再回复 INVALID_TARGET。
func TestHealOtherInvalidTargetSpendsMP(t *testing.T) {
	cases := []struct {
		name string
		in   magicUseRequest
	}{
		{"宠物栏位是空的", magicUseRequest{Slot: slotPtr(1), Target: slotPtr(3)}},
		{"队友（一期没有）", magicUseRequest{Slot: slotPtr(1), To: magicToMember, Target: slotPtr(0)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := healRecord()
			_, perr := healService(false).useMagic(newMetamoPresence(), r, c.in, time.Unix(1000, 0))
			if perr == nil || perr.Code != protocol.CodeInvalidTarget {
				t.Fatalf("应回复 INVALID_TARGET，得到 %+v", perr)
			}
			if r.MP != 44 {
				t.Errorf("对象无效时 MP 照原版先扣：剩 %d，应为 44", r.MP)
			}
		})
	}
}

// 滋润可以对自己（to self）：和治愈一样回自己。
func TestHealOtherSelf(t *testing.T) {
	r := healRecord()
	r.HP = 1
	out, perr := healService(false).useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(1), To: magicToSelf}, time.Unix(1000, 0))
	if perr != nil {
		t.Fatalf("滋润自己失败：%v", perr)
	}
	if heal := out["heal"].(map[string]any); heal["to"] != magicToSelf || r.HP <= 1 {
		t.Errorf("滋润自己：heal=%+v HP=%d", heal, r.HP)
	}
}

// MP 不够：回复 NOT_ENOUGH_MP，什么都不变。
func TestHealNotEnoughMP(t *testing.T) {
	r := healRecord()
	r.MP = 5
	r.HP = 1
	_, perr := healService(false).useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(0)}, time.Unix(1000, 0))
	if perr == nil || perr.Code != protocol.CodeNotEnoughMP || r.MP != 5 || r.HP != 1 {
		t.Fatalf("MP 不够应回复 NOT_ENOUGH_MP 且不变，得到 perr=%+v MP=%d HP=%d", perr, r.MP, r.HP)
	}
}

// 复活、解状态、防御类战斗外：扣 MP，没有效果，回复 heal 为 null，对象不检查（照原版）。
func TestNoEffectMagicSpendsMPOnly(t *testing.T) {
	r := healRecord()
	r.HP = 1
	out, perr := healService(false).useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(2), To: magicToPet, Target: slotPtr(3)}, time.Unix(1000, 0))
	if perr != nil {
		t.Fatalf("复活精灵战斗外应照原版扣 MP 无效果，得到 %v", perr)
	}
	if v, ok := out["heal"]; !ok || v != nil || r.HP != 1 || r.MP != 42 {
		t.Errorf("复活：heal=%v HP=%d MP=%d，应为 null、HP 不变、MP 42", v, r.HP, r.MP)
	}
}

// 攻击等其他精灵战斗外仍不能用，且不扣 MP。
func TestOtherMagicStillUnusable(t *testing.T) {
	r := healRecord()
	_, perr := healService(false).useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(3)}, time.Unix(1000, 0))
	if perr == nil || perr.Code != protocol.CodeMagicUnusable || r.MP != 50 {
		t.Fatalf("攻击精灵战斗外应回复 MAGIC_UNUSABLE 且不扣 MP，得到 %+v MP=%d", perr, r.MP)
	}
}

// 对象的解析：to 缺省时没有 target 是自己、有 target 是宠物；to 与 target 不配套、值不认识都回复 BAD_REQUEST。
func TestParseMagicObject(t *testing.T) {
	cases := []struct {
		name string
		in   magicUseRequest
		want magicObject
		code string
	}{
		{"缺省、没有 target 是自己", magicUseRequest{}, magicObject{kind: magicToSelf}, ""},
		{"缺省、有 target 是宠物", magicUseRequest{Target: slotPtr(2)}, magicObject{kind: magicToPet, index: 2}, ""},
		{"to 是 pet", magicUseRequest{To: magicToPet, Target: slotPtr(4)}, magicObject{kind: magicToPet, index: 4}, ""},
		{"to 是 member", magicUseRequest{To: magicToMember, Target: slotPtr(1)}, magicObject{kind: magicToMember, index: 1}, ""},
		{"to 是 self 忽略 target", magicUseRequest{To: magicToSelf}, magicObject{kind: magicToSelf}, ""},
		{"to 是 pet 缺 target", magicUseRequest{To: magicToPet}, magicObject{}, protocol.CodeBadRequest},
		{"to 是 member 缺 target", magicUseRequest{To: magicToMember}, magicObject{}, protocol.CodeBadRequest},
		{"target 越界", magicUseRequest{Target: slotPtr(MaxPetSlots)}, magicObject{}, protocol.CodeBadRequest},
		{"to 不认识", magicUseRequest{To: "enemy"}, magicObject{}, protocol.CodeBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, perr := parseMagicObject(c.in)
			if c.code != "" {
				if perr == nil || perr.Code != c.code {
					t.Fatalf("应回复 %s，得到 %+v", c.code, perr)
				}
				return
			}
			if perr != nil || got != c.want {
				t.Fatalf("得到 %+v, %+v，应为 %+v", got, perr, c.want)
			}
		})
	}
}
