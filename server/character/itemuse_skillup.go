// 英雄的祝福（STAGE-334，物品表使用函数 ITEM_useSkup）：战斗外给自己使用，未分配的能力点 +1，扣 1 个物品。
// 出处是原版服务端 item_event.c 的 ITEM_useSkup，原版没有别的限制。只能对自己用（target 非 0 由 useItem 回 INVALID_TARGET），
// 战斗中由 AbleUse=2 挡在 battle 提交之前，这里不用管。

package character

import (
	"context"
)

// useSkillUp 给自己用英雄的祝福：先扣物品（扣失败则点数不变），再把未分配点数 +1、写库，并推 status_changed。
// 调用时要持有角色的锁（在 Live.Operate 里）。
func (s *Service) useSkillUp(ctx context.Context, p *Presence, r *record, slot int) (map[string]any, error) {
	if err := s.consumeItem(ctx, p, r, slot); err != nil {
		return nil, err
	}
	r.SkillPoints++
	s.saveSkillPoints(p.charID, r.SkillPoints) // 未分配点数写库，同升级奖励的收尾（STAGE-151）
	p.pushAll("status_changed", map[string]any{"status": r.status()})
	return map[string]any{
		"target": "player", "name": r.Name,
		"skill_points": r.SkillPoints, "skill_points_delta": 1,
	}, nil
}
