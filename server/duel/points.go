// Package duel 是决斗（PK，STAGE-306）的纯规则：决斗点数（DP）的计算与写回。
// 不碰数据库、连接和战斗状态，调用方（战斗引擎）在事件发生时把它们喂进来。
// 规则依据 docs/pk.md 2.2，照原版 1.4 用整数运算。
package duel

// DP 的默认值与范围（pk.md 2.2）。
const (
	DefaultDP = 100
	MinDP     = 0
	MaxDP     = 100000000
)

// Share 是 floor(dp / 10)，负数按 0 算。
func Share(dp int) int {
	if dp <= 0 {
		return 0
	}
	return dp / 10
}

// SplitGain 把 pool 平分给 n 个人，每人至少 1 点：max(1, pool / n)。n 为 0 时返回 0。
func SplitGain(pool, n int) int {
	if n <= 0 {
		return 0
	}
	g := pool / n
	if g < 1 {
		g = 1
	}
	return g
}

// Settle 是结束写回：DP = clamp(dp + cum, 0, 1e8)。
func Settle(dp, cum int) int {
	v := dp + cum
	if v < MinDP {
		return MinDP
	}
	if v > MaxDP {
		return MaxDP
	}
	return v
}

// EscapeImmune 判断这次逃跑（或超时、掉线）是否免罚：第 1 回合、
// 双方都还没有提交过攻击类指令（攻击、精灵、宠物技能、道具），且没人倒下。
func EscapeImmune(firstRound, attackSubmitted, anyKnockedOut bool) bool {
	return firstRound && !attackSubmitted && !anyKnockedOut
}

// Ledger 记录一场决斗里每名人物的本场 DP 累计。开战时 Join 记下开战 DP，
// 之后的倒下、逃跑都按开战 DP 结算（原版同样用 CHAR_DUELPOINT，战斗中不更新）。
// 人物编号由调用方定义（单位号即可）；Ledger 不关心阵营。
type Ledger struct {
	start map[int]int
	cum   map[int]int
}

// NewLedger 返回一份空账本。
func NewLedger() *Ledger {
	return &Ledger{start: map[int]int{}, cum: map[int]int{}}
}

// Join 登记一名人物的开战 DP。
func (l *Ledger) Join(no, startDP int) {
	l.start[no] = startDP
	if _, ok := l.cum[no]; !ok {
		l.cum[no] = 0
	}
}

// Knockout 记录人物 victim 倒下：本场累计减去 Share(开战 DP)；这份按人平分给
// gainers（本次行动的出手者，或误伤时对方活着的人物），每人 SplitGain。
// gainers 里的重复编号只算一次；空列表时只扣不加。
func (l *Ledger) Knockout(victim int, gainers []int) {
	s := Share(l.start[victim])
	l.cum[victim] -= s
	ids := unique(gainers)
	g := SplitGain(s, len(ids))
	for _, id := range ids {
		l.cum[id] += g
	}
}

// Escape 记录人物 who 还活着时逃跑、超时或掉线：本场累计覆盖为 -2 × max(1, Share(开战 DP))，
// 其中 max(1, Share) 平分给 opponents（对方活着的人物）。覆盖（而不是累加）与原版 setWorkInt 一致，
// 因此会抹掉 who 之前在本场拿到的击倒所得。调用方先用 EscapeImmune 判断是否免罚。
func (l *Ledger) Escape(who int, opponents []int) {
	s := Share(l.start[who])
	if s < 1 {
		s = 1
	}
	l.cum[who] = -2 * s
	ids := unique(opponents)
	g := SplitGain(s, len(ids))
	for _, id := range ids {
		l.cum[id] += g
	}
}

// Delta 返回人物本场的累计变化（尚未钳位）。
func (l *Ledger) Delta(no int) int {
	return l.cum[no]
}

// Start 返回人物的开战 DP；未登记时返回 0。
func (l *Ledger) Start(no int) int {
	return l.start[no]
}

// Final 返回人物结束时应写回的 DP。
func (l *Ledger) Final(no int) int {
	return Settle(l.start[no], l.cum[no])
}

func unique(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
