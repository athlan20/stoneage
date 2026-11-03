// 真人同场战斗的指令提交（STAGE-275，docs/protocol.md 10.5 与 10.14）：每个真人各交自己的人物和宠物指令。
// 队长走 Submit 的原有校验；队员走这里：人物的动作与队长相同（攻击、精灵、道具、防御、捕捉、逃跑、换宠），
// 宠物的动作是攻击、技能、防御。参数检查与执行读的是这个队员自己的背包、精灵和宠物（party_actor.go）。

package battle

import "github.com/athlan20/stoneage/server/protocol"

// SubmitFor 是真人同场时某个真人交指令的入口：no 是人物单位号（0 为队长）。
// 队长交指令走 Submit 的原有校验，成功后同样启动截止计时（10.14）；单人战斗的 no 只能是 0，行为与 Submit 相同。
func (b *Battle) SubmitFor(no, turn int, actor, action string, cmd Cmd) (string, *protocol.Error) {
	if !b.party {
		return b.Submit(turn, actor, action, cmd)
	}
	if no == 0 {
		next, perr := b.Submit(turn, actor, action, cmd)
		if perr == nil {
			b.armDeadline()
		}
		return next, perr
	}
	return b.submitMate(no, turn, actor, action, cmd)
}

// armDeadline 在本回合第一条被接受的指令起算 120 秒的交指令截止时间（10.14）。
func (b *Battle) armDeadline() {
	if b.deadline.IsZero() {
		b.deadline = b.now().Add(PartyCommandWait)
	}
}

// submitMate 校验并记录队员的一条指令，返回下一步（pet / wait），与 Submit 的顺序一致：回合 → 身份 → 参数 → 合法性。
func (b *Battle) submitMate(no, turn int, actor, action string, cmd Cmd) (string, *protocol.Error) {
	if b.phase != PhaseCommand || turn != b.turn {
		return "", protocol.Errorf(protocol.CodeWrongTurn, "不是当前回合").WithData(map[string]any{"turn": b.turn})
	}
	m := b.mates[no]
	if m == nil || b.units[no] == nil {
		return "", protocol.Errorf(protocol.CodeCommandNotNeeded, "你已经不在这场战斗里")
	}
	allowed := playerActions
	switch actor {
	case "pet":
		allowed = petActions
	case "player":
	default:
		return "", protocol.Errorf(protocol.CodeBadRequest, "actor 只能是 player 或 pet")
	}
	if !allowed[action] { // 不属于该 actor 的动作（含不认识的）：同单人路径（submit.go）回 BAD_REQUEST，10.5
		return "", protocol.Errorf(protocol.CodeBadRequest, "%s 不能使用 %s", actor, action)
	}
	if !contains(m.needs, actor) {
		return "", protocol.Errorf(protocol.CodeCommandNotNeeded, "%s 本回合不需要指令", actor)
	}
	if actor == "pet" && len(m.needs) == 2 && b.mateCmd(no, "player") == nil {
		return "", protocol.Errorf(protocol.CodeCommandOrder, "先交人物指令，再交宠物指令")
	}
	// 人物指令在宠物指令交上之前可以重交（10.5）。
	redoPlayer := actor == "player" && b.mateCmd(no, "pet") == nil && len(m.needs) == 2
	if b.mateCmd(no, actor) != nil && !redoPlayer {
		return "", protocol.Errorf(protocol.CodeCommandSubmitted, "%s 本回合已经交过指令", actor)
	}
	if perr := b.checkParamsFor(no, actor, action, &cmd); perr != nil {
		return "", perr
	}
	cmd.Actor, cmd.Action = actor, action
	key := no
	if actor == "pet" {
		key = no + 5
	}
	b.mateCmds[key] = &cmd
	b.armDeadline()
	if actor == "player" && action == "swap_pet" {
		m.needs = withoutActor(m.needs, "pet") // 换上的宠物本回合不能行动（同队长，submit.go）
		return "wait", nil
	}
	if actor == "player" && len(m.needs) == 2 {
		return "pet", nil
	}
	return "wait", nil
}

func actorName(actor string) string {
	if actor == "pet" {
		return "宠物"
	}
	return "人物"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
