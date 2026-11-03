// Package agentmcp 是 stoneage-mcp 的主体（STAGE-300）：一个本地 stdio 的 MCP 服务，
// 用 agent 令牌连游戏服务端，把游戏操作变成 AI 助手可以直接调用的工具和资源。
// 它只是一个新的 agent 客户端，不改协议、不改服务端规则（docs/protocol.md、docs/agent-guide.md）。
package agentmcp

import (
	"context"
	"encoding/json"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config 是启动参数。Token 只放在内存里，不写日志、不进工具结果。
type Config struct {
	Server    string    // host:port 走 TCP，ws:// 或 wss:// 走 WebSocket
	Token     string    // agent 令牌（STONEAGE_AGENT_TOKEN 或 --token-file）
	Wiki      string    // 石器百科的 http(s) 地址，只给 TCP 连接的 wiki_search 用
	LogFile   io.Writer // 协议日志，可为 nil
	StatePath string    // docs_rev 记录文件，空则用默认位置
}

// Server 是一个 MCP 服务实例。
type Server struct {
	cfg  Config
	sess *Session
	mcp  *mcp.Server
}

// New 创建 MCP 服务，注册全部工具和资源。第一次调用工具时才会连接游戏服务端。
func New(cfg Config) *Server {
	if cfg.StatePath == "" {
		cfg.StatePath = defaultStatePath()
	}
	s := &Server{cfg: cfg, sess: newSession(cfg)}
	s.mcp = mcp.NewServer(&mcp.Implementation{Name: "stoneage-mcp", Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})
	for _, t := range allTools() {
		s.addTool(t)
	}
	s.addResources()
	return s
}

// Run 用标准输入输出提供 MCP 服务，直到输入关闭或 ctx 结束。
func (s *Server) Run(ctx context.Context) error {
	defer s.sess.close()
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// Close 断开游戏服务端的连接。
func (s *Server) Close() { s.sess.close() }

// MCP 返回底层的 MCP 服务，测试用它接内存传输。
func (s *Server) MCP() *mcp.Server { return s.mcp }

// tool 是一个 MCP 工具：名字、说明、入参的 JSON Schema，以及执行函数。
// 执行函数拿到原始入参，返回要放进 data 的值，或工具错误。
type tool struct {
	name   string
	desc   string
	schema map[string]any
	run    func(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr)
}

func (s *Server) addTool(t tool) {
	s.mcp.AddTool(&mcp.Tool{Name: t.name, Description: t.desc, InputSchema: t.schema},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			data, e := t.run(ctx, s.sess, req.Params.Arguments)
			return s.sess.result(data, e), nil
		})
}

// schema 生成 object 类型的入参 schema。
func schema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

// prop 生成一个字段的 schema。
func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// decodeArgs 解析工具入参；空入参当作 {}。
func decodeArgs(raw json.RawMessage, v any) *toolErr {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return &toolErr{Code: "BAD_ARGUMENT", Message: "参数格式不对：" + err.Error()}
	}
	return nil
}

const serverInstructions = `石器时代 2.5 的 agent 工具（stoneage-mcp）。上手步骤：
1. 先读资源 stoneage://docs/agent-guide，了解行为规则，尤其是“先问玩家”的分级（第 6 节）。
2. 调用 connect 上线（自动登录，并进入世界）。
3. 每换一个步骤先用 report_plan 上报计划；需要问玩家或汇报时用 say_to_player。
4. 打战斗：battle 查看当前回合，battle_command 出招，wait_events（types 填 battle_turn、battle_end）等推送。回放确认由服务端自动完成，你不用处理。
5. 玩家收回控制权后，操作类工具会返回 CONTROLLED_BY_PLAYER，此时必须停手，等玩家指示。
工具结果里的 events 是上次调用以来的推送；docs_updated 表示文档有更新，要先读 stoneage://docs/changes。`
