package character

// STAGE-216：宠物的成长率和五档素质（docs/protocol.md 5.19）。只在服务端现算，不存；
// 纯函数，不碰数据库。起始值来自 STAGE-215 的 petRec.Origin，素质点来自 petRec.Base。

import (
	"math"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 成长率的依据（growth.basis）。
const (
	growthRaised  = "raised"  // 起始等级是 1，从 1 级起养
	growthCaught  = "caught"  // 起始等级大于 1，捕获或获得时已有等级，只统计到手之后
	growthUnknown = "unknown" // 历史宠物，没有起始记录，不倒推
)

// growthOf 算宠物的成长率：每升一级的平均涨幅 =（现在的值 − 起始值）÷（现在的等级 − 起始等级），
// 四舍五入到两位小数。“值”是协议里发给客户端的整数：hp 用最大 HP（不是当前 HP），其余用攻击、防御、敏捷的算法。
// 历史宠物（Origin 为 nil）是 unknown；现在的等级不高于起始等级（还没升过级）时四项为 nil（null）。
func growthOf(p *petRec) growthView {
	if p.Origin == nil {
		return growthView{Basis: growthUnknown}
	}
	from := p.Origin.Level
	v := growthView{Basis: growthCaught, FromLevel: &from}
	if from <= 1 {
		v.Basis = growthRaised
	}
	gained := p.Level - from
	if gained <= 0 {
		return v
	}
	perLevel := func(now, start int) *float64 {
		r := math.Round(float64(now-start)/float64(gained)*100) / 100
		return &r
	}
	cur, origin := p.Stats, p.Origin.Stats
	v.Rates = ratesView{
		HP:      perLevel(gamedata.PetMaxHP(cur), gamedata.PetMaxHP(origin)),
		Attack:  perLevel(gamedata.PetAttack(cur), gamedata.PetAttack(origin)),
		Defense: perLevel(gamedata.PetDefense(cur), gamedata.PetDefense(origin)),
		Agility: perLevel(gamedata.PetAgility(cur), gamedata.PetAgility(origin)),
	}
	return v
}

// qualityOf 列出四项素质，顺序固定为 vital、strength、toughness、dexterity。
// points 是 petRec.Base（生成时 ±2 之后、追加 10 点之前的素质点）；tier = points −（模板资质 + 2），
// 不截断：超出 −4 到 0 的照实发。
func qualityOf(p *petRec, tmpl gamedata.PetTemplate) []qualityView {
	items := [...]struct {
		stat        string
		points, tpl int
	}{
		{"vital", p.Base.Vital, tmpl.Base.Vital},
		{"strength", p.Base.Strength, tmpl.Base.Strength},
		{"toughness", p.Base.Toughness, tmpl.Base.Toughness},
		{"dexterity", p.Base.Dexterity, tmpl.Base.Dexterity},
	}
	out := make([]qualityView, 0, len(items))
	for _, it := range items {
		out = append(out, qualityView{Stat: it.stat, Points: it.points, Tier: it.points - (it.tpl + 2)})
	}
	return out
}
