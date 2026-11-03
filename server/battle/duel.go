// 决斗的双真人战斗（STAGE-306，docs/pk.md 2.1–2.4、4.2，docs/protocol.md 5.74 与 10.14）：
// 发起方的真人占单位 0–4（宠物 +5），被挑战方的真人占 10–14（宠物 +5），两边都是真人，没有怪物。
// 指令、截止、离场、掉线的机制全部复用真人同场（party.go 顶部的调用顺序不变）；差异：
//   - Battle.duel 非 nil；逃跑必定成功、异常状态不计等级差、复活对人物无效、没有奖励和惩罚（duel_rules.go）；
//   - 倒下、逃跑、超时、掉线按 server/duel 的 Ledger 记决斗点数（DP），结束后用 DuelDelta 读出；
//   - 推送按观看者的方裁剪：对方人物只给血量百分比（duel_view.go）。
package battle

import (
	"fmt"

	dpkg "github.com/athlan20/stoneage/server/duel"
)

// 决斗里对方（被挑战方）队长的单位号；它的队员是 11–14。
const duelLeaderB = 10

// duelState 是决斗专有的状态。
type duelState struct {
	ledger   *dpkg.Ledger
	attacked bool // 有人提交过攻击类指令（攻击、精灵、宠物技能、道具），免罚判断用
	knocked  bool // 有人物倒下过
}

// NewDuel 开一场决斗。sideA 是发起方（必须含 0 号队长，其余 1–4），sideB 是被挑战方（必须含 10 号队长，其余 11–14）。
// 每个真人的 SideInput.DuelPoint 是开战时的决斗点数。开战时 Clock 为 time.Now，测试可以换成注入的时钟。
func NewDuel(id, scene string, sideA, sideB []PartyMate, rnd func(n int) int) (*Battle, error) {
	if err := checkDuelSide(sideA, 0); err != nil {
		return nil, err
	}
	if err := checkDuelSide(sideB, duelLeaderB); err != nil {
		return nil, err
	}
	var leader *SideInput
	for _, pm := range sideA {
		if pm.No == 0 {
			leader = pm.Side
		}
	}
	b := New(id, scene, leader, nil, rnd)
	b.Kind = "duel"
	b.AllowEscape, b.AllowCapture = true, false
	b.initParty()
	b.duel = &duelState{ledger: dpkg.NewLedger()}
	b.duel.ledger.Join(0, leader.DuelPoint)
	for _, pm := range append(append([]PartyMate(nil), sideA...), sideB...) {
		if pm.No == 0 {
			continue
		}
		b.addMate(pm)
		b.duel.ledger.Join(pm.No, pm.Side.DuelPoint)
	}
	for _, u := range b.units {
		if u != nil {
			u.pvp = true
		}
	}
	for _, slots := range append([][PetSlots]*Unit{b.pets}, b.matePets[:]...) { // 备用宠物换上场时也是决斗单位
		for _, u := range slots {
			if u != nil {
				u.pvp = true
			}
		}
	}
	b.refreshNeeds()
	return b, nil
}

// checkDuelSide 检查一方的名单：队长 lead 必须在，号码在 lead..lead+4 之内且不重复。
func checkDuelSide(side []PartyMate, lead int) error {
	if len(side) == 0 || len(side) > partyMaxMates+1 {
		return fmt.Errorf("决斗每方 1–%d 人，给了 %d 人", partyMaxMates+1, len(side))
	}
	seen := map[int]bool{}
	for _, pm := range side {
		if pm.No < lead || pm.No > lead+partyMaxMates || seen[pm.No] || pm.Side == nil {
			return fmt.Errorf("决斗的号码 %d 不合法或重复", pm.No)
		}
		seen[pm.No] = true
	}
	if !seen[lead] {
		return fmt.Errorf("决斗缺少队长（%d 号）", lead)
	}
	return nil
}

// IsDuel 表示这是一场决斗。
func (b *Battle) IsDuel() bool { return b.duel != nil }

// DuelDelta 是真人 no 本场的决斗点数累计变化（尚未钳位）；不是决斗时为 0。
func (b *Battle) DuelDelta(no int) int {
	if b.duel == nil {
		return 0
	}
	return b.duel.ledger.Delta(no)
}

// DuelStartPoint 是真人 no 开战时的决斗点数。
func (b *Battle) DuelStartPoint(no int) int {
	if b.duel == nil {
		return 0
	}
	return b.duel.ledger.Start(no)
}

// DuelHumans 返回决斗开战时的全部真人号（发起方在前，各自按号码升序）。
func (b *Battle) DuelHumans() []int {
	var out []int
	for _, no := range b.humanSlots() {
		if no == 0 || b.mates[no] != nil {
			out = append(out, no)
		}
	}
	return out
}
