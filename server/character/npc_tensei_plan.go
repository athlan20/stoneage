package character

// STAGE-266：人物转生的事件清空与赠宠规则（纯函数，不碰数据库；写库在 npc_tensei_store.go）。
// 原版 NPC_TransmigrationFlg_CLS（事件清空与补位）与 ten.arg 的 Present（按主城 × 转生次数）。

// 事件清空后保留的白名单（原版 NPC_TransmigrationFlg_CLS 的 endevflg / nowevflg）：
// 原本已完成的、原本进行中的各留一份，其余事件进度全部清掉。
var (
	tenseiKeepDone  = []int{69, 70, 71, 72, 109, 118, 122, 127, 131, 158, 159}
	tenseiKeepDoing = []int{69, 70, 71, 72}
	// tenseiRebirthEvents 是转生次数对应的完成标记：转 n 次设前 n 个（flg[] 数组，5 转封顶）。
	tenseiRebirthEvents = []int{48, 49, 50, 51, 52}
)

// planTenseiEvents 算转生后的任务与事件进度：全清、恢复白名单里原本完成和进行中的、再按新的转生次数 n 补 48–52。
// 顺序同原版：先设完成、再设进行中（已完成的事件不会再被改成进行中）。r 只读，返回的是副本。
func planTenseiEvents(r *record, n int) progress {
	after := progress{quests: map[int]int{}, events: map[int]int{}}
	for _, ev := range tenseiKeepDone {
		if r.eventDone(ev) {
			applyEventEdit(after, editSetDone, ev)
		}
	}
	for _, ev := range tenseiKeepDoing {
		if r.eventDoing(ev) {
			applyEventEdit(after, editSetDoing, ev)
		}
	}
	for i := 0; i < n && i < len(tenseiRebirthEvents); i++ {
		applyEventEdit(after, editSetDone, tenseiRebirthEvents[i])
	}
	return after
}

// 赠宠的宠物模板编号（petbase.tsv，原版 Present 写的是敌群 1377–1381，一一对应）。
const (
	tenseiPetUlisten = 6   // 乌力斯坦
	tenseiPetQina    = 232 // 奇娜
	tenseiPetDuobao  = 244 // 多鲁宝
	tenseiPetLokaren = 272 // 洛卡伦恩
	tenseiPetTilaso  = 304 // 帖拉所伊朵
)

// tenseiPresents 是主城（NPC 所在地图）× 转生次数（下标 n-1）的赠宠表，照 ten.arg 的 Present。
var tenseiPresents = map[int][5]int{
	1000: {tenseiPetUlisten, tenseiPetLokaren, tenseiPetDuobao, tenseiPetQina, tenseiPetTilaso}, // 萨姆吉尔
	2000: {tenseiPetLokaren, tenseiPetDuobao, tenseiPetQina, tenseiPetUlisten, tenseiPetTilaso}, // 玛丽娜丝
	3000: {tenseiPetDuobao, tenseiPetQina, tenseiPetUlisten, tenseiPetLokaren, tenseiPetTilaso}, // 加加
	4000: {tenseiPetQina, tenseiPetUlisten, tenseiPetLokaren, tenseiPetDuobao, tenseiPetTilaso}, // 卡鲁它那
}

// tenseiPresentLevel 是赠宠的等级（原版敌群 1377–1381 的等级都是 1）。
const tenseiPresentLevel = 1

// tenseiPresentOf 是在 floor 这个主城转到第 n 次时送的宠物模板编号；不是主城或 n 越界时 ok 为 false。
func tenseiPresentOf(floor, n int) (petID int, ok bool) {
	row, found := tenseiPresents[floor]
	if !found || n < 1 || n > len(row) {
		return 0, false
	}
	return row[n-1], true
}

// tenseiFreeSlot 是第一个空的宠物栏位；5 格全满返回 -1。
func tenseiFreeSlot(pets []petRec) int {
	var taken [MaxPetSlots]bool
	for _, p := range pets {
		if p.Slot >= 0 && p.Slot < MaxPetSlots {
			taken[p.Slot] = true
		}
	}
	for slot, used := range taken {
		if !used {
			return slot
		}
	}
	return -1
}
