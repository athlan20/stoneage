package character

// STAGE-341：ACCEPT 的事件号不在 EndSetFlg 里时，交付后事件号进入进行中（keepDoing）。
// 全库只有 poru/mary04、mary05、mary06 三块是这个形状，正好是任务 43、45/46、47/48 的入口。

import (
	"slices"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 小队长给鲁乌（ACCEPT 55，EndSetFlg:66）：任务 44（事件 66）完成，同时事件 55 进入进行中，
// 后面泰坦队长的交鲁乌块（要求 NOWEV=55）才有机会命中。
func TestKeepDoingMary04GivesEvent55(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, []int{poruQ40, poruQ42}, []int{poruQ41, poruQ44})
	kingRun(t, r, poruPick(t, s, r, poruSquad, 4))
	if !r.eventDone(poruQ44) {
		t.Fatalf("事件 66 应完成，实际 %v", r.EventFlags)
	}
	if !r.eventDoing(poruQ43) {
		t.Fatalf("小队长交付后事件 55 应进行中，实际 %v", r.EventFlags)
	}
	// 已进行中：小队长再说话不会重复给鲁乌，而是命中事件 55 的提示块（“练到 30 级才回来见我们”）。
	if blk := poruPick(t, s, r, poruSquad, 3); blk.Type != "MESSAGE" || blk.Event != poruQ43 {
		t.Fatalf("交付后应命中事件 55 的 MESSAGE 提示块，实际 %s 事件 %d", blk.Type, blk.Event)
	}
}

// 事件 55 已完成的玩家不会被改回进行中（editSetDoing 的约定）。
func TestKeepDoingDoesNotReopenDone(t *testing.T) {
	r := kingRecord(nil, []int{poruQ43}, nil)
	blk := &gamedata.EventBlock{Type: "ACCEPT", Event: poruQ43, EndSet: []int{poruQ44}}
	p := progressOf(r)
	keepDoing(p, blk)
	if r.eventDoing(poruQ43) {
		t.Fatalf("已完成的事件不应被改回进行中")
	}
}

// 事件号在 EndSetFlg 里（普通交付）或没有 EndSetFlg 时不动，保证只影响三块。
func TestKeepDoingIgnoresOrdinaryBlocks(t *testing.T) {
	for _, blk := range []*gamedata.EventBlock{
		{Type: "ACCEPT", Event: 7, EndSet: []int{7}},
		{Type: "ACCEPT", Event: 7, EndSet: []int{7, 8}},
		{Type: "ACCEPT", Event: 7},
		{Type: "ACCEPT", Event: -1, EndSet: []int{8}},
	} {
		r := kingRecord(nil, nil, nil)
		keepDoing(progressOf(r), blk)
		if r.eventDoing(7) || len(r.EventFlags) != 0 {
			t.Fatalf("普通块不应改进行中: %+v → %v", blk, r.EventFlags)
		}
	}
}

// 数据检查：全库 ACCEPT/MESSAGE 里“事件号不在 EndSetFlg 里”的块，恰好是 mary04/05/06 三块；
// 以后转换结果出现第四块，这个测试会提醒核对 keepDoing 的影响面。
func TestKeepDoingScope(t *testing.T) {
	var got []string
	for _, sc := range gamedata.NpcEventScripts() {
		for i := range sc.Blocks {
			b := &sc.Blocks[i]
			if b.Type == "ACCEPT" && b.Event >= 0 && len(b.EndSet) > 0 && !slices.Contains(b.EndSet, b.Event) {
				got = append(got, sc.File)
			}
		}
	}
	slices.Sort(got)
	want := []string{"poru/mary04", "poru/mary05", "poru/mary06"}
	if !slices.Equal(got, want) {
		t.Fatalf("keepDoing 的影响面变了：%v，期望 %v", got, want)
	}
}
