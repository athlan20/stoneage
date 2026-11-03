// Package docs 内嵌随服务端一起发布的文档（STAGE-118）：协议、agent 使用手册和 agent 参考脚本，
// 由 server/webpages 挂到 WebSocket 端口上，agent 直接从服务端下载，不用另找地方。
package docs

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
)

//go:embed protocol.md agent-guide.md agent-changes.md agent-connect.py leveling.md install.md mcp.md system-menu.md npc-menu-v2.md protocol/exits.md protocol/encounter-areas.md protocol/cases/quest_npcs.md
var FS embed.FS

// Rev 是随服务端发布的协议和手册的修订号（STAGE-188）：protocol.md 与 agent-guide.md 内容的 SHA-256 前 12 位十六进制。
// 两份文档有改动才会变，hello 响应里的 docs_rev 就是它，agent 用来判断要不要重读。
func Rev() string {
	return RevOf(readOrPanic("protocol.md"), readOrPanic("agent-guide.md"))
}

// RevOf 按两份文档的内容算修订号。中间的分隔符让两段内容的边界有区别，避免移动文字后得到同一个值。
func RevOf(protocol, guide []byte) string {
	h := sha256.New()
	h.Write(protocol)
	h.Write([]byte{0})
	h.Write(guide)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func readOrPanic(name string) []byte {
	raw, err := FS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("docs: 内嵌文件 %s 不存在: %v", name, err))
	}
	return raw
}
