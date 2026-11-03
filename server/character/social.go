package character

// STAGE-276：社交开关（隐身、拒绝邀请，docs/protocol.md 5.69，docs/social-players.md 第三节第 4 项）。
// STAGE-296：接收交易邀请（trade，docs/trade.md 3.4、5.3），与前两个开关一样存在 characters 表（迁移 0029、0030），改动立即写库；
// 隐身的附近玩家效果在 players_visible.go，组队邀请检查在 party_players_request.go，交易邀请检查在 trade_request.go。

import (
	"context"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// socialView 是协议 5.69 的 social 对象（enter_world、status、social_settings 的响应，以及 social_changed 的推送）。
func socialView(r *record) map[string]any {
	return map[string]any{"visible": !r.SocialHidden, "invite": !r.SocialNoInvite, "trade": !r.SocialNoTrade, "duel": r.SocialDuel}
}

// parseSocialArgs 解析 social_settings 的 data：只接受 visible、invite、trade 三个布尔字段，省略的为 nil（不改）。
// 其他字段，或值不是布尔（包括 null），回复 BAD_REQUEST，免得拼错的字段名静默失效。
func parseSocialArgs(req *protocol.Request) (visible, invite, trade, duel *bool, perr *protocol.Error) {
	var m map[string]any
	if perr := req.Decode(&m); perr != nil {
		return nil, nil, nil, nil, perr
	}
	for k, v := range m {
		b, ok := v.(bool)
		switch {
		case !ok, k != "visible" && k != "invite" && k != "trade" && k != "duel":
			return nil, nil, nil, nil, protocol.Errorf(protocol.CodeBadRequest, "social_settings 只接受布尔字段 visible、invite、trade、duel")
		case k == "visible":
			visible = &b
		case k == "invite":
			invite = &b
		case k == "duel":
			duel = &b
		default:
			trade = &b
		}
	}
	return visible, invite, trade, duel, nil
}

// handleSocialSettings 改隐身与拒绝邀请（操作类，世界中；控制权由守卫和 Operate 检查，协议 5.69）。
// 值真的变了才写库、推 social_changed（先于响应）；都省略时只回当前值。
func (s *Service) handleSocialSettings(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	visible, invite, trade, duel, perr := parseSocialArgs(req)
	if perr != nil {
		return nil, perr
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var dbErr error
	err = l.Operate(func(r *record) {
		hidden, noInvite, noTrade := r.SocialHidden, r.SocialNoInvite, r.SocialNoTrade
		if visible != nil {
			hidden = !*visible
		}
		if invite != nil {
			noInvite = !*invite
		}
		if trade != nil {
			noTrade = !*trade
		}
		duelWas, duelNow := r.SocialDuel, r.SocialDuel
		if duel != nil {
			duelNow = *duel
		}
		if hidden != r.SocialHidden || noInvite != r.SocialNoInvite || noTrade != r.SocialNoTrade || duelNow != duelWas {
			if dbErr = s.saveSocial(ctx, r.ID, !hidden, !noInvite, !noTrade); dbErr != nil {
				return
			}
			visibilityChanged := hidden != r.SocialHidden
			r.SocialHidden, r.SocialNoInvite, r.SocialNoTrade = hidden, noInvite, noTrade
			r.SocialDuel = duelNow
			if duelNow != duelWas {
				s.players.touch(l.p) // 同图的人列表里的 duel 变了，推 update（协议 5.66）
			}
			l.p.hidden.Store(hidden)
			if visibilityChanged {
				s.players.visibilityChanged(l.p) // 同图的其他角色收到 leave 或 enter（STAGE-276）
			}
			l.p.pushAll("social_changed", map[string]any{"social": socialView(r)})
		}
		out = map[string]any{"social": socialView(r)}
	})
	if err != nil {
		return nil, asProtocolError(err)
	}
	if dbErr != nil {
		return nil, dbErr
	}
	return out, nil
}

// saveSocial 把三个开关写回 characters（立即写库，不等 save）。角色已被删除时返回 errCharacterGone。
func (s *Service) saveSocial(ctx context.Context, charID int64, visible, invite, trade bool) error {
	tag, err := s.db.Exec(ctx, `UPDATE characters SET social_visible = $2, social_invite = $3, social_trade = $4 WHERE id = $1`,
		charID, visible, invite, trade)
	if err == nil && tag.RowsAffected() == 0 {
		return errCharacterGone
	}
	return err
}
