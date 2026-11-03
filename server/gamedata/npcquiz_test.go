package gamedata

// 猜谜题库与人物配置的数据检查（STAGE-329）：数量、关键人物的参数、与原版 NPC_QuizInit 的检查一致。
// 题库来自原版 data/question.txt（不是 racequiz.txt），原版第 8 行只有 8 个字段，加载时跳过，所以是 149 题。

import "testing"

func TestNpcQuizBankCount(t *testing.T) {
	if n := len(QuizBank()); n != 149 {
		t.Fatalf("题库应有 149 题（question.txt 150 行，第 8 行字段不足被原版跳过），实际 %d", n)
	}
	for _, q := range QuizBank() {
		if q.AnswerType != 1 && q.AnswerType != 2 && q.AnswerType != 4 {
			t.Errorf("第 %d 题答案类型 %d 不认识", q.No, q.AnswerType)
		}
		if q.Question == "" || len(q.Options) == 0 || q.Answer < 1 || q.Answer > len(q.Options) {
			t.Errorf("第 %d 题题干、选项或正确选项不合法：%+v", q.No, q)
		}
		if q.AnswerType == 4 && (len(q.Options) != 1 || q.Answer != 1) {
			t.Errorf("第 %d 题是文字题，应只有一个正确答案：%+v", q.No, q)
		}
	}
}

func TestNpcQuizCount(t *testing.T) {
	list := NpcQuizzes()
	if len(list) != 23 {
		t.Fatalf("猜谜人物应有 23 个（genout/quz_*），实际 %d", len(list))
	}
	placed := 0
	for _, c := range list {
		if c.Placed {
			placed++
		}
	}
	if placed != 22 {
		t.Errorf("放置的猜谜人物应有 22 个（quz_7777_20_20 在原版里被注释掉），实际 %d", placed)
	}
	if c := NpcQuizOf("quz_7777_20_20"); c == nil || c.Placed {
		t.Errorf("quz_7777_20_20 应存在但不放置：%+v", c)
	}
}

func TestNpcQuiz812(t *testing.T) {
	c := NpcQuizOf("quz_812_28_14")
	if c == nil {
		t.Fatal("没有 quz_812_28_14")
	}
	if c.Name != "猜谜大师" || c.Floor != 812 || !c.Placed {
		t.Errorf("认定场人物应是 812 的猜谜大师并放置，实际 %+v", c)
	}
	if c.Quiznum != 100 {
		t.Errorf("认定场应出 100 题，实际 %d", c.Quiznum)
	}
	if len(c.GetItem) != 1 || c.GetItem[0].Min != 100 || len(c.GetItem[0].Items) != 1 || c.GetItem[0].Items[0] != 2759 {
		t.Errorf("认定场全对应得谜之箱 2759，实际 GetItem=%+v", c.GetItem)
	}
	if c.EntryItem != "2753*1" {
		t.Errorf("认定场参赛要猜谜专家的认定证 2753*1，实际 %q", c.EntryItem)
	}
}

func TestNpcQuizEntryItemAndWarp(t *testing.T) {
	c := NpcQuizOf("quz_803_18_19")
	if c == nil || c.EntryItem != "2752*1" || len(c.Warp) != 2 {
		t.Fatalf("quz_803_18_19 参数不对：%+v", c)
	}
	if w := c.Warp[0]; w.Min != 2 || w.Floor != 803 || w.X != 18 || w.Y != 15 {
		t.Errorf("通过的传送点应是 803.18.15，实际 %+v", w)
	}
	if w := c.Warp[1]; w.Min != 0 || w.Floor != 802 || w.X != 20 || w.Y != 22 {
		t.Errorf("没过的传送点应是 802.20.22，实际 %+v", w)
	}
}

// 每个人物都能开一场（原版 NPC_QuizInit 检查 Quiznum 与能出的题数）。
func TestNpcQuizAllValidate(t *testing.T) {
	for _, c := range NpcQuizzes() {
		if err := c.Validate(QuizBank()); err != nil {
			t.Errorf("%v", err)
		}
	}
}

func TestNpcQuiz812Eligible(t *testing.T) {
	c := NpcQuizOf("quz_812_28_14")
	// Type:0、Answer:0 都是不限，Level:31 包含全部难度位，所以 149 题都能出。
	if n := len(c.Eligible(QuizBank())); n != 149 {
		t.Errorf("812 能出的题应是全部 149 题，实际 %d", n)
	}
}

func TestNpcQuiz7777Warp(t *testing.T) {
	c := NpcQuizOf("quz_7777_20_20")
	if len(c.Warp) != 2 || c.Warp[0].Min != 1 || c.Warp[0].Y != 22 || c.Warp[1].Min != 0 || c.Warp[1].Y != 21 {
		t.Errorf("7777 的传送应是通过去 20,22、没过去 20,21，实际 %+v", c.Warp)
	}
	if c.Quiznum != 1 || c.Border[0].Min != 1 {
		t.Errorf("7777 应出 1 题、答对才过，实际 Quiznum=%d Border=%+v", c.Quiznum, c.Border)
	}
}
