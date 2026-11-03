// 异常状态（STAGE-46）：攻击附毒、回合末的状态结算（递减与解除、中毒扣血）、
// 被打醒和混乱的目标随机。公式出处 gmsv/battle/battle_event.c 的
// BATTLE_StatusAttackCheck、battle.c 的 BATTLE_StatusSeq 与 Compute_Down。

package battle

// poisonRate 是普通攻击附带的毒的基础概率（BATTLE_Attack 的 suitpoison = 30），
// poisonBai 与 poisonRange 是等级差的系数与上下限（BATTLE_StatusAttackCheck 的 Bai = 2.0、Range = 40）。
const (
	poisonRate  = 30
	poisonBai   = 2.0
	poisonRange = 40
)

// poisonAttack 是普通攻击命中后附带的毒（BATTLE_StatusAttackCheck，非双人对战分支）：
// 已经带着任何异常状态就不中（原版一开始就返回 FALSE）；命中率见 statusPer。
// 原版普攻的毒持续 gBattleStausTurn + 1 = 1 回合。
func (b *Battle) poisonAttack(attacker, target *Unit) bool {
	if target.Status.anyBad() {
		return false
	}
	if b.roll(100) >= statusPer(attacker, target, poisonRate) {
		return false
	}
	target.Status.Poison = 1
	return true
}

// statusPer 是普攻附毒与异常攻击的命中率（offset 为 suitpoison = 30，等级差系数 2、限 ±40）。
func statusPer(attacker, target *Unit, offset int) int {
	return statusPerRange(attacker, target, offset, poisonRange, poisonBai)
}

// statusPerRange 是异常状态的命中率（BATTLE_StatusAttackCheck，非双人对战分支）：
// per = offset + 等级差 × bai（限 ±rng）+ 运气 − 体力占比抗性（体力 / 四项和 / 0.25 × 10），
// 结果截成整数，上限 80。状态精灵的偏移是精灵的成功率（“成 N”），bai 1.0、rng 30。
func statusPerRange(attacker, target *Unit, offset, rng int, bai float64) int {
	vital := float64(target.Vital) / float64(target.Vital+target.Strength+target.Toughness+target.Dexterity)
	resist := vital / 0.25 * 10
	level := int(float64(attacker.Level-target.Level) * bai)
	if attacker.pvp && target.pvp { // 决斗：异常状态不计等级差（pk.md 2.3）
		level = 0
	}
	if level > rng {
		level = rng
	}
	if level < -rng {
		level = -rng
	}
	luck := attacker.Luck // 运气：只有人物有，宠物和怪物为 0（STAGE-61）
	per := int(float64(offset+level+luck) - resist)
	if per > 80 {
		per = 80
	}
	return per
}

// statusCheck 是状态精灵的成败（BATTLE_StatusAttackCheck）：已经带着任何异常状态就不中，否则 RAND(1,100) < per。
func (b *Battle) statusCheck(attacker, target *Unit, offset, rng int, bai float64) bool {
	if target.Status.anyBad() {
		return false
	}
	return b.roll(100) < statusPerRange(attacker, target, offset, rng, bai)
}

// set 给一个异常状态设上剩余回合数（协议状态名，同 cure）；名字不认识时不动。
func (s *StatusSet) set(name string, turns int) {
	if f := s.field(name); f != nil {
		*f = turns
	}
}

// field 返回一个状态的剩余回合数字段，key 是协议 10.2 的状态名；引擎还没有的状态（剧毒）返回 nil。
func (s *StatusSet) field(key string) *int {
	switch key {
	case "poison":
		return &s.Poison
	case "paralysis":
		return &s.Paralysis
	case "sleep":
		return &s.Sleep
	case "stone":
		return &s.Stone
	case "drunk":
		return &s.Drunk
	case "confusion":
		return &s.Confusion
	case "weaken":
		return &s.Weaken
	case "barrier":
		return &s.Barrier
	case "nocast":
		return &s.Nocast
	}
	return nil
}

// cureOrder 是原版 BATTLE_MultiStatusRecovery 找“最后一个异常状态”用的顺序（按 aszStatus 的编号）：
// 前六种是“全”也能解的常见异常，后三种（虚、障、默）只有点名时才解（STAGE-201）。
var cureOrder = []string{"poison", "paralysis", "sleep", "stone", "drunk", "confusion", "weaken", "barrier", "nocast"}

// cureCommon 是 cureOrder 里“全”能解的个数（原版“全”只看 tostatus <= CHAR_WORKCONFUSION）。
const cureCommon = 6

// cureMagic 是解状态精灵的结算（BATTLE_MultiStatusRecovery）：每次只解一种。原版按 cureOrder 的顺序
// 找出最后一个有回合数的状态（tostatus），只有精灵是正好是它时才解（“全”只在前六种里找），所以同时中了毒和麻痹的人，
// 用“毒”的精灵解不掉。返回解掉的状态名；没有解掉时为 nil。
func (s *StatusSet) cureMagic(want string) []string {
	last, lastIdx := "", -1
	for i, key := range cureOrder {
		if *s.field(key) > 0 {
			last, lastIdx = key, i
		}
	}
	if last == "" || (want != "all" && want != last) {
		return nil
	}
	if want == "all" && lastIdx >= cureCommon { // 原版“全”：tostatus 超过乱就不解
		return nil
	}
	*s.field(last) = 0
	return []string{last}
}

// anyBad 表示已带着任何异常状态（原版 StatusAttackCheck 一开始就返回 FALSE）。
func (s StatusSet) anyBad() bool {
	return s.Poison > 0 || s.Paralysis > 0 || s.Sleep > 0 || s.Stone > 0 ||
		s.Drunk > 0 || s.Confusion > 0 || s.Weaken > 0 || s.Barrier > 0 || s.Nocast > 0
}

// cure 清掉指定名称的异常状态（STAGE-30 的解状态道具，原版 BATTLE_MultiStatusRecovery：
// status=0“全”解除全部常见异常）；返回被解除状态的协议名，没有对应状态时为空。
func (s *StatusSet) cure(name string) []string {
	fields := []struct {
		val   *int
		key   string
		named bool // 只有点名才解：虚、障、默（STAGE-201，原版“全”不解）
	}{
		{&s.Poison, "poison", false}, {&s.Paralysis, "paralysis", false}, {&s.Sleep, "sleep", false},
		{&s.Stone, "stone", false}, {&s.Drunk, "drunk", false}, {&s.Confusion, "confusion", false},
		{&s.Weaken, "weaken", true}, {&s.Barrier, "barrier", true}, {&s.Nocast, "nocast", true},
	}
	var removed []string
	for _, f := range fields {
		if name != "all" && name != f.key {
			continue
		}
		if name == "all" && f.named {
			continue
		}
		if *f.val > 0 {
			*f.val = 0
			removed = append(removed, f.key)
		}
	}
	return removed
}

// poisonDamage 是中毒一回合的扣血（Compute_Down）：四项能力的点数和减 20 再除 4，
// 至少 1；不会毒死，最多扣到剩 1 血。Unit 里的能力已经是点数。
func poisonDamage(u *Unit) int {
	points := u.Vital + u.Strength + u.Toughness + u.Dexterity
	d := (points - 20) / 4
	if d < 1 {
		d = 1
	}
	if u.HP <= d {
		d = u.HP - 1
	}
	if d < 0 {
		d = 0
	}
	return d
}

// statusTick 是回合末的状态结算（BATTLE_StatusSeq）：每个异常状态回合数减一、
// 归零的解除；中毒在这之前先扣血（Compute_Down）。返回一个 status_tick 事件
// （没有任何状态变化时返回 nil，不出事件）。
func (b *Battle) statusTick() []Event {
	ev := Event{Kind: "status_tick"}
	any := false
	for _, u := range b.units {
		if u == nil || !u.Alive() {
			continue
		}
		if u.Status.Poison > 0 {
			dmg := poisonDamage(u)
			if dmg > 0 {
				any = true
				b.hurt(u, dmg, "hit", &ev)
			}
		}
		b.tickDown(u, &ev, &any)
	}
	if !any {
		return nil
	}
	return []Event{ev}
}

// tickDown 递减一个单位的全部状态；归零的记进事件的 status_remove。
// STAGE-201 的冻结（原版 BATTLE_StatusSeq）：带着虚弱或魔障的单位，回合末每个状态减一后又加回 1，回合数卡在原值，
// 不会自己消失（减到 0 时照样记 status_remove，原版给客户端发的“解除”，内部值不变）。沉默不在冻结之内，正常减一。
func (b *Battle) tickDown(u *Unit, ev *Event, any *bool) {
	var removed []string
	s := &u.Status
	frozen := s.Weaken > 0 || s.Barrier > 0
	for _, f := range []struct {
		val *int
		key string
	}{
		{&s.Poison, "poison"}, {&s.Paralysis, "paralysis"}, {&s.Sleep, "sleep"},
		{&s.Stone, "stone"}, {&s.Drunk, "drunk"}, {&s.Confusion, "confusion"},
		{&s.Weaken, "weaken"}, {&s.Barrier, "barrier"}, {&s.Nocast, "nocast"},
		{&s.ResistMagic, "magic_resist"}, {&s.ResistFire, "fire_resist"},
		{&s.ResistThunder, "thunder_resist"}, {&s.ResistIce, "ice_resist"},
	} {
		if *f.val <= 0 {
			continue
		}
		left := *f.val - 1
		if left == 0 {
			removed = append(removed, f.key)
		}
		if !frozen {
			*f.val = left
		}
	}
	if len(removed) == 0 {
		return
	}
	*any = true
	eff := Effect{Target: u.No, Result: "none", Delta: 0, HPPct: u.HPPct(), Down: !u.Alive(),
		StatusRemove: removed}
	if b.hpShown(u) {
		hp := u.HP
		eff.HP = &hp
	}
	ev.Effects = append(ev.Effects, eff)
}

// wakeUp 是睡眠中的单位被击中后醒来（BATTLE_DamageWakeUp）；返回是否醒了。
func (b *Battle) wakeUp(target *Unit, ev *Event) bool {
	if target.Status.Sleep <= 0 {
		return false
	}
	target.Status.Sleep = 0
	if len(ev.Effects) > 0 {
		eff := &ev.Effects[len(ev.Effects)-1]
		if eff.Target == target.No {
			eff.StatusRemove = append(eff.StatusRemove, "sleep")
		}
	}
	return true
}

// confuseTarget 是混乱中的单位选攻击目标：从场上所有活着的单位里随机挑一个
// （包括自己人，也可能挑到自己，同原版混乱的随机目标）。
func (b *Battle) confuseTarget(u *Unit) *Unit {
	var alive []*Unit
	for _, t := range b.units {
		if t != nil && t.targetable() {
			alive = append(alive, t)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	return alive[b.Rnd(len(alive))]
}
