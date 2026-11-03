package character

// 任务脚本的宠物动作（STAGE-197，对照 npc_exchangeman.c 的 NPC_AcceptDel、NPC_EventAdd、NPC_EventDelPet、
// NPC_EventAddPet）：DelPet（收宠物，直接写和 EVDEL）、GetPet（送宠物）。计算在 planEvent 里做（npcevent_deal.go），
// 结果和道具、金币一起在 commitEvent 的同一个事务里写库。这里只算宠物的新列表和消息栏的提示。

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/gamedata"
)

// eventSel 是一趟交易的选择：命中的候选组（EVDEL 用，没有候选组时是 -1）和是否在战斗中（战斗中不能收宠物）。
type eventSel struct {
	group  int
	battle bool
}

// eventSelOf 取这一趟交易的选择：命中的候选组（按当前时间重算，同选块）和连接是不是在战斗中。
func (s *Service) eventSelOf(l *Live, r *record, blk *gamedata.EventBlock) eventSel {
	return eventSel{group: eventMatchedGroup(r, blk.Cond, s.opts.Now()), battle: l.p.battle != nil}
}

// delPetAsks 是这一块要收的宠物项：DelPet 直接写的，或者 DelPet:EVDEL 里命中的那一组。
func delPetAsks(blk *gamedata.EventBlock, group int) []gamedata.PetAsk {
	if blk.DelPetEV != nil {
		if group >= 0 && group < len(blk.DelPetEV) {
			return blk.DelPetEV[group]
		}
		return nil
	}
	return blk.DelPet
}

// planPets 算宠物的收送，返回新的宠物列表（按栏位排序）、消息栏的提示和失败的台词。
//
// 收（NPC_AcceptDel 的 DelPet）：先检查每一项是不是都够（每项各自数，同原版；数的是随身的、不是任务送的），
// 不够或者在战斗中收不成，就什么都不收、不说话（原版直接返回 FALSE，没有台词）。够了再逐项收走：写了数量的收到
// 那么多，没写的（Count 0）全部收走，都按栏位从小到大；收的宠物先下骑（删掉这一行，状态就没了），消息“交出XX。”。
//
// 送（NPC_EventAddPet）：收完之后找第一个空的随身栏位，没有就说 PetFullMsg，整笔交易不做。有空位就从 GetPet 的
// 怪物里选一个（多个时随机，同原版 rand），按怪物模板生成宠物（等级、资质同捕捉的规则），消息“拿到XX。”。
// 任务送的标记（event_given）只在 REQUEST 接任务的路径上打（原版 NPC_EventAdd 的 mode 0）；ACCEPT 和 MESSAGE 经
// NPC_AcceptDel，原版不打标记，送出的宠物照常被普通 PET 条件数到。
func planPets(r *record, blk *gamedata.EventBlock, sel eventSel, rnd func(n int) int) (pets []petRec, notes []string, fail string, ok bool) {
	pets = append([]petRec(nil), r.Pets...)
	if asks := delPetAsks(blk, sel.group); len(asks) > 0 {
		if sel.battle {
			return nil, nil, "", false
		}
		for _, a := range asks {
			if petCount(r, a) < max(a.Count, 1) {
				return nil, nil, "", false
			}
		}
		for _, a := range asks {
			taken := 0
			for i := 0; i < len(pets) && (a.Count == 0 || taken < a.Count); {
				p := pets[i]
				if !p.carried() || p.EventGiven || !a.Match(p.TemplateID, p.Level) {
					i++
					continue
				}
				notes = append(notes, fmt.Sprintf("交出%s。", p.Name))
				pets = append(pets[:i], pets[i+1:]...)
				taken++
			}
		}
	}
	if len(blk.GetPet) > 0 {
		slot := emptyCarriedSlot(pets)
		if slot < 0 {
			return nil, nil, blk.Msg.PetFull, false
		}
		enemyID := blk.GetPet[0]
		if len(blk.GetPet) > 1 {
			enemyID = blk.GetPet[rnd(len(blk.GetPet))]
		}
		p, ok := newEventPet(enemyID, slot, rnd, blk.Type == "REQUEST")
		if !ok {
			return nil, nil, blk.Msg.PetFull, false // 加载时已经查过，这里查不到只会是数据不一致，按满处理
		}
		pets = append(pets, p)
		notes = append(notes, fmt.Sprintf("拿到%s。", p.Name))
	}
	sort.Slice(pets, func(i, j int) bool { return pets[i].Slot < pets[j].Slot })
	return pets, notes, "", true
}

// emptyCarriedSlot 返回第一个空的随身栏位，没有返回 -1（原版 NPC_EventAddPet 从 0 开始找）。
func emptyCarriedSlot(pets []petRec) int {
	var used [MaxPetSlots]bool
	for _, p := range pets {
		if p.carried() {
			used[p.Slot] = true
		}
	}
	for slot, u := range used {
		if !u {
			return slot
		}
	}
	return -1
}

// newEventPet 按怪物编号生成送出的宠物（原版 ENEMY_createPetFromEnemyIndex）：等级在敌人的等级范围里随机
// （RAND，范围两端都算），资质和能力用 gamedata.NewPetGrowth，血量满，技能跟着模板走。flagged 为真时打上任务送的标记。
func newEventPet(enemyID, slot int, rnd func(n int) int, flagged bool) (petRec, bool) {
	enemy, ok := gamedata.EnemyByID(enemyID)
	if !ok {
		return petRec{}, false
	}
	tmpl, ok := gamedata.PetByID(enemy.Template)
	if !ok {
		return petRec{}, false
	}
	level := enemy.LVMin
	if enemy.LVMax > enemy.LVMin {
		level += rnd(enemy.LVMax - enemy.LVMin + 1)
	}
	base, stats := gamedata.NewPetGrowth(tmpl, level, rnd)
	return petRec{
		Slot: slot, TemplateID: tmpl.ID, Name: tmpl.Name, Level: level,
		Stats: stats, Base: base, Rank: gamedata.PetRank(tmpl),
		HP: gamedata.PetMaxHP(stats), State: petStateRest,
		Skills: tmpl.Skills, EventGiven: flagged,
		Origin: &PetOrigin{Level: level, Stats: stats}, // STAGE-215：起始值，生成时的等级和能力
	}, true
}

// samePets 报告两份宠物列表是不是一样（commitEvent 用它判断要不要写库、推送）。
func samePets(a, b []petRec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writePetDiff 把宠物的变化写进 character_pets（在交易的事务里）：栏位上的宠物没了就删（骑乘中的删掉这一行，
// 状态也就没了，同原版先下骑再收），新的或变了的就插入（先全部删，再全部插，栏位不会冲突）。
func writePetDiff(ctx context.Context, tx pgx.Tx, charID int64, before, after []petRec) error {
	oldBy := make(map[int]petRec, len(before))
	for _, p := range before {
		oldBy[p.Slot] = p
	}
	newBy := make(map[int]petRec, len(after))
	for _, p := range after {
		newBy[p.Slot] = p
	}
	for slot, old := range oldBy {
		if cur, ok := newBy[slot]; ok && cur == old {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM character_pets WHERE character_id = $1 AND slot = $2`, charID, slot); err != nil {
			return err
		}
	}
	for slot, p := range newBy {
		if old, ok := oldBy[slot]; ok && old == p {
			continue
		}
		if err := insertEventPet(ctx, tx, charID, p); err != nil {
			return err
		}
	}
	return nil
}

// insertEventPet 写一只宠物的整行（在事务里用）：和 insertPetRow 同样的列（起始值取自 p.Origin），多写 event_given。
func insertEventPet(ctx context.Context, tx pgx.Tx, charID int64, p petRec) error {
	skills := make([]int32, len(p.Skills))
	for i, id := range p.Skills {
		skills[i] = int32(id)
	}
	oLevel, oVital, oStrength, oToughness, oDexterity := originArgs(p.Origin)
	_, err := tx.Exec(ctx, `INSERT INTO character_pets (`+petInsertColumns+`, event_given)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)`,
		charID, p.Slot, p.TemplateID, p.Name, p.Level, p.Exp,
		p.Stats.Vital, p.Stats.Strength, p.Stats.Toughness, p.Stats.Dexterity,
		p.Base.Vital, p.Base.Strength, p.Base.Toughness, p.Base.Dexterity,
		p.Rank, p.VariableAI, p.HP, p.State, skills,
		oLevel, oVital, oStrength, oToughness, oDexterity, p.EventGiven)
	return err
}
