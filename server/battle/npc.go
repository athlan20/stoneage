package battle

// NPC 发起的战斗（STAGE-96，docs/protocol.md 10.3、10.9、10.11）：战斗的 kind 为 npc，带上 NPC 的身份，
// 开战和赢了时 NPC 说的话。战斗引擎不变，这里只管阵容和协议里的 npc 对象；开战、占用和胜负后的处理在 character。

import "github.com/athlan20/stoneage/server/gamedata"

// NPCRef 是战斗里 NPC 的身份（协议 10.11 的 npc 对象，不含台词）。
type NPCRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// NPCFight 是一场 NPC 发起的战斗：NPC 是谁，开战时（Start）和赢了时（End）它说的话。
type NPCFight struct {
	NPCRef
	Start string // startmsg
	End   string // endmsg
}

// NPCStart 是 battle_start 的 npc 对象（协议 10.3）：开战时 NPC 说的话。
type NPCStart struct {
	NPCRef
	Say string `json:"say"`
}

// NPCEnd 是 battle_end 的 npc 对象（协议 10.9）：赢了且 NPC 有结束台词时才有。
type NPCEnd struct {
	NPCRef
	Say string `json:"say"`
}

// SetNPC 把这场战斗标成 NPC 发起的：kind 为 npc，之后的 battle_start、battle、battle_end 带上 npc。
func (b *Battle) SetNPC(f NPCFight) {
	b.Kind = "npc"
	b.NPC = &f
}

// BuildNPC 按 NPC 的敌人列表开一场战斗：敌人按 enemies 的顺序入场，大型敌人的规则同野外，等级取敌人表
// （LVMin 为定值）。编号查不到的敌人跳过；一个都摆不出来时返回 false。调用方接着用 SetNPC 标上是哪个 NPC。
func BuildNPC(id, scene string, side *SideInput, enemies []int, rnd func(n int) int) (*Battle, bool) {
	var seats []*gamedata.EnemyVariant
	big := 0
	for _, eid := range enemies {
		e, ok := gamedata.EnemyByID(eid)
		if !ok {
			continue
		}
		seats, _ = placeEnemy(seats, e, &big) // 摆不下的（大型已满、找不到换位）不出，同野外
	}
	monsters := make([]*Unit, 0, len(seats))
	for _, e := range seats {
		t, ok := gamedata.PetByID(e.Template)
		if !ok {
			continue
		}
		monsters = append(monsters, NewMonster(t, enemyLevel(e, rnd), -1, rnd, e))
	}
	if len(monsters) == 0 {
		return nil, false
	}
	b := New(id, scene, side, monsters, rnd)
	b.Combo = true // STAGE-126：NPC 战斗有合击
	return b, true
}
