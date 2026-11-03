package character

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// agent 的计划（docs/protocol.md 9.7，STAGE-119）：agent 自己报的目标和步骤，每次整份替换，只存在内存里。
const (
	agentGoalMax  = 100 // 目标最长字符数
	agentStepsMax = 20  // 步骤最多项数
	agentStepMax  = 60  // 每一步的文字最长字符数
)

// 步骤的状态取值。
const (
	stepTodo  = "todo"  // 待做
	stepDoing = "doing" // 进行中
	stepDone  = "done"  // 已完成
)

// planStep 是计划里的一步。
type planStep struct {
	Text  string `json:"text"`
	State string `json:"state"`
}

// agentPlan 是当前计划，推送给玩家时就是这个结构；Goal 为空表示还没有计划。
type agentPlan struct {
	Goal        string     `json:"goal"`
	Steps       []planStep `json:"steps"`
	UpdatedAtMs int64      `json:"updated_at_ms"`
}

// emptyPlan 是没有计划时的样子：步骤是空数组而不是 null，JSON 里写成 []。
func emptyPlan() agentPlan { return agentPlan{Steps: []planStep{}} }

// handleAgentPlan 是 agent 报告计划（docs/protocol.md 9.7）：agent 连接，登录后即可发，不要求在世界中；
// 整份替换当前计划，并推送给同一账号的玩家连接。没有单独的频率限制，和 agent_say 一样。
func (s *Service) handleAgentPlan(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Goal  *string    `json:"goal"`
		Steps []planStep `json:"steps"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Goal == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 goal 字段")
	}
	plan, perr := checkPlan(*in.Goal, in.Steps, s.now())
	if perr != nil {
		return nil, perr
	}
	s.agentBoardOf(sess.AccountID()).setPlan(plan)
	s.pushToPlayers(sess.Account(), "agent_plan", plan)
	return map[string]any{}, nil
}

// checkPlan 检查计划的长度、条数、状态取值；不合法回复 BAD_REQUEST（docs/protocol.md 9.7）。
func checkPlan(goal string, steps []planStep, now time.Time) (agentPlan, *protocol.Error) {
	goal, ok := checkText(goal, agentGoalMax)
	if !ok {
		return agentPlan{}, protocol.Errorf(protocol.CodeBadRequest, "目标要在 1–%d 个字符之间，不能含控制字符", agentGoalMax)
	}
	if len(steps) > agentStepsMax {
		return agentPlan{}, protocol.Errorf(protocol.CodeBadRequest, "步骤最多 %d 项，现在有 %d 项", agentStepsMax, len(steps))
	}
	plan := agentPlan{Goal: goal, Steps: make([]planStep, 0, len(steps)), UpdatedAtMs: now.UnixMilli()}
	for i, st := range steps {
		text, ok := checkText(st.Text, agentStepMax)
		if !ok {
			return agentPlan{}, protocol.Errorf(protocol.CodeBadRequest, "第 %d 步的文字要在 1–%d 个字符之间，不能含控制字符", i+1, agentStepMax)
		}
		switch st.State {
		case stepTodo, stepDoing, stepDone:
		default:
			return agentPlan{}, protocol.Errorf(protocol.CodeBadRequest, "第 %d 步的状态只能是 %s、%s 或 %s", i+1, stepTodo, stepDoing, stepDone)
		}
		plan.Steps = append(plan.Steps, planStep{Text: text, State: st.State})
	}
	return plan, nil
}

// checkText 去掉首尾空白，检查字符数（按 UTF-8 字符计）和控制字符；返回去掉空白后的文字。
func checkText(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n == 0 || n > max {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}
