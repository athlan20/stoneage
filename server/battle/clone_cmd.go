// 玩家给分身指定攻击目标（STAGE-186，docs/protocol.md 10.4 / 10.5）：分身和分身的宠物仍由服务端自动出招，
// 但玩家可以在回合里给每个能行动的分身（actor 为 clone:N）和分身的宠物（clone_pet:N）各指定一个攻击目标。
// 这是可选的，不进 needs——队长交齐指令照样马上结算；没指定的、或来不及指定的，由 planAuto 按合击顺序选
// （STAGE-312，默认打顺序里的第一个）。
// 分身的动作只有普通攻击。

package battle

import (
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/protocol"
)

// cloneActor 解析 clone:N / clone_pet:N，返回对应的单位号（分身 N 占 N 号，它的宠物占 5+N 号）；
// 不是分身身份（或 N 不在 1–MaxClones）返回 false。
func cloneActor(actor string) (int, bool) {
	prefix, pet := "clone:", false
	if strings.HasPrefix(actor, "clone_pet:") {
		prefix, pet = "clone_pet:", true
	}
	if !strings.HasPrefix(actor, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(actor, prefix))
	if err != nil || n < 1 || n > MaxClones || strconv.Itoa(n) != strings.TrimPrefix(actor, prefix) {
		return 0, false
	}
	if pet {
		return firstPetNo + n, true
	}
	return n, true
}

// cloneActorName 是 cloneActor 的反向：分身单位的身份名；不是分身返回空串。
func cloneActorName(u *Unit) string {
	if u == nil || !u.Auto {
		return ""
	}
	if u.Kind == KindPet {
		return "clone_pet:" + strconv.Itoa(u.No-firstPetNo)
	}
	return "clone:" + strconv.Itoa(u.No)
}

// planOptional 在回合开始时列出这回合能被玩家指定目标的分身：活着、能行动（倒下、睡着、石化、麻痹的不列），
// 按单位号从小到大。
func (b *Battle) planOptional() {
	b.optional = nil
	for _, u := range b.units {
		if u == nil || !u.Auto || !u.Alive() || !u.Status.CanAct() {
			continue
		}
		b.optional = append(b.optional, cloneActorName(u))
	}
}

// Optional 返回这回合可以指定目标的分身身份（clone:N / clone_pet:N）；没有分身时为 nil。
func (b *Battle) Optional() []string {
	if len(b.optional) == 0 {
		return nil
	}
	return b.optional
}

// submitClone 记录玩家给分身指定的目标。只允许 attack；身份不在 optional 里（没有这个分身、倒下或不能行动）
// 回复 COMMAND_NOT_NEEDED；目标要是对方一个活着的单位。结算前可以改选，后来的覆盖先前的。
func (b *Battle) submitClone(actor string, no int, action string, cmd Cmd) *protocol.Error {
	if action != "attack" {
		return protocol.Errorf(protocol.CodeBadRequest, "%s 只能使用 attack", actor)
	}
	listed := false
	for _, a := range b.optional {
		if a == actor {
			listed = true
		}
	}
	if !listed {
		return protocol.Errorf(protocol.CodeCommandNotNeeded, "%s 本回合不需要指令", actor)
	}
	if cmd.Target < 0 || cmd.Target > 19 {
		return protocol.Errorf(protocol.CodeBadRequest, "target 要在 0–19 之间")
	}
	if t := b.Unit(cmd.Target); t == nil || !t.Alive() || t.Side() == b.Unit(no).Side() {
		return protocol.Errorf(protocol.CodeInvalidTarget, "要选择对方一个活着的单位")
	}
	b.cloneCmds[no] = &Cmd{Actor: actor, Action: "attack", Target: cmd.Target}
	return nil
}

// nextNeed 是队长这回合还没交的第一条指令（player / pet）；交齐了返回 wait。分身指令的成功响应用它做 next。
func (b *Battle) nextNeed() string {
	for _, a := range b.needs {
		if b.cmds[a] == nil {
			return a
		}
	}
	return "wait"
}
