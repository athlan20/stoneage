package gamedata

// 猜谜人物（STAGE-329，原版 gmsv/npc/npc_quiz.c）的题库与人物配置。
// 数据由 tools/npcquiz.py 生成（npc-quiz-questions.json、npc-quiz.json），勿手改，改了就重新运行脚本。
// 题库来自 setup.cf 的 quizfile=data/question.txt；人物参数来自 genout/quz_*，名字、楼层、朝向来自 quizman.create。
// 抽题、判题和过关的规则在 npcquiz_logic.go。

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed npc-quiz-questions.json
var npcQuizQuestionsJSON []byte

//go:embed npc-quiz.json
var npcQuizJSON []byte

// QuizQuestion 是题库里的一题。
// AnswerType 1 是两选一（Options 两个）、2 是三选一（三个）、4 是文字题（Options 只有正确答案一个，玩家自己写）。
// Answer 是正确选项的序号（从 1 起，即原版 answerNo），文字题恒为 1。
// Type 是题型位（原版 type，题目位是 1<<(Type-1)），Level 是难度位（原版 level）。
type QuizQuestion struct {
	No         int      `json:"no"`
	Type       int      `json:"type"`
	Level      int      `json:"level"`
	AnswerType int      `json:"answertype"`
	Answer     int      `json:"answer"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
}

// QuizStep 是 Border 的一条：答对题数不小于 Min 时取这条台词（列表按原版顺序查，取第一条匹配的）。
type QuizStep struct {
	Min int    `json:"min"`
	Msg string `json:"msg"`
}

// QuizItemStep 是 GetItem 的一条：答对题数不小于 Min 时，从 Items 里随机发一件（原版 NPC_QuizAddItem）。
type QuizItemStep struct {
	Min   int   `json:"min"`
	Items []int `json:"items"`
}

// QuizWarp 是 Warp 的一条：答对题数不小于 Min 时传送到 Floor 的 (X, Y)。
type QuizWarp struct {
	Min   int `json:"min"`
	Floor int `json:"floor"`
	X     int `json:"x"`
	Y     int `json:"y"`
}

// NpcQuiz 是一个猜谜人物的参数。
// TypeMask、AnswerMask、LevelMask 是题目筛选的位掩码，<=0 当作不限（同原版 NPC_GetQuestion）。
// Placed 为 false 的人物（quz_7777_20_20）在原版 quizman.create 里是注释掉的，不刷出来。
type NpcQuiz struct {
	File        string         `json:"file"`
	Placed      bool           `json:"placed"`
	Name        string         `json:"name"`
	Floor       int            `json:"floor"`
	Corner      [4]int         `json:"corner"`
	Dir         int            `json:"dir"`
	Graphic     int            `json:"graphic"`
	TypeMask    int            `json:"type_mask"`
	AnswerMask  int            `json:"answer_mask"`
	LevelMask   int            `json:"level_mask"`
	Quiznum     int            `json:"quiznum"`
	Border      []QuizStep     `json:"border"`
	GetItem     []QuizItemStep `json:"get_item,omitempty"`
	Warp        []QuizWarp     `json:"warp,omitempty"`
	StartMsg    string         `json:"start_msg"`
	FailureMsg  string         `json:"failure_msg"`
	ItemFullMsg string         `json:"item_full_msg,omitempty"`
	NoEntryMsg  string         `json:"no_entry_msg,omitempty"`
	PartyMsg    string         `json:"party_msg,omitempty"`
	EntryItem   string         `json:"entry_item,omitempty"`
}

var (
	npcQuizOnce      sync.Once
	npcQuizQuestions []QuizQuestion
	npcQuizList      []*NpcQuiz
	npcQuizByFile    map[string]*NpcQuiz
)

func loadNpcQuiz() {
	if err := json.Unmarshal(npcQuizQuestionsJSON, &npcQuizQuestions); err != nil {
		panic("npc-quiz-questions.json: " + err.Error())
	}
	var list []NpcQuiz
	if err := json.Unmarshal(npcQuizJSON, &list); err != nil {
		panic("npc-quiz.json: " + err.Error())
	}
	npcQuizByFile = make(map[string]*NpcQuiz, len(list))
	for i := range list {
		c := &list[i]
		npcQuizList = append(npcQuizList, c)
		npcQuizByFile[c.File] = c
	}
}

// QuizBank 返回题库（按原版文件顺序）。返回的切片不要修改。
func QuizBank() []QuizQuestion {
	npcQuizOnce.Do(loadNpcQuiz)
	return npcQuizQuestions
}

// NpcQuizOf 按参数文件名（如 quz_812_28_14）返回猜谜人物的参数；没有这个人物时返回 nil。
func NpcQuizOf(file string) *NpcQuiz {
	npcQuizOnce.Do(loadNpcQuiz)
	return npcQuizByFile[file]
}

// NpcQuizzes 返回全部猜谜人物（按参数文件名排序，包括没有放置的）。
func NpcQuizzes() []*NpcQuiz {
	npcQuizOnce.Do(loadNpcQuiz)
	return npcQuizList
}
