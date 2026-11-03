// Package webpages 把 agent 要读的文档和石器百科挂到游戏服务端的 WebSocket 端口上（STAGE-118）：
//
//	/protocol.md       协议（docs/protocol.md）
//	/agent.md          agent 使用手册（docs/agent-guide.md）
//	/agent-connect.py  agent 参考连接脚本（docs/agent-connect.py）
//	/changes.md        agent 变更记录（docs/agent-changes.md，STAGE-188）
//	/leveling.md       agent 练级路线（docs/leveling.md，STAGE-253）
//	/install.md        客户端安装说明（docs/install.md，STAGE-301；docs/play.md 是面向玩家的完整版）
//	/mcp.md            stoneage-mcp 接入说明（docs/mcp.md，STAGE-300）
//	/system-menu.md、/npc-menu-v2.md、/protocol/*.md  协议里链接的子文档（STAGE-180）
//	/wiki/             石器百科，/wiki/q 是给 agent 用的纯文本查询
//
// 文件编进程序，和服务端版本一起发布。
package webpages

import (
	"io/fs"
	"net/http"

	"github.com/athlan20/stoneage/docs"
	"github.com/athlan20/stoneage/internal/gamewiki"
	"github.com/athlan20/stoneage/server/gateway"
)

// 发布路径和嵌入文件的对应关系。
var textFiles = []struct{ path, file, mime string }{
	{"/protocol.md", "protocol.md", "text/markdown; charset=utf-8"},
	{"/agent.md", "agent-guide.md", "text/markdown; charset=utf-8"},
	{"/agent-connect.py", "agent-connect.py", "text/x-python; charset=utf-8"},
	{"/changes.md", "agent-changes.md", "text/markdown; charset=utf-8"},
	{"/leveling.md", "leveling.md", "text/markdown; charset=utf-8"},
	{"/install.md", "install.md", "text/markdown; charset=utf-8"},
	{"/mcp.md", "mcp.md", "text/markdown; charset=utf-8"},
	// protocol.md 与 agent-guide.md 里用相对路径链接的子文档，发布路径与链接写法一致。
	{"/system-menu.md", "system-menu.md", "text/markdown; charset=utf-8"},
	{"/npc-menu-v2.md", "npc-menu-v2.md", "text/markdown; charset=utf-8"},
	{"/protocol/exits.md", "protocol/exits.md", "text/markdown; charset=utf-8"},
	{"/protocol/encounter-areas.md", "protocol/encounter-areas.md", "text/markdown; charset=utf-8"},
	{"/protocol/cases/quest_npcs.md", "protocol/cases/quest_npcs.md", "text/markdown; charset=utf-8"},
}

// Install 把上面的文档和百科登记到 srv。必须在 srv.ServeWS 之前调用。wikiCDN 是百科图片的来源。
func Install(srv *gateway.Server, wikiCDN string) {
	for _, f := range textFiles {
		srv.HandleHTTP(f.path, textFile(f.file, f.mime))
	}
	wiki := gamewiki.NewHandler(wikiCDN)
	srv.HandleHTTP("/wiki", wiki)
	srv.HandleHTTP("/wiki/", wiki)
}

// textFile 返回只读的文本文件处理函数。文件在编译时已内嵌，读取不会失败。
func textFile(name, mime string) http.Handler {
	body, err := fs.ReadFile(docs.FS, name)
	if err != nil {
		panic("webpages: 内嵌文件 " + name + " 不存在: " + err.Error())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "只支持读取", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	})
}
