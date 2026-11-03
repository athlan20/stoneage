package agentmcp

// 游戏工具（STAGE-300 第 3 节）：connect、status、inventory、pets、warp、battle、battle_command。
// 每个工具就是一条协议请求的薄包装，参数和协议章节一一对应。

import (
	"context"
	"encoding/json"
)

// allTools 返回全部工具，顺序就是 tools/list 的顺序。
func allTools() []tool {
	return append(gameTools(), miscTools()...)
}

// gameTools 返回游戏工具。
func gameTools() []tool {
	return []tool{
		{
			name: "connect",
			desc: "连接游戏服务端并登录（自动完成 hello 和 agent_login，令牌来自启动参数），然后取角色列表并进入世界（docs/protocol.md 5.5、5.8、9.2）。" +
				"断线后再调用本工具会重新连接并重新进入世界。返回进入世界的结果：角色、位置、status、control（谁在操控）、是否在战斗中（battle 字段）。" +
				"已经在世界中时，返回 already_in_world 和当前 status。",
			schema: schema(map[string]any{
				"slot": prop("integer", "角色栏位（0 起）。不填则用账号下第一个角色。"),
			}),
			run: runConnect,
		},
		{
			name:   "status",
			desc:   "查看当前角色状态（docs/protocol.md 5.12）：数值、位置、战斗状态等。换步骤或重新连接后先调它确认状态。",
			schema: schema(nil),
			run:    simpleRun("status"),
		},
		{
			name:   "inventory",
			desc:   "查看背包：物品和装备，编号是 slot（docs/protocol.md 5.17）。",
			schema: schema(nil),
			run:    simpleRun("inventory"),
		},
		{
			name:   "pets",
			desc:   "查看宠物：出战、待命、寄放等状态和编号（docs/protocol.md 5.19）。",
			schema: schema(nil),
			run:    simpleRun("pets"),
		},
		{
			name: "warp",
			desc: "传送到一个地点（操作类，世界中；docs/protocol.md 5.16）。place 和 floor 二选一：place 是地点编号，floor 是楼层编号。" +
				"第二级操作：做完用 say_to_player 汇报。",
			schema: schema(map[string]any{
				"place": prop("integer", "地点编号（来自 places 或百科）。"),
				"floor": prop("integer", "楼层编号。"),
			}),
			run: passRun("warp"),
		},
		{
			name:   "battle",
			desc:   "查看当前战斗（docs/protocol.md 10.11）：回合 turn、阶段 phase（command 等出招，replay 在播放）、needs（这回合要你下的指令）、units（场上单位）。不在战斗中时 in_battle 为 false。",
			schema: schema(nil),
			run:    simpleRun("battle"),
		},
		{
			name: "battle_command",
			desc: "在战斗中出招（操作类，docs/protocol.md 10.5）。turn、actor、action 必填；actor 取 player 或 pet，action 常用 attack（target 填单位号）、magic（magic 为 0–4）、item（slot 为 5–19）、guard。" +
				"出招的分级以 stoneage://docs/agent-guide 第 6 节为准；属于第三级的，必须先用 say_to_player 问玩家，等回复再出。" +
				"观察方会收到 CONTROLLED_BY_PLAYER，此时停手。",
			schema: schema(map[string]any{
				"turn":   prop("integer", "当前回合号（battle_turn 推送或 battle 查询给出）。"),
				"actor":  prop("string", "player 或 pet；分身队里还可以是 clone:N、clone_pet:N。"),
				"action": prop("string", "attack、magic、item、guard、capture、escape 等，见 docs/protocol.md 10.5。"),
				"target": prop("integer", "攻击或施法的目标单位号。"),
				"magic":  prop("integer", "精灵编号（0–4），action 为 magic 时用。"),
				"slot":   prop("integer", "背包槽位（5–19），action 为 item 时用。"),
			}, "turn", "actor", "action"),
			run: passRun("battle_command"),
		},
	}
}

// simpleRun 生成一个没有参数的查看类工具：直接发出 typ 请求。
func simpleRun(typ string) func(context.Context, *Session, json.RawMessage) (any, *toolErr) {
	return func(ctx context.Context, s *Session, _ json.RawMessage) (any, *toolErr) {
		return s.do(ctx, typ, map[string]any{})
	}
}

// passRun 生成一个参数原样转给服务端的工具。参数的合法性由服务端判断。
func passRun(typ string) func(context.Context, *Session, json.RawMessage) (any, *toolErr) {
	return func(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
		var data map[string]any
		if e := decodeArgs(raw, &data); e != nil {
			return nil, e
		}
		if data == nil {
			data = map[string]any{}
		}
		return s.do(ctx, typ, data)
	}
}

// runConnect 实现 connect：连接登录，取角色，进入世界。
func runConnect(ctx context.Context, s *Session, raw json.RawMessage) (any, *toolErr) {
	var a struct {
		Slot *int `json:"slot"`
	}
	if e := decodeArgs(raw, &a); e != nil {
		return nil, e
	}
	if _, e := s.ensure(ctx); e != nil {
		return nil, e
	}
	if s.isInWorld() {
		st, e := s.do(ctx, "status", map[string]any{})
		if e != nil {
			return nil, e
		}
		return map[string]any{"already_in_world": true, "status": st}, nil
	}
	chars, e := s.do(ctx, "characters", map[string]any{})
	if e != nil {
		return nil, e
	}
	var c struct {
		Characters []struct {
			Slot int `json:"slot"`
		} `json:"characters"`
	}
	_ = json.Unmarshal(chars, &c)
	slot := 0
	switch {
	case a.Slot != nil:
		slot = *a.Slot
	case len(c.Characters) > 0:
		slot = c.Characters[0].Slot
	default:
		return nil, &toolErr{Code: "NO_CHARACTER", Message: "账号下还没有角色。agent 不能建角，请玩家先在游戏里创建角色。"}
	}
	return s.do(ctx, "enter_world", map[string]any{"slot": slot})
}
