// 攻击精灵的命中范围（STAGE-218，原版 battle/battle_magic.c 的 BATTLE_MultiAttMagic）。
//
// magic.txt 的目标码只决定“选什么”：1/9 选敌方一个人、10 选敌方一排、11 不用选（敌方两排）。
// 实际打到哪些格由 data/attmagic.bin 的 siField（3×5 掩码）决定：
//   - 目标码 1/9：选中的人是掩码第 1 行的第 2 列，向上下各取一行、左右各取两列，出界就裁掉；
//   - 目标码 10：选中的一排是掩码第 1 行，上一排用第 0 行、下一排用第 2 行，列从 0 到 4；
//   - 目标码 11：遍历敌方两排，第 0 排用掩码第 0 行、第 1 排用第 1 行。
// 原版 uiAttackType 只影响回包和表现，不参与取格。

package battle

import "github.com/athlan20/stoneage/server/gamedata"

// magicAttackTargets 是攻击精灵的命中集合，按单位号从小到大返回；掩码里只有空格子时返回空。
func (b *Battle) magicAttackTargets(u *Unit, cmd Cmd, t gamedata.MagicTemplate) []*Unit {
	if t.All { // 目标码 11（或 8）：不用选，敌方两排按掩码第 0、1 行取格
		return b.magicMaskTargets(u, func(row, col int) bool {
			return row < 2 && t.Mask[row*5+col]
		})
	}
	target := b.Unit(cmd.Target)
	if target == nil || !target.targetable() || target.Side() == u.Side() {
		return nil
	}
	tr, tc := unitLocalRow(target.No), unitLocalCol(target.No)
	if t.Target == 10 { // 一排：目标所在排是掩码第 1 行，列不偏移
		return b.magicMaskTargets(u, func(row, col int) bool {
			mr := row - tr + 1
			return 0 <= mr && mr < 3 && t.Mask[mr*5+col]
		})
	}
	// 目标码 1/9（其他没见过的按单体）：目标格是掩码第 1 行第 2 列，四周各扩一格/两列
	return b.magicMaskTargets(u, func(row, col int) bool {
		mr, mc := row-tr+1, col-tc+2
		return 0 <= mr && mr < 3 && 0 <= mc && mc < 5 && t.Mask[mr*5+mc]
	})
}

// magicMaskTargets 按 hit 判断敌方每个格子是否命中，跳过空格子、倒下的和同方的单位。
func (b *Battle) magicMaskTargets(u *Unit, hit func(row, col int) bool) []*Unit {
	var out []*Unit
	for no := 0; no < 2*unitsPerSide; no++ {
		x := b.Unit(no)
		if x == nil || !x.targetable() || x.Side() == u.Side() {
			continue
		}
		if hit(unitLocalRow(no), unitLocalCol(no)) {
			out = append(out, x)
		}
	}
	return out
}

// unitLocalRow 是单位在自己一方里的排（0 前排、1 后排），unitLocalCol 是列（0–4）。
func unitLocalRow(no int) int { return (no % unitsPerSide) / 5 }
func unitLocalCol(no int) int { return no % 5 }
