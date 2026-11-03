package agentmcp

// MCP 资源（STAGE-300 第 4 节）：四份 agent 要读的文档。
// WebSocket 连接时从服务端的 /agent.md 等路径取（和服务端版本一致）；TCP 连接时读二进制里内嵌的 docs/ 副本。

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/athlan20/stoneage/docs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	docMIME     = "text/markdown"
	docTimeout  = 10 * time.Second
	docMaxBytes = 4 << 20
)

// docResource 是一份文档资源：URI、说明、服务端路径（WebSocket 用）和内嵌文件名（TCP 用）。
type docResource struct {
	uri, name, desc string
	webPath         string
	embedded        string
}

var docResources = []docResource{
	{"stoneage://docs/agent-guide", "agent-guide", "agent 使用手册：上线步骤、推送与断线的处理、控制权礼仪、先问玩家的分级（docs/agent-guide.md）。先读它。", "/agent.md", "agent-guide.md"},
	{"stoneage://docs/protocol", "protocol", "协议：消息结构、各请求和推送的字段（docs/protocol.md）。", "/protocol.md", "protocol.md"},
	{"stoneage://docs/changes", "changes", "agent 变更记录：文档修订号和每次改了什么（docs/agent-changes.md）。docs_updated 提示时读它。", "/changes.md", "agent-changes.md"},
	{"stoneage://docs/leveling", "leveling", "练级路线（docs/leveling.md）。", "/leveling.md", "leveling.md"},
}

func (s *Server) addResources() {
	for _, d := range docResources {
		s.mcp.AddResource(&mcp.Resource{URI: d.uri, Name: d.name, Description: d.desc, MIMEType: docMIME},
			func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				text, err := s.readDoc(ctx, d)
				if err != nil {
					return nil, err
				}
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: d.uri, MIMEType: docMIME, Text: text}}}, nil
			})
	}
}

// readDoc 读一份文档的正文。
func (s *Server) readDoc(ctx context.Context, d docResource) (string, error) {
	if isWebSocket(s.cfg.Server) {
		base := wikiBase(s.cfg)
		text, err := httpText(ctx, base+d.webPath)
		if err != nil {
			return "", fmt.Errorf("从服务端读取 %s 失败：%w", d.webPath, err)
		}
		return text, nil
	}
	raw, err := docs.FS.ReadFile(d.embedded)
	if err != nil {
		return "", fmt.Errorf("内嵌文档 %s 不存在：%w", d.embedded, err)
	}
	return string(raw), nil
}

// httpText 用 GET 取一段文本，超时 10 秒，正文最多 4 MiB。
func httpText(ctx context.Context, target string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, docTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, docMaxBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return string(body), nil
}
