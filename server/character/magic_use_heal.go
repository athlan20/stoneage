// STAGE-230：战斗外的治愈、滋润、恩惠（docs/protocol.md 5.51），照原版 gmsv/magic/magic.c 的 MAGIC_Recovery、
// MAGIC_OtherRecovery 与 magic_field.c 的战斗外分支：
// - 回复量 RAND(p×0.9, p×1.1)，取整后加到血量上，封顶最大血量；_MAGIC_REHPAI 打开，不乘恢复率。
// - 治愈、恩惠战斗外只回自己（原版 MAGIC_Recovery_Field 不看 toindex）；滋润可以对自己、宠物或队友。
// - 先扣 MP 再判断对象（原版先扣后判）：对象无效回复 INVALID_TARGET，MP 已扣，见 5.51。
// - 中毒限制（原版 CHAR_CanCureFlg “HP”）：新服务端战斗外没有中毒状态（中毒只在战斗中），这条规则暂不生效。

package character

import (
	"fmt"
	"math/rand/v2"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// magicRnd 返回精灵回复量的随机源：协议用例按用例名固定种子；零值用全局随机（同 talkRnd）。
func (s *Service) magicRnd() func(n int) int {
	if s.opts.MagicUseRnd != nil {
		return s.opts.MagicUseRnd
	}
	return rand.IntN
}

// rollMagicHeal 是回复量 RAND(p×0.9, p×1.1)（取整），返回值在 [lo, hi] 之间（同战斗的 reviveUnit）。
func (s *Service) rollMagicHeal(power int) int {
	lo, hi := int(float64(power)*0.9), int(float64(power)*1.1)
	return lo + s.magicRnd()(hi-lo+1)
}

// useHealMagic 是治愈、滋润、恩惠的战斗外使用：扣 MP，找对象，回血，推送并回复 {"heal": ...}。
// 调用时要持有角色的锁。
func (s *Service) useHealMagic(p *Presence, r *record, t gamedata.MagicTemplate, ms magicSlot, obj magicObject) (map[string]any, *protocol.Error) {
	if t.Func == gamedata.MagicFuncHeal { // 治愈、恩惠战斗外只回自己
		obj = magicObject{kind: magicToSelf}
	}
	if perr := spendMagicMP(r, ms); perr != nil {
		return nil, perr
	}
	var out map[string]any
	var perr *protocol.Error
	switch obj.kind {
	case magicToSelf:
		out = s.healSelf(p, r, t.Power)
	case magicToPet:
		pet := findPet(r.Pets, obj.index)
		if pet == nil {
			perr = protocol.Errorf(protocol.CodeInvalidTarget, "%d 号栏位没有宠物", obj.index)
			break
		}
		out = s.healPet(p, r, pet, t.Power)
	default: // 队友：一期没有组队的玩家，任何队友栏位都是无效对象
		perr = protocol.Errorf(protocol.CodeInvalidTarget, "没有队友，不能对队友使用")
	}
	p.pushAll("status_changed", map[string]any{"status": r.status()}) // MP 已扣，血量可能也变了
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"heal": out}, nil
}

// healSelf 回复自己的血量，消息栏提示“耐久力回复N”（原版 MAGIC_Recovery_Field 的台词）。
func (s *Service) healSelf(p *Presence, r *record, power int) map[string]any {
	before := r.HP
	r.HP = min(before+s.rollMagicHeal(power), r.maxHP())
	amount := r.HP - before
	p.pushAll("notice", map[string]string{"level": "info", "text": fmt.Sprintf("耐久力回复%d", amount)})
	return map[string]any{"amount": amount, "to": magicToSelf}
}

// healPet 回复宠物的血量，血量写库并推送 pets_changed；台词“某某的耐久力回复N”（原版 MAGIC_OtherRecovery_Field）。
// 倒下的宠物（血量 0）照原版直接加血，不拦截。
func (s *Service) healPet(p *Presence, r *record, pet *petRec, power int) map[string]any {
	before := pet.HP
	pet.HP = min(before+s.rollMagicHeal(power), gamedata.PetMaxHP(pet.Stats))
	amount := pet.HP - before
	s.savePets(p.charID, r.Pets) // 宠物血量写库（同道具回复宠物，itemuse.go）
	p.pushAll("pets_changed", r.petsView())
	p.pushAll("notice", map[string]string{"level": "info", "text": fmt.Sprintf("%s的耐久力回复%d", pet.Name, amount)})
	return map[string]any{"amount": amount, "to": magicToPet, "target": pet.Slot}
}
