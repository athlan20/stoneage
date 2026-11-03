package character

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// inventoryViewSHA 是改动前 inventory 响应（record.inventoryView 的 JSON）的 sha256，覆盖 0–14000 号物品、
// 一个不存在的编号、耐久各种组合。STAGE-297 要求现有数据下输出与改动前逐字节一致。
// STAGE-326 起，只能在战斗外用、目标是他人或自己的物品多带 field_target_type（世界中的目标类型与战斗的不同），
// 这是有意的输出变化，登记值随之更新（改动前的值是 d25b6c25…）。
// STAGE-334 起，ITEM_useSkup（英雄的祝福 18543、泪之石 1680，只能在战斗外对自己用）也有了使用效果，
// 世界中的目标类型由 none 变为 self，同样是有意的输出变化，登记值随之更新。
// STAGE-340 起，ITEM_useNoenemy（龙王的守护 18544，还有除剂 1844、神签 2615）同样只能在战斗外对自己用，
// 世界中的目标类型由 none 变为 self，有意的输出变化，登记值随之更新。
const inventoryViewSHA = "7a08857ae641061a54eeaf8205a0606fae525feacf2ae7c035470f6819a2676f"

// TestInventoryViewUnchanged 是 STAGE-297 的回归：精工数据通道加进来以后，背包输出不变。
func TestInventoryViewUnchanged(t *testing.T) {
	var items []itemRec
	for id := 0; id <= 14000; id++ {
		items = append(items, itemRec{Slot: id, ItemID: id, Count: 1 + id%5, Durability: id, MaxDurability: id % 3 * 100, CrushLevel: id % 4})
	}
	items = append(items, itemRec{Slot: 99999, ItemID: 1 << 30, Count: 1})
	r := &record{Items: items, Gold: 12345}
	b, err := json.Marshal(r.inventoryView())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != inventoryViewSHA {
		t.Errorf("inventory 输出变了：sha256 = %s，登记的是 %s", got, inventoryViewSHA)
	}
}
