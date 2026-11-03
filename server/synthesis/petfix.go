package synthesis

import "github.com/athlan20/stoneage/server/gamedata"

// STAGE-206 第 6 节：宠物固定原子的整理（原版 ITEM_merge_getPetFix，item_gen.c:504，PET_ADD_INGRED 的 _FMVER21 分支；
// gmsv 构建定义了 _FMVER21，见 original-data/gmsv/include/version.h）。

// petFixedAtoms 取宠物模板的固定原子，整理成 fixedAtom（规则见 buildFixedAtoms）。
func petFixedAtoms(templateID int) []fixedAtom {
	return buildFixedAtoms(gamedata.PetAtoms(templateID))
}

// buildFixedAtoms 按序号顺序整理固定原子：
//   - 原子名不在原子表里（Known 为假，原版打印 "fucking atom"）：原版宏在 for(i<5) 循环体里用 continue，
//     会跳过同一轮后面的序号，所以从第一个缺口起，后面的固定原子全部丢失。
//     （docs/synthesis.md 写的是“跳过”，源码为准；现有数据的缺口只在序号 4、5，两种写法结果相同。）
//   - FIXMIN、FIXMAX 先按大小交换，再把负数换成 1000（非守护兽，ITEM_RANDRANGEDOM）。
func buildFixedAtoms(atoms []gamedata.PetAtom) []fixedAtom {
	var out []fixedAtom
	for _, a := range atoms {
		idx, ok := gamedata.AtomIndex(a.Atom)
		if !ok {
			break
		}
		mn, mx := a.FixMin, a.FixMax
		if mn > mx {
			mn, mx = mx, mn
		}
		if mn < 0 {
			mn = randRangeDomain
		}
		if mx < 0 {
			mx = randRangeDomain
		}
		out = append(out, fixedAtom{index: idx, baseAdd: a.BaseAdd, fixMin: mn, fixMax: mx})
	}
	return out
}

// findFixed 返回原子的固定原子（同一原子出现多次时取序号靠前的，与原版 j 循环的 break 一致）；没有返回 nil。
func findFixed(fixed []fixedAtom, index int) *fixedAtom {
	for i := range fixed {
		if fixed[i].index == index {
			return &fixed[i]
		}
	}
	return nil
}
