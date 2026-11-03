package character

import "time"

// STAGE-207：加工、料理（合成）的冷却。对应原版 ITEM_mergeItem（item_gen.c:747）：
// 距上次合成的时间小于 5 + (材料数 − 2) 秒，就是“合成料理过於频繁”，扣料并还回一件材料。
// 冷却时间戳只放在角色的运行时数据里（Presence.lastMerge），不写库，重新进入世界清零。

// mergeCooldown 报告现在合成 n 件材料是否还在冷却中。last 是上次进入合成的时间（零值表示从来没有）。
// 原版用的是整数秒（time_t），这里也按整数秒比较，避免亚秒的抖动。
func mergeCooldown(last, now time.Time, n int) bool {
	return now.Unix()-last.Unix() < int64(5+(n-2))
}
