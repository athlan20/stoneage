// 装备自带的精灵（STAGE-190）：身上穿的装备带 ITEM_MAGICID 时，对应装备位（0–4）就是一个可施放的精灵栏位，
// 栏位号 = 装备位号，同原版战斗指令 J 的“装备位号即精灵号”（gmsv/battle/battle_command.c、
// gmsv/char/char.c 的 'j' 状态）。施放耗的 MP 取装备的 ITEM_MAGICUSEMP，不用魔法表里推算的值。

package character

import "github.com/athlan20/stoneage/server/gamedata"

// magicSlot 是一个精灵栏位里实际可用的精灵：ID 为 -1 表示空，MP 是这个栏位施放要耗的气力。
type magicSlot struct {
	ID, MP int
}

// magicSlots 返回 0–4 号精灵栏位实际可用的精灵（status.magic 与战斗共用这一份）。
// 栏位里存着管理命令给的精灵（characters.magics）时以它为准；否则看同号装备位上穿的装备带不带精灵。
// 两者同号时装备自带的精灵被占住不显示：目前人物自己的精灵只有管理命令能给，正常玩家的栏位都是空的。
func (r *record) magicSlots() [gamedata.MaxMagicSlots]magicSlot {
	var out [gamedata.MaxMagicSlots]magicSlot
	for i := range out {
		out[i] = magicSlot{ID: -1}
		if r.Magics[i] >= 0 {
			if t, ok := gamedata.MagicByID(r.Magics[i]); ok {
				out[i] = magicSlot{ID: t.ID, MP: t.MP}
			}
			continue
		}
		for _, it := range r.Items {
			if it.Slot != i {
				continue
			}
			// 魔法表里没有的编号（一些原版服务端的特殊精灵，tools/gen-magic.py 没收）当作没有。
			if def, ok := gamedata.ItemByID(it.ItemID); ok {
				def = effectiveTemplate(def, it.Inlay) // 精工覆盖的精灵（STAGE-298）
				if def.MagicID >= 0 {
					if _, known := gamedata.MagicByID(def.MagicID); known {
						out[i] = magicSlot{ID: def.MagicID, MP: def.MagicMP}
					}
				}
			}
			break
		}
	}
	return out
}
