// 队伍的协议视图（STAGE-162，docs/protocol.md 5.57）：party 响应、party_changed 推送、enter_world / status 的 party 字段共用。

package character

import "github.com/athlan20/stoneage/server/gamedata"

// partyView 是队伍对象；没有队伍时调用方拿到的是 nil 指针，编码出来是 null。
type partyView struct {
	Kind    string            `json:"kind"`
	Leader  string            `json:"leader"`
	Members []partyMemberView `json:"members"`
}

// partyMemberView 是队伍里的一个成员（队长或分身）。
type partyMemberView struct {
	Index    int               `json:"index"`
	Name     string            `json:"name"`
	Leader   bool              `json:"leader"`
	Clone    bool              `json:"clone"`
	Level    int               `json:"level"`
	HP       int               `json:"hp"`
	MaxHP    int               `json:"max_hp"`
	MP       int               `json:"mp"`
	MaxMP    int               `json:"max_mp"`
	Pet      *partyPetView     `json:"pet"`
	Position partyPositionView `json:"position"`
	partyDetail
}

// partyPetView 是成员的出战宠物。
type partyPetView struct {
	Name   string   `json:"name"`
	Level  int      `json:"level"`
	HP     int      `json:"hp"`
	MaxHP  int      `json:"max_hp"`
	Skills []string `json:"skills"`
}

type partyPositionView struct {
	Floor   int    `json:"floor"`
	MapName string `json:"map_name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

// partyDetail 是详情页用的快照：能力、衍生数值、装备和精灵。创建分身时从队长此刻的状态取一份，分身之后不再变化。
type partyDetail struct {
	Stats     Stats                `json:"stats"`
	Derived   partyDerivedView     `json:"derived"`
	Equipment []partyEquipmentView `json:"equipment"`
	Magic     []partyMagicView     `json:"magic"`
}

type partyDerivedView struct {
	Attack  int `json:"attack"`
	Defense int `json:"defense"`
	Agility int `json:"agility"`
}

type partyEquipmentView struct {
	Slot int    `json:"slot"`
	Name string `json:"name"`
}

type partyMagicView struct {
	Slot int    `json:"slot"`
	Name string `json:"name"`
}

// newPartyDetail 取角色此刻的详情快照；列表字段没有内容时是空数组，不是 null。
func newPartyDetail(r *record) partyDetail {
	st := r.status()
	d := partyDetail{
		Stats:     r.Stats,
		Derived:   partyDerivedView{Attack: st.Derived.Attack, Defense: st.Derived.Defense, Agility: st.Derived.Agility},
		Equipment: []partyEquipmentView{},
		Magic:     []partyMagicView{},
	}
	for _, it := range r.Items {
		if !gamedata.IsEquipSlot(it.Slot) {
			continue
		}
		if def, ok := gamedata.ItemByID(it.ItemID); ok {
			d.Equipment = append(d.Equipment, partyEquipmentView{Slot: it.Slot, Name: def.Name})
		}
	}
	for _, m := range st.Magic {
		d.Magic = append(d.Magic, partyMagicView{Slot: m.Slot, Name: m.Name})
	}
	return d
}

// leaderPet 是队长的出战宠物（只读）：标为出战的优先，没有时第一只随身、没在骑的顶上，同 sideInput 的取法但不改状态。
func leaderPet(r *record) *petRec {
	var first *petRec
	hasRider := false
	for i := range r.Pets {
		p := &r.Pets[i]
		if !p.carried() {
			continue
		}
		switch {
		case p.State == petStateRide:
			hasRider = true
		case p.State == petStateBattle:
			return p
		case first == nil:
			first = p
		}
	}
	if hasRider {
		return nil
	}
	return first
}

// partyView 生成队伍对象；没有队伍返回 nil。队长的数值读角色的实时值，分身用自己的快照。调用时要持有 p.mu。
func (p *Presence) partyView(r *record) *partyView {
	if p.party == nil {
		return nil
	}
	pos := partyPositionView{Floor: r.Floor, MapName: mapName(r.Floor), X: r.X, Y: r.Y}
	leader := partyMemberView{
		Index: 0, Name: r.Name, Leader: true, Level: r.Level,
		HP: r.HP, MaxHP: r.maxHP(), MP: r.MP, MaxMP: gamedata.NewPlayerMaxMP,
		Position: pos, partyDetail: newPartyDetail(r),
	}
	if lp := leaderPet(r); lp != nil {
		leader.Pet = &partyPetView{Name: lp.Name, Level: lp.Level, HP: lp.HP, MaxHP: gamedata.PetMaxHP(lp.Stats), Skills: petSkillNames(lp.Skills[:])}
	}
	v := &partyView{Kind: "clone", Leader: r.Name, Members: []partyMemberView{leader}}
	for _, c := range p.party.clones {
		m := partyMemberView{
			Index: c.index, Name: c.in.Name, Clone: true, Level: c.in.Level,
			HP: c.in.HP, MaxHP: c.in.MaxHP, MP: c.in.MP, MaxMP: c.in.MaxMP,
			Position: pos, partyDetail: c.detail,
		}
		if bp := c.in.BattlePet; bp != nil {
			m.Pet = &partyPetView{Name: bp.Name, Level: bp.Level, HP: bp.HP, MaxHP: bp.MaxHP, Skills: petSkillNames(bp.Skills[:])}
		}
		v.Members = append(v.Members, m)
	}
	return v
}

// petSkillNames 列出宠物认识的技能名（空栏位不列）。
func petSkillNames(skills []int) []string {
	out := []string{}
	for _, id := range skills {
		if sk, ok := gamedata.PetSkillByID(id); ok {
			out = append(out, sk.Name)
		}
	}
	return out
}
