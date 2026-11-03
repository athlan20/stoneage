package main

// 报告的几节（STAGE-213）：不由任务脚本完成的任务、B 比 A 多走通的任务、只有 C 能走通的任务、整本放不开的任务。清单见 report_lists.go。

import (
	"fmt"
	"sort"
	"strings"
)

// dirGroup 是按目录分组的一组脚本文件。
type dirGroup struct {
	dir   string
	files []string
}

// groupByDir 把脚本文件按目录分组：已知目录按 batchDirs 的顺序，其余按名字排在后面。
func groupByDir(files []string) []dirGroup {
	by := map[string][]string{}
	for _, f := range files {
		by[dirOf(f)] = append(by[dirOf(f)], f)
	}
	var dirs []string
	for _, d := range batchDirs {
		if _, ok := by[d]; ok {
			dirs = append(dirs, d)
		}
	}
	var rest []string
	for d := range by {
		if !contains(batchDirs, d) {
			rest = append(rest, d)
		}
	}
	sort.Strings(rest)
	dirs = append(dirs, rest...)
	out := make([]dirGroup, 0, len(dirs))
	for _, d := range dirs {
		list := by[d]
		sort.Strings(list)
		out = append(out, dirGroup{dir: d, files: list})
	}
	return out
}

// writeUnscripted 写不由任务脚本完成的任务：完成事件没有任何任务脚本设置（EndSetFlg），它们不在三个场景的范围里。
func (a *analysis) writeUnscripted(b *strings.Builder) {
	b.WriteString("## 不由任务脚本完成的任务\n\n")
	b.WriteString("这些任务的完成事件没有任何任务脚本设置（EndSetFlg），是别的 NPC 类型或系统完成的（如飞机、水怪、动物园之类）。")
	b.WriteString("本分析只看任务脚本，所以表里这些行没有判断，要另外核对。\n\n")
	n := 0
	for _, row := range a.rows {
		if row.scripted {
			continue
		}
		n++
		fmt.Fprintf(b, "- %d %s（完成事件 %d）\n", row.q.No, row.q.Name, row.q.EndEvent)
	}
	if n == 0 {
		b.WriteString("无。\n")
	}
	b.WriteString("\n")
}

// writeBFromA 写 B 比 A 多走通的任务，以及每个任务需要新放开的脚本（推导里不在白名单的，按目录分组）。
func (a *analysis) writeBFromA(b *strings.Builder) {
	b.WriteString("## B 比 A 多走通的任务\n\n")
	b.WriteString("每个任务列的是它在场景 B 里的第一条推导用到的、不在白名单里的脚本（要新放开的）。\n\n")
	n := 0
	for _, row := range a.rows {
		if !row.reach[scenB] || row.reach[scenA] {
			continue
		}
		n++
		var need []string
		for _, f := range row.files {
			if !a.rules.Enabled(f) {
				need = append(need, f)
			}
		}
		fmt.Fprintf(b, "### %d %s（事件 %d）\n\n", row.q.No, row.q.Name, row.q.EndEvent)
		fmt.Fprintf(b, "需要新放开的脚本（%d 个）：\n", len(need))
		for _, g := range groupByDir(need) {
			fmt.Fprintf(b, "- %s：%s\n", g.dir, codeList(g.files))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		b.WriteString("无。\n\n")
	}
}

// writeCOnly 写只有 C 能走通的任务：缺什么。推导里的不支持的块直接列出（脚本、块序号从 0 起、类型、事件号和原因）；
// 推导用到的脚本如果整本放不开，列出它的全部不支持块（它的支持块在推导里也用到了，但整本放不开就进不了 B）。
func (a *analysis) writeCOnly(b *strings.Builder) {
	b.WriteString("## 只有 C 能走通的任务\n\n")
	b.WriteString("缺什么：沿场景 C 的推导走到的不支持的块（脚本、块序号从 0 起、类型、事件号和原因），以及推导用到的、整本放不开的脚本的全部不支持块。补上这些，任务才能在 B 里走通。\n\n")
	n := 0
	for _, row := range a.rows {
		if !row.reach[scenC] || row.reach[scenB] {
			continue
		}
		n++
		fmt.Fprintf(b, "### %d %s（事件 %d）\n\n", row.q.No, row.q.Name, row.q.EndEvent)
		listed := map[string]bool{}
		for _, u := range row.der.Units {
			switch {
			case u.Block.Unsupported != "":
				fmt.Fprintf(b, "- `%s` 块 %d（%s，事件 %d）：%s\n", u.File, u.Index, u.Block.Type, u.Block.Event, u.Block.Unsupported)
			case !a.rules.Full(u.File) && !a.rules.Enabled(u.File) && !listed[u.File]:
				listed[u.File] = true
				fmt.Fprintf(b, "- 脚本 `%s` 整本放不开，它的不支持块：\n", u.File)
				for _, v := range a.m.Units {
					if v.File == u.File && v.Block.Unsupported != "" {
						fmt.Fprintf(b, "  - 块 %d（%s，事件 %d）：%s\n", v.Index, v.Block.Type, v.Block.Event, v.Block.Unsupported)
					}
				}
			}
		}
		b.WriteString("\n")
	}
	if n == 0 {
		b.WriteString("无。\n\n")
	}
}

// writeDOnly 写“整本放不开”的任务：逐块都支持（诊断用的场景 D）能走完，B 走不完。推导里用到的脚本如果整本不支持，
// 列出它的不支持块。运行时一个文件有不支持的块，整段对话都会被拒绝（选块的第 2 条），所以只能整本放开，要先修这些块。
func (a *analysis) writeDOnly(b *strings.Builder) {
	b.WriteString("## 整本放不开的任务\n\n")
	b.WriteString("诊断用：逐块都支持（不看整本）能走完、但场景 B 走不完的任务。它们卡在脚本里有不支持的块，整本放不开；列出的是这条推导用到的、整本不支持的脚本，以及它的不支持块。\n\n")
	n := 0
	for _, row := range a.rows {
		end := Fact{Kind: KindEnd, N: row.q.EndEvent}
		if row.reach[scenB] || !a.closD.Has[end] {
			continue
		}
		n++
		der := a.closD.Derive(end)
		fmt.Fprintf(b, "### %d %s（事件 %d）\n\n", row.q.No, row.q.Name, row.q.EndEvent)
		for _, f := range unitFiles(der.Units) {
			if a.rules.Full(f) || a.rules.Enabled(f) {
				continue
			}
			fmt.Fprintf(b, "- 脚本 `%s`（整本放不开），这条推导用到的块：%s\n", f, indexList(der.Units, f))
			for _, u := range a.m.Units {
				if u.File == f && u.Block.Unsupported != "" {
					fmt.Fprintf(b, "  - 块 %d（%s，事件 %d）：%s\n", u.Index, u.Block.Type, u.Block.Event, u.Block.Unsupported)
				}
			}
		}
		b.WriteString("\n")
	}
	if n == 0 {
		b.WriteString("无。\n\n")
	}
}

// indexList 列出推导里属于脚本 file 的块序号（升序，用顿号连起来）。
func indexList(units []*Unit, file string) string {
	var idx []int
	for _, u := range units {
		if u.File == file {
			idx = append(idx, u.Index)
		}
	}
	sort.Ints(idx)
	parts := make([]string, len(idx))
	for i, k := range idx {
		parts[i] = fmt.Sprint(k)
	}
	return strings.Join(parts, "、")
}

// writeLists 写清单：外部补给道具、需要自备的宠物、需要说的暗号。清单按每个任务的推荐推导（最窄的能走完的场景）汇总。
// writeKeywords 写暗号：数据里所有暗号块（KeyWord），以及有效果的（给道具或设事件）。没有效果的对能否走完没有影响。
// factName 把事实写成中文：完成事件、进行中事件、道具、宠物模板。
// writeIDList 写一组编号（道具或宠物模板）：编号、名字、用到它的任务。
// writeEventList 写外部事件：事件号、完成还是进行中、用到它的任务，以及来源。
// joinNos 把任务编号排序后用顿号连起来（去重）。
func joinNos(nos []int) string {
	sort.Ints(nos)
	var parts []string
	for i, n := range nos {
		if i > 0 && nos[i-1] == n {
			continue
		}
		parts = append(parts, fmt.Sprint(n))
	}
	return strings.Join(parts, "、")
}

// codeList 把脚本文件写成用顿号连起来的代码片段。
func codeList(files []string) string {
	parts := make([]string, len(files))
	for i, f := range files {
		parts[i] = "`" + f + "`"
	}
	return strings.Join(parts, "、")
}

// itemName、petName 取名字（测试里没有名字表时返回空串）。
