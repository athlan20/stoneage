// 战斗外使用宠物技能（STAGE-144，docs/protocol.md 5.50）：pet_skill 的校验。
// 原版 lssproto_PS_recv 按宠物编号、技能栏位取技能再调用 PETSKILL_Use；PETSKILL_Use 对场合带“战斗”位的技能
// 要求主人处于战斗中（gmsv/battle/pet_skill.c 的 _PETSKILL_CHECKTYPE），所以战斗外只能用场合为战斗外的技能。
// 一期数据里这样的技能只有加工、料理（PETSKILL_Merge）：效果是原版的合成（ITEM_mergeItem：配方、原子、随机结果），
// STAGE-207 起由 merge.go 实现，这里只负责挡住战斗中、倒下、没有宠物、空栏位和战斗技能。

package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

type petSkillRequest struct {
	Slot  *int   `json:"slot"`
	Skill *int   `json:"skill"`
	Items *[]int `json:"items"` // 合成用的背包格号（STAGE-207），只有加工、料理用
}

// handlePetSkill 是 pet_skill（docs/protocol.md 5.50）：战斗外使用宠物的技能，slot 是宠物栏位（0–19），skill 是宠物页的技能栏位（1–7）。
func (s *Service) handlePetSkill(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in petSkillRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := petSlotArg(in.Slot)
	if err != nil {
		return nil, err
	}
	if in.Skill == nil || *in.Skill < 1 || *in.Skill > gamedata.PetSkillSlots {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 skill 字段，是 1–%d 的整数（宠物页的技能栏位）", gamedata.PetSkillSlots)
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var opErr error
	out := map[string]any{}
	if err := l.Operate(func(r *record) {
		if perr := inBattle(l); perr != nil { // 战斗中用技能走 battle_command 的 skill 指令（10.5）
			opErr = perr
			return
		}
		if r.HP <= 0 {
			opErr = protocol.Errorf(protocol.CodeCharacterDead, "角色已经死亡，不能使用技能")
			return
		}
		if perr := checkPetSkillOutside(r, slot, *in.Skill); perr != nil {
			opErr = perr
			return
		}
		if pet := findPet(r.Pets, slot); mergeSkillAt(pet, *in.Skill) { // 加工、料理：合成（STAGE-207）
			out, opErr = s.petSkillMerge(ctx, l.p, r, pet, in.Items)
		}
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return out, nil
}

// checkPetSkillOutside 检查战斗外使用宠物技能的条件，调用时要持有角色的锁（在 Live.Operate 里）。
// 能通过的是战斗外场合的技能：加工、料理（STAGE-207 起是合成，见 merge.go）；其他战斗外技能目前没有效果，回复 SKILL_UNUSABLE。
func checkPetSkillOutside(r *record, slot, skill int) *protocol.Error {
	pet := findPet(r.Pets, slot)
	if pet == nil {
		return protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
	}
	id := pet.Skills[skill-1]
	if id < 0 {
		return protocol.Errorf(protocol.CodeSkillUnusable, "%d 号宠物的 %d 号技能栏位是空的", slot, skill)
	}
	sk, ok := gamedata.PetSkillByID(id)
	if !ok {
		return protocol.Errorf(protocol.CodeSkillUnusable, "不认识的技能编号 %d", id)
	}
	if sk.Field == gamedata.PetSkillFieldBattle {
		return protocol.Errorf(protocol.CodeSkillUnusable, "%s 只能在战斗中使用", sk.Name)
	}
	if sk.Func == "PETSKILL_Merge" {
		return nil
	}
	return protocol.Errorf(protocol.CodeSkillUnusable, "%s 现在还不能使用", sk.Name)
}
