// 战斗推送的视图（docs/protocol.md 10.3、10.4、10.6、10.11）：battle_start、
// battle_turn、battle_progress 和 battle 查询的内容。同一份内容由 character 服务
// 原样编码成 JSON 推给玩家的每条连接。

package battle

// StartView 是 battle_start 的 data。
type StartView struct {
	BattleID string     `json:"battle_id"`
	Kind     string     `json:"kind"`
	Scene    string     `json:"scene"`
	You      int        `json:"you"`
	Allow    AllowView  `json:"allow"`
	Units    []UnitView `json:"units"`
	NPC      *NPCStart  `json:"npc,omitempty"`   // NPC 发起的战斗（STAGE-96）
	Field    *FieldView `json:"field,omitempty"` // 当前场地，无属性时省略（STAGE-200）
}

// AllowView 是本场战斗允许的指令。
type AllowView struct {
	Escape  bool `json:"escape"`
	Capture bool `json:"capture"`
}

// TurnView 是 battle_turn 的 data。
type TurnView struct {
	BattleID     string       `json:"battle_id"`
	Turn         int          `json:"turn"`
	DeadlineMS   int64        `json:"deadline_ms"`
	ServerTimeMS int64        `json:"server_time_ms"`
	Needs        []string     `json:"needs"`
	Optional     []string     `json:"optional,omitempty"` // 可以指定目标的分身（STAGE-186），不进 needs
	Progress     ProgressView `json:"progress"`
	Field        *FieldView   `json:"field,omitempty"` // 当前场地，无属性时省略（STAGE-200）
}

// ProgressView 是 battle_turn 和 battle_progress 里的交指令进度。
type ProgressView struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// View 是 battle 查询（docs/protocol.md 10.11）和 enter_world 里 battle 字段的内容。
type View struct {
	InBattle bool `json:"in_battle"`
	*QueryCore
}

// QueryCore 是战斗现状的公共部分。
type QueryCore struct {
	BattleID     string          `json:"battle_id"`
	Kind         string          `json:"kind"`
	Scene        string          `json:"scene"`
	You          int             `json:"you"`
	Allow        AllowView       `json:"allow"`
	Units        []UnitView      `json:"units"`
	Turn         int             `json:"turn"`
	Phase        string          `json:"phase"`
	DeadlineMS   int64           `json:"deadline_ms"`
	ServerTimeMS int64           `json:"server_time_ms"`
	Needs        []string        `json:"needs,omitempty"`
	Optional     []string        `json:"optional,omitempty"`
	Submitted    map[string]bool `json:"submitted,omitempty"`
	Progress     ProgressView    `json:"progress,omitempty"`
	NPC          *NPCRef         `json:"npc,omitempty"`   // NPC 发起的战斗（STAGE-96，不含台词）
	Field        *FieldView      `json:"field,omitempty"` // 当前场地，无属性时省略（STAGE-200）
}

// StartView 生成战斗开始的内容。
func (b *Battle) StartView() StartView {
	v := StartView{
		BattleID: b.ID, Kind: b.Kind, Scene: b.Scene, You: b.You,
		Allow: AllowView{Escape: b.AllowEscape, Capture: b.AllowCapture},
	}
	for _, u := range b.units {
		if u != nil {
			v.Units = append(v.Units, u.view(b.You/unitsPerSide, b.You))
		}
	}
	if b.NPC != nil {
		v.NPC = &NPCStart{NPCRef: b.NPC.NPCRef, Say: b.NPC.Start}
	}
	v.Field = b.fieldView()
	return v
}

// TurnView 生成回合开始的内容。deadline 由 character 服务算好传入（它知道真实时间）。
func (b *Battle) TurnView(deadlineMS, serverTimeMS int64) TurnView {
	return TurnView{
		BattleID: b.ID, Turn: b.turn, DeadlineMS: deadlineMS, ServerTimeMS: serverTimeMS,
		Needs: b.needs, Optional: b.Optional(), Progress: ProgressView{Done: 0, Total: 1},
		Field: b.fieldView(),
	}
}

// ProgressDone 生成有人交完指令的推送内容（一期单玩家，交完即 done 1/1）。
func (b *Battle) ProgressDone() map[string]any {
	return map[string]any{"battle_id": b.ID, "turn": b.turn, "done": 1, "total": 1}
}

// QueryView 生成 battle 查询和 enter_world 快照的内容（phase：command 或 replay）。
func (b *Battle) QueryView(deadlineMS, serverTimeMS int64) View {
	core := &QueryCore{
		BattleID: b.ID, Kind: b.Kind, Scene: b.Scene, You: b.You,
		Allow: AllowView{Escape: b.AllowEscape, Capture: b.AllowCapture},
		Turn:  b.turn, DeadlineMS: deadlineMS, ServerTimeMS: serverTimeMS,
		Progress: ProgressView{Done: 0, Total: 1},
	}
	for _, u := range b.units {
		if u != nil {
			core.Units = append(core.Units, u.view(b.You/unitsPerSide, b.You))
		}
	}
	if b.phase == PhaseCommand {
		core.Phase = "command"
		core.Needs = b.needs
		core.Optional = b.Optional()
		core.Submitted = b.Submitted()
	} else {
		core.Phase = "replay"
	}
	if b.NPC != nil {
		core.NPC = &b.NPC.NPCRef
	}
	core.Field = b.fieldView()
	return View{InBattle: true, QueryCore: core}
}
