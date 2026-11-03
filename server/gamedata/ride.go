package gamedata

// 骑乘（STAGE-122，docs/ride.md）：骑乘等级、四级课程、骑乘表（人物外形 → 能骑的虎和雷龙）
// 和骑乘训练师的对话窗口数据（npc-riderman.json，由 tools/npcriderman.py 生成）。
// 规则出处：gmsv/npc/npc_riderman.c（课程与台词）、gmsv/char/char_base.c 的 ridePetTable（骑乘表）。

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"sync"
)

// 骑乘等级（原版 CHAR_LEARNRIDE）：能骑的宠物等级上限就是它，200 即不限（docs/ride.md §2）。
const (
	RideLevelNone    = 0
	RideLevelBasic   = 40  // 初级班
	RideLevelMiddle  = 80  // 中级班
	RideLevelHigh    = 120 // 高级班
	RideLevelSpecial = 200 // 特级班
)

// RideCourse 是一级课程：学会后骑乘等级变成 Level，Name 是课程名（台词里用，如“初级班”）。
type RideCourse struct {
	Level int
	Name  string
	// Allow 是学会后的描述：可以骑乘的宠物等级上限（特级是“所有等级”）。
	Allow string
}

// RideCourses 是四级课程，必须按顺序学（docs/ride.md §2）。
var RideCourses = [...]RideCourse{
	{RideLevelBasic, "初级班", "Lv40 以下的宠物"},
	{RideLevelMiddle, "中级班", "Lv80 以下的宠物"},
	{RideLevelHigh, "高级班", "Lv120 以下的宠物"},
	{RideLevelSpecial, "特级班", "所有等级的宠物"},
}

// RideCourseIndex 返回骑乘等级 level 对应的课程下标（0–3，即学的是第几级）；不是课程等级时 ok 为 false。
func RideCourseIndex(level int) (int, bool) {
	for i, c := range RideCourses {
		if c.Level == level {
			return i, true
		}
	}
	return -1, false
}

// 训练师的台词（npc_riderman.c 里写死的文字，不在 riderman.conf 里）。学费免费（docs/ride.md §1），
// 所以确认页的文字是改过的，不用原版的“需要 N 石币”。
func RideLearnedText(c RideCourse) string {
	return "你已经学会" + c.Name + "了呀。\n不需要再学了。"
}

// RidePrereqText 是没学前一级时的话：“你必须先学过 prev 才能来 cur 唷。”
func RidePrereqText(prev, cur RideCourse) string {
	return "你必须先学过" + prev.Name + "才能来" + cur.Name + "唷。"
}

// RideConfirmText 是确认页的正文（免费，docs/ride.md §1）。
func RideConfirmText(c RideCourse) string {
	return "学习骑乘" + c.Name + "不需要学费。你确定要学习吗？"
}

// RideLearnedNotice 是学会后的系统消息（原版恭喜话，不带学费）。
func RideLearnedNotice(c RideCourse) string {
	if c.Level == RideLevelSpecial {
		return "恭喜你！你已经完成骑乘" + c.Name + "了。\n可以骑乘所有等级的宠物，\n请小心骑乘喔！"
	}
	return "恭喜你！你已经完成骑乘" + c.Name + "了。\n可以骑乘 Lv" + strconv.Itoa(c.Level) + " 以下的宠物，\n请小心骑乘喔！"
}

// 骑乘表（原版 ridePetTable 的原生部分，docs/ride.md §3 第 10 条）。metamoImage 的行是虎的颜色，
// 列是雷龙的外形；身体图像编号（BodyImage）落在第 i 行第 j 列时，配套虎是 tigerByRow[i]、雷龙是 dragonByBody[j]。
// 出生配套宠（newplayerpets.go）和骑乘检查共用这一张表。
var metamoImage = [4][12]int{
	{100000, 100025, 100055, 100060, 100095, 100100, 100135, 100145, 100165, 100190, 100200, 100230}, // 红
	{100005, 100030, 100050, 100065, 100085, 100115, 100120, 100140, 100170, 100195, 100210, 100225}, // 绿
	{100010, 100035, 100045, 100070, 100090, 100110, 100125, 100150, 100160, 100185, 100215, 100220}, // 黄
	{100015, 100020, 100040, 100075, 100080, 100105, 100130, 100155, 100175, 100180, 100205, 100235}, // 灰
}

// tigerByRow 是配套虎的模板编号（按 metamoImage 的行）：红 格鲁西斯、绿 贝鲁卡、黄 金格萨贝鲁、灰 贝鲁伊卡。
var tigerByRow = [...]int{193, 191, 194, 192}

// dragonByBody 是配套雷龙的模板编号（按外形列）：布洛多斯、布林帖斯、布拉奇多斯、斯天多斯、邦恩多斯。
var dragonByBody = [...]int{252, 255, 251, 253, 255, 253, 254, 254, 252, 251, 253, 255}

// RidePair 返回这个人物外形（身体与颜色）配套的虎和雷龙模板编号；外形不在表里时 ok 为 false。
func RidePair(body, color int) (tiger, dragon int, ok bool) {
	if body < 0 || body >= BodyCount || color < 0 || color >= ColorCount {
		return 0, 0, false
	}
	image := BodyImage(body, color)
	for i := range metamoImage {
		for j := range metamoImage[i] {
			if metamoImage[i][j] == image {
				return tigerByRow[i], dragonByBody[j], true
			}
		}
	}
	return 0, 0, false
}

// RideableSpecies 返回这个人物外形能骑的宠物模板编号（虎、雷龙各一种）；没有时返回 nil。
func RideableSpecies(body, color int) []int {
	tiger, dragon, ok := RidePair(body, color)
	if !ok {
		return nil
	}
	return []int{tiger, dragon}
}

// RideAllowed 表示这个人物外形能不能骑这种宠物（不看等级，等级检查在 character 服务里）。
func RideAllowed(body, color, species int) bool {
	for _, s := range RideableSpecies(body, color) {
		if s == species {
			return true
		}
	}
	return false
}

// RidermanWindow 是骑乘训练师的一个对话窗口（npc-riderman.json）。窗口号按原版 winno，
// Options 的 Target 是选了之后跳到的窗口；Prev/Next/Yes/No 是按钮跳到的窗口（0 表示没有）。
type RidermanWindow struct {
	WinNo    int              `json:"winno"`
	Text     string           `json:"text"`
	Options  []RidermanOption `json:"options,omitempty"`
	Prev     int              `json:"prev,omitempty"`
	Next     int              `json:"next,omitempty"`
	Yes      int              `json:"yes,omitempty"`
	No       int              `json:"no,omitempty"`
	TakeGold int              `json:"takegold,omitempty"` // 原版学费，免费后不用
}

// RidermanOption 是窗口里的一个选项：Label 是文字，GoTo 是选了之后跳到的窗口号。
type RidermanOption struct {
	Label string `json:"label"`
	GoTo  int    `json:"gotowin"`
}

//go:embed npc-riderman.json
var npcRidermanJSON []byte

var (
	ridermanOnce sync.Once
	ridermanByNo map[int]RidermanWindow
)

// RidermanWindowOf 返回骑乘训练师的一个窗口；没有这个窗口号时 ok 为 false。
func RidermanWindowOf(no int) (RidermanWindow, bool) {
	ridermanOnce.Do(loadRiderman)
	w, ok := ridermanByNo[no]
	return w, ok
}

func loadRiderman() {
	ridermanByNo = map[int]RidermanWindow{}
	var list []RidermanWindow
	if err := json.Unmarshal(npcRidermanJSON, &list); err != nil {
		ridermanByNo = map[int]RidermanWindow{}
		return
	}
	for _, w := range list {
		ridermanByNo[w.WinNo] = w
	}
}
