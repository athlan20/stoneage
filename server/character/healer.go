package character

// 护士治疗（STAGE-71，docs/protocol.md 5.39）：talk 护士 NPC 弹确认窗口（正文按原版
// 文案生成：受伤报价 / 免费线以下免费 / 全满「似乎没有必要回复唷！」），选「治疗」后
// 人物 HP、MP 回满并回满背包里全部宠物的 HP，扣一次费用（等级低于护士的免费线不收钱，
// 原版 npc_windowhealer.c 的「耐久力气力回复」一路：费用 = 等级 × 倍率，各按缺口算）。
// 费用只改内存金币（离线保存回写，同商店）；HP/MP 也是内存字段，保存路径同战斗结算。

import (
	"context"
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// healerConfirm 是护士窗口的一个选项（只有 1 治疗，2 算了是关闭）。
const healerOptionHeal = 1

// openHealer 是 talk 成功后的护士确认窗口。调用时要持有 p.mu（在 talk 的 Operate 里）。
func (s *Service) openHealer(p *Presence, npc gamedata.NPC, healer *gamedata.NpcHealer) {
	p.npcWindow = &npcWindow{npc: npc.ID}
	p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": healerPrompt(&p.rec, healer), "options": healerOptions(&p.rec),
	})
}

// healerOptions 是窗口的选项：有可治的（人物或宠物）才给「治疗」；全满时没有选项
// （原版是 OK 窗口，按任意数字或 Esc 关闭）。
func healerOptions(r *record) []npcOption {
	if needsHeal(r) {
		return []npcOption{{No: healerOptionHeal, Label: "治疗"}}
	}
	return []npcOption{}
}

// needsHeal 报告有没有要治的：人物 HP/MP 不满，或背包里有受伤的宠物。
func needsHeal(r *record) bool {
	if r.HP < r.maxHP() || r.MP < gamedata.NewPlayerMaxMP {
		return true
	}
	for i := range r.Pets {
		if r.Pets[i].carried() && r.Pets[i].HP < gamedata.PetMaxHP(r.Pets[i].Stats) {
			return true
		}
	}
	return false
}

// healerPrompt 是确认窗口的正文（原版 npc_windowhealer.c 的文案）：
// 全满「似乎没有必要回复唷！」；免费线以下「现在的等级可以免费帮你回复唷！」；
// 否则报价「是要回复耐久力气力没错吧！收您 X 的 STONE。」。
func healerPrompt(r *record, healer *gamedata.NpcHealer) string {
	if !needsHeal(r) {
		return "＜耐久力气力回复＞\n\n似乎没有必要回复唷！"
	}
	if r.Level < healer.Level {
		return "＜耐久力气力回复＞\n\n是要回复耐久力气力没错吧！\n\n现在的等级可以免费帮你回复唷！"
	}
	cost, petCost := healerTotalCost(r, healer)
	if petCost > 0 {
		return fmt.Sprintf("＜耐久力气力回复＞\n\n是要回复耐久力气力没错吧！\n\n收您 %d 的 STONE（含宠物回复）。", cost)
	}
	return fmt.Sprintf("＜耐久力气力回复＞\n\n是要回复耐久力气力没错吧！\n\n收您 %d 的 STONE。", cost)
}

// healerTotalCost 是这次治疗的费用：人物 HP、MP 各按缺口算（等级 × 各自倍率），宠物回复
// 原版免费（NPC_PetHealerCheck 的「宠物回复(免费)」），所以费用只含人物两项。
func healerTotalCost(r *record, healer *gamedata.NpcHealer) (int, int) {
	cost := 0
	petCost := 0
	if r.HP < r.maxHP() {
		c, _ := healer.HPCost(r.Level)
		cost += c
	} else {
		petCost++
	}
	if r.MP < gamedata.NewPlayerMaxMP {
		c, _ := healer.MPCost(r.Level)
		cost += c
	}
	if petCost == 0 {
		for i := range r.Pets {
			if r.Pets[i].carried() && r.Pets[i].HP < gamedata.PetMaxHP(r.Pets[i].Stats) {
				petCost++
				break
			}
		}
	}
	return cost, petCost
}

// healerChoose 处理护士窗口的选项（npc_choose）。调用时要持有 p.mu。
func (s *Service) healerChoose(l *Live, r *record, w *npcWindow, npc gamedata.NPC, healer *gamedata.NpcHealer, no int) *protocol.Error {
	if no != healerOptionHeal {
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
	}
	if !needsHeal(r) {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "似乎没有必要回复唷！")
	}
	cost, _ := healerTotalCost(r, healer)
	if r.Level < healer.Level {
		cost = 0 // 免费线以下不收钱（原版「现在的等级可以免费帮你回复唷！」）
	}
	if r.Gold < int64(cost) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "真可惜似乎所带的金钱不够唷！（需要 %d）", cost)
	}
	// 治疗：人物 HP、MP 回满，背包里的宠物全部回满（原版 NPC_WindowHealerAllHeal）。
	r.HP = r.maxHP()
	r.MP = gamedata.NewPlayerMaxMP
	for i := range r.Pets {
		if r.Pets[i].carried() {
			r.Pets[i].HP = gamedata.PetMaxHP(r.Pets[i].Stats)
		}
	}
	if cost > 0 {
		r.Gold -= int64(cost)
	}
	s.savePets(l.p.charID, r.Pets) // 宠物血量写库：设出战的规则判定读的是数据库（STAGE-168）
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushAll("pets_changed", r.petsView())
	l.p.pushWindowClosed(WindowReasonChosen)
	return nil
}

// handleHealerChoose 挂进 npc_choose 的分派（dialog.go 里按窗口调）。
func (s *Service) handleHealerChoose(ctx context.Context, l *Live, r *record, w *npcWindow, no int) *protocol.Error {
	npc, _ := s.npcs.ByID(w.npc)
	healer := gamedata.NpcHealerOf(w.npc)
	return s.healerChoose(l, r, w, npc, healer, no)
}
