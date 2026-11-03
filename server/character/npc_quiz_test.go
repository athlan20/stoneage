package character

// STAGE-330：猜谜人物的纯逻辑测试（不需要数据库）：参数对应、参赛道具、座位表（上限 8、走开清掉、释放）、
// 距离和台词换行。对话的整条流程（交道具、发道具）在 casetest 的用例里看。

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

func TestQuizConfigOfMaster(t *testing.T) {
	n := 0
	for _, npc := range gamedata.NPCsByFloor(812) {
		if npc.Kind == quizKind {
			n++
			if cfg := quizConfigOf(npc); cfg == nil || cfg.Name != "猜谜大师" || cfg.Quiznum != 100 || cfg.EntryItem != "2753*1" {
				t.Errorf("812 的猜谜大师参数不对: %+v", cfg)
			}
		}
	}
	if n != 1 {
		t.Errorf("812 应有 1 个猜谜大师，实际 %d", n)
	}
	if quizConfigOf(gamedata.NPC{Kind: "npcgen_man", Floor: 812, X: 28, Y: 14}) != nil {
		t.Error("非猜谜类型不应有猜谜参数")
	}
}

func TestParseQuizNeeds(t *testing.T) {
	got := parseQuizNeeds("2752*1, 2753 ,")
	want := []quizNeed{{id: 2752, n: 1, star: true}, {id: 2753, n: 1, star: false}}
	if len(got) != len(want) {
		t.Fatalf("解析得到 %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 项 = %+v，应为 %+v", i, got[i], want[i])
		}
	}
	if len(parseQuizNeeds("")) != 0 {
		t.Error("空参数应没有要求")
	}
}

func TestQuizEntryCheckAndTake(t *testing.T) {
	r := &record{Items: []itemRec{{Slot: 10, ItemID: 2752, Count: 1}, {Slot: 11, ItemID: 9, Count: 1}}}
	if !quizEntryOK(r, "") || !quizEntryOK(r, "2752*1") {
		t.Error("有参赛券或没有参赛道具要求时应通过")
	}
	if quizEntryOK(r, "2753*1") {
		t.Error("没有 2753 时不应通过")
	}
	if quizEntryOK(r, "2752*2") {
		t.Error("只有 1 张时要 2 张不应通过")
	}
	res := quizEntryResult(r, "2752*1")
	if len(res.items) != 1 || res.items[0].ItemID != 9 {
		t.Errorf("交出 1 张参赛券后背包应只剩 9 号道具，实际 %+v", res.items)
	}
	if len(r.Items) != 2 {
		t.Error("quizEntryResult 不应改动原背包")
	}
}

func TestQuizSeatsCapacityAndRelease(t *testing.T) {
	h := newQuizHub()
	npc := gamedata.NPC{ID: 1814, Floor: 800, X: 29, Y: 19, Kind: quizKind}
	var seats []*quizSeat
	for i := 0; i < quizMaxPlayers; i++ {
		p := &Presence{rec: record{Floor: 800, X: 29, Y: 20}}
		seat, ok := h.acquire(p, npc)
		if !ok {
			t.Fatalf("第 %d 个人应能占座位", i+1)
		}
		seats = append(seats, seat)
	}
	if _, ok := h.acquire(&Presence{rec: record{Floor: 800, X: 29, Y: 20}}, npc); ok {
		t.Error("第 9 个人应等待（MEPLAYER=8）")
	}
	// 释放一个座位后，别人能进；释放两次没有影响。
	h.release(seats[0])
	h.release(seats[0])
	if h.isActive(seats[0]) {
		t.Error("释放后的座位不应有效")
	}
	if _, ok := h.acquire(&Presence{rec: record{Floor: 800, X: 29, Y: 20}}, npc); !ok {
		t.Error("释放后应有空位")
	}
	// 其他人的座位和别的人物无关。
	other := gamedata.NPC{ID: 1815, Floor: 801, X: 18, Y: 19, Kind: quizKind}
	if _, ok := h.acquire(&Presence{rec: record{Floor: 801, X: 18, Y: 20}}, other); !ok {
		t.Error("另一位人物应有自己的座位")
	}
}

func TestQuizSeatEvictsAbsentOwner(t *testing.T) {
	h := newQuizHub()
	npc := gamedata.NPC{ID: 1814, Floor: 800, X: 29, Y: 19, Kind: quizKind}
	var seats []*quizSeat
	for i := 0; i < quizMaxPlayers; i++ {
		seat, _ := h.acquire(&Presence{rec: record{Floor: 800, X: 29, Y: 20}}, npc)
		seats = append(seats, seat)
	}
	// 第一个人走到别的图去了：下一个人来时他的座位被清掉（原版 NPC_RealyCheack）。
	seats[0].owner.rec.Floor = 801
	if _, ok := h.acquire(&Presence{rec: record{Floor: 800, X: 29, Y: 20}}, npc); !ok {
		t.Fatal("走开的人占的座位应被清掉")
	}
	if h.isActive(seats[0]) {
		t.Error("被清掉的座位应标记为无效，窗口上的人再答题时要关窗")
	}
	if !h.isActive(seats[1]) {
		t.Error("还在旁边的人的座位不应被清掉")
	}
}

func TestQuizSeatKeepsOwnerWhoIsNearby(t *testing.T) {
	h := newQuizHub()
	npc := gamedata.NPC{ID: 1814, Floor: 800, X: 29, Y: 19, Kind: quizKind}
	p := &Presence{rec: record{Floor: 800, X: 30, Y: 20}} // 斜着一格，仍然算在旁边
	seat, _ := h.acquire(p, npc)
	if !seat.stillNear() {
		t.Error("切比雪夫距离 1 应算在旁边")
	}
	p.rec.X = 31
	if seat.stillNear() {
		t.Error("离开两格应算走开")
	}
}

func TestQuizDistanceAndText(t *testing.T) {
	npc := gamedata.NPC{ID: 1814, Floor: 812, X: 28, Y: 14, Kind: quizKind, Name: "猜谜大师"}
	if quizDistance(&record{Floor: 812, X: 29, Y: 15}, npc) != nil {
		t.Error("斜一格应能说话")
	}
	if quizDistance(&record{Floor: 812, X: 30, Y: 14}, npc) == nil {
		t.Error("离两格应太远")
	}
	if quizDistance(&record{Floor: 800, X: 28, Y: 14}, npc) == nil {
		t.Error("不在同一张图应太远")
	}
	if got := quizText(`\n跟我以猜谜比赛胜负\n全部答对`); got != "\n跟我以猜谜比赛胜负\n全部答对" {
		t.Errorf("字面 \\n 应换成换行，得到 %q", got)
	}
}

// quizMasterCfg 是认定场猜谜大师（quz_812_28_14）的参数。
func quizMasterCfg(t *testing.T) *gamedata.NpcQuiz {
	t.Helper()
	cfg := gamedata.NpcQuizOf("quz_812_28_14")
	if cfg == nil {
		t.Fatal("没有猜谜大师的参数")
	}
	return cfg
}

// STAGE-332：agent 直接认定的纯计算：通过时扣 2753、发 2759，原记录不动；没有入门证、已经持有箱子时不改数据。
func TestQuizAgentPlanGrantsBox(t *testing.T) {
	cfg := quizMasterCfg(t)
	r := kingRecord(kingItems(quizPass), []int{quizQuestEvent}, nil)
	out := quizAgentPlan(r, cfg)
	if out.kind != qaGrant {
		t.Fatalf("有 2753 应直接通过，实际 kind=%d msg=%q", out.kind, out.msg)
	}
	if countItem(out.next.items, quizPass) != 0 || countItem(out.next.items, quizBox) != 1 {
		t.Errorf("通过后应扣 2753、发 2759，实际 %v", out.next.items)
	}
	if !strings.Contains(out.msg, "100题全都答对") {
		t.Errorf("认定台词不对: %q", out.msg)
	}
	if countItem(r.Items, quizPass) != 1 || countItem(r.Items, quizBox) != 0 {
		t.Errorf("规划不应改动原记录，实际 %v", r.Items)
	}
}

func TestQuizAgentPlanNoEntryAndHolding(t *testing.T) {
	cfg := quizMasterCfg(t)
	out := quizAgentPlan(kingRecord(nil, []int{quizQuestEvent}, nil), cfg)
	if out.kind != qaNoEntry || !strings.Contains(out.msg, "你没有猜谜大师的认定证") {
		t.Errorf("没有 2753 应说 NoEntryMsg，实际 kind=%d msg=%q", out.kind, out.msg)
	}
	out = quizAgentPlan(kingRecord(kingItems(quizPass, quizBox), []int{quizQuestEvent}, nil), cfg)
	if out.kind != qaHolding || len(out.next.items) != 0 {
		t.Errorf("已经持有谜之箱时只说台词，实际 kind=%d", out.kind)
	}
}

// quizAnswerFor 找出这一题的正确答案（按题干和选项对题库），返回 choice 的序号或文字题的答案。
func quizAnswerFor(t *testing.T, bank []gamedata.QuizQuestion, ask gamedata.QuizAsk) (int, string) {
	t.Helper()
	for _, q := range bank {
		if q.Question != ask.Text || (q.AnswerType == 4) != ask.Typed {
			continue
		}
		if ask.Typed {
			return 0, q.Options[0]
		}
		want := q.Options[q.Answer-1]
		for i, o := range ask.Options {
			if o == want {
				return i + 1, ""
			}
		}
	}
	t.Fatalf("题库里找不到这一题：%q", ask.Text)
	return 0, ""
}

// STAGE-332：人类答满 100 题（全对）的判题、过关和发奖：Result 给 2759，背包放得下时 quizGrant 发成。
func TestQuizHumanFullMarksGrantsBox(t *testing.T) {
	cfg := quizMasterCfg(t)
	bank := gamedata.QuizBank()
	run, err := gamedata.NewQuizRun(cfg, bank)
	if err != nil {
		t.Fatal(err)
	}
	rnd := rand.New(rand.NewPCG(7, 11)).IntN
	for !run.Finished() {
		ask, ok := run.Next(rnd)
		if !ok {
			t.Fatal("出题失败")
		}
		if ask.Typed {
			_, ans := quizAnswerFor(t, bank, ask)
			run.AnswerText(ans)
		} else {
			no, _ := quizAnswerFor(t, bank, ask)
			if correct, _ := run.AnswerChoice(no); !correct {
				t.Fatalf("第 %d 题选 %d 应判对", ask.No, no)
			}
		}
	}
	res := run.Result(rnd)
	if res.Correct != 100 || !res.Passed || res.Item != quizBoxItem {
		t.Fatalf("全对应过关并给 2759，实际 %+v", res)
	}
	r := kingRecord(nil, []int{quizQuestEvent}, nil)
	next, ok := quizGrant(r, r.Items, res.Item)
	if !ok || countItem(next.items, quizBoxItem) != 1 {
		t.Fatalf("空背包应放得下谜之箱，实际 ok=%v items=%v", ok, next.items)
	}
	full := make([]itemRec, 0, MaxItemSlot-firstBagSlot+1)
	for slot := firstBagSlot; slot <= MaxItemSlot; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: 1, Count: 1})
	}
	if _, ok := quizGrant(r, full, res.Item); ok {
		t.Error("背包满时不应发出谜之箱")
	}
}
