// battle_command 的校验与提交（docs/protocol.md 10.5）。校验按“回合 → 身份与顺序 → 参数 →
// 合法性”的顺序；任何错误都不消耗指令，客户端修正后可以继续提交。

package battle

import (
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// playerActions / petActions 是两个身份各自允许的动作（docs/protocol.md 10.5 的表）。
var playerActions = map[string]bool{
	"attack": true, "magic": true, "item": true, "guard": true,
	"capture": true, "escape": true, "swap_pet": true,
}
var petActions = map[string]bool{"attack": true, "skill": true, "guard": true}

// Submit 校验并记录一条出招指令，成功时返回下一步（pet / wait）。
// cmd 里是解析好的参数；turn 是请求里的回合号。
func (b *Battle) Submit(turn int, actor, action string, cmd Cmd) (string, *protocol.Error) {
	if b.phase != PhaseCommand || turn != b.turn {
		return "", protocol.Errorf(protocol.CodeWrongTurn, "不是当前回合").WithData(map[string]any{"turn": b.turn})
	}
	if no, ok := cloneActor(actor); ok { // 分身（STAGE-186）：可选指令，不进 needs
		if perr := b.submitClone(actor, no, action, cmd); perr != nil {
			return "", perr
		}
		return b.nextNeed(), nil
	}
	allowed := playerActions
	if actor == "pet" {
		allowed = petActions
	} else if actor != "player" {
		return "", protocol.Errorf(protocol.CodeBadRequest, "actor 只能是 player、pet 或 clone:N、clone_pet:N")
	}
	if !allowed[action] {
		return "", protocol.Errorf(protocol.CodeBadRequest, "%s 不能使用 %s", actor, action)
	}
	inNeeds := false
	for _, a := range b.needs {
		if a == actor {
			inNeeds = true
		}
	}
	if !inNeeds {
		return "", protocol.Errorf(protocol.CodeCommandNotNeeded, "%s 本回合不需要指令", actor)
	}
	if actor == "pet" && len(b.needs) == 2 && b.cmds["player"] == nil {
		return "", protocol.Errorf(protocol.CodeCommandOrder, "先交人物指令，再交宠物指令")
	}
	// 人物指令在宠物指令交上之前可以重交（10.5）：两个身份都还要交时覆盖先交的那条。
	redoPlayer := actor == "player" && b.cmds["pet"] == nil && len(b.needs) == 2
	if b.cmds[actor] != nil && !redoPlayer {
		return "", protocol.Errorf(protocol.CodeCommandSubmitted, "%s 本回合已经交过指令", actor)
	}
	if perr := b.checkParams(actor, action, &cmd); perr != nil {
		return "", perr
	}
	cmd.Actor, cmd.Action = actor, action
	b.cmds[actor] = &cmd
	if actor == "player" && action == "swap_pet" {
		b.dropNeed("pet") // 换上的宠物本回合不能行动
		return "wait", nil
	}
	if actor == "player" && len(b.needs) == 2 {
		return "pet", nil
	}
	return "wait", nil
}

func (b *Battle) dropNeed(actor string) {
	out := b.needs[:0]
	for _, a := range b.needs {
		if a != actor {
			out = append(out, a)
		}
	}
	b.needs = out
}

// checkParams 是队长的参数检查（no 为 0）。
func (b *Battle) checkParams(actor, action string, cmd *Cmd) *protocol.Error {
	return b.checkParamsFor(0, actor, action, cmd)
}

// checkParamsFor 按动作检查真人 no 的参数的类型、范围与合法性（docs/protocol.md 10.5 的表）。
// 物品、精灵、捕捉、换宠读的是这个真人自己的人物、背包、精灵和宠物（party_actor.go）。
// item 的 target 在这里规范化（“只能对自己”的物品把缺省目标落到使用者身上），写回 *cmd。
func (b *Battle) checkParamsFor(no int, actor, action string, cmd *Cmd) *protocol.Error {
	needTarget := func() *protocol.Error {
		if cmd.Target < 0 || cmd.Target > 19 {
			return protocol.Errorf(protocol.CodeBadRequest, "target 要在 0–19 之间")
		}
		return nil
	}
	targetUnit := func() *Unit {
		u := b.Unit(cmd.Target)
		if u == nil || !u.Alive() {
			return nil
		}
		return u
	}
	mine := no / unitsPerSide // 提交指令的真人所在的方（决斗里对方的真人是 1）
	b.chkSide = mine
	switch action {
	case "attack":
		if perr := needTarget(); perr != nil {
			return perr
		}
		u := targetUnit()
		if u == nil || u.Side() == mine {
			return protocol.Errorf(protocol.CodeInvalidTarget, "要选择对方一个活着的单位")
		}
	case "magic":
		if cmd.Magic < 0 || cmd.Magic > 4 {
			return protocol.Errorf(protocol.CodeBadRequest, "magic 要在 0–4 之间")
		}
		if b.humanOf(no).Status.Nocast > 0 { // STAGE-201：沉默中的人物不能用精灵（原版是把菜单灰掉，协议里没有这个包，所以在这里拦）
			return protocol.Errorf(protocol.CodeMagicUnusable, "沉默中，不能使用精灵")
		}
		id := b.sideOfNo(no).Magics[cmd.Magic]
		if id < 0 {
			return protocol.Errorf(protocol.CodeMagicUnusable, "这个精灵栏位是空的")
		}
		t, ok := b.slotMagicFor(no, cmd.Magic)
		if !ok {
			return protocol.Errorf(protocol.CodeMagicUnusable, "不认识的精灵编号 %d", id)
		}
		if t.Func == gamedata.MagicFuncMetamo { // 变幻的精灵（STAGE-203）只在战斗外用，原版战斗里也不能变身（本期不做）
			return protocol.Errorf(protocol.CodeMagicUnusable, "%s 变身只能在战斗外使用", t.Name)
		}
		if b.humanOf(no).MP < t.MP {
			return protocol.Errorf(protocol.CodeNotEnoughMP, "气力不够，需要 %d", t.MP)
		}
		if perr := b.checkTarget(t.BattleTarget(), cmd.Target); perr != nil {
			return perr
		}
	case "item":
		if cmd.Slot < 5 || cmd.Slot > 19 {
			return protocol.Errorf(protocol.CodeBadRequest, "slot 要在 5–19 之间")
		}
		it := b.sideOfNo(no).Items[cmd.Slot]
		if it.Name == "" {
			return protocol.Errorf(protocol.CodeItemUnusable, "这个槽位没有能用的物品")
		}
		def, ok := gamedata.ItemByID(it.ID)
		if !ok {
			return protocol.Errorf(protocol.CodeItemUnusable, "不认识的物品编号 %d", it.ID)
		}
		if def.AbleUse == gamedata.AbleUseField {
			return protocol.Errorf(protocol.CodeItemUnusable, "%s 只能在战斗外使用", def.Name)
		}
		eff, usable := def.UseEffect()
		if !usable {
			return protocol.Errorf(protocol.CodeItemUnusable, "%s 现在还不能使用", def.Name)
		}
		if eff.Revive { // 复活类（STAGE-262）：目标是己方倒下的单位，不看 def.Target（原版 101）
			return b.reviveItemTarget(cmd.Target)
		}
		you := b.humanOf(no)
		switch def.Target {
		case gamedata.TargetMyself: // 只能对自己
			if cmd.Target != -1 && cmd.Target != you.No {
				return protocol.Errorf(protocol.CodeInvalidTarget, "%s 只能对自己使用", def.Name)
			}
			cmd.Target = you.No
		case gamedata.TargetOther: // 己方任意活着的单位（含自己），一期没有队友；缺省目标是自己
			if cmd.Target == -1 {
				cmd.Target = you.No
			}
			u := b.Unit(cmd.Target)
			if u == nil || !u.Alive() || u.Side() != you.Side() {
				return protocol.Errorf(protocol.CodeInvalidTarget, "要选己方一个活着的单位（自己或宠物）")
			}
			if eff.HP == 0 && !eff.Full && u.Kind == KindPet {
				return protocol.Errorf(protocol.CodeInvalidTarget, "%s 不能给宠物使用", def.Name)
			}
		case gamedata.TargetMySide: // 全己方：不带 target 就是对己方全体生效
			if cmd.Target != -1 {
				u := b.Unit(cmd.Target)
				if u == nil || !u.Alive() || u.Side() != you.Side() {
					return protocol.Errorf(protocol.CodeInvalidTarget, "要选己方一个活着的单位（自己或宠物）")
				}
				if eff.HP == 0 && !eff.Full && u.Kind == KindPet {
					return protocol.Errorf(protocol.CodeInvalidTarget, "%s 不能给宠物使用", def.Name)
				}
			}
		default:
			return protocol.Errorf(protocol.CodeItemUnusable, "%s 不能用在对方身上", def.Name)
		}
	case "guard":
	case "capture":
		if !b.AllowCapture {
			return protocol.Errorf(protocol.CodeActionNotAllowed, "这场战斗不能捕捉")
		}
		if perr := needTarget(); perr != nil {
			return perr
		}
		u := targetUnit()
		if u == nil || u.Kind != KindMonster {
			return protocol.Errorf(protocol.CodeInvalidTarget, "只能捕捉野外的怪物")
		}
		if u.Level > b.humanOf(no).Level+5 {
			return protocol.Errorf(protocol.CodeActionNotAllowed, "怪物的等级太高，抓不了")
		}
	case "escape":
		if !b.AllowEscape {
			return protocol.Errorf(protocol.CodeActionNotAllowed, "这场战斗不能逃跑")
		}
	case "swap_pet":
		if b.humanOf(no).ridden() { // 骑乘中不能换宠、收回（docs/ride.md §5）
			return protocol.Errorf(protocol.CodeActionNotAllowed, "骑乘中不能换宠")
		}
		if cmd.PetSlot < -1 || cmd.PetSlot >= PetSlots {
			return protocol.Errorf(protocol.CodeBadRequest, "pet_slot 要在 0–%d 之间，-1 表示收回", PetSlots-1)
		}
		if cmd.PetSlot < 0 {
			if b.petOf(no) == nil {
				return protocol.Errorf(protocol.CodePetUnavailable, "没有出战的宠物")
			}
			return nil
		}
		p := b.sideOfNo(no).Bench[cmd.PetSlot]
		if p == nil || p.HP <= 0 {
			return protocol.Errorf(protocol.CodePetUnavailable, "这个栏位没有能出战的宠物")
		}
		if out := b.petOf(no); out != nil && out.PetSlot == cmd.PetSlot {
			return protocol.Errorf(protocol.CodePetUnavailable, "这只宠物已经在出战")
		}
	case "skill": // 宠物技能
		if cmd.Skill < 1 || cmd.Skill > 7 {
			return protocol.Errorf(protocol.CodeBadRequest, "skill 要在 1–7 之间")
		}
		pet := b.petOf(no)
		if pet == nil || pet.Skills[cmd.Skill-1] < 0 {
			return protocol.Errorf(protocol.CodeSkillUnusable, "这个技能栏位是空的")
		}
		sk, ok := gamedata.PetSkillByID(pet.Skills[cmd.Skill-1])
		if !ok {
			return protocol.Errorf(protocol.CodeSkillUnusable, "不认识的技能编号 %d", pet.Skills[cmd.Skill-1])
		}
		if sk.Field != gamedata.PetSkillFieldBattle {
			return protocol.Errorf(protocol.CodeSkillUnusable, "%s 只能在战斗外使用", sk.Name)
		}
		if perr := b.checkTarget(sk.BattleTarget(), cmd.Target); perr != nil {
			return perr
		}
	}
	return nil
}
