// 骑乘的上骑和下骑（STAGE-123，docs/ride.md §3–4，docs/protocol.md 5.47–5.48）：pet_ride 把宠物骑上去，
// pet_unride 骑下来。检查顺序照原版 gmsv/char/family.c 的 FAMILY_RidePet（规则在 planPetRide），
// 写库和推送同 pet_state：加行锁读宠物，检查，事务里写 state，提交后推 pets_changed。
// 骑乘中的宠物（state=4）不占出战和待命的名额，同时最多 1 只。

package character

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// MaxRidePets 是同时骑乘的宠物上限（原版一个人只能骑一只，CHAR_RIDEPET）。
const MaxRidePets = 1

// DefaultRideLevelGap 是人物等级加上它之后不能低于宠物等级（docs/ride.md §3 第 8 条，原版写死 5）。
const DefaultRideLevelGap = 5

// MaxRideTransmigration 是宠物能骑的最高转生次数（docs/ride.md 第 9 条，原版 family.c 的 _PET_2TRANS 检查）：超过 2 次不能骑。
const MaxRideTransmigration = 2

// rideTarget 是上骑时人物那边要看的数值：骑乘等级、人物等级、外形和忠诚所需的人物魅力等。
type rideTarget struct {
	CharLevel int
	RideLevel int
	LevelGap  int
	Body      int
	Color     int
	Loyalty   int // 这只宠物的忠诚（现算，见 petLoyalty）
}

// petLoyalty 是这只宠物的忠诚（同宠物页的算法，gamedata.Loyalty）。
func petLoyalty(r *record, p *petRec, tmpl gamedata.PetTemplate) int {
	return gamedata.Loyalty(r.Level, gamedata.EffectiveCharm(r.Charm, equipBonus(r.Items).Charm), r.Transmigration,
		p.Level, tmpl.ModAI, p.VariableAI)
}

// planPetRide 检查把 slot 上骑能不能做（docs/protocol.md 5.47 的顺序），返回要改的栏位和新状态。
// 已经在骑这只：成功但不改任何东西（返回空的 map）。
func planPetRide(pets []petRec, slot int, t rideTarget) (map[int]int, error) {
	target := findPet(pets, slot)
	if target == nil {
		return nil, protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
	}
	for i := range pets {
		if pets[i].State == petStateRide {
			if pets[i].Slot == slot {
				return nil, nil
			}
			return nil, protocol.Errorf(protocol.CodeAlreadyRiding, "已经在骑 %d 号栏的宠物，先下骑", pets[i].Slot)
		}
	}
	if target.State == petStateBattle || target.State == petStateStandby {
		return nil, protocol.Errorf(protocol.CodePetNotRest, "出战或待命的宠物不能骑，先休息")
	}
	if t.RideLevel < target.Level {
		if t.RideLevel == gamedata.RideLevelNone { // 一次都没学过：说“没学过”，免得以为是宠物等级的问题（STAGE-181）
			return nil, protocol.Errorf(protocol.CodeRideNotLearned, "你还没有学过骑乘，先去骑乘训练师那里学习")
		}
		return nil, protocol.Errorf(protocol.CodeRideNotLearned, "你目前只能骑乘等级小于等于 %d 级的宠", t.RideLevel)
	}
	if t.Loyalty < 100 {
		return nil, protocol.Errorf(protocol.CodeRideLoyaltyLow, "该骑宠的忠诚小于 100")
	}
	if t.CharLevel+t.LevelGap < target.Level {
		return nil, protocol.Errorf(protocol.CodeRideLevelGap, "你最高只能骑宠等级比你大 %d 级的宠", t.LevelGap)
	}
	if target.Transmigration > MaxRideTransmigration {
		return nil, protocol.Errorf(protocol.CodeRideTransLimit, "转生次数超过 %d 次的宠物不能骑", MaxRideTransmigration)
	}
	if !gamedata.RideAllowed(t.Body, t.Color, target.TemplateID) {
		return nil, protocol.Errorf(protocol.CodeRideNotMatch, "这只宠物不适合你的外形")
	}
	return map[int]int{slot: petStateRide}, nil
}

// planPetUnride 下骑：骑着的宠物回到休息；没有在骑时不改任何东西。
func planPetUnride(pets []petRec) map[int]int {
	for i := range pets {
		if pets[i].State == petStateRide {
			return map[int]int{pets[i].Slot: petStateRest}
		}
	}
	return nil
}

// handlePetRide 是 pet_ride（docs/protocol.md 5.47）。
func (s *Service) handlePetRide(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in petSlotRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := petSlotArg(in.Slot)
	if err != nil {
		return nil, err
	}
	return s.operateRide(ctx, sess, func(r *record, pets []petRec) (map[int]int, error) {
		target := findPet(pets, slot)
		if target == nil {
			return planPetRide(pets, slot, rideTarget{})
		}
		tmpl, ok := gamedata.PetByID(target.TemplateID)
		if !ok {
			return nil, fmt.Errorf("宠物模板 %d 不存在", target.TemplateID)
		}
		return planPetRide(pets, slot, rideTarget{
			CharLevel: r.Level, RideLevel: r.RideLevel, LevelGap: s.opts.RideLevelGap,
			Body: r.Appearance.Body, Color: r.Appearance.Color,
			Loyalty: petLoyalty(r, target, tmpl),
		})
	})
}

// handlePetUnride 是 pet_unride（docs/protocol.md 5.48）。
func (s *Service) handlePetUnride(ctx context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	return s.operateRide(ctx, sess, func(_ *record, pets []petRec) (map[int]int, error) {
		return planPetUnride(pets), nil
	})
}

// petSlotRequest 是只带栏位的请求（pet_ride）。
type petSlotRequest struct {
	Slot *int `json:"slot"`
}

// operateRide 是 pet_ride 和 pet_unride 共用的入口：世界中、操控方、战斗外，然后执行 plan 并写库。
// plan 返回要改的栏位 → 新状态；空的 map 表示不用改，不写库、不推送。
func (s *Service) operateRide(ctx context.Context, sess *gateway.Session, plan func(r *record, pets []petRec) (map[int]int, error)) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var changeErr error
	if err := l.Operate(func(r *record) {
		if e := inBattle(l); e != nil { // 战斗中不能上下骑（docs/protocol.md 5.47）
			changeErr = e
			return
		}
		changeErr = s.changePets(ctx, l.p, r, func(tx pgx.Tx, charID int64, pets []petRec) ([]petRec, bool, error) {
			changes, err := plan(r, pets)
			if err != nil || len(changes) == 0 {
				return nil, false, err
			}
			next := append([]petRec(nil), pets...)
			for i := range next {
				if st, ok := changes[next[i].Slot]; ok {
					next[i].State = st
				}
			}
			for sl, st := range changes {
				if _, err := tx.Exec(ctx, `UPDATE character_pets SET state = $3 WHERE character_id = $1 AND slot = $2`, charID, sl, st); err != nil {
					return nil, false, err
				}
			}
			return next, true, nil
		})
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if changeErr != nil {
		return nil, changeErr
	}
	return map[string]any{}, nil
}
