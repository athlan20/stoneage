// 精工的规则（STAGE-298，docs/inlay.md 2.1、2.2、1.4，docs/protocol.md 5.73）：请求形状、校验顺序和逐个镶。
// 不碰数据库，只算出改动后的物品列表；写库和推送在 item_inlay.go。

package character

import (
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// maxInlayMaterials 是一次精工最多的材料件数：目标加材料共 4 件（原版“精工每次只能选择四个物品”）。
const maxInlayMaterials = 3

// inlayOutcome 是一次精工算出的结果。Full 为真表示中途镶满：前 Applied 件已经镶上、扣掉（Consumed），其余没有镶。
type inlayOutcome struct {
	Next     []itemRec // 改动后的完整背包（按格子从小到大）
	Applied  int
	Consumed []int // 被扣掉的材料格，按请求顺序，每格 1 个
	Full     bool
}

// checkInlayShape 检查请求的形状（5.73 第 3 步）：target 是 0–19；materials 至少 1 个，都是背包格 5–19，
// 不重复，也不等于 target。件数上限在 planInlay 里查（第 4 步）。
func checkInlayShape(target int, materials []int) error {
	if err := validItemSlot(target); err != nil {
		return err
	}
	if len(materials) == 0 {
		return protocol.Errorf(protocol.CodeBadRequest, "需要至少一个材料格（%d–%d）", firstBagSlot, MaxItemSlot)
	}
	seen := make(map[int]bool, len(materials))
	for _, slot := range materials {
		if slot < firstBagSlot || slot > MaxItemSlot {
			return protocol.Errorf(protocol.CodeBadRequest, "材料格要在 %d–%d 之间（只能用背包里的物品）", firstBagSlot, MaxItemSlot)
		}
		if slot == target || seen[slot] {
			return protocol.Errorf(protocol.CodeBadRequest, "材料格 %d 重复了，或和目标格相同", slot)
		}
		seen[slot] = true
	}
	return nil
}

// planInlay 按 5.73 的校验顺序（第 4–10 步）检查，通过后逐个镶。调用方已做控制权、战斗和形状检查。
// 中途镶满不是错误：返回 Full 为真的结果，由调用方决定怎么回复。第一件就镶满时 Applied 为 0，调用方不写库。
func planInlay(items []itemRec, target int, materials []int) (inlayOutcome, error) {
	if len(materials) > maxInlayMaterials { // 第 4 步
		return inlayOutcome{}, protocol.Errorf(protocol.CodeInlayTooMany, "精工每次只能选择四个物品。")
	}
	slots := append([]int{target}, materials...)
	defs := make([]gamedata.ItemTemplate, len(slots))
	for i, slot := range slots { // 第 5 步：格子是空的
		it := findItem(items, slot)
		if it == nil {
			return inlayOutcome{}, protocol.Errorf(protocol.CodeEmptySlot, "%d 号格子没有物品", slot)
		}
		defs[i], _ = gamedata.ItemByID(it.ItemID) // 模板里没有的编号类别码为空，下一步判不适合
	}
	// 第 6、7 步：目标在前、材料依次在后逐件检查；第二件武器或防具报 MULTI。第 8 步要全部读完才查。
	gears := 0
	for _, def := range defs {
		if !inlaySuitable(def) { // 第 6 步
			return inlayOutcome{}, protocol.Errorf(protocol.CodeInlayNotSuitable, "这样物品不适合精工。")
		}
		if isInlayGear(def) {
			gears++
			if gears == 2 { // 第 7 步
				return inlayOutcome{}, protocol.Errorf(protocol.CodeInlayMultiTarget, "精工每次只能选择一个武器或防具。")
			}
		}
	}
	if !isInlayGear(defs[0]) { // 第 8 步：目标必须是武器或防具
		return inlayOutcome{}, protocol.Errorf(protocol.CodeInlayNoTarget, "精工必须选择一个武器或防具。")
	}
	target0 := findItem(items, target)
	if target0.Count != 1 { // 第 9 步：目标不能叠放
		return inlayOutcome{}, protocol.Errorf(protocol.CodeBadRequest, "武器或防具要一件一件镶，这一格有 %d 个", target0.Count)
	}

	// 第 10 步：依次镶，遇到目标已满就停（前面的不回滚）。
	cur := stateOf(*target0, defs[0])
	out := inlayOutcome{}
	for i, slot := range materials {
		if freeInlaySlots(cur.Codes) == 0 {
			out.Full = true
			break
		}
		applyInlayMaterial(cur, defs[i+1])
		out.Applied++
		out.Consumed = append(out.Consumed, slot)
	}
	out.Next = items
	if out.Applied == 0 {
		return out, nil
	}
	tgt := *target0
	tgt.Inlay = cur
	tgt.Inlay.Effect = inlayEffectText(effectiveTemplate(defs[0], cur))
	next := replaceItem(items, tgt)
	for _, slot := range out.Consumed {
		next = consumeOne(next, slot)
	}
	out.Next = next
	return out, nil
}

// stateOf 复制物品现在的镶嵌状态（没镶过的从模板的初始镶嵌码起算），改它不影响原物品。
func stateOf(it itemRec, def gamedata.ItemTemplate) *inlayState {
	if it.Inlay != nil {
		cp := *it.Inlay
		return &cp
	}
	return &inlayState{Codes: parseInlayCodes(def.InlayInit)}
}

// applyInlayMaterial 把一个材料镶到 st 上（5.73“每个材料的改动”）：占一个空格，数值累加，
// 材料带魔法就覆盖魔法编号和 MP，使用函数与参数整个覆盖（材料没有的也清空，同原版）。调用前要确认还有空格。
// 效果文字在全部镶完后统一重写（planInlay）。
func applyInlayMaterial(st *inlayState, mat gamedata.ItemTemplate) {
	for i := range st.Codes {
		if st.Codes[i] == nil {
			code := mat.InlayCode
			st.Codes[i] = &code
			break
		}
	}
	b := &st.Bonus
	b.Attack += mat.Attack
	b.Defence += mat.Defence
	b.Agility += mat.Agility
	b.HP += mat.HP
	b.MP += mat.MP
	b.Luck += mat.Luck
	b.OtherDamage += mat.OtherDamage
	b.OtherDefc += mat.OtherDefc
	if mat.MagicID > 0 {
		st.Magic = &inlayMagic{ID: mat.MagicID, MP: mat.MagicMP}
	}
	useFunc, argument := mat.UseFunc, mat.Argument
	st.UseFunc, st.Argument = &useFunc, &argument
}

// replaceItem 返回把格子 it.Slot 换成 it 的新列表（不改原列表）。
func replaceItem(items []itemRec, it itemRec) []itemRec {
	out := make([]itemRec, len(items))
	copy(out, items)
	for i := range out {
		if out[i].Slot == it.Slot {
			out[i] = it
		}
	}
	return out
}

// consumeOne 扣掉格子上的 1 个物品：叠放的只扣 1 个，扣到 0 删掉这一格（同加工、料理，docs/inlay.md 2.1）。
func consumeOne(items []itemRec, slot int) []itemRec {
	out := make([]itemRec, 0, len(items))
	for _, it := range items {
		if it.Slot == slot {
			if it.Count > 1 {
				it.Count--
				out = append(out, it)
			}
			continue
		}
		out = append(out, it)
	}
	return out
}
