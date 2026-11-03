package character

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// 不连数据库：战斗外使用精灵（STAGE-203，magic_use）的各个分支，直接用内存里的角色。
// 精灵栏 0 是变幻的精灵 Lv1（350，一分钟，MP 8），1 是岩石撞击（301，战斗精灵），其余空。
func metamoRecord() *record {
	return &record{
		Name: "测试", Level: 10, HP: 100, MP: 50,
		Magics: [5]int{350, 301, -1, -1, -1},
		Pets: []petRec{
			{Slot: 0, TemplateID: 1, Name: "乌力", State: petStateStandby},
			{Slot: 1, TemplateID: 21, Name: "加美", State: petStateStandby}, // 加美：不能变身成它
		},
	}
}

func slotPtr(v int) *int { return &v }

func TestMagicUseChecks(t *testing.T) {
	s := &Service{}
	cases := []struct {
		name   string
		mutate func(r *record)
		in     magicUseRequest
		code   string
	}{
		{"没有 slot", nil, magicUseRequest{}, protocol.CodeBadRequest},
		{"slot 越界", nil, magicUseRequest{Slot: slotPtr(5)}, protocol.CodeBadRequest},
		{"target 越界", nil, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(MaxPetSlots)}, protocol.CodeBadRequest},
		{"栏位是空的", nil, magicUseRequest{Slot: slotPtr(2)}, protocol.CodeMagicUnusable},
		{"不是变幻的精灵", nil, magicUseRequest{Slot: slotPtr(1)}, protocol.CodeMagicUnusable},
		{"目标栏位没有宠物", nil, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(3)}, protocol.CodeInvalidTarget},
		{"骑乘中", func(r *record) { r.Pets[0].State = petStateRide }, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(0)}, protocol.CodeRequirementNotMet},
		{"手上是弓", func(r *record) {
			r.Items = []itemRec{{Slot: 2, ItemID: 400, Count: 1}}
		}, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(0)}, protocol.CodeRequirementNotMet},
		{"不能变成加美", nil, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(1)}, protocol.CodeRequirementNotMet},
		{"MP 不够", func(r *record) { r.MP = 7 }, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(0)}, protocol.CodeNotEnoughMP},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := metamoRecord()
			if c.mutate != nil {
				c.mutate(r)
			}
			mpBefore := r.MP
			view, perr := s.useMagic(newMetamoPresence(), r, c.in, time.Unix(1000, 0))
			if perr == nil {
				t.Fatalf("应该失败（%s），结果 view=%+v", c.code, view)
			}
			if perr.Code != c.code {
				t.Errorf("错误码 %s，应为 %s（%s）", perr.Code, c.code, perr.Message)
			}
			if perr.Message == "" {
				t.Error("失败要有给玩家看的说明")
			}
			if r.metamo != nil || r.MP != mpBefore {
				t.Errorf("失败不能改状态：metamo=%+v MP=%d", r.metamo, r.MP)
			}
		})
	}
}

// 变身成功：扣 MP，状态里有宠物名、图像编号和到期时间；再选自己变回，MP 照扣，变身清掉。
func TestMagicUseMetamoAndRevert(t *testing.T) {
	s := &Service{}
	r := metamoRecord()
	p := newMetamoPresence()
	now := time.Unix(1000, 0)

	out, perr := s.useMagic(p, r, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(0)}, now)
	if perr != nil {
		t.Fatalf("变身失败：%v", perr)
	}
	view := metamoOf(out)
	if view == nil || view.Pet != "乌力" || view.Image != 100250 || view.Until != now.Add(time.Minute).UnixMilli() {
		t.Fatalf("变身后的状态 = %+v", view)
	}
	if r.MP != 42 {
		t.Errorf("变身扣的 MP：剩 %d，应为 42（50-8）", r.MP)
	}
	defer stopMetamo(r)

	out, perr = s.useMagic(p, r, magicUseRequest{Slot: slotPtr(0)}, now)
	if perr != nil {
		t.Fatalf("变回自己失败：%v", perr)
	}
	view = metamoOf(out)
	if view != nil || r.metamo != nil {
		t.Fatalf("变回自己后应没有变身，得到 %+v", view)
	}
	if r.MP != 34 {
		t.Errorf("变回自己也要扣 MP：剩 %d，应为 34", r.MP)
	}
}

// metamoOf 取出 useMagic 响应里的变身状态（变回自己或不是变身时为 nil）。
func metamoOf(out map[string]any) *metamoView {
	v, _ := out["metamo"].(*metamoView)
	return v
}

// 变身的效果只改外观，属性不变：变身前后的衍生数值、等级、HP 都一样。
func TestMetamoKeepsStats(t *testing.T) {
	s := &Service{}
	r := metamoRecord()
	before := r.status()
	if _, perr := s.useMagic(newMetamoPresence(), r, magicUseRequest{Slot: slotPtr(0), Target: slotPtr(0)}, time.Now()); perr != nil {
		t.Fatal(perr)
	}
	defer stopMetamo(r)
	after := r.status()
	if after.Derived != before.Derived || after.Level != before.Level || after.HP != before.HP || after.MaxHP != before.MaxHP {
		t.Fatalf("变身不改属性：before=%+v after=%+v", before, after)
	}
}
