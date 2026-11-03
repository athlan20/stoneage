package gamedata

// 猜谜抽题、判题、过关的单元测试（STAGE-329，规则见 npcquiz_logic.go）。

import (
	"math/rand"
	"testing"
)

// quizTestBank 是一份小题库：三选一、两选一、文字题、三选二各一题。
func quizTestBank() []QuizQuestion {
	return []QuizQuestion{
		{No: 1, Type: 1, Level: 1, AnswerType: 2, Answer: 2, Question: "三选", Options: []string{"甲", "乙", "丙"}},
		{No: 2, Type: 1, Level: 2, AnswerType: 1, Answer: 1, Question: "两选", Options: []string{"A", "B"}},
		{No: 3, Type: 1, Level: 4, AnswerType: 4, Answer: 1, Question: "写", Options: []string{"Abc"}},
		{No: 4, Type: 1, Level: 8, AnswerType: 2, Answer: 3, Question: "三选二", Options: []string{"x", "y", "z"}},
	}
}

func seededRand(seed int64) func(int) int {
	return rand.New(rand.NewSource(seed)).Intn
}

func TestNpcQuizEligibleMasks(t *testing.T) {
	bank := quizTestBank()
	// 难度位 3（1|2）：只有难度 1 和 2 的题能出；答案类型不限，题型位 1。
	c := &NpcQuiz{TypeMask: 1, LevelMask: 3, Quiznum: 1}
	got := c.Eligible(bank)
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("难度位 3 应筛出题 0、1，实际 %v", got)
	}
	// 答案类型掩码 4：只有文字题能出。
	c = &NpcQuiz{AnswerMask: 4, Quiznum: 1}
	if got := c.Eligible(bank); len(got) != 1 || got[0] != 2 {
		t.Errorf("答案类型掩码 4 应只筛出文字题，实际 %v", got)
	}
	// 题型掩码 2（只要题型位 2）：题目都是题型 1，一题都不出。
	c = &NpcQuiz{TypeMask: 2, Quiznum: 1}
	if got := c.Eligible(bank); len(got) != 0 {
		t.Errorf("题型掩码 2 不该筛出题目，实际 %v", got)
	}
}

func TestNpcQuizNextNoRepeat(t *testing.T) {
	bank := quizTestBank()
	c := &NpcQuiz{Quiznum: 4, Border: []QuizStep{{Min: 0, Msg: "失败"}}}
	for seed := int64(1); seed <= 50; seed++ {
		r, err := NewQuizRun(c, bank)
		if err != nil {
			t.Fatal(err)
		}
		rnd := seededRand(seed)
		seen := map[string]bool{}
		for i := 0; i < 4; i++ {
			ask, ok := r.Next(rnd)
			if !ok {
				t.Fatalf("第 %d 题应能出", i+1)
			}
			if seen[ask.Text] {
				t.Fatalf("seed %d 题目重复：%s", seed, ask.Text)
			}
			seen[ask.Text] = true
			if ask.No != i+1 {
				t.Errorf("题号应从 1 起连续，实际 %d", ask.No)
			}
			if _, ok := r.Next(rnd); ok {
				t.Fatal("上一题没答不该出下一题")
			}
			if ask.Typed {
				r.AnswerText("Abc")
			} else {
				r.AnswerChoice(1)
			}
		}
		if _, ok := r.Next(rnd); ok {
			t.Error("出够 Quiznum 题后不该再出")
		}
		if !r.Finished() {
			t.Error("答完应算结束")
		}
	}
}

// 选择题的正确选项要在显示的选项里，位置随机，干扰项不变。
func TestNpcQuizArrangeOptions(t *testing.T) {
	q := quizTestBank()[0] // 正确是“乙”
	pos := map[int]bool{}
	for seed := int64(1); seed <= 200; seed++ {
		opts, right := arrangeOptions(q, seededRand(seed))
		if opts[right-1] != "乙" {
			t.Fatalf("seed %d：第 %d 个选项应是正确答案，实际 %v", seed, right, opts)
		}
		if len(opts) != 3 || !(has(opts, "甲") && has(opts, "丙")) {
			t.Fatalf("seed %d：选项缺项：%v", seed, opts)
		}
		pos[right] = true
	}
	if len(pos) != 3 {
		t.Errorf("正确选项应在三个位置都出现过，实际位置 %v", pos)
	}
}

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestNpcQuizAnswerChoice(t *testing.T) {
	bank := quizTestBank()
	// 答案类型掩码 1：只有两选一的题能出（bank[1]，正确是 A）。
	c := &NpcQuiz{TypeMask: 1, AnswerMask: 1, Quiznum: 1, Border: []QuizStep{{Min: 0, Msg: "失败"}}}
	r, err := NewQuizRun(c, bank)
	if err != nil {
		t.Fatal(err)
	}
	ask, ok := r.Next(seededRand(7))
	if !ok || ask.Typed || len(ask.Options) != 2 {
		t.Fatalf("应出一道两选一：%+v", ask)
	}
	if _, acc := r.AnswerChoice(0); acc {
		t.Error("输入 0 不算作答")
	}
	if _, acc := r.AnswerText("A"); acc {
		t.Error("选择题不接受文字答案")
	}
	correct, acc := r.AnswerChoice(ask.right)
	if !acc || !correct {
		t.Errorf("选中正确选项应算对：correct=%v accepted=%v", correct, acc)
	}
	if !r.Finished() {
		t.Error("只出 1 题，答完应结束")
	}
}

func TestNpcQuizAnswerTextContains(t *testing.T) {
	bank := []QuizQuestion{{No: 1, Type: 1, Level: 1, AnswerType: 4, Answer: 1, Question: "写", Options: []string{"Abc"}}}
	c := &NpcQuiz{Quiznum: 1}
	cases := []struct {
		input string
		want  bool
	}{
		{"Abc", true},
		{"xxAbcyy", true}, // 包含即算对
		{"abc", false},    // 原版 strstr 区分大小写
		{"ＡＢＣ", false},    // 全角不转换
		{"", false},
	}
	for _, tc := range cases {
		r, err := NewQuizRun(c, bank)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := r.Next(seededRand(1)); !ok {
			t.Fatal("应出题")
		}
		correct, acc := r.AnswerText(tc.input)
		if !acc || correct != tc.want {
			t.Errorf("输入 %q：correct=%v accepted=%v，期望 correct=%v", tc.input, correct, acc, tc.want)
		}
	}
}

func TestNpcQuizResultFromData(t *testing.T) {
	bank := QuizBank()
	c := NpcQuizOf("quz_812_28_14")
	noRand := func(int) int { return 0 }

	pass := &QuizRun{cfg: c, correct: 100, asked: 100}
	res := pass.Result(noRand)
	if !res.Passed || res.Item != 2759 || res.Warp != nil || res.Message != c.Border[0].Msg {
		t.Errorf("812 全对应过关、得 2759、不传送，实际 %+v", res)
	}

	fail := &QuizRun{cfg: c, correct: 99, asked: 100}
	res = fail.Result(noRand)
	if res.Passed || res.Item != 0 || res.Message != c.Border[1].Msg {
		t.Errorf("812 差一题应没过、不得道具，实际 %+v", res)
	}

	// 参数本身也要能开场：全部 149 题都在题库里。
	if _, err := NewQuizRun(c, bank); err != nil {
		t.Errorf("812 应能开场：%v", err)
	}
}

func TestNpcQuizResultWarp(t *testing.T) {
	c := NpcQuizOf("quz_7777_20_20")
	noRand := func(int) int { return 0 }

	res := (&QuizRun{cfg: c, correct: 1, asked: 1}).Result(noRand)
	if !res.Passed || res.Warp == nil || res.Warp.Floor != 7777 || res.Warp.X != 20 || res.Warp.Y != 22 {
		t.Errorf("7777 答对应传到 7777.20.22，实际 %+v", res)
	}
	res = (&QuizRun{cfg: c, correct: 0, asked: 1}).Result(noRand)
	if res.Passed || res.Warp == nil || res.Warp.Y != 21 || res.Message != c.Border[1].Msg {
		t.Errorf("7777 答错应传到 7777.20.21、台词用失败的，实际 %+v", res)
	}
}

func TestNpcQuizResultItemPick(t *testing.T) {
	c := &NpcQuiz{Quiznum: 2, Border: []QuizStep{{Min: 1, Msg: "过"}, {Min: 0, Msg: "没过"}},
		GetItem: []QuizItemStep{{Min: 1, Items: []int{10, 20, 30}}}}
	r := &QuizRun{cfg: c, correct: 1, asked: 2}
	if res := r.Result(func(n int) int { return n - 1 }); res.Item != 30 || !res.Passed {
		t.Errorf("候选里随机取一个，这里取最后一个 30，实际 %+v", res)
	}
	r.correct = 0
	if res := r.Result(func(n int) int { return 0 }); res.Item != 0 || res.Passed || res.Message != "没过" {
		t.Errorf("没达到阈值不发道具，实际 %+v", res)
	}
}

func TestNpcQuizValidate(t *testing.T) {
	bank := quizTestBank()
	if err := (&NpcQuiz{Quiznum: 0}).Validate(bank); err == nil {
		t.Error("Quiznum 0 应报错")
	}
	if err := (&NpcQuiz{Quiznum: 101}).Validate(bank); err == nil {
		t.Error("Quiznum 超过 100 应报错")
	}
	if err := (&NpcQuiz{Quiznum: 5}).Validate(bank); err == nil {
		t.Error("题数不够应报错")
	}
	if _, err := NewQuizRun(&NpcQuiz{Quiznum: 5}, bank); err == nil {
		t.Error("NewQuizRun 应拒绝题数不够的人物")
	}
}
