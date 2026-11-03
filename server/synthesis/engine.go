package synthesis

import (
	"errors"
	"sync"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-206：合成引擎（纯函数，不碰数据库、背包和协议）。对应原版 ITEM_mergeItem（item_gen.c:727）
// 和 ITEM_mergeItem_merge（item_gen.c:1042）里和“结果”有关的部分；扣料、冷却、背包、协议是 STAGE-207 的事。

// Kind 是合成类型：原版 searchtable，道具为 0，食物（料理）为 1。
type Kind int

const (
	KindItem Kind = iota // 道具，原版 searchtable=0
	KindFood             // 食物（料理），原版 searchtable=1
)

// Outcome 是一次合成的结果。err 为 nil 时才有效。
type Outcome struct {
	Kind     Kind
	OK       bool // 匹配到了配方
	RecipeID int  // OK 时是配方产物的物品模板号
	Attempts int  // 用了几次匹配尝试（1–5），给测试和日志看；没有原子时为 0
}

// ErrMixed 表示食物与道具混合（原版 ITEM_mergeItem 返回 -10，提示“非法的合成方法”）。
var ErrMixed = errors.New("食物与道具混合")

// ErrTooFew 表示有效材料不足 2 件（原版 cnt > 1 的检查）。
var ErrTooFew = errors.New("有效材料少于 2 件")

// Engine 持有配方索引。索引只建一次，之后所有 Merge 共用；Merge 本身不改动引擎状态，可以并发调用。
type Engine struct {
	idx *recipeIndex
}

var (
	engineOnce sync.Once
	engine     *Engine
)

// NewEngine 返回共用的合成引擎，第一次调用时建配方索引。
func NewEngine() *Engine {
	engineOnce.Do(func() {
		engine = &Engine{idx: buildRecipeIndex()}
	})
	return engine
}

// Merge 对材料（物品模板号，按放入顺序）和宠物（宠物模板号）做一次合成，用 rnd 做随机。
// 不能作材料（CanMergeFrom 为假）的物品不参与；参与材料不足 2 件返回 ErrTooFew；
// 类型由第一件参与材料决定，食物与道具混合返回 ErrMixed。
// 匹配 5 次都失败时返回 OK=false，不算错误：扣料和“随机还一件材料”由调用方决定。
func (e *Engine) Merge(materials []int, petTemplateID int, rnd Rand) (Outcome, error) {
	parts := mergeableMaterials(materials)
	if len(parts) < 2 {
		return Outcome{}, ErrTooFew
	}
	dish := parts[0].tpl.Type == gamedata.ItemTypeDish
	out := Outcome{Kind: KindItem}
	if dish {
		out.Kind = KindFood
	}
	for _, p := range parts[1:] {
		if (p.tpl.Type == gamedata.ItemTypeDish) != dish {
			return out, ErrMixed
		}
	}

	srcs := gatherAtoms(parts)
	if len(srcs) == 0 {
		// 原版 ITEM_mergeItem 在这里返回 -3，既不随机也不匹配，结果是失败。
		return out, nil
	}
	vals := simplifyAtoms(srcs)

	// 随机化。原版 ITEM_mergeItem 的随机化循环（item_gen.c:795）写成 for(i<ingnum)，但循环体的末尾是
	// “匹配 5 次，然后 return”，所以第一次迭代就返回，实际只随机化第一个原子（vals[0]）。
	// 其余原子保持汇总后的值直接参与匹配。这是原版行为，docs/synthesis.md 写的“每个原子都随机化”与源码不符，
	// 已在文档里更正，并列入待确认。
	fixed := petFixedAtoms(petTemplateID)
	vals[0].value = randomizeValue(vals[0].value, findFixed(fixed, vals[0].index), dish, rnd)

	excluded := make(map[int]bool, len(parts))
	for _, p := range parts {
		excluded[p.id] = true
	}
	ideal := len(vals)
	if ideal > maxIngredients {
		ideal = maxIngredients
	}
	// 候选只看数值，和抽取的次数无关，所以只算一次；5 次尝试用同一份候选。
	cands := e.idx.candidates(vals, dish, excluded)
	for a := 1; a <= maxAttempts; a++ {
		out.Attempts = a
		if id, ok := retryMatch(&cands, ideal, rnd); ok {
			out.OK = true
			out.RecipeID = id
			return out, nil
		}
	}
	return out, nil
}
