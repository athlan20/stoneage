package battle

import "time"

// 真人同场的野外遇敌（STAGE-275）：敌群、敌人与合击的抽法同单人野外遇敌（BuildWild），只是人物侧换成真人队。

// BuildWildParty 按遇敌表开一场真人同场的野外战斗。leader 是队长快照，mates 是 1–4 号队员快照（见 NewParty）。
// 抽不出敌群或敌人时返回 false（同 BuildWild）。
func BuildWildParty(id, scene string, leader *SideInput, mates []PartyMate, spec *WildSpec, rnd func(n int) int) (*Battle, bool, error) {
	slots, ok := wildEnemySlots(spec, rnd)
	if !ok {
		return nil, false, nil
	}
	monsters := wildMonsters(slots, rnd)
	if len(monsters) == 0 {
		return nil, false, nil
	}
	b, err := NewParty(id, scene, leader, mates, monsters, rnd)
	if err != nil {
		return nil, false, err
	}
	b.Combo = true // STAGE-126：真实遇敌有合击
	return b, true, nil
}

// Deadline 是本回合交指令的截止时间（真人同场，第一条指令起 120 秒）；还没人交过指令时为零值。
func (b *Battle) Deadline() time.Time { return b.deadline }
