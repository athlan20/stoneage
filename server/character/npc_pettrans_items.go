package character

// STAGE-237：宠物转生成功后删掉的道具和设置的事件（原版 NPC_PetTransManStatus 的删除物品段与 NPC_NowEventSetFlg(71)，
// 出处 original-data/gmsv/npc/npc_transmigration.c:1410–1413、1565–1596）。
// 原版不查数量：身上有几件删几件，没有也能转生；删的是整格（CHAR_DelItem），和堆叠数量无关。

// petTransDelRange 是一段要删的道具编号（两端都包含）。
type petTransDelRange struct{ min, max int }

// petTransDelRanges 是原版 DelItem 表的三段：地灵石至风灵石（19625–19628）、地灵项链至风灵项链（19629–19632）、
// 19672–19687（地灵项链至风灵项链的另一组）。三段互不重叠，一件道具只会命中一段。
var petTransDelRanges = []petTransDelRange{
	{19625, 19628},
	{19629, 19632},
	{19672, 19687},
}

// petTransEventNo 是转生成功后设为进行中的事件号（NPC_NowEventSetFlg(toindex, 71)，npc_transmigration.c:1596）。
// 事件 71 对应任务 51“精灵王的传说 打封印精灵王”（server/gamedata/quests.tsv），是 king 任务链的入口。
const petTransEventNo = 71

// petTransItemDeleted 报告道具编号是否在转生要删的范围内。
func petTransItemDeleted(id int) bool {
	for _, rg := range petTransDelRanges {
		if id >= rg.min && id <= rg.max {
			return true
		}
	}
	return false
}

// planPetTransItems 返回转生之后的背包：范围内的道具整格删掉，其余不动。不修改传入的切片。
// 原版的删除循环遍历全部道具格（包括已装备的），这里同样不区分，所以装备中的项链也会被删。
func planPetTransItems(items []itemRec) []itemRec {
	out := make([]itemRec, 0, len(items))
	for _, it := range items {
		if !petTransItemDeleted(it.ItemID) {
			out = append(out, it)
		}
	}
	return out
}
