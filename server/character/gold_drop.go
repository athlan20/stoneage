package character

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-328：丢弃石币（gold_drop，docs/protocol.md 5.76）。agent 把石币刷到上限附近时，卖东西会因为 GOLD_FULL 失效；
// 这里允许主动丢掉一部分石币，把持有量压下来。丢弃就是销毁，不可恢复。

// goldDropRequest 的两个字段保持原始 JSON：类型不对也要等到控制权、战斗中检查之后才回 BAD_REQUEST，顺序与 item_drop 一致。
type goldDropRequest struct {
	Amount  json.RawMessage `json:"amount"`
	Confirm json.RawMessage `json:"confirm"`
}

// handleGoldDrop 是 gold_drop：丢弃 amount 石币，需要 confirm 为 "yes"。
// 控制权由守卫（guard）和 Live.Operate 检查；这里按 战斗中 → 死亡 → 参数 → 数量超过持有 的顺序检查。
func (s *Service) handleGoldDrop(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in goldDropRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var opErr error
	if err := l.Operate(func(r *record) {
		// inBattle 返回 *protocol.Error，先判断再赋值，避免 nil 变成非 nil 的 error 接口（见 itemuse.go）。
		if e := inBattle(l); e != nil {
			opErr = e
			return
		}
		if r.HP <= 0 {
			opErr = protocol.Errorf(protocol.CodeCharacterDead, "角色已经死亡，不能丢弃石币")
			return
		}
		out, opErr = s.dropGold(ctx, l.p, r, in)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return out, nil
}

// dropGold 校验并丢弃石币。调用时要持有角色的锁（在 Live.Operate 里）。
// 先写库，成功后才改内存，并推送 status_changed（石币在 status 里）。
func (s *Service) dropGold(ctx context.Context, p *Presence, r *record, in goldDropRequest) (map[string]any, error) {
	amount, err := parseGoldAmount(in.Amount)
	if err != nil {
		return nil, err
	}
	if !goldConfirmed(in.Confirm) {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "丢弃石币要确认：confirm 必须是 \"yes\"")
	}
	if amount > r.Gold {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "石币不够丢：持有 %d，要丢 %d", r.Gold, amount)
	}
	gold := r.Gold - amount
	if _, err := s.db.Exec(ctx, `UPDATE characters SET gold=$1 WHERE id=$2`, gold, r.ID); err != nil {
		return nil, protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Gold = gold
	p.pushAll("status_changed", map[string]any{"status": r.status()})
	return map[string]any{"gold": gold}, nil
}

// parseGoldAmount 解析 amount：必须是 JSON 整数且不小于 1。缺少、null、小数、科学计数法、字符串都回复 BAD_REQUEST。
func parseGoldAmount(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, protocol.Errorf(protocol.CodeBadRequest, "需要丢弃的数量 amount（正整数）")
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || n < 1 {
		return 0, protocol.Errorf(protocol.CodeBadRequest, "数量要是正整数")
	}
	return n, nil
}

// goldConfirmed 判断 confirm 是不是字符串 "yes"。
func goldConfirmed(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && s == "yes"
}
