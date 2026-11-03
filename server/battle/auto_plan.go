// 分身按队长的自动战斗方案出招（STAGE-195，docs/protocol.md 5.57）：队长组队时带来的方案只留服务端用得上的字段，
// 挂在战斗上（SetAutoPlan）。方案的决策顺序对照客户端 client/src/app/battle/auto.rs：
// 自动捕捉回避 → 人物补血 → 首回合 / 常规动作，每一步都可能因为“来源没有、目标不合法”而跳过，最后退回普通攻击。
// 没有方案（plan 为 nil）时 planAuto 走原来的普通攻击，行为不变。

package battle

import "strings"

// AutoPlan 是一份自动战斗方案（服务端只认这里的字段；出招延迟、逃跑、换宠、播放设置等客户端自用的字段不带过来）。
type AutoPlan struct {
	Target  string // "combo"（缺省，合击顺序）或 "random"（随机一个活着的敌人）
	Player  PlanAct
	Pet     PlanAct
	First   PlanFirst
	Recover PlanRecover
	Capture PlanCapture
}

// PlanAct 是一个常规动作。人物的 Act 是 attack / magic / guard，战宠是 attack / skill / guard；
// Slot 是人物精灵栏位 0–4 或战宠技能栏位 1–7；Name 是保存时的名字快照，用来认出“栏位已经换成别的精灵/技能”。
type PlanAct struct {
	Act  string
	Slot int
	Name string
}

// PlanFirst 是首回合策略：只在第 1 回合用一次（Enabled 为真时）。
type PlanFirst struct {
	Enabled bool
	Player  PlanAct
	Pet     PlanAct
}

// PlanRecover 是战斗内恢复的门槛（百分比，≤ 门槛时触发）。分身没有背包也没有补气来源，宠物的喂药也用不上，
// 这些字段只为解析完整保留。
type PlanRecover struct {
	PlayerHeal    bool
	PlayerHealPct int
	PetHeal       bool
	PetHealPct    int
	PlayerMP      bool
	PlayerMPPct   int
}

// PlanCapture 是自动捕捉的筛选：有符合条件的敌方怪物时，分身一律防御，不冒险打死候选。
type PlanCapture struct {
	Enabled  bool
	LevelMin int
	LevelMax int
	Names    []string
	Keep     *CaptureKeep // 资质筛选（STAGE-264）；分身目前不自己捕捉，这里只存着，见 docs/protocol.md 5.57
}

// SetAutoPlan 挂上队长的方案（nil 表示没有方案，分身走默认的普通攻击）。在 AddClones 之后、开战推送之前调用。
func (b *Battle) SetAutoPlan(p *AutoPlan) { b.plan = p }

// planFromProfile 按方案给分身单位（人物或它的宠物）定这回合的指令。用不了的动作一律退回普通攻击，
// 没有敌人时防御，所以总会返回一条指令。
func (b *Battle) planFromProfile(u *Unit) *Cmd {
	if b.captureWanted() { // 4.1：有候选就所有分身和分身的宠物防御，不出手
		return guardCmd()
	}
	first := b.turn <= 1 && b.plan.First.Enabled
	if u.Kind == KindPet {
		act := b.plan.Pet
		if first {
			act = b.plan.First.Pet
		}
		return b.petCmd(u, act)
	}
	if b.plan.Recover.PlayerHeal && u.HP*100 <= u.MaxHP*b.plan.Recover.PlayerHealPct {
		if c := b.healCmd(u); c != nil { // 4.2.2：补血找不到精灵或 MP 不够就往下走，不卡回合
			return c
		}
	}
	act := b.plan.Player
	if first {
		act = b.plan.First.Player
	}
	return b.playerCmd(u, act)
}

// captureWanted 判断本回合有没有符合方案捕捉条件的敌方怪物（4.1）：活着、名字符合、等级在 [LevelMin, LevelMax]
// 之内（0 表示不限），且不超过队长等级 +5（服务端的抓宠规则）。
func (b *Battle) captureWanted() bool {
	c := b.plan.Capture
	if !c.Enabled || !b.AllowCapture {
		return false
	}
	mine := b.sides[0].Level
	side := b.playerSide()
	for _, u := range b.units {
		if u == nil || u.Kind != KindMonster || !u.Alive() || u.Side() == side {
			continue
		}
		if !nameIn(c.Names, u.Name) {
			continue
		}
		if c.LevelMin != 0 && u.Level < c.LevelMin {
			continue
		}
		if c.LevelMax != 0 && u.Level > c.LevelMax {
			continue
		}
		if u.Level <= mine+5 {
			return true
		}
	}
	return false
}

// nameIn 是包含匹配：名单为空表示不限，否则名字包含名单里任一项即可。
func nameIn(names []string, name string) bool {
	if len(names) == 0 {
		return true
	}
	for _, n := range names {
		if strings.Contains(name, n) {
			return true
		}
	}
	return false
}

// guardCmd 是防御指令（不需要目标）。
func guardCmd() *Cmd { return &Cmd{Action: "guard", Target: -1} }
