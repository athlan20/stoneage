package character

import (
	"encoding/json"
	"testing"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
)

// keepMonster 造一只 1 号模板的怪物：体力档位 −3（资质比模板少 1，tier = 资质 − (模板 + 2)），
// 腕力、耐久力、速度都是 0 档，所以 keep 只有体力会挡住它。
func keepMonster(t *testing.T) *battle.Unit {
	t.Helper()
	tmpl, ok := gamedata.PetByID(1)
	if !ok {
		t.Fatal("没有 1 号宠物模板")
	}
	return &battle.Unit{
		Name: "乌力", TemplateID: 1, Level: 3, HP: 10,
		Growth: gamedata.Growth{
			Vital:     tmpl.Base.Vital - 1,
			Strength:  tmpl.Base.Strength + 2,
			Toughness: tmpl.Base.Toughness + 2,
			Dexterity: tmpl.Base.Dexterity + 2,
		},
	}
}

func intPtr(v int) *int { return &v }

// TestCaptureOutcomeKeep：不合格不入库、返回 discarded；合格入栏位；不设 keep 照旧入栏位。
func TestCaptureOutcomeKeep(t *testing.T) {
	m := keepMonster(t)
	if q := captureQuality(m); q[0].Tier != -3 || q[1].Tier != 0 {
		t.Fatalf("构造的怪物档位不对: %+v", q)
	}

	r := &record{Pets: []petRec{{Slot: 0, TemplateID: 1}}}
	slot, dis := captureOutcome(r, m, &battle.CaptureKeep{Vital: intPtr(-2)})
	if slot != -1 || dis == nil {
		t.Fatalf("体力 −3 < −2 应被筛掉，实际 slot=%d dis=%+v", slot, dis)
	}
	if dis.Name != "乌力" || dis.Level != 3 || dis.Reason != "体力 −3 < −2" || len(dis.Quality) != 4 || dis.Quality[0].Tier != -3 {
		t.Errorf("discarded 内容不对: %+v", dis)
	}
	if len(r.Pets) != 1 {
		t.Errorf("被筛掉的宠物不能入库，实际 %d 只", len(r.Pets))
	}

	slot, dis = captureOutcome(r, m, &battle.CaptureKeep{Vital: intPtr(-3), Strength: intPtr(0)})
	if slot != 1 || dis != nil || len(r.Pets) != 2 {
		t.Errorf("合格的应入 1 号栏：slot=%d dis=%+v pets=%d", slot, dis, len(r.Pets))
	}

	r2 := &record{}
	if slot, dis := captureOutcome(r2, m, nil); slot != 0 || dis != nil {
		t.Errorf("不设 keep 应照旧入栏：slot=%d dis=%+v", slot, dis)
	}
}

// TestCaptureOutcomeFull：宠位满时不看资质，返回 -1 且没有 discarded；引擎据此判抓取失败（STAGE-267）。
func TestCaptureOutcomeFull(t *testing.T) {
	m := keepMonster(t)
	r := &record{}
	for slot := 0; slot < MaxPetSlots; slot++ {
		r.Pets = append(r.Pets, petRec{Slot: slot, TemplateID: 1})
	}
	slot, dis := captureOutcome(r, m, &battle.CaptureKeep{Vital: intPtr(-2)})
	if slot != -1 || dis != nil || len(r.Pets) != MaxPetSlots {
		t.Errorf("宠位满且不合格：应仍是 -1、无 discarded、宠位不变，实际 slot=%d dis=%+v pets=%d", slot, dis, len(r.Pets))
	}
}

// TestAutoPlanCaptureKeep：方案里的 capture.keep 解析进 PlanCapture；取值不对整个方案 ignored。
func TestAutoPlanCaptureKeep(t *testing.T) {
	ok := json.RawMessage(`{"version":1,"capture":{"enabled":true,"keep":{"vital":-2}}}`)
	plan, state := parseAutoPlan(ok)
	if state != autoProfileApplied || plan == nil || plan.Capture.Keep == nil || *plan.Capture.Keep.Vital != -2 {
		t.Fatalf("keep 应进方案: state=%s plan=%+v", state, plan)
	}
	bad := json.RawMessage(`{"version":1,"capture":{"enabled":true,"keep":{"vital":-7}}}`)
	if plan, state := parseAutoPlan(bad); state != autoProfileIgnored || plan != nil {
		t.Errorf("越界的 keep 应 ignored: state=%s plan=%+v", state, plan)
	}
	none := json.RawMessage(`{"version":1,"capture":{"enabled":true}}`)
	if plan, _ := parseAutoPlan(none); plan == nil || plan.Capture.Keep != nil {
		t.Errorf("没带 keep 应为不限: %+v", plan)
	}
}
