package gamedata

// NPC 对话参数（STAGE-69，docs/protocol.md 5.33）：npcs.tsv 里每个 NPC 说什么，由
// tools/gen-npcs.py 从原版参数文件（data/npc/genout 的 msg_*/signb_*/tman_*）生成。
// 规则出处是原版服务端的 NPC 实现：
//   - 对话（npcgen_man）：gmsv/npc/npc_townpeople.c，参数按半角逗号分成多句，说话时随机取一句；
//   - 告示牌（npcgen_signboard）：gmsv/npc/npc_signboard.c，固定一段文字，`\n` 是字面两个字符；
//   - 报时（npcgen_timeman）：gmsv/npc/npc_timeman.c，time 是游戏时间段（ALLNOON 等），
//     时段内说 main_msg、时段外说 change_msg；游戏时钟沿用 handletime.c 的现实时间映射；
//   - 摆设（npcgen_mugon）：没有参数，固定回「……」，所以这里没有它的记录。
// 查不到记录的对话类 NPC 同样按「……」处理：原版数据快照偶尔缺参数文件（生成器会跳过），
// 服务端不因此拒绝对话。

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

//go:embed npc-talk.json
var npcTalkJSON []byte

// NpcTalk 是一个 NPC 的对话参数。Lines / Change 里已经按原版分好句。
type NpcTalk struct {
	Lines  []string `json:"lines,omitempty"`  // 对话、报时的 main_msg：随机说一句
	Text   string   `json:"text,omitempty"`   // 告示牌：固定文字（\n 是字面两个字符）
	Time   string   `json:"time,omitempty"`   // 报时的游戏时间段（原版 time 参数）
	Change []string `json:"change,omitempty"` // 报时的 change_msg：时段外说的话
}

// MugonLine 是摆设（npcgen_mugon）和一切没有参数的 NPC 的默认回答（STAGE-69）。
const MugonLine = "……"

var (
	talkOnce   sync.Once
	talkByID   map[int]NpcTalk
	talkBroken bool // 数据文件坏掉时置位：全部 NPC 都按「……」处理，不拦着启动
)

// NpcTalkOf 返回一个 NPC 的对话参数；没有参数（含编号不存在）时返回 nil。
func NpcTalkOf(id int) *NpcTalk {
	talkOnce.Do(loadNpcTalk)
	if t, ok := talkByID[id]; ok {
		return &t
	}
	return nil
}

// TalkBroken 报告对话参数数据是否没能加载（测试和诊断用）。
func TalkBroken() bool {
	talkOnce.Do(loadNpcTalk)
	return talkBroken
}

func loadNpcTalk() {
	talkByID = map[int]NpcTalk{}
	var list []NpcTalkRecord
	if err := json.Unmarshal(npcTalkJSON, &list); err != nil {
		talkBroken = true
		talkByID = map[int]NpcTalk{}
		return
	}
	for _, r := range list {
		talkByID[r.ID] = r.NpcTalk
	}
}

// NpcTalkRecord 是 npc-talk.json 的一行：编号加对话参数（数组的次序就是表里的编号序）。
type NpcTalkRecord struct {
	ID int `json:"id"`
	NpcTalk
}

// TalkText 从对话参数里选出这次要说的话：告示牌原文里的字面 `\n` 转成真换行；
// 多句的随机取一句。此方法保留 main_msg 优先的静态口径；服务端按当前游戏时间调用
// TalkTextAt。参数为 nil（没有参数）时返回摆设的默认回答。
func (t *NpcTalk) TalkText(rnd func(n int) int) string {
	if t == nil {
		return MugonLine
	}
	return t.talkText(t.Lines, rnd)
}

// TalkTextAt 按原版 npc_timeman.c 的游戏时间段选择 main_msg 或 change_msg，再随机取一句。
// 游戏日为 5400 秒，分成 1024 个游戏时刻；其纪元与时段边界沿用旧 handletime.c。
// 当 NPC 只有其中一组台词时，全天使用现有台词，避免无 change_msg 的村民夜间变成空白。
func (t *NpcTalk) TalkTextAt(now time.Time, rnd func(n int) int) string {
	if t == nil {
		return MugonLine
	}
	lines := t.Lines
	if len(lines) == 0 {
		lines = t.Change // 没有 main_msg 的报时 NPC，时段外的话就是它唯一的话
	} else if len(t.Change) > 0 && t.Time != "" && !timemanActive(t.Time, lsHour(now)) {
		lines = t.Change
	}
	return t.talkText(lines, rnd)
}

func (t *NpcTalk) talkText(lines []string, rnd func(n int) int) string {
	if t == nil {
		return MugonLine
	}
	if t.Text != "" {
		return strings.ReplaceAll(t.Text, `\n`, "\n")
	}
	if len(lines) == 0 {
		lines = t.Change
	}
	if len(lines) == 0 {
		return MugonLine
	}
	if rnd == nil {
		return lines[0]
	}
	return lines[rnd(len(lines))]
}

type timemanPeriod struct {
	name       string
	born, dead int
}

// npc_timeman.c 的 TimeTble；范围端点按原版严格比较，不包含 born/dead 本身。
var timemanPeriods = [...]timemanPeriod{
	{name: "ALLNIGHT", born: 301, dead: 700},
	{name: "ALLNOON", born: 701, dead: 300},
	{name: "AM", born: 501, dead: 125},
	{name: "PM", born: 126, dead: 500},
	{name: "FORE", born: 701, dead: 125},
	{name: "AFTER", born: 126, dead: 300},
	{name: "EVNING", born: 301, dead: 500},
	{name: "MORNING", born: 501, dead: 700},
	{name: "FREE", born: 0, dead: 1024},
}

func lsHour(now time.Time) int {
	const (
		secondsPerDay = int64(5400)
		eraUnix       = int64(912766409 + 5400) // handletime.c 的 era
		hoursPerDay   = int64(1024)
	)
	seconds := (now.Unix() - eraUnix) % secondsPerDay
	if seconds < 0 {
		seconds += secondsPerDay
	}
	return int(seconds * hoursPerDay / secondsPerDay)
}

func timemanActive(period string, hour int) bool {
	for _, p := range timemanPeriods {
		if !strings.Contains(period, p.name) {
			continue
		}
		if p.born < p.dead {
			return p.born < hour && hour < p.dead
		}
		return p.born < hour && hour < 1024 || 0 < hour && hour < p.dead
	}
	return true // 未识别时段沿用 main_msg，避免把 NPC 对话吞掉。
}
