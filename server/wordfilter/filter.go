// Package wordfilter 是聊天和名字共用的屏蔽词匹配器（STAGE-287，docs/chat.md 1.3）。
//
// 匹配前先把文字归一化：全角转半角（NFKC）、转小写、丢掉空白、标点、符号和零宽字符，
// 所以「傻 逼」「傻.逼」「ＳＢ」都和词表里的写法等价；命中后把**原文**里从头到尾的那一段
// （含夹在中间的符号）整体换成 *，长度不变。
package wordfilter

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	// minWordChars 是词表里一个词归一化后的最少字符数：单字的词会把正常聊天大面积替换掉，一律忽略。
	minWordChars = 2
	// maxGap 是命中的相邻两个字符之间在原文里最多夹几个被丢掉的字符（空格、标点之类），
	// 超过就不算同一个词，避免把隔了一整句话的两个字拼成词。
	maxGap = 3
)

// Stats 是一次解析词表的统计，用来在日志和管理命令里回显。
type Stats struct {
	Words   int // 屏蔽词条数（去重后）
	Allow   int // 白名单条数
	Skipped int // 被忽略的条数（归一化后为空或不足两个字符）
}

// Filter 是解析好的词表，只读，可以被多个协程同时使用。
type Filter struct {
	banned *automaton
	allow  *automaton
	stats  Stats
}

// Parse 解析词表文本：一行一个词，接受 CRLF 和 LF 混用、开头的 BOM；空行忽略，
// # 开头是注释，! 开头是白名单（命中的屏蔽词整个落在白名单词内时不替换）。
func Parse(content string) *Filter {
	f := &Filter{banned: newAutomaton(), allow: newAutomaton()}
	content = strings.TrimPrefix(content, "\xEF\xBB\xBF")
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		target, isAllow := f.banned, false
		if strings.HasPrefix(line, "!") {
			target, isAllow = f.allow, true
			line = line[1:]
		}
		word, _ := normalize([]rune(line))
		if len(word) < minWordChars {
			f.stats.Skipped++
			continue
		}
		if target.add(word) {
			if isAllow {
				f.stats.Allow++
			} else {
				f.stats.Words++
			}
		}
	}
	f.banned.build()
	f.allow.build()
	return f
}

// Stats 返回词表的统计。
func (f *Filter) Stats() Stats { return f.stats }

// Mask 把 text 里命中屏蔽词的部分换成 *，字符数不变；没有命中时原样返回。
func (f *Filter) Mask(text string) string {
	runes := []rune(text)
	spans := f.spans(runes)
	if len(spans) == 0 {
		return text
	}
	for _, sp := range spans {
		for i := sp[0]; i <= sp[1]; i++ {
			runes[i] = '*'
		}
	}
	return string(runes)
}

// Contains 判断 text 是否含屏蔽词（名字类用：命中就拒绝）。
func (f *Filter) Contains(text string) bool {
	return len(f.spans([]rune(text))) > 0
}

// spans 返回命中的屏蔽词在原文（按字符序号）里的区间，闭区间 [start, end]，可能重叠。
func (f *Filter) spans(text []rune) [][2]int {
	nrm, pos := normalize(text)
	if len(nrm) < minWordChars {
		return nil
	}
	var allow [][2]int
	f.allow.find(nrm, func(s, e int) { allow = append(allow, [2]int{s, e}) })
	var out [][2]int
	f.banned.find(nrm, func(s, e int) {
		if !gapsOK(pos, s, e) || !asciiBoundaryOK(nrm, pos, s, e) || insideAny(allow, s, e) {
			return
		}
		out = append(out, [2]int{pos[s], pos[e]})
	})
	return out
}

// normalize 返回归一化后的字符，以及每个字符来自原文的第几个字符。
func normalize(text []rune) (out []rune, pos []int) {
	out = make([]rune, 0, len(text))
	pos = make([]int, 0, len(text))
	for i, r := range text {
		for _, c := range norm.NFKC.String(string(r)) {
			if dropped(c) {
				continue
			}
			out = append(out, unicode.ToLower(c))
			pos = append(pos, i)
		}
	}
	return out, pos
}

// dropped 判断归一化时要丢掉的字符：空白、控制、格式（零宽）、标点、符号、组合记号和私有区。
func dropped(c rune) bool {
	return unicode.IsSpace(c) || unicode.IsControl(c) || unicode.IsPunct(c) || unicode.IsSymbol(c) ||
		unicode.Is(unicode.Cf, c) || unicode.Is(unicode.Mn, c) || unicode.Is(unicode.Me, c) ||
		unicode.Is(unicode.Co, c) || c == utf8.RuneError
}

// gapsOK 检查命中区间 [s, e] 里相邻两个字符在原文里隔着的被丢字符不超过 maxGap 个。
func gapsOK(pos []int, s, e int) bool {
	for k := s; k < e; k++ {
		if pos[k+1]-pos[k]-1 > maxGap {
			return false
		}
	}
	return true
}

// asciiBoundaryOK：整个词都是 ASCII 字母数字时（如 sb、fuck），命中处两侧在原文里紧挨着的
// 字符不能还是 ASCII 字母数字，否则是更长单词的一部分（class 里的 ass）。
func asciiBoundaryOK(nrm []rune, pos []int, s, e int) bool {
	for k := s; k <= e; k++ {
		if !asciiAlnum(nrm[k]) {
			return true
		}
	}
	if s > 0 && pos[s]-pos[s-1] == 1 && asciiAlnum(nrm[s-1]) {
		return false
	}
	if e+1 < len(nrm) && pos[e+1]-pos[e] == 1 && asciiAlnum(nrm[e+1]) {
		return false
	}
	return true
}

func asciiAlnum(c rune) bool {
	return c < 128 && (c >= 'a' && c <= 'z' || c >= '0' && c <= '9')
}

func insideAny(spans [][2]int, s, e int) bool {
	for _, sp := range spans {
		if sp[0] <= s && e <= sp[1] {
			return true
		}
	}
	return false
}
