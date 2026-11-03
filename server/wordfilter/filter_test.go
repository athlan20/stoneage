package wordfilter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// STAGE-287：词表匹配（纯函数，不碰数据库）。词表用的都是内置的普通脏话和英文缩写。

func testFilter() *Filter {
	return Parse("\xEF\xBB\xBF傻逼\r\n他妈的\n# 注释\n\n  fuck  \r\nsb\n!傻逼们的歌\nass\n!class\n")
}

func TestMaskBasic(t *testing.T) {
	f := testFilter()
	cases := map[string]string{
		"你这个傻逼":     "你这个**",
		"他妈的，走":     "***，走",
		"今天天气不错":    "今天天气不错",
		"FuCk you":  "**** you",
		"傻逼傻逼":      "****",
		"":          "",
		"a":         "a",
		"他妈的 fuck ": "*** **** ",
	}
	for in, want := range cases {
		if got := f.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q，想要 %q", in, got, want)
		}
	}
}

func TestMaskEvasion(t *testing.T) {
	f := testFilter()
	cases := map[string]string{
		"傻 逼":          "***",
		"傻.逼":          "***",
		"傻，，逼":         "****",
		"傻​逼":          "***",  // 零宽空格
		"ＦＵＣＫ":         "****", // 全角英文
		"f.u.c.k":      "*******",
		"你傻́逼啊":        "你***啊",    // 夹组合记号
		"傻      逼":     "傻      逼", // 隔了 6 个空格，超过 maxGap，不算同一个词
		"傻🙂逼":          "***",
		"他\t妈\n的":      "*****",
		"xsb你":         "xsb你", // sb 前面紧挨字母，是更长单词的一部分
		"sb ":          "** ",
		"傻sb":          "傻**",
		"sb2":          "sb2",
		"class":        "class", // ass 在 class 里，且 class 在白名单
		"class ass":    "class ***",
		"c l a s s":    "c l a s s",
		"hello ass.":   "hello ***.",
		"assassin":     "assassin",
		"这是傻逼们的歌":      "这是傻逼们的歌", // 白名单词包住了屏蔽词
		"傻逼们的歌，傻逼":     "傻逼们的歌，**",
		"ASS":          "***",
		"Ａ.Ｓ.Ｓ":        "*****",
		"傻\n\n\n\n\n逼": "傻\n\n\n\n\n逼",
	}
	for in, want := range cases {
		if got := f.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q，想要 %q", in, got, want)
		}
	}
}

func TestMaskKeepsLength(t *testing.T) {
	f := testFilter()
	for _, in := range []string{"傻 逼，他妈的 FUCK!", "ｓｂ傻​逼", "ﬁ傻逼ﬁ", "😀傻😀逼😀"} {
		got := f.Mask(in)
		if utf8.RuneCountInString(got) != utf8.RuneCountInString(in) {
			t.Errorf("Mask(%q) = %q，字符数变了", in, got)
		}
	}
}

func TestContains(t *testing.T) {
	f := testFilter()
	if !f.Contains("勇者傻逼") || !f.Contains("傻-逼") || !f.Contains("SB") {
		t.Error("应当命中")
	}
	if f.Contains("勇者小明") || f.Contains("Classic") || f.Contains("class") || f.Contains("") {
		t.Error("不应命中")
	}
}

func TestParseRules(t *testing.T) {
	f := Parse("\xEF\xBB\xBF傻逼\r\n傻逼\r\n \r\n#注释\r\n逼\r\n！！\r\n!ab\r\n!单\r\nab\r\n")
	// 傻逼（重复一次只算一条）、ab；逼、单字白名单、全标点的行被忽略；!ab 是白名单。
	st := f.Stats()
	if st.Words != 2 || st.Allow != 1 || st.Skipped != 3 {
		t.Fatalf("统计不对：%+v", st)
	}
	if got := f.Mask("傻逼"); got != "**" {
		t.Fatalf("BOM 之后的第一个词没生效：%q", got)
	}
	if got := f.Mask("ab"); got != "ab" {
		t.Fatalf("ab 在白名单里，不应替换：%q", got)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if f, err := LoadFile(write("ok.txt", []byte("傻逼\r\n他妈的\n"))); err != nil || f.Stats().Words != 2 {
		t.Fatalf("正常文件应加载成功：%v", err)
	}
	if _, err := LoadFile(write("gbk.txt", []byte{0xc9, 0xb5, 0xb1, 0xc6, '\n'})); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("GBK 文件应报编码错误：%v", err)
	}
	if _, err := LoadFile(write("empty.txt", []byte("# 只有注释\n\n"))); err == nil {
		t.Fatal("没有有效词应报错")
	}
	if _, err := LoadFile(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("文件不存在应报错")
	}
}

func TestReloadKeepsOldOnFailure(t *testing.T) {
	old := Current()
	t.Cleanup(func() { Set(old) })
	dir := t.TempDir()
	good := filepath.Join(dir, "good.txt")
	_ = os.WriteFile(good, []byte("测试词语\n"), 0o644)
	if _, err := ReloadFile(good); err != nil {
		t.Fatal(err)
	}
	if got := Current().Mask("这是测试词语啊"); got != "这是****啊" {
		t.Fatalf("重载后新词表没生效：%q", got)
	}
	if _, err := ReloadFile(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("读不到应报错")
	}
	if got := Current().Mask("这是测试词语啊"); got != "这是****啊" {
		t.Fatalf("重载失败后应保留旧词表：%q", got)
	}
	if st, err := ReloadFile(""); err != nil || st.Words != 1 {
		t.Fatalf("路径为空应保持现状：%+v %v", st, err)
	}
}

func TestBuiltinDefault(t *testing.T) {
	old := Current()
	t.Cleanup(func() { Set(old) })
	Set(nil)
	if got := Current().Mask("你个傻逼"); got != "你个**" {
		t.Fatalf("内置词表应能兜底：%q", got)
	}
}

func BenchmarkMask(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 2500; i++ {
		sb.WriteString("测试词")
		sb.WriteString(string(rune(0x4e00 + i)))
		sb.WriteString(string(rune(0x4e00 + i*7%5000)))
		sb.WriteString("\n")
	}
	f := Parse(sb.String())
	text := "今天我们一起去加加村打宠物，有人要组队吗，傻 逼"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Mask(text)
	}
}
