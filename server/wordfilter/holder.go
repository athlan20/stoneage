package wordfilter

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/gamedata"
)

// maxFileBytes 是词表文件大小的上限，防止误把别的大文件当词表。
const maxFileBytes = 8 << 20

var current atomic.Pointer[Filter]

// Current 返回此刻生效的词表：没有 Set 过时是编译进程序的内置词表（gamedata/chat_filter.txt）。
func Current() *Filter {
	if f := current.Load(); f != nil {
		return f
	}
	current.CompareAndSwap(nil, Parse(strings.Join(gamedata.ChatFilterWords(), "\n")))
	return current.Load()
}

// Set 换上新词表，之后的聊天和名字检查立即使用它。
func Set(f *Filter) { current.Store(f) }

// LoadFile 读取并解析词表文件（UTF-8，一行一词）。文件读不到、太大、不是 UTF-8、没有任何有效词都返回错误：
// 启动时让服务端起不来，重载时让旧词表继续用，都比悄悄用一份空词表好。
func LoadFile(path string) (*Filter, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("屏蔽词文件 %s 有 %d 字节，超过 %d 的上限", path, info.Size(), maxFileBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("屏蔽词文件 %s 不是 UTF-8 编码", path)
	}
	f := Parse(string(raw))
	if f.stats.Words == 0 {
		return nil, fmt.Errorf("屏蔽词文件 %s 里没有有效的词", path)
	}
	return f, nil
}

// ReloadFile 读取 path 并换上；失败时不动现有词表。path 为空表示不用外部文件，保持现状。
func ReloadFile(path string) (Stats, error) {
	if path == "" {
		return Current().Stats(), nil
	}
	f, err := LoadFile(path)
	if err != nil {
		return Stats{}, err
	}
	Set(f)
	return f.Stats(), nil
}
