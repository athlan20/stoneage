package gamedata

import (
	"os"
	"strings"
	"testing"
)

// copyItemTable 复制一份物品表，覆盖表的测试只改副本，不影响其他测试。
func copyItemTable(t *testing.T) map[int]ItemTemplate {
	t.Helper()
	ItemByID(0)
	out := make(map[int]ItemTemplate, len(itemTable))
	for k, v := range itemTable {
		out[k] = v
	}
	return out
}

func TestInlayOverlayDefaultIsEmpty(t *testing.T) {
	rows, err := parseInlayOverlay(DefaultInlayOverlay())
	if err != nil || len(rows) != 0 {
		t.Fatalf("默认覆盖表应只有表头，解析得到 %d 行，err=%v", len(rows), err)
	}
	if err := applyInlayOverlay(copyItemTable(t), DefaultInlayOverlay()); err != nil {
		t.Fatalf("默认覆盖表应能加载: %v", err)
	}
	if it, _ := ItemByID(0); it.InlayCode != "" || it.InlayInit != "" {
		t.Errorf("没有覆盖表时小斧头不应有精工码，现在 %+v", it)
	}
}

func TestInlayOverlayAppliesCodes(t *testing.T) {
	tbl := copyItemTable(t)
	if err := applyInlayOverlay(tbl, "物品编号\t类别码\t初始镶嵌码\n0\tAXE\tRUBY\n1\tRUBY\t\n"); err != nil {
		t.Fatal(err)
	}
	if it := tbl[0]; it.InlayCode != "AXE" || it.InlayInit != "RUBY" || it.Name != "小斧头" {
		t.Errorf("0 号应得到类别码 AXE、初始镶嵌码 RUBY，现在 %+v", it)
	}
	if it := tbl[1]; it.InlayCode != "RUBY" || it.InlayInit != "" {
		t.Errorf("1 号应得到类别码 RUBY、没有初始镶嵌码，现在 %+v", it)
	}
	if it, _ := ItemByID(0); it.InlayCode != "" {
		t.Error("覆盖只改副本，全局物品表不应受影响")
	}
}

func TestInlayOverlayTestdata(t *testing.T) {
	data, err := os.ReadFile("../casetest/testdata/inlay_overlay.tsv")
	if err != nil {
		t.Fatal(err)
	}
	tbl := copyItemTable(t)
	if err := applyInlayOverlay(tbl, string(data)); err != nil {
		t.Fatalf("测试专用覆盖表应能加载: %v", err)
	}
	// STAGE-298：测试覆盖表给 20 号（轻的斧头）标 AXE_INSLAY、30 号带初始镶嵌码 GEM|NULL|NULL，1600 号是材料 RUBY。
	if tbl[20].InlayCode != "AXE_INSLAY" || tbl[30].InlayInit != "GEM|NULL|NULL" || tbl[1600].InlayCode != "RUBY" {
		t.Errorf("测试覆盖表没套上: 20=%+v 30=%+v 1600=%+v", tbl[20], tbl[30], tbl[1600])
	}
}

func TestInlayOverlayRejectsBadRows(t *testing.T) {
	cases := map[string]struct{ tsv, want string }{
		"编号不存在":  {"1\tRUBY\t\n1999999999\tRUBY\t\n", "1999999999"},
		"编号不是整数": {"abc\tRUBY\t\n", "abc"},
		"列数不够":   {"0\tRUBY\n", "3 列"},
		"编号重复":   {"0\tAXE\t\n0\tRUBY\t\n", "重复"},
	}
	for name, c := range cases {
		err := applyInlayOverlay(copyItemTable(t), c.tsv)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s：应报错且包含 %q，现在 err=%v", name, c.want, err)
		}
	}
}

func TestInlayOverlayUnknownIDFailsWholeTable(t *testing.T) {
	tbl := copyItemTable(t)
	err := applyInlayOverlay(tbl, "0\tAXE\tRUBY\n999999\tRUBY\t\n")
	if err == nil {
		t.Fatal("物品编号不存在应报错")
	}
	if tbl[0].InlayCode != "" {
		t.Error("整张表有错时不应写入任何模板")
	}
}

// TestItemInlayColumnsParsed 检查 STAGE-297 新加的列（TSV 第 32–37 列）按原版列号读到：
// 水灵项链（19630）的 HP 修正是 2（itemset.txt 第 43 列），其余精工列在原版数据里都是空的。
func TestItemInlayColumnsParsed(t *testing.T) {
	if it, ok := ItemByID(19630); !ok || it.HP != 2 || it.MP != 0 || it.OtherDamage != 0 || it.OtherDefc != 0 {
		t.Errorf("19630 应有 HP 2、其余为 0，现在 %+v", it)
	}
	if it, ok := ItemByID(19677); !ok || it.HP != 1 {
		t.Errorf("19677 应有 HP 1，现在 %+v", it)
	}
	if it, ok := ItemByID(0); !ok || it.InlayCode != "" || it.InlayInit != "" {
		t.Errorf("原版数据里精工码应为空，现在 %+v", it)
	}
}
