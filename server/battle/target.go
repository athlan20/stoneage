// 精灵与宠物技能的目标校验（STAGE-78，docs/protocol.md 10.5）：目标类型（gamedata.TargetType）
// 决定要不要带 target，需要的时候只能选合法的一方、合法的状态（活着或倒下）。道具的校验在 submit.go
// （它还要分自己、他人、全己方）。

package battle

import (
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// checkTarget 校验 target 是否符合目标类型 tt（docs/protocol.md 10.5）。需要选单位的类型必须带 target，
// 范围不对回复 BAD_REQUEST；己方一个缺省时落在自己身上（同道具）；单位不合法（不存在、状态不对、不在能选的一方）
// 回复 INVALID_TARGET。其余类型不看 target。
func (b *Battle) checkTarget(tt gamedata.TargetType, target int) *protocol.Error {
	if !tt.NeedsTarget() || (tt == gamedata.TargetAlly && target == -1) {
		return nil
	}
	if target < 0 || target > 19 {
		return protocol.Errorf(protocol.CodeBadRequest, "target 要在 0–19 之间")
	}
	if u := b.Unit(target); u != nil && b.validTarget(tt, u) {
		return nil
	}
	switch tt {
	case gamedata.TargetAny: // 属性反转（STAGE-200）：己方或敌方任一个活着的单位
		return protocol.Errorf(protocol.CodeInvalidTarget, "要选一个活着的单位")
	case gamedata.TargetAlly:
		return protocol.Errorf(protocol.CodeInvalidTarget, "要选己方一个活着的单位")
	case gamedata.TargetAllyDown:
		return protocol.Errorf(protocol.CodeInvalidTarget, "要选己方一个倒下的单位")
	}
	return protocol.Errorf(protocol.CodeInvalidTarget, "要选对方一个活着的单位")
}

// reviveItemTarget 是复活类道具的目标校验（STAGE-262）：必须选己方一个倒下的单位（人或宠物）。
// 缺省、活着的单位分别给 INVALID_TARGET，活着的单位说明“不需要复活”。
func (b *Battle) reviveItemTarget(target int) *protocol.Error {
	if target == -1 {
		return protocol.Errorf(protocol.CodeInvalidTarget, "要选己方一个倒下的单位")
	}
	if u := b.Unit(target); u != nil && u.Alive() && u.Side() == b.chkSide {
		return protocol.Errorf(protocol.CodeInvalidTarget, "%s 是活着的，不需要复活", u.Name)
	}
	return b.checkTarget(gamedata.TargetAllyDown, target)
}

// validTarget 报告单位 u 能不能被这个类型选中：ally 是己方活着的，ally_down 是己方倒下的（怪物不能复活），
// enemy 是敌方活着的，any 是任一方活着的（STAGE-200）。
func (b *Battle) validTarget(tt gamedata.TargetType, u *Unit) bool {
	mine := u.Side() == b.chkSide
	switch tt {
	case gamedata.TargetAny:
		return u.Alive()
	case gamedata.TargetAlly:
		return mine && u.Alive()
	case gamedata.TargetAllyDown:
		return mine && !u.Alive() && u.Kind != KindMonster
	case gamedata.TargetEnemy:
		return !mine && u.Alive()
	}
	return false
}
