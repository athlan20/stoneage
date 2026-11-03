package character

// 美容师（STAGE-255，原版 gmsv/npc/npc_charm.c）：talk 美容师弹「给我魅力 / 什么也不做」，
// 选 1 后弹报价（是 / 否），选「是」扣石币、魅力 +5（封顶 100），再弹结果窗口（OK，没有选项）。
// 费用 = 等级 × 10 × ⌊魅力 / 3⌋ × (转生次数+1)，魅力按基础值算（不含装备），0 和 1 按 3 算；
// 魅力已满 100 时直接给「你的魅力真是完美」，不收钱。金币和魅力都只改内存（离开世界时随角色保存）。
// 随身宠物的忠诚由宠物视图现算，魅力变化后推 status_changed、pets_changed 即可。

import (
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

const (
	charmRate       = 10 // 原版 RATE：费用倍率
	charmWaru       = 3  // 原版 WARU：魅力按这个档位换算
	charmStepUp     = 5  // 原版 CHARMHEAL：每次美容加的魅力
	charmOptionGive = 1  // 第一步：给我魅力
	charmOptionNot  = 2  // 第一步：什么也不做
	charmOptionYes  = 1  // 报价：是
	charmOptionNo   = 2  // 报价：否
)

// charmStage 是美容师窗口所在的步骤。
type charmStage int

const (
	charmStart  charmStage = iota + 1 // 第一步：给我魅力 / 什么也不做
	charmAsk                          // 第二步：报价，是 / 否
	charmResult                       // 结果：OK 窗口，没有选项，任意选择都关窗
)

// charmSession 是美容师窗口的当前步骤（挂在 npcWindow.charm 上，其他 NPC 为 nil）。
type charmSession struct {
	stage charmStage
}

// charmPrice 是这次美容的石币价（照 NPC_CharmCost）。魅力已满（>= 100）返回 -1。
func charmPrice(level, charm, trans int) int {
	if charm >= gamedata.CharmMax {
		return -1
	}
	if charm <= 1 {
		charm = charmWaru
	}
	return level * charmRate * (charm / charmWaru) * (trans + 1)
}

// charmImageComment 是美容成功后的第二行台词，按形象分支（原版 CHAR_IMAGETYPE，见 char_data.c 的
// CHAR_defaultCharacterGet：Body 0 儿童男、1–3 男孩、4–5 男人、6 儿童女、7–9 女孩、10–11 女人）。
func charmImageComment(body int) string {
	switch {
	case body == 0 || body == 6:
		return "真的变得好可爱唷！"
	case body >= 1 && body <= 3:
		return "是不是比刚才要帅多了呢？"
	case body == 4 || body == 5:
		return "变得更酷了呢！"
	case body >= 7 && body <= 9:
		return "愈来愈可爱了呢！"
	default:
		return "可真是变得愈来愈美了呢！"
	}
}

const (
	charmTitle        = "美容师"
	charmStartText    = charmTitle + "\n欢迎光临，今天要做什麽呢？\n《　给我魅力　》\n《 什麽也不做 》"
	charmFullText     = charmTitle + "\n\n你的魅力真是完美\n\n请永远保持你的魅力唷。"
	charmNotEnoughMsg = "哎呀!你的钱不够唷！先去存好钱下次再来吧！"
)

func charmStartOptions() []npcOption {
	return []npcOption{{No: charmOptionGive, Label: "给我魅力"}, {No: charmOptionNot, Label: "什么也不做"}}
}

func charmAskOptions() []npcOption {
	return []npcOption{{No: charmOptionYes, Label: "是"}, {No: charmOptionNo, Label: "否"}}
}

// openCharm 是 talk 美容师后的第一步窗口。调用时要持有 p.mu。
func openCharm(p *Presence, npc gamedata.NPC) {
	p.npcWindow = &npcWindow{npc: npc.ID, options: charmStartOptions(), charm: &charmSession{stage: charmStart}}
	p.pushAll("npc_window", npcWindowView(npc, charmStartText, charmStartOptions()...))
}

// openCharmResult 弹结果窗口（OK，没有选项）。先关掉上一个窗口（CHOSEN）。调用时要持有 p.mu。
func openCharmResult(p *Presence, npc gamedata.NPC, text string) {
	p.pushWindowClosed(WindowReasonChosen)
	p.npcWindow = &npcWindow{npc: npc.ID, options: []npcOption{}, charm: &charmSession{stage: charmResult}}
	p.pushAll("npc_window", npcWindowView(npc, text))
}

// charmChoose 处理美容师窗口的选项（npc_choose）。调用时要持有 p.mu。
func (s *Service) charmChoose(l *Live, r *record, w *npcWindow, npc gamedata.NPC, no int) *protocol.Error {
	switch w.charm.stage {
	case charmStart:
		switch no {
		case charmOptionGive:
			cost := charmPrice(r.Level, r.Charm, r.Transmigration)
			if cost < 0 { // 已满：直接给 OK 窗口，不报价不扣钱（原版 cost == -1）
				openCharmResult(l.p, npc, charmFullText)
				return nil
			}
			l.p.pushWindowClosed(WindowReasonChosen)
			l.p.npcWindow = &npcWindow{npc: npc.ID, options: charmAskOptions(), charm: &charmSession{stage: charmAsk}}
			text := fmt.Sprintf("%s\n现在,千禧特惠价实施中??\n\n要将你的魅力上升五点的话\n需要 %d 的 stone唷！\n\n即使这样也可以吗？", charmTitle, cost)
			l.p.pushAll("npc_window", npcWindowView(npc, text, charmAskOptions()...))
			return nil
		case charmOptionNot:
			l.p.pushWindowClosed(WindowReasonChosen)
			return nil
		}
	case charmAsk:
		switch no {
		case charmOptionYes:
			return s.charmApply(l, r, npc)
		case charmOptionNo:
			l.p.pushWindowClosed(WindowReasonChosen)
			return nil
		}
	case charmResult:
		l.p.pushWindowClosed(WindowReasonChosen) // OK 窗口：任意选择都关窗
		return nil
	}
	return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
}

// charmApply 执行一次美容：扣石币、魅力 +5 封顶 100，推 status_changed 和 pets_changed，再弹结果。
// 钱不够时回 NOT_ENOUGH_GOLD，窗口保持（不扣钱、不加魅力）。调用时要持有 p.mu。
func (s *Service) charmApply(l *Live, r *record, npc gamedata.NPC) *protocol.Error {
	cost := charmPrice(r.Level, r.Charm, r.Transmigration)
	if cost < 0 { // 报价之后魅力已满（理论上不会发生，窗口之间魅力不变）
		openCharmResult(l.p, npc, charmFullText)
		return nil
	}
	if r.Gold < int64(cost) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "%s", charmNotEnoughMsg)
	}
	r.Gold -= int64(cost)
	r.Charm = gamedata.ClampCharm(r.Charm + charmStepUp)
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushAll("pets_changed", r.petsView())
	openCharmResult(l.p, npc, fmt.Sprintf("%s\n\n嗯,这样你的魅力就上升罗！\n\n%s", charmTitle, charmImageComment(r.Appearance.Body)))
	return nil
}
