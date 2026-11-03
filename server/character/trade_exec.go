package character

// STAGE-294：交易的校验、预演和事务交换（docs/trade.md 2.4、2.6、4.2）。
// 预演（锁定时）和交换（确定时）走同一个 tradeCheck，保证预演通过就等于能交换。
// 交换时先取两个角色的锁（按 charID 从小到大），再在一个数据库事务里重新读取、重新校验、过户。

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

const (
	tradeBagFirst     = 5  // 背包可交易格 5–19
	tradeBagLast      = 19 //
	tradeLevelGap     = 5  // 宠物比人物高 5 级以上，未转生的接收方不能收
	tradeExecTimeout  = 10 * time.Second
	tradeSwapPetState = petStateRest // 过户后的宠物状态：休息（docs/trade.md 2.4）
)

// tradeItemRow 是背包里一件物品的数据库行（行 id 用于过户）。
type tradeItemRow struct {
	rowID                          int64
	slot, itemID, count            int
	durability, maxDur, crushLevel int
	merged                         bool
}

// tradeHolding 是一个角色在交易里的数据库快照。
type tradeHolding struct {
	charID    int64
	account   string
	name      string
	level     int
	trans     int
	gold      int64
	items     []tradeItemRow
	pets      []petRec
	petRowIDs map[int]int64 // 宠物栏位 → 行 id
}

func (h *tradeHolding) itemAt(slot int) *tradeItemRow {
	for i := range h.items {
		if h.items[i].slot == slot {
			return &h.items[i]
		}
	}
	return nil
}

// tradeFail 是校验或交换失败：code、谁的问题（角色编号）、以及这一方报价里已经不在背包里、应当移除的格号。
type tradeFail struct {
	code    string
	who     int64
	removed []int
	msg     string
}

func (f *tradeFail) err() *protocol.Error {
	return protocol.Errorf(f.code, "%s", f.msg)
}

// tradeReadHolding 读一个角色的快照。lock 为真时加行锁（事务里用）。
func tradeReadHolding(ctx context.Context, q dbQuerier, charID int64, lock bool) (*tradeHolding, error) {
	h := &tradeHolding{charID: charID, petRowIDs: map[int]int64{}}
	cq := `SELECT name, level, transmigration, gold FROM characters WHERE id = $1`
	if lock {
		cq += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, cq, charID)
	if err != nil {
		return nil, err
	}
	if rows.Next() {
		if err := rows.Scan(&h.name, &h.level, &h.trans, &h.gold); err != nil {
			rows.Close()
			return nil, err
		}
	}
	rows.Close()
	iq := `SELECT id, slot, item_id, count, merged, durability, max_durability, crush_level FROM character_items WHERE character_id = $1 ORDER BY slot`
	if lock {
		iq += ` FOR UPDATE`
	}
	irows, err := q.Query(ctx, iq, charID)
	if err != nil {
		return nil, err
	}
	for irows.Next() {
		var it tradeItemRow
		if err := irows.Scan(&it.rowID, &it.slot, &it.itemID, &it.count, &it.merged, &it.durability, &it.maxDur, &it.crushLevel); err != nil {
			irows.Close()
			return nil, err
		}
		h.items = append(h.items, it)
	}
	irows.Close()
	pq := `SELECT id, slot FROM character_pets WHERE character_id = $1`
	if lock {
		pq += ` FOR UPDATE`
	}
	prows, err := q.Query(ctx, pq, charID)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var id int64
		var slot int
		if err := prows.Scan(&id, &slot); err != nil {
			prows.Close()
			return nil, err
		}
		h.petRowIDs[slot] = id
	}
	prows.Close()
	h.pets, err = readPets(ctx, q, charID, false)
	return h, err
}

// tradeFreeBag 返回 h 在交出 out 格之后的空背包格（升序）。
func tradeFreeBag(h *tradeHolding, out []int) []int {
	gone := map[int]bool{}
	for _, s := range out {
		gone[s] = true
	}
	taken := map[int]bool{}
	for _, it := range h.items {
		if !gone[it.slot] {
			taken[it.slot] = true
		}
	}
	var free []int
	for s := tradeBagFirst; s <= tradeBagLast; s++ {
		if !taken[s] {
			free = append(free, s)
		}
	}
	return free
}

// tradeFreePets 返回 h 在交出 out 栏之后的空宠物栏（只算随身的 0–19，升序）。
func tradeFreePets(h *tradeHolding, out int) []int {
	taken := map[int]bool{}
	for _, p := range h.pets {
		if p.Slot < MaxPetSlots && p.Slot != out {
			taken[p.Slot] = true
		}
	}
	var free []int
	for s := 0; s < MaxPetSlots; s++ {
		if !taken[s] {
			free = append(free, s)
		}
	}
	return free
}

// tradeCheck 是交换前的统一校验（docs/trade.md 2.6），预演和交换共用。通过返回 nil。
func tradeCheck(a, b *tradeHolding, offA, offB tradeOffer) *tradeFail {
	hs := [2]*tradeHolding{a, b}
	os := [2]tradeOffer{offA, offB}
	for i, h := range hs {
		o := os[i]
		var removed []int
		for _, slot := range o.Items {
			if h.itemAt(slot) == nil {
				removed = append(removed, slot)
			}
		}
		if len(removed) > 0 {
			return &tradeFail{code: protocol.CodeEmptySlot, who: h.charID, removed: removed, msg: h.name + " 的物品已经不在背包里"}
		}
		if o.Gold > h.gold {
			return &tradeFail{code: protocol.CodeNotEnoughGold, who: h.charID, msg: h.name + " 的石币不够"}
		}
		if o.Pet >= 0 {
			p := findPet(h.pets, o.Pet)
			if p == nil {
				return &tradeFail{code: protocol.CodeEmptySlot, who: h.charID, msg: h.name + " 的宠物已经不在宠物栏里"}
			}
			if p.State == petStateRide {
				return &tradeFail{code: protocol.CodePetRiding, who: h.charID, msg: h.name + " 的宠物正在骑乘，先下骑"}
			}
		}
	}
	// 接收方的容量、石币上限、宠物等级（按接收方算）
	for i := 0; i < 2; i++ {
		recv, give := hs[1-i], hs[i] // give 的报价交给 recv
		gOff, rOff := os[i], os[1-i]
		if len(gOff.Items) > len(tradeFreeBag(recv, rOff.Items)) {
			return &tradeFail{code: protocol.CodeBagFull, who: recv.charID, msg: recv.name + " 的背包空间不够"}
		}
		if recv.gold-rOff.Gold+gOff.Gold > battle.MaxHaveGold {
			return &tradeFail{code: protocol.CodeGoldFull, who: recv.charID, msg: recv.name + " 的石币超过携带上限"}
		}
		if gOff.Pet >= 0 {
			if len(tradeFreePets(recv, rOff.Pet)) == 0 {
				return &tradeFail{code: protocol.CodePetFull, who: recv.charID, msg: recv.name + " 的宠物栏已满"}
			}
			p := findPet(give.pets, gOff.Pet)
			if p != nil && recv.trans == 0 && p.Level-recv.level > tradeLevelGap {
				return &tradeFail{code: protocol.CodePetTooStrong, who: recv.charID, msg: recv.name + " 的等级不够，无法照顾交易后的宠物"}
			}
		}
	}
	return nil
}

// tradeMove 是一次过户：物品或宠物的行，搬到 to 的 slot。
type tradeMove struct {
	pet   bool
	rowID int64
	to    int64
	slot  int
}

// tradePlan 是校验通过后的过户计划：每个角色的新石币和要做的过户。
type tradePlan struct {
	moves []tradeMove
	gold  map[int64]int64
}

// planTrade 生成过户计划。调用前已经 tradeCheck 通过。
func planTrade(a, b *tradeHolding, offA, offB tradeOffer) tradePlan {
	plan := tradePlan{gold: map[int64]int64{
		a.charID: a.gold - offA.Gold + offB.Gold,
		b.charID: b.gold - offB.Gold + offA.Gold,
	}}
	hs := [2]*tradeHolding{a, b}
	os := [2]tradeOffer{offA, offB}
	for i := 0; i < 2; i++ {
		recv, gOff, rOff, give := hs[1-i], os[i], os[1-i], hs[i]
		free := tradeFreeBag(recv, rOff.Items)
		for j, slot := range gOff.Items {
			it := give.itemAt(slot)
			plan.moves = append(plan.moves, tradeMove{rowID: it.rowID, to: recv.charID, slot: free[j]})
		}
		if gOff.Pet >= 0 {
			p := findPet(give.pets, gOff.Pet)
			plan.moves = append(plan.moves, tradeMove{pet: true, rowID: give.petRowIDs[p.Slot], to: recv.charID, slot: tradeFreePets(recv, rOff.Pet)[0]})
		}
	}
	return plan
}

// tradePreflight 是锁定前的预演：读两边的快照（不加锁），跑 tradeCheck。通过返回 nil；失败的错误说明“谁”用 who 给出。
func (s *Service) tradePreflight(tr *trade) *tradeFail {
	ctx, cancel := context.WithTimeout(context.Background(), tradeExecTimeout)
	defer cancel()
	a, errA := tradeReadHolding(ctx, s.db, tr.Sides[0].CharID, false)
	b, errB := tradeReadHolding(ctx, s.db, tr.Sides[1].CharID, false)
	if errA != nil || errB != nil {
		return &tradeFail{code: protocol.CodeInternal, msg: "读取背包失败"}
	}
	return tradeCheck(a, b, tr.Sides[0].Offer, tr.Sides[1].Offer)
}

// tradeExecute 是确定后的交换（docs/trade.md 2.4）：取两个角色的锁，事务内重新读取、校验、过户、写日志，
// 提交后更新内存、推送 inventory_changed、pets_changed，最后 trade_ended。调用时持有 tradeHub.mu。
// 返回 (失败, 数据库错误)：失败时什么都没改；数据库错误时已回滚，由调用方取消交易（INTERNAL）。
func (s *Service) tradeExecute(tr *trade) (*tradeFail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tradeExecTimeout)
	defer cancel()
	sa, sb := tr.Sides[0], tr.Sides[1]
	unlock := lockPresences(sa.P, sb.P)
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// 按 charID 从小到大锁行（同一顺序，避免数据库死锁）
	first, second := sa, sb
	if second.CharID < first.CharID {
		first, second = second, first
	}
	hFirst, err := tradeReadHolding(ctx, tx, first.CharID, true)
	if err != nil {
		return nil, err
	}
	hSecond, err := tradeReadHolding(ctx, tx, second.CharID, true)
	if err != nil {
		return nil, err
	}
	hA, hB := hFirst, hSecond
	if hA.charID != sa.CharID {
		hA, hB = hSecond, hFirst
	}
	hA.account = tradeAccountName(ctx, tx, sa.P.acct)
	hB.account = tradeAccountName(ctx, tx, sb.P.acct)
	if fail := tradeCheck(hA, hB, sa.Offer, sb.Offer); fail != nil {
		return fail, nil
	}
	plan := planTrade(hA, hB, sa.Offer, sb.Offer)
	if err := applyTradePlan(ctx, tx, plan, hA, hB); err != nil {
		return nil, err
	}
	if err := insertTradeLog(ctx, tx, tr, hA, hB, sa, sb); err != nil {
		return nil, err
	}
	// 提交前在事务里读出最终结果，提交后直接写回内存
	items := map[int64][]itemRec{}
	pets := map[int64][]petRec{}
	for _, side := range []*tradeSide{sa, sb} {
		its, err := readItems(ctx, tx, side.CharID, false)
		if err != nil {
			return nil, err
		}
		ps, err := readPets(ctx, tx, side.CharID, false)
		if err != nil {
			return nil, err
		}
		items[side.CharID], pets[side.CharID] = its, ps
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for _, side := range []*tradeSide{sa, sb} {
		r := &side.P.rec
		r.Gold = plan.gold[side.CharID]
		r.Items = items[side.CharID]
		r.Pets = pets[side.CharID]
		side.P.pushAll("inventory_changed", r.inventoryView())
		side.P.pushAll("pets_changed", r.petsView())
	}
	summaries := tradeSummaries(sa, sb)
	locked = false
	unlock()
	s.trades.finishLocked(tr, "completed", "", map[int64]any{sa.CharID: nil, sb.CharID: nil}, summaries)
	return nil, nil
}

// tradeAccountName 读账号名（写 trade_logs 用，名字是当时的快照）。
func tradeAccountName(ctx context.Context, tx pgx.Tx, acct int64) string {
	var name string
	_ = tx.QueryRow(ctx, `SELECT name FROM accounts WHERE id = $1`, acct).Scan(&name)
	return name
}

// applyTradePlan 在事务里执行过户和石币更新（物品和宠物行整体搬到对方，耐久、合成标记等列原样保留）。
func applyTradePlan(ctx context.Context, tx pgx.Tx, plan tradePlan, a, b *tradeHolding) error {
	for _, m := range plan.moves {
		if m.pet {
			if _, err := tx.Exec(ctx, `UPDATE character_pets SET character_id = $2, slot = $3, state = $4 WHERE id = $1`,
				m.rowID, m.to, m.slot, tradeSwapPetState); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE character_items SET character_id = $2, slot = $3 WHERE id = $1`,
			m.rowID, m.to, m.slot); err != nil {
			return err
		}
	}
	for _, h := range []*tradeHolding{a, b} {
		if _, err := tx.Exec(ctx, `UPDATE characters SET gold = $2 WHERE id = $1`, h.charID, plan.gold[h.charID]); err != nil {
			return err
		}
	}
	return nil
}

// tradeSummaries 是 trade_ended 的 summary（协议 5.72），按 CharID 分别给出每一方的视角。
// STAGE-294：甲的 summary 由 tradeSummary(甲, 乙) 生成，乙的由 tradeSummary(乙, 甲) 生成。
func tradeSummaries(sa, sb *tradeSide) map[int64]map[string]any {
	return map[int64]map[string]any{
		sa.CharID: tradeSummary(sa, sb),
		sb.CharID: tradeSummary(sb, sa),
	}
}

// tradeSummary 是 me 视角的摘要：gave 是 me 报价给出的，got 是 peer 报价给出的（即 me 拿到的）。
func tradeSummary(me, peer *tradeSide) map[string]any {
	return map[string]any{
		"gave": map[string]any{"gold": me.Offer.Gold},
		"got":  map[string]any{"gold": peer.Offer.Gold},
	}
}

// insertTradeLog 写 trade_logs（docs/trade.md 2.7）：一笔成功交易一行。
func insertTradeLog(ctx context.Context, tx pgx.Tx, tr *trade, a, b *tradeHolding, sa, sb *tradeSide) error {
	viaAgent := sa.P.controller == protocol.RoleAgent || sb.P.controller == protocol.RoleAgent
	aItems, aPet := tradeLogSide(a, sa.Offer)
	bItems, bPet := tradeLogSide(b, sb.Offer)
	var aPetJSON, bPetJSON *string
	if aPet != nil {
		s := string(aPet)
		aPetJSON = &s
	}
	if bPet != nil {
		s := string(bPet)
		bPetJSON = &s
	}
	_, err := tx.Exec(ctx, `INSERT INTO trade_logs
		(a_char_id, b_char_id, a_name, b_name, a_account, b_account, a_gold, b_gold, a_items, b_items, a_pet, b_pet, via_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		a.charID, b.charID, a.name, b.name, a.account, b.account, sa.Offer.Gold, sb.Offer.Gold,
		string(aItems), string(bItems), aPetJSON, bPetJSON, viaAgent)
	return err
}

// tradeLogSide 生成一方的物品和宠物 JSON（物品行 id、模板编号、名字、数量、耐久；宠物行 id、种族、名字、等级、转生次数、素质点）。
func tradeLogSide(h *tradeHolding, o tradeOffer) ([]byte, []byte) {
	items := []map[string]any{}
	for _, slot := range o.Items {
		it := h.itemAt(slot)
		if it == nil {
			continue
		}
		tpl, _ := gamedata.ItemByID(it.itemID)
		items = append(items, map[string]any{
			"row": it.rowID, "item_id": it.itemID, "name": tpl.Name, "count": it.count, "durability": it.durability,
		})
	}
	ib, _ := json.Marshal(items)
	var pb []byte
	if p := findPet(h.pets, o.Pet); o.Pet >= 0 && p != nil {
		tpl, _ := gamedata.PetByID(p.TemplateID)
		pb, _ = json.Marshal(map[string]any{
			"row": h.petRowIDs[p.Slot], "template_id": p.TemplateID, "species": tpl.Name, "name": p.Name,
			"level": p.Level, "transmigration": p.Transmigration,
			"base": map[string]int{"vital": p.Base.Vital, "strength": p.Base.Strength, "toughness": p.Base.Toughness, "dexterity": p.Base.Dexterity},
		})
	}
	return ib, pb
}
