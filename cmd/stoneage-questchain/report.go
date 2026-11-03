package main

// 报告（STAGE-213，docs/quest-chains.md）：摘要和任务表。输出只依赖数据，不带时间戳，排序固定，两次生成逐字相同。

import (
	"fmt"
	"sort"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
)

// 场景的下标：0 是 A，1 是 B，2 是 C。
const (
	scenA = iota
	scenB
	scenC
)

var scenNames = [3]string{"A 现状", "B 开 all", "C 理想"}

// analysis 是三个场景的结果和每个任务的推导。closD 是诊断用的场景 D：白名单加上所有脚本里支持的块（不看整本），
// 只用来找“块本身都支持、只是整本放不开”的脚本（报告里的“整本放不开的脚本”一节），不是正式场景。
type analysis struct {
	m      *Model
	rules  Rules
	clos   [3]*Closure
	closD  *Closure
	levels [3]map[Fact]int
	rows   []questRow
}

// questRow 是一个任务的结果：三个场景是否能走完，按最窄的能走完的场景算的最低等级和推导。
type questRow struct {
	q        gamedata.Quest
	scripted bool // 完成事件由任务脚本设置（EndSetFlg）；否则是别的系统完成的，不在本分析范围
	reach    [3]bool
	pick     int // 最窄的能走完的场景（0、1、2），都不能时为 -1
	level    int
	der      Derivation
	files    []string // 推导里用到的脚本文件（去重排序）
}

// analyze 跑三个场景的闭包和等级需求，并算每个任务的推导。
func analyze(m *Model, r Rules) *analysis {
	a := &analysis{m: m, rules: r}
	a.clos = [3]*Closure{m.CloseA(r), m.CloseB(r), m.CloseC(r)}
	a.closD = m.Close(r.allowA, func(u *Unit) bool { return u.Block.Unsupported == "" })
	a.levels = [3]map[Fact]int{m.Levels(r.AllowA()), m.Levels(r.AllowB()), m.Levels(r.AllowC())}
	for _, q := range m.Quests {
		row := questRow{q: q, pick: -1, scripted: m.setEnd[q.EndEvent]}
		end := Fact{Kind: KindEnd, N: q.EndEvent}
		for s := scenA; s <= scenC; s++ {
			row.reach[s] = a.clos[s].Has[end]
			if row.reach[s] && row.pick < 0 {
				row.pick = s
			}
		}
		if row.pick >= 0 {
			row.level = a.levels[row.pick][end]
			row.der = a.clos[row.pick].Derive(end)
			row.files = unitFiles(row.der.Units)
		}
		a.rows = append(a.rows, row)
	}
	return a
}

// unitFiles 返回推导里块所在的脚本文件（去重，升序）。
func unitFiles(units []*Unit) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range units {
		if !seen[u.File] {
			seen[u.File] = true
			out = append(out, u.File)
		}
	}
	sort.Strings(out)
	return out
}

// count 数场景 s 里能走完的任务和完成事件。
func (a *analysis) count(s int) (quests, events int) {
	for _, row := range a.rows {
		if row.reach[s] {
			quests++
		}
	}
	for f := range a.clos[s].Has {
		if f.Kind == KindEnd {
			events++
		}
	}
	return quests, events
}

// renderReport 生成整份报告。
func renderReport(m *Model, r Rules) string {
	a := analyze(m, r)
	var b strings.Builder
	b.WriteString("# 任务链可走性评估（STAGE-213）\n\n")
	b.WriteString("> 由 `go run ./cmd/stoneage-questchain -o docs/quest-chains.md` 生成，不要手改。数据来自 `server/gamedata` 的任务脚本、任务表和白名单。\n\n")
	a.writeSummary(&b)
	a.writeTable(&b)
	a.writeUnscripted(&b)
	a.writeBFromA(&b)
	a.writeCOnly(&b)
	a.writeDOnly(&b)
	a.writeLists(&b)
	a.writeBatches(&b)
	writeMethod(&b)
	return b.String()
}

// writeSummary 写摘要：任务数、三个场景各能走完几个，以及脚本的分类。
func (a *analysis) writeSummary(b *strings.Builder) {
	enabled, full := 0, 0
	for _, f := range a.m.Files {
		switch {
		case a.rules.Enabled(f):
			enabled++
		case a.rules.Full(f):
			full++
		}
	}
	fmt.Fprintf(b, "## 摘要\n\n")
	scripted := 0
	for _, row := range a.rows {
		if row.scripted {
			scripted++
		}
	}
	fmt.Fprintf(b, "- 任务总数：%d（`quests.tsv`），其中 %d 个由任务脚本完成（完成事件有 EndSetFlg），其余 %d 个由别的系统完成，不在本分析范围（见“不由任务脚本完成的任务”）。\n",
		len(a.rows), scripted, len(a.rows)-scripted)
	fmt.Fprintf(b, "- 任务脚本的文件：%d 个，其中白名单（`npcEventEnabled`）%d 个，整本支持但没放开 %d 个，其余含不支持的块。\n",
		len(a.m.Files), enabled, full)
	for s := scenA; s <= scenC; s++ {
		q, e := a.count(s)
		fmt.Fprintf(b, "- 场景 %s：能走完 **%d** 个由脚本完成的任务（完成事件 %d 个）。\n", scenNames[s], q, e)
	}
	b.WriteString("\n场景含义：A 只放白名单；B 在白名单之外再放开整本支持的脚本（和 `STONEAGE_NPC_EVENT_OPEN=all` 一样）；C 放开所有脚本和所有块，忽略 `Unsupported`。\n")
	b.WriteString("“能走完”是必要条件（单调近似，见下文方法），不是证明：标 ✓ 的还要用端到端测试验证，标 ✗ 的是确定走不通。\n\n")
}

// writeTable 写任务表：每个任务一行。
func (a *analysis) writeTable(b *strings.Builder) {
	b.WriteString("## 任务表\n\n")
	b.WriteString("最低等级是最窄的能走完的场景里，链路上 LV 条件要求的最大值（取所有推导里的最小）；“—”表示没有等级要求或走不通。\n")
	b.WriteString("脚本数是推导里用到的脚本文件数（同一场景下的第一条推导，见方法）。\n\n")
	b.WriteString("| 编号 | 任务 | 完成事件 | A | B | C | 最低等级 | 脚本数 |\n")
	b.WriteString("| ---: | --- | ---: | :-: | :-: | :-: | ---: | ---: |\n")
	for _, row := range a.rows {
		if !row.scripted {
			fmt.Fprintf(b, "| %d | %s | %d | — | — | — | — | — |\n", row.q.No, cell(row.q.Name), row.q.EndEvent)
			continue
		}
		level, scripts := "—", "—"
		if row.pick >= 0 {
			scripts = fmt.Sprint(len(row.files))
			if row.level > 0 {
				level = fmt.Sprint(row.level)
			}
		}
		fmt.Fprintf(b, "| %d | %s | %d | %s | %s | %s | %s | %s |\n",
			row.q.No, cell(row.q.Name), row.q.EndEvent, mark(row.reach[scenA]), mark(row.reach[scenB]), mark(row.reach[scenC]), level, scripts)
	}
	b.WriteString("\n")
}

// mark 把能否走完写成 ✓ 或 ✗。
func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

// cell 转义表格里的竖线，避免打乱表格。
func cell(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
