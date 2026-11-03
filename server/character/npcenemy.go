package character

// 战斗 NPC（STAGE-96，docs/battle-npc.md 5.2–5.5）：对放开的战斗 NPC 说话时，前置道具不全或正被占用就弹一个
// 没有选项的台词窗口（原版 deniedmsg / alreadymsg）；能开战且有 askbattlemsg 就弹是/否的确认，选是时再检查一次，
// 通过就开战。开战时的占用登记（one_battle，原子地检查并登记）在 startNPCFight；胜负后的收尾在 battle_apply.go 的
// endNPCFight，掉线时释放占用在 exitBattleOffline。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// 前置道具（npc-enemy.json 的 steal）什么时候收走。
const (
	stealNever = -1 // 不收
	stealStart = 0  // 开战时收
	stealWin   = 1  // 赢了才收
)

// fightYesNo 是确认窗口的两个选项：1 是，2 否（同任务 NPC 的问答，自己定义一份，不依赖 npcevent.go）。
var fightYesNo = []npcOption{{No: 1, Label: "是"}, {No: 2, Label: "否"}}

// errNPCBusy 是 one_battle 的 NPC 正被别的战斗占用，开战登记不上。
var errNPCBusy = errors.New("这个 NPC 正在被别的战斗占用")

// npcBattle 是 NPC 发起的战斗在角色上的记录：哪个 NPC、它的参数，以及有没有登记 one_battle 占用（结束时要释放）。
type npcBattle struct {
	npc      int
	spec     *gamedata.NpcEnemy
	occupied bool
}

// enemyText 是 NPC 的拒绝或占用台词；参数里没写时用“{名字}没有理你。”（设计 5.2 的兜底）。
func enemyText(text string, npc gamedata.NPC) string {
	if text != "" {
		return text
	}
	return npcDisplayName(npc) + "没有理你。"
}

// enemyBlocked 检查现在能不能开战：前置道具要全部在背包里（不含穿戴），one_battle 的 NPC 不能正被占用。
// 不能时返回要说的台词；这里只是预先检查，真正开战时以 startNPCFight 的登记为准。
func (s *Service) enemyBlocked(r *record, npc gamedata.NPC, ne *gamedata.NpcEnemy) (string, bool) {
	for _, id := range ne.Items {
		if findItemSlot(r, id, 1) < 0 {
			return enemyText(ne.Denied, npc), true
		}
	}
	if ne.OneBattle && s.npcs.Busy(npc.ID) {
		return enemyText(ne.Already, npc), true
	}
	return "", false
}

// talkEnemy 和战斗 NPC 说话（talk，协议 5.33）：不能开战就弹一个没有选项的台词窗口；能开战且有 askbattlemsg
// 就弹是/否（选项见 fightYesNo）；没有台词就直接开战。调用方持有 p.mu。
func (s *Service) talkEnemy(l *Live, r *record, npc gamedata.NPC, ne *gamedata.NpcEnemy) *protocol.Error {
	if text, blocked := s.enemyBlocked(r, npc, ne); blocked {
		openEventText(l.p, npc, text)
		return nil
	}
	if len(ne.Ask) == 0 {
		return s.beginEnemyFight(l, r, npc, ne)
	}
	l.p.npcWindow = &npcWindow{npc: npc.ID, options: fightYesNo, fight: true}
	l.p.pushAll("npc_window", npcWindowView(npc, strings.Join(ne.Ask, "\n"), fightYesNo...))
	return nil
}

// fightChoose 回应战斗 NPC 的是/否（npc_choose，协议 5.35）：选 2 只关窗口；选 1 先关窗口（CHOSEN），再按现在的
// 状态检查一次（原版不重查，窗口开着的时候道具可能丢了、或者别人先开战了），通过就开战，不通过换成对应的台词窗口。
func (s *Service) fightChoose(l *Live, r *record, npc gamedata.NPC, no int) *protocol.Error {
	if no < 1 || no > len(fightYesNo) {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
	}
	l.p.pushWindowClosed(WindowReasonChosen)
	ne, ok := gamedata.NpcEnemyOf(npc.ID)
	if no != 1 || !ok || ne.Unsupported != "" || !s.npcEnemyOpen(ne.File) {
		return nil // 选了“否”；参数已经不能开战（白名单变了）也只关窗口
	}
	if text, blocked := s.enemyBlocked(r, npc, ne); blocked {
		openEventText(l.p, npc, text)
		return nil
	}
	return s.beginEnemyFight(l, r, npc, ne)
}

// beginEnemyFight 按参数开战：敌人按 enemies 的顺序入场（battle.BuildNPC），开战后收走 steal=0 的道具。
// 占用登记不上（同时有人在打）时说占用台词，不开战。调用方持有 p.mu。
func (s *Service) beginEnemyFight(l *Live, r *record, npc gamedata.NPC, ne *gamedata.NpcEnemy) *protocol.Error {
	err := s.startNPCFight(l.p, npc, ne, func(id string) (*battle.Battle, error) {
		b, ok := battle.BuildNPC(id, battle.EncounterScene(r.Floor), sideInput(r), ne.Enemies, s.wildRnd())
		if !ok {
			return nil, errNoEnemies
		}
		return b, nil
	})
	switch {
	case errors.Is(err, errNPCBusy):
		openEventText(l.p, npc, enemyText(ne.Already, npc))
		return nil
	case err != nil:
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if s.takeEnemyItems(l.p, ne, stealStart) {
		l.p.pushAll("inventory_changed", r.inventoryView())
	}
	return nil
}

var errNoEnemies = errors.New("战斗 NPC 的敌人一个都摆不出来")

// startNPCFight 把 NPC 战斗挂到角色上：one_battle 的先原子地登记占用（已被占用返回 errNPCBusy），再造战斗，
// 挂上 NPC 的身份（battle_start 的 npc.say 是开战台词），推第一回合。造不出来时释放刚登记的占用。调用方持有 p.mu。
func (s *Service) startNPCFight(p *Presence, npc gamedata.NPC, ne *gamedata.NpcEnemy, build func(id string) (*battle.Battle, error)) error {
	id := s.nextBattleID()
	if ne.OneBattle && !s.npcs.Occupy(npc.ID, id) {
		return errNPCBusy
	}
	b, err := build(id)
	if err != nil {
		if ne.OneBattle {
			s.npcs.Release(npc.ID, id)
		}
		return err
	}
	b.SetNPC(battle.NPCFight{NPCRef: battle.NPCRef{ID: npc.ID, Name: npc.Name}, Start: ne.Start, End: ne.End})
	s.wireBattle(p, b)
	p.battle.npc = &npcBattle{npc: npc.ID, spec: ne, occupied: ne.OneBattle}
	return nil
}

// startScenarioNPCBattle 按测试剧本开一场 NPC 战斗（协议用例的 start_battle，剧本 npc_win，STAGE-96）：NPC 和台词
// 取自参数文件，怪物与随机种子取自剧本。调用方持有 p.mu。
func (s *Service) startScenarioNPCBattle(p *Presence, sc *battle.Scenario) error {
	ne, ok := gamedata.NpcEnemyByFile(sc.NPCFile)
	if !ok {
		return fmt.Errorf("没有参数文件 %s 的战斗 NPC", sc.NPCFile)
	}
	npc, ok := gamedata.NPCByID(ne.ID)
	if !ok {
		return fmt.Errorf("战斗 NPC %d 不在 NPC 表里", ne.ID)
	}
	return s.startNPCFight(p, npc, ne, func(id string) (*battle.Battle, error) {
		b, err := sc.Build(id, sideInput(&p.rec))
		if err == nil {
			b.Scene = battle.EncounterScene(p.rec.Floor) // 和真实的 NPC 战斗一样，场景是地图名
		}
		return b, err
	})
}

// takeEnemyItems 收走战斗 NPC 的前置道具（steal 等于 when 时）：每件拿走背包里的一件并写库。返回是否收走了东西，
// 由调用方推送 inventory_changed。调用方持有 p.mu。
func (s *Service) takeEnemyItems(p *Presence, ne *gamedata.NpcEnemy, when int) bool {
	if ne.Steal != when {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	taken := false
	for _, id := range ne.Items {
		slot := findItemSlot(&p.rec, id, 1)
		if slot < 0 {
			continue
		}
		if err := s.takeBagItem(ctx, &p.rec, slot, 1); err != nil {
			s.opts.Logger.Warn("收走战斗道具失败", "character", p.charID, "item", id, "err", err)
			continue
		}
		taken = true
	}
	return taken
}

// npcEnemyOpen 报告战斗 NPC 的参数文件是否放开：测试用 Options.NpcEnemyEnabled 换白名单，零值用 gamedata 的白名单。
func (s *Service) npcEnemyOpen(file string) bool {
	if s.opts.NpcEnemyEnabled != nil {
		return s.opts.NpcEnemyEnabled(file)
	}
	return gamedata.NpcEnemyEnabled(file)
}

// endNPCFight 收尾 NPC 发起的战斗（STAGE-96，docs/battle-npc.md 5.5）：释放 one_battle 占用；赢了的话收走 steal=1 的
// 道具，die_act=1 的传送到参数里的落点（窗口关掉，同 warp）。输、逃、平不改 NPC，NPC 留在原地。
// 返回是否传送、是否收走了道具。没有 NPC 的战斗什么都不做。调用方持有 p.mu。
func (s *Service) endNPCFight(p *Presence, bs *battleState, end *battle.EndView) (warped, stolen bool) {
	if bs.npc == nil {
		return false, false
	}
	s.releaseNPCFight(bs)
	if end.Result != "win" {
		return false, false
	}
	stolen = s.takeEnemyItems(p, bs.npc.spec, stealWin)
	if w := bs.npc.spec.Warp; bs.npc.spec.DieAct == 1 && w != nil {
		p.rec.Floor, p.rec.X, p.rec.Y = w.Floor, w.X, w.Y
		p.players.moved(p) // STAGE-273：换图
		p.pushWindowClosed(WindowReasonMoved)
		return true, stolen
	}
	return false, stolen
}

// releaseNPCFight 释放 NPC 发起的战斗登记的 one_battle 占用：战斗结束（赢输逃平）和掉线都要调，只放自己登记的。
func (s *Service) releaseNPCFight(bs *battleState) {
	if bs.npc != nil && bs.npc.occupied {
		s.npcs.Release(bs.npc.npc, bs.b.ID)
	}
}
