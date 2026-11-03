package agentmcp

// 工具结果的统一外形（STAGE-300 第 3 节）：一段 JSON 文本，里面是 ok / data 或 ok / error，
// 还可能带 events、dropped_events、docs_updated。

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolErr 是工具的失败信息。Code 是服务端的错误码，或 MCP 服务自己定义的码（如 CONNECT_FAILED）。
type toolErr struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// result 把数据或错误包成工具结果，并取出积压的推送和文档提醒。
func (s *Session) result(data any, e *toolErr) *mcp.CallToolResult {
	out := map[string]any{}
	if e != nil {
		out["ok"] = false
		out["error"] = e
	} else {
		out["ok"] = true
		out["data"] = orEmptyObject(data)
	}
	if events, dropped := s.events.drain(); len(events) > 0 || dropped > 0 {
		if len(events) > 0 {
			raws := make([]json.RawMessage, 0, len(events))
			for _, ev := range events {
				raws = append(raws, ev.Raw)
			}
			out["events"] = raws
		}
		if dropped > 0 {
			out["dropped_events"] = dropped
		}
	}
	if notice := s.takeDocsNotice(); notice != "" {
		out["docs_updated"] = notice
	}
	b, err := json.Marshal(out)
	if err != nil {
		b = []byte(`{"ok":false,"error":{"code":"INTERNAL","message":"工具结果编码失败"}}`)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s.log.scrub(string(b))}},
		IsError: e != nil,
	}
}

// orEmptyObject 让没有数据的成功结果也是 {}，而不是 null。
func orEmptyObject(data any) any {
	if raw, ok := data.(json.RawMessage); ok {
		if len(raw) == 0 {
			return map[string]any{}
		}
		return raw
	}
	if data == nil {
		return map[string]any{}
	}
	return data
}
