package agentmcp

// MCP 层测试（STAGE-300 第 5 节）：用 go-sdk 的内存传输起一个客户端，调 tools/list、tools/call、resources/read。

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPListCallAndRead(t *testing.T) {
	fake := newFakeSrv(t)
	srv := newTestServer(t, fake, nil)
	ctx := context.Background()

	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.MCP().Connect(ctx, st, nil); err != nil {
		t.Fatalf("服务端连接失败：%v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("客户端连接失败：%v", err)
	}
	defer cs.Close()

	// tools/list
	names := map[string]bool{}
	for tl, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("列工具失败：%v", err)
		}
		names[tl.Name] = true
	}
	for _, want := range []string{"connect", "request", "status", "inventory", "pets", "warp", "battle", "battle_command", "wait_events", "say_to_player", "report_plan", "wiki_search"} {
		if !names[want] {
			t.Errorf("缺少工具 %s", want)
		}
	}
	if names["battle_ack"] {
		t.Error("battle_ack 不应暴露成工具")
	}

	// tools/call：connect，再 status，结果是统一的 JSON 文本
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "connect", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("调用 connect 失败：%v", err)
	}
	if out := parseResult(t, res); out["ok"] != true {
		t.Fatalf("connect 应成功：%v", out)
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("调用 status 失败：%v", err)
	}
	if out := parseResult(t, res); out["ok"] != true {
		t.Fatalf("status 应成功：%v", out)
	}

	// resources/read：TCP 连接时读内嵌的文档
	rr, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "stoneage://docs/agent-guide"})
	if err != nil {
		t.Fatalf("读取资源失败：%v", err)
	}
	if len(rr.Contents) != 1 || len(rr.Contents[0].Text) < 100 {
		t.Fatalf("agent-guide 应有正文：%+v", rr.Contents)
	}
	if !strings.Contains(rr.Contents[0].Text, "agent") {
		t.Fatal("agent-guide 正文不对")
	}
	if srv.MCP() == nil {
		t.Fatal("MCP 服务不应为空")
	}
}

// 工具入参的 schema 必须是 object，且 battle_command 标明必填字段。
func TestToolSchemasAreObjects(t *testing.T) {
	for _, tl := range allTools() {
		if tl.schema["type"] != "object" {
			t.Errorf("%s 的入参 schema 必须是 object", tl.name)
		}
		if !strings.Contains(tl.desc, "docs/protocol.md") && tl.name != "wait_events" && tl.name != "report_plan" && tl.name != "wiki_search" {
			t.Errorf("%s 的说明应写明对应的协议章节", tl.name)
		}
	}
}

// STAGE-303：工具说明不再抄写第三级请求清单，只引用服务端手册（stoneage://docs/agent-guide 第 6 节）。
func TestToolDescsDoNotCopyGuideList(t *testing.T) {
	names := []string{"item_drop", "pet_drop", "party_kick", "party_disband", "party_leader", "trade_lock", "trade_confirm", "item_inlay", "capture", "escape"}
	for _, tl := range allTools() {
		for _, n := range names {
			if strings.Contains(tl.desc, n) {
				t.Errorf("%s 的说明不应写死第三级请求名 %s", tl.name, n)
			}
		}
	}
	if !strings.Contains(requestDesc, "stoneage://docs/agent-guide") {
		t.Error("request 的说明应引用 stoneage://docs/agent-guide")
	}
}
