package character

// STAGE-135：窗口修订号由所有连接共享；不从 NPC 文本猜输入类型或交易数据。
import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

func (p *Presence) checkWindow(id string) *protocol.Error {
	w := p.npcWindow
	if w == nil || ((p.npcMenuV2 || id != "") && (id == "" || id != w.id)) {
		return protocol.Errorf(protocol.CodeWrongState, "对话窗口已更新，请重新选择")
	}
	return nil
}

func (p *Presence) decorateWindow(data any) {
	v, ok := data.(map[string]any)
	w := p.npcWindow
	if !ok || w == nil || !p.npcMenuV2 {
		return
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	w.id = hex.EncodeToString(b[:])
	v["window_id"] = w.id
	v["stage"] = "options"
	if accepts, _ := v["accepts_text"].(bool); accepts {
		v["input"] = map[string]any{"kind": "text", "max_chars": 220}
	}
	if w.shop == nil {
		return
	}
	v["stage"] = w.shop.stage
	shop := gamedata.NpcShopOf(w.npc)
	if shop == nil {
		return
	}
	options, _ := v["options"].([]npcOption)
	for i := range options {
		o := &options[i]
		if w.shop.stage == shopStageBuyList {
			id, price, ok := shop.BuyPrice(o.No - 1)
			if ok {
				o.Product = &shopProduct{ItemID: id, Slot: -1, UnitPrice: price, MaxCount: 15}
			}
		}
		if w.shop.stage == shopStageSellList {
			if it := findItem(p.rec.Items, o.No); it != nil {
				price, _ := shop.SellPrice(it.ItemID)
				o.Product = &shopProduct{ItemID: it.ItemID, Slot: it.Slot, UnitPrice: price, Owned: it.Count, MaxCount: it.Count}
			}
		}
	}
	v["options"] = options
	if w.shop.stage == shopStageBuyQty || w.shop.stage == shopStageSellQty {
		max := 15
		if w.shop.stage == shopStageSellQty {
			max = w.shop.selected.Count
		}
		v["input"] = map[string]any{"kind": "integer", "min": 1, "max": max, "max_chars": 10}
		v["accepts_text"] = true
	}
	if w.shop.quote != nil {
		q := w.shop.quote
		v["trade"] = q.view()
		v["text"] = fmt.Sprintf("%s · 单价 %d · 数量 %d · 总价 %d 金币", itemName(q.itemID), q.unit, q.count, q.total)
		if q.note != "" {
			v["text"] = q.note + "，请核对交易\n" + v["text"].(string)
		}
	}
}

type shopProduct struct {
	ItemID    int `json:"item_id"`
	Slot      int `json:"slot"`
	UnitPrice int `json:"unit_price"`
	Owned     int `json:"owned"`
	MaxCount  int `json:"max_count"`
}
