package character

// STAGE-207：加工、料理的合成（协议 5.55，docs/synthesis.md）：战斗外 pet_skill 的合成分支，
// 校验、决定结果在 merge_plan.go，这里负责写库、改内存和推送。

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// mergeSkillAt 报告宠物的第 skill 个技能栏位（从 1 起）是不是加工或料理（原版 PETSKILL_Merge）。
func mergeSkillAt(pet *petRec, skill int) bool {
	if pet == nil || skill < 1 || skill > len(pet.Skills) || pet.Skills[skill-1] < 0 {
		return false
	}
	sk, ok := gamedata.PetSkillByID(pet.Skills[skill-1])
	return ok && sk.Func == "PETSKILL_Merge"
}

// mergeRnd 返回合成的随机源：协议用例按用例名固定种子，重放的结果可复现；零值用全局随机（同 wildRnd）。
func (s *Service) mergeRnd() func(n int) int {
	if s.opts.MergeRnd != nil {
		return s.opts.MergeRnd
	}
	return rand.IntN
}

// petSkillMerge 是加工、料理的合成。调用时持有角色的锁（在 Live.Operate 里）。
// 扣料和放入产物在一个事务里（changeItems）：任何一步失败背包都不变、不推送；写库成功后才改内存、推送
// inventory_changed，并记下这次合成的时间（冷却，无论结果如何）。
func (s *Service) petSkillMerge(ctx context.Context, p *Presence, r *record, pet *petRec, items *[]int) (map[string]any, error) {
	slots, perr := checkMergeSlots(items)
	if perr != nil {
		return nil, perr
	}
	now := s.now()
	var res mergeResult
	var placed *itemRec
	err := s.changeItems(ctx, p, r, func(bag []itemRec, _ int) ([]itemRec, error) {
		if firstEmptyBagSlot(bag) < 0 {
			return nil, protocol.Errorf(protocol.CodeBagFull, "合成时，最少需空一格物品栏位！")
		}
		mats, perr := materialIDs(bag, slots)
		if perr != nil {
			return nil, perr
		}
		out, err := runMerge(mats, pet.TemplateID, mergeCooldown(p.lastMerge, now, len(slots)), s.mergeRnd())
		if err != nil {
			return nil, err
		}
		if out.Kind == "created" {
			if _, ok := gamedata.ItemByID(out.Product); !ok { // 配方产物不在物品表里：数据问题，按内部错误处理，不扣料
				return nil, fmt.Errorf("合成配方的产物 %d 不在物品表里", out.Product)
			}
		}
		next, pl, err := consumeAndPlace(bag, slots, out)
		if err != nil {
			return nil, err
		}
		res, placed = out, pl
		return next, nil
	})
	if err != nil {
		return nil, err
	}
	p.lastMerge = now
	return mergeView(res, placed, slots), nil
}
