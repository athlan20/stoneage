package agentmcp

// 通用请求工具 request（STAGE-300 第 3 节）：发任意请求，等它的响应。

import (
	"context"
	"encoding/json"
	"fmt"
)

// agentBlocked 是 MCP 服务自己管理的请求，不放给 request 工具。
var agentBlocked = map[string]string{
	"hello":       "握手由 stoneage-mcp 自动完成",
	"agent_login": "登录由 connect 自动完成",
	"ping":        "心跳由 stoneage-mcp 自动发送",
	"battle_ack":  "战斗回放确认由 stoneage-mcp 自动发送",
}

// requestDesc 是 request 工具的说明。STAGE-303：不再列具体请求名，分级以服务端手册为准（stoneage://docs/agent-guide 第 6 节），
// 这样服务端新增请求或改分级时，旧版 MCP 不用跟着改。
const requestDesc = "发送任意游戏请求并等待响应，type 是请求类型名，data 是参数（协议见 docs/protocol.md）。" +
	"只有 hello、agent_login、ping、battle_ack 由 MCP 服务自己管理，这里发会被拒绝；其他类型一律转给服务端，" +
	"由服务端判断权限和状态，错误原样返回。" +
	"【先问玩家】做任何改变角色、花钱、丢东西、影响别的真人的操作前，先读资源 stoneage://docs/agent-guide 第 6 节的分级表；" +
	"第三级操作必须先用 say_to_player 问玩家，等玩家回复再做；拿不准就先问。分级以服务端手册为准（连接服务端时资源从服务端现取）。" +
	"观察方收到 CONTROLLED_BY_PLAYER 时停手，不要重试。"

func requestTool() tool {
	return tool{
		name: "request",
		desc: requestDesc,
		schema: schema(map[string]any{
			"type": prop("string", "请求类型名，如 turn、talk、item_use，见 docs/protocol.md。"),
			"data": map[string]any{"type": "object", "description": "请求参数（JSON 对象），没有参数时省略。"},
		}, "type"),
		run: runRequest,
	}
}

// runRequest 只拦 MCP 自己管理的请求；其他类型原样转给服务端（STAGE-303：不再用 protocol.Lookup 查表放行）。
func runRequest(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
	var a struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if e := decodeArgs(raw, &a); e != nil {
		return nil, e
	}
	if a.Type == "" {
		return nil, &toolErr{Code: "BAD_ARGUMENT", Message: "缺少 type"}
	}
	if why, ok := agentBlocked[a.Type]; ok {
		return nil, &toolErr{Code: "NOT_ALLOWED", Message: fmt.Sprintf("%s：%s", a.Type, why)}
	}
	if a.Data == nil {
		a.Data = map[string]any{}
	}
	return s.do(ctx, a.Type, a.Data)
}
