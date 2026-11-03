package gamedata

// 猜谜的抽题、判题和过关（STAGE-329，原版 gmsv/npc/npc_quiz.c）。纯逻辑：不碰角色、窗口和协议，
// 对话层（下一单）用 NewQuizRun 开一场，Next 出题，AnswerChoice / AnswerText 收答案，Result 算结果。
//
// 与原版的对应和差异：
//   - 抽题：原版随机抽一题，抽到出过的就重抽（5000 次后兜底允许重复）；这里是不放回的均匀抽样，
//     分布相同，没有兜底（能出的题数不少于 Quiznum 时，5000 次连续重复的概率可以忽略）。
//   - 选项：正确选项放在随机位置（原版 NPC_QuizRandAnswer）。原版在“正确选项为 3 且放在第 1 位”的分支里，
//     两个干扰项的顺序固定（复制粘贴的笔误），这里统一随机，只影响干扰项的顺序，不影响判题。
//   - 文字题：原版 strstr(输入, 正确答案)，即包含判断，区分大小写，不做全角半角转换。
//     任务书里写的“大小写、全角半角”归一化在原版里没有，这里按原版不做。
//   - 选择题：原版用选项序号比对（atoi）。输入 0 不算作答，窗口不推进；其他序号（包括超出选项范围的）都算作答并推进，
//     超出范围当然算错。
//   - 过关：原版没有“过关”标记，只按 Border 查到的那条台词发。这里把查到的那条阈值大于 0 的算过关；
//     数据里每个 Border 都以阈值 0 的失败台词结尾，所以查到阈值 0 的就是没过关。
//   - Border、GetItem、Warp 都是“答对题数不小于阈值时取列表里第一条”（原版 NPC_QuizBorderLine、NPC_QuizWarpBorderLine）。
//
// 没有实现的（属于对话层或下一单）：参赛道具和团队检查（EntryItem、Party）、道具栏是否放得下（ItemFullMsg）、
// 玩家离开后释放状态（NPC_QuizReleasePlayer）、两个人同时挑战的排队。

import (
	"fmt"
	"strings"
)

// quizMaxNum 是原版 OLDNO：一场最多 100 题（NPC_QuizInit 检查 Quiznum）。
const quizMaxNum = 100

// quizMask 把位掩码的 <=0 当作不限（0xffff），同原版 NPC_GetQuestion。
func quizMask(v int) int {
	if v > 0 {
		return v
	}
	return 0xFFFF
}

// Eligible 返回能出的题在题库里的下标（按题库顺序）。条件同原版 NPC_GetQuestion：
// 题型位、答案类型、难度位都必须是对应掩码的子集。
func (c *NpcQuiz) Eligible(bank []QuizQuestion) []int {
	tm, am, lm := quizMask(c.TypeMask), quizMask(c.AnswerMask), quizMask(c.LevelMask)
	var out []int
	for i, q := range bank {
		if q.Type < 1 || tm&(1<<(q.Type-1)) != 1<<(q.Type-1) {
			continue
		}
		if am&q.AnswerType != q.AnswerType || lm&q.Level != q.Level {
			continue
		}
		out = append(out, i)
	}
	return out
}

// Validate 同原版 NPC_QuizInit 的检查：Quiznum 在 1..100 之间，且不超过能出的题数。
// 原版允许 Quiznum 为 0，但数据里没有，这里当作错误。
func (c *NpcQuiz) Validate(bank []QuizQuestion) error {
	if c.Quiznum < 1 || c.Quiznum > quizMaxNum {
		return fmt.Errorf("%s：Quiznum=%d 超出 1..%d", c.File, c.Quiznum, quizMaxNum)
	}
	if n := len(c.Eligible(bank)); c.Quiznum > n {
		return fmt.Errorf("%s：Quiznum=%d 超过能出的题数 %d", c.File, c.Quiznum, n)
	}
	return nil
}

// QuizAsk 是正在问的一题。Options 是选择题的选项（按显示顺序，序号从 1 起）；文字题没有选项。
type QuizAsk struct {
	No      int      // 第几题，从 1 起
	Text    string   // 题干
	Typed   bool     // 文字题：玩家自己写答案
	Options []string // 选择题的选项
	right   int      // 选择题：正确选项的序号
	want    string   // 文字题：正确答案
}

// QuizRun 是一位玩家一场猜谜的状态：已出的题、答对的题数，以及正在问的题。
type QuizRun struct {
	cfg     *NpcQuiz
	bank    []QuizQuestion
	pool    []int // 能出的题（题库下标）；前 asked 个已经出过
	asked   int
	cur     *QuizAsk
	correct int
}

// NewQuizRun 开一场猜谜；人物参数不合法（见 Validate）时返回错误。
func NewQuizRun(cfg *NpcQuiz, bank []QuizQuestion) (*QuizRun, error) {
	if err := cfg.Validate(bank); err != nil {
		return nil, err
	}
	return &QuizRun{cfg: cfg, bank: bank, pool: cfg.Eligible(bank)}, nil
}

// Next 出下一题。rnd(n) 返回 [0, n) 的随机数。上一题还没答、或者已经出够 Quiznum 题时返回 false。
// 抽题是从还没出过的题里均匀抽一题，然后随机排选项。
func (r *QuizRun) Next(rnd func(n int) int) (QuizAsk, bool) {
	if r.cur != nil || r.asked >= r.cfg.Quiznum {
		return QuizAsk{}, false
	}
	i := r.asked + rnd(len(r.pool)-r.asked)
	r.pool[r.asked], r.pool[i] = r.pool[i], r.pool[r.asked]
	q := r.bank[r.pool[r.asked]]
	r.asked++
	ask := QuizAsk{No: r.asked, Text: q.Question, Typed: q.AnswerType == 4}
	if ask.Typed {
		ask.want = q.Options[0]
	} else {
		ask.Options, ask.right = arrangeOptions(q, rnd)
	}
	r.cur = &ask
	return ask, true
}

// arrangeOptions 把正确选项放到随机位置，其余选项顺序打乱后排在剩下的位置。返回选项和正确选项的序号（从 1 起）。
func arrangeOptions(q QuizQuestion, rnd func(int) int) ([]string, int) {
	var wrong []string
	for i, o := range q.Options {
		if i != q.Answer-1 {
			wrong = append(wrong, o)
		}
	}
	for i := len(wrong) - 1; i > 0; i-- {
		j := rnd(i + 1)
		wrong[i], wrong[j] = wrong[j], wrong[i]
	}
	pos := rnd(len(q.Options))
	out := make([]string, 0, len(q.Options))
	out = append(out, wrong[:pos]...)
	out = append(out, q.Options[q.Answer-1])
	out = append(out, wrong[pos:]...)
	return out, pos + 1
}

// AnswerChoice 收选择题的答案（序号从 1 起）。输入 0 不算作答，返回 accepted=false，题目还在。
// 其他序号算作答并推进到下一题，correct 表示是否等于正确选项。当前题不是选择题时返回 accepted=false。
func (r *QuizRun) AnswerChoice(n int) (correct, accepted bool) {
	if r.cur == nil || r.cur.Typed || n == 0 {
		return false, false
	}
	correct = n == r.cur.right
	r.finish(correct)
	return correct, true
}

// AnswerText 收文字题的答案：输入里包含正确答案就算对（原版 strstr，区分大小写，不做全角半角转换）。
// 文字题只要提交就推进，不论对错。当前题不是文字题时返回 accepted=false。
func (r *QuizRun) AnswerText(input string) (correct, accepted bool) {
	if r.cur == nil || !r.cur.Typed {
		return false, false
	}
	correct = strings.Contains(input, r.cur.want)
	r.finish(correct)
	return correct, true
}

func (r *QuizRun) finish(correct bool) {
	if correct {
		r.correct++
	}
	r.cur = nil
}

// Finished 是否已经出够 Quiznum 题并全部答完。
func (r *QuizRun) Finished() bool {
	return r.cur == nil && r.asked >= r.cfg.Quiznum
}

// QuizResult 是一场猜谜的结果。
type QuizResult struct {
	Correct int       // 答对题数
	Total   int       // Quiznum
	Passed  bool      // 查到的 Border 阈值大于 0
	Message string    // 查到的 Border 台词；一条都没查到时用 FailureMsg（数据里每个 Border 都有阈值 0 的兜底，不会出现）
	Item    int       // GetItem 发的道具模板编号；0 表示不发
	Warp    *QuizWarp // 传送去的地方；nil 表示不传
}

// Result 算结果，取值规则见文件开头。GetItem 的候选道具用 rnd 随机取一个（原版 NPC_QuizAddItem）。
func (r *QuizRun) Result(rnd func(n int) int) QuizResult {
	c := r.cfg
	res := QuizResult{Correct: r.correct, Total: c.Quiznum, Message: c.FailureMsg}
	for _, b := range c.Border {
		if r.correct >= b.Min {
			res.Message = b.Msg
			res.Passed = b.Min > 0
			break
		}
	}
	for _, g := range c.GetItem {
		if r.correct >= g.Min {
			if len(g.Items) > 0 {
				res.Item = g.Items[rnd(len(g.Items))]
			}
			break
		}
	}
	for _, w := range c.Warp {
		if r.correct >= w.Min {
			wc := w
			res.Warp = &wc
			break
		}
	}
	return res
}
