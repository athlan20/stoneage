package battle

// 队员交未知动作回 BAD_REQUEST（同单人路径），不属于该 actor 的已知动作同样回 BAD_REQUEST（与 docs/protocol.md 10.5、单人路径一致）（STAGE-275 第三轮）。

import (
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

func TestMateUnknownActionIsBadRequest(t *testing.T) {
	b := partyWith(t, testSide(), rollRnd(9999), PartyMate{No: 1, Side: testSide()})
	_, perr := b.SubmitFor(1, b.Turn(), "player", "fly", Cmd{})
	if perr == nil || perr.Code != protocol.CodeBadRequest {
		t.Fatalf("未知动作 fly 应回 BAD_REQUEST，实际 %v", perr)
	}
}

func TestMateActionNotOfActorIsBadRequest(t *testing.T) {
	b := partyWith(t, testSide(), rollRnd(9999), PartyMate{No: 1, Side: testSide()})
	// 人物动作里的 skill 只有宠物能用：已知，但队员的人物不允许。
	if _, perr := b.SubmitFor(1, b.Turn(), "player", "skill", Cmd{}); perr == nil || perr.Code != protocol.CodeBadRequest {
		t.Fatalf("人物的 skill 应回 BAD_REQUEST，实际 %v", perr)
	}
	// 宠物的 magic 同理。
	if _, perr := b.SubmitFor(1, b.Turn(), "pet", "magic", Cmd{}); perr == nil || perr.Code != protocol.CodeBadRequest {
		t.Fatalf("宠物的 magic 应回 BAD_REQUEST，实际 %v", perr)
	}
}
