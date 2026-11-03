// 道具指令的结算（STAGE-30，docs/protocol.md 10.5 的 item 与 10.7 的 item 事件）：
// 回复出自 gmsv/battle/battle_item.c 的 ITEM_useRecovery_Battle（走 battle_magic.c 的
// BATTLE_MultiRecovery），解状态出自 ITEM_useStatusRecovery_Battle（BATTLE_MultiStatusRecovery）。
// 复活类（ITEM_useRessurect、ITEM_ResAndDef，STAGE-262）对倒下的己方单位生效，结算同复活精灵（reviveAmount）。
// 物品的可用性与目标在提交时已校验（submit.go），事件带 slot（消耗的背包槽位），
// 写库与 inventory_changed 推送由 character 服务做（协议 10.7）。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// useItem 执行一次道具指令：回复 HP/MP 或解除异常状态，并把背包快照里的槽位置空
// （同一件物品不能再交）。回复量 = RAND(0.9N,1.1N)，HP 再乘恢复率（GetRecoveryRate），MP 不乘。
func (b *Battle) useItem(u *Unit, cmd Cmd) Event {
	items := &b.sideOfNo(b.actorNo(u)).Items // 真人同场时用使用者自己的背包（party_actor.go）
	it := items[cmd.Slot]
	items[cmd.Slot] = BattleItem{}
	def, _ := gamedata.ItemByID(it.ID)
	eff, _ := def.UseEffect()
	ev := Event{Kind: "item", Actor: &u.No, Name: def.Name, Slot: cmd.Slot}
	if eff.Recovery {
		for _, t := range b.itemTargets(u, cmd.Target, def.Target) {
			if eff.Full || eff.HP > 0 {
				power := 0
				if eff.Full {
					power = t.MaxHP // 回满：原版 power=10000000，靠上限截断
				} else {
					power = gamedata.RecoveryPower(eff.HP, t.Vital, t.Kind == KindPet, true, b.Rnd)
				}
				b.healUnit(t, power, &ev)
			}
			if (eff.Full || eff.MP > 0) && t.Kind == KindPlayer { // 气力只对人物生效（原版限定）
				gain := 0
				if eff.Full {
					gain = t.MaxMP // 原版 power=100，人物的气力上限就是 100
				} else {
					gain = gamedata.RecoveryPower(eff.MP, t.Vital, false, false, b.Rnd)
				}
				if gain > t.MaxMP-t.MP {
					gain = t.MaxMP - t.MP
				}
				if gain > 0 {
					t.MP += gain
					ev.Effects = append(ev.Effects, Effect{Target: t.No, Result: "heal", MPDelta: gain, HPPct: t.HPPct()})
				}
			}
		}
	}
	if eff.Revive { // 复活类（STAGE-262）：提交时已选定一个倒下的己方单位（人或宠物，怪物不行）
		if x := b.Unit(cmd.Target); x != nil && !x.Alive() && x.Side() == u.Side() && x.Kind != KindMonster && !b.duelNoRevive(x) {
			b.reviveAmount(x, eff.RevivePower, eff.RevivePercent, &ev)
			if eff.Defend != "" { // ITEM_ResAndDef：复活之后再挂上防御次数（同 reviveDefendMagic）
				b.defendUnit(x, eff.Defend, eff.DefendTurns, &ev)
			}
		}
	}
	if eff.Cure {
		for _, t := range b.itemTargets(u, cmd.Target, def.Target) {
			removed := t.Status.cure(eff.Status)
			if len(removed) == 0 { // 目标没有这种状态：没有效果（原版同）
				continue
			}
			eff := Effect{Target: t.No, Result: "none", HPPct: t.HPPct(), StatusRemove: removed}
			if b.hpShown(t) {
				hp := t.HP
				eff.HP = &hp
			}
			ev.Effects = append(ev.Effects, eff)
		}
	}
	return ev
}

// itemTargets 是道具指令的目标列表：target 是己方某个活着的单位；tg=全己方且没给
// 单体目标时是己方全部活着的单位。提交时已校验目标，这里再防一遍（结算与提交之间可能倒下）。
func (b *Battle) itemTargets(u *Unit, target, tg int) []*Unit {
	if tg == gamedata.TargetMySide && target == -1 {
		var out []*Unit
		for _, x := range b.units {
			if x != nil && x.Alive() && x.Side() == u.Side() {
				out = append(out, x)
			}
		}
		return out
	}
	if x := b.Unit(target); x != nil && x.Alive() && x.Side() == u.Side() {
		return []*Unit{x}
	}
	return nil
}
