package character

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// 「实际活动」（docs/protocol.md 9.7，STAGE-119）：服务端按 agent 发出的操作类请求记录，和 agent_audit 同一个位置写入。

// activitySummaryMax 是摘要的最长字符数，超出截断并加省略号。
const activitySummaryMax = 40

// activityLabels 是操作类请求的中文类别名；没有登记的请求摘要里直接用请求类型名。
var activityLabels = map[string]string{
	"enter_world":    "进入世界",
	"leave_world":    "离开世界",
	"turn":           "转向",
	"warp":           "传送",
	"addpoint":       "加点",
	"talk":           "和 NPC 对话",
	"npc_choose":     "选择对话",
	"npc_reply":      "回复数量",
	"item_equip":     "穿装备",
	"item_unequip":   "卸装备",
	"item_move":      "移动物品",
	"item_drop":      "丢弃物品",
	"gold_drop":      "丢弃石币",
	"item_use":       "使用物品",
	"pet_state":      "设置宠物状态",
	"pet_rename":     "给宠物改名",
	"pet_drop":       "放生宠物",
	"battle_command": "战斗出招",
	"encounter":      "遇敌开关",
	"party_clone":    "组成分身队",
	"party_disband":  "解散队伍",
	"magic_use":      "使用精灵",
}

// activityItem 是「实际活动」的一行。Code 为 nil 表示成功，否则是被拒绝或失败的错误码。
type activityItem struct {
	TimeMs  int64   `json:"time_ms"`
	Request string  `json:"request"`
	Summary string  `json:"summary"`
	OK      bool    `json:"ok"`
	Code    *string `json:"code"`
}

// recordActivity 记下 agent 的一次操作类请求（不论成功或被拒绝），并实时推送给同一账号的玩家连接。
// 只在 auditRequest 里调用，调用条件与写 agent_audit 相同。
func (s *Service) recordActivity(sess *gateway.Session, req *protocol.Request, result string) {
	item := activityItem{TimeMs: s.now().UnixMilli(), Request: req.Type, Summary: activitySummary(req), OK: result == "ok"}
	if !item.OK {
		code := result
		item.Code = &code
	}
	s.agentBoardOf(sess.AccountID()).append(item)
	s.pushToPlayers(sess.Account(), "agent_activity", map[string]any{"full": false, "items": []activityItem{item}})
}

// activitySummary 生成摘要：类别名加上请求里的标量参数，按键名排序写成 `名=值`，例如「转向 direction=3」。
// data 不是对象或没有标量参数时只写类别名。
func activitySummary(req *protocol.Request) string {
	label, ok := activityLabels[req.Type]
	if !ok {
		label = req.Type
	}
	var fields map[string]any
	if json.Unmarshal(req.Data, &fields) != nil || len(fields) == 0 {
		return label
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if v, ok := scalarText(fields[k]); ok {
			parts = append(parts, k+"="+v)
		}
	}
	return truncateRunes(strings.TrimSpace(label+" "+strings.Join(parts, " ")), activitySummaryMax)
}

// scalarText 把字符串、数字、布尔值转成文字，去掉控制字符；其他类型（对象、数组、null）返回 false。
func scalarText(v any) (string, bool) {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case float64:
		s = strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		s = strconv.FormatBool(x)
	default:
		return "", false
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s), true
}

// truncateRunes 按字符数截断，超出时在末尾加省略号。
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}
