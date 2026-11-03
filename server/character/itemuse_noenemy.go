// 龙王的守护（STAGE-340，物品表使用函数 ITEM_useNoenemy，18544）：战斗外给自己使用，一段时间内不遇敌，扣 1 个物品。
// 出处是原版服务端 item_event.c 的 ITEM_useNoenemy（setNoenemy）与 net.c 的计数：每次使用把计数设为 6，
// 计数在全服固定的 60 秒格点上每次减 1，减到 0 时推“守护消失了。”。原版计数挂在连接上，掉线即清零；
// 这里挂在角色的 Presence 上（最后一条连接离开即随 Presence 消失），到期判断用时间戳，不另起计时器。
// 与原版的差别：持续时间取 5 分钟（原版实际落在 300–360 秒之间，取决于使用时间在格点上的位置），
// 到期不推“守护消失了。”（需求没有这一条，免得加协议）。

package character

import (
	"context"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// noEnemyDuration 是龙王的守护的持续时间（见文件头的说明）。
const noEnemyDuration = 5 * time.Minute

// noEnemyText 是使用成功时的提示（原版 ITEM_useNoenemy 的 CHAR_talkToCli 文字）。
const noEnemyText = "你感受到周边的杀气消失了。"

// noEnemyActive 报告龙王的守护是否还在生效（now 之前都算）。调用方持有 p.mu。
func (p *Presence) noEnemyActive(now time.Time) bool {
	return now.Before(p.noEnemyUntil)
}

// useNoEnemy 使用龙王的守护：先扣物品（扣失败则守护不生效），再把守护期限设为从现在起 noEnemyDuration，
// 重复使用是重新计时（原版 setNoenemy 是把计数重设为 6，不叠加），推 notice 提示。
// 调用时要持有角色的锁（在 Live.Operate 里）。
func (s *Service) useNoEnemy(ctx context.Context, p *Presence, r *record, slot int) (map[string]any, error) {
	if err := s.consumeItem(ctx, p, r, slot); err != nil {
		return nil, err
	}
	p.noEnemyUntil = s.now().Add(noEnemyDuration)
	p.pushAll("notice", map[string]string{"level": "info", "text": noEnemyText})
	return map[string]any{"target": "player", "name": r.Name}, nil
}

// checkNoEnemyEncounter 在主动开始一场野外遇敌（遇敌开关、按区域遇敌、1 级宠物目录）前调用：
// 守护生效中回复 NO_ENEMY_HERE，不开战、不动位置。调用方持有 p.mu。
func (s *Service) checkNoEnemyEncounter(p *Presence) *protocol.Error {
	if p.noEnemyActive(s.now()) {
		return protocol.Errorf(protocol.CodeNoEnemyHere, "龙王的守护还在生效，周边没有敌人")
	}
	return nil
}
