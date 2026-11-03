// 宠物管理的规则（STAGE-32，docs/protocol.md 5.22–5.24，docs/ui.md 宠物页 B）：不碰数据库，
// 只检查和算出要改什么，写库和推送在 pets_manage.go。

package character

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/wordfilter"
)

// MaxBattlePets 是出战宠物的上限（原版服务端 PET_SelectBattleEntryPet：同时只有一只上场）。
const MaxBattlePets = 1

// MaxStandbyPets 是待命宠物的上限（原版服务端 SPET 待命掩码：3 只）。
const MaxStandbyPets = 3

// maxPetNameChars 是宠物名字的字符数上限（按 UTF-8 字符数计，与角色名相同）。
const maxPetNameChars = 16

// petStateByName 把协议里的 state（rest、standby、battle）换成 character_pets.state 的编码。
// 寄放和骑乘不能用 pet_state 设（寄放在宠物店，骑乘用 pet_ride），所以只认前三个。
func petStateByName(name *string) (int, bool) {
	if name == nil {
		return 0, false
	}
	for i, n := range petStateNames[:petStateBattle+1] {
		if n == *name {
			return i, true
		}
	}
	return 0, false
}

// findPet 返回栏位上的宠物；没有就返回 nil。
func findPet(pets []petRec, slot int) *petRec {
	for i := range pets {
		if pets[i].Slot == slot {
			return &pets[i]
		}
	}
	return nil
}

// planPetState 检查把 slot 设成 state 能不能做，返回要改的栏位和新状态（栏位 → 状态）。
// 不用改（已经是那个状态）时返回空的 map 和 nil 错误，调用方不写库、不推送。
// 出战时原来的出战宠自动变成休息，所以出战本身不会超过上限；待命满了、宠物倒下、栏位没有宠物都会失败，状态不变。
func planPetState(pets []petRec, slot, state int) (map[int]int, error) {
	target := findPet(pets, slot)
	if target == nil {
		return nil, protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
	}
	if target.State == petStateRide {
		return nil, protocol.Errorf(protocol.CodePetRiding, "骑乘中的宠物不能改状态，先下骑")
	}
	if target.State == state {
		return nil, nil
	}
	switch state {
	case petStateBattle:
		if target.HP <= 0 {
			return nil, protocol.Errorf(protocol.CodePetFainted, "%s 已经倒下，不能出战", target.Name)
		}
	case petStateStandby:
		standby := 0
		for i := range pets {
			if pets[i].State == petStateStandby && pets[i].Slot != slot {
				standby++
			}
		}
		if standby >= MaxStandbyPets {
			return nil, protocol.Errorf(protocol.CodeStandbyFull, "待命的宠物已经有 %d 只，先把一只改成休息", MaxStandbyPets)
		}
	}
	changes := map[int]int{slot: state}
	if state == petStateBattle {
		for i := range pets {
			if pets[i].State == petStateBattle && pets[i].Slot != slot {
				changes[pets[i].Slot] = petStateRest
			}
		}
	}
	return changes, nil
}

// normalizePetName 检查改名的新名字（docs/protocol.md 5.23）：去掉首尾空白后，1–16 个字符，不能含空白或不可见字符。
// 返回去掉首尾空白后的名字；空字符串表示恢复种族名，由调用方换成模板里的名字。
func normalizePetName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", nil
	}
	if n := utf8.RuneCountInString(name); n > maxPetNameChars {
		return "", protocol.Errorf(protocol.CodeInvalidName, "宠物名字最多 %d 个字符，现在有 %d 个", maxPetNameChars, n)
	}
	for _, r := range name {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return "", protocol.Errorf(protocol.CodeInvalidName, "宠物名字不能包含空白或不可见字符")
		}
	}
	// STAGE-287：和聊天共用同一份屏蔽词表，命中就拒绝，不回显是哪个词。
	if wordfilter.Current().Contains(name) {
		return "", protocol.Errorf(protocol.CodeInvalidName, "宠物名字含有不允许的内容")
	}
	return name, nil
}

// petSlotArg 检查请求里的 slot：必须有，且是 0–MaxPetSlots-1 的整数。
func petSlotArg(slot *int) (int, error) {
	if slot == nil || *slot < 0 || *slot >= MaxPetSlots {
		return 0, protocol.Errorf(protocol.CodeBadRequest, "需要 slot 字段，是 0–%d 的整数", MaxPetSlots-1)
	}
	return *slot, nil
}

// petName 返回改名后要存的名字：空名字恢复成模板里的种族名。
func petName(name string, templateName string) string {
	if name == "" {
		return templateName
	}
	return name
}
