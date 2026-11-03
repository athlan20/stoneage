// stoneage-questchain 的模型（STAGE-213，docs/quest.md「本批任务能否走完」的第二版）：把任务脚本的块编译成“条件 → 效果”，
// 在只增的事实集合上做单调闭包。选块、判条件、做动作的规则对照 server/character/npcevent*.go，这里只保留可达性需要的部分。
package main

import (
	"slices"
	"sort"

	"github.com/athlan20/stoneage/server/gamedata"
)

// Kind 是事实的种类。
type Kind byte

const (
	KindEnd  Kind = 'e' // 事件号可能完成（ENDEV，EndSetFlg）
	KindNow  Kind = 'n' // 事件号可能进行中（NOWEV，REQUEST 和 MESSAGE 的 event）
	KindItem Kind = 'i' // 道具编号可能拿到（GetItem、GetRandItem）
	KindPet  Kind = 'p' // 宠物模板编号可能拿到（GetPet 的怪物换成模板）
)

// Fact 是一条只增的事实：种类和编号。
type Fact struct {
	Kind Kind
	N    int
}

// Env 是分析要查的游戏数据。正式运行用 gamedata，测试里换成小表。
type Env struct {
	EnemyTemplate func(enemyID int) (int, bool) // GetPet 的怪物编号 → 宠物模板
	PetCatchable  func(template int) bool       // 可捕捉的野生宠物（宠物表 PetFlag > 0），视为随时可得
	ItemName      func(id int) string
	PetName       func(template int) string
}

// term 是条件里的一项。alts 里任一个可用即满足（空表示不需要事实）；lv 是对等级的要求；bad 表示永远不满足
// （未知的条件、缺宠物模板的 PET）。
type term struct {
	alts []Fact
	lv   int
	bad  bool
}

// Unit 是一个块：脚本文件、块序号（文件里的顺序），以及编译好的条件和效果。
type Unit struct {
	ID      int
	File    string
	Index   int
	Block   *gamedata.EventBlock
	groups  [][]term // 条件：外层是或（候选组），内层是与
	pre     []Fact   // 交易的前提：DelPet 直接写的宠物（缺了整笔不做）
	produce []Fact   // 执行后得到的事实（没有台词的 MESSAGE、没有感谢话的 ACCEPT / REQUEST 不产生任何效果）
}

// Model 是一次分析的输入：去重后的脚本文件、块（按文件和块序号排列），以及任务表。
type Model struct {
	Files  []string // 去重后的脚本文件（升序）
	Units  []*Unit
	Quests []gamedata.Quest
	env    Env
	givers map[int]bool // 任何脚本会给的道具：不在其中的道具视为外部补给（商店、战斗掉落）
	// 事件号的外部来源：条件里引用了（ENDEV= 或 NOWEV=），但任何任务脚本都不设置的事件，由别的系统（C 代码里的
	// 宠物转生、精灵少女转生等，见 sources.go）设置。这些事件按外部可得处理，报告里单独列出。
	extEnd map[int]bool
	extNow map[int]bool
	// setEnd 是任何任务脚本设置过的完成事件号（EndSetFlg）。任务的完成事件不在里面，说明它不由任务脚本完成
	// （比如金飞航空、水怪这些是别的 NPC 类型完成的），这种任务的 ✗ 不说明脚本有问题，报告里单独列出。
	setEnd map[int]bool
}

// NewModel 把脚本编译成模型。同一个脚本文件挂在几个任务 NPC 上时块完全相同（已检查），只取第一份。
func NewModel(scripts []*gamedata.EventScript, quests []gamedata.Quest, env Env) *Model {
	m := &Model{Quests: quests, env: env, givers: map[int]bool{}, extEnd: map[int]bool{}, extNow: map[int]bool{}}
	byFile := map[string]*gamedata.EventScript{}
	for _, s := range scripts {
		if _, ok := byFile[s.File]; !ok {
			byFile[s.File] = s
			m.Files = append(m.Files, s.File)
		}
	}
	sort.Strings(m.Files)
	setEnd, setNow := map[int]bool{}, map[int]bool{}
	for _, f := range m.Files {
		s := byFile[f]
		for i := range s.Blocks {
			blk := &s.Blocks[i]
			for _, g := range blk.GetItem {
				m.givers[g.ID] = true
			}
			for _, id := range blk.GetRand {
				m.givers[id] = true
			}
			for _, n := range blk.EndSet {
				setEnd[n] = true
			}
			if blk.Event != -1 && (blk.Type == "REQUEST" || blk.Type == "MESSAGE") {
				setNow[blk.Event] = true
			}
			if keepsDoing(blk) {
				setNow[blk.Event] = true
			}
			for _, group := range blk.Cond {
				for _, t := range group {
					switch {
					case t.Op == "ENDEV" && t.Cmp == "=":
						m.extEnd[t.N] = true
					case t.Op == "NOWEV" && t.Cmp == "=":
						m.extNow[t.N] = true
					}
				}
			}
		}
	}
	for n := range setEnd {
		delete(m.extEnd, n)
	}
	for n := range setNow {
		delete(m.extNow, n)
	}
	m.setEnd = setEnd
	for _, f := range m.Files {
		s := byFile[f]
		for i := range s.Blocks {
			u := &Unit{ID: len(m.Units), File: f, Index: i, Block: &s.Blocks[i]}
			m.compile(u)
			m.Units = append(m.Units, u)
		}
	}
	return m
}

// compile 把块的条件和效果编译成事实。支持的块没有条件时和运行时一样永远不命中（eventMatchedGroup 没有候选组）；
// 不支持的块如果条件没解析出来（数据里只有这种情况），视为一个不需要事实的候选组，只在场景 C 里有意义。
func (m *Model) compile(u *Unit) {
	blk := u.Block
	if len(blk.Cond) == 0 && blk.Unsupported != "" {
		u.groups = [][]term{nil}
	}
	for _, group := range blk.Cond {
		terms := make([]term, 0, len(group))
		for _, t := range group {
			terms = append(terms, compileTerm(t))
		}
		u.groups = append(u.groups, terms)
	}
	for _, d := range blk.DelPet {
		u.pre = append(u.pre, Fact{Kind: KindPet, N: d.Template})
	}
	u.produce = m.effects(blk)
}

// compileTerm 把一个条件转成一项。ITEM 数量、宠物等级和数量不看（单调近似）；ITEM!=、ENDEV!=、NOWEV!=、TIME!= 视为可满足。
func compileTerm(t gamedata.EventTerm) term {
	switch t.Op {
	case "ITEM":
		if t.Cmp == "!=" {
			return term{}
		}
		return term{alts: []Fact{{Kind: KindItem, N: t.ID}}}
	case "LV":
		return term{lv: levelReq(t.Cmp, t.N)}
	case "ENDEV":
		if t.Cmp == "=" {
			return term{alts: []Fact{{Kind: KindEnd, N: t.N}}}
		}
		return term{}
	case "NOWEV":
		if t.Cmp == "=" { // 进行中，或者已经完成过（进行中的块要在接任务之后才出现，这里近似）
			return term{alts: []Fact{{Kind: KindNow, N: t.N}, {Kind: KindEnd, N: t.N}}}
		}
		return term{}
	case "TIME":
		return term{}
	case "PET":
		if t.Pet == nil {
			return term{bad: true}
		}
		return term{alts: []Fact{{Kind: KindPet, N: t.Pet.Template}}}
	}
	return term{bad: true}
}

// levelReq 是 LV 条件对角色等级的最低要求：LV>n 要 n+1 级，LV=n 要 n 级，LV<n 从 1 级起就能满足。
func levelReq(cmp string, n int) int {
	switch cmp {
	case "=":
		return n
	case ">":
		return n + 1
	}
	return 0
}

// effects 是块执行（交易成功、有台词）之后得到的事实。MESSAGE 和 REQUEST 的进行中（event 不是 -1）、EndSetFlg、
// 给的道具和宠物都算；CLEAN 只清进度，不算。
func (m *Model) effects(blk *gamedata.EventBlock) []Fact {
	switch blk.Type {
	case "MESSAGE":
		if len(blk.Msg.Pages("nomal")) == 0 && len(blk.Msg.Pages("nomal_window")) == 0 {
			return nil
		}
	case "ACCEPT", "REQUEST":
		if len(blk.Msg.Pages("thanks")) == 0 {
			return nil
		}
	default:
		return nil
	}
	var out []Fact
	if ((blk.Type == "MESSAGE" || blk.Type == "REQUEST") && blk.Event != -1) || keepsDoing(blk) {
		out = append(out, Fact{Kind: KindNow, N: blk.Event})
	}
	for _, n := range blk.EndSet {
		out = append(out, Fact{Kind: KindEnd, N: n})
	}
	for _, g := range blk.GetItem {
		out = append(out, Fact{Kind: KindItem, N: g.ID})
	}
	for _, id := range blk.GetRand {
		out = append(out, Fact{Kind: KindItem, N: id})
	}
	for _, enemy := range blk.GetPet {
		if m.env.EnemyTemplate == nil {
			break
		}
		if t, ok := m.env.EnemyTemplate(enemy); ok {
			out = append(out, Fact{Kind: KindPet, N: t})
		}
	}
	return out
}

// external 报告事实是不是外部可得（不需要任何脚本）：没有脚本会给的道具（商店、战斗掉落），可捕捉的野生宠物，
// 以及条件里引用但没有脚本设置的事件（由别的系统设，见 NewModel 的 extEnd、extNow）。
func (m *Model) external(f Fact) bool {
	switch f.Kind {
	case KindItem:
		return !m.givers[f.N]
	case KindPet:
		return m.env.PetCatchable != nil && m.env.PetCatchable(f.N)
	case KindEnd:
		return m.extEnd[f.N]
	case KindNow:
		return m.extNow[f.N]
	}
	return false
}

// keepsDoing 是 STAGE-341 的 keepDoing（运行时 npcevent_flags.go）：ACCEPT 的事件号不在它的 EndSetFlg 里时，
// 交付后事件号进入进行中。全库只有 poru/mary04、mary05、mary06 三块是这个形状。
func keepsDoing(blk *gamedata.EventBlock) bool {
	return blk.Type == "ACCEPT" && blk.Event != -1 && len(blk.EndSet) > 0 && !slices.Contains(blk.EndSet, blk.Event)
}
