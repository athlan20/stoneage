package agentmcp

// 其他工具（STAGE-300 第 3 节）：request、wait_events、say_to_player、report_plan、wiki_search。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	defaultWaitSeconds = 30
	maxWaitSeconds     = 120
)

// miscTools 返回 request 之外的工具。
func miscTools() []tool {
	return []tool{
		{
			name: "wait_events",
			desc: "等推送（不发请求）。等到任何一条匹配 types 的推送，或超时，就返回期间收到的全部推送（在 events 里）。" +
				"types 不填则任何推送都算匹配；disconnect 和 mcp_closed（连接断开）总是算匹配。" +
				"打战斗时用它等 battle_turn（轮到你出招）和 battle_end（战斗结束）。" +
				"要等的推送如果已经出现在上一次工具结果的 events 里（例如最后一回合的 battle_end 常随出招结果一起到），" +
				"会立刻返回 matched: true 和 already_delivered（类型名列表），不再等待。",
			schema: schema(map[string]any{
				"types": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "要等的推送类型名，如 battle_turn、battle_end、control_changed。",
				},
				"timeout_seconds": prop("integer", fmt.Sprintf("最长等待秒数，默认 %d，最大 %d。", defaultWaitSeconds, maxWaitSeconds)),
			}),
			run: runWaitEvents,
		},
		{
			name: "say_to_player",
			desc: "私聊玩家（docs/protocol.md 9.5 agent_say）。用来汇报进度、问第三级操作的确认。文本 1–200 个字符。" +
				"玩家不在线时返回 PEER_OFFLINE。",
			schema: schema(map[string]any{
				"text": prop("string", "要说的话，1–200 个字符。"),
			}, "text"),
			run: runSayToPlayer,
		},
		{
			name: "report_plan",
			desc: "上报计划（docs/protocol.md 9.7 agent_plan）。每次都是整份替换，换一步就发完整的计划。goal 1–100 字，steps 最多 20 项，state 取 todo、doing、done。",
			schema: schema(map[string]any{
				"goal": prop("string", "总目标，1–100 个字符。"),
				"steps": map[string]any{
					"type":        "array",
					"description": "步骤，最多 20 项。",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text":  prop("string", "步骤内容，1–60 个字符。"),
							"state": prop("string", "todo、doing 或 done。"),
						},
						"required": []string{"text", "state"},
					},
				},
			}, "goal"),
			run: runReportPlan,
		},
		{
			name: "wiki_search",
			desc: "查石器百科（服务端 /wiki/q 纯文本接口，docs/game-wiki.md）。q 搜关键词，kind 限定分类，limit 限制条数（默认 20，最多 50）；" +
				"key 查词条详情（如 map:1001），给了 key 就忽略 q。TCP 连接时需要启动参数 --wiki 指定百科地址。",
			schema: schema(map[string]any{
				"q":     prop("string", "关键词。"),
				"kind":  prop("string", "分类 id，如 quest、pet、equipment、item、skill、map、npc、enemy。"),
				"limit": prop("integer", "返回条数，默认 20，最多 50。"),
				"key":   prop("string", "词条编号，如 map:1001；查详情用。"),
			}),
			run: runWikiSearch,
		},
		requestTool(),
	}
}

func runWaitEvents(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
	var a struct {
		Types          []string `json:"types"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}
	if e := decodeArgs(raw, &a); e != nil {
		return nil, e
	}
	secs := a.TimeoutSeconds
	if secs <= 0 {
		secs = defaultWaitSeconds
	}
	if secs > maxWaitSeconds {
		secs = maxWaitSeconds
	}
	// STAGE-300：要等的推送已经随上一次工具结果交出去了（比如最后一回合的 battle_end 跟着出招的结果一起到），
	// 直接返回，并写明是哪几种，让 AI 去上一次结果的 events 里找。
	if hit := s.events.takeRecent(a.Types); len(hit) > 0 {
		return map[string]any{"matched": true, "already_delivered": hit}, nil
	}
	matched := s.events.waitFor(ctx, a.Types, time.Duration(secs)*time.Second)
	return map[string]any{"matched": matched}, nil
}

func runSayToPlayer(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
	var a struct {
		Text string `json:"text"`
	}
	if e := decodeArgs(raw, &a); e != nil {
		return nil, e
	}
	return s.do(ctx, "agent_say", map[string]any{"text": a.Text})
}

func runReportPlan(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
	var a struct {
		Goal  string          `json:"goal"`
		Steps json.RawMessage `json:"steps,omitempty"`
	}
	if e := decodeArgs(raw, &a); e != nil {
		return nil, e
	}
	data := map[string]any{"goal": a.Goal}
	if len(a.Steps) > 0 {
		data["steps"] = a.Steps
	}
	return s.do(ctx, "agent_plan", data)
}

func runWikiSearch(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
	var a struct {
		Q     string `json:"q"`
		Kind  string `json:"kind"`
		Limit int    `json:"limit"`
		Key   string `json:"key"`
	}
	if e := decodeArgs(raw, &a); e != nil {
		return nil, e
	}
	base := wikiBase(s.cfg)
	if base == "" {
		return nil, &toolErr{Code: "WIKI_UNAVAILABLE", Message: "TCP 连接时没有百科地址。启动时加 --wiki http://主机:端口 指定，或改用 ws:// 连接。"}
	}
	q := url.Values{}
	if a.Key != "" {
		q.Set("key", a.Key)
	} else {
		q.Set("q", a.Q)
		if a.Kind != "" {
			q.Set("kind", a.Kind)
		}
		if a.Limit > 0 {
			q.Set("limit", fmt.Sprint(a.Limit))
		}
	}
	text, e := fetchWiki(ctx, base+"/wiki/q?"+q.Encode())
	if e != nil {
		return nil, e
	}
	return map[string]any{"text": text}, nil
}

// wikiBase 返回百科的 http(s) 根地址：WebSocket 连接时用同一台主机，TCP 连接时用 --wiki。
func wikiBase(cfg Config) string {
	if isWebSocket(cfg.Server) {
		scheme := "http"
		if strings.HasPrefix(cfg.Server, "wss://") {
			scheme = "https"
		}
		if u, err := url.Parse(cfg.Server); err == nil && u.Host != "" {
			return scheme + "://" + u.Host
		}
		return ""
	}
	return strings.TrimRight(cfg.Wiki, "/")
}

// fetchWiki 取一次百科纯文本查询的结果。
func fetchWiki(ctx context.Context, target string) (string, *toolErr) {
	text, err := httpText(ctx, target)
	if err != nil {
		return "", &toolErr{Code: "WIKI_ERROR", Message: "百科请求失败：" + err.Error()}
	}
	return text, nil
}
