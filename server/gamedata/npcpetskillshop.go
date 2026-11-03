package gamedata

// 宠物技能店（STAGE-143，docs/protocol.md 5.51）：npcs.tsv 里每家宠物技能店（npcgen_petskillshop）
// 教的技能、学习价格的倍率与欢迎语，由 tools/gen-npcs.py 从原版参数文件（genout/psks_*）生成。
// 规则出处是 gmsv/npc/npc_petskillshop.c：
//   - 上架的技能按 pet_skill 的顺序，跳过编号查不到的和行首 T 的非法技能（PETSKILL_ILLEGAL）；
//   - 学习价格 = 技能的 PETSKILL_COST × skill_rate（小数向下取整）；
//   - 学到哪个栏位就覆盖那个栏位原来的技能（CHAR_setPetSkill），不另外扣别的东西。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-petskillshop.json
var npcPetskillshopJSON []byte

// NpcPetskillShop 是一家宠物技能店的参数。
type NpcPetskillShop struct {
	SkillRate float64 `json:"skill_rate"`
	Skills    []int   `json:"skills"`         // 这家店教的技能编号，按原版顺序
	Main      string  `json:"main,omitempty"` // 欢迎语，`\n` 保持字面，发送时才换行
}

var (
	petskillshopOnce sync.Once
	petskillshopByID map[int]*NpcPetskillShop
)

// NpcPetskillShopOf 返回一家宠物技能店的参数；这个 NPC 不是宠物技能店时返回 nil。
func NpcPetskillShopOf(id int) *NpcPetskillShop {
	petskillshopOnce.Do(loadNpcPetskillShop)
	return petskillshopByID[id]
}

// ListedSkills 是这家店上架的技能：按原版顺序取编号能查到、且不是非法（行首 T）的技能。
func (s *NpcPetskillShop) ListedSkills() []PetSkill {
	out := []PetSkill{}
	for _, id := range s.Skills {
		sk, ok := PetSkillByID(id)
		if !ok || sk.Illegal {
			continue
		}
		out = append(out, sk)
	}
	return out
}

// LearnCost 是学一个技能的价格：技能的学习价格乘这家店的倍率，向下取整（原版 cost * rate 的截断）。
func (s *NpcPetskillShop) LearnCost(sk PetSkill) int {
	return int(float64(sk.Cost) * s.SkillRate)
}

func loadNpcPetskillShop() {
	petskillshopByID = map[int]*NpcPetskillShop{}
	var list []struct {
		ID int `json:"id"`
		NpcPetskillShop
	}
	if err := json.Unmarshal(npcPetskillshopJSON, &list); err != nil {
		petskillshopByID = map[int]*NpcPetskillShop{}
		return
	}
	for i := range list {
		r := list[i].NpcPetskillShop
		petskillshopByID[list[i].ID] = &r
	}
}
