package character

import "github.com/athlan20/stoneage/server/gamedata"

// 宠物的协议内容（STAGE-31，docs/protocol.md 5.19、5.20）。能力、资质只在服务端算，
// 客户端拿到的是显示用的数字：攻击、防御、敏捷、最大 HP 都已按公式截成整数，忠诚现算。

// petSkillView 是宠物技能栏里的一格。slot 从 1 开始，和页面上的编号一致；target_type 是技能的目标类型（10.5）。
type petSkillView struct {
	Slot       int                 `json:"slot"`
	ID         int                 `json:"id"`
	Name       string              `json:"name"`
	TargetType gamedata.TargetType `json:"target_type"`
}

// petView 是 pets 响应和 pets_changed 推送里的一只宠物（docs/protocol.md 5.19）。
type petView struct {
	Slot           int            `json:"slot"`
	SpeciesID      int            `json:"species_id"`
	Species        string         `json:"species"`
	Name           string         `json:"name"`
	Level          int            `json:"level"`
	Transmigration int            `json:"transmigration"`
	Exp            int64          `json:"exp"`
	NextExp        int64          `json:"next_exp"`
	HP             int            `json:"hp"`
	MaxHP          int            `json:"max_hp"`
	Attack         int            `json:"attack"`
	Defense        int            `json:"defense"`
	Agility        int            `json:"agility"`
	Loyalty        int            `json:"loyalty"`
	Elements       Elements       `json:"elements"`
	Growth         growthView     `json:"growth"`  // STAGE-216：成长率，算法见 pet_growth_view.go
	Quality        []qualityView  `json:"quality"` // STAGE-216：四项素质点和档位，固定四项
	State          string         `json:"state"`
	Skills         []petSkillView `json:"skills"`
}

// growthView 是宠物的成长率（STAGE-216，docs/protocol.md 5.19）。basis 为 unknown（历史宠物）时
// from_level 和 rates 的四项都是 null；rates 本身总是对象。
type growthView struct {
	Basis     string    `json:"basis"`
	FromLevel *int      `json:"from_level"`
	Rates     ratesView `json:"rates"`
}

// ratesView 是成长率的四项，每升一级的平均涨幅（两位小数）；算不出时为 nil，序列化成 null。
type ratesView struct {
	HP      *float64 `json:"hp"`
	Attack  *float64 `json:"attack"`
	Defense *float64 `json:"defense"`
	Agility *float64 `json:"agility"`
}

// qualityView 是一项素质（STAGE-216）：points 是生成时保存的素质点（库里 base_*），tier 是它离满值的档位，
// 0 为满，正常范围 −4 到 0，超出时照实发。
type qualityView struct {
	Stat   string `json:"stat"`
	Points int    `json:"points"`
	Tier   int    `json:"tier"`
}

// petsView 是 pets 响应和 pets_changed 推送的内容：只列有宠物的栏位，按栏位从小到大排列；
// 空栏位由客户端按 max_slots 补上。
func (r *record) petsView() map[string]any {
	pets := make([]petView, 0, len(r.Pets))
	for _, p := range r.Pets {
		if p.Slot >= gamedata.PooledSlotBase {
			continue // 寄放在宠物店的（STAGE-73）不在随身列表里
		}
		if v, ok := r.petEntry(p); ok {
			pets = append(pets, v)
		}
	}
	return map[string]any{"max_slots": MaxPetSlots, "pets": pets}
}

// petEntry 把一行宠物换成协议里的内容。模板找不到（表里没有这个编号）时返回 false 跳过；
// 管理命令只能给出表里有的编号，所以正常情况下不会发生。
func (r *record) petEntry(p petRec) (petView, bool) {
	tmpl, ok := gamedata.PetByID(p.TemplateID)
	if !ok {
		return petView{}, false
	}
	skills := make([]petSkillView, 0, gamedata.PetSkillSlots)
	for i, id := range p.Skills {
		if id < 0 {
			continue
		}
		sk, _ := gamedata.PetSkillByID(id)
		skills = append(skills, petSkillView{Slot: i + 1, ID: id, Name: sk.Name, TargetType: sk.BattleTarget()})
	}
	return petView{
		Slot:           p.Slot,
		SpeciesID:      tmpl.ID,
		Species:        tmpl.Name,
		Name:           p.Name,
		Level:          p.Level,
		Transmigration: p.Transmigration,
		Exp:            p.Exp,
		NextExp:        nextPetExp(p.Level, gamedata.PetMaxLevelFor(petLimit(p, tmpl))),
		HP:             p.HP,
		MaxHP:          gamedata.PetMaxHP(p.Stats),
		Attack:         gamedata.PetAttack(p.Stats),
		Defense:        gamedata.PetDefense(p.Stats),
		Agility:        gamedata.PetAgility(p.Stats),
		Loyalty:        petLoyalty(r, &p, tmpl),
		Elements:       Elements{Earth: tmpl.Earth, Water: tmpl.Water, Fire: tmpl.Fire, Wind: tmpl.Wind},
		Growth:         growthOf(&p),
		Quality:        qualityOf(&p, tmpl),
		State:          petStateNames[p.State],
		Skills:         skills,
	}, true
}

// nextPetExp 是升到下一级需要的累计经验；到了这种宠物的等级上限（gamedata.PetMaxLevel）时为 -1，
// 同角色的 next_exp。经验表比等级上限长（201 项），所以不能只看表：上限本身没有下一级。
func nextPetExp(level, maxLevel int) int64 {
	if level >= maxLevel {
		return -1
	}
	return gamedata.NextExp(level)
}
