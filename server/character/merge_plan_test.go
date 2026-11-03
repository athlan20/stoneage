package character

import (
	"reflect"
	"testing"

	"github.com/athlan20/stoneage/server/protocol"
)

// STAGE-207：合成的纯函数部分（不连数据库）：参数检查、材料检查、结果、背包改动。
func TestCheckMergeSlots(t *testing.T) {
	ints := func(v ...int) *[]int { return &v }
	cases := []struct {
		name  string
		items *[]int
		code  string // 空表示通过
	}{
		{"缺 items", nil, protocol.CodeBadRequest},
		{"空列表", ints(), protocol.CodeBadRequest},
		{"只有一件", ints(5), protocol.CodeBadRequest},
		{"16 件", ints(5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 5), protocol.CodeBadRequest},
		{"装备格 4", ints(4, 5), protocol.CodeBadRequest},
		{"格号 20", ints(5, 20), protocol.CodeBadRequest},
		{"重复", ints(5, 6, 5), protocol.CodeBadRequest},
		{"两件", ints(5, 19), ""},
		{"十五件", ints(5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			slots, perr := checkMergeSlots(c.items)
			if c.code == "" {
				if perr != nil || !reflect.DeepEqual(slots, *c.items) {
					t.Fatalf("应通过，得到 %v %v", slots, perr)
				}
				return
			}
			if perr == nil || perr.Code != c.code || perr.Message == "" {
				t.Fatalf("应回复 %s（带说明），得到 %v", c.code, perr)
			}
		})
	}
}

// 扣料：叠放的格子只扣 1 件，扣光的格子空出来；产物放进扣料之后的第一个空格，并带标记。
func TestConsumeAndPlace(t *testing.T) {
	bag := []itemRec{
		{Slot: 5, ItemID: mergeAxeA, Count: 1},
		{Slot: 6, ItemID: mergeAxeB, Count: 3},
		{Slot: 9, ItemID: mergeDish, Count: 1},
	}
	next, placed, err := consumeAndPlace(bag, []int{5, 6}, mergeResult{Kind: "created", Placed: true, Product: mergeRecipe, Merged: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []itemRec{
		{Slot: 5, ItemID: mergeRecipe, Count: 1, Merged: true},
		{Slot: 6, ItemID: mergeAxeB, Count: 2},
		{Slot: 9, ItemID: mergeDish, Count: 1},
	}
	if !reflect.DeepEqual(next, want) || placed == nil || placed.Slot != 5 {
		t.Errorf("背包 = %+v（放进 %v），应为 %+v", next, placed, want)
	}
	if !reflect.DeepEqual(bag[1], itemRec{Slot: 6, ItemID: mergeAxeB, Count: 3}) {
		t.Error("入参不应被改动")
	}
}

// 没有产物（食物与道具混合）：只扣料，不放东西。
func TestConsumeAndPlaceIllegalPlacesNothing(t *testing.T) {
	bag := []itemRec{{Slot: 5, ItemID: mergeDish, Count: 1}, {Slot: 6, ItemID: mergeAxeA, Count: 1}, {Slot: 7, ItemID: mergeAxeB, Count: 1}}
	next, placed, err := consumeAndPlace(bag, []int{5, 6}, mergeResult{Kind: "illegal"})
	if err != nil || placed != nil || !reflect.DeepEqual(next, []itemRec{{Slot: 7, ItemID: mergeAxeB, Count: 1}}) {
		t.Errorf("illegal 应只扣料: %+v %v %v", next, placed, err)
	}
}

// 冷却或失败时还回的是参与材料的复制品，不带合成品标记。
func TestRunMergeCoolingReturnsOneMaterial(t *testing.T) {
	mats := []int{mergeDish, mergeAxeA}
	res, err := runMerge(mats, mergePetTpl, true, func(n int) int { return n - 1 })
	if err != nil || res.Kind != "cooldown" || !res.Placed || res.Product != mergeAxeA || res.Merged {
		t.Errorf("冷却应还回最后一件材料的复制品，不带标记: %+v %v", res, err)
	}
}

// 食物与道具混合：引擎判 ErrMixed，结果是 illegal，不随机。
func TestRunMergeMixedIsIllegal(t *testing.T) {
	res, err := runMerge([]int{mergeDish, mergeAxeA}, mergePetTpl, false, func(n int) int {
		t.Fatalf("混合时不应抽随机数")
		return 0
	})
	if err != nil || res.Kind != "illegal" || res.Placed {
		t.Errorf("混合应是 illegal 且没有产物: %+v %v", res, err)
	}
}

// 不能作材料的物品：整次请求拒绝，指出是哪件、哪一格。
func TestMaterialIDsRejectsNonMaterial(t *testing.T) {
	bag := []itemRec{{Slot: 5, ItemID: mergeAxeA, Count: 1}, {Slot: 8, ItemID: mergeNotMat, Count: 1}}
	if _, perr := materialIDs(bag, []int{5, 8}); perr == nil || perr.Code != protocol.CodeItemUnusable {
		t.Fatalf("非材料应回复 ITEM_UNUSABLE: %v", perr)
	}
	if _, perr := materialIDs(bag, []int{5, 9}); perr == nil || perr.Code != protocol.CodeEmptySlot {
		t.Fatalf("空格应回复 EMPTY_SLOT: %v", perr)
	}
	ids, perr := materialIDs(bag, []int{5, 5})
	if perr != nil || !reflect.DeepEqual(ids, []int{mergeAxeA, mergeAxeA}) {
		t.Errorf("材料按请求顺序返回: %v %v", ids, perr)
	}
}

func TestFirstEmptyBagSlot(t *testing.T) {
	if got := firstEmptyBagSlot(nil); got != 5 {
		t.Errorf("空背包应是 5 号格，得到 %d", got)
	}
	var full []itemRec
	for slot := 5; slot <= 19; slot++ {
		full = append(full, itemRec{Slot: slot, ItemID: mergeAxeA, Count: 1})
	}
	if got := firstEmptyBagSlot(full); got != -1 {
		t.Errorf("背包 5–19 满时应返回 -1，得到 %d", got)
	}
	if got := firstEmptyBagSlot(append(full[:3:3], full[4:]...)); got != 8 {
		t.Errorf("8 号格空了应返回 8，得到 %d", got)
	}
}
