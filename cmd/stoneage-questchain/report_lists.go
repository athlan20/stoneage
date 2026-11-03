package main

// 报告的清单一节（STAGE-213）：外部补给道具、需要自备的宠物、脚本没有设置的事件（外部来源）、暗号。
// 清单按每个任务的推荐推导（最窄的能走完的场景）汇总，见 report.go 的 analysis。

import (
	"fmt"
	"sort"
	"strings"
)

func (a *analysis) writeLists(b *strings.Builder) {
	items, pets := map[int][]int{}, map[int][]int{}
	events := map[Fact][]int{}
	for _, row := range a.rows {
		if row.pick < 0 {
			continue
		}
		for _, f := range row.der.External {
			switch f.Kind {
			case KindItem:
				items[f.N] = append(items[f.N], row.q.No)
			case KindPet:
				pets[f.N] = append(pets[f.N], row.q.No)
			case KindEnd, KindNow:
				events[f] = append(events[f], row.q.No)
			}
		}
	}
	b.WriteString("## 清单\n\n")
	b.WriteString("清单按每个任务的推荐推导（最窄的能走完的场景）汇总，括号里是用到它的任务编号。\n\n")
	b.WriteString("### 外部补给道具\n\n")
	b.WriteString("没有任何脚本会给的道具，按“商店或战斗掉落”视为可得。要确认玩家真能拿到。\n\n")
	writeIDList(b, items, func(id int) string { return a.m.itemName(id) }, "道具")
	b.WriteString("\n### 需要自备的宠物\n\n")
	b.WriteString("脚本没人送、但宠物表里可捕捉（`PetFlag > 0`）的宠物，视为可得，要自己抓。\n\n")
	writeIDList(b, pets, func(id int) string { return a.m.petName(id) }, "宠物模板")
	b.WriteString("\n### 脚本没有设置的事件（按外部可得处理）\n\n")
	b.WriteString("条件里引用了，但任务脚本都不设置的事件，由别的系统设置（来源见下，已查的 C 代码）。这些任务要先让对应的系统跑通，才能走完。\n\n")
	writeEventList(b, events)
	b.WriteString("\n")
	a.writeKeywords(b)
}

func (a *analysis) writeKeywords(b *strings.Builder) {
	b.WriteString("### 需要说的暗号\n\n")
	total := 0
	var lines []string
	for _, u := range a.m.Units {
		if u.Block.Keyword == "" {
			continue
		}
		total++
		if len(u.produce) == 0 {
			continue
		}
		var effects []string
		for _, f := range u.produce {
			effects = append(effects, factName(f))
		}
		lines = append(lines, fmt.Sprintf("- 「%s」：`%s` 块 %d（%s）：%s", u.Block.Keyword, u.File, u.Index, u.Block.Type, strings.Join(effects, "、")))
	}
	fmt.Fprintf(b, "脚本里共有 %d 个暗号块（同一脚本文件只算一次），其中 %d 个有效果（给道具或设事件）。没有效果的暗号块不影响能否走完。\n\n", total, len(lines))
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n")
}

func factName(f Fact) string {
	switch f.Kind {
	case KindEnd:
		return fmt.Sprintf("完成事件 %d", f.N)
	case KindNow:
		return fmt.Sprintf("进行中事件 %d", f.N)
	case KindItem:
		return fmt.Sprintf("道具 %d", f.N)
	}
	return fmt.Sprintf("宠物模板 %d", f.N)
}

func writeIDList(b *strings.Builder, by map[int][]int, name func(int) string, kind string) {
	if len(by) == 0 {
		b.WriteString("无。\n")
		return
	}
	var ids []int
	for id := range by {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		fmt.Fprintf(b, "- %s %d（%s）：任务 %s\n", kind, id, name(id), joinNos(by[id]))
	}
}

func writeEventList(b *strings.Builder, events map[Fact][]int) {
	if len(events) == 0 {
		b.WriteString("无。\n")
		return
	}
	var keys []Fact
	for f := range events {
		keys = append(keys, f)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Kind != keys[j].Kind {
			return keys[i].Kind < keys[j].Kind
		}
		return keys[i].N < keys[j].N
	})
	for _, f := range keys {
		state := "完成"
		if f.Kind == KindNow {
			state = "进行中"
		}
		fmt.Fprintf(b, "- 事件 %d（%s）：任务 %s；%s\n", f.N, state, joinNos(events[f]), eventSource(f))
	}
}

func (m *Model) itemName(id int) string {
	if m.env.ItemName == nil {
		return ""
	}
	return m.env.ItemName(id)
}

func (m *Model) petName(template int) string {
	if m.env.PetName == nil {
		return ""
	}
	return m.env.PetName(template)
}
