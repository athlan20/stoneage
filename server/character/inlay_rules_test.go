// STAGE-298：精工规则的单元测试（inlay_rules.go）：校验顺序、累加、叠放只扣 1、中途镶满。
// 测试覆盖表是 server/casetest/testdata/inlay_overlay.tsv（同协议用例），这里只用它补类别码，不碰数据库。

package character

import (
	"errors"
	"os"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// loadTestOverlay 加载测试覆盖表，测试结束时还原，不污染同包里读真实数据的测试（如背包回归）。
func loadTestOverlay(t *testing.T) {
	t.Helper()
	tsv, err := os.ReadFile("../casetest/testdata/inlay_overlay.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if err := gamedata.LoadInlayOverlay(string(tsv)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gamedata.UnloadInlayOverlay)
}

func bagItem(slot, id, count int) itemRec {
	return itemRec{Slot: slot, ItemID: id, Count: count}
}

// wantInlayCode 检查错误是不是指定的错误码。
func wantInlayCode(t *testing.T, err error, code string) {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("want error %s, got %v", code, err)
	}
	if perr.Code != code {
		t.Fatalf("want code %s, got %s (%s)", code, perr.Code, perr.Message)
	}
}

func TestPlanInlayErrorOrder(t *testing.T) {
	loadTestOverlay(t)
	cases := []struct {
		name     string
		items    []itemRec
		target   int
		material []int
		code     string
	}{
		{"件数超过 3", []itemRec{bagItem(5, 20, 1), bagItem(6, 1600, 1), bagItem(7, 1600, 1), bagItem(8, 1600, 1), bagItem(9, 1600, 1)}, 5, []int{6, 7, 8, 9}, "INLAY_TOO_MANY"},
		{"材料格空", []itemRec{bagItem(5, 20, 1)}, 5, []int{6}, "EMPTY_SLOT"},
		{"目标不适合（模板里没有类别码的物品，如未进覆盖表的 100）", []itemRec{bagItem(5, 100, 1), bagItem(6, 1600, 1)}, 5, []int{6}, "INLAY_NOT_SUITABLE"},
		{"材料不适合", []itemRec{bagItem(5, 20, 1), bagItem(6, 1601, 1)}, 5, []int{6}, "INLAY_NOT_SUITABLE"},
		{"两件武器防具", []itemRec{bagItem(5, 20, 1), bagItem(6, 30, 1)}, 5, []int{6}, "INLAY_MULTI_TARGET"},
		{"目标不是武器防具", []itemRec{bagItem(5, 11, 1), bagItem(6, 1600, 1)}, 5, []int{6}, "INLAY_NO_TARGET"},
		{"目标叠放", []itemRec{bagItem(5, 20, 2), bagItem(6, 1600, 1)}, 5, []int{6}, "BAD_REQUEST"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := planInlay(c.items, c.target, c.material)
			wantInlayCode(t, err, c.code)
		})
	}
}

// 多个材料按顺序累加，最后一个材料的函数覆盖；叠放的材料只扣 1 个。
func TestPlanInlayAccumulatesAndConsumesOne(t *testing.T) {
	loadTestOverlay(t)
	items := []itemRec{bagItem(5, 20, 1), bagItem(6, 1600, 1), bagItem(7, 11, 2)}
	out, err := planInlay(items, 5, []int{6, 7})
	if err != nil {
		t.Fatal(err)
	}
	if out.Applied != 2 || out.Full || len(out.Consumed) != 2 || out.Consumed[0] != 6 || out.Consumed[1] != 7 {
		t.Fatalf("applied/consumed 不对: %+v", out)
	}
	tgt := findItem(out.Next, 5)
	if tgt == nil || tgt.Inlay == nil {
		t.Fatalf("目标没有镶嵌状态: %+v", tgt)
	}
	if got := tgt.Inlay.Bonus.Attack; got != 12 {
		t.Errorf("攻击累加 = %d，要 12（11 的攻击）", got)
	}
	if got := *tgt.Inlay.Codes[0]; got != "RUBY" {
		t.Errorf("第一格类别码 = %s，要 RUBY（先镶的 1600）", got)
	}
	if got := *tgt.Inlay.Codes[1]; got != "EMERALD" || tgt.Inlay.Codes[2] != nil {
		t.Errorf("镶嵌格 = %v", tgt.Inlay.Codes)
	}
	if m := tgt.Inlay.Magic; m == nil || m.ID != 135 || m.MP != 6 {
		t.Errorf("魔法覆盖 = %+v，要 135/6", m)
	}
	if want := "攻+31 防-11 敏-10 HP+0 MP+0 伤+0 吸+0 毒雾的精灵 Lv1"; tgt.Inlay.Effect != want {
		t.Errorf("效果文字 = %q，要 %q", tgt.Inlay.Effect, want)
	}
	if tgt.Inlay.UseFunc == nil || *tgt.Inlay.UseFunc != "" {
		t.Errorf("最后一个材料没有函数，使用函数应被清空，实际 %v", tgt.Inlay.UseFunc)
	}
	if findItem(out.Next, 6) != nil {
		t.Errorf("1600 应整格扣掉")
	}
	if m := findItem(out.Next, 7); m == nil || m.Count != 1 {
		t.Errorf("叠放的 11 应剩 1 个，实际 %+v", m)
	}
	if items[0].Inlay != nil {
		t.Errorf("输入的物品不能被改动")
	}
}

// 目标只剩 2 格：第 3 个材料遇到镶满，前两个保留，第 3 个留在背包。
func TestPlanInlayFullKeepsApplied(t *testing.T) {
	loadTestOverlay(t)
	items := []itemRec{bagItem(5, 30, 1), bagItem(6, 1600, 1), bagItem(7, 11, 1), bagItem(8, 1600, 1)}
	out, err := planInlay(items, 5, []int{6, 7, 8})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Full || out.Applied != 2 || len(out.Consumed) != 2 {
		t.Fatalf("want Full applied 2, got %+v", out)
	}
	if findItem(out.Next, 8) == nil {
		t.Errorf("第 3 个材料应留在背包")
	}
	if tgt := findItem(out.Next, 5); tgt == nil || tgt.Inlay == nil || tgt.Inlay.Codes[0] == nil || *tgt.Inlay.Codes[0] != "GEM" {
		t.Errorf("覆盖表里的初始镶嵌码 GEM 应保留在第一格: %+v", tgt)
	}
}

// 一格也镶不上（目标已经镶满）：Applied 为 0，什么都不改。
func TestPlanInlayNothingFits(t *testing.T) {
	loadTestOverlay(t)
	target := bagItem(5, 20, 1)
	target.Inlay = &inlayState{Codes: inlayCodes{ptrTo("A"), ptrTo("B"), ptrTo("C")}}
	out, err := planInlay([]itemRec{target, bagItem(6, 1600, 1)}, 5, []int{6})
	if err != nil {
		t.Fatal(err)
	}
	if out.Applied != 0 || !out.Full {
		t.Fatalf("want nothing applied and full, got %+v", out)
	}
}

// 没镶过的物品，效果随模板；镶过的，背包显示的 inlay 和效果文字跟着实例走。
func TestInlayViewAndEffectiveTemplate(t *testing.T) {
	loadTestOverlay(t)
	gear := bagItem(5, 30, 1)
	if v := itemInlayView(gear, mustItem(t, 30)); v == nil || v.Role != "gear" || *v.Free != 2 {
		t.Errorf("没镶过的装备按覆盖表初始镶嵌码算 free，GEM|NULL|NULL 应为 2，实际 %+v", v)
	}
	if v := itemInlayView(bagItem(6, 1600, 1), mustItem(t, 1600)); v == nil || v.Role != "material" || v.Free != nil {
		t.Errorf("材料只有 role，实际 %+v", v)
	}
	if v := itemInlayView(bagItem(7, 100, 1), mustItem(t, 100)); v != nil {
		t.Errorf("类别码为空的物品不带 inlay，实际 %+v", v)
	}
	eff := effectiveTemplate(mustItem(t, 20), &inlayState{Bonus: inlayBonus{Attack: 5, HP: 7}})
	if eff.Attack != 24 || eff.HP != 7 {
		t.Errorf("实际模板 attack/HP = %d/%d，要 24/7", eff.Attack, eff.HP)
	}
	if same := effectiveTemplate(mustItem(t, 20), nil); same.Attack != 19 {
		t.Errorf("没有覆盖时应原样返回，攻击 %d", same.Attack)
	}
}

func mustItem(t *testing.T, id int) gamedata.ItemTemplate {
	t.Helper()
	def, ok := gamedata.ItemByID(id)
	if !ok {
		t.Fatalf("物品 %d 不在模板里", id)
	}
	return def
}

func ptrTo(s string) *string { return &s }
