// STAGE-215：宠物的起始快照，宠物进库（到手）那一刻的等级和能力，之后任何更新都不改它。
// 列见迁移 0021_pet_origin.sql；历史宠物五列全空，读出来 Origin 为 nil，不倒推。

package character

import "github.com/athlan20/stoneage/server/gamedata"

// PetOrigin 是宠物的起始等级与起始能力（gamedata.Ability，1/100 点）。
// 一经写入不再修改：对象可能在 petRec 之间共享（party.go 的拷贝、交易的快照），所以只读，不原地改。
type PetOrigin struct {
	Level int
	Stats gamedata.Ability
}

// originArgs 把起始值展开成 origin_level、origin_vital、origin_strength、origin_toughness、origin_dexterity
// 五个参数（与 petOriginColumns 的顺序一致）。o 为 nil（历史宠物，没有基准）时五个都是 nil，写库成 NULL。
func originArgs(o *PetOrigin) (level, vital, strength, toughness, dexterity any) {
	if o == nil {
		return nil, nil, nil, nil, nil
	}
	return o.Level, o.Stats.Vital, o.Stats.Strength, o.Stats.Toughness, o.Stats.Dexterity
}

// originOf 把读出来的五列还原成起始值：五列全空得 nil，全有得结构（迁移的 CHECK 保证不会只有一部分）。
func originOf(level, vital, strength, toughness, dexterity *int) *PetOrigin {
	if level == nil || vital == nil || strength == nil || toughness == nil || dexterity == nil {
		return nil
	}
	return &PetOrigin{
		Level: *level,
		Stats: gamedata.Ability{Vital: *vital, Strength: *strength, Toughness: *toughness, Dexterity: *dexterity},
	}
}

// petOriginColumns 是起始值的五列（写入和读出时共用，顺序同 originArgs）。
const petOriginColumns = `origin_level, origin_vital, origin_strength, origin_toughness, origin_dexterity`
