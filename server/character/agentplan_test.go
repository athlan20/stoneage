package character

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-119：agent 计划的长度、条数和状态校验（docs/protocol.md 9.7）。
func TestCheckPlan(t *testing.T) {
	now := time.UnixMilli(1760000000000)
	long := strings.Repeat("字", agentGoalMax+1)

	plan, perr := checkPlan("  带主角去加加村  ", []planStep{{Text: "走路", State: stepDoing}}, now)
	if perr != nil {
		t.Fatalf("合法计划被拒绝: %v", perr)
	}
	if plan.Goal != "带主角去加加村" || plan.UpdatedAtMs != now.UnixMilli() || len(plan.Steps) != 1 {
		t.Fatalf("计划内容不对: %+v", plan)
	}

	bad := map[string]func() (agentPlan, *protocol.Error){
		"目标为空白":   func() (agentPlan, *protocol.Error) { return checkPlan("   ", nil, now) },
		"目标超长":    func() (agentPlan, *protocol.Error) { return checkPlan(long, nil, now) },
		"目标含控制字符": func() (agentPlan, *protocol.Error) { return checkPlan("a\x07b", nil, now) },
		"步骤超过 20 项": func() (agentPlan, *protocol.Error) {
			return checkPlan("目标", make([]planStep, agentStepsMax+1), now)
		},
		"步骤文字为空": func() (agentPlan, *protocol.Error) {
			return checkPlan("目标", []planStep{{Text: " ", State: stepTodo}}, now)
		},
		"步骤状态非法": func() (agentPlan, *protocol.Error) {
			return checkPlan("目标", []planStep{{Text: "走路", State: "running"}}, now)
		},
	}
	for name, f := range bad {
		if _, perr := f(); perr == nil || perr.Code != protocol.CodeBadRequest {
			t.Errorf("%s：应当回复 BAD_REQUEST，实际 %v", name, perr)
		}
	}
}

// 步骤上限正好 20 项可以，没有步骤也可以（steps 可以省略，编码成 []）。
func TestCheckPlanLimits(t *testing.T) {
	now := time.UnixMilli(1)
	steps := make([]planStep, agentStepsMax)
	for i := range steps {
		steps[i] = planStep{Text: "走路", State: stepDone}
	}
	plan, perr := checkPlan("目标", steps, now)
	if perr != nil || len(plan.Steps) != agentStepsMax {
		t.Fatalf("20 项步骤应当可以：%v", perr)
	}
	plan, perr = checkPlan("目标", nil, now)
	if perr != nil {
		t.Fatalf("没有步骤应当可以：%v", perr)
	}
	if b, _ := json.Marshal(plan); !strings.Contains(string(b), `"steps":[]`) {
		t.Errorf("没有步骤时应当编码成 []，实际 %s", b)
	}
}

// 「补发和清空」用的 reset：返回清空前是否非空，第二次清空时都是空的。
func TestBoardReset(t *testing.T) {
	b := &agentBoard{plan: emptyPlan()}
	if hadPlan, hadItems := b.reset(); hadPlan || hadItems {
		t.Fatalf("空面板清空时不应报告有内容：%v %v", hadPlan, hadItems)
	}
	b.setPlan(agentPlan{Goal: "目标", Steps: []planStep{}})
	b.append(activityItem{Request: "turn", Summary: "转向", OK: true})
	if hadPlan, hadItems := b.reset(); !hadPlan || !hadItems {
		t.Fatalf("有内容时清空应当报告：%v %v", hadPlan, hadItems)
	}
	if plan, items := b.snapshot(); plan.Goal != "" || len(items) != 0 {
		t.Fatalf("清空后应当为空：%+v %v", plan, items)
	}
}

// 活动最多保存 200 条，超出时丢掉最旧的。
func TestBoardActivityCap(t *testing.T) {
	b := &agentBoard{plan: emptyPlan()}
	for i := 0; i < agentActivityMax+5; i++ {
		b.append(activityItem{TimeMs: int64(i), Request: "turn", OK: true})
	}
	_, items := b.snapshot()
	if len(items) != agentActivityMax {
		t.Fatalf("应当保留 %d 条，实际 %d", agentActivityMax, len(items))
	}
	if items[0].TimeMs != 5 || items[len(items)-1].TimeMs != agentActivityMax+4 {
		t.Fatalf("应当丢掉最旧的 5 条：首条 %d，末条 %d", items[0].TimeMs, items[len(items)-1].TimeMs)
	}
}
