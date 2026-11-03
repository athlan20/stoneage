package gamedata

import (
	_ "embed"
	"strings"
)

// 公开聊天屏蔽词（STAGE-278，docs/chat.md 1.3）：chat_filter.txt 编译进二进制，启动时解析一次。
//
//go:embed chat_filter.txt
var chatFilterTXT string

// ChatFilterWords 返回屏蔽词表：去掉空行和 # 注释、去掉首尾空白，保持文件里的顺序。
func ChatFilterWords() []string {
	var out []string
	for _, line := range strings.Split(chatFilterTXT, "\n") {
		w := strings.TrimSpace(line)
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		out = append(out, w)
	}
	return out
}
