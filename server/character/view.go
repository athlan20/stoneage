package character

import (
	"fmt"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 下面是协议里的响应结构（docs/protocol.md 5.5–5.8）。

type locationView struct {
	Floor   int    `json:"floor"`
	MapName string `json:"map_name"`
}

type characterView struct {
	Slot       int          `json:"slot"`
	Name       string       `json:"name"`
	Level      int          `json:"level"`
	Appearance Appearance   `json:"appearance"`
	Location   locationView `json:"location"`
}

type positionView struct {
	Floor     int    `json:"floor"`
	MapName   string `json:"map_name"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Direction int    `json:"direction"`
}

// statusView 是 enter_world / status 响应里的 status 对象（docs/protocol.md 5.8）。
// 角色页（STAGE-23）用到的能力、衍生数值、属性、转生、称号和未分配点数也在这里。
type statusView struct {
	Level          int         `json:"level"`
	HP             int         `json:"hp"`
	MaxHP          int         `json:"max_hp"`
	MP             int         `json:"mp"`
	MaxMP          int         `json:"max_mp"`
	Exp            int64       `json:"exp"`
	NextExp        int64       `json:"next_exp"`
	Gold           int64       `json:"gold"`
	Transmigration int         `json:"transmigration"`
	Title          string      `json:"title"`
	SkillPoints    int         `json:"skill_points"`
	Stats          Stats       `json:"stats"`
	Derived        derivedView `json:"derived"`
	Elements       Elements    `json:"elements"`
	Magic          []magicView `json:"magic"`
	RideLevel      int         `json:"ride_level"`
	Metamo         *metamoView `json:"metamo,omitempty"` // 变幻的精灵的变身中状态（STAGE-203）；没有变身时省略
}

// magicView 是 status 里精灵栏位的一项（docs/protocol.md 5.8）。
type magicView struct {
	Slot       int                 `json:"slot"`
	ID         int                 `json:"id"`
	Name       string              `json:"name"`
	MP         int                 `json:"mp"`
	TargetType gamedata.TargetType `json:"target_type"`
	Effect     string              `json:"effect"`
}

// derivedView 是衍生数值：由服务端按旧公式算好，客户端只显示。
type derivedView struct {
	Attack  int `json:"attack"`
	Defense int `json:"defense"`
	Agility int `json:"agility"`
	Charm   int `json:"charm"`
	Luck    int `json:"luck"`
}

func mapName(floor int) string {
	name, _ := gamedata.MapName(floor)
	return name
}

func (r *record) view() characterView {
	return characterView{
		Slot: r.Slot, Name: r.Name, Level: r.Level, Appearance: r.Appearance,
		Location: locationView{Floor: r.Floor, MapName: mapName(r.Floor)},
	}
}

func (r *record) position() positionView {
	return positionView{Floor: r.Floor, MapName: mapName(r.Floor), X: r.X, Y: r.Y, Direction: r.Direction}
}

func (r *record) status() statusView {
	s := r.Stats
	eq := equipBonus(r.Items)
	return statusView{
		Level: r.Level, HP: r.HP, MaxHP: r.maxHP(), MP: r.MP, MaxMP: gamedata.NewPlayerMaxMP,
		Exp: r.Exp, NextExp: gamedata.NextExp(r.Level), Gold: r.Gold,
		Transmigration: r.Transmigration,
		Title:          "", // 新服务端还没有称号，客户端显示“无”
		SkillPoints:    r.SkillPoints,
		Stats:          s,
		Derived: derivedView{
			Attack:  nonNegative(gamedata.Attack(s.Vital, s.Strength, s.Toughness, s.Dexterity) + eq.Attack),
			Defense: nonNegative(gamedata.Defense(s.Vital, s.Strength, s.Toughness, s.Dexterity) + eq.Defence),
			Agility: nonNegative(gamedata.Agility(s.Dexterity) + eq.Agility),
			Charm:   gamedata.EffectiveCharm(r.Charm, eq.Charm),
			Luck:    gamedata.EffectiveLuck(r.Luck, eq.Luck),
		},
		Elements:  r.Elements,
		Magic:     r.magicView(),
		RideLevel: r.RideLevel,
		Metamo:    r.metamoView(),
	}
}

// magicView 列出有精灵的栏位（人物自己的和装备自带的，见 magicSlots）；空栏位由客户端按 magic 的最大栏位数补上。
func (r *record) magicView() []magicView {
	out := make([]magicView, 0, gamedata.MaxMagicSlots)
	for i, m := range r.magicSlots() {
		if m.ID < 0 {
			continue
		}
		if t, ok := gamedata.MagicByID(m.ID); ok {
			out = append(out, magicView{Slot: i, ID: m.ID, Name: t.Name, MP: m.MP, TargetType: t.BattleTarget(), Effect: t.Effect()})
		}
	}
	return out
}

// nonNegative 把衍生数值截到 0 以上：装备的负数加成（如小斧头 防 -3）不会让数值变成负数。
func nonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func (r *record) maxHP() int {
	return gamedata.MaxHP(r.Stats.Vital, r.Stats.Strength, r.Stats.Toughness, r.Stats.Dexterity)
}

// inventorySlotView 是 inventory / inventory_changed 里的一个格子（docs/protocol.md 5.17）。
type inventorySlotView struct {
	Slot       int                 `json:"slot"`
	ItemID     int                 `json:"item_id"`
	Name       string              `json:"name"`
	Effect     string              `json:"effect"`
	Type       int                 `json:"type"`
	Count      int                 `json:"count"`
	TargetType gamedata.TargetType `json:"target_type"`
	// FieldTargetType 是世界中「使用」的目标类型（STAGE-326），只在和 target_type 不同时才带（缺省同 target_type）：
	// 只能在战斗外用的忠诚道具选宠物，战斗列表仍看 target_type。
	FieldTargetType *gamedata.TargetType `json:"field_target_type,omitempty"`
	// 装备耐久（STAGE-209）：只有有耐久的物品（最大耐久 > 0）才带这三项；crush_level 缺省为 0（完好）。
	Durability    int `json:"durability,omitempty"`
	MaxDurability int `json:"max_durability,omitempty"`
	CrushLevel    int `json:"crush_level,omitempty"`
	// 精工（STAGE-298，docs/protocol.md 5.73）：类别码非空的物品才有。
	Inlay *inlayView `json:"inlay,omitempty"`
}

// inventoryView 是 inventory 响应和 inventory_changed 推送的内容：
// 名字和效果文字来自物品模板（已转 UTF-8），客户端不需要物品数据。
// 模板找不到的物品（数据里漏了这个编号）显示成“未知物品 #编号”：仍占格子，玩家能看见、能丢弃，
// 不能静默跳过（STAGE-191：否则格子看起来是空的，买东西却报空格不足）。
func (r *record) inventoryView() map[string]any {
	slots := make([]inventorySlotView, 0, len(r.Items))
	for _, it := range r.Items {
		slots = append(slots, inventorySlotOf(it))
	}
	return map[string]any{"gold": r.Gold, "slots": slots}
}

// inventorySlotOf 是一个格子的显示内容（inventory、inventory_changed 和精工的响应共用）。
func inventorySlotOf(it itemRec) inventorySlotView {
	def, ok := gamedata.ItemByID(it.ItemID)
	if !ok {
		return inventorySlotView{
			Slot: it.Slot, ItemID: it.ItemID, Name: fmt.Sprintf("未知物品 #%d", it.ItemID),
			Effect: "物品表里没有这个编号，可以丢弃", Count: it.Count, TargetType: gamedata.TargetNone,
		}
	}
	eff := effectiveTemplate(def, it.Inlay) // 镶过的装备用实例的效果文字和使用函数（STAGE-298）
	v := inventorySlotView{
		Slot: it.Slot, ItemID: it.ItemID, Name: def.Name, Effect: eff.Effect, Type: def.Type, Count: it.Count,
		TargetType: eff.BattleTarget(), Inlay: itemInlayView(it, def),
	}
	if ft := eff.FieldTarget(); ft != v.TargetType {
		v.FieldTargetType = &ft
	}
	if it.MaxDurability > 0 {
		v.Durability, v.MaxDurability, v.CrushLevel = it.Durability, it.MaxDurability, it.CrushLevel
	}
	return v
}
