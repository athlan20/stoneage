// 宠物成长（STAGE-33，docs/ui.md 宠物页 C）：战斗经验与升级、捕捉入栏。
// 对照原版 gmsv/battle/battle.c 的战后结算、gmsv/char/char_data.c 的 CHAR_LevelUpCheck 与
// CHAR_PetLevelUp、gmsv/char/pet.c 的 PET_createPetFromCharaIndex。

package character

import (
	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// gainPetExp 给宠物加上战斗经验，按经验表连升到不能升为止，返回升到的等级。
// 经验按级计：每升一级从经验里扣掉这一级所需的量，剩下的是本级进度（原版 CHAR_LevelUpCheck
// 调 CHAR_HandleExp 的按级扣除，STAGE-57），不跨级累计。
// 每升一级：四项能力在现有基础上累加一次随机成长（gamedata.PetLevelUp，按资质和品质），
// 忠诚的可变部分 +5（AI_FIX_PETLEVELUP）。血量不变（原版升级不回血，只按新上限截断，上限只会变大）。
// 有等级上限（模板的，或对话改写过的个体值）的宠物到了上限就不再升，经验清零（CHAR_LevelUpCheck 的 LIMITLEVEL 分支）。
// rnd 用战斗的随机数，测试剧本里结果可复现。
func gainPetExp(pet *petRec, gain int64, rnd func(n int) int) int {
	pet.Exp += gain
	tmpl, ok := gamedata.PetByID(pet.TemplateID)
	if !ok {
		return pet.Level
	}
	// STAGE-234：上限用个体值（对话能改写），没改过的用模板的 LIMITLEVEL
	limit := petLimit(*pet, tmpl)
	maxLevel := gamedata.PetMaxLevelFor(limit)
	for {
		if limit > 0 && pet.Level >= limit {
			pet.Exp = 0
			break
		}
		next := gamedata.NextExp(pet.Level)
		if pet.Level >= maxLevel || next < 0 || pet.Exp < next {
			break
		}
		pet.Exp -= next
		pet.Level++
		pet.Stats = pet.Stats.Add(gamedata.PetLevelUp(pet.Base, pet.Rank, rnd))
		pet.VariableAI = gamedata.AddVariableAI(pet.VariableAI, gamedata.PetLevelUpAI)
	}
	return pet.Level
}

// placeCapturedPet 给捕捉到的怪物安排第一个空的宠物栏位（不做资质筛选），返回栏位号；
// 宠物栏已满时返回 -1。结算见 captureOutcome。
func placeCapturedPet(r *record, m *battle.Unit) int {
	slot, _ := captureOutcome(r, m, nil)
	return slot
}

// captureOutcome 是抓成功之后的入栏结算（引擎回调，STAGE-264）：先看随身栏位，满了返回 -1，
// 与原来一样（宠位满不再看资质，不会变成“抓成功被丢弃”）；有空位再按 keep 判资质，不合格不入库、
// 返回 discarded；合格才入栏位。照 PET_createPetFromCharaIndex：能力、等级、资质、品质原样从怪物身上带走，
// 血量是被抓时剩下的血量（不回满），忠诚的可变部分从 0 开始。
func captureOutcome(r *record, m *battle.Unit, keep *battle.CaptureKeep) (int, *battle.Discarded) {
	free := freeCarriedSlot(r)
	if free < 0 {
		return -1, nil
	}
	if keep != nil {
		q := captureQuality(m)
		if reason, ok := keep.Judge(q); !ok {
			return -1, &battle.Discarded{Name: m.Name, Level: m.Level, Quality: q, Reason: reason}
		}
	}
	hp := min(max(m.HP, 1), gamedata.PetMaxHP(m.Ability))
	r.Pets = append(r.Pets, petRec{
		Slot: free, TemplateID: m.TemplateID, Name: m.Name, Level: m.Level,
		Stats: m.Ability, Base: m.Growth, Rank: m.Rank, HP: hp,
		State: petStateRest, Skills: m.Skills, // 技能跟着怪物走（原版 PET_createPetFromCharaIndex）
		Origin: &PetOrigin{Level: m.Level, Stats: m.Ability}, // STAGE-215：起始值，被抓时怪物的等级和能力，进库后不改
	})
	return free, nil
}

// freeCarriedSlot 返回第一个空的随身栏位，满了返回 -1。寄放的宠物不占随身栏位（STAGE-73）。
func freeCarriedSlot(r *record) int {
	taken := [MaxPetSlots]bool{}
	for _, p := range r.Pets {
		if p.carried() { // 寄放的宠物不占随身栏位（STAGE-73，原来这里会越界）
			taken[p.Slot] = true
		}
	}
	for slot, used := range taken {
		if !used {
			return slot
		}
	}
	return -1
}

// captureQuality 按宠物页 5.19 的口径算这只怪物的四项资质：points 是生成时的资质（Growth），
// tier = points −（模板资质 + 2），同 qualityOf。
func captureQuality(m *battle.Unit) []battle.Quality {
	tmpl, _ := gamedata.PetByID(m.TemplateID)
	qs := qualityOf(&petRec{Base: m.Growth}, tmpl)
	out := make([]battle.Quality, 0, len(qs))
	for _, q := range qs {
		out = append(out, battle.Quality{Stat: q.Stat, Points: q.Points, Tier: q.Tier})
	}
	return out
}
