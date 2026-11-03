package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// STAGE-241：批量发放的纯逻辑测试（选人清单解析、批次号、幂等计划、记录文件、参数组合）。数据库部分由冒烟和手工验收覆盖。

func TestParseNameList(t *testing.T) {
	text := "  勇者小明 \r\n\n# 注释行\n小红\n勇者小明\n  \n小红\n"
	got := parseNameList(text)
	want := []string{"勇者小明", "小红"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("清单解析不对: got %q want %q", got, want)
	}
}

func TestValidBatchName(t *testing.T) {
	for _, ok := range []string{"b1", "compensate-2026_10", "A"} {
		if !validBatchName(ok) {
			t.Errorf("%q 应当合法", ok)
		}
	}
	for _, bad := range []string{"", "a b", "批次1", "a/b", "a.b", strings.Repeat("x", 65)} {
		if validBatchName(bad) {
			t.Errorf("%q 应当拒绝", bad)
		}
	}
}

func TestPlanGrantSkipsOkAndRetriesFailed(t *testing.T) {
	recs := []grantRecord{
		{Character: "甲", Status: "ok"},
		{Character: "乙", Status: "failed"},
		{Character: "乙", Status: "ok"}, // 重试成功，最后一行为准
		{Character: "丙", Status: "failed"},
	}
	todo, skipped := planGrant([]string{"甲", "乙", "丙", "丁"}, latestStatus(recs))
	if !reflect.DeepEqual(todo, []string{"丙", "丁"}) {
		t.Errorf("待执行不对: %q", todo)
	}
	if !reflect.DeepEqual(skipped, []string{"甲", "乙"}) {
		t.Errorf("跳过不对: %q", skipped)
	}
}

func TestFailedNamesInFileOrder(t *testing.T) {
	recs := []grantRecord{
		{Character: "甲", Status: "failed"},
		{Character: "乙", Status: "ok"},
		{Character: "丙", Status: "failed"},
		{Character: "甲", Status: "ok"}, // 甲后来成功了
	}
	got := failedNames(recs)
	if !reflect.DeepEqual(got, []string{"丙"}) {
		t.Fatalf("失败列表不对: %q", got)
	}
}

func TestCheckRecordSpecRejectsMixedBatch(t *testing.T) {
	recs := []grantRecord{{Kind: "item", ID: 0, Qty: 1}}
	if err := checkRecordSpec(recs, grantSpec{Kind: "item", ID: 0, Qty: 1}); err != nil {
		t.Fatalf("同样的物资不应拒绝: %v", err)
	}
	for _, s := range []grantSpec{
		{Kind: "item", ID: 1, Qty: 1},
		{Kind: "item", ID: 0, Qty: 2},
		{Kind: "pet", ID: 0, Qty: 1},
	} {
		if err := checkRecordSpec(recs, s); err == nil {
			t.Errorf("%+v 与记录不一致，应当拒绝", s)
		}
	}
}

func TestRecordFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "b1.jsonl")
	recs, err := readGrantRecords(path)
	if err != nil || recs != nil {
		t.Fatalf("文件不存在时应返回空: %v %v", recs, err)
	}
	in := []grantRecord{
		{Batch: "b1", Character: "甲", Kind: "item", ID: 0, Qty: 1, Status: "ok", Time: "t1"},
		{Batch: "b1", Character: "乙", Kind: "item", ID: 0, Qty: 1, Status: "failed", Error: "背包已满", Time: "t2"},
	}
	for _, r := range in {
		if err := appendGrantRecord(path, r); err != nil {
			t.Fatalf("追加失败: %v", err)
		}
	}
	out, err := readGrantRecords(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("往返不一致:\n got %+v\nwant %+v", out, in)
	}
	spec, ok := specFromRecords(out)
	if !ok || spec != (grantSpec{Kind: "item", ID: 0, Qty: 1}) {
		t.Fatalf("从记录取物资不对: %+v %v", spec, ok)
	}
}

func TestParseGrantArgsRequiresExplicitZeroID(t *testing.T) {
	// 小斧头是编号 0：--id 0 必须算“给了编号”，不能被当成没填。
	f, err := parseGrantArgs([]string{"--kind", "item", "--id", "0", "--batch", "b1", "--names-file", "x.txt"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := buildGrantSpec(f)
	if err != nil {
		t.Fatalf("编号 0 应当合法: %v", err)
	}
	if spec != (grantSpec{Kind: "item", ID: 0, Qty: 1}) {
		t.Fatalf("物资不对: %+v", spec)
	}
}

func TestBuildGrantSpecRejectsBadCombinations(t *testing.T) {
	cases := map[string][]string{
		"缺编号":         {"--kind", "item", "--batch", "b1", "--names-file", "x"},
		"pet 带 count": {"--kind", "pet", "--id", "1", "--count", "2", "--batch", "b1", "--names-file", "x"},
		"point 带 id":  {"--kind", "point", "--points", "3", "--id", "1", "--batch", "b1", "--names-file", "x"},
		"未知类型":        {"--kind", "gold", "--id", "1", "--batch", "b1", "--names-file", "x"},
		"物品不存在":       {"--kind", "item", "--id", "999999", "--batch", "b1", "--names-file", "x"},
		"点数为 0":       {"--kind", "point", "--points", "0", "--batch", "b1", "--names-file", "x"},
		"宠物等级超界":      {"--kind", "pet", "--id", "1", "--level", "999", "--batch", "b1", "--names-file", "x"},
	}
	for what, args := range cases {
		f, err := parseGrantArgs(args)
		if err != nil {
			t.Errorf("%s：解析不该失败: %v", what, err)
			continue
		}
		if _, err := buildGrantSpec(f); err == nil {
			t.Errorf("%s：应当报错", what)
		}
	}
}

func TestBuildGrantSpecDefaults(t *testing.T) {
	f, _ := parseGrantArgs([]string{"--kind", "pet", "--id", "1", "--batch", "b1", "--all"})
	if s, err := buildGrantSpec(f); err != nil || s.Qty != 1 {
		t.Errorf("宠物不写等级应为 1: %+v %v", s, err)
	}
	f, _ = parseGrantArgs([]string{"--kind", "magic", "--id", "301", "--batch", "b1", "--all"})
	if s, err := buildGrantSpec(f); err != nil || s != (grantSpec{Kind: "magic", ID: 301, Qty: 1}) {
		t.Errorf("精灵数量应固定为 1: %+v %v", s, err)
	}
}

func TestBuildGrantSpecGold(t *testing.T) {
	f, _ := parseGrantArgs([]string{"--kind", "gold", "--amount", "5000000", "--batch", "b1", "--all"})
	if s, err := buildGrantSpec(f); err != nil || s != (grantSpec{Kind: "gold", ID: 0, Qty: 5000000}) {
		t.Fatalf("石币规格不对: %+v %v", s, err)
	}
	f, _ = parseGrantArgs([]string{"--kind", "gold", "--batch", "b1", "--all"})
	if _, err := buildGrantSpec(f); err == nil {
		t.Fatal("石币没给 --amount 应该报错")
	}
}

func TestCheckSourceFlags(t *testing.T) {
	cases := map[string]struct {
		args []string
		bad  bool
	}{
		"无人群":        {[]string{"--kind", "item", "--id", "0"}, true},
		"两种人群":       {[]string{"--names-file", "a", "--all"}, true},
		"过滤没配 all":   {[]string{"--names-file", "a", "--min-level", "5"}, true},
		"日期格式错":      {[]string{"--all", "--registered-before", "2026/10/01"}, true},
		"all 带过滤":    {[]string{"--all", "--registered-before", "2026-10-01", "--min-level", "5"}, false},
		"retry 单独用":  {[]string{"--retry"}, false},
		"retry 带人群":  {[]string{"--retry", "--all"}, true},
		"retry 带 id": {[]string{"--retry", "--id", "0"}, true},
		"账号清单":       {[]string{"--accounts-file", "a"}, false},
	}
	for what, c := range cases {
		f, err := parseGrantArgs(c.args)
		if err != nil {
			t.Errorf("%s：解析失败 %v", what, err)
			continue
		}
		err = checkSourceFlags(f)
		if c.bad && err == nil {
			t.Errorf("%s：应当报错", what)
		}
		if !c.bad && err != nil {
			t.Errorf("%s：不该报错: %v", what, err)
		}
	}
}
