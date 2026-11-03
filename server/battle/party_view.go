// 真人同场战斗的推送内容（STAGE-275，docs/protocol.md 10.14）：battle_start（每人一条，you 不同，带 party.members）、
// battle_turn（每人的 needs 不同，deadline_ms 为 null）、battle_progress（带 120 秒截止）、battle_leave，
// 以及 battle 查询的每人视图。己方真人（队长和队员）都带 hp、max_hp、mp、max_mp。
package battle

// PartyMember 是 battle_start.party.members 的一项。
type PartyMember struct {
	No     int    `json:"no"`
	Name   string `json:"name"`
	Leader bool   `json:"leader"`
}

// PartyView 是 battle_start 里的 party 字段。
type PartyView struct {
	Members []PartyMember `json:"members"`
}

// PartyStart 是真人同场时 battle_start 的 data：StartView 加上 party 字段（kind 为 party）。
type PartyStart struct {
	StartView
	Party *PartyView `json:"party,omitempty"`
}

// PartyTurn 是真人同场时 battle_turn 的 data：deadline_ms 为 null（计时见 battle_progress）。
type PartyTurn struct {
	BattleID     string       `json:"battle_id"`
	Turn         int          `json:"turn"`
	DeadlineMS   *int64       `json:"deadline_ms"`
	ServerTimeMS int64        `json:"server_time_ms"`
	Needs        []string     `json:"needs"`
	Progress     ProgressView `json:"progress"`
	Field        *FieldView   `json:"field,omitempty"`
}

// PartyProgress 是真人同场时 battle_progress 的 data。第一条指令被接受时带截止时间（deadline_ms、server_time_ms）。
type PartyProgress struct {
	BattleID     string `json:"battle_id"`
	Turn         int    `json:"turn"`
	Done         int    `json:"done"`
	Total        int    `json:"total"`
	DeadlineMS   *int64 `json:"deadline_ms,omitempty"`
	ServerTimeMS *int64 `json:"server_time_ms,omitempty"`
}

// LeaveView 是 battle_leave 的 data（只推给仍在战斗中的真人，逃跑不推，见 10.14）。
type LeaveView struct {
	BattleID string       `json:"battle_id"`
	No       int          `json:"no"`
	Name     string       `json:"name"`
	Cause    string       `json:"cause"`
	Left     []int        `json:"left"`
	Progress ProgressView `json:"progress"`
}

// RosterView 返回开战时的真人名单（队长在前，队员按号码）。
func (b *Battle) RosterView() *PartyView {
	v := &PartyView{Members: []PartyMember{{No: 0, Name: b.sides[0].Name, Leader: true}}}
	for no := 1; no <= partyMaxMates; no++ {
		if m := b.mates[no]; m != nil {
			v.Members = append(v.Members, PartyMember{No: no, Name: m.Side.Name})
		}
	}
	return v
}

// partyUnitView 是真人同场时的单位视图：己方真人都带 mp、max_mp（10.14 的队友）。
func (b *Battle) partyUnitView(u *Unit, mySide int) UnitView {
	v := u.view(mySide, -1)
	if u.Side() == mySide && u.Kind == KindPlayer {
		mp, maxMP := u.MP, u.MaxMP
		v.MP, v.MaxMP = &mp, &maxMP
	}
	return v
}

// partyUnits 返回场上所有单位的视图（真人同场）。
func (b *Battle) partyUnits(mySide int) []UnitView {
	var out []UnitView
	for _, u := range b.units {
		if u != nil {
			out = append(out, b.partyUnitView(u, mySide))
		}
	}
	return out
}

// stateOf 是 battle_round.state 里一个单位的内容。真人同场时己方真人都带 mp，所以各人看到的 state 相同。
func (b *Battle) stateOf(u *Unit) StateEntry {
	if !b.party {
		return u.stateEntry(0, b.You)
	}
	if b.duel != nil { // 决斗：两方都带完整数值，推送时按观看者的方裁剪（Round.ForSide，duel_view.go）
		e := u.stateEntry(u.Side(), -1)
		if u.Kind == KindPlayer {
			mp := u.MP
			e.MP = &mp
		}
		return e
	}
	e := u.stateEntry(0, -1)
	if u.Side() == 0 && u.Kind == KindPlayer {
		mp := u.MP
		e.MP = &mp
	}
	return e
}

// StartViewFor 生成真人 no 收到的 battle_start（you 为他自己的单位号）。
func (b *Battle) StartViewFor(no int) PartyStart {
	v := b.StartView()
	v.You = no
	v.Units = b.partyUnits(no / unitsPerSide)
	return PartyStart{StartView: v, Party: b.RosterView()}
}

// TurnFor 生成真人 no 收到的 battle_turn：needs 是他自己的，progress 是全队的。
func (b *Battle) TurnFor(no int, serverTimeMS int64) PartyTurn {
	needs := append([]string(nil), b.needsOf(no)...)
	return PartyTurn{
		BattleID: b.ID, Turn: b.turn, ServerTimeMS: serverTimeMS,
		Needs: needs, Progress: b.progress(), Field: b.fieldView(),
	}
}

// ProgressPush 生成 battle_progress 的内容。本回合已经有人交过指令（截止时间已定）时带 deadline_ms。
func (b *Battle) ProgressPush(serverTimeMS int64) PartyProgress {
	pv := b.progress()
	p := PartyProgress{BattleID: b.ID, Turn: b.turn, Done: pv.Done, Total: pv.Total}
	if !b.deadline.IsZero() {
		dl := b.deadline.UnixMilli()
		p.DeadlineMS = &dl
		p.ServerTimeMS = &serverTimeMS
	}
	return p
}

// LeaveFor 生成 battle_leave 的内容（离场之后的进度以 Departure.Progress 为准）。
func (b *Battle) LeaveFor(d Departure) LeaveView {
	return LeaveView{BattleID: b.ID, No: d.No, Name: d.Name, Cause: d.Cause, Left: d.Left, Progress: d.Progress}
}

// SubmittedFor 返回真人 no 本回合两个身份是否已经交过指令（查询用）。
func (b *Battle) SubmittedFor(no int) map[string]bool {
	out := map[string]bool{}
	for _, actor := range []string{"player", "pet"} {
		out[actor] = b.cmdOfHuman(no, actor) != nil
	}
	return out
}

// QueryFor 生成真人 no 的 battle 查询内容（真人同场）：needs、submitted 按他自己算，units 带全队的 mp。
// 截止时间由调用方传入（与 QueryView 同），本回合还没人交过时为 0。
func (b *Battle) QueryFor(no int, deadlineMS, serverTimeMS int64) View {
	v := b.QueryView(deadlineMS, serverTimeMS)
	v.You = no
	v.Units = b.partyUnits(no / unitsPerSide)
	v.Progress = b.progress()
	if b.phase == PhaseCommand {
		v.Needs = append([]string(nil), b.needsOf(no)...)
		v.Optional = nil
		v.Submitted = b.SubmittedFor(no)
	}
	return v
}
