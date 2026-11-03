package character

import (
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

func TestNormalizeName(t *testing.T) {
	ok := map[string]string{
		"勇者小明":             "勇者小明",
		"  勇者小明\t":         "勇者小明",
		"abcdefghijklmnop": "abcdefghijklmnop", // 正好 16 个字符
		"一二三四五六七八九十一二三四五六": "一二三四五六七八九十一二三四五六",
		"Gamer": "Gamer", // 含 g、m 但不连着
		"勇者_1":  "勇者_1",
	}
	for in, want := range ok {
		got, perr := normalizeName(in)
		if perr != nil || got != want {
			t.Errorf("normalizeName(%q) = %q, %v，期望 %q", in, got, perr, want)
		}
	}
	bad := []string{
		"", "   ", "\t\n", "abcdefghijklmnopq", strings.Repeat("勇", 17), // 空、过长
		"勇 者", "勇　者", "勇\tx", "勇 者", // 空白
		"勇\x07者", "勇​者", "勇\x7f者", "�", // 控制和不可见字符
		"华义", "小华义", "神秘人物", "神秘人物乙",
		"傻逼", "傻.逼", "大傻逼王", // STAGE-287：屏蔽词表（内置词）；夹符号的写法也算
		"gm", "GM", "Gm", "gM", "xgmx", "ｇｍ", "ＧＭ", "Ｇｍ", "ｇＭ", "我是GM啊",
	}
	for _, in := range bad {
		if _, perr := normalizeName(in); perr == nil || perr.Code != protocol.CodeInvalidName || perr.Message == "" {
			t.Errorf("normalizeName(%q) 应返回 INVALID_NAME，实际 %v", in, perr)
		}
	}
}

func TestFoldName(t *testing.T) {
	if foldName("ＡbＣ１") != "abc1" || foldName("勇者") != "勇者" {
		t.Errorf("foldName 结果不对：%q", foldName("ＡbＣ１"))
	}
	if foldName("Ｇｍ") != foldName("gM") {
		t.Error("全角半角、大小写应折叠成同一个键")
	}
}

func TestStatsValidate(t *testing.T) {
	for _, s := range []Stats{{8, 4, 4, 4}, {20, 0, 0, 0}, {0, 0, 0, 20}, {5, 5, 5, 0}, {0, 0, 0, 0}} {
		if perr := s.validate(); perr != nil {
			t.Errorf("%+v 应合法: %v", s, perr)
		}
	}
	for _, s := range []Stats{{21, 0, 0, 0}, {-1, 4, 4, 4}, {8, 5, 4, 4}, {20, 20, 20, 20}, {0, 0, 0, 21}} {
		if perr := s.validate(); perr == nil || perr.Code != protocol.CodeInvalidStats {
			t.Errorf("%+v 应返回 INVALID_STATS，实际 %v", s, perr)
		}
	}
}

func TestElementsValidate(t *testing.T) {
	for _, e := range []Elements{{6, 4, 0, 0}, {10, 0, 0, 0}, {0, 0, 10, 0}, {0, 0, 0, 10}, {5, 0, 0, 5}, {0, 5, 5, 0}, {0, 3, 7, 0}, {4, 0, 0, 6}} {
		if perr := e.validate(); perr != nil {
			t.Errorf("%+v 应合法: %v", e, perr)
		}
	}
	for name, e := range map[string]Elements{
		"总和 9":   {6, 3, 0, 0},
		"总和 11":  {6, 5, 0, 0},
		"单项 11":  {11, 0, 0, 0},
		"负数":     {-1, 11, 0, 0},
		"三项":     {4, 3, 0, 3},
		"地和火":    {5, 0, 5, 0},
		"水和风":    {0, 5, 0, 5},
		"全 0":    {0, 0, 0, 0},
		"四项都有":   {3, 3, 2, 2},
		"地火水风混合": {2, 2, 3, 3},
	} {
		if perr := e.validate(); perr == nil || perr.Code != protocol.CodeInvalidElements {
			t.Errorf("%s %+v 应返回 INVALID_ELEMENTS，实际 %v", name, e, perr)
		}
	}
}

func TestAppearanceValidate(t *testing.T) {
	for _, a := range []Appearance{{0, 0, 0, 0}, {11, 3, 4, 4}, {3, 2, 1, 4}} {
		if perr := a.validate(); perr != nil {
			t.Errorf("%+v 应合法: %v", a, perr)
		}
	}
	for _, a := range []Appearance{{12, 0, 0, 0}, {-1, 0, 0, 0}, {0, 4, 0, 0}, {0, -1, 0, 0}, {0, 0, 5, 0}, {0, 0, 0, 5}, {0, 0, -1, 0}} {
		if perr := a.validate(); perr == nil || perr.Code != protocol.CodeInvalidAppearance {
			t.Errorf("%+v 应返回 INVALID_APPEARANCE，实际 %v", a, perr)
		}
	}
}
