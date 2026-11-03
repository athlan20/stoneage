// 变幻的精灵（STAGE-203，原版 gmsv/magic/magic.c 的 MAGIC_Metamo）：战斗外变身成自己的一只宠物，只改外观。
// 变身状态只在内存里（原版是 work 变量，不存档）：登出、服务端重启、重新进游戏都回到自己的外观。
// 到期由定时器清除（原版在 CHAR_WORKITEMMETAMO 到期时变回）；卸下身体装备（装备格 1）时也清除（原版 CHAR_sendItemDetachEvent）。
// 变身的施放检查在 magic_use.go，装备联动在 equip.go 调用这里的函数。

package character

import (
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// metamoState 是一次变身。Until 是到期时间；timer 到期时清除它。清除时比较指针，旧的定时器清不掉新的变身。
type metamoState struct {
	Pet   string // 宠物的名字（改过名的用改后的名字，原版用 CHAR_NAME）
	Image int    // 宠物模板的图像编号（petbase 第 3 列）
	Until time.Time
	timer *time.Timer
}

// metamoView 是 status.metamo（协议 5.8）和 magic_use 响应里的变身状态；Until 是到期的毫秒时间戳。
type metamoView struct {
	Pet   string `json:"pet"`
	Image int    `json:"image"`
	Until int64  `json:"until"`
}

// metamoView 返回变身状态；没有变身时返回 nil（status 里省略 metamo）。
func (r *record) metamoView() *metamoView {
	if r.metamo == nil {
		return nil
	}
	return &metamoView{Pet: r.metamo.Pet, Image: r.metamo.Image, Until: r.metamo.Until.UnixMilli()}
}

// startMetamo 让角色变成 pet（图像编号 image），持续 d；先停掉原来的变身和它的定时器。调用时要持有 p.mu。
func (p *Presence) startMetamo(r *record, pet string, image int, now time.Time, d time.Duration) {
	stopMetamo(r)
	st := &metamoState{Pet: pet, Image: image, Until: now.Add(d)}
	st.timer = time.AfterFunc(d, func() { p.expireMetamo(st) })
	r.metamo = st
}

// stopMetamo 清除变身（不推送），并停掉定时器。调用时要持有 p.mu（或者还没有其他协程能访问记录）。
func stopMetamo(r *record) {
	if r.metamo == nil {
		return
	}
	if r.metamo.timer != nil {
		r.metamo.timer.Stop()
	}
	r.metamo = nil
}

// expireMetamo 是到期定时器：只清除这一次的变身（同一个 st），然后推送变回来的状态和提示。
func (p *Presence) expireMetamo(st *metamoState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rec.metamo != st {
		return
	}
	p.endMetamoLocked(&p.rec, "变身失效了。")
}

// endMetamoLocked 清除变身，并推送变化和一条消息栏提示。调用时要持有 p.mu。
func (p *Presence) endMetamoLocked(r *record, text string) {
	stopMetamo(r)
	p.pushMetamoLocked(r, text)
}

// pushMetamoLocked 给世界中的每条连接推送 status_changed（metamo 随状态变）和一条消息栏提示。调用时要持有 p.mu。
func (p *Presence) pushMetamoLocked(r *record, text string) {
	p.pushAll("status_changed", map[string]any{"status": r.status()})
	p.pushAll("notice", map[string]string{"level": "info", "text": text})
}

// isRangedWeapon 报告物品是不是远距离武器：弓、投掷斧、投掷石、回旋标（原版 ITEM_BOW、ITEM_BOUNDTHROW、ITEM_BREAKTHROW、ITEM_BOOMERANG）。
func isRangedWeapon(itemID int) bool {
	def, ok := gamedata.ItemByID(itemID)
	if !ok {
		return false
	}
	switch def.Type {
	case gamedata.ItemTypeBow, gamedata.ItemTypeBoomerang, gamedata.ItemTypeBoundThrow, gamedata.ItemTypeBreakThrow:
		return true
	}
	return false
}

// bodyArmourChanged 报告身体装备（装备格 1）是不是被卸下或换掉了。原版只看 CHAR_BODY 这个部位。
func bodyArmourChanged(before, after []itemRec) bool {
	old := findItem(before, gamedata.SlotBody)
	if old == nil {
		return false
	}
	now := findItem(after, gamedata.SlotBody)
	return now == nil || now.ItemID != old.ItemID
}

// checkMetamoWear 变身中不能把远距离武器穿在手上（原版在战斗里出手时拦，这里在穿的时候拦，见 STAGE-203）。
// 调用时要持有角色的锁，before 和 after 是改动前后的物品列表。
func checkMetamoWear(r *record, before, after []itemRec) *protocol.Error {
	if r.metamo == nil {
		return nil
	}
	hand := findItem(after, gamedata.SlotHand)
	if hand == nil || !isRangedWeapon(hand.ItemID) {
		return nil
	}
	if old := findItem(before, gamedata.SlotHand); old != nil && old.ItemID == hand.ItemID {
		return nil // 手上本来就是这件，没有新穿
	}
	return protocol.Errorf(protocol.CodeRequirementNotMet, "变身中无法装备远距离武器")
}

// hasRidingPet 报告有没有正在骑的宠物（原版 CHAR_RIDEPET ≠ -1）。
func hasRidingPet(pets []petRec) bool {
	for i := range pets {
		if pets[i].State == petStateRide {
			return true
		}
	}
	return false
}
