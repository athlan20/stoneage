package character

// STAGE-266：主城精灵王（人物转生）窗口，原版 npc_transmigration.c 的 TENSEI 段（台词在 npc_tensei_text.go）。
// talk 精灵王（类型 npc_tensei，不复用 npc_transmigration：那个类型先走宠物转生分流）：转生 5 次或等级不到 80
// 只说一句文字窗口；资格够了走窗口：开始（下一步）→ 确认（确定 / 取消）→ 执行 → 结果（下一步、确定）。
// 执行（tenseiExecute）在一个事务里完成数值、事件清空和赠宠（npc_tensei_store.go），规则见 npc_tensei_plan.go。

import (
	"context"
	"errors"
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// tenseiKind 是主城精灵王的 NPC 类型（gen-npcs.py 的 kind）。
const tenseiKind = "npc_tensei"

// tenseiMinLevel 是转生的最低等级（原版 npc_transmigration.c 的 Level < 80 不让转）。
const tenseiMinLevel = 80

// 窗口的步骤（tenseiSession.stage）。
const (
	tsStart     = iota // startmsg：只有“下一步”
	tsConfirm          // mainmsg：确定 / 取消
	tsCancelled        // 取消的台词：只有“确定”
	tsDone             // yesmsg1：只有“下一步”
	tsEnd              // yesmsg2：只有“确定”
)

// tenseiSession 是转生窗口的当前步骤（挂在 npcWindow.tensei 上，其他 NPC 为 nil）。
// texts 是开窗时取的台词（转生次数+1 对应的 TENSEI 段）：执行后转生次数已经加一，后面的台词仍用开窗时的。
type tenseiSession struct {
	stage int
	texts tenseiTexts
}

// tenseiQualify 按原版顺序检查资格：转生次数已到上限时说“最强的力量”那句，等级不到 80 说条件那句。
// 通过时 ok 为 true。
func tenseiQualify(r *record) (text string, ok bool) {
	if r.Transmigration >= TenseiMaxTrans {
		return tenseiMaxedMsg, false
	}
	if r.Level < tenseiMinLevel {
		return tenseiLowMsg, false
	}
	return "", true
}

// tenseiScriptOf 是当前转生次数对应的台词：转生 n 次时用 TENSEI:n+1（下标 n）。
func tenseiScriptOf(r *record) tenseiTexts {
	i := min(max(r.Transmigration, 0), len(tenseiScript)-1)
	return tenseiScript[i]
}

// tenseiYesNo 是“是 / 否”形式的两个选项，标签由调用方给。
func tenseiYesNo(yes, no string) []npcOption {
	return []npcOption{{No: 1, Label: yes}, {No: 2, Label: no}}
}

// tenseiOpen 打开（或替换）转生窗口，sess 为 nil 时只是一句话。调用时要持有 p.mu。
func tenseiOpen(p *Presence, npc gamedata.NPC, sess *tenseiSession, text string, options []npcOption) {
	p.npcWindow = &npcWindow{npc: npc.ID, tensei: sess}
	p.pushAll("npc_window", npcWindowView(npc, text, options...))
}

// talkTensei 是 talk 主城精灵王：资格不够只弹一句文字窗口，够了开始窗口。调用时要持有 p.mu。
func (s *Service) talkTensei(l *Live, r *record, npc gamedata.NPC) *protocol.Error {
	text, ok := tenseiQualify(r)
	if !ok {
		tenseiOpen(l.p, npc, nil, text, nil)
		return nil
	}
	tenseiOpen(l.p, npc, &tenseiSession{stage: tsStart, texts: tenseiScriptOf(r)}, tenseiStartMsg,
		[]npcOption{{No: 1, Label: "下一步"}})
	return nil
}

// tenseiChoose 处理转生窗口的选项（npc_choose）。调用时要持有 p.mu。
func (s *Service) tenseiChoose(ctx context.Context, l *Live, r *record, npc gamedata.NPC, sess *tenseiSession, no int) *protocol.Error {
	p := l.p
	switch sess.stage {
	case tsStart: // 下一步 → 确认
		if no != 1 {
			return tenseiBadChoice(no)
		}
		p.pushWindowClosed(WindowReasonChosen)
		tenseiOpen(p, npc, &tenseiSession{stage: tsConfirm, texts: sess.texts}, sess.texts.mainmsg, tenseiYesNo("确定", "取消"))
	case tsConfirm: // 确定 → 执行；取消 → 取消台词
		switch no {
		case 1:
			return s.tenseiExecute(ctx, l, r, npc, sess.texts)
		case 2:
			p.pushWindowClosed(WindowReasonChosen)
			tenseiOpen(p, npc, &tenseiSession{stage: tsCancelled}, tenseiCancelMsg, []npcOption{{No: 1, Label: "确定"}})
		default:
			return tenseiBadChoice(no)
		}
	case tsCancelled, tsEnd: // 确定：关窗
		if no != 1 {
			return tenseiBadChoice(no)
		}
		p.pushWindowClosed(WindowReasonChosen)
	case tsDone: // 下一步 → yesmsg2
		if no != 1 {
			return tenseiBadChoice(no)
		}
		p.pushWindowClosed(WindowReasonChosen)
		tenseiOpen(p, npc, &tenseiSession{stage: tsEnd, texts: sess.texts}, sess.texts.yesmsg2, []npcOption{{No: 1, Label: "确定"}})
	}
	return nil
}

// tenseiBadChoice 是选项编号不在窗口里时的错误（同 npc_choose 的其他窗口）。
func tenseiBadChoice(no int) *protocol.Error {
	return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
}

// tenseiPetsFullMsg 是宠物栏 5 格全满时的提示：整次转生拒绝，什么都不改。
const tenseiPetsFullMsg = "你的宠物栏满了，先腾出一个位置再来。"

// tenseiExecute 是确认后的转生：commitTensei 在一个事务里写完（能力、等级、事件清空、下骑、赠宠），成功后才改内存。
// 推送顺序：status_changed → pets_changed → quests_changed → 每只赠宠一条 notice → 弹 yesmsg1（yesmsg2 在下一步弹）。
// 宠物栏满时整次拒绝，只弹一句文字窗口。调用时要持有 p.mu。
func (s *Service) tenseiExecute(ctx context.Context, l *Live, r *record, npc gamedata.NPC, texts tenseiTexts) *protocol.Error {
	p := l.p
	if _, ok := tenseiQualify(r); !ok { // 窗口开着的时候资格变了（理论上不会）：不转
		return protocol.Errorf(protocol.CodeActionNotAllowed, "现在还不能转生")
	}
	oldQuests := r.Quests
	res, err := s.commitTensei(ctx, r, npc.Floor, s.petTransRnd())
	if errors.Is(err, errTenseiPetsFull) {
		p.pushWindowClosed(WindowReasonChosen)
		tenseiOpen(p, npc, nil, tenseiPetsFullMsg, nil)
		return nil
	}
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, "转生失败：%v", err)
	}
	p.players.touch(p) // STAGE-273：转生后等级和转生次数变了
	p.pushAll("status_changed", map[string]any{"status": r.status()})
	p.pushAll("pets_changed", r.petsView())
	if changed := changedQuests(oldQuests, r.Quests); len(changed) > 0 {
		p.pushAll("quests_changed", map[string]any{"quests": r.questsView(), "changed": changed})
	}
	p.pushAll("notice", map[string]string{"level": "info", "text": fmt.Sprintf("拿到%s。", res.petName)})
	p.pushWindowClosed(WindowReasonChosen)
	tenseiOpen(p, npc, &tenseiSession{stage: tsDone, texts: texts}, texts.yesmsg1, []npcOption{{No: 1, Label: "下一步"}})
	return nil
}

// tenseiApplyRecord 把一次转生的结果写进 r（纯内存，不碰数据库，单测直接用）：数值按 CalcTensei，
// 等级 1、经验 0、能力点 n×10（覆盖）、累计两列写回，HP 按新的最大值补满（MP 不动），骑着的宠物下骑。
// 骑乘等级（RideLevel）不动：原版 CHAR_LEARNRIDE 那行是注释掉的。
// 事件清空（planTenseiEvents）和赠宠（tenseiPresentOf）在 commitTensei 里做，和这里的结果放在同一个事务。
func tenseiApplyRecord(r *record) error {
	res, err := CalcTensei(TenseiInput{
		Vital: r.Stats.Vital, Strength: r.Stats.Strength, Toughness: r.Stats.Toughness, Dexterity: r.Stats.Dexterity,
		Level: r.Level, Transmigration: r.Transmigration,
		TransQuestTotal: r.TransQuestTotal, TransLevelTotal: r.TransLevelTotal,
	})
	if err != nil {
		return fmt.Errorf("算转生数值: %w", err)
	}
	r.Stats = Stats{Vital: res.Vital, Strength: res.Strength, Toughness: res.Toughness, Dexterity: res.Dexterity}
	r.Transmigration = res.Transmigration
	r.TransQuestTotal, r.TransLevelTotal = res.TransQuestTotal, res.TransLevelTotal
	r.Level = 1
	r.Exp = 0
	r.SkillPoints = res.SkillPoints
	r.HP = r.maxHP()
	for slot, st := range planPetUnride(r.Pets) {
		if pet := findPet(r.Pets, slot); pet != nil {
			pet.State = st
		}
	}
	return nil
}
