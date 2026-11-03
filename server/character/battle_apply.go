// 战斗的推进与结束（docs/protocol.md 10.6–10.9）：结算一回合并推送 battle_round、
// 等所有连接确认后进入下一回合、战斗结束时把血量经验等写回角色并推数据变化。
// 这里所有函数都要求已持有 p.mu。

package character

import (
	"context"
	"strconv"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// sideInput 从角色数据构造战斗里玩家方的快照：出战宠物优先取 state 为出战的，
// 没有就第一只顶上（并把它的状态标成出战），其余进 Bench 供换宠（20 个栏位，STAGE-346）。
func sideInput(r *record) *battle.SideInput {
	eq := equipBonus(r.Items)
	side := &battle.SideInput{
		Name: r.Name, Level: r.Level,
		HP: r.HP, MaxHP: r.maxHP(), MP: r.MP, MaxMP: gamedata.NewPlayerMaxMP,
		Attack:    gamedata.Attack(r.Stats.Vital, r.Stats.Strength, r.Stats.Toughness, r.Stats.Dexterity),
		Defense:   gamedata.Defense(r.Stats.Vital, r.Stats.Strength, r.Stats.Toughness, r.Stats.Dexterity),
		Agility:   gamedata.Agility(r.Stats.Dexterity),
		Charm:     gamedata.EffectiveCharm(r.Charm, eq.Charm),
		Luck:      gamedata.EffectiveLuck(r.Luck, eq.Luck),
		Overkill:  r.Overkill,
		Vital:     r.Stats.Vital,
		Strength:  r.Stats.Strength,
		Toughness: r.Stats.Toughness,
		Dexterity: r.Stats.Dexterity,
		Elem:      battle.Elements{Earth: r.Elements.Earth * 10, Water: r.Elements.Water * 10, Fire: r.Elements.Fire * 10, Wind: r.Elements.Wind * 10},
		// 魔法熟练度与抗性（STAGE-226）：抗性在持久化值上加穿着的抗魔装备（原版 battle_magic.c 防守时的合算）。
		MagicExp: r.MagicExp, MagicResist: battleResist(r.MagicResist, eq), MagicAttExp: r.MagicAttExp, MagicDefExp: r.MagicDefExp,
		MagicEquip: magicEquip(eq),
	}
	pets := make([]*battle.PetInput, 0, len(r.Pets))
	for i := range r.Pets {
		p := &r.Pets[i]
		if !p.carried() { // 寄放在宠物店的不上战场（STAGE-73）
			continue
		}
		tmpl, ok := gamedata.PetByID(p.TemplateID)
		if !ok {
			continue
		}
		// 忠诚的基础值按开战时主人的有效魅力算（同宠物页 loyalty），可变部分随战斗变（battle/loyalty.go）。
		loyaltyBase, loyaltyBonus := gamedata.LoyaltyBase(r.Level, gamedata.EffectiveCharm(r.Charm, eq.Charm),
			r.Transmigration, p.Level, tmpl.ModAI)
		in := &battle.PetInput{
			Slot: p.Slot, TemplateID: p.TemplateID, Name: p.Name, Level: p.Level,
			HP: p.HP, MaxHP: gamedata.PetMaxHP(p.Stats),
			Attack: gamedata.PetAttack(p.Stats), Defense: gamedata.PetDefense(p.Stats),
			Agility:    gamedata.PetAgility(p.Stats),
			Vital:      p.Stats.Vital / 100,
			Strength:   p.Stats.Strength / 100,
			Toughness:  p.Stats.Toughness / 100,
			Dexterity:  p.Stats.Dexterity / 100,
			Elem:       battle.Elements{Earth: tmpl.Earth, Water: tmpl.Water, Fire: tmpl.Fire, Wind: tmpl.Wind},
			SkillSlots: tmpl.SkillSlots, Skills: p.Skills,
			LoyaltyBase: loyaltyBase, LoyaltyBonus: loyaltyBonus, VariableAI: p.VariableAI,
		}
		if p.State == petStateRide { // 骑着的宠物：人宠合体，不上格子、不进替补（docs/ride.md §5）
			side.Rider = in
			continue
		}
		pets = append(pets, in)
		if p.State == petStateBattle && side.BattlePet == nil {
			side.BattlePet = in
		}
	}
	// 没有设置出战的宠物时第一只顶上；骑着宠物时不顶，出战要玩家自己设（STAGE-149）。
	if side.BattlePet == nil && side.Rider == nil && len(pets) > 0 {
		side.BattlePet = pets[0]
		for i := range r.Pets {
			if r.Pets[i].Slot == pets[0].Slot {
				r.Pets[i].State = petStateBattle
			}
		}
	}
	for _, in := range pets {
		if side.BattlePet == nil || in.Slot != side.BattlePet.Slot {
			side.Bench[in.Slot] = in
		}
	}
	for _, it := range r.Items {
		if it.Slot >= 5 && it.Slot <= 19 {
			if def, ok := gamedata.ItemByID(it.ItemID); ok {
				side.Items[it.Slot] = battle.BattleItem{ID: it.ItemID, Name: def.Name}
			}
		}
	}
	for i, m := range r.magicSlots() {
		side.Magics[i], side.MagicMP[i], side.MagicMPSet[i] = m.ID, m.MP, m.ID >= 0
	}
	// 手里的武器（装备位 2）：反击系数表与暴击加成用（STAGE-60）。
	side.WeaponType = -1
	for _, it := range r.Items {
		if it.Slot == 2 {
			if def, ok := gamedata.ItemByID(it.ItemID); ok {
				side.WeaponType, side.WeaponCritical = def.Type, def.Critical
			}
			break
		}
	}
	// 人物装备栏（0–4）的耐久（STAGE-209）：没有耐久的装备也放进去，挑栏时它仍占着格子（原版如此）。
	for _, it := range r.Items {
		if gamedata.IsEquipSlot(it.Slot) {
			if def, ok := gamedata.ItemByID(it.ItemID); ok {
				side.Equips[it.Slot] = &battle.EquipInput{Slot: it.Slot, Name: def.Name, Durability: it.Durability, MaxDurability: it.MaxDurability, CrushLevel: it.CrushLevel}
			}
		}
	}
	side.Gold, side.Transmigration = r.Gold, r.Transmigration
	return side
}

// savePet 把一只宠物写进数据库：新宠物（捕捉成功）插入，已有的栏位整行覆盖（战斗后的等级、经验、
// 能力、血量与状态都要写回，否则下线就丢了）。栏位由 placeCapturedPet 安排。
// 列用 petInsertColumns 加 kills（共 25 列，与这里的 25 个参数一一对应；少一个占位符整句 INSERT 都会失败，STAGE-52）。
// STAGE-215：起始值五列只在插入时写，冲突时的 DO UPDATE 不列 origin_*，已有宠物的起始值原样保留，
// 哪怕内存里的 p.Origin 和库里不一样。
func (s *Service) savePet(ctx context.Context, charID int64, p petRec) error {
	oLevel, oVital, oStrength, oToughness, oDexterity := originArgs(p.Origin)
	_, err := s.db.Exec(ctx, `INSERT INTO character_pets (`+petInsertColumns+`, kills)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		ON CONFLICT (character_id, slot) DO UPDATE SET
			template_id = EXCLUDED.template_id, name = EXCLUDED.name, level = EXCLUDED.level, exp = EXCLUDED.exp,
			vital = EXCLUDED.vital, strength = EXCLUDED.strength, toughness = EXCLUDED.toughness, dexterity = EXCLUDED.dexterity,
			base_vital = EXCLUDED.base_vital, base_strength = EXCLUDED.base_strength,
			base_toughness = EXCLUDED.base_toughness, base_dexterity = EXCLUDED.base_dexterity,
			rank = EXCLUDED.rank, variable_ai = EXCLUDED.variable_ai, hp = EXCLUDED.hp, state = EXCLUDED.state, skills = EXCLUDED.skills, kills = EXCLUDED.kills`,
		charID, p.Slot, p.TemplateID, p.Name, p.Level, p.Exp,
		p.Stats.Vital, p.Stats.Strength, p.Stats.Toughness, p.Stats.Dexterity,
		p.Base.Vital, p.Base.Strength, p.Base.Toughness, p.Base.Dexterity,
		p.Rank, p.VariableAI, p.HP, p.State, skillsArray(p.Skills),
		oLevel, oVital, oStrength, oToughness, oDexterity, p.Kills)
	return err
}

// skillsArray 把技能栏转成数据库的 int[]。
func skillsArray(skills [7]int) []int32 {
	out := make([]int32, len(skills))
	for i, id := range skills {
		out[i] = int32(id)
	}
	return out
}

// nowMS 是当前时间的毫秒时间戳。
func (s *Service) nowMS() int64 { return s.opts.Now().UnixMilli() }

func (s *Service) now() time.Time { return s.opts.Now() }

// deadlineMS 是本回合出招的截止毫秒时间戳。
func deadlineMS(bs *battleState) int64 { return bs.deadline.UnixMilli() }

// nextBattleID 生成下一场战斗的编号（b_1、b_2……）。计数挂在服务上，
// 每个服务端实例独立，重放用例时总是从 b_1 开始（调用方持有 p.mu）。
func (s *Service) nextBattleID() string {
	s.battleSeq++
	return "b_" + strconv.Itoa(s.battleSeq)
}

// pushTurn 推送本回合的 battle_turn 并启动出招超时。
func (s *Service) pushTurn(p *Presence) {
	bs := p.battle
	bs.deadline = s.now().Add(battleTurnTimeout)
	p.pushAll("battle_turn", bs.b.TurnView(deadlineMS(bs), s.nowMS()))
	turn := bs.b.Turn()
	bs.timers = append(bs.timers, time.AfterFunc(battleTurnTimeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.battle == bs && bs.b.Turn() == turn && bs.b.Phase() == battle.PhaseCommand && !bs.b.AllSubmitted() {
			s.settle(p) // 超时没交齐：没交的按 skip 结算（协议 10.4）
		}
	}))
}

// settle 结算当前回合：先推 battle_progress（这个玩家交完了全部指令，协议 10.6），
// 再消耗本回合用掉的道具并推 inventory_changed、推 battle_round；战斗结束时紧接着推
// battle_end 并写回，否则进入等确认阶段（协议 10.7、10.8）。
func (s *Service) settle(p *Presence) {
	bs := p.battle
	bs.stopTimers()
	p.pushAll("battle_progress", bs.b.ProgressDone())
	round, end := bs.b.Settle()
	s.consumeBattleItems(p, round.Events) // 道具因出招被消耗：battle_round 之前推送（协议 10.7）
	p.pushAll("battle_round", round)
	if end != nil {
		s.finishBattle(p, end)
		return
	}
	bs.ackWait, bs.ackSeen, bs.turn = true, map[protocol.Role]bool{}, round.Turn
	bs.timers = append(bs.timers, time.AfterFunc(battleAckTimeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.battle == bs && bs.ackWait && bs.turn == round.Turn {
			s.advanceAfterAck(p) // 确认等待超时：视为都已确认（协议 10.8）
		}
	}))
}

// advanceAfterAck 所有连接都确认（或超时）后进入下一回合。
func (s *Service) advanceAfterAck(p *Presence) {
	p.battle.ackWait = false
	s.pushTurn(p)
}

// exitBattleOffline 是最后一条连接在战斗中断开时的收场（STAGE-53，对照原版
// gmsv char.c CHAR_logout → BATTLE_Exit）：人物和宠物立即退出战斗，战斗就此作废——
// 不结算奖励、不判负、不回出生点（掉线不能躲掉的是已受的伤害，不是惩罚）；
// 当前血量、魔法和宠物状态照常写回并保存，人物倒下则血量回 1（原版 CHAR_setInt(CHAR_HP,1)）。
// persist 为 false 时只改内存（撤销进入用，角色已删，不必写库）。调用方持有 p.mu。
func (s *Service) exitBattleOffline(p *Presence, persist bool) {
	bs := p.battle
	if bs == nil {
		return
	}
	bs.stopTimers() // 计时器随战斗一起销毁，不留在无连接的 presence 上
	s.releaseNPCFight(bs)
	if u := bs.b.PlayerUnit(); u != nil {
		p.rec.HP, p.rec.MP = u.HP, u.MP
		if p.rec.HP < 1 {
			p.rec.HP = 1
		}
	}
	applyPets(&p.rec, bs.b)
	p.battle = nil
	p.players.touch(p) // STAGE-273：战斗结束
	if persist {
		s.savePets(p.charID, p.rec.Pets)
	}
}

// finishBattle 结束战斗：推 battle_end，把结果写回角色数据，推数据变化，摘掉战斗。
// 宠物表的数据库写入见 savePets（持锁同步写）；掉落见 saveBattleItems。
func (s *Service) finishBattle(p *Presence, end *battle.EndView) {
	bs := p.battle
	bs.stopTimers()
	flown := bs.b.PlayerFlown() // STAGE-194：只有被打飞才传走，被打死原地复活，位置不动
	dropped := s.applyResult(&p.rec, bs.b, end)
	crushed := applyItemCrush(&p.rec, bs.b) // STAGE-209：战斗中装备的耐久、受损等级写回背包，耗尽的删掉
	s.saveItemCrush(p.charID, crushed)
	p.pushAll("battle_end", end)
	p.battle = nil
	// STAGE-282：留给迟到的 battle_ack 认账
	p.lastBattleTurn = end.Turn
	p.players.moved(p) // STAGE-273：战斗结束可能被打飞换图，等级也可能变（applyResult 里升级）
	p.players.touch(p)
	warped, stolen := s.endNPCFight(p, bs, end) // NPC 发起的战斗：释放占用，赢了收道具、按参数传送（STAGE-96）

	if warped || (end.Result == "lose" && flown) {
		p.pushPosition(&p.rec)
	}
	p.pushAll("status_changed", map[string]any{"status": p.rec.status()})
	if bs.b.PetsChanged() {
		p.pushAll("pets_changed", p.rec.petsView())
	}
	if p.party != nil { // 分身战后恢复满血，队伍状态随战斗结束推一次（STAGE-162）
		if p.party.rebuild { // 战斗中组的队：按结算后的队长（经验、等级已加）重新做分身（STAGE-189）
			p.party = newCloneParty(&p.rec)
		}
		p.party.restore()
		p.pushPartyChanged(&p.rec, partyBattleEnd)
	}
	s.saveProgress(p)                              // STAGE-290：升级、经验、金币、血量立即存盘，不等离开世界
	s.savePets(p.charID, p.rec.Pets)               // 经验、等级、能力、血量、状态都可能变了，整张宠物表写回
	s.saveSkillPoints(p.charID, p.rec.SkillPoints) // 升级奖励的未分配点数：加点按库里的值扣，必须落库（STAGE-151）
	if len(dropped) > 0 {                          // 掉落（STAGE-60）：进背包的物品写库并推送
		s.saveBattleItems(p.charID, dropped)
	}
	if len(dropped) > 0 || stolen || len(crushed) > 0 {
		p.pushAll("inventory_changed", p.rec.inventoryView())
	}

	// 遇敌开关还开着的话（STAGE-40）：被打飞送走的关掉并说明原因；其余结果隔一段时间开下一场。
	// 被打死原地复活（STAGE-194）不关，和赢了、逃跑一样排下一场。
	// NPC 战斗赢了被传送走的，按传送处理（同 warp，协议 5.29）。
	if p.encounterOn {
		switch {
		case warped:
			p.stopEncounter(encounterOffWarp)
		case end.Result == "lose" && flown:
			p.stopEncounter(encounterOffLose)
		case p.plan != nil && p.plan.Mode == "once":
			// 单次遇敌（STAGE-152）：第一场打完就停，赢、逃、捕获、输都一样不再开下一场。
			p.stopEncounter(encounterOnceDone)
		default:
			s.scheduleEncounter(p)
		}
	}
}

// applyResult 把战斗结果写进角色数据：血量、经验、金币、宠物（血量经验、捕捉、换宠后的
// 出战状态），失败时人物复活（STAGE-194）。rewards 里的等级变化也在这一步算好填回。
// 返回战斗掉落里装进背包的物品（含安排好的槽位），供写库与推送（STAGE-60）。
func (s *Service) applyResult(r *record, b *battle.Battle, end *battle.EndView) (dropped []itemRec) {
	// 人物与出战宠物的最终血量、MP。
	if u := b.PlayerUnit(); u != nil {
		r.HP, r.MP = u.HP, u.MP
		r.Overkill = u.Overkill
		// 魔法熟练度与抗性的成长（STAGE-227）：写回持久化值，抗性不含抗魔装备。
		r.MagicExp, r.MagicResist, r.MagicAttExp, r.MagicDefExp = b.PlayerMagic()
	}
	applyPets(r, b)
	if end.Result == "lose" {
		flown := b.PlayerFlown()
		// 被打飞：送回存档点的落点（记录过的话，STAGE-72），没有存档点回出生地（项目规则，原版没有）。
		// 被打死：原地以 1 血复活，不传送（原版 battle.c 的 _BATTLE_Exit：ISDIE 清掉、HP 设为 1）。
		if flown {
			f, x, y, spOK := SavepointRespawn(r)
			if !spOK {
				h := gamedata.Hometowns[r.Hometown]
				f, x, y = h.Floor, h.X, h.Y
			}
			r.Floor, r.X, r.Y, r.Direction = f, x, y, gamedata.DefaultDirection
		}
		r.HP = 1 // 战斗失败不删档：血量剩 1
		// 被打倒或打飞：魅力减少（STAGE-61、STAGE-63，见 charmOnDeath）。
		r.Charm = gamedata.ClampCharm(r.Charm + charmOnDeath(r.Level, flown))
		if end.Penalty != nil {
			end.Penalty.Respawn = r.position()
			end.Penalty.InPlace = !flown
		}
		return nil
	}
	if end.Rewards == nil {
		return nil
	}
	for i, e := range end.Rewards.Exp {
		if e.Who == "player" {
			r.Exp += e.Gain
			to, points, remain := levelUp(r.Level, r.Exp, 3) // 每级 3 点（原版 CHAR_SKILLUPPOINT 的 UpLevel*3）
			// 升级：魅力 +2，升几级都只加一次（battle.c 的 CH_FIX_PLAYERLEVELUP）。
			if to > r.Level {
				r.Charm = gamedata.ClampCharm(r.Charm + gamedata.CharmOnLevelUp)
			}
			r.Level, r.SkillPoints, r.Exp = to, r.SkillPoints+points, remain
			end.Rewards.Exp[i].LevelTo = to
			end.Rewards.Exp[i].SkillPoints = points
		} else if e.Who == "pet" {
			for j := range r.Pets {
				if e.Slot != nil && r.Pets[j].Slot == *e.Slot {
					end.Rewards.Exp[i].LevelTo = gainPetExp(&r.Pets[j], e.Gain, b.Rnd)
				}
			}
		}
	}
	// 宠物击杀数（STAGE-150）：出过手且战后还活着的宠物，按击败数累加，写回宠物记录。
	for _, k := range end.Rewards.PetKills {
		for j := range r.Pets {
			if r.Pets[j].Slot == k.Slot {
				r.Pets[j].Kills += k.Kills
			}
		}
	}
	r.Gold += end.Rewards.Gold
	end.Rewards.Items, dropped = placeRewardItems(r, end.Rewards.Items)
	return dropped
}

// placeRewardItems 把战斗掉落的物品放进背包（第一个空格，5–19）：装得下的留在
// rewards.items 里（battle_end 报的是实际拿到的东西，同原版 lssproto_RS），
// 背包满的丢弃（BATTLE_GetExpGold：没有空格的战利品直接销毁）。
// 返回装下的条目和它们安排好的背包行（写库用）。
func placeRewardItems(r *record, items []battle.RewardItem) (kept []battle.RewardItem, placed []itemRec) {
	taken := [MaxItemSlot + 1]bool{}
	for _, it := range r.Items {
		taken[it.Slot] = true
	}
	for _, it := range items {
		if _, ok := gamedata.ItemByID(it.ItemID); !ok {
			continue // 掉落表引用了物品表里没有的编号：不发放，免得背包里出现占格的幽灵物品（STAGE-191）
		}
		slot := -1
		for s := firstBagSlot; s <= MaxItemSlot; s++ {
			if !taken[s] {
				slot = s
				break
			}
		}
		if slot < 0 {
			continue // 背包满：丢弃（不提示，同原版）
		}
		taken[slot] = true
		dur, maxDur := newItemDurability(it.ItemID) // STAGE-209：战斗掉落的装备按物品表设初始耐久
		rec := itemRec{Slot: slot, ItemID: it.ItemID, Count: it.Count, Durability: dur, MaxDurability: maxDur}
		r.Items = append(r.Items, rec)
		placed = append(placed, rec)
		kept = append(kept, it)
	}
	return kept, placed
}

// applyPets 把战斗里出过场的宠物的血量、忠诚的可变部分写回宠物列表，并定出战状态：
// 结束时在场的为出战（逃跑时一起离场的也算在场，STAGE-247），换下去的为休息，待命的保持待命；
// 血量、忠诚的可变部分（STAGE-127 起）不论哪种都写回。
func applyPets(r *record, b *battle.Battle) {
	applyPetEnds(r, b.PetEnds())
}

// applyPetEnds 同 applyPets，但宠物的结局由调用方给出（真人同场时每人取自己的宠物，STAGE-275）。
func applyPetEnds(r *record, pets []battle.PetEnd) {
	ends := map[int]battle.PetEnd{}
	for _, e := range pets {
		ends[e.Slot] = e
	}
	for i := range r.Pets {
		p := &r.Pets[i]
		if !p.carried() { // 寄放的状态不由战斗决定
			continue
		}
		if p.State == petStateRide { // 骑着打完还是骑着（落马不在本期，docs/ride.md §7）
			if e, ok := ends[p.Slot]; ok {
				p.HP, p.VariableAI = e.HP, e.VariableAI
			}
			continue
		}
		prev := p.State
		p.State = petStateRest
		if prev == petStateStandby { // STAGE-247：待命的不是出战，战斗不改它的状态（原版非 DEFAULTPET 的宠物不受 BATTLE_Exit 影响）
			p.State = petStateStandby
		}
		e, ok := ends[p.Slot]
		if !ok {
			continue
		}
		p.HP, p.VariableAI = e.HP, e.VariableAI
		if e.Active {
			p.State = petStateBattle
		}
	}
	// STAGE-245：战斗结束（不论胜负、逃跑、平局，也不论这场有没有出场）血量 ≤ 0 的随身宠物一律回到 1 血，
	// 对照原版 battle.c 的 BATTLE_Exit：遍历全部宠物，死亡标记或 HP ≤ 0 都清掉、HP 设为 1。
	// 我们没有单独的死亡标记，“已倒下”就是 HP ≤ 0（pets_rules.go 的出战校验），所以只看血量。
	// 寄放的宠物不遍历（对应原版 CHAR_MAILMODE 跳过），见 carried。
	for i := range r.Pets {
		if p := &r.Pets[i]; p.carried() && p.HP < 1 {
			p.HP = 1
		}
	}
}

// levelUp 是人物的升级：按经验表连升到不能升为止——每升一级从经验里扣掉这一级所需
// （原版 CHAR_LevelUpCheck 调 CHAR_HandleExp 的 EXP -= NeedLevelUpTbls[level+1]），
// 返回升到的等级、总共获得的点数（每级 3 点）和扣完剩下的本级经验。
// 最高 160 级（gmsv/setup.cf 的 MAXLEVEL）。宠物见 gainPetExp（同样按级扣除）。
func levelUp(from int, exp int64, pointsPerLevel int) (to int, points int, remain int64) {
	to = from
	for to < 160 {
		need := gamedata.NextExp(to)
		if need < 0 || exp < need {
			break
		}
		exp -= need
		to++
		points += pointsPerLevel
	}
	return to, points, exp
}

// saveBattleItems 把战斗掉落写进数据库（只插新行；调用方持有 p.mu，同步写完再放锁，
// 与 savePets 同一个道理）。背包格由 placeRewardItems 安排，不会与现有行冲突。
func (s *Service) saveBattleItems(charID int64, items []itemRec) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, it := range items {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO character_items (character_id, slot, item_id, count, durability, max_durability, crush_level) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			charID, it.Slot, it.ItemID, it.Count, it.Durability, it.MaxDurability, it.CrushLevel); err != nil {
			s.opts.Logger.Warn("保存战斗掉落失败", "character", charID, "item", it.ItemID, "err", err)
		}
	}
}

// savePets 把角色的宠物表整张写回数据库：捕捉到的宠物要插入，出战宠物的经验、等级、能力、血量也要写回。
// 宠物最多 5 只，逐行写即可。finishBattle 由持有 p.mu 的调用方触发（settle 在锁内），所以这里不再加锁，
// 同步写完再放锁，保证两场战斗的写回不会乱序。
func (s *Service) savePets(charID int64, pets []petRec) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, pet := range pets {
		if err := s.savePet(ctx, charID, pet); err != nil {
			s.opts.Logger.Warn("保存宠物失败", "character", charID, "slot", pet.Slot, "err", err)
		}
	}
}
