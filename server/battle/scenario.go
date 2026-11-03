// 测试剧本（docs/protocol.md 10.13）：协议用例的 admin 行 start_battle 用它们开战。
// 每个剧本固定怪物的组合、等级和随机数种子，让用例重放时结果可复现；
// “导演”干预（预设怪物的行动）随 STAGE-44 起按需添加，逐步退成只给种子。

package battle

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
)

// MonsterSpec 是剧本里的一只怪物。
type MonsterSpec struct {
	TemplateID int
	Level      int
	Paralysis  int // 开局就麻痹的回合数（battle_events 用例的展示）
	Power      int // 攻击乘上的倍数（0 表示不改；fly 用例让怪物一下打飞人物，STAGE-63）
	Drop       int // 出生时必掉的物品编号（0 为不掉，battle_drop 用例；原版按概率摇，这里固定）
}

// Scenario 是一个测试剧本。
type Scenario struct {
	AllowEscape  bool
	AllowCapture bool
	PlayerPoison bool // 玩家开局中毒（battle_events 用例）
	PetDown      bool // 玩家的出战宠物开局倒下（battle_item_revive 用例，STAGE-262）
	FleeTurn     int  // 怪物在这一回合全部逃走（展示 draw 的用例用，0 表示不逃）
	Combo        bool // 打开合击（STAGE-126）；默认关，已校准的协议用例不受合击的随机数影响
	Monsters     []MonsterSpec
	Seed         uint64
	// NPCFile 非空时这场是该战斗 NPC（参数文件，如 king/event69_5）发起的战斗（STAGE-96）：NPC 与台词取自参数文件，
	// 怪物仍用剧本的，让协议用例能稳定地打赢。
	NPCFile string
}

// scenarios 是已知的剧本；名字来自 docs/protocol/cases/battle_*.jsonl 的 scenario 字段。
// 一期的角色是新建立的 1 级人物 + give_pet 给的宠物，见习怪用乌力（模板 1）和布比（模板 11），
// 等级按“新手打得过 / 打不过”挑：win 的怪 1 级，lose / no_escape 的怪 6–8 级。
var scenarios = map[string]*Scenario{
	"win":       {AllowEscape: true, AllowCapture: true, Monsters: twoBoars(1), Seed: 43},
	"lose":      {AllowEscape: true, AllowCapture: true, Monsters: twoBoars(6), Seed: 44},
	"escape":    {AllowEscape: true, AllowCapture: true, Monsters: twoBoars(3), Seed: 45},
	"no_escape": {AllowEscape: false, AllowCapture: true, Monsters: twoBoars(8), Seed: 46},
	// 合击（STAGE-126）：与 win 同样的怪物，打开合击。种子按“第一回合宠物和人物起头判定通过、怪物不合击”挑。
	"combo": {AllowEscape: true, AllowCapture: true, Combo: true, Monsters: twoBoars(1), Seed: 43},
	"swap":  {AllowEscape: true, AllowCapture: true, Monsters: twoBoars(1), Seed: 47},
	// 与 win 完全相同的一场（同样的种子与怪物），只是两只怪出生时各掉一把小棍棒：用来验证战利品进背包与背包满的丢弃。
	"drop": {AllowEscape: true, AllowCapture: true, Seed: 43, Monsters: []MonsterSpec{{TemplateID: 1, Level: 1, Drop: 100}, {TemplateID: 1, Level: 1, Drop: 100}}},
	"capture": {
		AllowEscape: true, AllowCapture: true, Seed: 52,
		// 两只 1 级乌力：血少好打残（捕捉概率按 HP²/MaxHP 算，满血是负的）。
		Monsters: []MonsterSpec{{TemplateID: 1, Level: 1}, {TemplateID: 1, Level: 1}},
	},
	// 一击打飞（STAGE-63）：两只 1 级乌力的攻击放大到能一下打飞 1 级人物（魅力比普通被打倒多扣）。
	"fly": {
		AllowEscape: true, AllowCapture: true, Seed: 48,
		Monsters: []MonsterSpec{{TemplateID: 1, Level: 6, Power: 8}, {TemplateID: 1, Level: 6, Power: 8}},
	},
	// 打飞的遇敌测试专用（STAGE-291 批 B）：怪物计划改为回合开始定下后，fly 的种子 48 走向变了（人物不再被打飞），
	// 换一个让人物被一下打飞的种子；fly 本身不动，协议用例 battle_fly 仍用原种子。
	"fly_defeat": {
		AllowEscape: true, AllowCapture: true, Seed: 43,
		Monsters: []MonsterSpec{{TemplateID: 1, Level: 6, Power: 8}, {TemplateID: 1, Level: 6, Power: 8}},
	},
	// 宠物倒下的测试专用（STAGE-291 批 B）：怪物计划改为回合开始定下后，win 的种子 43 走向变了（出战宠物不再倒下），
	// 换一个让出战宠物在胜利战里倒下的种子；win 本身不动。
	"win_faint": {AllowEscape: true, AllowCapture: true, Seed: 48, Monsters: twoBoars(1)},
	"showcase": {
		AllowEscape: true, AllowCapture: true, PlayerPoison: true, Seed: 46,
		FleeTurn: 2, // 第二回合怪物全部逃走：展示 draw
		Monsters: []MonsterSpec{{TemplateID: 1, Level: 1}, {TemplateID: 1, Level: 1},
			{TemplateID: 1, Level: 3, Paralysis: 1}},
	},
	//挨一顿就收场：第一回合怪物打人（人物掉血，宠物防御），第二回合全体逃走按 draw 结束。
	//掉血保留（原版 draw 不清血量），给 item_use 用例造“残血的人物”（STAGE-30）。
	"beaten": {AllowEscape: true, AllowCapture: true, FleeTurn: 2, Monsters: twoBoars(3), Seed: 48},
	//带毒开场：与 beaten 一样第二回合全体逃走，玩家开局中毒（battle_item 用例先解状态再回血）。
	"poisoned": {AllowEscape: true, AllowCapture: true, PlayerPoison: true, FleeTurn: 2, Monsters: twoBoars(3), Seed: 49},
	// 宠物开局倒下（STAGE-262）：第一回合用复活药救宠物，第二回合全体逃走按 draw 收场（battle_item_revive 用例）。
	"pet_down": {AllowEscape: true, AllowCapture: true, PetDown: true, FleeTurn: 2, Monsters: twoBoars(3), Seed: 49},
	// 里昂蛙王的一场（STAGE-96）：真正的阵容 90 级，1 级人物打不赢，所以怪物换成与 win 相同的两只 1 级的，
	// 种子也相同，协议用例重放一定是胜利（battle_end 的结束台词、传送到 30607）。
	"npc_win": {AllowEscape: true, AllowCapture: true, NPCFile: "king/event69_5", Seed: 43, Monsters: twoBoars(1)},
}

func twoBoars(level int) []MonsterSpec {
	return []MonsterSpec{{TemplateID: 1, Level: level}, {TemplateID: 1, Level: level}}
}

// Lookup 按名字取剧本；不认识的名字返回错误。
// 名字可以带 "@种子"（如 "beaten@40"）：用这个种子替换剧本自带的种子。怪物计划改了之后，同一个种子的走向会变，
// 协议用例要固定住某个走向时，在用例里写明种子，不用为每个用例新增剧本（STAGE-291 批 B）。
func Lookup(name string) (*Scenario, error) {
	base, seedText, hasSeed := strings.Cut(name, "@")
	sc, ok := scenarios[base]
	if !ok {
		return nil, fmt.Errorf("没有叫 %q 的战斗剧本", name)
	}
	cp := *sc
	if hasSeed {
		seed, err := strconv.ParseUint(seedText, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("战斗剧本 %q 的种子不是数字", name)
		}
		cp.Seed = seed
	}
	return &cp, nil
}

// Monsters 按剧本生成怪物单位（占对方的格子，号码由 Battle 分配）。
func (sc *Scenario) makeMonsters(rnd func(n int) int) ([]*Unit, error) {
	out := make([]*Unit, 0, len(sc.Monsters))
	for _, m := range sc.Monsters {
		t, ok := gamedata.PetByID(m.TemplateID)
		if !ok {
			return nil, fmt.Errorf("剧本里的怪物编号 %d 不在宠物模板表里", m.TemplateID)
		}
		u := NewMonster(t, m.Level, -1, rnd, nil)
		u.Status.Paralysis = m.Paralysis
		if m.Power > 0 {
			u.Attack *= m.Power
		}
		if m.Drop > 0 {
			u.Drops = append(u.Drops, m.Drop)
		}
		out = append(out, u)
	}
	return out, nil
}

// NewRand 用剧本的种子造一个随机源（math/rand/v2 的 PCG；用例重放时结果可复现）。
func (sc *Scenario) NewRand() func(n int) int {
	r := rand.New(rand.NewPCG(sc.Seed, sc.Seed*2+1))
	return r.IntN
}

// Build 按剧本开一场战斗（协议用例与本地测试的入口）。
func Build(id string, side *SideInput, name string) (*Battle, error) {
	sc, err := Lookup(name)
	if err != nil {
		return nil, err
	}
	return sc.Build(id, side)
}

// Build 用这个剧本开一场战斗。
func (sc *Scenario) Build(id string, side *SideInput) (*Battle, error) {
	rnd := sc.NewRand()
	monsters, err := sc.makeMonsters(rnd)
	if err != nil {
		return nil, err
	}
	b := New(id, "野外", side, monsters, rnd)
	b.AllowEscape = sc.AllowEscape
	b.AllowCapture = sc.AllowCapture
	if sc.PlayerPoison {
		b.PlayerUnit().Status.Poison = 9 // 开局带毒，撑到用例打完两个回合
	}
	if sc.PetDown {
		if pet := b.PetUnit(); pet != nil {
			pet.HP, pet.Down = 0, true
		}
	}
	b.FleeTurn = sc.FleeTurn
	b.Combo = sc.Combo
	return b, nil
}
