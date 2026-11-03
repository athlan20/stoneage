// 忠诚道具（STAGE-326）：安抚之素（忠30）、高级宠物食品（忠10）给宠物加忠诚度。
// 出处是原版服务端 item_event.c 的 ITEM_useRecovery_Field：忠诚（BD_KIND_AI）只对宠物生效，
// 写进可变部分（CHAR_VARIABLEAI，单位是忠诚 × 100，上限 ±10000，见 gamedata.AddVariableAI），
// 忠诚总值由 petLoyalty 现算并限制在 0–100。只能在战斗外用（原版 battle_item.c 不认“忠”）。
// 与原版的差别：增量固定为参数 N，不取 RAND(0.9N, 1.1N)，与需求的验收口径（70→100、40→70）一致。

package character

import (
	"context"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// loyaltyCap 是宠物忠诚的上限（gamedata.Loyalty 的 0–100）。
const loyaltyCap = 100

// useLoyaltyOnPet 给宠物用忠诚道具：忠诚已满不扣道具、回 ITEM_UNUSABLE；否则加 gain 点，
// 扣物品，推 pets_changed 与 inventory_changed，并把忠诚写库（同 useItemOnPet 的收尾）。
// 调用时要持有角色的锁（在 Live.Operate 里）。
func (s *Service) useLoyaltyOnPet(ctx context.Context, p *Presence, r *record, slot int, pet *petRec, gain int) (map[string]any, error) {
	tmpl, ok := gamedata.PetByID(pet.TemplateID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s 的宠物模板不存在，不能使用", pet.Name)
	}
	before := petLoyalty(r, pet, tmpl)
	if before >= loyaltyCap {
		return nil, protocol.Errorf(protocol.CodeItemUnusable, "%s 的忠诚度已经是满的，用不着", pet.Name)
	}
	pet.VariableAI = gamedata.AddVariableAI(pet.VariableAI, gain*100)
	after := petLoyalty(r, pet, tmpl)
	if err := s.consumeItem(ctx, p, r, slot); err != nil {
		return nil, err
	}
	s.savePets(p.charID, r.Pets)
	p.pushAll("pets_changed", r.petsView())
	return map[string]any{
		"target": "pet", "slot": pet.Slot, "name": pet.Name,
		"loyalty": after, "loyalty_delta": after - before,
	}, nil
}
