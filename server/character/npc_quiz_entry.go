package character

// STAGE-330：猜谜的参赛道具（原版 npc_quiz.c 的 NPC_EntryItemCheck / NPC_EntryItemDel / NPC_QUIZPARTY_CHAECK）。
// 参数 EntryItem 的写法是 `道具编号*个数`，多项用逗号分开（如 2752*1）；没有 * 的只看有没有，交的时候全部收走。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
)

// quizNeed 是参赛道具的一项。
type quizNeed struct {
	id   int
	n    int
	star bool // 写成 编号*个数：要 n 个，交的时候只收 n 个；否则只要有 1 个，交的时候全部收走（同原版）
}

// parseQuizNeeds 解析参赛道具参数；写错的项按 0 处理（原版 atoi 同样不报错）。
func parseQuizNeeds(spec string) []quizNeed {
	var out []quizNeed
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, n, star := part, 1, false
		if a, b, ok := strings.Cut(part, "*"); ok {
			id, star = a, true
			n = quizInt(b)
		}
		out = append(out, quizNeed{id: quizInt(id), n: n, star: star})
	}
	return out
}

func quizInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

// quizEntryOK 检查背包里的参赛道具够不够（没有参赛道具的人物恒为 true）。
func quizEntryOK(r *record, spec string) bool {
	if spec == "" {
		return true
	}
	for _, need := range parseQuizNeeds(spec) {
		have := 0
		for _, it := range r.Items {
			if it.ItemID == need.id {
				have += it.Count
			}
		}
		if have < need.n || (!need.star && have < 1) {
			return false
		}
	}
	return true
}

// quizEntryResult 是交出参赛道具之后的交易结果（只改背包，金币、任务进度和宠物都照旧）。
func quizEntryResult(r *record, spec string) eventResult {
	items := cloneItems(r.Items)
	for _, need := range parseQuizNeeds(spec) {
		if need.star {
			items = removeItems(items, need.id, need.n, false)
		} else {
			items = removeItems(items, need.id, 0, true)
		}
	}
	return eventResult{items: items, gold: r.Gold, charm: r.Charm, prog: progressOf(r), pets: r.Pets}
}

// inQuizTeam 报告角色是不是在队伍里（分身队或真人队，原版 CHAR_WORKPARTYMODE != CHAR_PARTY_NONE）：组队不能参加猜谜。
func inQuizTeam(p *Presence) bool {
	return p.party != nil || p.realParty.Load() != nil
}

// quizItemName 是道具的名字（拿到道具的消息用），查不到时用编号。
func quizItemName(id int) string {
	if t, ok := gamedata.ItemByID(id); ok && t.Name != "" {
		return t.Name
	}
	return strconv.Itoa(id)
}

// quizGrant 把谜之箱（id）放进 base 背包，返回交易结果（金币、宠物、任务进度照旧，消息栏提示“拿到XX。”）。
// 背包放不下时返回 false，什么都不改（原版 NPC_QuizAddItem：“道具栏已满”）。base 不会被改动。
func quizGrant(r *record, base []itemRec, id int) (eventResult, bool) {
	if freeBagSlots(base) == 0 {
		return eventResult{}, false
	}
	return eventResult{
		items: addItem(cloneItems(base), id), gold: r.Gold, charm: r.Charm, prog: progressOf(r), pets: r.Pets,
		notes: []string{fmt.Sprintf("拿到%s。", quizItemName(id))},
	}, true
}

// quizHolds 报告背包里有没有这件道具。
func quizHolds(r *record, id int) bool {
	for _, it := range r.Items {
		if it.ItemID == id {
			return true
		}
	}
	return false
}

// quizBorderText 是答对 correct 题时的台词：按 Border 的顺序取第一条阈值不大于 correct 的。
func quizBorderText(cfg *gamedata.NpcQuiz, correct int) string {
	for _, b := range cfg.Border {
		if correct >= b.Min {
			return quizText(b.Msg)
		}
	}
	return ""
}
