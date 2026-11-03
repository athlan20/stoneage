// 决斗的推送内容（STAGE-306，docs/protocol.md 10.14 与 5.74）：battle_start 的 duel 字段、
// 按观看者的方裁剪的 battle_round，以及 battle_end。

package battle

// DuelMember 是 battle_start.duel.sides[].members 的一项。
type DuelMember struct {
	No   int    `json:"no"`
	Name string `json:"name"`
}

// DuelSideView 是决斗一方的名单。
type DuelSideView struct {
	Leader  string       `json:"leader"`
	Members []DuelMember `json:"members"`
}

// DuelView 是 battle_start 里的 duel 字段（代替 party）：sides[0] 是发起方，sides[1] 是被挑战方。
type DuelView struct {
	Sides []DuelSideView `json:"sides"`
}

// DuelStart 是决斗时 battle_start 的 data：StartView 加上 duel 字段（kind 为 duel）。
type DuelStart struct {
	StartView
	Duel *DuelView `json:"duel"`
}

// DuelPointView 是 battle_end.duel_point：change 是写回后的变化量，total 是写回后的决斗点数。
type DuelPointView struct {
	Change int `json:"change"`
	Total  int `json:"total"`
}

// DuelRoster 返回开战时双方的名单。
func (b *Battle) DuelRoster() *DuelView {
	v := &DuelView{Sides: make([]DuelSideView, 2)}
	for _, no := range b.DuelHumans() {
		name := b.sides[0].Name
		if no != 0 {
			name = b.mates[no].Side.Name
		}
		s := &v.Sides[no/unitsPerSide]
		if no%unitsPerSide == 0 {
			s.Leader = name
		}
		s.Members = append(s.Members, DuelMember{No: no, Name: name})
	}
	return v
}

// DuelStartFor 生成真人 no 收到的决斗 battle_start（you 为他自己的单位号，对方人物只给血量百分比）。
func (b *Battle) DuelStartFor(no int) DuelStart {
	v := b.StartView()
	v.You = no
	v.Units = b.partyUnits(no / unitsPerSide)
	return DuelStart{StartView: v, Duel: b.DuelRoster()}
}

// ForSide 返回给 side 方观看的 battle_round：对方单位的精确血量和气力裁掉，只留百分比。
// 引擎结算时决斗两方都写完整数值（hpShown），所以每个人收到的回合要先过这里。
func (r Round) ForSide(side int) Round {
	out := Round{BattleID: r.BattleID, Turn: r.Turn, Events: make([]Event, len(r.Events))}
	for i, ev := range r.Events {
		if len(ev.Effects) > 0 {
			ev.Effects = append([]Effect(nil), ev.Effects...)
			for j := range ev.Effects {
				if ev.Effects[j].Target/unitsPerSide != side {
					ev.Effects[j].HP = nil
				}
			}
		}
		if len(ev.Joined) > 0 {
			ev.Joined = append([]UnitView(nil), ev.Joined...)
			for j := range ev.Joined {
				ev.Joined[j] = ev.Joined[j].forSide(side)
			}
		}
		out.Events[i] = ev
	}
	out.State = make([]StateEntry, len(r.State))
	for i, st := range r.State {
		if st.No/unitsPerSide != side {
			st.HP, st.MP = nil, nil
			st.Mount = st.Mount.hideHP()
		}
		out.State[i] = st
	}
	return out
}

// forSide 把不属于 side 方的单位视图裁成只有百分比。
func (v UnitView) forSide(side int) UnitView {
	if v.No/unitsPerSide != side {
		v.HP, v.MaxHP, v.MP, v.MaxMP = nil, nil, nil, nil
		v.Mount = v.Mount.hideHP()
	}
	return v
}

// hideHP 返回去掉精确血量的骑宠视图（不改原值）。
func (m *Mount) hideHP() *Mount {
	if m == nil {
		return nil
	}
	c := *m
	c.HP, c.MaxHP = nil, nil
	return &c
}

// duelEnd 是决斗里真人 no 的 battle_end（在场的人，战场结果以他的方为准）：没有奖励、没有惩罚。
// 决斗点数由 character 服务按写回后的值填进 DuelPoint。
func (b *Battle) duelEnd(no int) *EndView {
	res := b.result
	if no/unitsPerSide == 1 { // b.result 是发起方（0 号方）的角度
		switch res {
		case "win":
			res = "lose"
		case "lose":
			res = "win"
		}
	}
	return &EndView{BattleID: b.ID, Result: res, Turn: b.turn}
}
