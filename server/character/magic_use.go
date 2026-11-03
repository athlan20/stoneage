// 战斗外使用精灵（docs/protocol.md 5.51，magic_use）。能在战斗外用的精灵：
// - 变幻的精灵（STAGE-203，原版 MAGIC_Metamo）：变身成自己的宠物、变回自己；
// - 治愈、滋润、恩惠（STAGE-230，原版 MAGIC_Recovery、MAGIC_OtherRecovery 的战斗外分支，见 magic_use_heal.go）；
// - 复活、解状态、防御类（STAGE-230）：原版扣 MP 后没有效果、没有台词，这里照做。
// 其他精灵（攻击、状态、场地等）战斗外不生效，回复 MAGIC_UNUSABLE。原版先扣 MP 再分支，所以变回自己也扣。

package character

import (
	"context"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// metamoRefusedImage 是加美的图像编号（原版 MAGIC_Metamo 写死：不能变身成加美），加美是骑乘用的宠物。
const metamoRefusedImage = 100259

// magicUseRequest 是 magic_use 的请求（5.51）。对象由 To 和 Target 决定，见 parseMagicObject。
type magicUseRequest struct {
	Slot   *int   `json:"slot"`   // 精灵栏位 0–4
	Target *int   `json:"target"` // 对象的栏位 0–19：to 为 pet 时是宠物栏位，为 member 时是队友；不带 = 自己（to 缺省时）
	To     string `json:"to"`     // 对象：self（自己）、pet（宠物）、member（队友）；缺省时带了 target 就是 pet
}

// handleMagicUse 是 magic_use（docs/protocol.md 5.51）：战斗外使用精灵。观察方由 Operate 拦截。
func (s *Service) handleMagicUse(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in magicUseRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	var opErr *protocol.Error
	if err := l.Operate(func(r *record) {
		if perr := inBattle(l); perr != nil { // 战斗中不做变身（只改外观，终端不画图像），回复 IN_BATTLE
			opErr = perr
			return
		}
		if r.HP <= 0 {
			opErr = protocol.Errorf(protocol.CodeCharacterDead, "角色已经死亡，不能使用精灵")
			return
		}
		out, opErr = s.useMagic(l.p, r, in, s.now())
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if opErr != nil {
		return nil, opErr
	}
	return out, nil
}

// magicObject 是 magic_use 的对象：自己、宠物或队友，index 是宠物或队友的栏位（自己时无意义）。
type magicObject struct {
	kind  string
	index int
}

const (
	magicToSelf   = "self"
	magicToPet    = "pet"
	magicToMember = "member"
)

// parseMagicObject 读请求里的对象。target 有值时先查范围（0–19）；to 缺省时没有 target 是自己、有 target 是宠物。
func parseMagicObject(in magicUseRequest) (magicObject, *protocol.Error) {
	if in.Target != nil && (*in.Target < 0 || *in.Target >= MaxPetSlots) {
		return magicObject{}, protocol.Errorf(protocol.CodeBadRequest, "target 要在 0–%d 之间（宠物或队友栏位）", MaxPetSlots-1)
	}
	kind := in.To
	if kind == "" {
		kind = magicToSelf
		if in.Target != nil {
			kind = magicToPet
		}
	}
	switch kind {
	case magicToSelf:
		return magicObject{kind: kind}, nil
	case magicToPet, magicToMember:
		if in.Target == nil {
			return magicObject{}, protocol.Errorf(protocol.CodeBadRequest, "to 是 %s 时需要 target（栏位）", kind)
		}
		return magicObject{kind: kind, index: *in.Target}, nil
	}
	return magicObject{}, protocol.Errorf(protocol.CodeBadRequest, "to 只能是 self、pet 或 member")
}

// useMagic 检查并执行战斗外使用精灵，调用时要持有角色的锁。返回响应的 data：变幻的精灵是 {"metamo": 变身状态或 null}，
// 治愈、滋润、恩惠是 {"heal": ...}，没有效果的是 {"heal": null}。
func (s *Service) useMagic(p *Presence, r *record, in magicUseRequest, now time.Time) (map[string]any, *protocol.Error) {
	if in.Slot == nil || *in.Slot < 0 || *in.Slot >= gamedata.MaxMagicSlots {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 slot 字段，是 0–%d 的整数（精灵栏位）", gamedata.MaxMagicSlots-1)
	}
	obj, perr := parseMagicObject(in)
	if perr != nil {
		return nil, perr
	}
	ms := r.magicSlots()[*in.Slot]
	if ms.ID < 0 {
		return nil, protocol.Errorf(protocol.CodeMagicUnusable, "这个栏位没有精灵")
	}
	t, ok := gamedata.MagicByID(ms.ID)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeMagicUnusable, "这个精灵不能在战斗外使用")
	}
	switch {
	case t.Func == gamedata.MagicFuncMetamo:
		return s.useMetamo(p, r, t, ms, obj, now)
	case t.Func == gamedata.MagicFuncHeal || t.Func == gamedata.MagicFuncHealOther:
		return s.useHealMagic(p, r, t, ms, obj)
	case isOutsideNoEffect(t.Func):
		if perr := spendMagicMP(r, ms); perr != nil {
			return nil, perr
		}
		p.pushAll("status_changed", map[string]any{"status": r.status()})
		return map[string]any{"heal": nil}, nil
	}
	return nil, protocol.Errorf(protocol.CodeMagicUnusable, "这个精灵不能在战斗外使用")
}

// useMetamo 是变幻的精灵（原版 MAGIC_Metamo）：骑乘、远距离武器、目标宠物、MP，照原版顺序检查。
func (s *Service) useMetamo(p *Presence, r *record, t gamedata.MagicTemplate, ms magicSlot, obj magicObject, now time.Time) (map[string]any, *protocol.Error) {
	if hasRidingPet(r.Pets) {
		return nil, protocol.Errorf(protocol.CodeRequirementNotMet, "无法变身，骑乘中不能变身！")
	}
	if hand := findItem(r.Items, gamedata.SlotHand); hand != nil && isRangedWeapon(hand.ItemID) {
		return nil, protocol.Errorf(protocol.CodeRequirementNotMet, "使用远距离武器无法变身！")
	}
	if obj.kind == magicToMember {
		return nil, protocol.Errorf(protocol.CodeInvalidTarget, "变幻的精灵只能变成自己的宠物")
	}
	var pet *petRec
	var tmpl gamedata.PetTemplate
	var ok bool
	if obj.kind == magicToPet {
		if pet = findPet(r.Pets, obj.index); pet == nil {
			return nil, protocol.Errorf(protocol.CodeInvalidTarget, "%d 号栏位没有宠物", obj.index)
		}
		if tmpl, ok = gamedata.PetByID(pet.TemplateID); !ok {
			return nil, protocol.Errorf(protocol.CodeNoPet, "%d 号宠物的模板不认识", obj.index)
		}
		if tmpl.Image == metamoRefusedImage {
			return nil, protocol.Errorf(protocol.CodeRequirementNotMet, "不能变身成加美！")
		}
	}
	if perr := spendMagicMP(r, ms); perr != nil {
		return nil, perr
	}
	if pet == nil { // 变回自己：到期时间清掉
		stopMetamo(r)
		p.pushMetamoLocked(r, "变回自己！")
		return map[string]any{"metamo": nil}, nil
	}
	p.startMetamo(r, pet.Name, tmpl.Image, now, time.Duration(t.Power)*time.Second)
	p.pushMetamoLocked(r, "变身成"+pet.Name+"！")
	return map[string]any{"metamo": r.metamoView()}, nil
}

// spendMagicMP 扣精灵的 MP（原版先判 MP 再扣）。MP 不够回复 NOT_ENOUGH_MP，什么都不做；扣了不推送，调用方自己推 status_changed。
func spendMagicMP(r *record, ms magicSlot) *protocol.Error {
	if r.MP < ms.MP {
		return protocol.Errorf(protocol.CodeNotEnoughMP, "气力不够，需要 %d", ms.MP)
	}
	r.MP -= ms.MP
	return nil
}

// isOutsideNoEffect 报告战斗外扣 MP 后没有效果、没有台词的精灵（STAGE-230，原版 MAGIC_Ressurect、
// MAGIC_StatusRecovery、MAGIC_MagicDef、MAGIC_MagicStatusChange 的战斗外分支）。
func isOutsideNoEffect(fn string) bool {
	switch fn {
	case gamedata.MagicFuncRevive, gamedata.MagicFuncCure, gamedata.MagicFuncDefend, gamedata.MagicFuncReviveDefend, gamedata.MagicFuncResist:
		return true
	}
	return false
}
