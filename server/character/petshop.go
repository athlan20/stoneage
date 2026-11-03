package character

// 宠物店（STAGE-73，docs/protocol.md 5.41）：talk 宠物店 NPC 弹菜单（寄放/领取/卖/买，
// 不支持寄放的店没有寄放和领取），列表页选宠物、确认页成交。卖宠价按原版
// NPC_GetCost（等级、捕获等级、稀有度、店的 nomal_rate/special_rate），寄放费
// 50+等级×4（NPC_GETPOOLCOST），领取免费；寄放池 15 格，state=3、slot=50+池位落库，
// 随身列表（petsView）不显示寄放的宠物。买是原版没有的功能（原版宠物店只有
// 卖/寄放/领取）：商品清单本期用出生宠物同款模板、价格按同一估价公式 ×10，
// 属于占位数据——见 STAGE-73 评论的决策点。

import (
	"context"
	"fmt"
	"github.com/athlan20/stoneage/server/battle"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// 宠物店窗口的阶段。
const (
	shopStagePetMenu   = "pet_menu"    // 菜单
	shopStagePetSell   = "pet_sell"    // 卖：随身宠物列表
	shopStagePetSellOK = "pet_sell_ok" // 卖：确认页
	shopStagePetPool   = "pet_pool"    // 寄放：随身宠物列表
	shopStagePetPoolOK = "pet_pool_ok" // 寄放：确认页
	shopStagePetGet    = "pet_get"     // 领取：寄放池列表
	shopStagePetBuy    = "pet_buy"     // 买：商品列表
	shopStagePetBuyOK  = "pet_buy_ok"  // 买：确认页
)

// petshopSession 记录宠物店对话的阶段和待定的宠物（卖/寄放是随身槽位，领取是池位，
// 买是模板编号）。
type petshopSession struct {
	stage  string
	slot   int // 卖/寄放：随身槽位；领取：池位
	tmplID int // 买：已选中的模板编号
}

// shopPetMenuOptions 是菜单选项：支持寄放的店多两项（原版 pool_flg）。
func petMenuOptions(pool bool) []npcOption {
	options := []npcOption{}
	if pool {
		options = append(options, npcOption{No: 1, Label: "寄放宠物"}, npcOption{No: 2, Label: "领取宠物"})
	}
	options = append(options, npcOption{No: 3, Label: "卖宠物"}, npcOption{No: 4, Label: "买宠物"})
	return options
}

// openPetshop 是 talk 成功后的菜单。调用时要持有 p.mu（在 talk 的 Operate 里）。
func (s *Service) openPetshop(p *Presence, npc gamedata.NPC, shop *gamedata.NpcPetshop) {
	p.npcWindow = &npcWindow{npc: npc.ID, petshop: &petshopSession{stage: shopStagePetMenu}}
	p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": shopText(shop.Main), "options": petMenuOptions(shop.Pool),
	})
}

// petMenuView 组一个宠物店窗口的推送。
func (s *Service) petMenuView(l *Live, w *npcWindow, npc gamedata.NPC, text string, options []npcOption) {
	if options == nil {
		options = []npcOption{}
	}
	l.p.pushAll("npc_window", map[string]any{
		"npc": npc.ID, "name": npc.Name, "type": npc.Type,
		"text": shopText(text), "options": options,
	})
}

// petListOptions 是随身宠物列表（槽位 0–19），选项编号就是槽位号。
func petListOptions(r *record) []npcOption {
	options := []npcOption{}
	for i := range r.Pets {
		p := &r.Pets[i]
		if p.Slot >= gamedata.PooledSlotBase || p.Slot >= MaxPetSlots {
			continue
		}
		options = append(options, npcOption{No: p.Slot, Label: fmt.Sprintf("%s Lv%d", p.Name, p.Level)})
	}
	return options
}

// poolListOptions 是寄放池列表，选项编号是池位（0 起）。
func poolListOptions(r *record) []npcOption {
	options := []npcOption{}
	for i := range r.Pets {
		p := &r.Pets[i]
		if p.Slot < gamedata.PooledSlotBase {
			continue
		}
		options = append(options, npcOption{No: p.Slot - gamedata.PooledSlotBase, Label: fmt.Sprintf("%s Lv%d", p.Name, p.Level)})
	}
	return options
}

// petshopChoose 推进宠物店窗口（npc_choose）。调用时要持有 p.mu。
func (s *Service) petshopChoose(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcPetshop, no int) *protocol.Error {
	switch w.petshop.stage {
	case shopStagePetMenu:
		switch {
		case no == 1 && shop.Pool: // 寄放
			w.petshop.stage = shopStagePetPool
			s.petMenuView(l, w, npc, "要寄放哪一只？", petListOptions(r))
			return nil
		case no == 2 && shop.Pool: // 领取
			w.petshop.stage = shopStagePetGet
			s.petMenuView(l, w, npc, "要领回哪一只？", poolListOptions(r))
			return nil
		case no == 3: // 卖
			w.petshop.stage = shopStagePetSell
			s.petMenuView(l, w, npc, "要卖哪一只？", petListOptions(r))
			return nil
		case no == 4: // 买（原版没有的功能，商品是占位数据，见协议 5.41）
			w.petshop.stage = shopStagePetBuy
			options := []npcOption{}
			for i, tmplID := range gamedata.ShopPetIDs {
				tmpl, ok := gamedata.PetByID(tmplID)
				if !ok {
					continue
				}
				price := petBuyCost(shop, tmplID)
				options = append(options, npcOption{No: i + 1, Label: fmt.Sprintf("%s（%d 金币）", tmpl.Name, price)})
			}
			s.petMenuView(l, w, npc, "要买哪一只？", options)
			return nil
		}
		return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)

	case shopStagePetSell:
		p := findPetAtSlot(r, no)
		if p == nil {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "身上没有 %d 号栏位的宠物", no)
		}
		w.petshop.stage = shopStagePetSellOK
		w.petshop.slot = no
		cost := sellCost(r, shop, p)
		s.petMenuView(l, w, npc, fmt.Sprintf("是[%s]没错吧？\n\n%s\n\n    买卖价格         %d STONE", p.Name, shop.Really, cost),
			[]npcOption{{No: 1, Label: "卖掉"}})
		return nil

	case shopStagePetSellOK:
		return s.petSell(l, r, w, npc, shop)

	case shopStagePetPool:
		p := findPetAtSlot(r, no)
		if p == nil {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "身上没有 %d 号栏位的宠物", no)
		}
		w.petshop.stage = shopStagePetPoolOK
		w.petshop.slot = no
		cost := gamedata.PoolCost(r.Level)
		s.petMenuView(l, w, npc, fmt.Sprintf("寄放[%s]\n\n    寄放价格            %d stone", p.Name, cost),
			[]npcOption{{No: 1, Label: "寄放"}})
		return nil

	case shopStagePetPoolOK:
		return s.petPool(ctx, l, r, w, npc, shop)

	case shopStagePetGet:
		poolSlot := no
		p := findPooledPet(r, poolSlot)
		if p == nil {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "寄放池 %d 号没有宠物", no)
		}
		free := freePetSlot(r)
		if free < 0 {
			return protocol.Errorf(protocol.CodeActionNotAllowed, "%s", shop.GetFull)
		}
		if _, err := s.db.Exec(ctx,
			`UPDATE character_pets SET slot = $3, state = $4 WHERE character_id = $1 AND slot = $2`,
			r.ID, p.Slot, free, petStateRest); err != nil {
			return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
		}
		p.Slot, p.State = free, petStateRest
		l.p.pushAll("pets_changed", r.petsView())
		l.p.pushWindowClosed(WindowReasonChosen)
		return nil

	case shopStagePetBuy:
		idx := no - 1
		if idx < 0 || idx >= len(gamedata.ShopPetIDs) {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这个窗口没有 %d 号选项", no)
		}
		tmplID := gamedata.ShopPetIDs[idx]
		tmpl, ok := gamedata.PetByID(tmplID)
		if !ok {
			return protocol.Errorf(protocol.CodeNPCBadChoice, "这只宠物没有货了，重新打开试试")
		}
		w.petshop.stage = shopStagePetBuyOK
		w.petshop.tmplID = tmplID
		s.petMenuView(l, w, npc, fmt.Sprintf("%s（%d 金币）\n要买下它吗？", tmpl.Name, petBuyCost(shop, tmplID)),
			[]npcOption{{No: 1, Label: "买下"}})
		return nil

	case shopStagePetBuyOK:
		return s.petBuy(ctx, l, r, w, npc, shop)
	}
	return protocol.Errorf(protocol.CodeWrongState, "窗口状态不对")
}

// petSell 卖掉待定的宠物（petshop.slot）：金币不能超上限（over_msg）。
func (s *Service) petSell(l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcPetshop) *protocol.Error {
	p := findPetAtSlot(r, w.petshop.slot)
	if p == nil {
		return protocol.Errorf(protocol.CodeWrongState, "这只宠物已经不在身上了")
	}
	if p.State == petStateRide {
		return protocol.Errorf(protocol.CodePetRiding, "骑乘中的宠物不能卖，先下骑")
	}
	cost := sellCost(r, shop, p)
	if r.Gold+int64(cost) > battle.MaxHaveGold {
		return protocol.Errorf(protocol.CodeGoldFull, "%s（金币上限 %d）", shop.Over, battle.MaxHaveGold)
	}
	if _, err := s.db.Exec(context.Background(),
		`DELETE FROM character_pets WHERE character_id = $1 AND slot = $2`, r.ID, p.Slot); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	removePetSlot(r, p.Slot)
	r.Gold += int64(cost)
	l.p.pushAll("pets_changed", r.petsView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushWindowClosed(WindowReasonChosen)
	return nil
}

// petPool 寄放待定的宠物（petshop.slot）：收寄放费（50+等级×4），寄放池满回 poolfull。
func (s *Service) petPool(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcPetshop) *protocol.Error {
	p := findPetAtSlot(r, w.petshop.slot)
	if p == nil {
		return protocol.Errorf(protocol.CodeWrongState, "这只宠物已经不在身上了")
	}
	if p.State == petStateRide {
		return protocol.Errorf(protocol.CodePetRiding, "骑乘中的宠物不能寄放，先下骑")
	}
	poolSlot := freePoolSlot(r)
	if poolSlot < 0 {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "%s", shop.PoolFull)
	}
	cost := gamedata.PoolCost(r.Level)
	if r.Gold < int64(cost) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "%s（寄放要 %d）", shop.Cost, cost)
	}
	if _, err := s.db.Exec(ctx,
		`UPDATE character_pets SET slot = $3, state = $4 WHERE character_id = $1 AND slot = $2`,
		r.ID, p.Slot, gamedata.PooledSlotBase+poolSlot, petStatePooled); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Gold -= int64(cost)
	p.Slot, p.State = gamedata.PooledSlotBase+poolSlot, petStatePooled
	l.p.pushAll("pets_changed", r.petsView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushWindowClosed(WindowReasonChosen)
	return nil
}

// petBuy 买入待定的宠物（petshop.tmplID）：随身栏位满回 getfull、钱不够回 cost_msg；
// 资质按出生宠同一套规则生成（NewPetGrowth），事务写库。
func (s *Service) petBuy(ctx context.Context, l *Live, r *record, w *npcWindow, npc gamedata.NPC, shop *gamedata.NpcPetshop) *protocol.Error {
	tmpl, ok := gamedata.PetByID(w.petshop.tmplID)
	if !ok {
		return protocol.Errorf(protocol.CodeWrongState, "这只宠物没有货了，重新打开试试")
	}
	free := freePetSlot(r)
	if free < 0 {
		return protocol.Errorf(protocol.CodeActionNotAllowed, "%s", shop.GetFull)
	}
	price := petBuyCost(shop, w.petshop.tmplID)
	if r.Gold < int64(price) {
		return protocol.Errorf(protocol.CodeNotEnoughGold, "%s（要 %d 金币）", shop.Cost, price)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	defer tx.Rollback(ctx)
	if err := insertPetRow(ctx, tx, r.ID, free, tmpl, gamedata.NewPetLevel, s.createRnd()); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	if err := tx.Commit(ctx); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	// 内存里补上同一只（资质重掷一次只是测试可见性差异，进世界重读后以库为准；
	// 为保持一致，按同一随机源重算一份和库相同的值是不必要的——这里直接重读这一行）。
	if err := s.reloadPetRow(ctx, r, free); err != nil {
		return protocol.Errorf(protocol.CodeInternal, protocol.MessageInternal)
	}
	r.Gold -= int64(price)
	l.p.pushAll("pets_changed", r.petsView())
	l.p.pushAll("status_changed", map[string]any{"status": r.status()})
	l.p.pushWindowClosed(WindowReasonChosen)
	return nil
}

// reloadPetRow 重读刚写库的一只宠物到内存（买宠后 petsView 立即可见）。
func (s *Service) reloadPetRow(ctx context.Context, r *record, slot int) error {
	rows, err := s.db.Query(ctx,
		`SELECT `+petColumns+` FROM character_pets WHERE character_id = $1 AND slot = $2`, r.ID, slot)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPet(rows)
		if err != nil {
			return err
		}
		replacePet(r, p)
	}
	return rows.Err()
}

// ---- 宠物列表的小工具 ----

func findPetAtSlot(r *record, slot int) *petRec {
	for i := range r.Pets {
		if r.Pets[i].Slot == slot && slot < gamedata.PooledSlotBase {
			return &r.Pets[i]
		}
	}
	return nil
}

func findPooledPet(r *record, poolSlot int) *petRec {
	for i := range r.Pets {
		if r.Pets[i].Slot == gamedata.PooledSlotBase+poolSlot {
			return &r.Pets[i]
		}
	}
	return nil
}

func freePetSlot(r *record) int {
	for slot := 0; slot < MaxPetSlots; slot++ {
		if findPetAtSlot(r, slot) == nil {
			return slot
		}
	}
	return -1
}

func freePoolSlot(r *record) int {
	for slot := 0; slot < gamedata.PoolPetLimit(r.Transmigration); slot++ {
		if findPooledPet(r, slot) == nil {
			return slot
		}
	}
	return -1
}

func removePetSlot(r *record, slot int) {
	out := r.Pets[:0]
	for i := range r.Pets {
		if r.Pets[i].Slot != slot {
			out = append(out, r.Pets[i])
		}
	}
	r.Pets = out
}

func replacePet(r *record, p petRec) {
	for i := range r.Pets {
		if r.Pets[i].Slot == p.Slot {
			r.Pets[i] = p
			return
		}
	}
	r.Pets = append(r.Pets, p)
}

func mustPetTemplate(id int) gamedata.PetTemplate {
	t, _ := gamedata.PetByID(id)
	return t
}

// sellCost 是卖这只宠物的店价（原版 NPC_GetCost）：稀有度用模板的 CHAR_RARE，魅力系数由
// 人物的有效魅力（含装备）和宠物当前的忠诚（Loyalty，同宠物页）算出。
func sellCost(r *record, shop *gamedata.NpcPetshop, p *petRec) int {
	tmpl := mustPetTemplate(p.TemplateID)
	host := gamedata.EffectiveCharm(r.Charm, equipBonus(r.Items).Charm)
	ai := gamedata.Loyalty(r.Level, host, r.Transmigration, p.Level, tmpl.ModAI, p.VariableAI)
	return shop.SellCost(p.Level, 1, tmpl.Rare, p.TemplateID, gamedata.SellCharm(host, ai))
}

// petBuyCost 是买入价：原版没有买宠（宠物店只收宠），用同一估价公式 ×10 作占位
// （1 级、无稀有加成、不打魅力折扣），见 STAGE-73 评论。
func petBuyCost(shop *gamedata.NpcPetshop, tmplID int) int {
	return shop.SellCost(gamedata.NewPetLevel, 1, 0, tmplID, 100) * 10
}
